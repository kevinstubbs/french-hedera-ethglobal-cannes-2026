package config

import (
	"os"
	"strings"
)

// NaryoIngestSecret is the shared secret Naryo broadcasters send as X-Naryo-Webhook-Secret.
func NaryoIngestSecret() string {
	return os.Getenv("NARYO_INGEST_SECRET")
}

// NaryoConfigAPIBaseURL is the Naryo Configuration API origin (e.g. http://127.0.0.1:6060).
// Required at API startup together with [NaryoPlatformIngestURL].
func NaryoConfigAPIBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("NARYO_CONFIG_API_BASE")), "/")
}

// NaryoPlatformIngestURL is the HTTP base URL Naryo uses for broadcaster POSTs (scheme + host + port).
// Must match a URL reachable from the Naryo container (e.g. http://host.docker.internal:8080).
func NaryoPlatformIngestURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("NARYO_PLATFORM_INGEST_URL")), "/")
}

// NaryoBroadcasterDestPath is the base path for HTTP ALL-target broadcasters (no trailing slash).
// The API registers POST {path}/{sessionId}; the Naryo client appends /{sessionId} per pipeline so routing does not depend on Naryo’s JSON body.
func NaryoBroadcasterDestPath() string {
	if p := strings.TrimSpace(os.Getenv("NARYO_BROADCASTER_DEST_PATH")); p != "" {
		return p
	}
	return "/internal/naryo/v1/events"
}

// NaryoBroadcasterConfigurationID, when non-empty, skips POST create of broadcaster-configuration and
// reuses this id (hash from GET). Use when Naryo async-creates fail (e.g. NPE) but a row already exists.
func NaryoBroadcasterConfigurationID() string {
	return strings.TrimSpace(os.Getenv("NARYO_BROADCASTER_CONFIGURATION_ID"))
}

// NaryoSkipEgressProvision when true: real Naryo client skips Configuration API broadcaster (and config) provisioning on start/resume.
// Naryo will not HTTP-push to the platform; recentNaryoEvents on status stay empty unless something else POSTs ingest.
func NaryoSkipEgressProvision() bool {
	v := strings.TrimSpace(os.Getenv("NARYO_SKIP_EGRESS_PROVISION"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// NaryoHederaNodeID is the Configuration API node UUID for Hedera (must match Naryo application.yml).
func NaryoHederaNodeID() string {
	if s := strings.TrimSpace(os.Getenv("NARYO_HEDERA_NODE_ID")); s != "" {
		return s
	}
	return "7f3b8e1a-4d2c-4b9a-8e5f-1a2b3c4d5e6f"
}

// NaryoEthereumNodeID is the Configuration API node UUID for Ethereum/EVM (must match Naryo application.yml).
func NaryoEthereumNodeID() string {
	if s := strings.TrimSpace(os.Getenv("NARYO_ETHEREUM_NODE_ID")); s != "" {
		return s
	}
	return "eadc75b2-4217-4018-95af-f67c13058976"
}

// NaryoDefaultHCSTopicID, when set, is used as a Hedera TRANSACTION filter (IDENTITY_ID) when eventSubscriptions
// has no hedera_hcs_topic entry (same shape as declarative filters in Naryo YAML).
func NaryoDefaultHCSTopicID() string {
	return strings.TrimSpace(os.Getenv("NARYO_DEFAULT_HCS_TOPIC_ID"))
}

// NaryoAllowAllBroadcasterFallback when true (default): (1) if eventSubscriptions (and NARYO_DEFAULT_HCS_TOPIC_ID)
// cannot yield a scoped filter plan, provision a broadcaster with target ALL (legacy); (2) if a scoped filter or
// FILTER broadcaster async operation fails with a Naryo revision hook error (e.g. onAfterApply RuntimeException),
// EnsurePipeline falls back to an ALL-target broadcaster instead of failing start. When false, those paths error.
func NaryoAllowAllBroadcasterFallback() bool {
	v := strings.TrimSpace(os.Getenv("NARYO_ALLOW_ALL_BROADCASTER_FALLBACK"))
	if v == "" {
		return true
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
