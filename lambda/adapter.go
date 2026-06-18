// Package lambda provides a lightweight AWS API Gateway v2 ↔ net/http adapter.
//
// It is a minimal, dependency-free bridge over the AWS Lambda Go events.
// ServeHTTP converts an APIGatewayV2HTTPRequest into a standard *http.Request,
// dispatches it to any http.Handler (e.g., a chi router), and converts the
// captured response back to an APIGatewayV2HTTPResponse.
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
	"net/http"
	"net/url"
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

// lambdaResponseWriter implements http.ResponseWriter and captures the response
// so it can be converted to an APIGatewayV2HTTPResponse.
type lambdaResponseWriter struct {
	statusCode int
	headers    http.Header
	body       *bytes.Buffer
}

func newLambdaResponseWriter() *lambdaResponseWriter {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	return &lambdaResponseWriter{
		statusCode: http.StatusOK,
		headers:    make(http.Header),
		body:       buf,
	}
}

func (w *lambdaResponseWriter) Header() http.Header         { return w.headers }
func (w *lambdaResponseWriter) WriteHeader(code int)        { w.statusCode = code }
func (w *lambdaResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

// toAPIGatewayResponse converts the captured response to an APIGatewayV2HTTPResponse.
// Binary bodies (non-valid-UTF-8) are base64-encoded and IsBase64Encoded is set.
// Set-Cookie headers are routed to the dedicated Cookies field because API Gateway v2
// does not support comma-joined Set-Cookie values in the single-value Headers map.
func (w *lambdaResponseWriter) toAPIGatewayResponse() events.APIGatewayV2HTTPResponse {
	defer bufferPool.Put(w.body)

	headers := make(map[string]string, len(w.headers))

	var cookies []string

	for k, v := range w.headers {
		if http.CanonicalHeaderKey(k) == "Set-Cookie" {
			cookies = append(cookies, v...)
			continue
		}

		headers[k] = strings.Join(v, ",")
	}

	body := w.body.Bytes()
	if utf8.Valid(body) {
		return events.APIGatewayV2HTTPResponse{
			StatusCode:      w.statusCode,
			Headers:         headers,
			Cookies:         cookies,
			Body:            string(body),
			IsBase64Encoded: false,
		}
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode:      w.statusCode,
		Headers:         headers,
		Cookies:         cookies,
		Body:            base64.StdEncoding.EncodeToString(body),
		IsBase64Encoded: true,
	}
}

// toHTTPRequest converts an APIGatewayV2HTTPRequest to a standard *http.Request.
// Headers, cookies, query parameters, and the request body (including base64-
// encoded binary bodies) are all faithfully translated. The API Gateway request
// ID is preserved as X-Request-ID for downstream middleware.
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

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
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

	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bodyReader)
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
		req.Header.Set("X-Request-ID", reqID)
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
		errResp, marshalErr := json.Marshal(httputil.ErrorResponse{
			Error: constants.ErrorMessageInternalError,
			Code:  constants.ErrorCodeInternalError,
		})

		body := string(errResp)
		if marshalErr != nil {
			body = `{"error":"` + constants.ErrorMessageInternalError + `","code":"` + constants.ErrorCodeInternalError + `"}`
		}

		return events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       body,
			Headers:    map[string]string{constants.HeaderContentType: constants.ContentTypeJSON},
		}, err
	}

	w := newLambdaResponseWriter()
	handler.ServeHTTP(w, req)

	return w.toAPIGatewayResponse(), nil
}
