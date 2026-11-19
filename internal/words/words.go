// Package words writes counts, lists and missing values the way a sentence says them.
package words

import (
	"fmt"
	"strings"
)

// Count writes a number with the word that fits it, the singular for one.
func Count[N ~int | ~int64](n N, one, many string) string {
	// Take the singular for a single one
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// JoinAnd joins names the way a sentence lists them, the last of several with and.
func JoinAnd(names []string) string {
	return join(names, "and")
}

// JoinOr joins choices the way a sentence offers them, the last of several with or.
func JoinOr(choices []string) string {
	return join(choices, "or")
}

// join joins items with commas and the last of several with the word given.
func join(items []string, last string) string {
	// Join a single item as it stands
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + last + " " + items[len(items)-1]
}

// Or shows an empty value as the one given in its place.
func Or(v, fallback string) string {
	// Take the fallback for an empty value
	if v == "" {
		return fallback
	}
	return v
}
