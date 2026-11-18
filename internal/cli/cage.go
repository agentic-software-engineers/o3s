package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/ide"
	"github.com/hansehart/o3s/internal/paths"
	"github.com/hansehart/o3s/internal/rebuild"
	"github.com/hansehart/o3s/internal/restart"
)

// newCage builds the commands that show the home's cage and step into it.
func newCage() *cobra.Command {
	c := group("cage", "Show the home's cage, or open it in VS Code or a shell", showCage)
	c.AddCommand(newCageIDE(), newCageShell())
	return c
}

// showCage writes whether each part of the home's stack runs.
func showCage(cmd *cobra.Command) error {
	dir, err := cageHome()
	// Hand back a folder that holds no home
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PART\tSTATE")
	// Write whether each part of the stack runs
	for _, service := range []string{"cage", "gateway", "proxy"} {
		ids, err := restart.Containers(cmd.Context(), dir, service)
		state := "stopped"
		// Name a part whose state stays unknown
		if err != nil {
			state = "unknown"
		}
		// Name a part that runs
		if len(ids) > 0 {
			state = "running"
		}
		fmt.Fprintf(w, "%s\t%s\n", service, state)
	}
	return w.Flush()
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
			env := ide.Detect()
			// Hand back what VS Code needs before it opens the cage
			if err := env.Check(cmd.Context()); err != nil {
				return err
			}
			code, err := exec.LookPath("code")
			// Point at the VS Code CLI the command runs
			if err != nil {
				return errors.New("o3s cage ide runs the VS Code CLI code, put it on PATH (VS Code: Shell Command: Install 'code' command in PATH)")
			}
			host, err := env.HostPath(cmd.Context(), dir)
			// Hand back a folder the environment fails to name
			if err != nil {
				return err
			}
			uri := ide.URI(host, env.SSHHost(), filepath.Base(home.Workspace(dir)))
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
	var out, progress bytes.Buffer
	logPath := filepath.Join(paths.Home(dir), "cage.log")
	log, err := os.Create(logPath)
	// Keep the progress in memory alone where the log fails to open
	if err != nil {
		log, logPath = nil, ""
	}
	up.Stdout, up.Stderr = &out, io.MultiWriter(cmd.ErrOrStderr(), &progress, orDiscard(log))
	err = up.Run()
	// Close the log once the build ends
	if log != nil {
		_ = log.Close()
	}
	// Hand back a cage that fails to come up, named by its step and its cause
	if err != nil {
		return cage{}, buildFailure(progress.String()+out.String(), logPath, err)
	}
	var r raised
	// Hand back a result that names no container
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.ContainerID == "" {
		return cage{}, fmt.Errorf("raise the cage: devcontainer up named no container in %q", out.String())
	}
	return cage{cli: cli, dir: dir, id: r.ContainerID}, nil
}

// orDiscard hands back a file to write to, or a writer that drops everything where there is none.
func orDiscard(f *os.File) io.Writer {
	// Drop everything where no file is open
	if f == nil {
		return io.Discard
	}
	return f
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
