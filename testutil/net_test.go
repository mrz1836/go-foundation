package testutil_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/go-foundation/testutil"
)

// TestFreeAddrReturnsBindableLoopbackAddress proves FreeAddr hands back a
// loopback host:port that a server can actually bind — the port was reserved and
// released, so re-binding it succeeds.
func TestFreeAddrReturnsBindableLoopbackAddress(t *testing.T) {
	t.Parallel()
	addr := testutil.FreeAddr(t)

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
	assert.NotEmpty(t, port)

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	require.NoError(t, err, "the released address is free to bind")
	require.NoError(t, ln.Close())
}
