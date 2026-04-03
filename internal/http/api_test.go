package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	x402 "github.com/coinbase/x402/go"
	"github.com/coinbase/x402/go/types"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/http/middleware"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

type mockFacilitator struct{}

func (mockFacilitator) Verify(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.VerifyResponse, error) {
	return &x402.VerifyResponse{IsValid: true, Payer: "0xpayer"}, nil
}

func (mockFacilitator) Settle(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.SettleResponse, error) {
	return &x402.SettleResponse{Success: true, Transaction: "0xabc", Network: x402.Network("eip155:84532"), Payer: "0xpayer"}, nil
}

func (mockFacilitator) GetSupported(ctx context.Context) (x402.SupportedResponse, error) {
	return x402.SupportedResponse{
		Kinds: []x402.SupportedKind{
			{X402Version: 2, Scheme: "exact", Network: "eip155:84532"},
		},
		Extensions: []string{},
		Signers:    map[string][]string{},
	}, nil
}

func testX402Config() config.X402 {
	return config.X402{
		FacilitatorURL: "http://unused.example",
		PayTo:          "0x1111111111111111111111111111111111111111",
		Network:        "eip155:84532",
		Price:          "$0.001",
	}
}

func newTestStack(t *testing.T) (*httptest.Server, *http.Client, *pipeline.Service) {
	t.Helper()
	xcfg := testX402Config()
	store := pipeline.NewMemoryStore()
	svc := pipeline.NewService(store, &naryo.MockClient{}, nil, 1, nil)
	api := &API{Svc: svc}
	mux := NewMux(api)
	gate := middleware.PaymentGate(xcfg, mockFacilitator{}, true)(mux)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", api.Health)
	root.Handle("/v1/", gate)

	ts := httptest.NewServer(root)
	t.Cleanup(ts.Close)
	return ts, ts.Client(), svc
}

func paymentRequiredFrom402(t *testing.T, resp *http.Response) types.PaymentRequired {
	t.Helper()
	hdr := resp.Header.Get("PAYMENT-REQUIRED")
	if hdr == "" {
		t.Fatal("missing PAYMENT-REQUIRED header")
	}
	raw, err := base64.StdEncoding.DecodeString(hdr)
	if err != nil {
		t.Fatalf("decode PAYMENT-REQUIRED: %v", err)
	}
	var pr types.PaymentRequired
	if err := json.Unmarshal(raw, &pr); err != nil {
		t.Fatalf("unmarshal payment required: %v", err)
	}
	if len(pr.Accepts) == 0 {
		t.Fatal("expected accepts in PAYMENT-REQUIRED")
	}
	return pr
}

func doPaidJSON(t *testing.T, c *http.Client, method, urlStr, jsonBody string) *http.Response {
	t.Helper()
	do := func(sig string) *http.Response {
		req, err := http.NewRequest(method, urlStr, strings.NewReader(jsonBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if sig != "" {
			req.Header.Set("PAYMENT-SIGNATURE", sig)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := do("")
	if resp.StatusCode != http.StatusPaymentRequired {
		return resp
	}
	_, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	pr := paymentRequiredFrom402(t, resp)
	acc := pr.Accepts[0]
	if acc.Extra == nil {
		acc.Extra = map[string]any{}
	}
	acc.Extra["resourceUrl"] = urlStr
	pp := types.PaymentPayload{
		X402Version: 2,
		Payload:     map[string]any{"sig": "test"},
		Accepted:    acc,
	}
	raw, err := json.Marshal(pp)
	if err != nil {
		t.Fatal(err)
	}
	sig := base64.StdEncoding.EncodeToString(raw)
	return do(sig)
}

func TestHealthzUnprotected(t *testing.T) {
	ts, client, _ := newTestStack(t)
	resp, err := client.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestCreatePipelineRequiresPayment(t *testing.T) {
	ts, client, _ := newTestStack(t)
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
	resp := doPaidJSON(t, client, http.MethodPost, ts.URL+"/v1/pipelines", `{"agentId":"a1"}`)
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
	resp2 := doPaidJSON(t, client, http.MethodPost, startURL, `{}`)
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
