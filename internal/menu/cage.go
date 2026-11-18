package menu

// Values the cage view answers with.
const (
	ide   = "ide"
	shell = "shell"
)

// cageDialog shows the state of the stack and asks how to step into the cage.
func cageDialog(h Home, a Answers) (Question, bool) {
	return ask(a, Question{Key: "action", Kind: Select, Title: "Cage", Facts: h.Stack, Options: []Option{{Value: ide, Label: "Open in VS Code"}, {Value: shell, Label: "Open a shell"}}})
}

// cagePlan opens the cage the way the user chose, a shell on the real terminal.
func cagePlan(a Answers) []Command {
	action := a.get("action")
	return []Command{{Args: []string{"cage", action}, Terminal: action == shell}}
}
