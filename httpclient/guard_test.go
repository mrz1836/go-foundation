package httpclient_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/httpclient"
)

// TestIsPublicAddressRefusesNonPublicRanges covers every refused range, its
// IPv4-mapped IPv6 form, and public addresses that must stay reachable.
func TestIsPublicAddressRefusesNonPublicRanges(t *testing.T) {
	refused := []string{
		// loopback, unspecified
		"127.0.0.1", "127.255.255.254", "::1", "0.0.0.0", "::",
		// private (RFC 1918, fc00::/7)
		"10.0.0.5", "172.16.0.1", "172.31.255.255", "192.168.1.1", "fd00::1", "fc00::1",
		// link-local unicast (incl. cloud metadata) and multicast
		"169.254.169.254", "169.254.0.1", "fe80::1", "224.0.0.1", "ff02::1",
		// multicast
		"239.255.255.250", "ff0e::1",
		// ranges the netip predicates do not cover
		"0.1.2.3", "100.64.0.1", "100.127.255.254", "192.0.0.8", "192.0.2.10",
		"198.18.0.1", "198.19.255.255", "198.51.100.7", "203.0.113.9", "240.0.0.1",
		"255.255.255.255", "fd00:ec2::254",
		// IPv4-mapped IPv6 forms
		"::ffff:127.0.0.1", "::ffff:10.0.0.5", "::ffff:169.254.169.254", "::ffff:100.64.0.1",
		"::ffff:192.168.0.1", "::ffff:0.0.0.0",
	}
	for _, s := range refused {
		assert.Falsef(t, httpclient.IsPublicAddress(netip.MustParseAddr(s)), "%s must be refused", s)
	}

	public := []string{"93.184.216.34", "2606:4700::1", "::ffff:93.184.216.34", "100.128.0.1", "8.8.8.8"}
	for _, s := range public {
		assert.Truef(t, httpclient.IsPublicAddress(netip.MustParseAddr(s)), "%s must be allowed", s)
	}

	assert.False(t, httpclient.IsPublicAddress(netip.Addr{}), "an invalid address must be refused")
}

// TestIsPublicAddressJudgesEmbeddedIPv4 covers IPv6 addresses that carry an
// IPv4 address for a translator or tunnel.
func TestIsPublicAddressJudgesEmbeddedIPv4(t *testing.T) {
	tests := []struct {
		name   string
		addr   string
		public bool
	}{
		{"NAT64 of a private address (10.0.0.5)", "64:ff9b::a00:5", false},
		{"NAT64 of loopback (127.0.0.1)", "64:ff9b::7f00:1", false},
		{"NAT64 of the metadata endpoint", "64:ff9b::a9fe:a9fe", false},
		{"NAT64 of CGNAT (100.64.0.1)", "64:ff9b::6440:1", false},
		{"NAT64 of this network (0.0.0.0)", "64:ff9b::", false},
		{"local-use NAT64 of a private address", "64:ff9b:1::a00:5", false},
		{"local-use NAT64 of a public address", "64:ff9b:1::808:808", false},
		{"6to4 of a private address (10.0.0.5)", "2002:a00:5::1", false},
		{"6to4 of the metadata endpoint", "2002:a9fe:a9fe::1", false},
		{"6to4 of loopback", "2002:7f00:1::", false},
		{"IPv4-compatible private address", "::a00:5", false},
		{"IPv4-compatible loopback", "::7f00:1", false},
		{"IPv4-compatible metadata endpoint", "::169.254.169.254", false},
		{"Teredo (RFC 4380 example)", "2001:0:4136:e378:8000:63bf:3fff:fdd2", false},
		{"Teredo, any", "2001::1", false},
		{"NAT64 of a public address (8.8.8.8)", "64:ff9b::808:808", true},
		{"6to4 of a public address (8.8.8.8)", "2002:808:808::1", true},
		{"public IPv6", "2606:4700:4700::1111", true},
		{"public IPv6 beside Teredo", "2001:4860:4860::8888", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.public, httpclient.IsPublicAddress(netip.MustParseAddr(tt.addr)), tt.addr)
		})
	}
}

// TestIsPublicAddressIgnoresZones proves a zoned IPv6 address is judged as its
// unzoned form.
func TestIsPublicAddressIgnoresZones(t *testing.T) {
	tests := []struct {
		name   string
		addr   string
		public bool
	}{
		{"zoned NAT64 of a private address", "64:ff9b::a00:5%eth0", false},
		{"zoned Teredo", "2001:0:4136:e378:8000:63bf:3fff:fdd2%eth0", false},
		{"zoned metadata over IPv6", "fd00:ec2::254%eth0", false},
		{"zoned link-local", "fe80::1%eth0", false},
		{"zoned NAT64 of a public address", "64:ff9b::808:808%eth0", true},
		{"zoned public IPv6", "2606:4700:4700::1111%eth0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := netip.MustParseAddr(tt.addr)
			assert.Equal(t, tt.public, httpclient.IsPublicAddress(addr), tt.addr)
			assert.Equal(t, httpclient.IsPublicAddress(addr.WithZone("")), httpclient.IsPublicAddress(addr))
		})
	}
}

// TestIsPublicAddressEmbeddedFollowsIPv4 proves an IPv4 address carried in a
// NAT64, 6to4, or IPv4-compatible IPv6 address is judged exactly as the IPv4
// address itself.
func TestIsPublicAddressEmbeddedFollowsIPv4(t *testing.T) {
	carriers := []struct {
		name  string
		carry func(v4 [4]byte) netip.Addr
	}{
		{"NAT64", func(v4 [4]byte) netip.Addr {
			return netip.AddrFrom16([16]byte{0, 0x64, 0xff, 0x9b, 12: v4[0], 13: v4[1], 14: v4[2], 15: v4[3]})
		}},
		{"6to4", func(v4 [4]byte) netip.Addr {
			return netip.AddrFrom16([16]byte{0x20, 0x02, v4[0], v4[1], v4[2], v4[3], 15: 1})
		}},
		{"IPv4-compatible", func(v4 [4]byte) netip.Addr {
			return netip.AddrFrom16([16]byte{12: v4[0], 13: v4[1], 14: v4[2], 15: v4[3]})
		}},
	}
	ipv4s := []string{
		"8.8.8.8", "93.184.216.34", "100.128.0.1", "1.1.1.1",
		"10.0.0.5", "127.0.0.1", "169.254.169.254", "100.64.0.1", "172.16.0.1",
		"192.168.1.1", "192.0.2.10", "198.18.0.1", "224.0.0.1", "240.0.0.1", "255.255.255.255",
	}
	for _, c := range carriers {
		for _, s := range ipv4s {
			v4 := netip.MustParseAddr(s)
			carried := c.carry(v4.As4())
			assert.Equalf(t, httpclient.IsPublicAddress(v4), httpclient.IsPublicAddress(carried),
				"%s %s must be judged as %s", c.name, carried, s)
		}
	}
}

// FuzzIsPublicAddress checks that no 16-byte address panics the guard, and
// that an IPv4-mapped address is always judged as the IPv4 address it maps.
func FuzzIsPublicAddress(f *testing.F) {
	for _, s := range []string{"127.0.0.1", "8.8.8.8", "::1", "64:ff9b::a00:5", "2002:a00:5::1", "fe80::1"} {
		b := netip.MustParseAddr(s).As16()
		f.Add(b[:])
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) != 16 {
			return
		}
		addr := netip.AddrFrom16([16]byte(raw))
		public := httpclient.IsPublicAddress(addr)
		if addr.Is4In6() {
			assert.Equal(t, httpclient.IsPublicAddress(addr.Unmap()), public)
		}
	})
}

// BenchmarkIsPublicAddress measures the guard on a public IPv4 address (every
// range checked), a loopback address (refused early), and a NAT64 address
// (judged by the IPv4 address it carries).
func BenchmarkIsPublicAddress(b *testing.B) {
	for _, s := range []string{"93.184.216.34", "127.0.0.1", "64:ff9b::808:808"} {
		addr := netip.MustParseAddr(s)
		b.Run(s, func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				_ = httpclient.IsPublicAddress(addr)
			}
		})
	}
}
