// Package lambda provides a lightweight AWS API Gateway v2 ↔ net/http adapter.
//
// It is a minimal, dependency-free bridge over the AWS Lambda Go events.
// ServeHTTP converts an APIGatewayV2HTTPRequest into a standard *http.Request,
// dispatches it to any http.Handler (e.g., a chi router), and converts the
// captured response back to an APIGatewayV2HTTPResponse.
//
// Responses are written as net/http writes them. The first final status wins:
// later WriteHeader calls are ignored, a Write before any WriteHeader means
// 200, and a handler that writes nothing answers 200. An informational status
// other than 101 Switching Protocols is dropped, because API Gateway cannot
// carry an interim response. A 1xx, 204, or 304 response, or a response to a
// HEAD request, has no body: a Write under a 1xx, 204, or 304 status returns
// http.ErrBodyNotAllowed, and a Write for a HEAD request is discarded. Those
// 1xx, 204, and 304 responses drop Content-Length and Transfer-Encoding, and a
// 304 drops Content-Type too; a HEAD response keeps its headers, Content-Length
// included, but drops Transfer-Encoding.
//
// Usage:
//
//	lambda.Start(func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
//	    return lambdahttp.ServeHTTP(ctx, req, router)
//	})
package lambda

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"

	"github.com/mrz1836/go-foundation/constants"
	"github.com/mrz1836/go-foundation/httputil"
)

//nolint:gochecknoglobals // bufferPool is safe to be global to preserve memory per AWS Lambda constraint
var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// marshalJSON is the JSON marshaller used by ServeHTTP for the error response.
// It is a package-level variable so tests can inject a failure and exercise the
// otherwise-unreachable static-fallback branch.
//
//nolint:gochecknoglobals // injectable seam for testing the marshal-failure fallback
var marshalJSON = json.Marshal

// lambdaResponseWriter implements http.ResponseWriter and captures the response
// so it can be converted to an APIGatewayV2HTTPResponse. It records the status
// and body the way net/http's server sends them (see the package documentation).
type lambdaResponseWriter struct {
	statusCode  int
	wroteHeader bool
	head        bool // a HEAD request: writes are discarded
	headers     http.Header
	body        *bytes.Buffer
}

// newLambdaResponseWriter returns a writer with a pooled body buffer. head
// reports whether the request is a HEAD request, whose body is discarded.
func newLambdaResponseWriter(head bool) *lambdaResponseWriter {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	return &lambdaResponseWriter{
		statusCode: http.StatusOK,
		head:       head,
		headers:    make(http.Header),
		body:       buf,
	}
}

func (w *lambdaResponseWriter) Header() http.Header { return w.headers }

// WriteHeader records the response status. Only the first final status counts;
// later calls are ignored. An informational status other than 101 Switching
// Protocols is ignored and leaves the status open: net/http sends it as an
// interim response, which API Gateway cannot carry.
func (w *lambdaResponseWriter) WriteHeader(code int) {
	if w.wroteHeader || isInterimStatus(code) {
		return
	}

	w.statusCode = code
	w.wroteHeader = true
}

// Write buffers p as the response body, sending a 200 first if no status has
// been written. Under a status that allows no body (1xx, 204, 304) it keeps
// nothing and returns http.ErrBodyNotAllowed. For a HEAD request it keeps
// nothing and reports p as written.
func (w *lambdaResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	if len(p) == 0 {
		return 0, nil
	}

	if !bodyAllowedForStatus(w.statusCode) {
		return 0, http.ErrBodyNotAllowed
	}

	if w.head {
		return len(p), nil
	}

	return w.body.Write(p)
}

// toAPIGatewayResponse converts the captured response to an APIGatewayV2HTTPResponse.
// Binary bodies (non-valid-UTF-8) are base64-encoded and IsBase64Encoded is set.
// Set-Cookie headers are routed to the dedicated Cookies field because API Gateway v2
// does not support comma-joined Set-Cookie values in the single-value Headers map.
// A status that allows no body drops the headers that describe one, as net/http does.
func (w *lambdaResponseWriter) toAPIGatewayResponse() events.APIGatewayV2HTTPResponse {
	defer bufferPool.Put(w.body)

	headers := make(map[string]string, len(w.headers))

	var cookies []string

	for k, v := range w.headers {
		key := http.CanonicalHeaderKey(k)
		if key == "Set-Cookie" {
			cookies = append(cookies, v...)
			continue
		}

		if bodyHeaderSuppressed(w.statusCode, w.head, key) {
			continue
		}

		headers[k] = strings.Join(v, ",")
	}

	raw := w.body.Bytes()

	body := string(raw)

	isB64 := false
	if !utf8.Valid(raw) {
		body = base64.StdEncoding.EncodeToString(raw)
		isB64 = true
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode:      w.statusCode,
		Headers:         headers,
		Cookies:         cookies,
		Body:            body,
		IsBase64Encoded: isB64,
	}
}

// isInterimStatus reports whether code is an informational status that
// net/http sends ahead of the final response: any 1xx except 101 Switching
// Protocols, which is final.
func isInterimStatus(code int) bool {
	return code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols
}

// bodyAllowedForStatus reports whether a response with status code may carry a
// body. As in net/http, 1xx, 204, and 304 responses may not.
func bodyAllowedForStatus(code int) bool {
	switch {
	case code >= 100 && code <= 199:
		return false
	case code == http.StatusNoContent, code == http.StatusNotModified:
		return false
	default:
		return true
	}
}

// bodyHeaderSuppressed reports whether net/http leaves the canonical header key
// off a response with status code: Transfer-Encoding on any response without a
// body (a HEAD request's included), Content-Length when the status allows no
// body, and Content-Type as well on a 304.
func bodyHeaderSuppressed(code int, head bool, key string) bool {
	switch key {
	case "Transfer-Encoding":
		return head || !bodyAllowedForStatus(code)
	case "Content-Length":
		return !bodyAllowedForStatus(code)
	case constants.HeaderContentType:
		return code == http.StatusNotModified
	default:
		return false
	}
}

// toHTTPRequest converts an APIGatewayV2HTTPRequest to a standard *http.Request.
// Headers, cookies, query parameters, and the request body (including base64-
// encoded binary bodies) are all faithfully translated. The API Gateway request
// ID is preserved as X-Request-ID for downstream middleware. The caller's
// source IP (requestContext.http.sourceIp, the address API Gateway observed on
// the connection) becomes RemoteAddr as "host:0", so handlers can identify the
// client from the transport rather than from client-supplied headers such as
// X-Forwarded-For. RemoteAddr stays empty when the event carries no source IP.
//
//nolint:gocognit,gocyclo // HTTP request conversion requires multiple conditional branches
func toHTTPRequest(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*http.Request, error) {
	rawPath := event.RawPath
	if rawPath == "" {
		rawPath = "/"
	}

	rawURL := "https://lambda.local" + rawPath
	if event.RawQueryString != "" {
		rawURL += "?" + event.RawQueryString
	}

	var bodyReader io.Reader

	if event.IsBase64Encoded {
		decoded, decErr := base64.StdEncoding.DecodeString(event.Body)
		if decErr != nil {
			return nil, decErr
		}

		bodyReader = bytes.NewReader(decoded)
	} else {
		bodyReader = strings.NewReader(event.Body)
	}

	method := event.RequestContext.HTTP.Method
	if method == "" {
		method = http.MethodGet
	}

	// NewRequestWithContext parses rawURL once (and reports a malformed URL as an
	// error), so there is no need for a separate url.Parse round-trip.
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range event.Headers {
		req.Header.Set(k, v)
	}

	for _, cookie := range event.Cookies {
		req.Header.Add("Cookie", cookie)
	}

	// Forward the API Gateway request ID so LoggingMiddleware and error
	// responses can include it without requiring Lambda-specific imports.
	if reqID := event.RequestContext.RequestID; reqID != "" {
		req.Header.Set(constants.HeaderXRequestID, reqID)
	}

	// Populate RemoteAddr from the connection-level source IP. The port is not
	// known to API Gateway, so "0" keeps the value parseable by net.SplitHostPort.
	if ip := event.RequestContext.HTTP.SourceIP; ip != "" {
		req.RemoteAddr = net.JoinHostPort(ip, "0")
	}

	return req, nil
}

// ServeHTTP dispatches an APIGatewayV2HTTPRequest to handler and returns an
// APIGatewayV2HTTPResponse. This is the single integration point between
// API Gateway and a net/http handler. Errors are only returned for request
// conversion failures; handler panics should be caught by RecoverMiddleware.
func ServeHTTP(
	ctx context.Context,
	event events.APIGatewayV2HTTPRequest,
	handler http.Handler,
) (events.APIGatewayV2HTTPResponse, error) {
	req, err := toHTTPRequest(ctx, event)
	if err != nil {
		errResp, marshalErr := marshalJSON(httputil.ErrorResponse{
			Error: constants.ErrorMessageInternalError,
			Code:  constants.ErrorCodeInternalError,
		})

		body := string(errResp)
		if marshalErr != nil {
			body = constants.StaticErrorJSON(constants.ErrorMessageInternalError, constants.ErrorCodeInternalError)
		}

		return events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       body,
			Headers:    map[string]string{constants.HeaderContentType: constants.ContentTypeJSON},
		}, err
	}

	w := newLambdaResponseWriter(req.Method == http.MethodHead)
	handler.ServeHTTP(w, req)

	return w.toAPIGatewayResponse(), nil
}
