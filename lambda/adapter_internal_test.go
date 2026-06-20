package lambda

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/mrz1836/go-foundation/constants"
)

// errForcedMarshal is a package-scope sentinel (keeps err113 happy) used to
// force the marshal-failure fallback branch in ServeHTTP.
var errForcedMarshal = errors.New("forced marshal failure")

// TestServeHTTP_MarshalFallback exercises the otherwise-unreachable marshal
// failure branch in ServeHTTP. json.Marshal of the static ErrorResponse cannot
// fail in practice, so marshalJSON is swapped for a stub that returns an error,
// confirming ServeHTTP falls back to the hand-built static JSON body.
func TestServeHTTP_MarshalFallback(t *testing.T) {
	orig := marshalJSON

	marshalJSON = func(any) ([]byte, error) {
		return nil, errForcedMarshal
	}

	defer func() { marshalJSON = orig }()

	// IsBase64Encoded with an invalid base64 body forces toHTTPRequest to error,
	// driving ServeHTTP into the error branch where marshalJSON is invoked.
	event := events.APIGatewayV2HTTPRequest{
		RawPath:         "/upload",
		IsBase64Encoded: true,
		Body:            "!!!not-valid-base64!!!",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "POST"},
		},
	}

	resp, err := ServeHTTP(context.Background(), event, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err == nil {
		t.Fatal("expected error from ServeHTTP on invalid base64 body, got nil")
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", resp.StatusCode)
	}

	want := `{"error":"` + constants.ErrorMessageInternalError + `","code":"` + constants.ErrorCodeInternalError + `"}`
	if resp.Body != want {
		t.Errorf("Body = %q, want static fallback %q", resp.Body, want)
	}

	if resp.Headers[constants.HeaderContentType] != constants.ContentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", resp.Headers[constants.HeaderContentType], constants.ContentTypeJSON)
	}
}
