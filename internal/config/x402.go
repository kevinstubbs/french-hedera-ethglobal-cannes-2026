package config

import (
	"os"
	"strings"

	x402http "github.com/coinbase/x402/go/http"
)

// X402 holds Coinbase / x402.org facilitator and payee settings.
type X402 struct {
	FacilitatorURL string
	PayTo          string
	Network        string // CAIP-2, e.g. eip155:84532
	Price          string // e.g. "$0.001"
}

// LoadX402FromEnv reads x402 settings. Empty FacilitatorURL defaults to Coinbase public facilitator.
func LoadX402FromEnv() X402 {
	fac := strings.TrimSpace(os.Getenv("X402_FACILITATOR_URL"))
	if fac == "" {
		fac = x402http.DefaultFacilitatorURL
	}
	payTo := strings.TrimSpace(os.Getenv("X402_PAY_TO"))
	if payTo == "" {
		payTo = "0x0000000000000000000000000000000000000000"
	}
	net := strings.TrimSpace(os.Getenv("X402_NETWORK"))
	if net == "" {
		net = "eip155:84532"
	}
	price := strings.TrimSpace(os.Getenv("X402_PRICE"))
	if price == "" {
		price = "$0.001"
	}
	return X402{
		FacilitatorURL: fac,
		PayTo:          payTo,
		Network:        net,
		Price:          price,
	}
}
