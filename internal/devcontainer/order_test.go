package devcontainer_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/devcontainer"
	"github.com/hansehart/o3s/internal/fixture"
)

// third is a third feature the order tests place.
var third = fixture.Store + "/" + fixture.ThirdFeature

// The order joins the config in its layout, takes the place of the order it held, and leaves with the last feature.
func TestSetInstallOrderKeepsTheLayout(t *testing.T) {
	got := edited(t, aConfig, func(c *devcontainer.Config) {
		c.SetInstallOrder([]string{other})
		c.SetInstallOrder([]string{tool, other})
	})

	want := fmt.Sprintf(`// The cage
{
    // Name for the dev container
    "name": "o3s",
    // Additional software layered onto the o3s image
    "features": {},
    // Non-root user VS Code connects as
    "remoteUser": "ubuntu",
    "overrideFeatureInstallOrder": [
        %q,
        %q
    ]
}
`, tool, other)
	// Report a layout that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
	// Report an order that stays once the features are all gone
	if got := edited(t, want, func(c *devcontainer.Config) { c.SetInstallOrder(nil) }); got != aConfig {
		t.Errorf("config after SetInstallOrder(nil) = %s, want the config without an order", got)
	}
}

// The order reads back as the config holds it, and as empty where the config lacks one.
func TestInstallOrderReadsTheOrder(t *testing.T) {
	var got, none []string
	edited(t, aConfig, func(c *devcontainer.Config) {
		none = c.InstallOrder()
		c.SetInstallOrder([]string{other, tool})
		got = c.InstallOrder()
	})

	// Report an order read from a config that lacks one
	if none != nil {
		t.Errorf("InstallOrder() = %v, want none", none)
	}
	// Report an order that differs
	if diff := cmp.Diff([]string{other, tool}, got); diff != "" {
		t.Errorf("InstallOrder() (-want +got):\n%s", diff)
	}
}

// Features keep the order they stand in, each moved just as far as it must to follow the ones it installs after.
func TestOrderedMovesAFeatureBehindWhatItFollows(t *testing.T) {
	tests := []struct {
		name            string
		features, first []string
		after           map[string][]string
		want            []string
	}{
		{"as they stand", []string{tool, other, third}, nil, nil, []string{tool, other, third}},
		{"behind what it follows", []string{tool, other, third}, nil, map[string][]string{tool: {third}}, []string{other, third, tool}},
		{"named first", []string{tool, other, third}, []string{third, other}, nil, []string{third, other, tool}},
		{"named first, still followed", []string{tool, other, third}, []string{other}, map[string][]string{tool: {third}}, []string{other, third, tool}},
		{"in a circle", []string{tool, other}, nil, map[string][]string{tool: {other}, other: {tool}}, []string{tool, other}},
		{"following one not installed", []string{tool}, nil, map[string][]string{tool: {other}}, []string{tool}},
	}
	// Check every case
	for _, tt := range tests {
		got, err := devcontainer.Ordered(tt.features, tt.after, tt.first)

		// Report an order that differs or fails
		if err != nil || !cmp.Equal(tt.want, got) {
			t.Errorf("%s: Ordered() = %v, %v, want %v", tt.name, got, err, tt.want)
		}
	}
}

// A feature named before one it installs after, or one left uninstalled, is turned away with both named.
func TestOrderedTurnsAwayAnOrderThatBreaksAFeature(t *testing.T) {
	_, err := devcontainer.Ordered([]string{tool, other}, map[string][]string{tool: {other}}, []string{tool})

	// Report an order the features break, let through or left unnamed
	if !errors.Is(err, devcontainer.ErrOrder) || err.Error() != fmt.Sprintf("invalid install order: %s installs after %s, so %s comes first", tool, other, other) {
		t.Errorf("Ordered() = %v, want %s named before %s", err, other, tool)
	}
	_, err = devcontainer.Ordered([]string{tool}, nil, []string{other})
	// Report a feature the home lacks that the order takes
	if !errors.Is(err, devcontainer.ErrOrder) {
		t.Errorf("Ordered() = %v, want %v", err, devcontainer.ErrOrder)
	}
}
