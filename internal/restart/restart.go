// Package restart finds the containers an o3s home runs, and stops or removes them.
package restart

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Containers lists the running containers of one service of the home's stack.
func Containers(ctx context.Context, home, service string) ([]string, error) {
	project := "label=com.docker.compose.project.working_dir=" + filepath.Join(home, ".devcontainer")
	out, err := docker(ctx, "ps", "-q", "--filter", project, "--filter", "label=com.docker.compose.service="+service)
	return strings.Fields(out), err
}

// Stop stops every container of the home's stack, and reports how many it stopped.
func Stop(ctx context.Context, home string) (int, error) {
	return each(ctx, home, "stop")
}

// Remove removes every container of the home's stack, its files and volumes kept, and reports how many it removed.
func Remove(ctx context.Context, home string) (int, error) {
	return each(ctx, home, "rm", "--force")
}

// each runs one engine command on every container of the home's stack, running or stopped, and reports how many it
// reached.
func each(ctx context.Context, home string, verb ...string) (int, error) {
	out, err := docker(ctx, "ps", "-aq", "--filter", "label=com.docker.compose.project.working_dir="+filepath.Join(home, ".devcontainer"))
	// Hand back an engine that fails to answer
	if err != nil {
		return 0, err
	}
	ids := strings.Fields(out)
	// Leave a home free of containers as it is
	if len(ids) == 0 {
		return 0, nil
	}
	_, err = docker(ctx, append(verb, ids...)...)
	return len(ids), err
}

// docker runs the container engine's CLI and hands back what it printed.
func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Hand back the engine's own output along with the failure
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
