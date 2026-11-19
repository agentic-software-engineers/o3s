package menutest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// Every path of the menu on every home reads as one line from the main menu to the commands it ends in, so a path
// that comes, goes or changes shows in review.
func TestPaths(t *testing.T) {
	var b strings.Builder

	// Write the paths of every home
	for _, h := range Homes() {
		paths, err := Walk(h.Home)
		// Report a menu the walk can't follow
		if err != nil {
			t.Fatalf("%s: %v", h.Name, err)
		}
		fmt.Fprintf(&b, "# %s, %d paths\n", h.Name, len(paths))
		// Write every path as a line
		for _, p := range paths {
			fmt.Fprintln(&b, Line(p))
		}
		b.WriteString("\n")
	}

	golden.RequireEqual(t, b.String())
}
