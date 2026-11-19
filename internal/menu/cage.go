package menu

import "github.com/hansehart/o3s/internal/vm"

// Values the cage view answers with.
const (
	ide     = "ide"
	shell   = "shell"
	vmShell = "shell --vm"
	start   = "start"
	stop    = "stop"
	finish  = "finish"
)

// cageDialog shows where the VM or the stack stands and offers what applies there, then the confirmation of a
// removal.
func cageDialog(h Home, a Answers) (Question, bool) {
	// Ask what to do with the cage first
	if !a.has("action") {
		return Question{Key: "action", Kind: Select, Title: "Cage", Facts: h.Stack, Options: cageActions(h)}, false
	}
	// Confirm a removal, naming what goes
	if a.get("action") == remove {
		line := "- the stack's containers, the home's files and the projects volume staying"
		// Name the VM and its disk, which hold the home and its projects
		if h.VM != "" {
			line = "- the VM and its disk, with the home and the projects inside"
		}
		return ask(a, apply([]string{line}, "Removes the cage", true))
	}
	return Question{}, true
}

// cageActions offers what applies where the cage stands: a VM init has yet to finish is finished, a stopped one
// started, a running one stepped into or stopped, and a home on this machine opened, which raises it, started or
// stopped. Every cage can go.
func cageActions(h Home) []Option {
	var out []Option
	// Offer what the VM's state allows, or what the stack's does
	switch {
	case h.VM == vm.Absent || h.VM == vm.Unfinished:
		out = []Option{{Value: finish, Label: "Finish making the VM"}}
	case h.VM == vm.Stopped:
		out = []Option{{Value: start, Label: "Start"}}
	case h.VM == vm.Running:
		out = []Option{{Value: vmShell, Label: "Open a shell in the VM"}, {Value: stop, Label: "Stop"}}
	case stackState(h) == "running":
		out = []Option{{Value: ide, Label: "Open in VS Code"}, {Value: shell, Label: "Open a shell"}, {Value: stop, Label: "Stop"}}
	default:
		out = []Option{{Value: ide, Label: "Open in VS Code"}, {Value: shell, Label: "Open a shell"}, {Value: start, Label: "Start"}}
	}
	return append(out, Option{Value: remove, Label: "Remove"})
}

// cagePlan runs what the user chose: a shell on the real terminal, init for a VM it has yet to finish, the cage's
// start, stop or removal, or VS Code.
func cagePlan(a Answers) []Command {
	// Run what the user chose
	switch action := a.get("action"); action {
	case vmShell:
		return []Command{{Args: []string{"cage", "shell", "--vm"}, Terminal: true}}
	case finish:
		return []Command{{Args: []string{"init", "--yes"}}}
	case remove:
		// Leave the cage as it was where the user declines
		if !confirmed(a) {
			return declined()
		}
		return []Command{{Args: []string{"cage", "rm", "--yes"}}}
	default:
		return []Command{{Args: []string{"cage", action}, Terminal: action == shell}}
	}
}
