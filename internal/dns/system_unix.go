//go:build unix

package dns

// System hands back the nameservers the host uses, read from its resolv.conf on every query.
func System() Upstream {
	return &ResolvConf{Path: "/etc/resolv.conf"}
}
