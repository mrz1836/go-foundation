package testutil

import (
	"context"
	"net"
	"testing"
)

// FreeAddr reserves and releases an ephemeral loopback port and returns its
// address (host:port), so a test can bind a server to a port known to be free.
// The listener is closed before returning, so there is an inherent race window
// before the caller re-binds; it is a pragmatic helper for tests, not a
// guarantee.
func FreeAddr(t testing.TB) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free address: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release free address: %v", err)
	}
	return addr
}
