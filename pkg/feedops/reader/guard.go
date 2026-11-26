package reader

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
)

// ErrPrivateAddress: a feed, page or image URL leads to an address that isn't
// on the public internet (loopback, a private network, link-local, the cloud
// metadata service). pcom fetches what users give it, so it must not reach the
// machines next to it.
var ErrPrivateAddress = errors.New("address is not on the public internet")

// nonPublic are the ranges netip has no predicate for that still aren't
// public: shared address space, IETF protocol assignments, benchmarking,
// documentation, reserved and the IPv6 prefixes that embed an IPv4 address.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

// IsPublicAddress reports whether ip is on the public internet.
func IsPublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()

	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}

	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}

	return true
}

// publicOnly is a net.Dialer Control: it runs after the name is resolved,
// for every connection, redirects included, so neither a redirect nor a name
// that resolves to a private address gets past it.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}

	if !IsPublicAddress(ip) {
		return fmt.Errorf("%s: %w", ip, ErrPrivateAddress)
	}

	return nil
}

// publicClient is an HTTP client that only connects to public addresses. It
// dials directly, without the environment's proxy: through a proxy the dialed
// address would be the proxy's, and the guard would check the wrong address.
func publicClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&net.Dialer{Control: publicOnly}).DialContext

	return &http.Client{Transport: transport}
}
