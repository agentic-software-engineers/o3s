package cli

import (
	"strings"
	"testing"
)

// The interview takes the VM on Enter, the shared kernel with AppArmor on its own Enter, and without it on 2, asking
// again on an answer that names no choice.
func TestInterviewChoosesHowTheHomeIsolates(t *testing.T) {
	tests := []struct {
		name, answers string
		want          isolation
	}{
		{"Enter keeps the VM", "\n", isolation{}},
		{"1 keeps the VM", "1\n", isolation{}},
		{"the shared kernel with AppArmor", "2\n\n", isolation{skipVM: true}},
		{"the shared kernel without AppArmor", "2\n2\n", isolation{skipVM: true, skipAppArmor: true}},
		{"a wrong answer, then the shared kernel", "3\n2\n1\n", isolation{skipVM: true}},
		{"input that ends keeps the recommended", "", isolation{}},
	}
	// Answer every way
	for _, tt := range tests {
		var out strings.Builder

		got := interview(strings.NewReader(tt.answers), &out)

		// Report a choice that differs
		if got != tt.want {
			t.Errorf("%s: interview() = %+v, want %+v\n%s", tt.name, got, tt.want, out.String())
		}
	}
}

// The interview shows every choice numbered with what it means, the recommended one first, and AppArmor only for the
// shared kernel.
func TestInterviewShowsTheChoices(t *testing.T) {
	var vm, process strings.Builder

	interview(strings.NewReader("\n"), &vm)
	interview(strings.NewReader("2\n\n"), &process)

	// Require the isolation choices, numbered, with what they mean
	for _, want := range []string{"Isolation", "  1. VM (recommended)   the home runs in its own VM, AppArmor inside it", "  2. Process            the cage shares this machine's kernel", "Choose [1]: "} {
		// Report a line the interview lacks
		if !strings.Contains(vm.String(), want) {
			t.Errorf("interview lacks %q:\n%s", want, vm.String())
		}
	}
	// Require the AppArmor question for the shared kernel alone
	if strings.Contains(vm.String(), "AppArmor\n") || !strings.Contains(process.String(), "  1. On (recommended)") {
		t.Errorf("AppArmor asked for the VM or left out for the shared kernel:\n%s\n---\n%s", vm.String(), process.String())
	}
}
