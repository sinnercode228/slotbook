package httpapi

import (
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// clientAddr returns the address a request is counted against. Without
// trusted proxies it is the TCP peer, and X-Forwarded-For is ignored: any
// client can send one. When the peer is a trusted proxy, the header is read
// from the right, where the proxies appended the addresses they saw, and
// the first address that is not a trusted proxy is the client. Entries left
// of it came from the client itself. The result is invalid only when the
// peer address cannot be read.
func clientAddr(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	client := plainAddr(peer.Addr())
	if !isTrusted(client, trusted) {
		return client
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		hop, ok := parseHop(hops[i])
		if !ok {
			// A trusted proxy always appends an address. Counting against the
			// proxy that passed this on is safer than reading further left,
			// where the client writes.
			return client
		}
		client = hop
		if !isTrusted(hop, trusted) {
			return hop
		}
	}
	return client
}

// parseHop reads one X-Forwarded-For entry: an address, possibly in
// brackets or with a port.
func parseHop(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")); err == nil {
		return plainAddr(a), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return plainAddr(ap.Addr()), true
	}
	return netip.Addr{}, false
}

// plainAddr drops the zone and turns an IPv4-mapped IPv6 address into
// IPv4, so that it compares with configured prefixes and keys.
func plainAddr(a netip.Addr) netip.Addr {
	return a.WithZone("").Unmap()
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	return slices.ContainsFunc(trusted, func(p netip.Prefix) bool { return p.Contains(a) })
}

// limitKey groups IPv6 clients by /64, the block one subscriber usually
// gets whole, so stepping through its addresses buys no extra requests.
func limitKey(a netip.Addr) string {
	if a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().String()
	}
	return a.String()
}
