package httpapi

import (
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
