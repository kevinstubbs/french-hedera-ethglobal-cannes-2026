package mcp

import (
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// StreamableHTTPHandler serves the MCP streamable HTTP transport at a single URL (e.g. POST https://host/mcp).
// It uses stateless mode so clients only need JSON-RPC POST without session affinity.
// Pipeline tools run in-process via [pipeline.Service] (not via REST), so x402 on /v1 does not apply here.
func StreamableHTTPHandler(svc *pipeline.Service) http.Handler {
	mcpServer := NewPipelineServer(svc)
	return mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return mcpServer
	}, &mcpsdk.StreamableHTTPOptions{Stateless: true})
}
