package menu

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/project"
	"github.com/hansehart/o3s/internal/words"
)

// errEmpty marks a line the user has yet to type.
var errEmpty = errors.New("type at least one")

// checkHosts reads hosts separated by spaces, naming the first one the gateway would turn away.
func checkHosts(line string) (string, error) {
	hosts := strings.Fields(line)
	// Wait for a host
	if len(hosts) == 0 {
		return "", errEmpty
	}
	// Check every host
	for _, h := range hosts {
		// Name the host the gateway would turn away
		if err := config.CheckHost(h); err != nil {
			return "", fmt.Errorf("%q is not a host name", h)
		}
	}
	return words.Count(len(hosts), "host", "hosts"), nil
}

// checkPorts reads ports separated by commas, naming the first one outside the TCP range.
func checkPorts(line string) (string, error) {
	parts := strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' })
	// Wait for a port
	if len(parts) == 0 {
		return "", errEmpty
	}
	// Check every port
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		// Name the port outside the range
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("%q is not a port from 1 to 65535", p)
		}
	}
	return "ports " + strings.Join(parts, ", "), nil
}

// checkFor reads how long hosts stay open.
func checkFor(line string) (string, error) {
	d, err := config.ParseFor(strings.TrimSpace(line))
	// Name a duration that reads as no time
	if err != nil {
		return "", fmt.Errorf("%q is not a duration like 90s, 20m, 1h or 1h30m", strings.TrimSpace(line))
	}
	return "open for " + spoken(d), nil
}

// checkSecretName reads the name of a secret, as the proxy takes it.
func checkSecretName(line string) (string, error) {
	name := strings.TrimSpace(line)
	// Name a secret name the proxy would turn away
	if !config.IsSecretName(name) {
		return "", fmt.Errorf("%q is not an uppercase env name", name)
	}
	return "reads as $" + name, nil
}

// checkStore reads a store's id.
func checkStore(line string) (string, error) {
	id := strings.TrimSpace(line)
	// Name an id that names no repository in full
	if config.CheckStore(id) != nil {
		return "", fmt.Errorf("%q is not a registry and a repository", id)
	}
	return "reads as a store", nil
}

// checkRepos reads repositories separated by spaces, naming the first one o3s cannot read, and where each clones.
func checkRepos(line string) (string, error) {
	args := strings.Fields(line)
	// Wait for a repository
	if len(args) == 0 {
		return "", errEmpty
	}
	var paths []string
	// Check every repository
	for _, arg := range args {
		r, err := project.Parse(arg)
		// Name the repository o3s cannot read
		if err != nil {
			return "", fmt.Errorf("%q is not owner/repo, host/owner/repo or a git URL", arg)
		}
		paths = append(paths, r.URL)
	}
	return strings.Join(paths, ", "), nil
}

// checkName holds a typed name to the one it must match.
func checkName(want string) func(string) (string, error) {
	return func(line string) (string, error) {
		// Name a name that differs
		if strings.TrimSpace(line) != want {
			return "", fmt.Errorf("type %s", want)
		}
		return "matches", nil
	}
}
