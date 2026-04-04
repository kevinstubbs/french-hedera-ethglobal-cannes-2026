package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

func wantStatus(t *testing.T, m map[string]any, want int) {
	t.Helper()
	var got int
	switch v := m["statusCode"].(type) {
	case float64:
		got = int(v)
	case int:
		got = v
	case int64:
		got = int(v)
	default:
		t.Fatalf("statusCode: unexpected type %T in %v", v, m)
	}
	if got != want {
		t.Fatalf("statusCode: got %d want %d (full=%v)", got, want, m)
	}
}

func toolMap(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func bodyMap(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	b, ok := m["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body object in %v", m)
	}
	return b
}

// TestAgentPipelineDemo exercises MCP tools against [pipeline.Service] (in-process, same as folded API server).
func TestAgentPipelineDemo(t *testing.T) {
	ctx := context.Background()
	svc := pipeline.NewService(pipeline.NewMemoryStore(), &naryo.MockClient{}, nil, 1, nil)
	srv := NewPipelineServer(svc)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "e2e-agent", Version: "1"}, nil)

	t1, t2 := mcpsdk.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	})

	rent, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "rent_pipeline",
		Arguments: map[string]any{"agentId": "demo-agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rentOut := toolMap(t, rent)
	wantStatus(t, rentOut, 201)
	id, _ := bodyMap(t, rentOut)["id"].(string)
	if id == "" {
		t.Fatalf("missing pipeline id: %v", rentOut)
	}

	start, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "start_pipeline",
		Arguments: map[string]any{"pipelineId": id},
	})
	if err != nil {
		t.Fatal(err)
	}
	startOut := toolMap(t, start)
	wantStatus(t, startOut, 200)

	st, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "get_pipeline_status",
		Arguments: map[string]any{"pipelineId": id},
	})
	if err != nil {
		t.Fatal(err)
	}
	stOut := toolMap(t, st)
	b := bodyMap(t, stOut)
	if b["state"] != "running" {
		t.Fatalf("expected running, got %v", b["state"])
	}

	pause, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "pause_pipeline",
		Arguments: map[string]any{"pipelineId": id},
	})
	if err != nil {
		t.Fatal(err)
	}
	pauseOut := toolMap(t, pause)
	wantStatus(t, pauseOut, 200)

	_, err = clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "resume_pipeline",
		Arguments: map[string]any{"pipelineId": id},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "reconfigure_pipeline",
		Arguments: map[string]any{"pipelineId": id, "patch": map[string]any{"k": "v"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	stop, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "stop_pipeline",
		Arguments: map[string]any{"pipelineId": id},
	})
	if err != nil {
		t.Fatal(err)
	}
	stopOut := toolMap(t, stop)
	wantStatus(t, stopOut, 200)
}
