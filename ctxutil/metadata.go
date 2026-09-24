package ctxutil

import (
	"encoding/json"

	"github.com/mrz1836/go-foundation/constants"
)

// RequestIDToMetadata returns a JSON object blob (always non-empty) carrying the
// request id under the "request_id" key. base, when non-empty, is merged in
// first so a caller-supplied metadata blob can carry additional tags alongside
// the request id; an empty or malformed base is ignored. When requestID is
// empty the id is simply not stamped. The result is never empty: a marshal
// failure (or an empty result) falls back to "{}", so callers can persist the
// return value into a NOT NULL JSON column without branching.
//
// It is the write side of the request-id-over-metadata seam: a producer copies
// the id from context into a row's metadata at enqueue time, and a consumer
// reads it back with RequestIDFromMetadata across the storage boundary.
func RequestIDToMetadata(base []byte, requestID string) []byte {
	// Fast path: with no base blob to merge, avoid the map round-trip entirely.
	if len(base) == 0 {
		if requestID == "" {
			return []byte("{}")
		}
		// json.Marshal on the id gives the exact same escaping as the map path.
		v, err := json.Marshal(requestID)
		if err != nil {
			return []byte("{}")
		}
		out := make([]byte, 0, len(v)+16)
		out = append(out, `{"`...)
		out = append(out, constants.FieldRequestID...)
		out = append(out, `":`...)
		out = append(out, v...)
		out = append(out, '}')
		return out
	}

	m := map[string]any{}
	_ = json.Unmarshal(base, &m)
	if requestID != "" {
		m[constants.FieldRequestID] = requestID
	}
	out, err := json.Marshal(m)
	if err != nil || len(out) == 0 {
		return []byte("{}")
	}
	return out
}

// RequestIDFromMetadata extracts the request id from a JSON metadata blob, or
// returns "" when none is present. It tolerates an empty or malformed blob and
// returns "" rather than erroring, so a partial upgrade across an old stored row
// does not crash the consumer.
func RequestIDFromMetadata(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	// The struct tag must stay a literal (Go tags cannot reference a constant);
	// it must match constants.FieldRequestID.
	var m struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return m.RequestID
}
