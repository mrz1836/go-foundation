package testutil_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/testutil"
)

// errFakeNetwork stands in for any caller-error the stubbed doer surfaces.
// Keeping it as a package-level sentinel avoids err113's "dynamic error"
// warnings and lets ErrorIs match exactly.
var (
	errFakeNetwork     = errors.New("fake: connection refused")
	errFakeBodyTrunc   = errors.New("fake: unexpected EOF")
	errTruncatedStream = errors.New("fake: truncated stream")
)

func newReq(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	return req
}

// closeBody is a no-fail close helper for the stub responses. The FakeHTTPDoer
// returns bodies backed by strings.Reader or errReader, both of which Close()
// to nil.
func closeBody(t *testing.T, body io.Closer) {
	t.Helper()
	require.NoError(t, body.Close())
}

//nolint:gocognit // table-driven test exercises the success/error/read-error branches
func TestFakeHTTPDoer(t *testing.T) {
	t.Parallel()
	const url = "https://api/x"
	tests := []struct {
		name        string
		stub        func(f *testutil.FakeHTTPDoer)
		wantDoErr   error  // non-nil → Do returns this error and a nil response
		wantStatus  int    // expected status on the success path
		wantBody    string // expected body on the success path
		wantJSONEq  bool   // compare wantBody as JSON rather than bytes
		wantReadErr error  // non-nil → Do succeeds but Body.Read fails with this
	}{
		{
			name:       "StubURL returns the programmed status and body",
			stub:       func(f *testutil.FakeHTTPDoer) { f.StubURL(url, http.StatusCreated, `{"ok":true}`) },
			wantStatus: http.StatusCreated,
			wantBody:   `{"ok":true}`,
			wantJSONEq: true,
		},
		{
			name:      "StubError propagates the transport error and returns a nil response",
			stub:      func(f *testutil.FakeHTTPDoer) { f.StubError(url, errFakeNetwork) },
			wantDoErr: errFakeNetwork,
		},
		{
			name:       "StubDefault applies to an unstubbed URL",
			stub:       func(f *testutil.FakeHTTPDoer) { f.StubDefault(http.StatusTeapot, "I am a teapot") },
			wantStatus: http.StatusTeapot,
			wantBody:   "I am a teapot",
		},
		{
			name:        "StubBodyReadError succeeds on Do but fails mid-stream on Read",
			stub:        func(f *testutil.FakeHTTPDoer) { f.StubBodyReadError(url, http.StatusOK, errFakeBodyTrunc) },
			wantStatus:  http.StatusOK,
			wantReadErr: errFakeBodyTrunc,
		},
		{
			name:       "an unstubbed URL with no default falls back to 200 OK and an empty body",
			stub:       func(*testutil.FakeHTTPDoer) {},
			wantStatus: http.StatusOK,
			wantBody:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := testutil.NewFakeHTTPDoer()
			tt.stub(f)

			resp, err := f.Do(newReq(t, url))
			if tt.wantDoErr != nil {
				require.Nil(t, resp)
				require.ErrorIs(t, err, tt.wantDoErr)
				return
			}
			require.NoError(t, err)
			defer closeBody(t, resp.Body)
			require.Equal(t, tt.wantStatus, resp.StatusCode)

			body, readErr := io.ReadAll(resp.Body)
			if tt.wantReadErr != nil {
				require.ErrorIs(t, readErr, tt.wantReadErr, "the response returns successfully — the failure is on Body.Read")
				return
			}
			require.NoError(t, readErr)
			if tt.wantJSONEq {
				assert.JSONEq(t, tt.wantBody, string(body))
			} else {
				assert.Equal(t, tt.wantBody, string(body))
			}
			assert.Equal(t, 1, f.Calls(), "each Do increments the call counter")
		})
	}
}

func TestFakeHTTPDoerCallsCounterIncrementsPerRequest(t *testing.T) {
	t.Parallel()
	f := testutil.NewFakeHTTPDoer().StubDefault(http.StatusOK, "")

	for range 3 {
		resp, err := f.Do(newReq(t, "https://api/x"))
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	assert.Equal(t, 3, f.Calls())
}

// TestFakeHTTPDoerStubDefaultBodyReadErrorFailsUnstubbedReads proves the fallback
// body-read error applies to any unstubbed request: Do succeeds, but consuming
// the body fails — the truncated-stream case reached through the default rather
// than a per-URL stub.
func TestFakeHTTPDoerStubDefaultBodyReadErrorFailsUnstubbedReads(t *testing.T) {
	t.Parallel()
	readErr := errTruncatedStream
	doer := testutil.NewFakeHTTPDoer().StubDefaultBodyReadError(http.StatusOK, readErr)

	resp, err := doer.Do(newReq(t, "https://example.test/x"))
	require.NoError(t, err)
	defer closeBody(t, resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, readBackErr := io.ReadAll(resp.Body)
	require.ErrorIs(t, readBackErr, readErr, "the default body-read error applies to any unstubbed request")
}
