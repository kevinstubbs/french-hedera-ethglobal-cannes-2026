package hcs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestEnvelopeDeterministicJSON(t *testing.T) {
	ts := time.Date(2026, 4, 4, 12, 0, 0, 123456789, time.UTC)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger := NewLogger(WithSlog(log), WithClock(fixedClock{t: ts}))

	ctx := context.Background()
	logger.PipelineCreated(ctx, "sess1", "agent-a")

	// slog JSON wraps the message; extract our envelope from the "msg" field.
	var outer struct {
		Msg string `json:"msg"`
	}
	line := buf.Bytes()
	if i := bytes.IndexByte(line, '{'); i >= 0 {
		line = line[i:]
	}
	if err := json.Unmarshal(line, &outer); err != nil {
		t.Fatalf("unmarshal slog line: %v\nraw=%s", err, buf.String())
	}
	var env Envelope
	if err := json.Unmarshal([]byte(outer.Msg), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v msg=%q", err, outer.Msg)
	}
	if env.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion: %q", env.SchemaVersion)
	}
	if env.EventType != EventPipelineCreated {
		t.Fatalf("eventType: %q", env.EventType)
	}
	if env.Timestamp != FormatTimestamp(ts) {
		t.Fatalf("timestamp: got %q want %q", env.Timestamp, FormatTimestamp(ts))
	}
	if env.SessionID != "sess1" {
		t.Fatalf("sessionId: %q", env.SessionID)
	}
	var p PayloadPipelineCreated
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.AgentID != "agent-a" {
		t.Fatalf("agentId: %q", p.AgentID)
	}

	// Golden: full envelope JSON key order (struct order).
	want := `{"schemaVersion":"1","eventType":"pipeline_created","timestamp":"2026-04-04T12:00:00.123456789Z","sessionId":"sess1","payload":{"agentId":"agent-a"}}`
	if strings.TrimSpace(outer.Msg) != want {
		t.Fatalf("golden mismatch\ngot:  %s\nwant: %s", outer.Msg, want)
	}
}

func TestBillingTickPayloadOrder(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger := NewLogger(WithSlog(log), WithClock(fixedClock{t: ts}))

	logger.BillingTick(context.Background(), "s9", 7, 3)

	var outer struct {
		Msg string `json:"msg"`
	}
	raw := buf.Bytes()
	if i := bytes.IndexByte(raw, '{'); i >= 0 {
		raw = raw[i:]
	}
	_ = json.Unmarshal(raw, &outer)
	want := `{"schemaVersion":"1","eventType":"billing_tick","timestamp":"2026-01-02T03:04:05Z","sessionId":"s9","payload":{"billedSeconds":7,"rateCentsPerSecond":3}}`
	if strings.TrimSpace(outer.Msg) != want {
		t.Fatalf("golden mismatch\ngot:  %s\nwant: %s", outer.Msg, want)
	}
}
