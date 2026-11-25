package httpclient

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Errors returned by clients and helpers. Test for them with errors.Is.
var (
	// ErrInvalidConfig reports options that are invalid or contradict each
	// other.
	ErrInvalidConfig = errors.New("httpclient: invalid configuration")

	// ErrSchemeNotAllowed reports a request, or a redirect hop, whose URL
	// scheme the client does not allow.
	ErrSchemeNotAllowed = errors.New("httpclient: scheme not allowed")

	// ErrHostNotAllowed reports a request, or a redirect hop, to a host
	// outside the client's allowlist.
	ErrHostNotAllowed = errors.New("httpclient: host not allowed")

	// ErrAddressBlocked reports a dial to an address the address guard refuses.
	ErrAddressBlocked = errors.New("httpclient: destination address not allowed")

	// ErrTooManyRedirects reports a redirect chain longer than the client
	// allows.
	ErrTooManyRedirects = errors.New("httpclient: too many redirects")

	// ErrResponseTooLarge reports a response body larger than the limit.
	ErrResponseTooLarge = errors.New("httpclient: response body too large")

	// ErrUnexpectedStatus reports a response status outside 2xx. The error
	// is a *StatusError, which carries the status code.
	ErrUnexpectedStatus = errors.New("httpclient: unexpected status")

	// ErrInvalidJSON reports a response body that does not decode into the
	// target. The error never quotes the body.
	ErrInvalidJSON = errors.New("httpclient: invalid JSON response")
)

// StatusError reports a response whose status is outside 2xx. It never holds
// the response body or headers, so it is safe to log.
type StatusError struct {
	// StatusCode is the response's HTTP status code.
	StatusCode int

	// Method is the request's HTTP method.
	Method string

	// Host is the request URL's host, with its port when the URL has one.
	Host string
}

// Error describes the status, method, and host.
func (e *StatusError) Error() string {
	return fmt.Sprintf("httpclient: %s %s: unexpected status %d", e.Method, e.Host, e.StatusCode)
}

// Unwrap returns ErrUnexpectedStatus, so errors.Is matches it.
func (e *StatusError) Unwrap() error {
	return ErrUnexpectedStatus
}

// RedactURL returns u as scheme://host/path: without its userinfo, query
// string, or fragment, which can carry credentials. A nil u returns "".
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	safe := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}

	return safe.String()
}

// RedactError returns err with the URL in any *url.Error it holds reduced to
// scheme://host/path, so neither the URL's userinfo nor its query string
// appears in err's message. (*http.Client).Do returns its errors as a
// *url.Error that carries the full URL. Other errors are returned unchanged.
func RedactError(err error) error {
	var uerr *url.Error
	if !errors.As(err, &uerr) {
		return err
	}
	redacted := *uerr
	redacted.URL = redactRawURL(uerr.URL)
	if err.Error() == uerr.Error() {
		// err is the *url.Error itself, or a wrapper that adds nothing to it.
		return &redacted
	}

	// The chain continues at the redacted *url.Error rather than at err, so
	// errors.As cannot reach the original URL. Whatever err wrapped between
	// itself and the *url.Error is dropped from the chain.
	msg := err.Error()
	if strings.Contains(msg, uerr.Error()) {
		msg = strings.Replace(msg, uerr.Error(), redacted.Error(), 1)
	} else {
		// A wrapper reformatted the inner message; keep only the redacted one
		// rather than risk keeping the URL.
		msg = redacted.Error()
	}

	return &redactedError{err: &redacted, msg: msg}
}

// redactRawURL redacts a URL string. A string that does not parse is replaced
// entirely, since it cannot be shown to be free of credentials.
func redactRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[unparsable URL]"
	}

	return RedactURL(u)
}

// redactedError carries a redacted message for an error that wrapped a
// *url.Error, and unwraps to the redacted *url.Error.
type redactedError struct {
	err error
	msg string
}

// Error returns the redacted message.
func (e *redactedError) Error() string {
	return e.msg
}

// Unwrap returns the redacted *url.Error.
func (e *redactedError) Unwrap() error {
	return e.err
}
