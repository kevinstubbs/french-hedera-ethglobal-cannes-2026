package naryo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type filterRow struct {
	ID, Hash, Name string
}

func pipelineFilterName(sessionID string, plan pipelineFilterPlan) string {
	if plan.HederaTopicID != "" {
		return "pf-" + sessionID + "-hcs"
	}
	if plan.EVMContract != "" {
		return "pf-" + sessionID + "-evm"
	}
	return "pf-" + sessionID + "-x"
}

func (c *HTTPClient) deleteFilter(ctx context.Context, id, hash string) (string, error) {
	body := map[string]string{"prevItemHash": hash}
	return c.req202(ctx, http.MethodDelete, "/api/v1/filters/"+id, body)
}

func (c *HTTPClient) postFilterBody(ctx context.Context, plan pipelineFilterPlan, sessionID string) (opID string, wantName string, err error) {
	wantName = pipelineFilterName(sessionID, plan)
	var body map[string]any
	switch {
	case plan.HederaTopicID != "":
		body = map[string]any{
			"type":           "TRANSACTION",
			"name":           wantName,
			"nodeId":         c.hederaNodeID,
			"identifierType": "IDENTITY_ID",
			"value":          plan.HederaTopicID,
			"statuses":       []string{"FAILED", "CONFIRMED", "UNCONFIRMED"},
		}
	case plan.EVMContract != "":
		body = map[string]any{
			"type":   "EVENT_CONTRACT",
			"name":   wantName,
			"nodeId": c.ethereumNodeID,
			"specification": map[string]any{
				"eventSignature": "Transfer(address,address,uint256)",
			},
			"statuses": []string{"CONFIRMED", "UNCONFIRMED"},
			"filterSyncState": map[string]any{
				"strategy":     "BLOCK_BASED",
				"initialBlock": 0,
			},
			"visibilityConfiguration": map[string]any{
				"visible": true,
			},
			"contractAddress": plan.EVMContract,
		}
	default:
		return "", "", errors.New("naryo: internal: postFilterBody without topic or contract")
	}
	opID, err = c.post202(ctx, "/api/v1/filters", body)
	return opID, wantName, err
}

func (c *HTTPClient) findFilterByNameWithRetry(ctx context.Context, wantName string) (filterRow, error) {
	var last error
	for attempt := 0; attempt < 12; attempt++ {
		row, err := c.findFilterByName(ctx, wantName)
		if err == nil {
			return row, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return filterRow{}, ctx.Err()
		case <-time.After(350 * time.Millisecond):
		}
	}
	return filterRow{}, last
}

func (c *HTTPClient) findFilterByName(ctx context.Context, wantName string) (filterRow, error) {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/filters", &list); err != nil {
		return filterRow{}, err
	}
	for _, row := range list {
		if row == nil {
			continue
		}
		n, _ := row["name"].(string)
		if strings.TrimSpace(n) != wantName {
			continue
		}
		id := fmt.Sprint(row["id"])
		h, _ := row["currentItemHash"].(string)
		if id == "" || h == "" {
			continue
		}
		return filterRow{ID: id, Hash: h, Name: n}, nil
	}
	return filterRow{}, fmt.Errorf("naryo: filter %q not found after create", wantName)
}

func (c *HTTPClient) filterExists(ctx context.Context, id string) (bool, error) {
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/filters", &list); err != nil {
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
