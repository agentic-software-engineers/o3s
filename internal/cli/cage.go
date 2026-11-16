package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/ide"
	"github.com/hansehart/o3s/internal/rebuild"
)

// newCage builds the commands that step into the home's cage.
func newCage() *cobra.Command {
	c := group("cage", "Open the home's cage in VS Code or a shell")
	c.AddCommand(newCageIDE(), newCageShell())
	return c
}

// newCageIDE builds the command that opens the home's workspace in VS Code straight in the cage.
func newCageIDE() *cobra.Command {
	return &cobra.Command{
		Use:   "ide",
		Short: "Open the home's workspace in VS Code inside the cage",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := cageHome()
			// Hand back a folder that holds no home
			if err != nil {
				return err
			}
			code, err := exec.LookPath("code")
			// Point at the VS Code CLI the command runs
			if err != nil {
				return errors.New("o3s cage ide runs the VS Code CLI code, put it on PATH (VS Code: Shell Command: Install 'code' command in PATH)")
			}
			env := ide.Detect()
			// Hand back what VS Code needs before it opens the cage
			if err := env.Check(cmd.Context()); err != nil {
				return err
			}
			host, err := env.HostPath(cmd.Context(), dir)
			// Hand back a folder the environment fails to name
			if err != nil {
				return err
			}
			uri := ide.URI(host, filepath.Base(home.Workspace(dir)))
			// Hand back a VS Code that fails to start
			if err := exec.CommandContext(cmd.Context(), code, "--file-uri", uri).Run(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Opening %s in the cage\n", dir)
			return nil
		},
	}
}

// newCageShell builds the command that raises the home's cage and opens a shell inside it.
func newCageShell() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open a shell inside the home's cage",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := cageHome()
			// Hand back a folder that holds no home
			if err != nil {
				return err
			}
			c, err := raise(cmd, dir)
			// Hand back a cage that fails to come up
			if err != nil {
				return err
			}
			return syscall.Exec(c.cli, c.exec("bash"), os.Environ()) //nolint:gosec // o3s hands the terminal to the devcontainer CLI it found, with the command line it builds itself
		},
	}
}

// cageHome finds the home a cage command works on, and turns away a folder that holds none.
func cageHome() (string, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return "", err
	}
	_, err = config.Load(dir)
	return dir, err
}

// cage is a raised cage: the devcontainer CLI that raised it, its home, and the container up reported.
type cage struct {
	cli, dir, id string
}

// raised is the result devcontainer up prints on stdout.
type raised struct {
	ContainerID string `json:"containerId"`
}

// raise brings the home's cage up with the devcontainer CLI, its log on stderr, and hands back the container it reports,
// so every later command reaches that same container, however the cage first came up.
func raise(cmd *cobra.Command, dir string) (cage, error) {
	cli, ok := rebuild.CLI()
	// Point at the installer where the devcontainer CLI is missing
	if !ok {
		return cage{}, fmt.Errorf("%s runs the devcontainer CLI, which install.sh installs unless run with --no-devcontainer", cmd.CommandPath())
	}
	up := exec.CommandContext(cmd.Context(), cli, "up", "--workspace-folder", dir)
	var out bytes.Buffer
	up.Stdout, up.Stderr = &out, cmd.ErrOrStderr()
	// Hand back a cage that fails to come up
	if err := up.Run(); err != nil {
		return cage{}, fmt.Errorf("raise the cage: %w\n%s", err, out.String())
	}
	var r raised
	// Hand back a result that names no container
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.ContainerID == "" {
		return cage{}, fmt.Errorf("raise the cage: devcontainer up named no container in %q", out.String())
	}
	return cage{cli: cli, dir: dir, id: r.ContainerID}, nil
}

// exec is the devcontainer CLI's command line that runs a command in the cage.
func (c cage) exec(args ...string) []string {
	return append([]string{c.cli, "exec", "--workspace-folder", c.dir, "--container-id", c.id}, args...)
}

// run runs a command inside the cage, its stderr on stderr, and hands back its stdout and exit code.
func (c cage) run(cmd *cobra.Command, args ...string) (string, int, error) {
	line := c.exec(args...)
	run := exec.CommandContext(cmd.Context(), line[0], line[1:]...) //nolint:gosec // o3s runs the devcontainer CLI it found, with the commands it builds itself
	var out bytes.Buffer
	run.Stdout, run.Stderr = &out, cmd.ErrOrStderr()
	err := run.Run()
	// Hand back the exit code of a command that ran and failed
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return out.String(), exit.ExitCode(), nil
	}
	return out.String(), 0, err
}
