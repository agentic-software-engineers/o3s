package menu

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
)

// Ports as a dialog takes them as text, the one a host opens by default and both a host may open.
var (
	port      = strconv.Itoa(fixture.Port)
	bothPorts = strconv.Itoa(fixture.OtherPort) + "," + port
)

// aHome is a home that allows a host with a secret, knows two secrets, and offers one store.
var aHome = Home{
	Dir:    "/home/ubuntu/o3s",
	Exists: true,
	Hosts: []config.Host{
		{Name: fixture.Host, Ports: []int{fixture.OtherPort, fixture.Port}, By: []string{"user"}},
		{Name: fixture.Subdomain, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header", "body"}, By: []string{"user"}},
		{Name: fixture.OtherHost, Ports: []int{fixture.Port}, By: []string{fixture.Store + "/" + fixture.Feature}},
	},
	Secrets:  []Secret{{Name: fixture.Secret, Set: false}, {Name: fixture.OtherSecret, Set: true}},
	Stores:   []string{fixture.Store},
	Projects: []string{fixture.NestedRepo},
	Blocked:  []Option{{Value: fixture.RefusedName, Note: "3 attempts, 5m ago"}, {Value: fixture.AbsentName, Note: "1 attempt, 2h ago"}},
}

// walk answers each question the core asks in turn and hands back what it asked and the command it ends in.
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

// titles lists what each question asked.
func titles(qs []Question) []string {
	out := make([]string, len(qs))
	// Take every question's title
	for i, q := range qs {
		out[i] = q.Title
	}
	return out
}

// Every dialog ends in the command line the CLI runs for it.
func TestEveryDialogEndsInItsCommand(t *testing.T) {
	value := fixture.SecretValue
	tests := []struct {
		name    string
		answers [][]string
		titles  []string
		want    []Command
	}{
		{
			"a new host with a new secret and two sites",
			[][]string{{"firewall"}, {"add"}, {fixture.ThirdHost}, {bothPorts}, {"new"}, {fixture.NewSecret}, {"header", "body"}, {"always"}},
			[]string{"What do you want to do?", "Action", "Host", "Ports", "Secret", "Secret name", "Inject", "For"},
			[]Command{{Args: []string{"firewall", "add", fixture.ThirdHost, "--port", bothPorts, "--secret", fixture.NewSecret, "--inject", "header,body"}}},
		},
		{
			"a host without a secret skips where to inject it",
			[][]string{{"firewall"}, {"add"}, {fixture.ThirdHost}, {port}, {"none"}, {"always"}},
			[]string{"What do you want to do?", "Action", "Host", "Ports", "Secret", "For"},
			[]Command{{Args: []string{"firewall", "add", fixture.ThirdHost, "--port", port}}},
		},
		{
			"a new host for a set time",
			[][]string{{"firewall"}, {"add"}, {fixture.ThirdHost}, {port}, {"none"}, {"1h"}},
			[]string{"What do you want to do?", "Action", "Host", "Ports", "Secret", "For"},
			[]Command{{Args: []string{"firewall", "add", fixture.ThirdHost, "--port", port, "--for", "1h"}}},
		},
		{
			"a known host for a set time, at the real terminal where the CLI asks before it closes sooner",
			[][]string{{"firewall"}, {"add"}, {fixture.Host}, {bothPorts}, {"none"}, {"1h"}},
			[]string{"What do you want to do?", "Action", "Host", "Ports", "Secret", "For"},
			[]Command{{Args: []string{"firewall", "add", fixture.Host, "--port", bothPorts, "--for", "1h"}, Terminal: true}},
		},
		{
			"pruning the expired hosts, at the real terminal where the CLI asks",
			[][]string{{"firewall"}, {"prune"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"firewall", "prune"}, Terminal: true}},
		},
		{
			"removing a host the user needs",
			[][]string{{"firewall"}, {"rm"}, {fixture.Host}},
			[]string{"What do you want to do?", "Action", "Host"},
			[]Command{{Args: []string{"firewall", "rm", fixture.Host}}},
		},
		{
			"listing the allowlist",
			[][]string{{"firewall"}, {"ls"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"firewall", "ls"}}},
		},
		{
			"showing the log",
			[][]string{{"firewall"}, {"log"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"firewall", "log"}}},
		},
		{
			"allowing two blocked hosts at once",
			[][]string{{"firewall"}, {"blocked"}, {fixture.RefusedName, fixture.AbsentName}, {port}},
			[]string{"What do you want to do?", "Action", "Blocked", "Ports"},
			[]Command{
				{Args: []string{"firewall", "add", fixture.RefusedName, "--port", port}},
				{Args: []string{"firewall", "add", fixture.AbsentName, "--port", port}},
			},
		},
		{
			"a value for a known secret",
			[][]string{{"secret"}, {"add"}, {fixture.Secret}, {value}},
			[]string{"What do you want to do?", "Action", "Name", "Value"},
			[]Command{{Args: []string{"secret", "add", fixture.Secret}, Stdin: &value}},
		},
		{
			"a value for a secret of another name",
			[][]string{{"secret"}, {"add"}, {"other"}, {fixture.NewSecret}, {value}},
			[]string{"What do you want to do?", "Action", "Name", "Secret name", "Value"},
			[]Command{{Args: []string{"secret", "add", fixture.NewSecret}, Stdin: &value}},
		},
		{
			"removing a stored secret",
			[][]string{{"secret"}, {"rm"}, {fixture.OtherSecret}},
			[]string{"What do you want to do?", "Action", "Name"},
			[]Command{{Args: []string{"secret", "rm", fixture.OtherSecret}}},
		},
		{
			"adding a store",
			[][]string{{"store"}, {"add"}, {fixture.AbsentStore}},
			[]string{"What do you want to do?", "Action", "Store"},
			[]Command{{Args: []string{"store", "add", fixture.AbsentStore}}},
		},
		{
			"removing a store",
			[][]string{{"store"}, {"rm"}, {fixture.Store}},
			[]string{"What do you want to do?", "Action", "Store"},
			[]Command{{Args: []string{"store", "rm", fixture.Store}}},
		},
		{
			"creating a home",
			[][]string{{"init"}, {"yes"}},
			[]string{"What do you want to do?", "Create an o3s home in /home/ubuntu/o3s?"},
			[]Command{{Args: []string{"init"}, Terminal: true}},
		},
		{
			"declining to create a home",
			[][]string{{"init"}, {"no"}},
			[]string{"What do you want to do?", "Create an o3s home in /home/ubuntu/o3s?"},
			nil,
		},
		{
			"updating the home",
			[][]string{{"update"}},
			[]string{"What do you want to do?"},
			[]Command{{Args: []string{"update"}, Terminal: true}},
		},
		{
			"cloning a project",
			[][]string{{"project"}, {"add"}, {fixture.Repo}},
			[]string{"What do you want to do?", "Action", "Repository"},
			[]Command{{Args: []string{"project", "add", fixture.Repo}}},
		},
		{
			"creating a project",
			[][]string{{"project"}, {"create"}, {fixture.Repo}},
			[]string{"What do you want to do?", "Action", "Repository"},
			[]Command{{Args: []string{"project", "create", fixture.Repo}}},
		},
		{
			"removing a project at the real terminal, where the user types its name",
			[][]string{{"project"}, {"rm"}, {fixture.NestedRepo}},
			[]string{"What do you want to do?", "Action", "Project"},
			[]Command{{Args: []string{"project", "rm", fixture.NestedRepo}, Terminal: true}},
		},
		{
			"listing the projects",
			[][]string{{"project"}, {"ls"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"project", "ls"}}},
		},
		{
			"opening the cage in VS Code",
			[][]string{{"cage"}, {"ide"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"cage", "ide"}}},
		},
		{
			"opening a shell in the cage",
			[][]string{{"cage"}, {"shell"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"cage", "shell"}, Terminal: true}},
		},
	}
	// Walk every dialog
	for _, tt := range tests {
		asked, got := walk(t, aHome, tt.answers...)

		// Report questions that differ
		if diff := cmp.Diff(tt.titles, titles(asked)); diff != "" {
			t.Errorf("%s: questions (-want +got):\n%s", tt.name, diff)
		}
		// Report a command that differs
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("%s: command (-want +got):\n%s", tt.name, diff)
		}
	}
}

// A host the allowlist holds offers its ports, its secret and its sites as the defaults.
func TestFirewallAddOffersTheHostsCurrentValues(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"add"}, []string{fixture.Subdomain}, []string{port}, []string{fixture.Secret}, []string{"header", "body"}, []string{"always"})

	got := []any{asked[3].Default, asked[4].Default, asked[5].Default}

	want := []any{[]string{port}, []string{fixture.Secret}, []string{"header", "body"}}
	// Report defaults that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("defaults (-want +got):\n%s", diff)
	}
}

// A new host starts on the HTTPS port and puts a secret into the header.
func TestFirewallAddOffersTheDefaultsOfANewHost(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"add"}, []string{fixture.ThirdHost}, []string{port}, []string{fixture.OtherSecret}, []string{"header"}, []string{"always"})

	got := []any{asked[3].Default, asked[4].Default, asked[5].Default}

	want := []any{[]string{port}, []string{"none"}, []string{"header"}}
	// Report defaults that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("defaults (-want +got):\n%s", diff)
	}
}

// The secrets on offer show whether each holds a value, and leave room for none or a new one.
func TestFirewallAddOffersEverySecret(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"add"}, []string{fixture.ThirdHost}, []string{port}, []string{"none"}, []string{"always"})

	got := asked[4].Options

	want := []Option{{Value: "none"}, {Value: fixture.Secret, Note: "empty"}, {Value: fixture.OtherSecret, Note: "set"}, {Value: "new"}}
	// Report options that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("options (-want +got):\n%s", diff)
	}
}

// The firewall offers its actions, the log and the blocked hosts among them.
func TestFirewallOffersTheLogAndTheBlockedHosts(t *testing.T) {
	q, _ := next(aHome, Answers{}.With("do", []string{"firewall"}))

	want := []Option{{Value: "add"}, {Value: "rm"}, {Value: "ls"}, {Value: "log"}, {Value: "blocked"}, {Value: "prune"}}
	// Report actions that differ
	if diff := cmp.Diff(want, q.Options); diff != "" {
		t.Errorf("firewall actions (-want +got):\n%s", diff)
	}
}

// The blocked hosts come as a multi-select, each noting how often and when it was refused.
func TestBlockedOffersEveryRefusedName(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"blocked"}, []string{fixture.RefusedName}, []string{port})

	got := asked[2]

	// Report a question that takes one answer alone
	if got.Kind != Multi {
		t.Errorf("Blocked kind = %v, want Multi", got.Kind)
	}
	// Report options that differ
	if diff := cmp.Diff(aHome.Blocked, got.Options); diff != "" {
		t.Errorf("options (-want +got):\n%s", diff)
	}
}

// Removing offers the hosts the user needs alone, since features own the rest.
func TestFirewallRmOffersTheUsersHosts(t *testing.T) {
	asked, _ := walk(t, aHome, []string{"firewall"}, []string{"rm"}, []string{fixture.Host})

	got := asked[2].Options

	want := []Option{{Value: fixture.Host}, {Value: fixture.Subdomain}}
	// Report options that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("options (-want +got):\n%s", diff)
	}
}

// A removal with nothing to remove ends with a note.
func TestRmWithNothingToRemoveEndsWithANote(t *testing.T) {
	tests := []struct {
		group string
		want  string
	}{
		{"firewall", "No host to remove"},
		{"secret", "No secret to remove"},
		{"store", "No store to remove"},
		{"project", "No project to remove"},
	}
	// Check every group
	for _, tt := range tests {
		_, got := walk(t, Home{Dir: "/home/ubuntu/o3s"}, []string{tt.group}, []string{"rm"})

		// Report a command that differs
		if diff := cmp.Diff([]Command{{Note: tt.want}}, got); diff != "" {
			t.Errorf("%s rm (-want +got):\n%s", tt.group, diff)
		}
	}
}

// Without a blocked host the dialog ends with a note, which points a stopped stack at starting it.
func TestBlockedWithNothingBlockedEndsWithANote(t *testing.T) {
	tests := []struct {
		running bool
		want    string
	}{
		{true, "No blocked host"},
		{false, "Start the stack to see the blocked hosts"},
	}
	// Check a running and a stopped stack
	for _, tt := range tests {
		_, got := walk(t, Home{Dir: "/home/ubuntu/o3s", Exists: true, Running: tt.running}, []string{"firewall"}, []string{"blocked"})

		// Report a command that differs
		if diff := cmp.Diff([]Command{{Note: tt.want}}, got); diff != "" {
			t.Errorf("firewall blocked with running %v (-want +got):\n%s", tt.running, diff)
		}
	}
}

// The cage offers its own actions, the editor and the shell.
func TestCageOffersTheEditorAndTheShell(t *testing.T) {
	q, _ := next(aHome, Answers{}.With("do", []string{"cage"}))

	want := []Option{{Value: "ide"}, {Value: "shell"}}
	// Report actions that differ
	if diff := cmp.Diff(want, q.Options); diff != "" {
		t.Errorf("cage actions (-want +got):\n%s", diff)
	}
}
