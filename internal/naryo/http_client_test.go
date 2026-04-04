package naryo

import "testing"

func TestExtractOperationID(t *testing.T) {
	t.Parallel()
	id, err := extractOperationID(map[string]any{"value": "550e8400-e29b-41d4-a716-446655440000"})
	if err != nil || id != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("got %q %v", id, err)
	}
	id2, err := extractOperationID(map[string]any{
		"operationId": map[string]any{"value": "aa0e8400-e29b-41d4-a716-446655440001"},
	})
	if err != nil || id2 != "aa0e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("nested got %q %v", id2, err)
	}
	if _, err := extractOperationID(map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewHTTPClientRequiresURLs(t *testing.T) {
	t.Parallel()
	if _, err := NewHTTPClient(HTTPClientConfig{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := NewHTTPClient(HTTPClientConfig{BaseURL: "http://x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPClientStatsNil(t *testing.T) {
	t.Parallel()
	var c *HTTPClient
	st := c.Stats()
	if st["mode"] != "nil" {
		t.Fatalf("%v", st)
	}
}
