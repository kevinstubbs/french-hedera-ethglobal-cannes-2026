package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	x402 "github.com/coinbase/x402/go"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/http/middleware"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/ledger"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/naryo"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/x402test"
)

type stackConfig struct {
	Facilitator x402.FacilitatorClient
	WithLedger  bool
	HederaCfg   config.Hedera
}

func newTestStackWith(t *testing.T, cfg stackConfig) (*httptest.Server, *http.Client, *pipeline.Service, *naryo.MockClient, *ledger.MemoryLedger) {
	t.Helper()
	t.Setenv("NARYO_INGEST_SECRET", "test-naryo-secret")

	xcfg := x402test.TestX402Config()
	fac := cfg.Facilitator
	if fac == nil {
		fac = x402test.MockFacilitator{}
	}
	nm := &naryo.MockClient{}
	store := pipeline.NewMemoryStore()
	var opts []pipeline.ServiceOption
	var memLed *ledger.MemoryLedger
	if cfg.WithLedger {
		memLed = ledger.NewMemoryLedger()
		opts = append(opts, pipeline.WithPrepaidLedger(memLed))
	}
	svc := pipeline.NewService(store, nm, nil, 1, nil, opts...)
	api := &API{Svc: svc, HederaCfg: cfg.HederaCfg}
	mux := NewMux(api)
	gate := middleware.PaymentGate(xcfg, fac, true)(mux)

	obs := &ObservabilityDeps{Svc: svc, Naryo: nm}
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", api.Health)
	root.HandleFunc("GET /observability/v1/summary", ObservabilitySummary(obs))
	root.HandleFunc("GET /observability/v1/pipelines/{id}", ObservabilityPipelineDetail(obs))
	RegisterInternalRoutes(root, api)
	root.Handle("/v1/", gate)

	ts := httptest.NewServer(root)
	t.Cleanup(ts.Close)
	c := &http.Client{Transport: &x402test.AutoPayTransport{Base: http.DefaultTransport}}
	return ts, c, svc, nm, memLed
}

func newTestStack(t *testing.T) (*httptest.Server, *http.Client, *pipeline.Service) {
	ts, c, svc, _, _ := newTestStackWith(t, stackConfig{})
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

func TestObservabilityPipelineDetail(t *testing.T) {
	ts, client, _ := newTestStack(t)
	resp, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"obs-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var cr struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		_ = resp.Body.Close()
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || cr.ID == "" {
		t.Fatalf("create pipeline: status %d id %q", resp.StatusCode, cr.ID)
	}

	notFound, err := http.Get(ts.URL + "/observability/v1/pipelines/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	_ = notFound.Body.Close()
	if notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 missing pipeline, got %d", notFound.StatusCode)
	}

	detail, err := http.Get(ts.URL + "/observability/v1/pipelines/" + cr.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer detail.Body.Close()
	if detail.StatusCode != http.StatusOK {
		t.Fatalf("detail: status %d", detail.StatusCode)
	}
	var body struct {
		Session map[string]any `json:"session"`
	}
	if err := json.NewDecoder(detail.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Session == nil {
		t.Fatal("expected session object")
	}
	if got, _ := body.Session["id"].(string); got != cr.ID {
		t.Fatalf("session.id: want %q got %q", cr.ID, got)
	}
}

func TestGetStatusDoesNotRequirePayment(t *testing.T) {
	ts, _, _ := newTestStack(t)
	client := ts.Client()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/pipelines/missing/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 without payment, got %d", resp.StatusCode)
	}
}

func TestGetStatusConflictWhenPrepaidExhausted(t *testing.T) {
	t.Setenv("PREPAID_DEV_AUTO_CREDIT_UNITS", "10000")
	ts, autoClient, _, _, led := newTestStackWith(t, stackConfig{WithLedger: true})
	if led == nil {
		t.Fatal("expected ledger")
	}
	cr, err := autoClient.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"ag-exhaust"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer cr.Body.Close()
	if cr.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(cr.Body)
		t.Fatalf("create: %d %s", cr.StatusCode, string(b))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(cr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	sr, err := autoClient.Post(ts.URL+"/v1/pipelines/"+out.ID+"/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(sr.Body)
		t.Fatalf("start: %d %s", sr.StatusCode, string(b))
	}
	led.TestingSetBalance("ag-exhaust", 0)

	plain := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/pipelines/"+out.ID+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	nopay, err := plain.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer nopay.Body.Close()
	if nopay.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(nopay.Body)
		t.Fatalf("expected 409 when prepaid empty on running session, got %d %s", nopay.StatusCode, string(b))
	}

	led.TestingSetBalance("ag-exhaust", 1)
	req2, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/pipelines/"+out.ID+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Accept", "application/json")
	ok, err := plain.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(ok.Body)
		t.Fatalf("expected 200 when prepaid > 0, got %d %s", ok.StatusCode, string(b))
	}
}

func TestOnlyPipelineStartRequiresPaymentWithoutAutoPay(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	client := ts.Client()
	pid := "deadbeef"
	agent := "agent1"

	t.Run("start", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/pipelines/"+pid+"/start", strings.NewReader(`{}`))
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
	})

	free := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v1/pipelines", `{"agentId":"` + agent + `"}`},
		{http.MethodPost, "/v1/pipelines/" + pid + "/stop", `{}`},
		{http.MethodPost, "/v1/pipelines/" + pid + "/pause", `{}`},
		{http.MethodPost, "/v1/pipelines/" + pid + "/resume", `{}`},
		{http.MethodPut, "/v1/pipelines/" + pid + "/reconfigure", `{"patch":{}}`},
		{http.MethodPut, "/v1/pipelines/" + pid + "/payment-stream", `{"active":true}`},
		{http.MethodPost, "/v1/agents/" + agent + "/topup/x402", `{"amountUnits":10,"idempotencyKey":"k1"}`},
		{http.MethodPost, "/v1/agents/" + agent + "/topup/deposit", `{"transactionId":"0.0.1","amountUnits":10}`},
		{http.MethodGet, "/v1/pipelines/" + pid + "/status", ""},
	}
	for _, tc := range free {
		tc := tc
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusPaymentRequired {
				t.Fatalf("unexpected 402 for non-start route, body=%s", string(body))
			}
		})
	}
}

func TestCreatePipelineWithoutPayment(t *testing.T) {
	ts, _, _ := newTestStack(t)
	client := ts.Client()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/pipelines", strings.NewReader(`{"agentId":"a1"}`))
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
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 without payment, got %d body=%s", resp.StatusCode, string(body))
	}
}

func TestMalformedPaymentSignatureRejected(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	client := ts.Client()
	cr, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"sig-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(cr.Body).Decode(&created)
	_ = cr.Body.Close()
	if cr.StatusCode != http.StatusCreated || created.ID == "" {
		t.Fatalf("create pipeline: %d", cr.StatusCode)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/pipelines/"+created.ID+"/start", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", "not-valid-base64!!!")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		t.Fatalf("unexpected success status %d", resp.StatusCode)
	}
}

func TestPaymentRejectedWhenFacilitatorVerifyInvalid(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{Facilitator: x402test.RejectVerifyFacilitator{}})
	plain := ts.Client()
	cr, err := plain.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(cr.Body).Decode(&out)
	_ = cr.Body.Close()
	if cr.StatusCode != http.StatusCreated || out.ID == "" {
		t.Fatalf("create: %d", cr.StatusCode)
	}
	client := &http.Client{Transport: &x402test.AutoPayTransport{Base: http.DefaultTransport}}
	resp, err := client.Post(ts.URL+"/v1/pipelines/"+out.ID+"/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected start to fail when verify rejects; got 200 body=%s", string(body))
	}
}

func TestPaymentFailsWhenFacilitatorVerifyErrors(t *testing.T) {
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{Facilitator: x402test.ErrorVerifyFacilitator{Err: errors.New("facilitator down")}})
	plain := ts.Client()
	cr, err := plain.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(cr.Body).Decode(&out)
	_ = cr.Body.Close()
	if cr.StatusCode != http.StatusCreated || out.ID == "" {
		t.Fatalf("create: %d", cr.StatusCode)
	}
	client := &http.Client{Transport: &x402test.AutoPayTransport{Base: http.DefaultTransport}}
	resp, err := client.Post(ts.URL+"/v1/pipelines/"+out.ID+"/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected start to fail when facilitator errors")
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

func TestStopPauseResumeReconfigurePaymentStreamWithPayment(t *testing.T) {
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{})
	resp, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}
	var cr struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&cr)
	_ = resp.Body.Close()
	if cr.ID == "" {
		t.Fatal("no id")
	}
	id := cr.ID

	mustOK := func(t *testing.T, method, url string, body string) {
		t.Helper()
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		r2, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r2.Body.Close()
		if r2.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(r2.Body)
			t.Fatalf("%s %s: %d %s", method, url, r2.StatusCode, string(b))
		}
	}

	mustOK(t, http.MethodPost, ts.URL+"/v1/pipelines/"+id+"/start", `{}`)
	mustOK(t, http.MethodPost, ts.URL+"/v1/pipelines/"+id+"/pause", `{}`)
	mustOK(t, http.MethodPost, ts.URL+"/v1/pipelines/"+id+"/resume", `{}`)
	mustOK(t, http.MethodPut, ts.URL+"/v1/pipelines/"+id+"/reconfigure", `{"patch":{"k":1}}`)
	mustOK(t, http.MethodPut, ts.URL+"/v1/pipelines/"+id+"/payment-stream", `{"active":false}`)
	mustOK(t, http.MethodPut, ts.URL+"/v1/pipelines/"+id+"/payment-stream", `{"active":true}`)
	mustOK(t, http.MethodPost, ts.URL+"/v1/pipelines/"+id+"/stop", `{}`)
}

func TestTopUpX402WithPayment(t *testing.T) {
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{WithLedger: true})
	resp, err := client.Post(ts.URL+"/v1/agents/agent-x/topup/x402", "application/json",
		strings.NewReader(`{"amountUnits":42,"idempotencyKey":"idem-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, string(b))
	}
}

func TestTopUpDepositWithPaymentSkipVerify(t *testing.T) {
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{
		WithLedger: true,
		HederaCfg:  config.Hedera{SkipTopupVerify: true},
	})
	resp, err := client.Post(ts.URL+"/v1/agents/agent-y/topup/deposit", "application/json",
		strings.NewReader(`{"transactionId":"0.0.123","amountUnits":7}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, string(b))
	}
}

func TestPairwiseAgentControlPlaneHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns node: run without -short")
	}
	ts, _, _, _, _ := newTestStackWith(t, stackConfig{})
	agentDir, err := filepath.Abs(filepath.Join("..", "..", "agents", "trading-signal"))
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("npm", "run", "build")
	build.Dir = agentDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("npm run build in trading-signal failed (skip pairwise): %v\n%s", err, string(out))
	}
	script := filepath.Join(agentDir, "scripts", "pairwise-control-plane.mjs")
	cmd := exec.Command("node", script, ts.URL)
	cmd.Dir = agentDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pairwise node script: %v\n%s", err, string(out))
	}
	if !bytes.Contains(out, []byte("PAIRWISE_OK")) {
		t.Fatalf("expected PAIRWISE_OK in output, got: %s", string(out))
	}
}

func TestE2E_CreateStartNaryoIngestStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e: run without -short")
	}
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{})
	resp, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"e2e"}`))
	if err != nil {
		t.Fatal(err)
	}
	var cr struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&cr)
	_ = resp.Body.Close()
	if cr.ID == "" {
		t.Fatal("no session id")
	}
	sr, err := client.Post(ts.URL+"/v1/pipelines/"+cr.ID+"/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("start: %d", sr.StatusCode)
	}

	ingBody := `{"sessionId":"` + cr.ID + `","eventId":"evt-1","payload":{"chain":"test"}}`
	ir, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(ingBody))
	if err != nil {
		t.Fatal(err)
	}
	ir.Header.Set("Content-Type", "application/json")
	ir.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	ingResp, err := ts.Client().Do(ir)
	if err != nil {
		t.Fatal(err)
	}
	defer ingResp.Body.Close()
	if ingResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(ingResp.Body)
		t.Fatalf("ingest: %d %s", ingResp.StatusCode, string(b))
	}

	st, err := client.Get(ts.URL + "/v1/pipelines/" + cr.ID + "/status")
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

	// Idempotent replay
	ir2, _ := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(ingBody))
	ir2.Header.Set("Content-Type", "application/json")
	ir2.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	ing2, err := ts.Client().Do(ir2)
	if err != nil {
		t.Fatal(err)
	}
	defer ing2.Body.Close()
	var ingJSON map[string]any
	_ = json.NewDecoder(ing2.Body).Decode(&ingJSON)
	if ingJSON["duplicate"] != true {
		t.Fatalf("expected duplicate true, got %#v", ingJSON)
	}
}

// TestE2E_FullPrepaidAndX402Flow exercises prepaid ledger enforcement, x402-gated top-up, then
// start, Naryo ingest, and status (same as production shape; mock facilitator auto-pays 402s).
func TestE2E_FullPrepaidAndX402Flow(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e: run without -short")
	}
	t.Setenv("PREPAID_DEV_AUTO_CREDIT_UNITS", "0")
	ts, client, _, _, _ := newTestStackWith(t, stackConfig{WithLedger: true})

	badCreate, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"e2e-ledger"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = badCreate.Body.Close()
	if badCreate.StatusCode != http.StatusConflict {
		t.Fatalf("create without prepaid: expected 409, got %d", badCreate.StatusCode)
	}

	tu, err := client.Post(ts.URL+"/v1/agents/e2e-ledger/topup/x402", "application/json",
		strings.NewReader(`{"amountUnits":10000,"idempotencyKey":"e2e-ledger-topup-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer tu.Body.Close()
	if tu.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(tu.Body)
		t.Fatalf("topup x402: %d %s", tu.StatusCode, string(b))
	}

	resp, err := client.Post(ts.URL+"/v1/pipelines", "application/json", strings.NewReader(`{"agentId":"e2e-ledger"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create pipeline: %d %s", resp.StatusCode, string(b))
	}
	var cr struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		t.Fatal(err)
	}
	if cr.ID == "" {
		t.Fatal("no session id")
	}

	sr, err := client.Post(ts.URL+"/v1/pipelines/"+cr.ID+"/start", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(sr.Body)
		t.Fatalf("start: %d %s", sr.StatusCode, string(b))
	}

	ingBody := `{"sessionId":"` + cr.ID + `","eventId":"evt-ledger-1","payload":{"source":"e2e","demo":true}}`
	ir, err := http.NewRequest(http.MethodPost, ts.URL+"/internal/naryo/v1/events", strings.NewReader(ingBody))
	if err != nil {
		t.Fatal(err)
	}
	ir.Header.Set("Content-Type", "application/json")
	ir.Header.Set("X-Naryo-Webhook-Secret", "test-naryo-secret")
	ingResp, err := ts.Client().Do(ir)
	if err != nil {
		t.Fatal(err)
	}
	defer ingResp.Body.Close()
	if ingResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(ingResp.Body)
		t.Fatalf("ingest: %d %s", ingResp.StatusCode, string(b))
	}

	st, err := client.Get(ts.URL + "/v1/pipelines/" + cr.ID + "/status")
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
}
