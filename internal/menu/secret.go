package menu

import (
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/words"
)

// Values a secret's detail and its binding answer with.
const (
	setValue = "set"
	notNow   = "+later"
)

// secretView shows the secrets the proxy injects, each with whether it holds a value and the hosts that carry it.
func secretView(h Home) Question {
	t := Tab{Name: "Secrets", About: "Values the proxy injects. The cage never sees them.", Header: SecretHeader, Options: []Option{{Value: add, Label: "+ Add a secret…"}}}
	// Show every secret with its state and its hosts
	for _, s := range h.Secrets {
		t.Options = append(t.Options, Option{Value: s.Name, Columns: SecretRow(s.Set, carrying(h, s.Name))})
	}
	return Question{Key: "view", Kind: View, Title: "Secret", Tabs: []Tab{t}}
}

// secretDialog asks what adding, setting or removing a secret needs, starting at the secret view.
func secretDialog(h Home, a Answers) (Question, bool) {
	// Show the view first
	if !a.has("view") {
		return secretView(h), false
	}
	_, picked := view(a)
	// End an empty pick
	if len(picked) == 0 {
		return Question{}, true
	}
	// Ask the new secret's name, its value, then whether to bind it to a host
	if picked[0] == add {
		// Ask the name first
		if !a.has("name") {
			return Question{Key: "name", Kind: Input, Title: "Name", About: "An uppercase env name, the way the proxy reads it.", Example: "GH_TOKEN", Check: checkSecretName}, false
		}
		// Ask the value
		if !a.has("value") {
			return Question{Key: "value", Kind: Hidden, Title: "Value for " + strings.TrimSpace(a.get("name"))}, false
		}
		return bindDialog(h, a)
	}
	name := picked[0]
	// Show the secret's facts and what the user can do with it
	if !a.has("action") {
		actions := []Option{{Value: setValue, Label: "Set a value"}}
		// Offer the removal of a value the secret holds
		if slices.ContainsFunc(h.Secrets, func(s Secret) bool { return s.Name == name && s.Set }) {
			actions = append(actions, Option{Value: remove, Label: "Remove"})
		}
		facts := []Fact{{Label: "Value", Value: SecretState(!waits(h, name))}, {Label: "Hosts", Value: words.Or(strings.Join(carrying(h, name), ", "), "none")}}
		return detail(name, facts, actions...), false
	}
	// Follow what the user chose
	switch a.get("action") {
	case setValue:
		return ask(a, Question{Key: "value", Kind: Hidden, Title: "Value for " + name})
	case remove:
		line := "- " + name
		// Name the hosts that stop receiving the value
		if hosts := carrying(h, name); len(hosts) > 0 {
			line += ", " + words.JoinAnd(hosts) + " stop receiving it"
		}
		return ask(a, apply([]string{line}, "Removes the value of "+name, true))
	}
	return Question{}, true
}

// bindDialog asks whether to bind a new secret to a host the proxy then injects it for, and which, where the home allows
// a host.
func bindDialog(h Home, a Answers) (Question, bool) {
	name := strings.TrimSpace(a.get("name"))
	// Leave a secret some host carries already as it is, and offer no binding where the home allows no host
	if len(carrying(h, name)) > 0 || len(h.Hosts) == 0 {
		return Question{}, true
	}
	// Ask whether to bind the secret now
	if !a.has("bind") {
		return Question{Key: "bind", Kind: Select, Title: "Bind " + name + " to a host?", Numbered: true, Options: []Option{{Value: yes, Label: "Yes, choose a host…"}, {Value: notNow, Label: "Not now"}}}, false
	}
	// Ask which host where the user binds it now
	if a.get("bind") == yes && !a.has("host") {
		q := Question{Key: "host", Kind: Select, Title: "Host"}
		// Offer every allowed host
		for _, host := range h.Hosts {
			q.Options = append(q.Options, Option{Value: host.Name, Note: words.Or(host.Secret, "")})
		}
		return q, false
	}
	return Question{}, true
}

// secretPlan builds the commands the secret view's dialogs lead to.
func secretPlan(a Answers) []Command {
	_, picked := view(a)
	// Go back to the view where the user picked nothing or stepped back
	if len(picked) == 0 || a.get("action") == back {
		return nil
	}
	value := a.get("value")
	// Store the new secret, and bind it to the host the user chose
	if picked[0] == add {
		name := strings.TrimSpace(a.get("name"))
		cmds := []Command{{Args: []string{"secret", "add", name, "--no-bind-hint"}, Stdin: &value}}
		// Bind the secret to the host the user chose
		if a.get("bind") == yes && a.has("host") {
			cmds = append(cmds, Command{Args: []string{"firewall", "add", a.get("host"), "--secret", name, "--yes"}})
		}
		return cmds
	}
	// Store the value the user typed
	if a.get("action") == setValue {
		return []Command{{Args: []string{"secret", "add", picked[0], "--no-bind-hint"}, Stdin: &value}}
	}
	// Leave the value as it was where the user declines
	if !confirmed(a) {
		return declined()
	}
	return []Command{{Args: []string{"secret", "rm", picked[0], "--yes"}}}
}

// carrying lists the hosts that carry a secret.
func carrying(h Home, name string) []string {
	var out []string
	// Take every host that carries the secret
	for _, host := range h.Hosts {
		// Take a host whose secret is the one named
		if host.Secret == name {
			out = append(out, host.Name)
		}
	}
	return out
}
