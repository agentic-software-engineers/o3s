package menu

// Values the cage view answers with.
const (
	ide     = "ide"
	shell   = "shell"
	vmShell = "shell --vm"
	start   = "start"
	stop    = "stop"
)

// cageDialog shows the state of the VM or the stack, and asks how to step into the cage or take it through its life,
// then the confirmation of its removal.
func cageDialog(h Home, a Answers) (Question, bool) {
	// Ask what to do with the cage first
	if !a.has("action") {
		actions := []Option{{Value: ide, Label: "Open in VS Code"}, {Value: shell, Label: "Open a shell"}}
		// Offer a shell in the VM of a home that lives in one, whose cage the VM's own o3s opens
		if h.InVM {
			actions = []Option{{Value: vmShell, Label: "Open a shell in the VM"}}
		}
		actions = append(actions, Option{Value: start, Label: "Start"}, Option{Value: stop, Label: "Stop"}, Option{Value: remove, Label: "Remove"})
		return Question{Key: "action", Kind: Select, Title: "Cage", Facts: h.Stack, Options: actions}, false
	}
	// Confirm a removal, naming what goes
	if a.get("action") == remove {
		line := "- the stack's containers, the home's files and the projects volume staying"
		// Name the VM and its disk, which hold the home and its projects
		if h.InVM {
			line = "- the VM and its disk, with the home and the projects inside"
		}
		return ask(a, apply([]string{line}, "Removes the cage", true))
	}
	return Question{}, true
}

// cagePlan runs what the user chose: a shell on the real terminal, the cage's start, stop or removal, or VS Code.
func cagePlan(a Answers) []Command {
	// Run the shell in the VM on the real terminal
	switch action := a.get("action"); action {
	case vmShell:
		return []Command{{Args: []string{"cage", "shell", "--vm"}, Terminal: true}}
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
