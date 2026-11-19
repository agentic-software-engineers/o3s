package cli

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/words"
)

var (
	// failedFeature matches the line the devcontainer CLI names a feature that failed to install with.
	failedFeature = regexp.MustCompile(`Feature "([^"]+)" \(([^)]+)\) failed to install`)
	// unreachable match the lines curl, git, apt and Go's own network name a host they could not reach with.
	unreachable = []*regexp.Regexp{
		regexp.MustCompile(`Could not resolve host:? '?([A-Za-z0-9.-]+\.[A-Za-z]{2,})'?`),
		regexp.MustCompile(`Temporary failure resolving '([^']+)'`),
		regexp.MustCompile(`Failed to connect to ([A-Za-z0-9.-]+\.[A-Za-z]{2,}) port`),
		regexp.MustCompile(`lookup ([A-Za-z0-9.-]+\.[A-Za-z]{2,})( on |: )`),
	}
)

// failure is a step that failed, read so the user knows what to do next: the step and its cause, what fixes it, and
// where the whole log is.
type failure struct {
	step, cause string
	fixes       []string
	log         string
}

// Error writes the step with its cause on the first line, then every fix and the log, one line each.
func (f failure) Error() string {
	lines := []string{f.step + ": " + f.cause}
	// Name every fix
	for _, fix := range f.fixes {
		lines = append(lines, "  "+fix)
	}
	// Name where the whole log is
	if f.log != "" {
		lines = append(lines, "  the whole log: "+f.log)
	}
	return strings.Join(lines, "\n")
}

// hostsIn lists every host an output names as out of reach, once each.
func hostsIn(output string) []string {
	var hosts []string
	// Collect every host a pattern names
	for _, re := range unreachable {
		// Take every host the pattern finds
		for _, m := range re.FindAllStringSubmatch(output, -1) {
			// Take a host not named yet
			if !slices.Contains(hosts, m[1]) {
				hosts = append(hosts, m[1])
			}
		}
	}
	return hosts
}

// buildFailure names why the cage failed to come up, read from what the devcontainer CLI printed: the feature that
// failed, and the hosts the cage's firewall kept the build from with the command that allows them.
func buildFailure(output, logPath string, err error) error {
	f := failure{step: "the cage failed to come up", cause: err.Error(), log: logPath}
	// Name the feature that failed to install
	if m := failedFeature.FindStringSubmatch(output); m != nil {
		f.step = fmt.Sprintf("the cage failed to build at the feature %s (%s)", m[1], m[2])
	}
	// Name the hosts the build could not reach and the command that allows them
	if hosts := hostsIn(output); len(hosts) > 0 {
		f.fixes = append(f.fixes, "it could not reach "+words.JoinAnd(hosts), "allow them with: o3s firewall add "+strings.Join(hosts, " "))
	}
	return f
}

// vmFailure names why a step of the home's VM failed, in the VM manager's words: the hosts this machine could not reach,
// and that o3s init goes on where it stopped.
func vmFailure(step string, err error, logPath string) error {
	f := failure{step: step, cause: err.Error(), log: logPath}
	// Name the hosts this machine could not reach
	if hosts := hostsIn(err.Error()); len(hosts) > 0 {
		f.fixes = append(f.fixes, "this machine could not reach "+words.JoinAnd(hosts)+", check its network")
	}
	f.fixes = append(f.fixes, "run o3s init again, it goes on where it stopped")
	return f
}
