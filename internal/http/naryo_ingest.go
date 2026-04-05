package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/pipeline"
)

// checkNaryoIngestAuth validates X-Naryo-Webhook-Secret against NARYO_INGEST_SECRET.
func (a *API) checkNaryoIngestAuth(w http.ResponseWriter, r *http.Request) bool {
	secret := config.NaryoIngestSecret()
	if secret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "naryo ingest not configured"})
		return false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Naryo-Webhook-Secret")), []byte(secret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

// PostNaryoEventSession handles POST /internal/naryo/v1/events/{sessionId}.
// Naryo’s HTTP broadcaster should use this path per pipeline so the platform does not rely on Naryo’s JSON shape for routing.
// Body: any JSON (typically Naryo’s event object). eventId is taken from common id fields or derived from a body hash.
func (a *API) PostNaryoEventSession(w http.ResponseWriter, r *http.Request) {
	if !a.checkNaryoIngestAuth(w, r) {
		return
	}
	sessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing sessionId"})
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	payload, eventID, perr := parseNaryoBroadcastBody(raw)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}
	dup, err := a.Svc.IngestNaryoEvent(r.Context(), sessionID, eventID, payload)
	if err != nil {
		if errors.Is(err, pipeline.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		if errors.Is(err, pipeline.ErrInvalidNaryoEvent) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "duplicate": dup, "eventId": eventID})
}

func parseNaryoBroadcastBody(raw []byte) (payload map[string]any, eventID string, err error) {
	raw = bytesTrimSpaceJSON(raw)
	if len(raw) == 0 {
		return nil, "", errors.New("empty body")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, "", errors.New("invalid json")
	}
	payload = normalizeJSONToPayloadMap(v)
	eventID = strings.TrimSpace(extractNaryoEventID(payload))
	if eventID == "" {
		sum := sha256.Sum256(raw)
		eventID = "naryo-" + hex.EncodeToString(sum[:12])
	}
	return payload, eventID, nil
}

func bytesTrimSpaceJSON(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func normalizeJSONToPayloadMap(v any) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		return x
	case []any:
		return map[string]any{"items": x}
	default:
		return map[string]any{"value": x}
	}
}

func extractNaryoEventID(m map[string]any) string {
	keys := []string{"eventId", "id", "transaction_id", "transactionId", "hash", "consensus_timestamp"}
	for _, k := range keys {
		if s := anyToStableString(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func anyToStableString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", x), "0"), ".")
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
