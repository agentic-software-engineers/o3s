package dns

import (
	"cmp"
	"net"
	"net/netip"
	"slices"
)

// siteLocal holds the deprecated site-local nameserver defaults some systems still list on adapters that lack a
// nameserver of their own.
var siteLocal = netip.MustParsePrefix("fec0:0:0:ffff::/64")

// adapter is one network adapter as the host reports it: whether it is up, how much the host prefers it, and its
// nameservers.
type adapter struct {
	Up      bool
	Metric  uint32
	Servers []netip.Addr
}

// rank lists the nameservers of the adapters that are up as host and port, the adapter the host prefers first, each
// nameserver once, and the site-local defaults left out.
func rank(adapters []adapter) []string {
	up := slices.DeleteFunc(slices.Clone(adapters), func(a adapter) bool { return !a.Up })
	slices.SortStableFunc(up, func(a, b adapter) int { return cmp.Compare(a.Metric, b.Metric) })
	var list []string
	// Take each adapter's nameservers in the order the host prefers the adapters
	for _, a := range up {
		// Take each nameserver once, leaving the site-local defaults out
		for _, ip := range a.Servers {
			addr := net.JoinHostPort(ip.String(), "53")
			// Keep a nameserver the list lacks so far
			if !siteLocal.Contains(ip) && !slices.Contains(list, addr) {
				list = append(list, addr)
			}
		}
	}
	return list
}
