package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPostNaryoEventUnauthorized(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(`{"sessionId":"x","eventId":"e"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPostNaryoEventBadJSON(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(`not json`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostNaryoEventMissingFields(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(`{"sessionId":"only"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d %s", resp.StatusCode, string(body))
	}
}

func TestPostNaryoEventUnknownSession(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	body := `{"sessionId":"does-not-exist","eventId":"e1","payload":{}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPostNaryoEventSessionUnauthorized(t *testing.T) {
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{})
	cr, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"naryo-path-auth"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(cr.Body).Decode(&created)
	_ = cr.Body.Close()
	if created.ID == "" {
		t.Fatal("no pipeline id")
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events/"+created.ID, strings.NewReader(`{"id":"e"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPostNaryoEventSessionUnknownSession(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events/does-not-exist", strings.NewReader(`{"id":"e1"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPostNaryoEventSessionIngestAndStatus(t *testing.T) {
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{})
	cr, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"naryo-path-ingest"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(cr.Body).Decode(&created)
	_ = cr.Body.Close()
	if created.ID == "" {
		t.Fatal("no pipeline id")
	}
	sid := created.ID
	native := `{"transaction_id":"0xabc1","blockNumber":1}`
	ing1, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events/"+sid, strings.NewReader(native))
	if err != nil {
		t.Fatal(err)
	}
	ing1.Header.Set("Content-Type", "application/json")
	ing1.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	r1, err := ts.Client().Do(ing1)
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r1.Body)
		t.Fatalf("ingest: %d %s", r1.StatusCode, string(b))
	}

	st, err := client.Get(ts.URL + "/v1/pipelines/" + sid + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Body.Close()
	var status map[string]any
	if err := json.NewDecoder(st.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	evs, ok := status["recentNaryoEvents"].([]any)
	if !ok || len(evs) != 1 {
		t.Fatalf("expected one recentNaryoEvents, got %#v", status["recentNaryoEvents"])
	}
	ev0, _ := evs[0].(map[string]any)
	if ev0["eventId"] != "0xabc1" {
		t.Fatalf("eventId: got %#v", ev0["eventId"])
	}

	ing2, _ := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events/"+sid, strings.NewReader(native))
	ing2.Header.Set("Content-Type", "application/json")
	ing2.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	r2, err := ts.Client().Do(ing2)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	var ingJSON map[string]any
	_ = json.NewDecoder(r2.Body).Decode(&ingJSON)
	if ingJSON["duplicate"] != true {
		t.Fatalf("expected duplicate true, got %#v", ingJSON)
	}
}
