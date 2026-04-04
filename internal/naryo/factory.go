package naryo

import (
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// NewFromEnv returns a real [HTTPClient] when NARYO_CONFIG_API_BASE and NARYO_PLATFORM_INGEST_URL
// are set; otherwise a [MockClient] for tests and offline use.
func NewFromEnv() Client {
	base := config.NaryoConfigAPIBaseURL()
	ingest := config.NaryoPlatformIngestURL()
	if base == "" || ingest == "" {
		return &MockClient{}
	}
	c, err := NewHTTPClient(HTTPClientConfig{
		BaseURL:           base,
		PlatformIngestURL: ingest,
		ActiveDestPath:    config.NaryoBroadcasterDestPath(),
	})
	if err != nil {
		return &MockClient{}
	}
	return c
}
