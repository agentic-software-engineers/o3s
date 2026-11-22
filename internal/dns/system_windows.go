//go:build windows

package dns

import (
	"errors"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// adapterBuffer is the size the adapter list starts with, which fits most hosts in one call.
const adapterBuffer = 15 << 10

// System hands back the nameservers the host uses, read from its network adapters on every query.
func System() Upstream {
	return adapters{}
}

// adapters reads the nameservers of the host's network adapters.
type adapters struct{}

// Nameservers lists the adapters' nameservers, the adapter the host prefers first.
func (adapters) Nameservers() ([]string, error) {
	list, err := readAdapters()
	// Hand back adapters that fail to read
	if err != nil {
		return nil, err
	}
	return rank(list), nil
}

// readAdapters asks the system for every adapter with its state, metric and nameservers, growing the buffer until the
// list fits.
func readAdapters() ([]adapter, error) {
	size := uint32(adapterBuffer)
	// Ask again with the size the system names until the list fits
	for {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC,
			windows.GAA_FLAG_SKIP_UNICAST|windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_FRIENDLY_NAME,
			0, first, &size)
		// Grow the buffer to the size the system names
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		// Hand back a list that fails to read
		if err != nil {
			return nil, err
		}
		var out []adapter
		// Read every adapter in the list
		for a := first; a != nil; a = a.Next {
			out = append(out, adapter{Up: a.OperStatus == windows.IfOperStatusUp, Metric: a.Ipv4Metric, Servers: servers(a)})
		}
		return out, nil
	}
}

// servers reads the nameservers of one adapter.
func servers(a *windows.IpAdapterAddresses) []netip.Addr {
	var out []netip.Addr
	// Read every nameserver the adapter lists
	for s := a.FirstDnsServerAddress; s != nil; s = s.Next {
		// Keep an address that parses
		if ip, ok := netip.AddrFromSlice(s.Address.IP()); ok {
			out = append(out, ip.Unmap())
		}
	}
	return out
}
