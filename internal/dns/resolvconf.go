package dns

import (
	"bufio"
	"bytes"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
)

// ResolvConf reads the nameservers of a resolv.conf file afresh on every call, and keeps the last good list while the
// file is missing or empty, as it is for a moment while the system rewrites it.
type ResolvConf struct {
	// Path is the file to read.
	Path string

	mu   sync.Mutex
	last []string
}

// Nameservers lists the file's nameservers in order as host and port.
func (r *ResolvConf) Nameservers() ([]string, error) {
	data, err := os.ReadFile(r.Path)
	list := nameservers(data)
	r.mu.Lock()
	defer r.mu.Unlock()
	// Keep the last good list while the file is unreadable or empty
	if err != nil || len(list) == 0 {
		// Report the failure until a good list arrives
		if r.last == nil {
			return nil, err
		}
		return slices.Clone(r.last), nil
	}
	r.last = list
	return slices.Clone(list), nil
}

// nameservers reads every nameserver line of a resolv.conf in order.
func nameservers(data []byte) []string {
	var list []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	// Look at every line
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		// Take a nameserver line that names an address
		if len(fields) >= 2 && fields[0] == "nameserver" {
			// Keep an address that parses, its zone included
			if ip, err := netip.ParseAddr(fields[1]); err == nil {
				list = append(list, net.JoinHostPort(ip.String(), "53"))
			}
		}
	}
	return list
}
