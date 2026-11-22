package hostd

import (
	"context"
	"os"
	"os/exec"
)

// Spawn starts the binary's host process for the place's home on its own, apart from the terminal and the command
// that starts it, with its output in the place's output file.
func Spawn(ctx context.Context, exe string, p Place) error {
	out, err := os.Create(p.Crash())
	// Hand back an output file that fails to open
	if err != nil {
		return err
	}
	defer out.Close()
	cmd := exec.CommandContext(context.WithoutCancel(ctx), exe, "host", "serve", "--home", p.Home) //nolint:gosec // o3s starts its own binary
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = detached()
	// Hand back a process that fails to start
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
