package ctxutil_test

import (
	"encoding/json"
	"testing"

	"github.com/mrz1836/go-foundation/ctxutil"
)

func TestRequestIDToMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		base          []byte
		requestID     string
		wantRequestID string            // expected value read back via RequestIDFromMetadata
		wantExtra     map[string]string // additional keys that must survive from base
	}{
		{name: "empty base, id set", base: nil, requestID: "req-1", wantRequestID: "req-1"},
		{name: "empty slice base, id set", base: []byte{}, requestID: "req-2", wantRequestID: "req-2"},
		{
			name:          "base with extra tag merges alongside id",
			base:          []byte(`{"tenant":"acme"}`),
			requestID:     "req-3",
			wantRequestID: "req-3",
			wantExtra:     map[string]string{"tenant": "acme"},
		},
		{
			name:          "base preserved when id empty",
			base:          []byte(`{"tenant":"acme"}`),
			requestID:     "",
			wantRequestID: "",
			wantExtra:     map[string]string{"tenant": "acme"},
		},
		{name: "empty base and empty id yields empty object", base: nil, requestID: "", wantRequestID: ""},
		{name: "malformed base ignored, id stamped", base: []byte("not json"), requestID: "req-4", wantRequestID: "req-4"},
		{
			name:          "base request_id overwritten by argument",
			base:          []byte(`{"request_id":"old"}`),
			requestID:     "new",
			wantRequestID: "new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := ctxutil.RequestIDToMetadata(tt.base, tt.requestID)

			// The result is never empty and is always valid JSON.
			if len(out) == 0 {
				t.Fatal("RequestIDToMetadata returned an empty blob; want non-empty")
			}
			decoded := decodeMetadata(t, out)

			// The id round-trips through the extractor.
			if got := ctxutil.RequestIDFromMetadata(out); got != tt.wantRequestID {
				t.Fatalf("RequestIDFromMetadata(RequestIDToMetadata(...)) = %q, want %q", got, tt.wantRequestID)
			}

			// An empty id must not stamp the key at all.
			if tt.requestID == "" {
				assertKeyAbsent(t, decoded, "request_id", out)
			}

			// Extra base keys survive the merge.
			assertExtraKeys(t, decoded, tt.wantExtra)
		})
	}
}

// decodeMetadata unmarshals a metadata blob, failing the test on invalid JSON.
func decodeMetadata(t *testing.T, out []byte) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("RequestIDToMetadata produced invalid JSON %q: %v", out, err)
	}

	return decoded
}

// assertKeyAbsent fails the test if key is present in decoded.
func assertKeyAbsent(t *testing.T, decoded map[string]any, key string, out []byte) {
	t.Helper()

	if _, present := decoded[key]; present {
		t.Fatalf("%s key present for empty id: %q", key, out)
	}
}

// assertExtraKeys verifies that each want key survives the merge with its value.
func assertExtraKeys(t *testing.T, decoded map[string]any, want map[string]string) {
	t.Helper()

	for k, wantVal := range want {
		if got, _ := decoded[k].(string); got != wantVal {
			t.Fatalf("merged key %q = %v, want %q", k, decoded[k], wantVal)
		}
	}
}

func TestRequestIDFromMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "present", raw: []byte(`{"request_id":"req-9"}`), want: "req-9"},
		{name: "present alongside other keys", raw: []byte(`{"tenant":"acme","request_id":"req-9"}`), want: "req-9"},
		{name: "nil raw", raw: nil, want: ""},
		{name: "empty slice raw", raw: []byte{}, want: ""},
		{name: "malformed raw", raw: []byte("not json"), want: ""},
		{name: "valid object without request_id", raw: []byte(`{"tenant":"acme"}`), want: ""},
		{name: "empty object", raw: []byte(`{}`), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ctxutil.RequestIDFromMetadata(tt.raw); got != tt.want {
				t.Fatalf("RequestIDFromMetadata(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
