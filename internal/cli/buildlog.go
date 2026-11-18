package cli

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	// failedFeature matches the line the devcontainer CLI names a feature that failed to install with.
	failedFeature = regexp.MustCompile(`Feature "([^"]+)" \(([^)]+)\) failed to install`)
	// unreachable match the lines curl, git and apt name a host they could not reach with.
	unreachable = []*regexp.Regexp{
		regexp.MustCompile(`Could not resolve host:? '?([A-Za-z0-9.-]+\.[A-Za-z]{2,})'?`),
		regexp.MustCompile(`Temporary failure resolving '([^']+)'`),
		regexp.MustCompile(`Failed to connect to ([A-Za-z0-9.-]+\.[A-Za-z]{2,}) port`),
	}
)

// buildFailure names why the cage failed to come up, read from what the devcontainer CLI printed: the feature that
// failed, the hosts the build could not reach with the command that allows them, and where the whole log is.
func buildFailure(output, logPath string, err error) error {
	step := "the cage failed to come up"
	// Name the feature that failed to install
	if m := failedFeature.FindStringSubmatch(output); m != nil {
		step = fmt.Sprintf("the cage failed to build at the feature %s (%s)", m[1], m[2])
	}
	var hosts []string
	// Collect every host the build could not reach, once each
	for _, re := range unreachable {
		// Take every host the pattern names
		for _, m := range re.FindAllStringSubmatch(output, -1) {
			// Take a host not named yet
			if !slices.Contains(hosts, m[1]) {
				hosts = append(hosts, m[1])
			}
		}
	}
	lines := []string{step + ": " + err.Error()}
	// Name the hosts the build could not reach and the command that allows them
	if len(hosts) > 0 {
		lines = append(lines, "  it could not reach "+joinAnd(hosts), "  allow them with: o3s firewall add "+strings.Join(hosts, " "))
	}
	// Name where the whole log is
	if logPath != "" {
		lines = append(lines, "  the whole log: "+logPath)
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}
