// Package x402test provides a mock facilitator and HTTP transport that satisfies
// x402 payment challenges for integration tests (API + MCP).
package x402test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	x402 "github.com/coinbase/x402/go"
	"github.com/coinbase/x402/go/types"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// MockFacilitator always verifies and settles successfully.
type MockFacilitator struct{}

func (MockFacilitator) Verify(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.VerifyResponse, error) {
	return &x402.VerifyResponse{IsValid: true, Payer: "0xpayer"}, nil
}

func (MockFacilitator) Settle(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.SettleResponse, error) {
	return &x402.SettleResponse{Success: true, Transaction: "0xabc", Network: x402.Network("eip155:84532"), Payer: "0xpayer"}, nil
}

func (MockFacilitator) GetSupported(ctx context.Context) (x402.SupportedResponse, error) {
	return x402.SupportedResponse{
		Kinds: []x402.SupportedKind{
			{X402Version: 2, Scheme: "exact", Network: "eip155:84532"},
		},
		Extensions: []string{},
		Signers:    map[string][]string{},
	}, nil
}

// RejectVerifyFacilitator verifies all payloads as invalid (payment rejected).
type RejectVerifyFacilitator struct{}

func (RejectVerifyFacilitator) Verify(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.VerifyResponse, error) {
	return &x402.VerifyResponse{IsValid: false, Payer: ""}, nil
}

func (RejectVerifyFacilitator) Settle(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.SettleResponse, error) {
	return &x402.SettleResponse{Success: false}, nil
}

func (RejectVerifyFacilitator) GetSupported(ctx context.Context) (x402.SupportedResponse, error) {
	return MockFacilitator{}.GetSupported(ctx)
}

// ErrorVerifyFacilitator returns an error from Verify (simulates facilitator outage).
type ErrorVerifyFacilitator struct{ Err error }

func (f ErrorVerifyFacilitator) Verify(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.VerifyResponse, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return nil, errors.New("verify failed")
}

func (f ErrorVerifyFacilitator) Settle(ctx context.Context, payloadBytes []byte, requirementsBytes []byte) (*x402.SettleResponse, error) {
	return nil, errors.New("settle failed")
}

func (f ErrorVerifyFacilitator) GetSupported(ctx context.Context) (x402.SupportedResponse, error) {
	return MockFacilitator{}.GetSupported(ctx)
}

// TestX402Config returns config aligned with [MockFacilitator] supported kinds.
func TestX402Config() config.X402 {
	return config.X402{
		FacilitatorURL: "http://unused.example",
		PayTo:          "0x1111111111111111111111111111111111111111",
		Network:        "eip155:84532",
		Price:          "$0.001",
	}
}

// AutoPayTransport retries once after answering HTTP 402 with a synthetic PAYMENT-SIGNATURE.
type AutoPayTransport struct {
	Base http.RoundTripper
}

func (t *AutoPayTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt := t.Base
	if rt == nil {
		rt = http.DefaultTransport
	}

	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	resp, err := rt.RoundTrip(r)
	if err != nil || resp == nil || resp.StatusCode != http.StatusPaymentRequired {
		return resp, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	hdr := resp.Header.Get("PAYMENT-REQUIRED")
	if hdr == "" {
		return resp, nil
	}
	raw, err := base64.StdEncoding.DecodeString(hdr)
	if err != nil {
		return nil, err
	}
	var pr types.PaymentRequired
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, err
	}
	if len(pr.Accepts) == 0 {
		return resp, nil
	}
	acc := pr.Accepts[0]
	if acc.Extra == nil {
		acc.Extra = map[string]any{}
	}
	acc.Extra["resourceUrl"] = r.URL.String()
	pp := types.PaymentPayload{
		X402Version: 2,
		Payload:     map[string]any{"sig": "test"},
		Accepted:    acc,
	}
	sigBytes, err := json.Marshal(pp)
	if err != nil {
		return nil, err
	}
	sig := base64.StdEncoding.EncodeToString(sigBytes)

	r2 := r.Clone(r.Context())
	r2.Header.Set("PAYMENT-SIGNATURE", sig)
	if ct := r.Header.Get("Content-Type"); ct != "" {
		r2.Header.Set("Content-Type", ct)
	}
	if ac := r.Header.Get("Accept"); ac != "" {
		r2.Header.Set("Accept", ac)
	} else {
		r2.Header.Set("Accept", "application/json")
	}
	if len(bodyBytes) > 0 {
		r2.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	return rt.RoundTrip(r2)
}
