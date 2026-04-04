package config

import "os"

// NaryoIngestSecret is the shared secret Naryo broadcasters send as X-Naryo-Webhook-Secret.
func NaryoIngestSecret() string {
	return os.Getenv("NARYO_INGEST_SECRET")
}
