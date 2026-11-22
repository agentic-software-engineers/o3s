package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// Resolver finds the addresses a name leads to.
type Resolver interface {
	// LookupNetIP answers with the addresses of the host.
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// errInward reports a host whose address leads into a network or to a machine's own services.
var errInward = errors.New("address leads inward")

// alwaysBlocked are the addresses the proxy refuses even where the allowlist names them: a machine's own services,
// link-local and the cloud metadata servers.
var alwaysBlocked = prefixes(
	"0.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"::/128",
	"::1/128",
	"fe80::/10",
	"fd20:ce::254/128",
	"fd00:ec2::254/128",
	"fd00:ec2::23/128",
)

// inward are the addresses a listed name is refused at, on top of the always-blocked ones: private and shared networks
// and the special-use ranges. The documentation ranges stay open, as they stay off the internet's routes.
var inward = prefixes(
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"192.0.0.0/24",
	"198.18.0.0/15",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"fc00::/7",
	"ff00::/8",
)

// prefixes reads the ranges.
func prefixes(ranges ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ranges))
	// Read each range
	for i, r := range ranges {
		out[i] = netip.MustParsePrefix(r)
	}
	return out
}

// within reports whether the address lies in one of the ranges.
func within(ip netip.Addr, ranges []netip.Prefix) bool {
	// Look at each range
	for _, r := range ranges {
		// Report the first range that holds the address
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// reach dials a host the policy let through: an address as it stands where it lies outside the always-blocked ranges,
// and a name on the first of its outward addresses that answers, dialing the very address it checked.
func (p *Proxy) reach(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	// Hand back an address that lacks its port
	if err != nil {
		return nil, err
	}
	// Dial an address the allowlist names, once it clears the always-blocked ranges
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		// Refuse an always-blocked address
		if within(ip, alwaysBlocked) {
			return nil, fmt.Errorf("%w: %s", errInward, ip)
		}
		return p.raw(ctx, network, addr)
	}
	ips, err := p.resolver.LookupNetIP(ctx, "ip", host)
	// Hand back a name that fails to resolve
	if err != nil {
		return nil, err
	}
	var last error
	// Try each outward address in the order the resolver gave
	for _, ip := range ips {
		ip = ip.Unmap()
		// Skip an address that leads inward
		if within(ip, alwaysBlocked) || within(ip, inward) {
			continue
		}
		c, err := p.raw(ctx, network, net.JoinHostPort(ip.String(), port))
		// Hand back the first connection that opens
		if err == nil {
			return c, nil
		}
		last = err
	}
	// Report a name whose every address leads inward
	if last == nil {
		return nil, fmt.Errorf("%w: %s resolves to %v", errInward, host, ips)
	}
	return nil, last
}
