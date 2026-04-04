package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNaryoMockClientCallSequenceLifecycle(t *testing.T) {
	ts, client, _, nm, _ := newTestStackWith(t, stackConfig{})
	if nm.EnsureCalls != 0 {
		t.Fatalf("unexpected initial ensure calls: %d", nm.EnsureCalls)
	}

	resp, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"n1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var cr struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		t.Fatal(err)
	}
	if cr.ID == "" {
		t.Fatal("no id")
	}

	start, err := client.Post(ts.URL+"/v1/pipelines/"+cr.ID+"/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = start.Body.Close()
	if start.StatusCode != http.StatusOK {
		t.Fatalf("start %d", start.StatusCode)
	}
	if nm.EnsureCalls != 1 {
		t.Fatalf("after start, EnsureCalls=%d want 1", nm.EnsureCalls)
	}

	pause, err := client.Post(ts.URL+"/v1/pipelines/"+cr.ID+"/pause", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = pause.Body.Close()
	if nm.PauseCalls != 1 {
		t.Fatalf("PauseCalls=%d want 1", nm.PauseCalls)
	}

	resume, err := client.Post(ts.URL+"/v1/pipelines/"+cr.ID+"/resume", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resume.Body.Close()
	if nm.ResumeCalls != 1 {
		t.Fatalf("ResumeCalls=%d want 1", nm.ResumeCalls)
	}

	stop, err := client.Post(ts.URL+"/v1/pipelines/"+cr.ID+"/stop", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = stop.Body.Close()
	if nm.StopCalls != 1 {
		t.Fatalf("StopCalls=%d want 1", nm.StopCalls)
	}
}
