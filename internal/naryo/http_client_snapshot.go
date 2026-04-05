package naryo

import (
	"context"
	"fmt"
	"slices"
)

// ConfigurationSnapshot implements [Client]: live GETs against Naryo’s Configuration API.
func (c *HTTPClient) ConfigurationSnapshot(ctx context.Context) (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("naryo: nil HTTPClient")
	}
	var filters, broadcasters, configs []map[string]any
	if err := c.getJSON(ctx, "/api/v1/filters", &filters); err != nil {
		return nil, fmt.Errorf("naryo filters: %w", err)
	}
	if err := c.getJSON(ctx, "/api/v1/broadcasters", &broadcasters); err != nil {
		return nil, fmt.Errorf("naryo broadcasters: %w", err)
	}
	if err := c.getJSON(ctx, "/api/v1/broadcaster-configurations", &configs); err != nil {
		return nil, fmt.Errorf("naryo broadcaster-configurations: %w", err)
	}
	toAny := func(rows []map[string]any) []any {
		out := make([]any, len(rows))
		for i := range rows {
			out[i] = rows[i]
		}
		return out
	}

	c.mu.Lock()
	skip := c.skipEgressProvision
	sids := make([]string, 0, len(c.sess))
	for sid := range c.sess {
		sids = append(sids, sid)
	}
	slices.Sort(sids)
	sessRows := make([]map[string]any, 0, len(sids))
	for _, sid := range sids {
		st := c.sess[sid]
		if st == nil {
			continue
		}
		sessRows = append(sessRows, map[string]any{
			"sessionId":                      sid,
			"filterId":                       st.filterID,
			"broadcasterId":                  st.broadcasterID,
			"broadcasterConfigurationId":     st.configID,
			"filterPlanKey":                  st.filterPlanKey,
			"sharedBroadcasterConfiguration": st.sharedBroadcasterConfig,
		})
	}
	c.mu.Unlock()

	out := map[string]any{
		"mode":                           "http",
		"configurationApiBaseURL":        c.base,
		"filters":                        toAny(filters),
		"filtersCount":                   len(filters),
		"broadcasters":                   toAny(broadcasters),
		"broadcastersCount":              len(broadcasters),
		"broadcasterConfigurations":      toAny(configs),
		"broadcasterConfigurationsCount": len(configs),
		"egressProvisionSkipped":         skip,
		"orchestratorSessions":           toAny(sessRows),
		"orchestratorSessionsCount":      len(sessRows),
	}
	appendNaryoSnapshotHints(out, skip, len(filters), len(broadcasters), sessRows)
	return out, nil
}

func appendNaryoSnapshotHints(out map[string]any, egressSkip bool, filtersCount, broadcastersCount int, sessions []map[string]any) {
	var hints []string
	if egressSkip {
		hints = append(hints, "NARYO_SKIP_EGRESS_PROVISION is enabled: start/resume does not call Naryo’s Configuration API, so filters/broadcasters/configs stay empty unless something else created them.")
	}
	if !egressSkip && filtersCount == 0 && broadcastersCount > 0 {
		hints = append(hints, "Naryo lists broadcasters but zero filters — normal for ALL-target pipelines (no FILTER row). Scoped sessions add filters named pf-<sessionId>-hcs or -evm; declarative filters from Naryo YAML also appear here.")
	}
	if !egressSkip && broadcastersCount == 0 {
		pausedLike := false
		for _, s := range sessions {
			fid, _ := s["filterId"].(string)
			bid, _ := s["broadcasterId"].(string)
			if fid != "" && bid == "" {
				pausedLike = true
				break
			}
		}
		switch {
		case pausedLike:
			hints = append(hints, "Naryo lists no HTTP broadcasters while the orchestrator still tracks a filter without a broadcaster—typical when the pipeline is paused (egress removed). Resume to recreate the broadcaster.")
		case len(sessions) == 0:
			hints = append(hints, "No broadcasters in Naryo and no in-memory orchestrator sessions. If you expect rows here, confirm start/resume succeeded against this NARYO_CONFIG_API_BASE and egress provisioning is not skipped.")
		}
	}
	if len(hints) == 0 {
		return
	}
	hs := make([]any, len(hints))
	for i := range hints {
		hs[i] = hints[i]
	}
	out["snapshotHints"] = hs
}
