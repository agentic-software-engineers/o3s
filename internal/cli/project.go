package cli

import (
	"bufio"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/project"
	"github.com/hansehart/o3s/internal/workspace"
)

// noAgent is the exit code the agent check gives where it reaches no agent.
const noAgent = 2

// newProject builds the commands that show, clone, create and remove the projects in the home's workspace.
func newProject() *cobra.Command {
	p := group("project", "Show, clone, create or remove the projects in the workspace", showProjects)
	p.AddCommand(newProjectAdd(), newProjectCreate(), newProjectRm())
	return p
}

// newProjectAdd builds the command that clones repositories into the projects volume and lists them in the workspace.
func newProjectAdd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <repository>...",
		Short: "Clone repositories into ~/projects/<host>/<owner>/<name> and add them to the workspace",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := parseRepos(args)
			// Hand back a repository o3s cannot read
			if err != nil {
				return err
			}
			dir, err := cageHome()
			// Hand back a folder that holds no home
			if err != nil {
				return err
			}
			c, err := config.Load(dir)
			// Hand back a home whose settings fail to load
			if err != nil {
				return err
			}
			ws, err := workspace.Load(home.Workspace(dir))
			// Hand back a workspace that fails to load
			if err != nil {
				return err
			}
			// Check every repository before the cage comes up
			for _, repo := range repos {
				// Turn away a clone the gateway would block, with the command that opens the way
				if !allows(c, repo.Host, repo.Port) {
					return fmt.Errorf("the firewall blocks %s:%d, allow it with: o3s firewall add %s --port %d", repo.Host, repo.Port, repo.Host, repo.Port)
				}
				// Turn away a project whose folder the workspace lists already
				if listed(ws, repo.Path()) {
					return fmt.Errorf("%s is %w", repo.Path(), workspace.ErrListed)
				}
			}
			cg, err := raise(cmd, dir)
			// Hand back a cage that fails to come up
			if err != nil {
				return err
			}
			// Clone every repository in turn
			for _, repo := range repos {
				// Hand back a clone that fails, the ones before it kept
				if err := cloneInto(cmd, cg, dir, ws, repo); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// cloneInto clones a repository in the cage where it can sign, leaves the clone to the user otherwise, and lists it
// in the workspace.
func cloneInto(cmd *cobra.Command, cg cage, dir string, ws *workspace.Workspace, repo project.Repository) error {
	cloned := true
	// Look for the SSH agent a clone over SSH signs with
	if repo.SSH {
		_, code, err := cg.run(cmd, "ssh-add", "-l")
		// Hand back an agent check that fails to run
		if err != nil {
			return err
		}
		cloned = code != noAgent
	}
	// Clone where the cage can sign, and leave the clone to the user otherwise
	if cloned {
		_, code, err := cg.run(cmd, "git", "clone", repo.URL, repo.Path())
		// Hand back a clone that fails to run
		if err != nil {
			return err
		}
		// Hand back a clone git turned down, its reason on stderr already
		if code != 0 {
			return fmt.Errorf("git clone %s exited with %d", repo.URL, code)
		}
	} else {
		warn(cmd.ErrOrStderr(), "no SSH agent in the cage, clone %s yourself:\n  o3s cage shell\n  git clone %s %s", repo.FullName(), repo.URL, repo.Path())
	}
	// Hand back a workspace that stays unwritten
	if err := addFolder(dir, ws, repo); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added %s\n", repo.FullName())
	return nil
}

// parseRepos reads every repository a command line names.
func parseRepos(args []string) ([]project.Repository, error) {
	repos := make([]project.Repository, 0, len(args))
	// Read every repository in the order given
	for _, arg := range args {
		repo, err := project.Parse(arg)
		// Hand back a repository o3s cannot read
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUsage, err)
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// newProjectCreate builds the command that starts an empty repository in the projects volume and lists it in the workspace.
func newProjectCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create <repository>",
		Short: "Start an empty repository in ~/projects/<host>/<owner>/<name> and add it to the workspace",
		Args:  usage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := project.Parse(args[0])
			// Hand back a repository o3s cannot read
			if err != nil {
				return fmt.Errorf("%w: %w", ErrUsage, err)
			}
			dir, err := cageHome()
			// Hand back a folder that holds no home
			if err != nil {
				return err
			}
			ws, err := workspace.Load(home.Workspace(dir))
			// Hand back a workspace that fails to load
			if err != nil {
				return err
			}
			path := repo.Path()
			// Turn away a project whose folder the workspace lists already
			if listed(ws, path) {
				return fmt.Errorf("%s is %w", path, workspace.ErrListed)
			}
			cg, err := raise(cmd, dir)
			// Hand back a cage that fails to come up
			if err != nil {
				return err
			}
			held, _, err := cg.run(cmd, "ls", "-A", path)
			// Hand back a look into the folder that fails to run
			if err != nil {
				return err
			}
			// Turn away a folder that holds files, so nothing there is overwritten
			if strings.TrimSpace(held) != "" {
				return fmt.Errorf("%s holds files already, add it with o3s project add or pick another name", path)
			}
			_, code, err := cg.run(cmd, "git", "init", "--quiet", path)
			// Hand back a repository that fails to start
			if err != nil {
				return err
			}
			// Hand back a repository git turned down, its reason on stderr already
			if code != 0 {
				return fmt.Errorf("git init %s exited with %d", path, code)
			}
			// Hand back a workspace that stays unwritten
			if err := addFolder(dir, ws, repo); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", repo.FullName())
			return nil
		},
	}
}

// newProjectRm builds the command that deletes projects' folders and takes them off the workspace, once the user
// types each name.
func newProjectRm() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <repository>...",
		Short: "Delete projects' folders and remove them from the workspace, once you type each owner/name",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := parseRepos(args)
			// Hand back a repository o3s cannot read
			if err != nil {
				return err
			}
			dir, err := cageHome()
			// Hand back a folder that holds no home
			if err != nil {
				return err
			}
			ws, err := workspace.Load(home.Workspace(dir))
			// Hand back a workspace that fails to load
			if err != nil {
				return err
			}
			// Turn away a project the workspace lacks, before anything is deleted
			for _, repo := range repos {
				// Name the project the workspace lacks
				if !listed(ws, repo.Path()) {
					return fmt.Errorf("unknown project %s, o3s project lists them", repo.FullName())
				}
			}
			// Turn away a run where nobody can type the names
			if !yes && !isTerminal(cmd.InOrStdin()) {
				return fmt.Errorf("%w: pass --yes to delete without a terminal", ErrUsage)
			}
			cg, err := raise(cmd, dir)
			// Hand back a cage that fails to come up
			if err != nil {
				return err
			}
			// Delete every project once its name is typed
			for _, repo := range repos {
				// Ask for owner/name before anything is deleted
				if !yes {
					// Hand back a deletion the user did not confirm
					if err := confirmName(cmd, repo.FullName()); err != nil {
						return err
					}
				}
				_, code, err := cg.run(cmd, "rm", "-rf", repo.Path())
				// Hand back a deletion that fails to run
				if err != nil {
					return err
				}
				// Hand back a folder that stays, its reason on stderr already
				if code != 0 {
					return fmt.Errorf("rm %s exited with %d", repo.Path(), code)
				}
				ws.Remove(repo.Path())
				// Hand back a workspace that stays unwritten
				if err := ws.Save(home.Workspace(dir)); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", repo.FullName())
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "delete without typing the names")
	return cmd
}

// showProjects writes the projects the workspace lists.
func showProjects(cmd *cobra.Command) error {
	dir, err := cageHome()
	// Hand back a folder that holds no home
	if err != nil {
		return err
	}
	ws, err := workspace.Load(home.Workspace(dir))
	// Hand back a workspace that fails to load
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATH")
	// Write a row per project, in the order the workspace holds them
	for _, r := range projects(ws) {
		fmt.Fprintf(w, "%s\t%s\n", r.FullName(), r.Path())
	}
	return w.Flush()
}

// projects lists the repositories the workspace's folders hold in the projects volume.
func projects(ws *workspace.Workspace) []project.Repository {
	var out []project.Repository
	// Take every folder that holds a project
	for _, f := range ws.Folders() {
		// Take a folder laid out as host/owner/name in the volume
		if r, ok := project.At(f.Path); ok {
			out = append(out, r)
		}
	}
	return out
}

// listed reports whether the workspace lists a folder.
func listed(ws *workspace.Workspace, path string) bool {
	return slices.ContainsFunc(ws.Folders(), func(f workspace.Folder) bool { return f.Path == path })
}

// addFolder lists a project's folder in the workspace under its owner/name and saves it.
func addFolder(dir string, ws *workspace.Workspace, repo project.Repository) error {
	// Hand back a folder the workspace turns away
	if err := ws.Add(workspace.Folder{Name: repo.FullName(), Path: repo.Path()}); err != nil {
		return err
	}
	return ws.Save(home.Workspace(dir))
}

// allows reports whether the home's allowlist lets the cage reach a host on a port, by its name or a wildcard over it.
func allows(c config.Config, host string, port int) bool {
	// Look at every host the allowlist holds
	for _, h := range c.Hosts {
		suffix, wildcard := strings.CutPrefix(h.Name, "*")
		// Take the host by its name, or by a wildcard that covers it
		if (h.Name == host || wildcard && strings.HasSuffix(host, suffix)) && slices.Contains(h.Ports, port) {
			return true
		}
	}
	return false
}

// errNotConfirmed marks a deletion the user did not confirm.
var errNotConfirmed = errors.New("the name does not match, nothing deleted")

// confirmName asks the user to type a name before a deletion, so nothing is deleted by a stray key.
func confirmName(cmd *cobra.Command, want string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Type %q to confirm deletion: ", want)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	// Read a failed read that typed nothing as a refusal
	if err != nil && line == "" {
		return errNotConfirmed
	}
	// Turn away a name that differs
	if strings.TrimSpace(line) != want {
		return errNotConfirmed
	}
	return nil
}
