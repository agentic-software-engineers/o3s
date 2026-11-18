package menu

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hansehart/o3s/internal/config"
)

// Values the firewall's view and its host's detail answer with.
const (
	prune        = "+prune"
	changePorts  = "ports"
	changeEnd    = "end"
	changeSecret = "secret"
	remove       = "remove"
)

// firewallView shows the hosts the cage reaches and the names the firewall refused.
func firewallView(h Home) Question {
	allowed := Tab{Name: "Allowed", About: "The cage reaches only these hosts.", Options: []Option{{Value: add, Label: "+ Allow a host…"}}}
	expired := false
	// Show every host with its ports, its traffic, when it closes, its secret and who needs it
	for _, host := range h.Hosts {
		t := h.Traffic[host.Name]
		allowed.Options = append(allowed.Options, Option{Value: host.Name, Columns: []string{joinInts(host.Ports), orDash(t.Data), orDash(t.Seen), closes(host, h.Now), host.Secret, strings.Join(host.By, ", ")}})
		expired = expired || host.Expired(h.Now)
	}
	// Offer to prune where a host has expired
	if expired {
		allowed.Keys = []Key{{Key: "p", Help: "prune expired", Value: prune}}
	}
	blocked := Tab{Name: "Blocked", About: "Names the cage asked for and the firewall refused, last 30 days.", Multi: true, Options: h.Blocked}
	blocked.Empty = "Nothing blocked yet. Names the cage asks for and the firewall refuses show here."
	// Name where the blocked names come from while the stack is down
	if !h.Running {
		blocked.Empty = "Blocked names show while the stack runs. Open the cage to start it."
	}
	return Question{Key: "view", Kind: View, Title: "Firewall", Tabs: []Tab{allowed, blocked}}
}

// firewallDialog asks what allowing, changing, removing or pruning hosts needs, starting at the firewall's view.
func firewallDialog(h Home, a Answers) (Question, bool) {
	// Show the view first
	if !a.has("view") {
		return firewallView(h), false
	}
	tab, picked := view(a)
	// Follow what the user picked
	switch {
	case len(picked) == 0:
		return Question{}, true
	case tab == "Blocked":
		return allowDialog(h, a, picked)
	case picked[0] == add:
		// Ask for the hosts first
		if !a.has("hosts") {
			return Question{Key: "hosts", Kind: Input, Title: "Hosts", About: "A name, a wildcard, an IPv4 address or an IPv4 range. Several, separated by spaces.", Example: "example.com  *.example.com  192.0.2.1  198.51.100.0/24", Check: checkHosts}, false
		}
		return allowDialog(h, a, strings.Fields(a.get("hosts")))
	case picked[0] == prune:
		return ask(a, apply(pruneLines(h), "Removes "+count(len(expiredHosts(h)), "expired host", "expired hosts"), true))
	}
	return hostDialog(h, a, picked[0])
}

// allowDialog asks the ports, the time, the secret and its sites of the hosts to allow, then the confirmation.
func allowDialog(h Home, a Answers, hosts []string) (Question, bool) {
	// Ask the ports, the time and the secret in turn
	for _, step := range []func(Home, Answers) (Question, bool){portsQuestion, forQuestion(true), secretQuestion(true)} {
		// Ask a question not yet answered
		if q, open := step(h, a); open {
			return q, false
		}
	}
	return ask(a, apply(allowLines(h, a, hosts), allowSummary(a, hosts), false))
}

// hostDialog asks what to do with an allowed host, then what the change needs.
func hostDialog(h Home, a Answers, name string) (Question, bool) {
	host, _ := (&config.Config{Hosts: h.Hosts}).Find(name)
	// Show the host's facts and what the user can do with it
	if !a.has("action") {
		actions := []Option{{Value: changePorts, Label: "Change ports"}, {Value: changeEnd, Label: "Set an end time"}, {Value: changeSecret, Label: "Change secret"}}
		// Offer the removal of a host the user added
		if slices.Contains(host.By, "user") {
			actions = append(actions, Option{Value: remove, Label: "Remove"})
		}
		return detail(name, hostFacts(h, host), actions...), false
	}
	var steps []func(Home, Answers) (Question, bool)
	// Ask what the chosen change needs
	switch a.get("action") {
	case changePorts:
		steps = []func(Home, Answers) (Question, bool){portsQuestion}
	case changeEnd:
		steps = []func(Home, Answers) (Question, bool){forQuestion(false)}
	case changeSecret:
		steps = []func(Home, Answers) (Question, bool){secretQuestion(false)}
	case remove:
		return ask(a, apply(removeLines(h, name), "Removes "+name, true))
	default:
		return Question{}, true
	}
	// Ask every step the change needs
	for _, step := range steps {
		// Ask a step not yet answered
		if q, open := step(h, a); open {
			return q, false
		}
	}
	return ask(a, apply(allowLines(h, a, []string{name}), "Changes "+name, false))
}

// portsQuestion asks the ports, offering the usual ones numbered and a way to type others, and reports whether it asks.
func portsQuestion(_ Home, a Answers) (Question, bool) {
	// Ask the ports first
	if !a.has("ports") {
		return Question{Key: "ports", Kind: Select, Title: "Ports", Numbered: true, Options: []Option{
			{Value: strconv.Itoa(443), Label: "443", Note: "HTTPS (recommended)"},
			{Value: "22,443", Label: "22, 443", Note: "SSH and HTTPS"},
			{Value: typed, Label: "Type ports…"},
		}}, true
	}
	// Ask for the ports the user types
	if a.get("ports") == typed && !a.has("portsTyped") {
		return Question{Key: "portsTyped", Kind: Input, Title: "Ports", About: "Ports from 1 to 65535, separated by commas.", Example: "22, 443, 8443", Check: checkPorts}, true
	}
	return Question{}, false
}

// forQuestion asks how long hosts stay open, offering the usual times numbered and a way to type another, and for good
// where the hosts may stay open, and reports whether it asks.
func forQuestion(forGood bool) func(Home, Answers) (Question, bool) {
	return func(_ Home, a Answers) (Question, bool) {
		// Ask how long first
		if !a.has("for") {
			q := Question{Key: "for", Kind: Select, Title: "For how long", Numbered: true}
			// Offer to keep the hosts open for good where they may
			if forGood {
				q.Options = append(q.Options, Option{Value: always, Label: "For good"})
			}
			q.Options = append(q.Options, Option{Value: "1h", Label: "1 hour"}, Option{Value: "8h", Label: "8 hours"}, Option{Value: "24h", Label: "1 day"}, Option{Value: typed, Label: "Type a duration…", Note: "e.g. 90m, 2h30m"})
			return q, true
		}
		// Ask for the time the user types
		if a.get("for") == typed && !a.has("forTyped") {
			return Question{Key: "forTyped", Kind: Input, Title: "For how long", About: "Seconds alone, or a duration.", Example: "90s  20m  1h  1h30m", Check: checkFor}, true
		}
		return Question{}, false
	}
}

// secretQuestion asks the secret the proxy injects and where it goes, offering none where hosts may go without one, and
// reports whether it asks.
func secretQuestion(withNone bool) func(Home, Answers) (Question, bool) {
	return func(h Home, a Answers) (Question, bool) {
		// Ask the secret first
		if !a.has("secret") {
			q := Question{Key: "secret", Kind: Select, Title: "Secret the proxy injects", Numbered: true}
			// Offer to go without a secret where hosts may
			if withNone {
				q.Options = append(q.Options, Option{Value: none, Label: "None"})
			}
			// Offer every secret the home knows
			for _, s := range h.Secrets {
				q.Options = append(q.Options, Option{Value: s.Name, Note: secretState(s)})
			}
			q.Options = append(q.Options, Option{Value: fresh, Label: "New secret…"})
			return q, true
		}
		// Ask for the name of a new secret
		if a.get("secret") == fresh && !a.has("secretName") {
			return Question{Key: "secretName", Kind: Input, Title: "Secret name", About: "An uppercase env name, the way the proxy reads it.", Example: "GH_TOKEN", Check: checkSecretName}, true
		}
		// Ask where the secret goes
		if chosenSecret(a) != "" && !a.has("inject") {
			return Question{Key: "inject", Kind: Multi, Title: "Where the secret goes", Options: []Option{{Value: "header"}, {Value: "query"}, {Value: "body"}}, Default: []string{"header"}}, true
		}
		return Question{}, false
	}
}

// firewallPlan builds the commands the firewall's dialogs lead to.
func firewallPlan(_ Home, a Answers) []Command {
	tab, picked := view(a)
	// Go back to the view where the user picked nothing to change
	if len(picked) == 0 || a.get("action") == back {
		return nil
	}
	// Leave everything as it was where the user declines
	if !confirmed(a) {
		return declined()
	}
	// Build the command of what the user picked
	switch {
	case tab == "Blocked":
		return []Command{{Args: allowArgs(a, picked)}}
	case picked[0] == add:
		return []Command{{Args: allowArgs(a, strings.Fields(a.get("hosts")))}}
	case picked[0] == prune:
		return []Command{{Args: []string{"firewall", "prune", "--yes"}}}
	case a.get("action") == remove:
		return []Command{{Args: []string{"firewall", "rm", picked[0], "--yes"}}}
	}
	return []Command{{Args: allowArgs(a, picked[:1])}}
}

// allowArgs builds the command line that allows hosts with what the dialog asked: ports, time and secret.
func allowArgs(a Answers, hosts []string) []string {
	args := append([]string{"firewall", "add"}, hosts...)
	// Name the ports the user chose
	if ports := chosenPorts(a); ports != "" {
		args = append(args, "--port", ports)
	}
	// Name the time the user chose, keeping a host open for good where the user said so
	if limit := chosenFor(a); limit != "" && limit != always {
		args = append(args, "--for", limit)
	}
	// Name the secret and where it goes
	if s := chosenSecret(a); s != "" {
		args = append(args, "--secret", s, "--inject", strings.Join(a.all("inject"), ","))
	}
	return append(args, "--yes")
}

// chosenPorts hands back the ports the user picked or typed, without spaces.
func chosenPorts(a Answers) string {
	// Take the ports the user typed
	if a.get("ports") == typed {
		return strings.ReplaceAll(a.get("portsTyped"), " ", "")
	}
	return a.get("ports")
}

// chosenFor hands back the time the user picked or typed.
func chosenFor(a Answers) string {
	// Take the time the user typed
	if a.get("for") == typed {
		return a.get("forTyped")
	}
	return a.get("for")
}

// chosenSecret hands back the secret hosts are to carry, and empty for none.
func chosenSecret(a Answers) string {
	s := a.get("secret")
	// Take the name the user typed for a new secret
	if s == fresh {
		return a.get("secretName")
	}
	// Read none as no secret
	if s == none {
		return ""
	}
	return s
}

// allowLines shows what allowing or changing hosts does: each host with its ports and when it closes.
func allowLines(h Home, a Answers, hosts []string) []string {
	var lines []string
	// Show every host the change reaches
	for _, name := range hosts {
		current, known := (&config.Config{Hosts: h.Hosts}).Find(name)
		sign, ports := "+", chosenPorts(a)
		// Mark a host the allowlist holds already, keeping its ports where the change names none
		if known {
			sign = "~"
			// Keep the ports the host opens where the change leaves them
			if ports == "" {
				ports = joinInts(current.Ports)
			}
		}
		line := fmt.Sprintf("%s %s:%s", sign, name, strings.ReplaceAll(orDefault(ports, "443"), ",", ", "))
		// Name when the host closes
		if d, err := config.ParseFor(chosenFor(a)); err == nil {
			line += "   closes at " + h.Now.Add(d).Local().Format("15:04")
		}
		// Name the secret the host carries
		if s := chosenSecret(a); s != "" {
			line += "   " + s + " in " + strings.Join(a.all("inject"), ", ")
		}
		lines = append(lines, line)
	}
	return lines
}

// allowSummary sums up in one line how many hosts the change allows and for how long.
func allowSummary(a Answers, hosts []string) string {
	line := "Allows " + count(len(hosts), "host", "hosts")
	// Name how long the hosts stay open
	if d, err := config.ParseFor(chosenFor(a)); err == nil {
		return line + " for " + spoken(d)
	}
	return line + " for good"
}

// removeLines names what removing a host does, and who keeps it open where features still need it.
func removeLines(h Home, name string) []string {
	host, _ := (&config.Config{Hosts: h.Hosts}).Find(name)
	others := slices.DeleteFunc(slices.Clone(host.By), func(b string) bool { return b == "user" })
	// Name the features that keep the host open
	if len(others) > 0 {
		return []string{fmt.Sprintf("~ %s stays open, still needed by %s", name, joinAnd(others))}
	}
	line := fmt.Sprintf("- %s:%s", name, strings.ReplaceAll(joinInts(host.Ports), ",", ", "))
	// Name the secret that stops being injected
	if host.Secret != "" {
		line += ", " + host.Secret + " stops being injected"
	}
	return []string{line}
}

// pruneLines names every expired host the prune removes and when it expired.
func pruneLines(h Home) []string {
	var lines []string
	// Name every expired host
	for _, host := range expiredHosts(h) {
		lines = append(lines, "- "+host.Name+"   expired "+endsAt(host.Expires))
	}
	return lines
}

// expiredHosts lists the hosts whose end time has come.
func expiredHosts(h Home) []config.Host {
	return (&config.Config{Hosts: h.Hosts}).Expired(h.Now)
}

// hostFacts are the facts a host's detail shows.
func hostFacts(h Home, host config.Host) []Fact {
	t := h.Traffic[host.Name]
	secret := "none"
	// Name the secret and where it goes
	if host.Secret != "" {
		secret = host.Secret + ", in the " + strings.Join(host.Inject, ", ")
	}
	return []Fact{
		{Label: "Ports", Value: strings.ReplaceAll(joinInts(host.Ports), ",", ", ")},
		{Label: "Data", Value: orDash(t.Data)},
		{Label: "Last seen", Value: orDash(t.Seen)},
		{Label: "Closes", Value: closes(host, h.Now)},
		{Label: "Secret", Value: secret},
		{Label: "Needed by", Value: joinAnd(host.By)},
	}
}

// closes names when a host closes: for good, expired, or how long from now.
func closes(host config.Host, now time.Time) string {
	end, err := time.Parse(config.EndTimeLayout, host.Expires)
	// Name a host that stays open
	if host.Expires == "" || err != nil {
		return "for good"
	}
	// Name a host whose end has come
	if !end.After(now) {
		return "expired"
	}
	return "in " + spoken(end.Sub(now).Round(time.Minute))
}

// endsAt writes the local date an end time falls on.
func endsAt(end string) string {
	t, err := time.Parse(config.EndTimeLayout, end)
	// Show an end time that fails to read as it stands
	if err != nil {
		return end
	}
	return t.Local().Format("Jan 2")
}

// spoken writes a duration the way a sentence says it, in its largest whole unit.
func spoken(d time.Duration) string {
	// Name the largest whole unit the duration fills
	switch {
	case d >= 24*time.Hour && d%(24*time.Hour) == 0:
		return count(int(d/(24*time.Hour)), "day", "days")
	case d >= time.Hour && d%time.Hour == 0:
		return count(int(d/time.Hour), "hour", "hours")
	case d >= time.Minute && d%time.Minute == 0:
		return count(int(d/time.Minute), "minute", "minutes")
	}
	return d.String()
}

// secretState notes whether a secret holds a value.
func secretState(s Secret) string {
	// Note a secret with a value
	if s.Set {
		return "set"
	}
	return "waits for a value"
}

// joinInts writes numbers as one comma-separated value.
func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	// Write every number
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// orDash shows an empty value as a dash.
func orDash(v string) string {
	return orDefault(v, "–")
}

// orDefault shows an empty value as the one given.
func orDefault(v, fallback string) string {
	// Take the fallback for an empty value
	if v == "" {
		return fallback
	}
	return v
}
