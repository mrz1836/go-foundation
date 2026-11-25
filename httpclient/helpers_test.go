package httpclient_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/httpclient"
)

// echo answers with a JSON object describing the request it received.
func echo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"method":        r.Method,
			"body":          string(body),
			"content_type":  r.Header.Get("Content-Type"),
			"accept":        r.Header.Get("Accept"),
			"authorization": r.Header.Get("Authorization"),
		})
	})
}

func TestGetJSONDecodesAResponse(t *testing.T) {
	srv := newTLSServer(t, echo())
	client := pinnedTo(t, srv)

	var got map[string]string
	header := http.Header{"authorization": {"Bearer " + fakeSecret}}
	require.NoError(t, httpclient.GetJSON(t.Context(), client, srv.URL, header, &got))

	assert.Equal(t, http.MethodGet, got["method"])
	assert.Equal(t, "application/json", got["accept"])
	assert.Equal(t, "Bearer "+fakeSecret, got["authorization"])
}

func TestPostJSONSendsAndDecodes(t *testing.T) {
	srv := newTLSServer(t, echo())
	client := pinnedTo(t, srv)

	var got map[string]string
	in := map[string]int{"n": 1}
	require.NoError(t, httpclient.PostJSON(t.Context(), client, srv.URL, nil, in, &got))

	assert.Equal(t, http.MethodPost, got["method"])
	assert.JSONEq(t, `{"n":1}`, got["body"])
	assert.Equal(t, "application/json", got["content_type"])
}

func TestPostJSONRefusesAnUnencodableBody(t *testing.T) {
	err := httpclient.PostJSON(t.Context(), http.DefaultClient, "https://api.example.com", nil, func() {}, nil)
	require.Error(t, err)
}

func TestJSONHelpersReturnAStatusErrorWithoutTheBody(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key `+fakeSecret+`"}`)
	}))
	client := pinnedTo(t, srv)

	err := httpclient.GetJSON(t.Context(), client, srv.URL, nil, &map[string]any{})
	require.ErrorIs(t, err, httpclient.ErrUnexpectedStatus)

	var statusErr *httpclient.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusUnauthorized, statusErr.StatusCode)
	assert.Equal(t, http.MethodGet, statusErr.Method)
	assert.Equal(t, hostOf(t, srv)+":"+portOf(t, srv), statusErr.Host)
	assert.NotContains(t, err.Error(), fakeSecret)
	assert.Contains(t, err.Error(), "401")
}

func TestDoJSONSkipsDecodingWhenThereIsNothingToDecode(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, "not json")
	}))
	client := pinnedTo(t, srv)

	var out map[string]any
	require.NoError(t, httpclient.GetJSON(t.Context(), client, srv.URL+"/empty", nil, &out))
	require.NoError(t, httpclient.GetJSON(t.Context(), client, srv.URL, nil, nil))
	assert.Nil(t, out)
}

func TestDoJSONRefusesANilClient(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.example.com", nil)
	require.NoError(t, err)

	require.ErrorIs(t, httpclient.DoJSON(nil, req, nil, 0), httpclient.ErrInvalidConfig)
}

func TestDecodeJSONErrorsNeverQuoteTheBody(t *testing.T) {
	type target struct {
		Count int `json:"count"`
	}

	tests := []struct {
		name string
		body string
		leak string
	}{
		// encoding/json quotes the offending byte, as 's'.
		{name: "syntax error", body: `{"count": ` + fakeSecret + `}`, leak: "'s'"},
		{name: "type error on a number", body: `{"count": "` + fakeSecret + `"}`, leak: fakeSecret},
		{name: "type error with a number value", body: `[987654321]`, leak: "987654321"},
		{name: "trailing data", body: `{"count": 1} ` + fakeSecret, leak: "'s'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(tt.body))}

			var out target
			err := httpclient.DecodeJSON(resp, &out, 0)
			require.ErrorIs(t, err, httpclient.ErrInvalidJSON)
			assert.NotContains(t, err.Error(), tt.leak)
		})
	}
}

func TestDecodeJSONReportsANonPointerTarget(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{}`))}

	var out map[string]any
	err := httpclient.DecodeJSON(resp, out, 0)
	require.ErrorIs(t, err, httpclient.ErrInvalidJSON)
}

func TestReadBodyLimits(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		limit   int64
		want    string
		wantErr error
	}{
		{name: "under the limit", body: "abc", limit: 4, want: "abc"},
		{name: "at the limit", body: "abcd", limit: 4, want: "abcd"},
		{name: "over the limit", body: "abcde", limit: 4, wantErr: httpclient.ErrResponseTooLarge},
		{name: "default limit", body: "abc", limit: 0, want: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := httpclient.ReadBody(&http.Response{Body: io.NopCloser(strings.NewReader(tt.body))}, tt.limit)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestReadBodyWithoutABody(t *testing.T) {
	got, err := httpclient.ReadBody(nil, 0)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestReadBodySurfacesTheClientBodyLimit(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, strings.Repeat("a", 64))
	}))
	client := pinnedTo(t, srv, httpclient.WithMaxBodySize(16))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	_, err = httpclient.ReadBody(resp, 1<<20)
	require.ErrorIs(t, err, httpclient.ErrResponseTooLarge)
}

// failingTarget fails to decode with an error that quotes its input.
type failingTarget struct{}

// UnmarshalJSON fails, quoting the input.
func (failingTarget) UnmarshalJSON(b []byte) error {
	return fmt.Errorf("%w: %s", errCause, b)
}

func TestDecodeJSONHidesACustomDecoderError(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`"` + fakeSecret + `"`))}

	err := httpclient.DecodeJSON(resp, &failingTarget{}, 0)
	require.ErrorIs(t, err, httpclient.ErrInvalidJSON)
	assert.NotContains(t, err.Error(), fakeSecret)
}

func TestDecodeJSONRefusesAnOversizedBody(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"a":"` + strings.Repeat("x", 64) + `"}`))}

	var out map[string]string
	require.ErrorIs(t, httpclient.DecodeJSON(resp, &out, 16), httpclient.ErrResponseTooLarge)
}

func TestPostJSONRefusesAnInvalidURL(t *testing.T) {
	err := httpclient.PostJSON(t.Context(), http.DefaultClient, "https://[bad"+fakeSecret, nil, map[string]int{}, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), fakeSecret)
}

// BenchmarkDecodeJSON measures reading and decoding a small JSON response
// body through the bounded reader.
func BenchmarkDecodeJSON(b *testing.B) {
	const body = `{"id":"msg_01","model":"example-1","usage":{"input_tokens":12,"output_tokens":3}}`

	type response struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	b.ReportAllocs()

	for range b.N {
		var out response
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
		if err := httpclient.DecodeJSON(resp, &out, 0); err != nil {
			b.Fatal(err)
		}
	}
}
