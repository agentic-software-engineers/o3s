package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// choice is one numbered answer an interview question offers, with the line that says what it means.
type choice struct {
	label, note string
}

// isolation is what a new home chose: its VM, or this machine's kernel with or without AppArmor.
type isolation struct {
	skipVM, skipAppArmor bool
}

// interview asks a new home how it isolates the cage: its VM, the recommended one, or this machine's kernel, and on
// the shared kernel whether AppArmor confines the cage.
func interview(in io.Reader, out io.Writer) isolation {
	r := bufio.NewReader(in)
	process := ask(r, out, "Isolation", choice{"VM (recommended)", "the home runs in its own VM, AppArmor inside it"}, choice{"Process", "the cage shares this machine's kernel"}) == 2
	// Keep the VM, which confines the cage on its own kernel
	if !process {
		return isolation{}
	}
	off := ask(r, out, "AppArmor", choice{"On (recommended)", "limits what the cage may do on the shared kernel"}, choice{"Off", "the cage keeps its other limits"}) == 2
	return isolation{skipVM: true, skipAppArmor: off}
}

// ask shows a question with its numbered choices and reads the number of one, the first and recommended one on an
// empty answer, and asks again on any other.
func ask(r *bufio.Reader, out io.Writer, title string, choices ...choice) int {
	fmt.Fprintln(out, title)
	width := 0
	// Measure the longest label, so the notes line up
	for _, c := range choices {
		width = max(width, len(c.label))
	}
	// Show every choice with its number and what it means
	for i, c := range choices {
		fmt.Fprintf(out, "  %d. %-*s   %s\n", i+1, width, c.label, c.note)
	}
	// Ask until the answer names a choice
	for {
		fmt.Fprint(out, "Choose [1]: ")
		line, err := r.ReadString('\n')
		answer := strings.TrimSpace(line)
		// Take the recommended choice on an empty answer
		if answer == "" {
			return 1
		}
		// Take the choice the number names
		for i := range choices {
			// Read the number of this choice
			if answer == fmt.Sprint(i+1) {
				return i + 1
			}
		}
		// Take the recommended choice where the input ends without a number to read
		if err != nil {
			return 1
		}
		fmt.Fprintf(out, "Type a number from 1 to %d\n", len(choices))
	}
}
