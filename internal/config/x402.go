package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	x402http "github.com/coinbase/x402/go/http"
)

// X402 holds Coinbase / x402.org facilitator and payee settings.
type X402 struct {
	FacilitatorURL string
	PayTo          string
	Network        string // CAIP-2, e.g. eip155:84532
	// Price is USDC quoted as a dollar string per 1 second of pipeline time (e.g. "$0.000034").
	// POST .../start charges Price × StartRunwaySeconds on-chain. GET .../status uses prepaid only (> 0 units).
	Price string
	// StartRunwaySeconds multiplies Price for the on-chain x402 amount on POST .../start (default 300).
	StartRunwaySeconds int64
}

// MulUSDPrice returns perUnit × units as a dollar string (e.g. "$0.01" × 300).
func MulUSDPrice(perUnit string, units int64) (string, error) {
	s := strings.TrimSpace(perUnit)
	if units <= 0 {
		units = 300
	}
	if !strings.HasPrefix(s, "$") {
		return "", fmt.Errorf("X402_PRICE must look like a dollar amount (got %q); quote it in shell: X402_PRICE='$0.0001'", s)
	}
	n, err := strconv.ParseFloat(strings.TrimPrefix(s, "$"), 64)
	if err != nil {
		return "", fmt.Errorf("parse X402_PRICE: %w", err)
	}
	return fmt.Sprintf("$%.10f", n*float64(units)), nil
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
		price = "$0.00017"
	}
	runway := int64(300)
	if v := strings.TrimSpace(os.Getenv("X402_START_RUNWAY_SECONDS")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			runway = n
		}
	}
	return X402{
		FacilitatorURL:     fac,
		PayTo:              payTo,
		Network:            net,
		Price:              price,
		StartRunwaySeconds: runway,
	}
}
