package words_test

import (
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/words"
)

// A count takes the singular for one and the plural otherwise.
func TestCountTakesTheWordThatFits(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 hosts"},
		{1, "1 host"},
		{3, "3 hosts"},
	}
	// Check every number
	for _, tt := range tests {
		// Report a count that reads otherwise
		if got := words.Count(tt.n, "host", "hosts"); got != tt.want {
			t.Errorf("Count(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// A list joins with commas and the last of several with and, or with or.
func TestJoinListsTheWayASentenceDoes(t *testing.T) {
	tests := []struct {
		items           []string
		wantAnd, wantOr string
	}{
		{nil, "", ""},
		{[]string{fixture.Host}, fixture.Host, fixture.Host},
		{[]string{fixture.Host, fixture.OtherHost}, fixture.Host + " and " + fixture.OtherHost, fixture.Host + " or " + fixture.OtherHost},
		{[]string{fixture.Host, fixture.OtherHost, fixture.ThirdHost}, fixture.Host + ", " + fixture.OtherHost + " and " + fixture.ThirdHost, fixture.Host + ", " + fixture.OtherHost + " or " + fixture.ThirdHost},
	}
	// Check every list
	for _, tt := range tests {
		// Report a list joined otherwise
		if got := words.JoinAnd(tt.items); got != tt.wantAnd {
			t.Errorf("JoinAnd(%v) = %q, want %q", tt.items, got, tt.wantAnd)
		}
		// Report a list offered otherwise
		if got := words.JoinOr(tt.items); got != tt.wantOr {
			t.Errorf("JoinOr(%v) = %q, want %q", tt.items, got, tt.wantOr)
		}
	}
}

// An empty value shows as the fallback, and any other value as it is.
func TestOrFillsAnEmptyValue(t *testing.T) {
	// Report an empty value left empty, or a value replaced
	if words.Or("", "none") != "none" || words.Or(fixture.Host, "none") != fixture.Host {
		t.Errorf("Or() fills otherwise")
	}
}
