package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

func TestClientAddr(t *testing.T) {
	proxies := []string{"10.0.0.0/8", "192.0.2.0/24", "2001:db8:ffff::/48"}
	cases := []struct {
		name    string
		trusted []string
		peer    string
		xff     []string
		want    string
	}{
		{name: "peer without proxies", peer: "203.0.113.7:51234", want: "203.0.113.7"},
		{name: "header ignored without trusted proxies", peer: "203.0.113.7:51234", xff: []string{"198.51.100.20"}, want: "203.0.113.7"},
		{name: "header ignored from a peer that is not a trusted proxy", trusted: proxies, peer: "203.0.113.7:51234", xff: []string{"198.51.100.20"}, want: "203.0.113.7"},
		{name: "trusted proxy without a header", trusted: proxies, peer: "10.0.0.5:443", want: "10.0.0.5"},
		{name: "one trusted proxy", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"198.51.100.20"}, want: "198.51.100.20"},
		{
			name: "chain of trusted proxies after a made-up entry", trusted: proxies, peer: "10.0.0.5:443",
			xff: []string{"6.6.6.6, 198.51.100.20, 192.0.2.10"}, want: "198.51.100.20",
		},
		{
			name: "chain split over several header lines", trusted: proxies, peer: "10.0.0.5:443",
			xff: []string{"6.6.6.6", "198.51.100.20,192.0.2.10"}, want: "198.51.100.20",
		},
		{name: "every hop trusted takes the left-most", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"10.1.1.1, 192.0.2.10"}, want: "10.1.1.1"},
		{name: "an unreadable hop stops at the proxy that wrote it", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"198.51.100.20, 192.0.2.10, unknown"}, want: "10.0.0.5"},
		{name: "an empty hop stops at the proxy that wrote it", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"6.6.6.6, "}, want: "10.0.0.5"},
		{name: "entry with a port", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"198.51.100.20:4711"}, want: "198.51.100.20"},
		{name: "IPv6 peer", peer: "[2001:db8:1:2::abcd]:5000", want: "2001:db8:1:2::abcd"},
		{name: "IPv6 peer with a zone", peer: "[fe80::1%eth0]:5000", want: "fe80::1"},
		{name: "IPv4-mapped peer", peer: "[::ffff:203.0.113.7]:5000", want: "203.0.113.7"},
		{name: "IPv4-mapped trusted proxy", trusted: proxies, peer: "[::ffff:10.0.0.5]:443", xff: []string{"198.51.100.20"}, want: "198.51.100.20"},
		{name: "IPv6 proxy and client", trusted: proxies, peer: "[2001:db8:ffff::1]:443", xff: []string{"2001:db8:1:2::abcd"}, want: "2001:db8:1:2::abcd"},
		{name: "IPv6 entry in brackets with a port", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"[2001:db8:1:2::abcd]:4711"}, want: "2001:db8:1:2::abcd"},
		{name: "IPv6 entry in brackets", trusted: proxies, peer: "10.0.0.5:443", xff: []string{"[2001:db8:1:2::abcd]"}, want: "2001:db8:1:2::abcd"},
		{name: "unreadable peer", peer: "pipe", want: "invalid IP"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/holds", nil)
			req.RemoteAddr = c.peer
			for _, v := range c.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := clientAddr(req, prefixes(t, c.trusted...)).String(); got != c.want {
				t.Fatalf("clientAddr = %s, want %s", got, c.want)
			}
		})
	}
}

func TestLimitKeyGroupsIPv6ByItsSlash64(t *testing.T) {
	key := func(s string) string { return limitKey(netip.MustParseAddr(s)) }
	if got := key("203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("IPv4 key %q", got)
	}
	if a, b := key("2001:db8:1:2::abcd"), key("2001:db8:1:2:ffff:ffff:ffff:ffff"); a != b {
		t.Fatalf("one /64, two keys: %q and %q", a, b)
	}
	if a, b := key("2001:db8:1:2::abcd"), key("2001:db8:1:3::abcd"); a == b {
		t.Fatalf("two /64s share the key %q", a)
	}
}
