package httpclient_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/httpclient"
)

// errCause is the cause the redaction tests wrap.
var errCause = errors.New("connection refused")

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{ //nolint:gosec // G101: a fake credential to redact
			name: "userinfo, query, and fragment",
			raw:  "https://user:pw@api.example.com/v1/x?key=abc#frag",
			want: "https://api.example.com/v1/x",
		},
		{name: "port kept", raw: "https://api.example.com:8443/v1?token=abc", want: "https://api.example.com:8443/v1"},
		{name: "escaped path kept", raw: "https://api.example.com/a%2Fb?x=1", want: "https://api.example.com/a%2Fb"},
		{name: "plain", raw: "https://api.example.com", want: "https://api.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, httpclient.RedactURL(u))
		})
	}

	assert.Empty(t, httpclient.RedactURL(nil))
}

func TestRedactError(t *testing.T) {
	leaky := &url.Error{Op: "Get", URL: "https://user:" + fakeSecret + "@api.example.com/v1?key=" + fakeSecret, Err: errCause}

	tests := []struct {
		name string
		err  error
	}{
		{name: "url error", err: leaky},
		{name: "wrapped url error", err: fmt.Errorf("calling the API: %w", leaky)},
		{name: "reformatted wrapper", err: &reformatted{inner: leaky}},
		{name: "unparsable URL", err: &url.Error{Op: "parse", URL: "https://[" + fakeSecret, Err: errCause}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httpclient.RedactError(tt.err)
			assert.NotContains(t, got.Error(), fakeSecret)
			require.ErrorIs(t, got, errCause)

			var uerr *url.Error
			require.ErrorAs(t, got, &uerr)
			assert.NotContains(t, uerr.URL, fakeSecret, "errors.As must not reach the original URL")
		})
	}

	assert.Equal(t, "https://api.example.com/v1", mustURLError(t, httpclient.RedactError(leaky)).URL)
	assert.Equal(t, errCause, httpclient.RedactError(errCause), "an error without a URL is unchanged")
	assert.NoError(t, httpclient.RedactError(nil))
}

// reformatted is a wrapper whose message does not contain its inner error's.
type reformatted struct {
	inner error
}

// Error reformats the inner message.
func (r *reformatted) Error() string {
	return strings.ToUpper(r.inner.Error())
}

// Unwrap returns the inner error.
func (r *reformatted) Unwrap() error {
	return r.inner
}

// mustURLError returns the *url.Error in err.
func mustURLError(t *testing.T, err error) *url.Error {
	t.Helper()
	var uerr *url.Error
	require.ErrorAs(t, err, &uerr)

	return uerr
}

func TestHelperErrorsDropUserinfoAndQuery(t *testing.T) {
	srv := newTLSServer(t, ok())
	client := pinnedTo(t, srv)

	leakyURL := "https://user:" + fakeSecret + "@localhost:" + portOf(t, srv) + "/v1?key=" + fakeSecret
	err := httpclient.GetJSON(t.Context(), client, leakyURL, nil, nil)
	require.ErrorIs(t, err, httpclient.ErrHostNotAllowed)
	assert.NotContains(t, err.Error(), fakeSecret)

	err = httpclient.GetJSON(t.Context(), client, "https://[bad"+fakeSecret, nil, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), fakeSecret)
}

func TestObserverNeverSeesSecrets(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "session="+fakeSecret)
		_, _ = io.WriteString(w, `{"echo":"`+fakeSecret+`"}`)
	}))

	var mu sync.Mutex
	var events []httpclient.Event
	client := pinnedTo(t, srv, httpclient.WithObserver(func(e httpclient.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}))

	rawURL := "https://user:" + fakeSecret + "@" + hostOf(t, srv) + ":" + portOf(t, srv) + "/v1/" + "?key=" + fakeSecret
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, rawURL, strings.NewReader(fakeSecret))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+fakeSecret)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// A refused request is reported too.
	_, _ = fetch(t, client, "https://localhost:"+portOf(t, srv)+"/?key="+fakeSecret)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, events, 2)
	assert.Equal(t, http.MethodPost, events[0].Method)
	assert.Equal(t, hostOf(t, srv)+":"+portOf(t, srv), events[0].Host)
	assert.Equal(t, http.StatusOK, events[0].Status)
	assert.Equal(t, 1, events[0].Attempt)
	assert.Positive(t, events[0].Duration)
	require.NoError(t, events[0].Err)
	require.ErrorIs(t, events[1].Err, httpclient.ErrHostNotAllowed)

	for _, e := range events {
		assert.NotContains(t, fmt.Sprintf("%+v %v", e, e.Err), fakeSecret)
	}
}

func TestStatusErrorMessage(t *testing.T) {
	err := &httpclient.StatusError{StatusCode: http.StatusForbidden, Method: http.MethodGet, Host: "api.example.com"}

	assert.Equal(t, "httpclient: GET api.example.com: unexpected status 403", err.Error())
	require.ErrorIs(t, err, httpclient.ErrUnexpectedStatus)
}
