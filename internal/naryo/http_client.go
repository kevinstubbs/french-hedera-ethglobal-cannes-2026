package naryo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// HTTPClient calls Naryo’s Configuration API. Pause/resume mapping:
//   - PauseEgress: DELETE the session’s HTTP broadcaster (egress off; filter + configuration kept).
//   - ResumeEgress / EnsurePipeline: POST a broadcaster again (FILTER target when a pipeline filter exists).
//   - StopPipeline: DELETE broadcaster (if any), DELETE pipeline filter (if any), then DELETE broadcaster-configuration when not shared.
type HTTPClient struct {
	base           string
	ingest         string
	dest           string
	http           *http.Client
	poll           time.Duration
	hederaNodeID   string
	ethereumNodeID string
	// fixedBroadcasterConfigID, if set, skips POST create and reuses this configuration id.
	fixedBroadcasterConfigID string
	// skipEgressProvision skips all Configuration API provisioning (no HTTP broadcaster → platform from Naryo).
	skipEgressProvision bool

	mu     sync.Mutex
	sess   map[string]*httpSessState
	stats  httpClientStats
	pollMS atomic.Int64
}

type httpClientStats struct {
	EnsureOK  int64
	PauseOK   int64
	ResumeOK  int64
	StopOK    int64
	LastErr   string
	LastErrAt time.Time
}

type httpSessState struct {
	configID        string
	configHash      string
	broadcasterID   string
	broadcasterHash string
	filterID        string
	filterHash      string
	filterPlanKey   string
	lastOpID        string
	// When true, configID was reused (fixed id or fallback); do not DELETE configuration on StopPipeline.
	sharedBroadcasterConfig bool
}

// HTTPClientConfig configures [NewHTTPClient].
type HTTPClientConfig struct {
	BaseURL           string // required, e.g. http://127.0.0.1:6060
	PlatformIngestURL string // required, base URL for HTTP broadcaster endpoint
	ActiveDestPath    string // HTTP ingest base path (default /internal/naryo/v1/events); session id is appended per pipeline
	HederaNodeID      string // Configuration API UUID for Hedera node (defaults from env)
	EthereumNodeID    string // Configuration API UUID for EVM node (defaults from env)
	HTTP              *http.Client
	PollInterval      time.Duration
	// FixedBroadcasterConfigID skips POST /broadcaster-configurations and uses this id (from GET hash).
	FixedBroadcasterConfigID string
	// SkipEgressProvision skips broadcaster-configuration and broadcaster create (Naryo never POSTs chain events to the platform).
	SkipEgressProvision bool
}

// NewHTTPClient builds a real Configuration API client. All URLs must be non-empty.
func NewHTTPClient(cfg HTTPClientConfig) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.PlatformIngestURL) == "" {
		return nil, errors.New("naryo: BaseURL and PlatformIngestURL are required")
	}
	dest := strings.TrimSpace(cfg.ActiveDestPath)
	if dest == "" {
		dest = "/internal/naryo/v1/events"
	}
	if dest[0] != '/' {
		dest = "/" + dest
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	hNode := strings.TrimSpace(cfg.HederaNodeID)
	if hNode == "" {
		hNode = "7f3b8e1a-4d2c-4b9a-8e5f-1a2b3c4d5e6f"
	}
	eNode := strings.TrimSpace(cfg.EthereumNodeID)
	if eNode == "" {
		eNode = "eadc75b2-4217-4018-95af-f67c13058976"
	}
	return &HTTPClient{
		base:                     strings.TrimRight(cfg.BaseURL, "/"),
		ingest:                   strings.TrimRight(cfg.PlatformIngestURL, "/"),
		dest:                     dest,
		http:                     hc,
		poll:                     poll,
		hederaNodeID:             hNode,
		ethereumNodeID:           eNode,
		fixedBroadcasterConfigID: strings.TrimSpace(cfg.FixedBroadcasterConfigID),
		skipEgressProvision:      cfg.SkipEgressProvision,
		sess:                     make(map[string]*httpSessState),
	}, nil
}

// EnsurePipeline provisions broadcaster-configuration, a pipeline-scoped Naryo filter (unless ALL fallback),
// and an HTTP broadcaster targeting that filter (or ALL).
func (c *HTTPClient) EnsurePipeline(ctx context.Context, in EnsurePipelineArgs) (string, error) {
	if c == nil {
		return "", errors.New("naryo: nil HTTPClient")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return "", errors.New("naryo: empty session id")
	}
	if c.skipEgressProvision {
		c.mu.Lock()
		c.stats.EnsureOK++
		c.stats.LastErr = ""
		c.mu.Unlock()
		return "naryo-egress-skipped", nil
	}
	plan := resolvePipelineFilterPlan(in.Config)
	if !plan.UseALLFallback && plan.HederaTopicID == "" && plan.EVMContract == "" {
		return "", fmt.Errorf("naryo: pipeline filter plan: %s", plan.Reason)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.sess[sessionID]
	if st == nil {
		st = &httpSessState{}
		c.sess[sessionID] = st
	}

	if st.broadcasterID != "" {
		if ok, _ := c.broadcasterExists(ctx, st.broadcasterID); ok && plan.Key == st.filterPlanKey {
			c.stats.EnsureOK++
			c.clearErrUnlocked()
			return st.lastOpID, nil
		}
		st.broadcasterID = ""
		st.broadcasterHash = ""
	}

	// Session filter no longer matches config — delete old filter (broadcaster already cleared above).
	if st.filterID != "" && st.filterPlanKey != plan.Key {
		op, err := c.deleteFilter(ctx, st.filterID, st.filterHash)
		if err == nil {
			_ = c.waitOperation(ctx, op)
		}
		st.filterID = ""
		st.filterHash = ""
	}
	st.filterPlanKey = plan.Key

	if st.configID == "" {
		if err := c.createConfig(ctx, st); err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
	}

	if plan.UseALLFallback {
		if err := c.provisionALLBroadcasterUnlocked(ctx, st, sessionID); err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
		c.stats.EnsureOK++
		c.clearErrUnlocked()
		return st.lastOpID, nil
	}

	if st.filterID == "" {
		op, wantName, err := c.postFilterBody(ctx, plan, sessionID)
		if err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
		if err := c.waitOperation(ctx, op); err != nil {
			id, e2 := c.fallbackALLAfterRevisionHookFailure(ctx, st, sessionID, "filter", err)
			if e2 == nil {
				c.stats.EnsureOK++
				c.clearErrUnlocked()
				return id, nil
			}
			c.recordErrUnlocked(e2)
			return "", e2
		}
		fr, err := c.findFilterByNameWithRetry(ctx, wantName)
		if err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
		st.filterID = fr.ID
		st.filterHash = fr.Hash
		st.lastOpID = op
	} else if ok, _ := c.filterExists(ctx, st.filterID); !ok {
		st.filterID = ""
		st.filterHash = ""
		op, wantName, err := c.postFilterBody(ctx, plan, sessionID)
		if err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
		if err := c.waitOperation(ctx, op); err != nil {
			id, e2 := c.fallbackALLAfterRevisionHookFailure(ctx, st, sessionID, "filter", err)
			if e2 == nil {
				c.stats.EnsureOK++
				c.clearErrUnlocked()
				return id, nil
			}
			c.recordErrUnlocked(e2)
			return "", e2
		}
		fr, err := c.findFilterByNameWithRetry(ctx, wantName)
		if err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
		st.filterID = fr.ID
		st.filterHash = fr.Hash
		st.lastOpID = op
	}

	if err := c.createBroadcaster(ctx, st, sessionID, st.filterID, "FILTER"); err != nil {
		id, e2 := c.fallbackALLAfterRevisionHookFailure(ctx, st, sessionID, "FILTER broadcaster", err)
		if e2 == nil {
			c.stats.EnsureOK++
			c.clearErrUnlocked()
			return id, nil
		}
		c.recordErrUnlocked(e2)
		return "", e2
	}
	c.stats.EnsureOK++
	c.clearErrUnlocked()
	return st.lastOpID, nil
}

// PauseEgress removes the HTTP broadcaster for the session (egress paused).
func (c *HTTPClient) PauseEgress(ctx context.Context, sessionID string) error {
	if c == nil {
		return errors.New("naryo: nil HTTPClient")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.sess[sessionID]
	if st == nil || st.broadcasterID == "" {
		return nil
	}
	brID, brHash := st.broadcasterID, st.broadcasterHash
	op, err := c.deleteBroadcaster(ctx, brID, brHash)
	if err != nil {
		c.recordErrUnlocked(err)
		return err
	}
	if err := c.waitOperation(ctx, op); err != nil {
		c.recordErrUnlocked(err)
		return err
	}
	if s2 := c.sess[sessionID]; s2 != nil && s2.broadcasterID == brID {
		s2.broadcasterID = ""
		s2.broadcasterHash = ""
		s2.lastOpID = op
	}
	c.stats.PauseOK++
	c.clearErrUnlocked()
	return nil
}

// ResumeEgress re-attaches egress by ensuring a broadcaster exists.
func (c *HTTPClient) ResumeEgress(ctx context.Context, in EnsurePipelineArgs) error {
	if c == nil {
		return errors.New("naryo: nil HTTPClient")
	}
	_, err := c.EnsurePipeline(ctx, in)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.stats.ResumeOK++
	c.mu.Unlock()
	return nil
}

// StopPipeline tears down broadcaster and configuration for the session.
func (c *HTTPClient) StopPipeline(ctx context.Context, sessionID string) error {
	if c == nil {
		return errors.New("naryo: nil HTTPClient")
	}
	c.mu.Lock()
	st := c.sess[sessionID]
	if st != nil {
		delete(c.sess, sessionID)
	}
	c.mu.Unlock()
	if st == nil {
		return nil
	}
	if st.broadcasterID != "" && st.broadcasterHash != "" {
		op, err := c.deleteBroadcaster(ctx, st.broadcasterID, st.broadcasterHash)
		if err == nil {
			_ = c.waitOperation(ctx, op)
		}
	}
	if st.filterID != "" && st.filterHash != "" {
		op, err := c.deleteFilter(ctx, st.filterID, st.filterHash)
		if err == nil {
			_ = c.waitOperation(ctx, op)
		}
	}
	if st.configID != "" && st.configHash != "" && !st.sharedBroadcasterConfig {
		op, err := c.deleteConfig(ctx, st.configID, st.configHash)
		if err == nil {
			_ = c.waitOperation(ctx, op)
		}
	}
	c.mu.Lock()
	c.stats.StopOK++
	c.clearErr()
	c.mu.Unlock()
	return nil
}

// Stats implements the observability shape used by [github.com/french-hedera-ethglobal-cannes2026/submission/internal/http.ObservabilityDeps].
func (c *HTTPClient) Stats() map[string]any {
	if c == nil {
		return map[string]any{"mode": "nil"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	healthy := c.stats.LastErr == ""
	out := map[string]any{
		"mode":           "http",
		"healthy":        healthy,
		"baseURL":        c.base,
		"ingestURL":      c.ingest,
		"destPath":       c.dest,
		"hederaNodeId":   c.hederaNodeID,
		"ethereumNodeId": c.ethereumNodeID,
		"egressSkip":     c.skipEgressProvision,
		"sessions":       len(c.sess),
		"ensureOK":       c.stats.EnsureOK,
		"pauseOK":        c.stats.PauseOK,
		"resumeOK":       c.stats.ResumeOK,
		"stopOK":         c.stats.StopOK,
		"lastChecked":    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if v := c.pollMS.Load(); v > 0 {
		out["lastPollMs"] = v
	}
	if !healthy && c.stats.LastErr != "" {
		out["lastError"] = c.stats.LastErr
		out["lastErrorAt"] = c.stats.LastErrAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func (c *HTTPClient) recordErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordErrUnlocked(err)
}

func (c *HTTPClient) recordErrUnlocked(err error) {
	c.stats.LastErr = err.Error()
	c.stats.LastErrAt = time.Now().UTC()
}

func (c *HTTPClient) clearErr() {
	c.mu.Lock()
	c.clearErrUnlocked()
	c.mu.Unlock()
}

func (c *HTTPClient) clearErrUnlocked() {
	c.stats.LastErr = ""
}

func (c *HTTPClient) createConfig(ctx context.Context, st *httpSessState) error {
	if c.fixedBroadcasterConfigID != "" {
		id := c.fixedBroadcasterConfigID
		hash, err := c.getConfigHash(ctx, id)
		if err != nil {
			return fmt.Errorf("naryo: NARYO_BROADCASTER_CONFIGURATION_ID %q: %w", id, err)
		}
		st.configID = id
		st.configHash = hash
		st.sharedBroadcasterConfig = true
		return nil
	}

	id := newUUID()
	body := map[string]any{
		"id":   id,
		"type": "HTTP",
		"endpoint": map[string]string{
			"url": c.ingest,
		},
		"cache": map[string]string{"expirationTime": "5m"},
	}
	op, err := c.post202(ctx, "/api/v1/broadcaster-configurations", body)
	if err != nil {
		return err
	}
	if err := c.waitOperation(ctx, op); err != nil {
		if isBroadcasterConfigNPE(err) {
			if err2 := c.attachExistingConfigByEndpoint(ctx, st, c.ingest); err2 == nil {
				return nil
			}
			return fmt.Errorf("%w (reuse failed: no HTTP broadcaster-configuration matches ingest URL; set NARYO_BROADCASTER_CONFIGURATION_ID or try a newer NARYO_IMAGE / docker compose pull)", err)
		}
		return err
	}
	hash, err := c.getConfigHash(ctx, id)
	if err != nil {
		return err
	}
	st.configID = id
	st.configHash = hash
	st.lastOpID = op
	return nil
}

func isBroadcasterConfigNPE(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "NullPointerException") && strings.Contains(s, "operation")
}

// attachExistingConfigByEndpoint sets st from GET /broadcaster-configurations where type=HTTP and endpoint.url matches.
func (c *HTTPClient) attachExistingConfigByEndpoint(ctx context.Context, st *httpSessState, wantURL string) error {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/broadcaster-configurations", &list); err != nil {
		return err
	}
	want := normalizeIngestBaseURL(wantURL)
	for _, row := range list {
		if row == nil || fmt.Sprint(row["type"]) != "HTTP" {
			continue
		}
		ep, _ := row["endpoint"].(map[string]any)
		if ep == nil {
			continue
		}
		got := normalizeIngestBaseURL(fmt.Sprint(ep["url"]))
		if got != want {
			continue
		}
		cid := fmt.Sprint(row["id"])
		h, _ := row["currentItemHash"].(string)
		if cid == "" || h == "" {
			continue
		}
		st.configID = cid
		st.configHash = h
		st.sharedBroadcasterConfig = true
		return nil
	}
	return errors.New("naryo: no broadcaster-configuration matches NARYO_PLATFORM_INGEST_URL")
}

func normalizeIngestBaseURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

func isNaryoRevisionHookFailure(err error) bool {
	if err == nil {
		return false
	}
	lo := strings.ToLower(err.Error())
	return strings.Contains(lo, "onafterapply") ||
		strings.Contains(lo, "hook error") ||
		(strings.Contains(lo, "unexpected_error") && strings.Contains(lo, "runtimeexception"))
}

// provisionALLBroadcasterUnlocked removes any session filter, then creates an ALL-target HTTP broadcaster.
func (c *HTTPClient) provisionALLBroadcasterUnlocked(ctx context.Context, st *httpSessState, sessionID string) error {
	if st.filterID != "" {
		op, err := c.deleteFilter(ctx, st.filterID, st.filterHash)
		if err == nil {
			_ = c.waitOperation(ctx, op)
		}
		st.filterID = ""
		st.filterHash = ""
	}
	return c.createBroadcaster(ctx, st, sessionID, "", "ALL")
}

// fallbackALLAfterRevisionHookFailure recovers from Naryo async worker failures (e.g. onAfterApply RuntimeException)
// on some images when applying filters or FILTER broadcasters. When NARYO_ALLOW_ALL_BROADCASTER_FALLBACK is true
// (default), provisions an ALL-target broadcaster instead.
func (c *HTTPClient) fallbackALLAfterRevisionHookFailure(ctx context.Context, st *httpSessState, sessionID, phase string, scopedErr error) (string, error) {
	if scopedErr == nil {
		return "", nil
	}
	if !config.NaryoAllowAllBroadcasterFallback() || !isNaryoRevisionHookFailure(scopedErr) {
		return "", scopedErr
	}
	slog.Warn("naryo: scoped provisioning hit revision hook failure; falling back to ALL-target broadcaster",
		"phase", phase, "sessionId", sessionID, "err", scopedErr)
	st.filterPlanKey = "ALL"
	if err := c.provisionALLBroadcasterUnlocked(ctx, st, sessionID); err != nil {
		return "", fmt.Errorf("%w; ALL fallback failed: %v", scopedErr, err)
	}
	return st.lastOpID, nil
}

func (c *HTTPClient) ingestDestinationPath(sessionID string) string {
	p := strings.TrimSuffix(c.dest, "/")
	return p + "/" + url.PathEscape(sessionID)
}

func (c *HTTPClient) createBroadcaster(ctx context.Context, st *httpSessState, sessionID, filterID, targetType string) error {
	if st.configID == "" {
		return errors.New("naryo: missing broadcaster-configuration id")
	}
	destPath := c.ingestDestinationPath(sessionID)
	tgt := map[string]any{
		"type":         targetType,
		"destinations": []string{destPath},
	}
	if targetType == "FILTER" {
		if filterID == "" {
			return errors.New("naryo: FILTER broadcaster requires filterId")
		}
		tgt["filterId"] = filterID
	}
	body := map[string]any{
		"configurationId": st.configID,
		"target":          tgt,
	}
	op, err := c.post202(ctx, "/api/v1/broadcasters", body)
	if err != nil {
		return err
	}
	if err := c.waitOperation(ctx, op); err != nil {
		return err
	}
	br, err := c.findBroadcasterWithRetry(ctx, st.configID, destPath, targetType, filterID)
	if err != nil {
		return err
	}
	if br.Hash == "" {
		h, err := c.getBroadcasterHash(ctx, br.ID)
		if err != nil {
			return fmt.Errorf("naryo: broadcaster %s listed without hash: %w", br.ID, err)
		}
		br.Hash = h
	}
	st.broadcasterID = br.ID
	st.broadcasterHash = br.Hash
	st.lastOpID = op
	return nil
}

func (c *HTTPClient) getBroadcasterHash(ctx context.Context, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("naryo: empty broadcaster id")
	}
	var row map[string]any
	if err := c.getJSON(ctx, "/api/v1/broadcasters/"+url.PathEscape(id), &row); err != nil {
		return "", err
	}
	h, _ := row["currentItemHash"].(string)
	if strings.TrimSpace(h) == "" {
		return "", errors.New("naryo: GET broadcaster missing currentItemHash")
	}
	return h, nil
}

func (c *HTTPClient) findBroadcasterWithRetry(ctx context.Context, configurationID, destPath, targetType, filterID string) (broadcasterRow, error) {
	var last error
	for attempt := 0; attempt < 20; attempt++ {
		br, err := c.findBroadcaster(ctx, configurationID, destPath, targetType, filterID)
		if err == nil {
			return br, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return broadcasterRow{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return broadcasterRow{}, last
}

type broadcasterRow struct {
	ID, Hash string
}

func (c *HTTPClient) broadcasterExists(ctx context.Context, id string) (bool, error) {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/broadcasters", &list); err != nil {
		return false, err
	}
	for _, row := range list {
		if row == nil {
			continue
		}
		if fmt.Sprint(row["id"]) == id {
			return true, nil
		}
	}
	return false, nil
}

func (c *HTTPClient) findBroadcaster(ctx context.Context, configurationID, destPath, targetType, filterID string) (broadcasterRow, error) {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/broadcasters", &list); err != nil {
		return broadcasterRow{}, err
	}
	configurationID = strings.TrimSpace(configurationID)
	type scored struct {
		br    broadcasterRow
		score int
	}
	var best *scored
	for _, row := range list {
		if row == nil {
			continue
		}
		cid := broadcasterRowConfigurationID(row)
		tgt, _ := row["target"].(map[string]any)
		if tgt == nil || fmt.Sprint(tgt["type"]) != targetType {
			continue
		}
		if targetType == "FILTER" {
			if broadcasterTargetFilterID(tgt) != filterID {
				continue
			}
		}
		dests, _ := tgt["destinations"].([]any)
		if !destinationsInclude(dests, destPath) {
			continue
		}
		id := strings.TrimSpace(fmt.Sprint(row["id"]))
		if id == "" {
			continue
		}
		h, _ := row["currentItemHash"].(string)
		br := broadcasterRow{ID: id, Hash: h}
		score := 0
		if cid != "" && strings.EqualFold(cid, configurationID) {
			score += 10
		}
		if br.Hash != "" {
			score += 5
		}
		if best == nil || score > best.score ||
			(score == best.score && br.Hash != "" && best.br.Hash == "") {
			best = &scored{br: br, score: score}
		}
	}
	if best == nil {
		return broadcasterRow{}, errors.New("naryo: broadcaster not found after create")
	}
	return best.br, nil
}

func broadcasterRowConfigurationID(row map[string]any) string {
	if row == nil {
		return ""
	}
	for _, k := range []string{"configurationId", "configurationID", "configuration_id"} {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func broadcasterTargetFilterID(tgt map[string]any) string {
	if tgt == nil {
		return ""
	}
	for _, k := range []string{"filterId", "filterID", "filter_id"} {
		v, ok := tgt[k]
		if !ok || v == nil {
			continue
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func destinationsInclude(dests []any, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	wantNorm := strings.TrimSuffix(want, "/")
	for _, d := range dests {
		s := strings.TrimSpace(destinationString(d))
		if s == "" {
			continue
		}
		sNorm := strings.TrimSuffix(s, "/")
		if sNorm == wantNorm {
			return true
		}
		// Naryo may store a full URL; we register a path suffix per session.
		if strings.HasSuffix(sNorm, wantNorm) {
			return true
		}
		if strings.Contains(sNorm, wantNorm) && len(wantNorm) >= 12 {
			return true
		}
	}
	return false
}

func destinationString(d any) string {
	switch x := d.(type) {
	case string:
		return x
	case map[string]any:
		if v, ok := x["value"].(string); ok {
			return v
		}
	}
	return strings.TrimSpace(fmt.Sprint(d))
}

func (c *HTTPClient) getConfigHash(ctx context.Context, id string) (string, error) {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/broadcaster-configurations", &list); err != nil {
		return "", err
	}
	for _, row := range list {
		if row == nil {
			continue
		}
		if fmt.Sprint(row["id"]) == id {
			h, _ := row["currentItemHash"].(string)
			if h == "" {
				return "", errors.New("naryo: broadcaster-configuration missing hash")
			}
			return h, nil
		}
	}
	return "", errors.New("naryo: broadcaster-configuration not found after create")
}

func (c *HTTPClient) deleteBroadcaster(ctx context.Context, id, hash string) (string, error) {
	body := map[string]string{"prevItemHash": hash}
	return c.req202(ctx, http.MethodDelete, "/api/v1/broadcasters/"+id, body)
}

func (c *HTTPClient) deleteConfig(ctx context.Context, id, hash string) (string, error) {
	body := map[string]string{"prevItemHash": hash}
	return c.req202(ctx, http.MethodDelete, "/api/v1/broadcaster-configurations/"+id, body)
}

func (c *HTTPClient) post202(ctx context.Context, path string, body any) (string, error) {
	return c.req202(ctx, http.MethodPost, path, body)
}

func (c *HTTPClient) req202(ctx context.Context, method, path string, body any) (string, error) {
	u := c.base + path
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("naryo: %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
	}
	var wrap map[string]any
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return "", fmt.Errorf("naryo: parse 202 body: %w", err)
	}
	return extractOperationID(wrap)
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, out any) error {
	u := c.base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("naryo: GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}

func (c *HTTPClient) waitOperation(ctx context.Context, opID string) error {
	if opID == "" {
		return errors.New("naryo: empty operation id")
	}
	deadline, hasDeadline := ctx.Deadline()
	for {
		if hasDeadline && time.Now().After(deadline) {
			return fmt.Errorf("naryo: operation %s: context deadline exceeded", opID)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/operations/"+opID, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		start := time.Now()
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		c.pollMS.Store(time.Since(start).Milliseconds())
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("naryo: poll operation %s: %s: %s", opID, resp.Status, strings.TrimSpace(string(raw)))
		}
		var st map[string]any
		if err := json.Unmarshal(raw, &st); err != nil {
			return err
		}
		state, _ := st["state"].(string)
		switch state {
		case "SUCCEEDED":
			return nil
		case "FAILED":
			code, _ := st["errorCode"].(string)
			msg, _ := st["errorMessage"].(string)
			return fmt.Errorf("naryo: operation %s failed: %s %s", opID, code, msg)
		case "PENDING", "RUNNING", "":
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.poll):
			}
		default:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.poll):
			}
		}
	}
}

func extractOperationID(body map[string]any) (string, error) {
	if v, ok := body["value"].(string); ok && v != "" {
		return v, nil
	}
	if op, ok := body["operationId"].(map[string]any); ok {
		if v, ok := op["value"].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", errors.New("naryo: 202 response missing operation id")
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
