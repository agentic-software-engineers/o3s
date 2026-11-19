package menu

// Values the init dialog answers with.
const (
	inVM      = "vm"
	inProcess = "process"
	on        = "on"
	off       = "off"
)

// initDialog asks how a new home isolates the cage, the same questions o3s init asks at a terminal, then the
// confirmation.
func initDialog(h Home, a Answers) (Question, bool) {
	// Ask the isolation first, the VM recommended
	if !a.has("isolation") {
		return Question{Key: "isolation", Kind: Select, Title: "Isolation", Numbered: true, Options: []Option{
			{Value: inVM, Label: "VM", Note: "the home runs in its own VM, AppArmor inside it (recommended)"},
			{Value: inProcess, Label: "Process", Note: "the cage shares this machine's kernel"},
		}}, false
	}
	// Ask about AppArmor on the shared kernel alone, on recommended
	if a.get("isolation") == inProcess && !a.has("apparmor") {
		return Question{Key: "apparmor", Kind: Select, Title: "AppArmor", Numbered: true, Options: []Option{
			{Value: on, Label: "On", Note: "limits what the cage may do on the shared kernel (recommended)"},
			{Value: off, Label: "Off", Note: "the cage keeps its other limits"},
		}}, false
	}
	where := "inside its own VM"
	// Name the shared kernel, and AppArmor's absence where the user turned it off
	if a.get("isolation") == inProcess {
		where = "on this machine's kernel"
		// Name the cage's run without AppArmor
		if a.get("apparmor") == off {
			where += ", without AppArmor"
		}
	}
	return ask(a, Question{Key: "apply", Kind: Confirm, Title: "Apply?", Summary: "Creates an o3s home in " + h.Dir + ", " + where, Default: []string{yes}})
}

// initPlan makes the home the way the user chose, handing the answers on so init asks nothing again: in the menu,
// which shows its steps as they come, and on the real terminal where init asks for sudo to load the cage's AppArmor
// profile on this machine.
func initPlan(a Answers) []Command {
	// Leave the folder as it was where the user declines
	if !confirmed(a) {
		return nil
	}
	// Make the home inside its VM, the recommended answer, in the menu
	if a.get("isolation") != inProcess {
		return []Command{{Args: []string{"init", "--yes"}}}
	}
	// Make the home on this machine without AppArmor in the menu, since it asks for no sudo
	if a.get("apparmor") == off {
		return []Command{{Args: []string{"init", "--dangerously-skip-vm", "--dangerously-skip-apparmor"}}}
	}
	return []Command{{Args: []string{"init", "--dangerously-skip-vm"}, Terminal: true}}
}
