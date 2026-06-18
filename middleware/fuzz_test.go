package middleware

import (
	"testing"
)

func FuzzExtractErrorMessage(f *testing.F) {
	f.Add(`{"error":"bad request"}`)
	f.Add(`{"message":"not found"}`)
	f.Add(`{"error_message":"validation failed"}`)
	f.Add(`{"error":42}`)
	f.Add(`not json at all`)
	f.Add("")
	f.Add(`{"nested":{"error":"deep"}}`)

	f.Fuzz(func(_ *testing.T, input string) {
		// Must not panic on any input
		_ = extractErrorMessage(input)
	})
}
