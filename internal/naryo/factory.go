package naryo

import (
	"fmt"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// NewFromEnv builds an [HTTPClient] from environment. Both NARYO_CONFIG_API_BASE and
// NARYO_PLATFORM_INGEST_URL are required.
func NewFromEnv() (Client, error) {
	base := config.NaryoConfigAPIBaseURL()
	ingest := config.NaryoPlatformIngestURL()
	if base == "" || ingest == "" {
		return nil, fmt.Errorf("naryo: NARYO_CONFIG_API_BASE and NARYO_PLATFORM_INGEST_URL must both be set")
	}
	c, err := NewHTTPClient(HTTPClientConfig{
		BaseURL:                  base,
		PlatformIngestURL:        ingest,
		ActiveDestPath:           config.NaryoBroadcasterDestPath(),
		HederaNodeID:             config.NaryoHederaNodeID(),
		EthereumNodeID:           config.NaryoEthereumNodeID(),
		FixedBroadcasterConfigID: config.NaryoBroadcasterConfigurationID(),
		SkipEgressProvision:      config.NaryoSkipEgressProvision(),
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
