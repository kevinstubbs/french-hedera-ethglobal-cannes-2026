package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

func TestStreamableHTTPRentPipeline(t *testing.T) {
	ctx := context.Background()
	svc := pipeline.NewService(pipeline.NewMemoryStore(), &naryo.MockClient{}, nil, 1, nil)
	ts := httptest.NewServer(StreamableHTTPHandler(svc))
	t.Cleanup(ts.Close)

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "streamable-test", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	res, err := sess.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "rent_pipeline",
		Arguments: map[string]any{"agentId": "http-agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, out, 201)
	id, _ := bodyMap(t, out)["id"].(string)
	if id == "" {
		t.Fatalf("expected id: %v", out)
	}
}
