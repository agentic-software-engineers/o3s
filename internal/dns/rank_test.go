package dns_test

import (
	"net"
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/dns"
	"github.com/hansehart/o3s/internal/fixture"
)

// The nameservers of the adapters that are up come first by the adapter the host prefers, once each, with the
// deprecated site-local defaults left out.
func TestRankOrdersTheNameserversByTheAdaptersMetric(t *testing.T) {
	addr := netip.MustParseAddr(fixture.Address)
	v6 := netip.MustParseAddr(fixture.AddressV6)
	adapters := []dns.Adapter{
		{Up: true, Metric: 50, Servers: []netip.Addr{addr.Next()}},
		{Up: false, Metric: 1, Servers: []netip.Addr{addr.Next().Next()}},
		{Up: true, Metric: 10, Servers: []netip.Addr{addr, v6, netip.MustParseAddr("fec0:0:0:ffff::1")}},
		{Up: true, Metric: 20, Servers: []netip.Addr{addr}},
	}

	got := dns.Rank(adapters)

	want := []string{net.JoinHostPort(fixture.Address, "53"), net.JoinHostPort(fixture.AddressV6, "53"), net.JoinHostPort(addr.Next().String(), "53")}
	// Require the preferred adapter's nameservers first, each once
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Rank (-want +got):\n%s", diff)
	}
}
