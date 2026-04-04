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
// Empty means the API uses the mock Naryo adapter (tests and local runs without Naryo).
func NaryoConfigAPIBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("NARYO_CONFIG_API_BASE")), "/")
}

// NaryoPlatformIngestURL is the HTTP base URL Naryo uses for broadcaster POSTs (scheme + host + port).
// Must match a URL reachable from the Naryo container (e.g. http://host.docker.internal:8080).
func NaryoPlatformIngestURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("NARYO_PLATFORM_INGEST_URL")), "/")
}

// NaryoBroadcasterDestPath is the path appended to the platform ingest URL for ALL-target broadcasters.
func NaryoBroadcasterDestPath() string {
	if p := strings.TrimSpace(os.Getenv("NARYO_BROADCASTER_DEST_PATH")); p != "" {
		return p
	}
	return "/internal/naryo/v1/events"
}
