package httpclient

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// maxHostLength is the longest DNS name, in bytes, without a trailing dot.
const maxHostLength = 253

// maxLabelLength is the longest DNS label, in bytes.
const maxLabelLength = 63

// normalizeHost validates an allowlist entry and returns its canonical form:
// lowercase, without a trailing dot. An entry is a bare host name or IP
// literal, with no scheme, port, path, userinfo, or wildcard.
func normalizeHost(host string) (string, error) {
	h := canonicalHost(host)
	if h == "" {
		return "", fmt.Errorf("%w: empty host", ErrInvalidConfig)
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		if addr.Zone() != "" {
			return "", fmt.Errorf("%w: host %q has an IPv6 zone", ErrInvalidConfig, host)
		}

		return addr.String(), nil
	}
	if !validHostName(h) {
		return "", fmt.Errorf("%w: %q is not a bare host name", ErrInvalidConfig, host)
	}

	return h, nil
}

// canonicalHost lowercases host and drops one trailing dot.
func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// requestHost returns the canonical host of u, as an allowlist compares it.
// An IP literal is reformatted so 2001:DB8::1 and 2001:db8:0::1 match.
func requestHost(u *url.URL) string {
	h := canonicalHost(u.Hostname())
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.String()
	}

	return h
}

// validHostName reports whether h, already canonical, is a DNS name of
// letters, digits, hyphens, and underscores, in labels that neither start nor
// end with a hyphen.
func validHostName(h string) bool {
	if len(h) > maxHostLength {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if !validLabel(label) {
			return false
		}
	}

	return true
}

// validLabel reports whether label is one valid DNS label.
func validLabel(label string) bool {
	if label == "" || len(label) > maxLabelLength {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := range len(label) {
		c := label[i]
		if !isHostByte(c) {
			return false
		}
	}

	return true
}

// isHostByte reports whether c may appear in a canonical host label.
func isHostByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
}
