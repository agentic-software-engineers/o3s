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
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/ide"
	"github.com/hansehart/o3s/internal/paths"
	"github.com/hansehart/o3s/internal/rebuild"
	"github.com/hansehart/o3s/internal/restart"
	"github.com/hansehart/o3s/internal/vm"
	"github.com/hansehart/o3s/internal/words"
)

// newCage builds the commands that show the home's cage, start, stop and remove it, and step into it.
func newCage() *cobra.Command {
	c := group("cage", "Show, start, stop or remove the home's cage, or open it in VS Code or a shell", showCage)
	c.AddCommand(newCageStart(), newCageStop(), newCageRm(), newCageIDE(), newCageShell())
	return c
}

// showCage writes where the home's VM stands where it lives in one, and whether each part of its stack runs otherwise.
func showCage(cmd *cobra.Command) error {
	// Show the VM of a home that lives in one
	if m, inVM, err := homeMachine(); inVM {
		// Hand back a VM out of reach
		if err != nil {
			return err
		}
		return m.show(cmd)
	}
	dir, err := cageHome()
	// Hand back a folder that lacks a home
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PART\tSTATE")
	// Write where each part of the stack stands
	for _, p := range homeStack(cmd.Context(), dir) {
		fmt.Fprintf(w, "%s\t%s\n", p.name, p.state)
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
			// Hand back a folder that lacks a home
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

// newCageStart builds the command that starts the home's VM where it lives in one, and raises its stack and cage otherwise.
func newCageStart() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the home's VM, or its stack and cage on this machine",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Start the VM of a home that lives in one
			if m, inVM, err := homeMachine(); inVM {
				// Hand back a VM out of reach
				if err != nil {
					return err
				}
				return m.start(cmd)
			}
			dir, err := cageHome()
			// Hand back a folder that lacks a home
			if err != nil {
				return err
			}
			// Leave a cage that runs as it is
			if homeStack(cmd.Context(), dir).caged() {
				fmt.Fprintln(cmd.OutOrStdout(), "The cage runs already")
				return nil
			}
			// Hand back a cage that fails to come up
			if _, err := raise(cmd, dir); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "The cage runs")
			return nil
		},
	}
}

// newCageStop builds the command that stops the home's VM where it lives in one, and its stack and cage otherwise.
func newCageStop() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the home's VM, or its stack and cage on this machine",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Stop the VM of a home that lives in one
			if m, inVM, err := homeMachine(); inVM {
				// Hand back a VM out of reach
				if err != nil {
					return err
				}
				return m.stop(cmd)
			}
			dir, err := cageHome()
			// Hand back a folder that lacks a home
			if err != nil {
				return err
			}
			// Leave a stopped stack as it is
			if !homeStack(cmd.Context(), dir).up() {
				fmt.Fprintln(cmd.OutOrStdout(), "The cage is stopped already")
				return nil
			}
			n, err := restart.Stop(cmd.Context(), dir)
			// Hand back a stack that keeps running
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped %s\n", words.Count(n, "container", "containers"))
			return nil
		},
	}
}

// newCageRm builds the command that removes the home's VM with its disk, or its stack's containers, once the user agrees.
func newCageRm() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm",
		Short: "Remove the home's VM with its disk, or its stack's containers on this machine",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, inVM, err := homeMachine()
			// Hand back a VM out of reach
			if inVM && err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			// Name what goes with the VM, or with the stack on this machine
			if inVM {
				fmt.Fprintf(out, "  - the VM %s and its disk, with the home and the projects inside\n", m.name)
			} else {
				// Hand back a folder that lacks a home
				if _, err := cageHome(); err != nil {
					return err
				}
				fmt.Fprintln(out, "  - the stack's containers")
				fmt.Fprintln(out, "  = the home's files and the projects volume stay")
			}
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes, "remove")
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the cage as it was when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the cage as it was")
				return nil
			}
			// Delete the VM of a home that lives in one
			if inVM {
				return m.remove(cmd)
			}
			dir, err := workingHome()
			// Hand back a working folder that stays unknown
			if err != nil {
				return err
			}
			n, err := restart.Remove(cmd.Context(), dir)
			// Hand back containers that stay
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Removed %s\n", words.Count(n, "container", "containers"))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// newCageShell builds the command that raises the home's cage and opens a shell inside it, or opens a shell or runs a
// command in the home's VM.
func newCageShell() *cobra.Command {
	var inVM bool
	cmd := &cobra.Command{
		Use:   "shell [--vm [command] [args...]]",
		Short: "Open a shell inside the home's cage, or with --vm a shell or a command in the home's VM",
		Args:  usage(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Open a shell or run a command in the home's VM
			if inVM {
				m, found, err := homeMachine()
				// Point a home that lacks a VM at the command that makes one
				if !found {
					return vm.ErrNoVM
				}
				// Hand back a VM out of reach
				if err != nil {
					return err
				}
				return m.shell(cmd, args)
			}
			// Turn away a command, which runs in the VM alone
			if len(args) > 0 {
				return fmt.Errorf("%w: a command runs in the home's VM, as in o3s cage shell --vm -- %s", ErrUsage, strings.Join(args, " "))
			}
			dir, err := cageHome()
			// Hand back a folder that lacks a home
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
	cmd.Flags().BoolVar(&inVM, "vm", false, "open the shell in the home's VM, or run the command that follows there")
	return cmd
}

// cageHome finds the home a cage command works on, and turns away a folder that lacks one.
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
	// Hand back a result that lacks a container
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.ContainerID == "" {
		return cage{}, fmt.Errorf("raise the cage: devcontainer up named no container in %q", out.String())
	}
	return cage{cli: cli, dir: dir, id: r.ContainerID}, nil
}

// orDiscard hands back a file to write to, or a writer that drops everything where the file is missing.
func orDiscard(f *os.File) io.Writer {
	// Drop everything where the file is missing
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
