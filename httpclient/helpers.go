package httpclient

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultBodyLimit is the most ReadBody, DecodeJSON, and DoJSON read when
// given a limit of zero or less, and the limit GetJSON and PostJSON use.
const DefaultBodyLimit = 10 << 20

// contentTypeJSON is the media type the JSON helpers send and accept.
const contentTypeJSON = "application/json"

// GetJSON sends a GET to rawURL with header, which may be nil, and decodes a
// 2xx JSON response into out, which may be nil to discard it. Any other status
// returns a *StatusError. The body is limited to DefaultBodyLimit.
func GetJSON(ctx context.Context, client *http.Client, rawURL string, header http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("httpclient: build GET request: %w", RedactError(err))
	}
	setHeaders(req, header, false)

	return DoJSON(client, req, out, DefaultBodyLimit)
}

// PostJSON sends in, encoded as JSON, in a POST to rawURL with header, which
// may be nil, and decodes a 2xx JSON response into out, which may be nil to
// discard it. Any other status returns a *StatusError. The request body can be
// replayed, so a retry policy that allows POST can resend it. The response body
// is limited to DefaultBodyLimit.
func PostJSON(ctx context.Context, client *http.Client, rawURL string, header http.Header, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("httpclient: encode request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("httpclient: build POST request: %w", RedactError(err))
	}
	setHeaders(req, header, true)

	return DoJSON(client, req, out, DefaultBodyLimit)
}

// DoJSON sends req through client and decodes a 2xx JSON response into out,
// reading at most limit bytes (DefaultBodyLimit when limit is zero or less).
// out may be nil to discard the body, and a 204 is not decoded. Any other
// status returns a *StatusError, and the body is discarded unread beyond a
// small bound. Errors never include the URL's userinfo or query string.
func DoJSON(client *http.Client, req *http.Request, out any, limit int64) error {
	if client == nil {
		return fmt.Errorf("%w: nil *http.Client", ErrInvalidConfig)
	}
	// G704: client is the caller's, and its transport enforces the caller's
	// scheme, host, and address rules on this request.
	resp, err := client.Do(req) //nolint:gosec // see the G704 note above
	if err != nil {
		return RedactError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		discard(resp)

		return &StatusError{StatusCode: resp.StatusCode, Method: cmp.Or(req.Method, http.MethodGet), Host: req.URL.Host}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		discard(resp)

		return nil
	}

	return DecodeJSON(resp, out, limit)
}

// DecodeJSON reads resp's body, at most limit bytes (DefaultBodyLimit when
// limit is zero or less), closes it, and decodes it into v. It does not check
// the status. A decode error wraps ErrInvalidJSON and never quotes the body.
func DecodeJSON(resp *http.Response, v any, limit int64) error {
	body, err := ReadBody(resp, limit)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, v); err != nil {
		return safeDecodeError(err)
	}

	return nil
}

// ReadBody reads resp's body, at most limit bytes (DefaultBodyLimit when limit
// is zero or less), and closes it. A body longer than limit fails with
// ErrResponseTooLarge. It does not check the status.
func ReadBody(resp *http.Response, limit int64) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	if limit <= 0 {
		limit = DefaultBodyLimit
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("httpclient: read response body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: over the %d-byte limit", ErrResponseTooLarge, limit)
	}

	return body, nil
}

// setHeaders copies header onto req, after the JSON defaults, so a caller's
// header wins.
func setHeaders(req *http.Request, header http.Header, hasBody bool) {
	req.Header.Set("Accept", contentTypeJSON)
	if hasBody {
		req.Header.Set("Content-Type", contentTypeJSON)
	}
	for key, values := range header {
		req.Header[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}
}

// safeDecodeError describes a JSON decode error without quoting the body.
// encoding/json's errors can include a character or a number from it.
func safeDecodeError(err error) error {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var invalid *json.InvalidUnmarshalError

	switch {
	case errors.As(err, &syntax):
		return fmt.Errorf("%w: syntax error at byte %d", ErrInvalidJSON, syntax.Offset)
	case errors.As(err, &typeErr):
		// Value is "number 42" for a number; keep only its kind.
		kind, _, _ := strings.Cut(typeErr.Value, " ")
		return fmt.Errorf("%w: cannot decode a JSON %s into %s at %q",
			ErrInvalidJSON, kind, typeErr.Type, typeErr.Field)
	case errors.As(err, &invalid):
		return fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	default:
		return fmt.Errorf("%w: %T", ErrInvalidJSON, err)
	}
}
