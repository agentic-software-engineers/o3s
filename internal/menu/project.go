package menu

import (
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/project"
	"github.com/hansehart/o3s/internal/words"
)

// create is the value the project view's create row answers with.
const create = "+create"

// projectView shows the projects the workspace lists.
func projectView(h Home) Question {
	t := Tab{Name: "Projects", About: "Repositories in the workspace, cloned into projects/<host>/<owner>/<name>.", Header: []string{"NAME", "HOST"}, Options: []Option{{Value: add, Label: "+ Add a project…"}, {Value: create, Label: "+ Create a project…"}}}
	// Show every project by its name with its host
	for _, p := range h.Projects {
		r, err := project.Parse(p)
		// Show a project o3s cannot read by its path alone
		if err != nil {
			t.Options = append(t.Options, Option{Value: p})
			continue
		}
		t.Options = append(t.Options, Option{Value: p, Label: r.FullName(), Columns: []string{r.Host}})
	}
	return Question{Key: "view", Kind: View, Title: "Project", Tabs: []Tab{t}}
}

// projectDialog asks what cloning, creating or removing projects needs, starting at the project view.
func projectDialog(h Home, a Answers) (Question, bool) {
	// Show the view first
	if !a.has("view") {
		return projectView(h), false
	}
	_, picked := view(a)
	// Follow what the user picked
	switch {
	case len(picked) == 0:
		return Question{}, true
	case picked[0] == add && !a.has("repos"):
		return Question{Key: "repos", Kind: Input, Title: "Repositories", About: "owner/repo, host/owner/repo or a git URL. Several, separated by spaces.", Example: "hansehart/o3s  gitlab.com/group/app", Check: checkRepos}, false
	case picked[0] == add:
		return openFirst(h, a)
	case picked[0] == create:
		return ask(a, Question{Key: "repos", Kind: Input, Title: "Repository", About: "owner/repo or host/owner/repo for the new, empty repository.", Example: "example/scratch", Check: checkRepos})
	}
	// Ask what to do with the project
	if !a.has("action") {
		return detail(picked[0], []Fact{{Label: "Path", Value: project.Dir + "/" + picked[0]}}, Option{Value: remove, Label: "Remove"}), false
	}
	// Ask for the name typed back before anything is deleted
	if a.get("action") == remove {
		name := fullName(picked[0])
		return ask(a, Question{Key: "typed", Kind: Input, Title: "Type " + name + " to confirm", About: "Deletes projects/" + picked[0] + " with every uncommitted change, and removes it from the workspace.", Check: checkName(name)})
	}
	return Question{}, true
}

// openFirst asks to open the firewall for the hosts the new projects clone from where it blocks them, and asks
// nothing where it lets every clone through.
func openFirst(h Home, a Answers) (Question, bool) {
	opens := opening(h, a, add)
	// Ask nothing where the firewall lets every clone through
	if len(opens) == 0 {
		return Question{}, true
	}
	var lines, hosts []string
	// Show every host the firewall opens, with the ports it keeps
	for _, o := range opens {
		sign := "+"
		// Mark a host the allowlist holds already
		if o.known {
			sign = "~"
		}
		lines, hosts = append(lines, sign+" "+o.host+":"+o.ports), append(hosts, o.host)
	}
	return ask(a, apply(lines, "Opens "+words.JoinAnd(hosts)+" so the clone reaches it", false))
}

// open is a host the firewall opens for a clone, with the ports it keeps open and whether the allowlist holds it.
type open struct {
	host, ports string
	known       bool
}

// opening lists the hosts the firewall blocks for the projects the user clones, each with the ports it already opens
// beside the one the clone needs, since a change of ports replaces them, and none for a new project, which clones
// nothing.
func opening(h Home, a Answers, picked string) []open {
	// Open nothing for a project the user creates
	if picked != add {
		return nil
	}
	c := config.Config{Hosts: h.Hosts}
	needs := map[string][]int{}
	var order []string
	// Note every host and port a clone needs that the firewall blocks
	for arg := range strings.FieldsSeq(a.get("repos")) {
		r, err := project.Parse(arg)
		// Pass over a repository the firewall lets through, or one o3s cannot read
		if err != nil || c.Allows(r.Host, r.Port) {
			continue
		}
		// Keep the hosts in the order the user named them
		if _, seen := needs[r.Host]; !seen {
			order = append(order, r.Host)
		}
		needs[r.Host] = append(needs[r.Host], r.Port)
	}
	out := make([]open, len(order))
	// Join every host's ports with the ones it opens already
	for i, host := range order {
		current, known := c.Find(host)
		ports := append(slices.Clone(current.Ports), needs[host]...)
		slices.Sort(ports)
		out[i] = open{host: host, ports: config.JoinPorts(slices.Compact(ports)), known: known}
	}
	return out
}

// projectPlan builds the commands the project view's dialogs lead to.
func projectPlan(h Home, a Answers) []Command {
	_, picked := view(a)
	// Go back to the view where the user picked nothing or stepped back
	if len(picked) == 0 || a.get("action") == back {
		return nil
	}
	// Clone or create the repositories the user typed
	if picked[0] == add || picked[0] == create {
		verb := "add"
		// Create a new repository where the user asked for one
		if picked[0] == create {
			verb = "create"
		}
		var cmds []Command
		// Open the firewall first for every host a clone needs and it blocks, where the user agreed
		for _, o := range opening(h, a, picked[0]) {
			// Leave everything as it was where the user declines
			if !confirmed(a) {
				return declined()
			}
			cmds = append(cmds, Command{Args: []string{"firewall", "add", o.host, "--port", o.ports, "--yes"}})
		}
		return append(cmds, Command{Args: append([]string{"project", verb}, strings.Fields(a.get("repos"))...)})
	}
	// Leave the project where the typed name differs
	if strings.TrimSpace(a.get("typed")) != fullName(picked[0]) {
		return declined()
	}
	return []Command{{Args: []string{"project", "rm", picked[0], "--yes"}}}
}

// fullName reads the owner/name of a project, or its path where o3s cannot read it.
func fullName(p string) string {
	r, err := project.Parse(p)
	// Take the path of a project o3s cannot read
	if err != nil {
		return p
	}
	return r.FullName()
}
