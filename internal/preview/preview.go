// Package preview names the features o3s ships ahead of their default, each turned on by name or all at once.
package preview

import (
	"os"
	"strings"
)

// Feature names a feature that ships as a preview.
type Feature string

const (
	// VM makes a new home inside its own VM rather than in the folder o3s init runs in.
	VM Feature = "vm"
)

// all reports whether this run turns every preview on.
var all bool

// EnableAll turns every preview on for this run.
func EnableAll() {
	all = true
}

// Enabled reports whether a preview is on: for this run, through O3S_PREVIEW=1, or through O3S_PREVIEW naming it.
func Enabled(f Feature) bool {
	value := os.Getenv("O3S_PREVIEW")
	// Turn on every preview this run or the environment asks for
	if all || value == "1" {
		return true
	}
	// Look for the feature among the names the environment lists
	for name := range strings.SplitSeq(value, ",") {
		// Take a name that matches the feature
		if strings.TrimSpace(name) == string(f) {
			return true
		}
	}
	return false
}
