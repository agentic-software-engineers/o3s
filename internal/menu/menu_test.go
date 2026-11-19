package menu

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
)

// Ports as a dialog takes them, the one a host opens by default and both a host may open.
var (
	port      = strconv.Itoa(fixture.Port)
	bothPorts = strconv.Itoa(fixture.OtherPort) + "," + port
)

// aHome is a home that allows a host the user added, a host with a secret, an expired host and one a feature needs,
// knows two secrets, refused two names, offers one store and lists one project.
var aHome = Home{
	Dir:    "/home/ubuntu/o3s",
	Exists: true,
	Now:    fixture.Now,
	Hosts: []config.Host{
		{Name: fixture.Host, Ports: []int{fixture.OtherPort, fixture.Port}, By: []string{"user"}},
		{Name: fixture.Subdomain, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header", "body"}, By: []string{"user"}},
		{Name: fixture.ThirdHost, Ports: []int{fixture.Port}, By: []string{"user"}, Expires: fixture.LongAgoText},
		{Name: fixture.OtherHost, Ports: []int{fixture.Port}, By: []string{fixture.Store + "/" + fixture.Feature, "user"}},
	},
	Traffic:  map[string]Traffic{fixture.Host: {Data: "1.2 GB", Seen: "2m ago"}},
	Secrets:  []Secret{{Name: fixture.Secret, Set: false}, {Name: fixture.OtherSecret, Set: true}},
	Stores:   []string{fixture.Store},
	Projects: []string{"github.com/" + fixture.Repo},
	Blocked:  []Option{{Value: fixture.RefusedName, Columns: []string{"3 attempts", "5m ago"}}, {Value: fixture.AbsentName, Columns: []string{"1 attempt", "2h ago"}}},
	Running:  true,
	Stack:    []Fact{{Label: "Cage", Value: "running"}},
	Lookup:   fakeLookup{},
}

// walk answers each question the core asks in turn and hands back what it asked and the commands it ends in.
func walk(t *testing.T, h Home, answers ...[]string) ([]Question, []Command) {
	t.Helper()
	var a Answers
	var asked []Question
	// Answer every question until the dialog ends
	for {
		q, done := next(h, a)
		// End the walk once the dialog asks nothing more
		if done {
			return asked, plan(h, a)
		}
		// Stop when the dialog asks more than the test answers
		if len(asked) == len(answers) {
			t.Fatalf("walk: asked %q beyond the %d answers", q.Title, len(answers))
		}
		a = a.With(q.Key, answers[len(asked)])
		asked = append(asked, q)
	}
}

// keys lists the key of each question asked.
func keys(qs []Question) []string {
	out := make([]string, len(qs))
	// Take every question's key
	for i, q := range qs {
		out[i] = q.Key
	}
	return out
}

// dialogs are walks through a dialog with the questions they ask and the commands they end in.
type dialogs []struct {
	name    string
	answers [][]string
	keys    []string
	want    []Command
}

// check walks every dialog and reports the questions and the commands that differ.
func (ds dialogs) check(t *testing.T, h Home) {
	t.Helper()
	// Walk every dialog
	for _, d := range ds {
		asked, cmds := walk(t, h, d.answers...)

		// Report questions that differ
		if diff := cmp.Diff(d.keys, keys(asked)); diff != "" {
			t.Errorf("%s: questions (-want +got):\n%s", d.name, diff)
		}
		// Report commands that differ
		if diff := cmp.Diff(d.want, cmds); diff != "" {
			t.Errorf("%s: commands (-want +got):\n%s", d.name, diff)
		}
	}
}

// The main menu of a home offers every group, each noting what it holds.
func TestTheMainMenuNotesWhatEachGroupHolds(t *testing.T) {
	q, _ := Next(aHome, nil)

	want := []Option{
		{Value: "cage", Note: "VM, stack and cage · running"},
		{Value: "feature", Note: "tools in the cage · 0 installed"},
		{Value: "firewall", Note: "hosts the cage reaches · 4 allowed · 2 blocked"},
		{Value: "project", Note: "repositories in the workspace · 1"},
		{Value: "secret", Note: "values the proxy injects · 2"},
		{Value: "update", Note: "o3s, the stack and the features"},
	}
	// Report entries that differ
	if diff := cmp.Diff(want, q.Options); diff != "" {
		t.Errorf("main menu (-want +got):\n%s", diff)
	}
}

// Outside a home the main menu offers what works there: making the home, and the secrets kept on the host.
func TestOutsideAHomeTheMainMenuOffersInit(t *testing.T) {
	q, _ := Next(Home{Dir: aHome.Dir}, nil)

	// Report entries that differ
	if got := []string{q.Options[0].Value, q.Options[1].Value}; len(q.Options) != 2 || !cmp.Equal([]string{"init", "secret"}, got) {
		t.Errorf("main menu = %v, want init and secret", q.Options)
	}
}

// The menu opens at the view of the group a command names, the stores at the feature view's Stores tab.
func TestOpenStartsAtTheGroupsView(t *testing.T) {
	got := []Answers{Open(), Open("firewall"), Open("feature", "store")}

	want := []Answers{nil, {{Key: "do", Values: []string{"firewall"}}}, {{Key: "do", Values: []string{"feature"}}, {Key: "tab", Values: []string{storesTab}}}}
	// Report answers that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Open() (-want +got):\n%s", diff)
	}
}

// The firewall's view shows every host with its ports, traffic, end and who needs it, a key to prune where one has
// expired, and the refused names to mark.
func TestTheFirewallViewShowsTheHostsAndTheRefusedNames(t *testing.T) {
	q, _ := Next(aHome, Answers{}.With("do", []string{"firewall"}))

	allowed, blocked := q.Tabs[0], q.Tabs[1]
	// Require the add row first, then every host with its columns
	if allowed.Options[0].Value != add || len(allowed.Options) != len(aHome.Hosts)+1 {
		t.Errorf("Allowed = %v", allowed.Options)
	}
	// Require the host's traffic and end in its columns
	if diff := cmp.Diff([]string{bothPorts, "1.2 GB", "2m ago", "for good", "", "user"}, allowed.Options[1].Columns); diff != "" {
		t.Errorf("host columns (-want +got):\n%s", diff)
	}
	// Require the expired host to read as expired, and the key that prunes it
	if allowed.Options[3].Columns[3] != "expired" || len(allowed.Keys) != 1 || allowed.Keys[0].Value != prune {
		t.Errorf("expired host = %v, keys %v", allowed.Options[3], allowed.Keys)
	}
	// Require the refused names to mark
	if !blocked.Multi || len(blocked.Options) != 2 {
		t.Errorf("Blocked = %+v", blocked)
	}
}

// Every firewall dialog ends in the command line the CLI runs for it, asking for --yes where the menu asked already.
func TestEveryFirewallDialogEndsInItsCommand(t *testing.T) {
	dialogs{
		{
			"a new host on the HTTPS port for good",
			[][]string{{"firewall"}, {"Allowed", add}, {"one.example.net"}, {port}, {always}, {none}, {yes}},
			[]string{"do", "view", "hosts", "ports", "for", "secret", "apply"},
			[]Command{{Args: []string{"firewall", "add", "one.example.net", "--port", port, "--yes"}}},
		},
		{
			"two hosts on typed ports for a typed time with a new secret in two sites",
			[][]string{{"firewall"}, {"Allowed", add}, {"one.example.net two.example.net"}, {typed}, {"22, 443"}, {typed}, {"90m"}, {fresh}, {fixture.NewSecret}, {"header", "body"}, {yes}},
			[]string{"do", "view", "hosts", "ports", "portsTyped", "for", "forTyped", "secret", "secretName", "inject", "apply"},
			[]Command{{Args: []string{"firewall", "add", "one.example.net", "two.example.net", "--port", bothPorts, "--for", "90m", "--secret", fixture.NewSecret, "--inject", "header,body", "--yes"}}},
		},
		{
			"two refused names for an hour",
			[][]string{{"firewall"}, {"Blocked", fixture.RefusedName, fixture.AbsentName}, {port}, {"1h"}, {none}, {yes}},
			[]string{"do", "view", "ports", "for", "secret", "apply"},
			[]Command{{Args: []string{"firewall", "add", fixture.RefusedName, fixture.AbsentName, "--port", port, "--for", "1h", "--yes"}}},
		},
		{
			"a host's ports",
			[][]string{{"firewall"}, {"Allowed", fixture.Host}, {changePorts}, {port}, {yes}},
			[]string{"do", "view", "action", "ports", "apply"},
			[]Command{{Args: []string{"firewall", "add", fixture.Host, "--port", port, "--yes"}}},
		},
		{
			"a host's end",
			[][]string{{"firewall"}, {"Allowed", fixture.Host}, {changeEnd}, {"8h"}, {yes}},
			[]string{"do", "view", "action", "for", "apply"},
			[]Command{{Args: []string{"firewall", "add", fixture.Host, "--for", "8h", "--yes"}}},
		},
		{
			"a host's secret",
			[][]string{{"firewall"}, {"Allowed", fixture.Host}, {changeSecret}, {fixture.OtherSecret}, {"header"}, {yes}},
			[]string{"do", "view", "action", "secret", "inject", "apply"},
			[]Command{{Args: []string{"firewall", "add", fixture.Host, "--secret", fixture.OtherSecret, "--inject", "header", "--yes"}}},
		},
		{
			"removing a host",
			[][]string{{"firewall"}, {"Allowed", fixture.Host}, {remove}, {yes}},
			[]string{"do", "view", "action", "apply"},
			[]Command{{Args: []string{"firewall", "rm", fixture.Host, "--yes"}}},
		},
		{
			"declining the removal",
			[][]string{{"firewall"}, {"Allowed", fixture.Host}, {remove}, {"no"}},
			[]string{"do", "view", "action", "apply"},
			[]Command{{Note: nothingChanged}},
		},
		{
			"stepping back from a host",
			[][]string{{"firewall"}, {"Allowed", fixture.Host}, {back}},
			[]string{"do", "view", "action"},
			nil,
		},
		{
			"pruning the expired hosts",
			[][]string{{"firewall"}, {"Allowed", prune}, {yes}},
			[]string{"do", "view", "apply"},
			[]Command{{Args: []string{"firewall", "prune", "--yes"}}},
		},
	}.check(t, aHome)
}

// A host's detail shows its facts, and offers the removal only of a host the user added.
func TestAHostsDetailShowsItsFacts(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"Allowed", fixture.Host}, []string{back})

	got := asked[2]
	// Require the facts of the host
	if diff := cmp.Diff([]Fact{{"Ports", bothPorts}, {"Data", "1.2 GB"}, {"Last seen", "2m ago"}, {"Closes", "for good"}, {"Secret", "none"}, {"Needed by", "user"}}, got.Facts); diff != "" {
		t.Errorf("facts (-want +got):\n%s", diff)
	}
	// Require the removal among the actions, and Back last
	if n := len(got.Options); n != 5 || got.Options[3].Value != remove || got.Options[4].Value != back {
		t.Errorf("actions = %v", got.Options)
	}
}

// Removing a host a feature still needs names the feature that keeps it open.
func TestRemovingAHostNamesWhoKeepsItOpen(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"Allowed", fixture.OtherHost}, []string{remove}, []string{"no"})

	// Require the feature named
	if want := "~ " + fixture.OtherHost + " stays open, still needed by " + fixture.Store + "/" + fixture.Feature; asked[3].Preview[0] != want {
		t.Errorf("preview = %v, want %q", asked[3].Preview, want)
	}
}

// Every secret, project, cage, update and init dialog ends in the command lines the CLI runs for it.
func TestEveryOtherDialogEndsInItsCommand(t *testing.T) {
	value := fixture.SecretValue
	dialogs{
		{
			"a new secret bound to a host",
			[][]string{{"secret"}, {"Secrets", add}, {fixture.NewSecret}, {value}, {yes}, {fixture.Host}},
			[]string{"do", "view", "name", "value", "bind", "host"},
			[]Command{{Args: []string{"secret", "add", fixture.NewSecret, "--no-bind-hint"}, Stdin: &value}, {Args: []string{"firewall", "add", fixture.Host, "--secret", fixture.NewSecret, "--yes"}}},
		},
		{
			"a new secret bound later",
			[][]string{{"secret"}, {"Secrets", add}, {fixture.NewSecret}, {value}, {notNow}},
			[]string{"do", "view", "name", "value", "bind"},
			[]Command{{Args: []string{"secret", "add", fixture.NewSecret, "--no-bind-hint"}, Stdin: &value}},
		},
		{
			"a value for a secret a host carries",
			[][]string{{"secret"}, {"Secrets", fixture.Secret}, {setValue}, {value}},
			[]string{"do", "view", "action", "value"},
			[]Command{{Args: []string{"secret", "add", fixture.Secret, "--no-bind-hint"}, Stdin: &value}},
		},
		{
			"removing a secret's value",
			[][]string{{"secret"}, {"Secrets", fixture.OtherSecret}, {remove}, {yes}},
			[]string{"do", "view", "action", "apply"},
			[]Command{{Args: []string{"secret", "rm", fixture.OtherSecret, "--yes"}}},
		},
		{
			"two projects",
			[][]string{{"project"}, {"Projects", add}, {fixture.Repo + " " + fixture.NestedRepo}},
			[]string{"do", "view", "repos"},
			[]Command{{Args: []string{"project", "add", fixture.Repo, fixture.NestedRepo}}},
		},
		{
			"a new project",
			[][]string{{"project"}, {"Projects", create}, {fixture.Repo}},
			[]string{"do", "view", "repos"},
			[]Command{{Args: []string{"project", "create", fixture.Repo}}},
		},
		{
			"removing a project once its name is typed",
			[][]string{{"project"}, {"Projects", "github.com/" + fixture.Repo}, {remove}, {fixture.Repo}},
			[]string{"do", "view", "action", "typed"},
			[]Command{{Args: []string{"project", "rm", "github.com/" + fixture.Repo, "--yes"}}},
		},
		{
			"the shell, on the real terminal",
			[][]string{{"cage"}, {shell}},
			[]string{"do", "action"},
			[]Command{{Args: []string{"cage", "shell"}, Terminal: true}},
		},
		{
			"the marked parts of an update",
			[][]string{{"update"}, {"Update", "o3s", "stack"}, {yes}},
			[]string{"do", "view", "apply"},
			[]Command{{Args: []string{"update", "o3s", "stack", "--yes"}}},
		},
		{
			"an update with nothing marked",
			[][]string{{"update"}, {"Update"}},
			[]string{"do", "view"},
			[]Command{{Note: "Everything is up to date."}},
		},
	}.check(t, aHome)
	dialogs{
		{
			"a new home inside its VM, on the real terminal for sudo and the VM",
			[][]string{{"init"}, {inVM}, {yes}},
			[]string{"do", "isolation", "apply"},
			[]Command{{Args: []string{"init"}, Terminal: true}},
		},
		{
			"a new home on this machine's kernel with AppArmor",
			[][]string{{"init"}, {inProcess}, {on}, {yes}},
			[]string{"do", "isolation", "apparmor", "apply"},
			[]Command{{Args: []string{"init", "--dangerously-skip-vm"}, Terminal: true}},
		},
		{
			"a new home on this machine's kernel without AppArmor",
			[][]string{{"init"}, {inProcess}, {off}, {yes}},
			[]string{"do", "isolation", "apparmor", "apply"},
			[]Command{{Args: []string{"init", "--dangerously-skip-vm", "--dangerously-skip-apparmor"}, Terminal: true}},
		},
		{
			"declining a new home",
			[][]string{{"init"}, {inVM}, {"no"}},
			[]string{"do", "isolation", "apply"},
			[]Command{{Note: nothingChanged}},
		},
	}.check(t, Home{Dir: aHome.Dir})
}

// The update's view marks every part with a newer version, each with what changes.
func TestTheUpdateViewMarksEveryNewerPart(t *testing.T) {
	q, _ := Next(aHome, Answers{}.With("do", []string{"update"}))

	tab := q.Tabs[0]
	// Require every part marked with what changes
	if !tab.Multi || !cmp.Equal([]string{"o3s", "stack", toolPkg}, tab.Marked) || tab.Options[0].Columns[0] != fixture.Version+" -> "+fixture.NewerVersion {
		t.Errorf("update tab = %+v", tab)
	}
}

// A project's removal holds the name typed back to the project's owner/name.
func TestAProjectsRemovalChecksTheTypedName(t *testing.T) {
	asked, cmds := walk(t, aHome, []string{"project"}, []string{"Projects", "github.com/" + fixture.Repo}, []string{remove}, []string{"other/name"})

	// Require the name checked as it is typed
	if _, err := asked[3].Check("other/name"); err == nil {
		t.Errorf("Check(other/name) = nil, want the name named")
	}
	// Require nothing deleted for a name that differs
	if diff := cmp.Diff([]Command{{Note: nothingChanged}}, cmds); diff != "" {
		t.Errorf("commands (-want +got):\n%s", diff)
	}
}

// A host shows the same row piped and in its view: ports, traffic, when it closes, its secret marked while it waits,
// and who needs it.
func TestHostRowShowsAHostTheSameEverywhere(t *testing.T) {
	tests := []struct {
		host    config.Host
		traffic Traffic
		waiting bool
		want    []string
	}{
		{aHome.Hosts[0], aHome.Traffic[fixture.Host], false, []string{bothPorts, "1.2 GB", "2m ago", "for good", "", "user"}},
		{aHome.Hosts[1], Traffic{}, true, []string{port, "–", "–", "for good", fixture.Secret + " !", "user"}},
		{aHome.Hosts[2], Traffic{}, false, []string{port, "–", "–", "expired", "", "user"}},
		{config.Host{Name: fixture.Host, Ports: []int{fixture.Port}, Expires: fixture.InAnHourText, By: []string{"user"}}, Traffic{}, false, []string{port, "–", "–", "in 1h", "", "user"}},
	}
	// Check every host
	for _, tt := range tests {
		// Report a row that differs
		if diff := cmp.Diff(tt.want, HostRow(tt.host, tt.traffic, tt.waiting, fixture.Now)); diff != "" {
			t.Errorf("HostRow(%s) (-want +got):\n%s", tt.host.Name, diff)
		}
	}
}

// A secret's value reads as set or empty, piped and in its view alike.
func TestSecretStateReadsSetOrEmpty(t *testing.T) {
	// Report a state that reads otherwise
	if SecretState(true) != "set" || SecretState(false) != "empty" {
		t.Errorf("SecretState() = %q, %q, want set and empty", SecretState(true), SecretState(false))
	}
}

// Every cage dialog ends in its command: a shell on the real terminal, the start, the stop, and the removal once
// confirmed, and a home in its VM opens its shell in the VM.
func TestEveryCageDialogEndsInItsCommand(t *testing.T) {
	dialogs{
		{"starting", [][]string{{"cage"}, {start}}, []string{"do", "action"}, []Command{{Args: []string{"cage", "start"}}}},
		{"stopping", [][]string{{"cage"}, {stop}}, []string{"do", "action"}, []Command{{Args: []string{"cage", "stop"}}}},
		{"removing", [][]string{{"cage"}, {remove}, {yes}}, []string{"do", "action", "apply"}, []Command{{Args: []string{"cage", "rm", "--yes"}}}},
		{"declining the removal", [][]string{{"cage"}, {remove}, {"no"}}, []string{"do", "action", "apply"}, []Command{{Note: nothingChanged}}},
	}.check(t, aHome)
	inVM := Home{Dir: aHome.Dir, InVM: true, Stack: []Fact{{Label: "VM", Value: "running"}}, Lookup: fakeLookup{}}
	dialogs{
		{"a shell in the VM, on the real terminal", [][]string{{"cage"}, {vmShell}}, []string{"do", "action"}, []Command{{Args: []string{"cage", "shell", "--vm"}, Terminal: true}}},
	}.check(t, inVM)
}

// Beside a home in its VM, the main menu offers the cage and the secrets, noting the VM's state, and the removal names
// the VM's disk.
func TestAHomeInItsVMOffersItsCage(t *testing.T) {
	h := Home{Dir: aHome.Dir, InVM: true, Stack: []Fact{{Label: "VM", Value: "running"}}, Lookup: fakeLookup{}}

	q, _ := Next(h, nil)
	asked, _ := walk(t, h, []string{"cage"}, []string{remove}, []string{"no"})

	// Require the cage with the VM's state, and the secrets
	if len(q.Options) != 2 || q.Options[0].Value != "cage" || q.Options[0].Note != "VM, stack and cage · running" || q.Options[1].Value != "secret" {
		t.Errorf("main menu = %+v", q.Options)
	}
	// Require the shell in the VM first among the actions
	if asked[1].Options[0].Value != vmShell {
		t.Errorf("actions = %+v, want the shell in the VM first", asked[1].Options)
	}
	// Require the removal to name the VM's disk
	if want := "- the VM and its disk, with the home and the projects inside"; asked[2].Preview[0] != want {
		t.Errorf("removal = %v, want %q", asked[2].Preview, want)
	}
}
