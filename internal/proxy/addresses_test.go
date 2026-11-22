package proxy_test

import (
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/proxy"
)

// connectTo asks the proxy for a tunnel to the host on the listed port and hands back the status.
func connectTo(t *testing.T, p *proxy.Proxy, host string) int {
	t.Helper()
	addr := net.JoinHostPort(host, strconv.Itoa(fixture.Port))
	status, _, _ := rawRequest(t, p, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")
	return status
}

// A listed name that resolves to an address inside a network, or to a machine's own services, is refused before any
// dial, whichever range it lands in.
func TestProxyRefusesANameThatResolvesInward(t *testing.T) {
	inward := map[string]string{
		"private ten":           "10.0.0.1",
		"private 172":           "172.16.0.1",
		"private 192":           "192.168.1.1",
		"shared carrier":        "100.64.0.1",
		"loopback":              "127.0.0.1",
		"metadata":              "169.254.169.254",
		"unspecified":           "0.0.0.0",
		"protocol assignments":  "192.0.0.8",
		"benchmarking":          "198.18.0.1",
		"broadcast":             "255.255.255.255",
		"multicast":             "224.0.0.1",
		"loopback v6":           "::1",
		"link-local v6":         "fe80::1",
		"unique local v6":       "fd00::1",
		"mapped loopback":       "::ffff:127.0.0.1",
		"mapped private":        "::ffff:10.0.0.1",
		"aws metadata v6":       "fd00:ec2::254",
		"gce metadata v6":       "fd20:ce::254",
		"unspecified v6":        "::",
		"multicast v6":          "ff02::1",
		"mapped metadata":       "::ffff:169.254.169.254",
		"mapped unspecified v4": "::ffff:0.0.0.0",
	}
	// Try each range on its own
	for name, addr := range inward {
		t.Run(name, func(t *testing.T) {
			p, u, _ := proxying(t, listed(fixture.Port))
			u.resolve(fixture.Host, addr)

			status := connectTo(t, p, fixture.Host)

			// Require a refusal before any dial
			if status != http.StatusForbidden || len(u.dials()) != 0 {
				t.Errorf("CONNECT %s resolving to %s = %d with dials %v, want %d and none", fixture.Host, addr, status, u.dials(), http.StatusForbidden)
			}
		})
	}
}

// The proxy dials the very address it checked, so the connection lands where the check looked.
func TestProxyDialsTheAddressItChecked(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	u.route(fixture.Port, replying(t, true))

	_ = connectTo(t, p, fixture.Host)

	// Require the dial to name the checked address
	if diff := cmp.Diff([]string{net.JoinHostPort(fixture.Address, strconv.Itoa(fixture.Port))}, u.dials()); diff != "" {
		t.Errorf("dials (-want +got):\n%s", diff)
	}
}

// A name that resolves to inward and outward addresses alike is reached on an outward one alone.
func TestProxySkipsTheInwardAddressesOfAName(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	u.route(fixture.Port, replying(t, true))
	u.resolve(fixture.Host, "10.0.0.1", fixture.Address)

	status := connectTo(t, p, fixture.Host)

	// Require the tunnel to open on the outward address alone
	want := []string{net.JoinHostPort(fixture.Address, strconv.Itoa(fixture.Port))}
	if status != http.StatusOK || !cmp.Equal(u.dials(), want) {
		t.Errorf("CONNECT %s = %d with dials %v, want %d and %v", fixture.Host, status, u.dials(), http.StatusOK, want)
	}
}

// An address inside a network that the allowlist names itself is reached, as the user chose it.
func TestProxyReachesAPrivateAddressTheAllowlistNames(t *testing.T) {
	const private = "10.0.0.5"
	p, u, _ := proxying(t, config.Host{Name: private, Ports: []int{fixture.Port}})

	_ = connectTo(t, p, private)

	// Require the proxy to dial the named address
	if diff := cmp.Diff([]string{net.JoinHostPort(private, strconv.Itoa(fixture.Port))}, u.dials()); diff != "" {
		t.Errorf("dials (-want +got):\n%s", diff)
	}
}

// An address the allowlist names that leads to a machine's own services or the metadata server is refused all the
// same.
func TestProxyRefusesAnAlwaysBlockedAddressTheAllowlistNames(t *testing.T) {
	// Try each always-blocked address on its own
	for _, addr := range []string{"127.0.0.1", "169.254.169.254", "::1", "fd00:ec2::254"} {
		t.Run(addr, func(t *testing.T) {
			p, u, _ := proxying(t, config.Host{Name: addr, Ports: []int{fixture.Port}})

			status := connectTo(t, p, addr)

			// Require a refusal before any dial
			if status != http.StatusForbidden || len(u.dials()) != 0 {
				t.Errorf("CONNECT %s = %d with dials %v, want %d and none", addr, status, u.dials(), http.StatusForbidden)
			}
		})
	}
}

// Plain web traffic to a listed name that resolves inward is refused before any dial.
func TestProxyRefusesPlainTrafficThatResolvesInward(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))
	u.resolve(fixture.Host, "169.254.169.254")

	status, _ := get(t, client(t, p), "http://"+fixture.Host+"/")

	// Require a refusal before any dial
	if status != http.StatusForbidden || len(u.dials()) != 0 {
		t.Errorf("GET http://%s resolving to the metadata server = %d with dials %v, want %d and none", fixture.Host, status, u.dials(), http.StatusForbidden)
	}
}

// A proxy still waiting for its policy refuses every host from the first connection.
func TestProxyRefusesEverythingWithoutAPolicy(t *testing.T) {
	u := newUpstreams()
	u.route(fixture.Port, replying(t, true))
	p := proxy.New(proxy.Options{Dial: u.dial, Resolver: u})
	t.Cleanup(func() { _ = p.Close() })

	status := connectTo(t, p, fixture.Host)

	// Require a refusal before any dial
	if status != http.StatusForbidden || len(u.dials()) != 0 {
		t.Errorf("CONNECT %s without a policy = %d with dials %v, want %d and none", fixture.Host, status, u.dials(), http.StatusForbidden)
	}
}
