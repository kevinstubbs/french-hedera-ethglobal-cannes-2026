package naryo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HTTPClient calls Naryo’s Configuration API. Pause/resume mapping:
//   - PauseEgress: DELETE the session’s HTTP broadcaster (egress off; configuration kept).
//   - ResumeEgress / EnsurePipeline: POST a broadcaster again on the same configuration.
//   - StopPipeline: DELETE broadcaster (if any), then DELETE broadcaster-configuration.
type HTTPClient struct {
	base   string
	ingest string
	dest   string
	http   *http.Client
	poll   time.Duration

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
	lastOpID        string
}

// HTTPClientConfig configures [NewHTTPClient].
type HTTPClientConfig struct {
	BaseURL           string // required, e.g. http://127.0.0.1:6060
	PlatformIngestURL string // required, base URL for HTTP broadcaster endpoint
	ActiveDestPath    string // path for ALL target, default /internal/naryo/v1/events
	HTTP              *http.Client
	PollInterval      time.Duration
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
	return &HTTPClient{
		base:   strings.TrimRight(cfg.BaseURL, "/"),
		ingest: strings.TrimRight(cfg.PlatformIngestURL, "/"),
		dest:   dest,
		http:   hc,
		poll:   poll,
		sess:   make(map[string]*httpSessState),
	}, nil
}

// EnsurePipeline provisions broadcaster-configuration + broadcaster for the session (idempotent).
func (c *HTTPClient) EnsurePipeline(ctx context.Context, sessionID string) (string, error) {
	if c == nil {
		return "", errors.New("naryo: nil HTTPClient")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.sess[sessionID]
	if st == nil {
		st = &httpSessState{}
		c.sess[sessionID] = st
	}

	if st.broadcasterID != "" {
		if ok, _ := c.broadcasterExists(ctx, st.broadcasterID); ok {
			c.stats.EnsureOK++
			return st.lastOpID, nil
		}
		st.broadcasterID = ""
		st.broadcasterHash = ""
	}

	if st.configID == "" {
		if err := c.createConfig(ctx, st); err != nil {
			c.recordErrUnlocked(err)
			return "", err
		}
	}

	if err := c.createBroadcaster(ctx, st); err != nil {
		c.recordErrUnlocked(err)
		return "", err
	}
	c.stats.EnsureOK++
	c.stats.LastErr = ""
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
func (c *HTTPClient) ResumeEgress(ctx context.Context, sessionID string) error {
	if c == nil {
		return errors.New("naryo: nil HTTPClient")
	}
	_, err := c.EnsurePipeline(ctx, sessionID)
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
	if st.configID != "" && st.configHash != "" {
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
		"mode":        "http",
		"healthy":     healthy,
		"baseURL":     c.base,
		"ingestURL":   c.ingest,
		"destPath":    c.dest,
		"sessions":    len(c.sess),
		"ensureOK":    c.stats.EnsureOK,
		"pauseOK":     c.stats.PauseOK,
		"resumeOK":    c.stats.ResumeOK,
		"stopOK":      c.stats.StopOK,
		"lastChecked": time.Now().UTC().Format(time.RFC3339Nano),
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

func (c *HTTPClient) createBroadcaster(ctx context.Context, st *httpSessState) error {
	if st.configID == "" {
		return errors.New("naryo: missing broadcaster-configuration id")
	}
	body := map[string]any{
		"configurationId": st.configID,
		"target": map[string]any{
			"type":         "ALL",
			"destinations": []string{c.dest},
		},
	}
	op, err := c.post202(ctx, "/api/v1/broadcasters", body)
	if err != nil {
		return err
	}
	if err := c.waitOperation(ctx, op); err != nil {
		return err
	}
	br, err := c.findBroadcaster(ctx, st.configID)
	if err != nil {
		return err
	}
	st.broadcasterID = br.ID
	st.broadcasterHash = br.Hash
	st.lastOpID = op
	return nil
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

func (c *HTTPClient) findBroadcaster(ctx context.Context, configurationID string) (broadcasterRow, error) {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/broadcasters", &list); err != nil {
		return broadcasterRow{}, err
	}
	for _, row := range list {
		if row == nil {
			continue
		}
		if fmt.Sprint(row["configurationId"]) != configurationID {
			continue
		}
		tgt, _ := row["target"].(map[string]any)
		if tgt == nil || fmt.Sprint(tgt["type"]) != "ALL" {
			continue
		}
		h, _ := row["currentItemHash"].(string)
		if h == "" {
			continue
		}
		return broadcasterRow{ID: fmt.Sprint(row["id"]), Hash: h}, nil
	}
	return broadcasterRow{}, errors.New("naryo: broadcaster not found after create")
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
