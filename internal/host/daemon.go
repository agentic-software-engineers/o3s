package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DaemonAppArmor reports whether the Docker daemon confines containers with AppArmor, as its own security options name it.
func DaemonAppArmor(ctx context.Context) (bool, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .SecurityOptions}}")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	// Name a daemon out of reach
	if err != nil {
		return false, fmt.Errorf("ask Docker for its security options: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var options []string
	// Hand back an answer in another shape
	if err := json.Unmarshal(bytes.TrimSpace(out), &options); err != nil {
		return false, fmt.Errorf("read Docker's security options %q: %w", strings.TrimSpace(string(out)), err)
	}
	// Look for AppArmor among the options, in any case
	for _, o := range options {
		// Report a daemon that names AppArmor
		if strings.Contains(strings.ToLower(o), "apparmor") {
			return true, nil
		}
	}
	return false, nil
}
