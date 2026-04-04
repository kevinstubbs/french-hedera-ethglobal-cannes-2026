// Package mcp exposes Model Context Protocol tools for pipeline control.
package mcp

import (
	"context"
	"errors"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// NewPipelineServer registers tools backed by [pipeline.Service] (same behavior as REST handlers).
func NewPipelineServer(svc *pipeline.Service) *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "pipeline-control", Version: "1.0.0"}, nil)

	type rentIn struct {
		AgentID string `json:"agentId" jsonschema:"Owning agent id for the rental session"`
	}
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "rent_pipeline",
		Description: "Create a new pipeline session (same as POST /v1/pipelines).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in rentIn) (*mcpsdk.CallToolResult, map[string]any, error) {
		sess, err := svc.Create(ctx, in.AgentID)
		if err != nil {
			return errToolResult(http.StatusInternalServerError, map[string]any{"error": err.Error()}, err)
		}
		return okToolResult(http.StatusCreated, map[string]any{
			"id":    sess.ID,
			"state": string(sess.State),
		})
	})

	type idIn struct {
		PipelineID string `json:"pipelineId" jsonschema:"Pipeline session id returned by rent_pipeline"`
	}
	for _, x := range []struct {
		name, desc string
		call       func(context.Context, string) error
	}{
		{"start_pipeline", "Start a created or paused pipeline.", svc.Start},
		{"stop_pipeline", "Stop the pipeline session.", svc.Stop},
		{"pause_pipeline", "Pause egress and billing stream.", svc.Pause},
		{"resume_pipeline", "Resume from paused.", svc.Resume},
	} {
		x := x
		mcpsdk.AddTool(s, &mcpsdk.Tool{Name: x.name, Description: x.desc}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in idIn) (*mcpsdk.CallToolResult, map[string]any, error) {
			if err := x.call(ctx, in.PipelineID); err != nil {
				st, body := mapPipelineErr(err)
				return errToolResult(st, body, err)
			}
			return okToolResult(http.StatusOK, map[string]string{"ok": "true"})
		})
	}

	type reconfIn struct {
		PipelineID string         `json:"pipelineId" jsonschema:"Pipeline session id"`
		Patch      map[string]any `json:"patch" jsonschema:"Opaque reconfiguration patch object"`
	}
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "reconfigure_pipeline",
		Description: "Apply a configuration patch (same as PUT /v1/pipelines/{id}/reconfigure).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in reconfIn) (*mcpsdk.CallToolResult, map[string]any, error) {
		if in.Patch == nil {
			in.Patch = map[string]any{}
		}
		if err := svc.Reconfigure(ctx, in.PipelineID, in.Patch); err != nil {
			st, body := mapPipelineErr(err)
			return errToolResult(st, body, err)
		}
		return okToolResult(http.StatusOK, map[string]string{"ok": "true"})
	})

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_pipeline_status",
		Description: "Fetch session state and billing counters (same as GET /v1/pipelines/{id}/status).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in idIn) (*mcpsdk.CallToolResult, map[string]any, error) {
		sess, err := svc.Status(in.PipelineID)
		if err != nil {
			st, body := mapPipelineErr(err)
			return errToolResult(st, body, err)
		}
		out := map[string]any{
			"id":                   sess.ID,
			"agentId":              sess.AgentID,
			"state":                string(sess.State),
			"billedSeconds":        sess.BilledSeconds,
			"paymentStreamActive":  sess.PaymentStreamActive,
			"rateCentsPerSecond":   sess.RateCentsPerSecond,
			"lastNaryoOpId":        sess.LastNaryoOpID,
			"config":               map[string]any{},
			"rateUnitsPerMinute":   sess.RateUnitsPerMinute,
			"chargedUnits":         sess.ChargedUnits,
			"summaryWindowMinutes": sess.SummaryWindowMinutes,
			"autoPausedForFunds":   sess.AutoPausedForFunds,
		}
		if len(sess.Config) > 0 {
			out["config"] = sess.Config
		}
		if sess.AgentID != "" {
			out["prepaidBalanceUnits"] = svc.PrepaidBalance(sess.AgentID)
		}
		return okToolResult(http.StatusOK, out)
	})

	return s
}

func mapPipelineErr(err error) (int, map[string]any) {
	switch {
	case errors.Is(err, pipeline.ErrNotFound):
		return http.StatusNotFound, map[string]any{"error": "not found"}
	case errors.Is(err, pipeline.ErrInvalidTransition):
		return http.StatusConflict, map[string]any{"error": err.Error()}
	case errors.Is(err, pipeline.ErrInsufficientPrepaid):
		return http.StatusConflict, map[string]any{"error": err.Error()}
	default:
		return http.StatusBadRequest, map[string]any{"error": err.Error()}
	}
}

func okToolResult(status int, body any) (*mcpsdk.CallToolResult, map[string]any, error) {
	return nil, map[string]any{"statusCode": status, "body": body}, nil
}

func errToolResult(status int, body map[string]any, err error) (*mcpsdk.CallToolResult, map[string]any, error) {
	return nil, map[string]any{"statusCode": status, "body": body}, err
}
