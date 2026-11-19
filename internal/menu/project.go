package menu

import (
	"strings"

	"github.com/hansehart/o3s/internal/project"
)

// create is the value the project view's create row answers with.
const create = "+create"

// projectView shows the projects the workspace lists.
func projectView(h Home) Question {
	t := Tab{Name: "Projects", About: "Repositories in the workspace, cloned into projects/<host>/<owner>/<name>.", Options: []Option{{Value: add, Label: "+ Add a project…"}, {Value: create, Label: "+ Create a project…"}}}
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
	case picked[0] == add:
		return ask(a, Question{Key: "repos", Kind: Input, Title: "Repositories", About: "owner/repo, host/owner/repo or a git URL. Several, separated by spaces.", Example: "hansehart/o3s  gitlab.com/group/app", Check: checkRepos})
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

// projectPlan builds the commands the project view's dialogs lead to.
func projectPlan(a Answers) []Command {
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
		return []Command{{Args: append([]string{"project", verb}, strings.Fields(a.get("repos"))...)}}
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
