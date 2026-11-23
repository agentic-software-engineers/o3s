package cli

import (
	"context"
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/hostd"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/paths"
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

// homeStack reads where every part of the home's stack stands: the cage, and the host process it reaches the internet
// through.
func homeStack(ctx context.Context, dir string) stack {
	ids, err := restart.Containers(ctx, dir, "cage")
	cage := stopped
	// Name a cage whose state stays unknown
	if err != nil {
		cage = unknown
	}
	// Name a cage that runs
	if len(ids) > 0 {
		cage = running
	}
	return stack{{name: "cage", state: cage}, {name: "host", state: hostState(ctx, dir)}}
}

// hostState reads whether the home's host process answers.
func hostState(ctx context.Context, dir string) string {
	// Name a host process that answers as running
	if _, err := hostd.GetInfo(ctx, hostd.Place{Sockets: paths.Sockets(dir)}.Control()); err == nil {
		return running
	}
	return stopped
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
