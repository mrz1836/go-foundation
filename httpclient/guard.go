package httpclient

import (
	"fmt"
	"net/netip"
	"syscall"
)

// IsPublicAddress reports whether addr is a public unicast address that a
// guarded client may dial. It is the default address guard of Pinned and
// Guarded, and of WithAddressGuard(IsPublicAddress).
//
// It refuses loopback, unspecified, private (RFC 1918 and fc00::/7),
// link-local, multicast, CGNAT (100.64.0.0/10), "this network" (0.0.0.0/8),
// documentation and benchmarking ranges, reserved space (240.0.0.0/4,
// including broadcast), protocol assignments (192.0.0.0/24), and the cloud
// metadata endpoints. An IPv4-mapped IPv6 address is judged as the IPv4 address
// it maps, and an IPv6 zone is ignored. An IPv6 address that carries an IPv4
// address for a translator or tunnel (NAT64, 6to4, IPv4-compatible) is refused
// when that IPv4 address would be, so 64:ff9b::a00:5, which NAT64 delivers to
// 10.0.0.5, is refused. Teredo and the NAT64 local-use prefix are refused
// outright. An invalid address is refused.
func IsPublicAddress(addr netip.Addr) bool {
	return !blocked(addr)
}

// blockedPrefixes are the refused ranges the netip predicates in
// specialPurpose do not already cover.
var blockedPrefixes = [...]netip.Prefix{ //nolint:gochecknoglobals // read-only lookup table
	netip.MustParsePrefix("0.0.0.0/8"),         // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),     // CGNAT shared address space
	netip.MustParsePrefix("192.0.0.0/24"),      // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),      // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),     // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"),   // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),    // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),       // reserved, incl. broadcast
	netip.MustParsePrefix("fd00:ec2::254/128"), // cloud metadata over IPv6
	// NAT64 local-use prefix (RFC 8215): not globally reachable, and the
	// operator picks where in it the IPv4 address sits, so no single
	// position can be unwrapped and judged.
	netip.MustParsePrefix("64:ff9b:1::/48"),
	// Teredo (RFC 4380): relay-tunneled, with the client's IPv4 address
	// obfuscated.
	netip.MustParsePrefix("2001::/32"),
}

// ipv4Carrier is an IPv6 range whose addresses carry an IPv4 address, starting
// at byte offset, that a translator or tunnel delivers to.
type ipv4Carrier struct {
	prefix netip.Prefix
	offset int
}

// ipv4Carriers are the IPv6 ranges judged by the IPv4 address they carry.
// IPv4-mapped addresses (::ffff:0:0/96) are not listed: blocked unmaps them
// first.
var ipv4Carriers = [...]ipv4Carrier{ //nolint:gochecknoglobals // read-only lookup table
	{netip.MustParsePrefix("64:ff9b::/96"), 12}, // NAT64 well-known prefix (RFC 6052)
	{netip.MustParsePrefix("2002::/16"), 2},     // 6to4 (RFC 3056)
	{netip.MustParsePrefix("::/96"), 12},        // IPv4-compatible (deprecated, RFC 4291)
}

// embeddedIPv4 returns the IPv4 address addr carries for a translator or
// tunnel, and whether it carries one.
func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	for _, c := range ipv4Carriers {
		if c.prefix.Contains(addr) {
			b := addr.As16()
			return netip.AddrFrom4([4]byte(b[c.offset : c.offset+4])), true
		}
	}

	return netip.Addr{}, false
}

// blocked reports whether a dial to addr must be refused. See IsPublicAddress.
func blocked(addr netip.Addr) bool {
	// netip.Prefix.Contains never matches a zoned address, so drop the zone.
	addr = addr.Unmap().WithZone("")
	if !addr.IsValid() || specialPurpose(addr) {
		return true
	}
	// The carried address is IPv4, so this recursion is one level deep.
	if carried, ok := embeddedIPv4(addr); ok && blocked(carried) {
		return true
	}

	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}

	return false
}

// specialPurpose reports the refused ranges the netip predicates classify:
// loopback, unspecified, private (RFC 1918, fc00::/7), link-local unicast and
// multicast, interface-local multicast, and multicast.
func specialPurpose(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsUnspecified() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast()
}

// dialControl returns a net.Dialer Control hook that runs after name
// resolution, on the exact address about to be connected, and refuses it
// unless allow reports true. An address that does not parse as an IP and port,
// such as a Unix socket path, is refused.
func dialControl(allow func(netip.Addr) bool) func(network, address string, conn syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		addrPort, err := netip.ParseAddrPort(address)
		if err != nil || !allow(addrPort.Addr()) {
			return fmt.Errorf("%w: %s", ErrAddressBlocked, address)
		}

		return nil
	}
}
