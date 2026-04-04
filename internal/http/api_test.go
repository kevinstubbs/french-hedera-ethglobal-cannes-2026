package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/http/middleware"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/x402test"
)

func newTestStack(t *testing.T) (*httptest.Server, *http.Client, *pipeline.Service) {
	t.Helper()
	xcfg := x402test.TestX402Config()
	store := pipeline.NewMemoryStore()
	svc := pipeline.NewService(store, &naryo.MockClient{}, nil, 1, nil)
	api := &API{Svc: svc}
	mux := NewMux(api)
	gate := middleware.PaymentGate(xcfg, x402test.MockFacilitator{}, true)(mux)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", api.Health)
	root.Handle("/v1/", gate)

	ts := httptest.NewServer(root)
	t.Cleanup(ts.Close)
	c := &http.Client{Transport: &x402test.AutoPayTransport{Base: http.DefaultTransport}}
	return ts, c, svc
}

func TestHealthzUnprotected(t *testing.T) {
	ts, _, _ := newTestStack(t)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestCreatePipelineRequiresPayment(t *testing.T) {
	ts, _, _ := newTestStack(t)
	client := ts.Client()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/pipelines", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestCreateAndStartWithPayment(t *testing.T) {
	ts, client, _ := newTestStack(t)
	resp, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d", resp.StatusCode)
	}
	var out struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" || out.State != "created" {
		t.Fatalf("unexpected body: %+v", out)
	}

	startURL := ts.URL + "/v1/pipelines/" + out.ID + "/start"
	resp2, err := client.Post(startURL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("start: status %d", resp2.StatusCode)
	}

	st, err := client.Get(ts.URL + "/v1/pipelines/" + out.ID + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Body.Close()
	if st.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", st.StatusCode)
	}
	var status struct {
		State               string `json:"state"`
		PaymentStreamActive bool   `json:"paymentStreamActive"`
	}
	if err := json.NewDecoder(st.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != "running" || !status.PaymentStreamActive {
		t.Fatalf("unexpected status: %+v", status)
	}
}
