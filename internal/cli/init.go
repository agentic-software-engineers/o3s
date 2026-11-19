package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/host"
	"github.com/hansehart/o3s/internal/paths"
	"github.com/hansehart/o3s/internal/release"
	"github.com/hansehart/o3s/internal/secrets"
	"github.com/hansehart/o3s/internal/vm"
)

// newInit builds the command that makes the working folder a home, and prepares the host for it on every start.
func newInit() *cobra.Command {
	var skipVM, skipAppArmor, guest, yes bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Make this folder an o3s home, or prepare the host for the home it is",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := workingHome()
			// Hand back a working folder that stays unknown
			if err != nil {
				return err
			}
			// Turn away AppArmor's opt-out for a home in its VM, where AppArmor always runs
			if skipAppArmor && !skipVM {
				return fmt.Errorf("%w: AppArmor always runs in the home's VM, pair --dangerously-skip-apparmor with --dangerously-skip-vm", ErrUsage)
			}
			_, vmErr := vm.Find(dir)
			// Ask a new home at a terminal how it isolates the cage, in the menu's own dialog, where no flag chose already
			if !guest && !skipVM && !yes && !home.Exists(dir) && vmErr != nil && isInteractive(cmd.InOrStdin(), cmd.OutOrStdout()) {
				return openMenu(cmd.Context(), "init")
			}
			// Make the home inside its VM, the default, where this folder holds no home of its own
			if !guest && !skipVM && !home.Exists(dir) {
				return initVM(cmd, dir)
			}
			secretsFile, err := secrets.Path()
			// Hand back a secrets file whose place stays unknown
			if err != nil {
				return err
			}
			// Turn away a home that would show the cage the host's secrets, CA or tools
			if f, ok := home.Holds(dir, filepath.Dir(secretsFile), paths.Config(), paths.State(), paths.Data()); ok {
				return fmt.Errorf("%s holds %s, which the cage would see, so o3s init needs a folder of its own, as in mkdir ~/o3s && cd ~/o3s", dir, f)
			}
			out := cmd.OutOrStdout()
			fresh := !home.Exists(dir)
			// Write a new home into a folder that holds none
			if fresh {
				written, err := home.Init(dir)
				// Hand back a folder that fails to become a home
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Wrote %s\n", written[0])
				fmt.Fprintf(out, "Wrote %s%c (%d files)\n", filepath.Join(dir, ".devcontainer"), filepath.Separator, len(written)-1)
				// Record the choice of a home without its VM, which every later start reads
				if skipVM {
					// Hand back a choice that stays unrecorded
					if err := recordProcess(dir, skipAppArmor); err != nil {
						return err
					}
				}
			}
			seeded, err := home.Seed(dir)
			// Hand back a home whose missing files stay unwritten
			if err != nil {
				return err
			}
			// Name every file the home lacked
			for _, path := range seeded {
				fmt.Fprintf(out, "Wrote %s\n", path)
			}
			// Hand back a host that fails to prepare
			if err := prepare(cmd, dir); err != nil {
				return err
			}
			// Point a new home at what comes next
			if fresh {
				nextSteps(out)
				return nil
			}
			changes, err := home.Refresh(dir)
			// Point a stack that differs from this binary at update
			if err == nil && len(changes) > 0 {
				fmt.Fprintln(out, "The stack differs from this o3s, run o3s update")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&skipVM, "dangerously-skip-vm", false, "run the stack on this machine's kernel, without the home's VM")
	c.Flags().BoolVar(&skipAppArmor, "dangerously-skip-apparmor", false, "run the cage without AppArmor, for a home without its VM on a host that lacks it")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "take the recommended answers, the home's VM among them, without asking")
	c.Flags().BoolVar(&guest, "guest", false, "make the home inside the VM it runs in")
	_ = c.Flags().MarkHidden("guest")
	return c
}

// initVM makes the home inside its VM, step by step, each named as it starts: it checks that this machine runs the VM,
// starts the VM, puts this o3s into it where it runs another, makes the home there, and marks it finished. A step that
// fails names its cause, and running init again goes on where it stopped.
func initVM(cmd *cobra.Command, dir string) error {
	current := version()
	folder, err := release.Folder(current)
	// Hand back a build without release files to install from
	if err != nil {
		return err
	}
	lima, err := vm.Manager()
	// Hand back a host without the VM manager
	if err != nil {
		return err
	}
	// Hand back a machine that lacks what runs the VM, before anything is written or downloads
	if err := lima.Check(); err != nil {
		return err
	}
	name, err := vm.Bind(dir)
	// Hand back a folder that fails to hold its VM's ID
	if err != nil {
		return err
	}
	m, err := managed(dir, name)
	// Hand back a host without the VM manager
	if err != nil {
		return err
	}
	ctx, out, errOut := cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	// Warn that the VM runs slowly without the machine's own acceleration
	if !m.lima.Accelerated() {
		warn(errOut, "this machine offers no KVM, so the home's VM runs slowly")
	}
	fmt.Fprintf(out, "Starting the VM %s, its first start downloads Ubuntu and takes a few minutes\n", name)
	// Hand back a VM that fails to come up, in the manager's words
	if err := m.lima.Start(ctx, name, errOut); err != nil {
		return vmFailure("start the VM "+name, err, vmLogPath(dir))
	}
	// Put this o3s into the VM where it runs another, and a dev build every time
	if !release.IsRelease(current) || !m.lima.Installed(ctx, name, current) {
		fmt.Fprintf(out, "Installing o3s %s in the VM\n", current)
		// Hand back o3s that fails to reach the VM
		if err := installInVM(ctx, m.lima, dir, name, folder, errOut); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "Making the home in the VM")
	// Hand back a home that fails to appear in the VM
	if err := m.lima.Init(ctx, name, cmd.InOrStdin(), out, errOut); err != nil {
		return vmFailure("make the home in the VM "+name, err, vmLogPath(dir))
	}
	// Hand back a home whose finish stays unrecorded
	if err := vm.MarkReady(dir); err != nil {
		return err
	}
	fmt.Fprintf(out, "The home runs in its VM %s, step in with: o3s cage shell --vm\n", name)
	return nil
}

// installInVM checks the release files of a folder on this host and installs o3s from them in a home's VM.
func installInVM(ctx context.Context, lima vm.Lima, dir, name, folder string, out io.Writer) error {
	staging, err := os.MkdirTemp("", "o3s")
	// Hand back a host without room for the release files
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	bundle := filepath.Join(staging, "o3s-release")
	// Hand back a folder for the release files that fails to appear
	if err := os.Mkdir(bundle, 0o755); err != nil { //nolint:gosec // the folder goes into the VM, which reads it as its user
		return err
	}
	// Hand back release files that fail to download or differ from their checksums
	if err := release.Bundle(ctx, folder, bundle); err != nil {
		return err
	}
	// Hand back an install that fails in the VM, in the manager's words
	if err := lima.Install(ctx, name, bundle, out); err != nil {
		return vmFailure("install o3s in the VM "+name, err, vmLogPath(dir))
	}
	return nil
}

// recordProcess records in a home's settings that its stack runs on this machine's kernel, and without AppArmor where
// the home chose so.
func recordProcess(dir string, skipAppArmor bool) error {
	c, err := config.Load(dir)
	// Hand back settings that fail to load
	if err != nil {
		return err
	}
	c.Isolation = config.Process
	// Record the cage's run without AppArmor
	if skipAppArmor {
		c.AppArmorProfile = config.Unconfined
	}
	return c.Save(dir)
}

// prepare readies the host for a home: the secrets file, the proxy's CA, the cage's confinement and the values compose reads.
func prepare(cmd *cobra.Command, dir string) error {
	out := cmd.OutOrStdout()
	path, err := secrets.Path()
	// Hand back a secrets file whose place stays unknown
	if err != nil {
		return err
	}
	// Hand back a secrets file that stays unwritten
	if _, err := home.SeedSecrets(path); err != nil {
		return err
	}
	caDir := paths.Proxy()
	made, err := host.CA(caDir)
	// Hand back a CA that fails to make
	if err != nil {
		return err
	}
	// Say whether the CA is new
	if made {
		fmt.Fprintf(out, "Made the proxy's CA in %s\n", caDir)
	} else {
		fmt.Fprintf(out, "The proxy's CA is ready in %s\n", caDir)
	}
	// Name a home that runs without its VM on every start
	if c, err := config.Load(dir); err == nil && c.Isolation == config.Process {
		warn(cmd.ErrOrStderr(), `the home runs without its VM, remove "isolation" from o3s.json to go back to the secure default`)
	}
	profile, err := appArmorProfile(cmd, dir)
	// Hand back a cage that would run without the confinement it asks for
	if err != nil {
		return err
	}
	// Load the cage's own profile where the cage runs with it
	if profile == cageProfile {
		note, err := host.Profile(cmd.Context(), filepath.Join(dir, ".devcontainer", "apparmor.conf"))
		// Hand back a profile that fails to read
		if err != nil {
			return err
		}
		// Say what the confinement step found or did
		if note != "" {
			fmt.Fprintln(out, note)
		}
	}
	// Hand back a home's state that stays unwritten
	if err := host.HomeState(dir); err != nil {
		return err
	}
	// Name every other home, since this release runs one home per machine
	for _, other := range host.OtherHomes(dir) {
		warn(cmd.ErrOrStderr(), "%s is already an o3s home, and o3s runs one home per machine for now", other)
	}
	// Hand back values compose reads that stay unwritten
	if err := host.Env(dir, profile); err != nil {
		return err
	}
	waiting(cmd.ErrOrStderr(), dir)
	return nil
}

// waiting warns about every secret a host carries that still waits for its value, with the command that gives it one.
func waiting(errOut io.Writer, dir string) {
	c, err := config.Load(dir)
	// Leave the names to the commands that read the settings, which report a broken file themselves
	if err != nil {
		return
	}
	_, store, err := loadSecrets()
	// Leave the names where the secrets file fails to read
	if err != nil {
		return
	}
	named := map[string]bool{}
	// Look at every host's secret
	for _, h := range c.Hosts {
		_, set := store.Value(h.Secret)
		// Name a secret once while it waits for its value
		if h.Secret != "" && !set && !named[h.Secret] {
			named[h.Secret] = true
			warn(errOut, "%s waits for its value: echo \"$TOKEN\" | o3s secret add %s", h.Secret, h.Secret)
		}
	}
}

// nextSteps points a new home at the commands that fill it and the one that opens its cage.
func nextSteps(out io.Writer) {
	fmt.Fprintf(out, "\nNext steps:\n")
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  o3s firewall add <host> --port 443\tallow a host\n")
	fmt.Fprintf(w, "  echo \"$TOKEN\" | o3s secret add <name>\tstore a secret\n")
	fmt.Fprintln(w, "  o3s cage ide\topen it in the cage")
	_ = w.Flush()
}

// cageProfile is the AppArmor profile the stack ships for the cage.
const cageProfile = "o3s-cage"

// appArmorProfile picks the cage's AppArmor profile: unconfined where the home opts out, its own where Docker confines with AppArmor, and none at all otherwise, since the cage would run less confined than it asks.
func appArmorProfile(cmd *cobra.Command, dir string) (string, error) {
	c, err := config.Load(dir)
	// Hand back a home whose settings fail to load
	if err != nil {
		return "", err
	}
	// Run unconfined where the home opts out, and say so
	if c.AppArmorProfile == config.Unconfined {
		warn(cmd.ErrOrStderr(), `the cage runs without AppArmor, remove "appArmorProfile" from o3s.json to go back to the secure default`)
		return config.Unconfined, nil
	}
	confines, err := host.DaemonAppArmor(cmd.Context())
	// Hand back a daemon whose security options stay unknown
	if err != nil {
		return "", err
	}
	// Refuse a cage Docker would start without its profile
	if !confines {
		return "", fmt.Errorf("the cage's profile %s needs AppArmor, but Docker reports it unavailable, so enable AppArmor on the daemon host or set \"appArmorProfile\": %q in %s", cageProfile, config.Unconfined, config.File)
	}
	return cageProfile, nil
}
