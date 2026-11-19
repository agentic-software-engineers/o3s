package stacktest

import (
	"strings"
	"testing"
)

// cageNetwork is the cage network the dev overlay gives the stack.
const cageNetwork = "o3s-dev-cage"

// CageRoute reads the route address Docker hands every container on the cage network.
func CageRoute(tb testing.TB) (string, error) {
	tb.Helper()
	out, err := run(tb.Context(), tb, "docker", "network", "inspect", cageNetwork, "--format", "{{(index .IPAM.Config 0).Gateway}}")
	return strings.TrimSpace(out), err
}

// OnTheCageNetwork runs a script in a throwaway container on the cage network, built from the gateway's dev image for
// its tools, and hands back what it printed.
func OnTheCageNetwork(tb testing.TB, script string) (string, error) {
	tb.Helper()
	return run(tb.Context(), tb, "docker", "run", "--rm", "--network", cageNetwork, "--entrypoint", "bash", "o3s-gateway:dev", "-c", script)
}
