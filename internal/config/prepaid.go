package config

import (
	"os"
	"strconv"
	"strings"
)

// PrepaidDevAutoCreditUnits returns optional units to credit on pipeline create (local demo only).
// Set PREPAID_DEV_AUTO_CREDIT_UNITS to a positive integer.
func PrepaidDevAutoCreditUnits() int64 {
	v := strings.TrimSpace(os.Getenv("PREPAID_DEV_AUTO_CREDIT_UNITS"))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// PrepaidRateUnitsPerMinute returns PREPAID_RATE_UNITS_PER_MINUTE or 0 if unset/invalid.
func PrepaidRateUnitsPerMinute() int64 {
	v := strings.TrimSpace(os.Getenv("PREPAID_RATE_UNITS_PER_MINUTE"))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
