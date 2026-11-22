package traffic

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
)

// now is the moment every test reads the record at.
var now = fixture.Now

// day is one day in seconds, the unit the record counts in.
const day = 24 * 3600

// A gateway yet to write a record hands back empty data, which reads as an empty record.
func TestParseReadsNothingAsEmpty(t *testing.T) {
	got, err := Parse(nil)

	// Report an empty read that fails
	if err != nil {
		t.Fatalf("Parse(nil) = %v, want an empty record", err)
	}
	// Report an empty read that holds hosts
	if len(got.Hosts) != 0 || len(got.Blocked) != 0 {
		t.Errorf("Parse(nil) = %+v, want an empty record", got)
	}
}

// A record reads back with every host and every refused name.
func TestParseReadsTheRecord(t *testing.T) {
	data := fmt.Sprintf(`{"since": 100, "updated": 200, "hosts": {%q: {"bytes": 1200, "last_seen": 150, "since": 120}}, "blocked": {%q: {"attempts": 3, "last_seen": 190}}}`, fixture.Host, fixture.RefusedName)

	got, err := Parse([]byte(data))

	// Stop when the record fails to read
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	want := Record{
		Since:   100,
		Updated: 200,
		Hosts:   map[string]Host{fixture.Host: {Bytes: 1200, LastSeen: 150, Since: 120}},
		Blocked: map[string]Refusal{fixture.RefusedName: {Attempts: 3, LastSeen: 190}},
	}
	// Report a record read wrong
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
	}
}

// A record that fails to parse as JSON is turned away.
func TestParseTurnsAwayBrokenJSON(t *testing.T) {
	_, err := Parse([]byte("{"))

	// Report broken JSON read as a record
	if err == nil {
		t.Errorf("Parse({) = nil, want an error")
	}
}

// A host is dead once it was tracked for the whole window and stayed idle through it.
func TestDeadNeedsAWholeWindowWithoutTraffic(t *testing.T) {
	n := now.Unix()
	tests := []struct {
		name string
		host Host
		want bool
	}{
		{"never moved data in a whole window", Host{Since: n - 31*day}, true},
		{"last moved data before the window", Host{Since: n - 90*day, LastSeen: n - 31*day, Bytes: 10}, true},
		{"moved data inside the window", Host{Since: n - 90*day, LastSeen: n - 2*day, Bytes: 10}, false},
		{"tracked for less than a window", Host{Since: n - 2*day}, false},
		{"never tracked", Host{}, false},
	}
	// Judge every host
	for _, tt := range tests {
		got := tt.host.Dead(now)

		// Report a host judged wrong
		if got != tt.want {
			t.Errorf("%s: Dead() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Sizes read in decimal units, the way disk and network tools write them.
func TestSizeWritesDecimalUnits(t *testing.T) {
	tests := map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 kB", 84_000_000: "84.0 MB", 1_234_567_890: "1.2 GB"}
	// Write every size
	for bytes, want := range tests {
		// Report a size written wrong
		if got := Size(bytes); got != want {
			t.Errorf("Size(%d) = %q, want %q", bytes, got, want)
		}
	}
}

// A moment reads as how long ago it was, and a missing one as "never".
func TestAgoWritesHowLongAgo(t *testing.T) {
	n := now.Unix()
	tests := map[int64]string{0: "never", n - 10: "10s ago", n - 5*60: "5m ago", n - 3*3600: "3h ago", n - 4*day: "4d ago"}
	// Write every moment
	for at, want := range tests {
		// Report a moment written wrong
		if got := Ago(at, now); got != want {
			t.Errorf("Ago(%d) = %q, want %q", at, got, want)
		}
	}
}

// The refused names come most recent first.
func TestRefusedComesMostRecentFirst(t *testing.T) {
	r := Record{Blocked: map[string]Refusal{"old.example": {Attempts: 9, LastSeen: 10}, "new.example": {Attempts: 1, LastSeen: 30}, "mid.example": {Attempts: 2, LastSeen: 20}}}

	got := r.Refused()

	want := []string{"new.example", "mid.example", "old.example"}
	// Report an order that differs
	if diff := cmp.Diff(want, names(got)); diff != "" {
		t.Errorf("Refused() order (-want +got):\n%s", diff)
	}
}

// names lists the names of refusals in their order.
func names(rs []Refused) []string {
	out := make([]string, len(rs))
	// Take every name
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}
