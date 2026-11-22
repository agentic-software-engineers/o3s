// Package traffic reads what the gateway records about each allowed host's traffic and the names it refused.
package traffic

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"time"

	"github.com/hansehart/o3s/internal/restart"
)

// path is where the gateway keeps its record, in its own state volume.
const path = "/state/traffic.json"

// Window is how long a host may stay idle before it counts as dead.
const Window = 30 * 24 * time.Hour

// Record is what the gateway recorded, with every moment in Unix seconds.
type Record struct {
	Since   int64              `json:"since"`
	Updated int64              `json:"updated"`
	Hosts   map[string]Host    `json:"hosts"`
	Blocked map[string]Refusal `json:"blocked"`
}

// Host is the traffic of one allowed host.
type Host struct {
	Bytes    int64 `json:"bytes"`
	LastSeen int64 `json:"last_seen"`
	// Since is when the gateway first counted the host.
	Since int64 `json:"since"`
}

// Refusal is how often a name the policy leaves out was asked for.
type Refusal struct {
	Attempts int64 `json:"attempts"`
	LastSeen int64 `json:"last_seen"`
}

// Refused is a refused name with how often it was asked for.
type Refused struct {
	Name string
	Refusal
}

// Read asks the home's running gateway for its record, and reports whether a gateway runs.
func Read(ctx context.Context, home string) (Record, bool, error) {
	ids, err := restart.Containers(ctx, home, "gateway")
	// Hand back an engine that fails to answer
	if err != nil {
		return Record{}, false, err
	}
	// Report a stack that is down
	if len(ids) == 0 {
		return Record{}, false, nil
	}
	out, err := exec.CommandContext(ctx, "docker", "exec", ids[0], "cat", path).Output() //nolint:gosec // the id comes from the engine itself, for the stack's own gateway
	// Read a gateway yet to write a record as one with an empty record
	if err != nil {
		return Record{}, true, nil
	}
	r, err := Parse(out)
	return r, true, err
}

// Parse reads a record the gateway wrote, and empty data as an empty record.
func Parse(data []byte) (Record, error) {
	var r Record
	// Read an empty file from the gateway as an empty record
	if len(data) == 0 {
		return r, nil
	}
	// Hand back a record that fails to parse as JSON
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, fmt.Errorf("read the traffic record: %w", err)
	}
	return r, nil
}

// Dead reports whether a host was tracked for a whole window and stayed idle through it.
func (h Host) Dead(now time.Time) bool {
	window := int64(Window / time.Second)
	return h.Since > 0 && now.Unix()-h.Since >= window && now.Unix()-h.LastSeen >= window
}

// Refused lists the refused names, the most recently refused first.
func (r Record) Refused() []Refused {
	out := make([]Refused, 0, len(r.Blocked))
	// Take every refused name
	for name, ref := range r.Blocked {
		out = append(out, Refused{Name: name, Refusal: ref})
	}
	slices.SortFunc(out, func(a, b Refused) int {
		return cmp.Or(cmp.Compare(b.LastSeen, a.LastSeen), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// Size writes a number of bytes in decimal units.
func Size(bytes int64) string {
	units := []string{"kB", "MB", "GB", "TB", "PB"}
	// Write a size below a kilobyte in bytes
	if bytes < 1000 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes) / 1000
	unit := 0
	// Step up a unit while the value fills the next one
	for value >= 1000 && unit < len(units)-1 {
		value /= 1000
		unit++
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// Ago writes how long ago a moment was, in its largest whole unit, and "never" for an unset moment.
func Ago(at int64, now time.Time) string {
	// Name a moment that stays unset
	if at <= 0 {
		return "never"
	}
	d := now.Sub(time.Unix(at, 0))
	// Write the largest whole unit the time spans
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
