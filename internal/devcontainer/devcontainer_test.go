package devcontainer_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/devcontainer"
	"github.com/hansehart/o3s/internal/fixture"
)

// The features the tests set, by package, with the keys their tags give them and the folder o3s hands the first.
var (
	tool         = fixture.Store + "/" + fixture.Feature
	other        = fixture.Store + "/" + fixture.OtherFeature
	toolOnMajor  = tool + ":" + fixture.Major
	toolOnNewer  = tool + ":" + fixture.NewerMajor
	otherOnNewer = other + ":" + fixture.NewerMajor
	toolsOnNewer = tool + "s:" + fixture.NewerMajor
	toolState    = "/home/ubuntu/features/" + tool
)

// aConfig is a config as the user keeps it, with comments, four-space indent and no features.
const aConfig = `// The cage
{
    // Name for the dev container
    "name": "o3s",
    // Additional software layered onto the o3s image
    "features": {},
    // Non-root user VS Code connects as
    "remoteUser": "ubuntu"
}
`

// aHomeWith hands a test an o3s home whose config holds the text given.
func aHomeWith(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	// Stop when the stack folder fails to appear
	if err := os.MkdirAll(filepath.Join(home, ".devcontainer"), 0o750); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the config fails to write
	if err := os.WriteFile(filepath.Join(home, devcontainer.File), []byte(text), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return home
}

// edited loads a home's config, edits it, saves it, and hands back the text it then holds.
func edited(t *testing.T, text string, edit func(*devcontainer.Config)) string {
	t.Helper()
	home := aHomeWith(t, text)
	c, err := devcontainer.Load(home)
	// Stop when the config fails to load
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	edit(c)
	// Stop when the config fails to save
	if err := c.Save(home); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, devcontainer.File))
	// Stop when the saved config fails to read
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	return string(data)
}

// A feature joins the features in the config's own layout, its comments kept.
func TestSetFeatureKeepsTheLayout(t *testing.T) {
	got := edited(t, aConfig, func(c *devcontainer.Config) {
		c.SetFeature(tool, toolOnMajor, map[string]string{"stateDir": toolState})
		c.SetFeature(other, otherOnNewer, nil)
	})

	want := fmt.Sprintf(`// The cage
{
    // Name for the dev container
    "name": "o3s",
    // Additional software layered onto the o3s image
    "features": {
        %[1]q: {
            "stateDir": %[2]q
        },
        %[3]q: {}
    },
    // Non-root user VS Code connects as
    "remoteUser": "ubuntu"
}
`, toolOnMajor, toolState, otherOnNewer)
	// Report a layout that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

// A feature set again under another tag takes the place of the one it updates.
func TestSetFeatureReplacesTheSamePackageInPlace(t *testing.T) {
	text := fmt.Sprintf(`{
    "features": {
        // The tool
        %[1]q: {},
        %[2]q: {}
    }
}
`, toolOnMajor, otherOnNewer)
	got := edited(t, text, func(c *devcontainer.Config) {
		c.SetFeature(tool, toolOnNewer, nil)
	})

	want := fmt.Sprintf(`{
    "features": {
        // The tool
        %[1]q: {},
        %[2]q: {}
    }
}
`, toolOnNewer, otherOnNewer)
	// Report a config that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

// Removing a feature drops it whatever tag it carries, and reports whether it was there.
func TestRemoveFeatureDropsThePackage(t *testing.T) {
	text := fmt.Sprintf(`{
    "features": {
        %[1]q: {},
        %[2]q: {}
    }
}
`, toolOnMajor, toolsOnNewer)
	var removed, again bool
	got := edited(t, text, func(c *devcontainer.Config) {
		removed = c.RemoveFeature(tool)
		again = c.RemoveFeature(tool)
	})

	want := fmt.Sprintf(`{
    "features": {
        %[1]q: {}
    }
}
`, toolsOnNewer)
	// Report a config that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
	// Require the first removal alone to report the feature
	if !removed || again {
		t.Errorf("RemoveFeature twice = %v, %v, want true, false", removed, again)
	}
}

// Env lands in the container's env, which the config gains where it lacks it.
func TestSetEnvAddsTheContainersEnv(t *testing.T) {
	got := edited(t, aConfig, func(c *devcontainer.Config) {
		c.SetEnv("EXAMPLE_MODE", "lax")
		c.SetEnv("EXAMPLE_MODE", "strict")
		c.SetEnv("EXAMPLE_HOME", "/home/ubuntu/features")
	})

	want := `// The cage
{
    // Name for the dev container
    "name": "o3s",
    // Additional software layered onto the o3s image
    "features": {},
    // Non-root user VS Code connects as
    "remoteUser": "ubuntu",
    "containerEnv": {
        "EXAMPLE_MODE": "strict",
        "EXAMPLE_HOME": "/home/ubuntu/features"
    }
}
`
	// Report a config that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

// Removing env drops the variable and keeps the rest.
func TestRemoveEnvDropsTheVariable(t *testing.T) {
	text := `{
    "containerEnv": {
        "EXAMPLE_MODE": "strict",
        "EXAMPLE_HOME": "/home/ubuntu/features"
    }
}
`
	got := edited(t, text, func(c *devcontainer.Config) { c.RemoveEnv("EXAMPLE_MODE") })

	want := `{
    "containerEnv": {
        "EXAMPLE_HOME": "/home/ubuntu/features"
    }
}
`
	// Report a config that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

// The features come back as the config names them, in its order.
func TestFeaturesListsTheConfigsFeatures(t *testing.T) {
	home := aHomeWith(t, fmt.Sprintf(`{"features": {%[1]q: {}, %[2]q: {"mode": "strict"}}}`, toolOnMajor, otherOnNewer))
	c, err := devcontainer.Load(home)
	// Stop when the config fails to load
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	got := c.Features()

	// Report features that differ
	if diff := cmp.Diff([]string{toolOnMajor, otherOnNewer}, got); diff != "" {
		t.Errorf("Features() (-want +got):\n%s", diff)
	}
}

// A config that holds no object is turned away.
func TestLoadTurnsAwayABrokenConfig(t *testing.T) {
	home := aHomeWith(t, `{"features": `)

	_, err := devcontainer.Load(home)

	// Require an error for a broken config
	if err == nil {
		t.Errorf("Load() = nil, want an error")
	}
}

// A feature moved to another tag keeps the options the user gave it.
func TestRetagFeatureKeepsTheOptions(t *testing.T) {
	text := fmt.Sprintf(`{
    "features": {
        %[1]q: {
            "mode": "strict"
        }
    }
}
`, toolOnMajor)
	var moved bool
	got := edited(t, text, func(c *devcontainer.Config) {
		moved = c.RetagFeature(tool, toolOnNewer)
	})

	want := fmt.Sprintf(`{
    "features": {
        %[1]q: {
            "mode": "strict"
        }
    }
}
`, toolOnNewer)
	// Report a config that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
	// Require the move to be reported
	if !moved {
		t.Errorf("RetagFeature() = false, want true")
	}
}

// A variable's value comes back as the config sets it.
func TestEnvReadsAVariable(t *testing.T) {
	home := aHomeWith(t, `{"containerEnv": {"EXAMPLE_MODE": "strict"}}`)
	c, err := devcontainer.Load(home)
	// Stop when the config fails to load
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	value, ok := c.Env("EXAMPLE_MODE")
	_, absent := c.Env("EXAMPLE_HOME")

	// Require the value set, and none for a variable the config lacks
	if value != "strict" || !ok || absent {
		t.Errorf("Env() = %q, %v and %v, want strict, true and false", value, ok, absent)
	}
}

// Reading a config leaves it as it was.
func TestReadingLeavesTheConfigAsItWas(t *testing.T) {
	text := "{\n    \"name\": \"o3s\"\n}\n"

	got := edited(t, text, func(c *devcontainer.Config) {
		c.Features()
		c.Env("EXAMPLE_MODE")
		c.RemoveEnv("EXAMPLE_MODE")
		c.RetagFeature(tool, toolOnNewer)
	})

	// Report a config that changed
	if got != text {
		t.Errorf("config after reading = %q, want %q", got, text)
	}
}
