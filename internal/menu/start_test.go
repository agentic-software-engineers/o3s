package menu

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The main menu of a home offers every group with a note on what it does.
func TestTheMainMenuExplainsEachEntry(t *testing.T) {
	q, _ := Next(aHome, nil)

	want := []Option{
		{Value: "firewall", Note: "allow hosts the cage reaches"},
		{Value: "secret", Note: "values the proxy injects"},
		{Value: "store", Note: "where features come from"},
		{Value: "feature", Note: "tools in the cage"},
		{Value: "cage", Note: "open VS Code or a shell"},
		{Value: "update", Note: "binary, stack and features"},
	}
	// Report entries that differ
	if diff := cmp.Diff(want, q.Options); diff != "" {
		t.Errorf("main menu (-want +got):\n%s", diff)
	}
}

// Outside a home the main menu offers what works there: making the home, and the secrets kept on the host.
func TestOutsideAHomeTheMainMenuOffersInit(t *testing.T) {
	q, _ := Next(Home{Dir: "/home/ubuntu/o3s"}, nil)

	want := []Option{
		{Value: "init", Note: "make this folder a home"},
		{Value: "secret", Note: "values the proxy injects"},
	}
	// Report entries that differ
	if diff := cmp.Diff(want, q.Options); diff != "" {
		t.Errorf("main menu (-want +got):\n%s", diff)
	}
}

// The commands that need the real terminal say so: init asks sudo, update may restart, the shell takes it over.
func TestTerminalCommandsAreMarked(t *testing.T) {
	tests := []struct {
		answers Answers
		want    bool
	}{
		{Answers{}.With("do", []string{"init"}).With("init", []string{yes}), true},
		{Answers{}.With("do", []string{"update"}), true},
		{Answers{}.With("do", []string{"cage"}).With("action", []string{"shell"}), true},
		{Answers{}.With("do", []string{"cage"}).With("action", []string{"ide"}), false},
		{Answers{}.With("do", []string{"firewall"}).With("action", []string{"ls"}), false},
	}
	// Plan every dialog
	for _, tt := range tests {
		cmds := Plan(aHome, tt.answers)

		// Report a command marked otherwise
		if len(cmds) != 1 || cmds[0].Terminal != tt.want {
			t.Errorf("Plan(%v) = %+v, want Terminal %v", tt.answers, cmds, tt.want)
		}
	}
}
