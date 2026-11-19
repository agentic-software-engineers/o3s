package cli

import (
	"context"
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/restart"
)

// The states a part of the stack on this machine is in.
const (
	running = "running"
	stopped = "stopped"
	unknown = "unknown"
)

// part is one part of the home's stack on this machine, by its compose service, and where it stands.
type part struct {
	name, state string
}

// stack is where every part of the home's stack on this machine stands, read once.
type stack []part

// homeStack reads where every part of the home's stack stands: the cage, the gateway and the proxy.
func homeStack(ctx context.Context, dir string) stack {
	var s stack
	// Read where every part stands
	for _, name := range []string{"cage", "gateway", "proxy"} {
		ids, err := restart.Containers(ctx, dir, name)
		state := stopped
		// Name a part whose state stays unknown
		if err != nil {
			state = unknown
		}
		// Name a part that runs
		if len(ids) > 0 {
			state = running
		}
		s = append(s, part{name: name, state: state})
	}
	return s
}

// caged reports whether the cage itself runs.
func (s stack) caged() bool {
	return slices.Contains(s, part{name: "cage", state: running})
}

// up reports whether any part of the stack runs.
func (s stack) up() bool {
	return slices.ContainsFunc(s, func(p part) bool { return p.state == running })
}

// facts are the stack's parts as the menu shows them, each by its name.
func (s stack) facts() []menu.Fact {
	out := make([]menu.Fact, len(s))
	// Name every part with where it stands
	for i, p := range s {
		out[i] = menu.Fact{Label: strings.ToUpper(p.name[:1]) + p.name[1:], Value: p.state}
	}
	return out
}
