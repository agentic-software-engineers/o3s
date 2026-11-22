package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// template is the workspace a new home starts with, as init writes it.
const template = `{
	"folders": [
		{
			"name": "o3s",
			"path": "/home/ubuntu/o3s"
		}
	]
}
`

// roundTrip writes a workspace, edits it, and hands back what the file then holds.
func roundTrip(t *testing.T, in string, edit func(*Workspace)) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "o3s.code-workspace")
	// Stop when the workspace stays unwritten
	if err := os.WriteFile(file, []byte(in), 0o600); err != nil {
		t.Fatalf("Setup: WriteFile = %v", err)
	}
	w, err := Load(file)
	// Stop when the workspace fails to load
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	edit(w)
	// Stop when the workspace fails to save
	if err := w.Save(file); err != nil {
		t.Fatalf("Save = %v", err)
	}
	data, err := os.ReadFile(file)
	// Stop when the workspace fails to read back
	if err != nil {
		t.Fatalf("ReadFile = %v", err)
	}
	return string(data)
}

// A folder added lands at the end, in the tabs VS Code writes, and leaves again cleanly.
func TestAddAndRemoveKeepTheLayout(t *testing.T) {
	added := roundTrip(t, template, func(w *Workspace) {
		// Stop when the folder is turned away
		if err := w.Add(Folder{Name: "app", Path: "/home/ubuntu/projects/app"}); err != nil {
			t.Fatalf("Add = %v", err)
		}
	})
	want := `{
	"folders": [
		{
			"name": "o3s",
			"path": "/home/ubuntu/o3s"
		},
		{
			"name": "app",
			"path": "/home/ubuntu/projects/app"
		}
	]
}
`
	// Report a layout the folder changed
	if diff := cmp.Diff(want, added); diff != "" {
		t.Errorf("Add mismatch (-want +got):\n%s", diff)
	}
	removed := roundTrip(t, added, func(w *Workspace) {
		// Report a folder the workspace failed to list
		if !w.Remove("/home/ubuntu/projects/app") {
			t.Errorf("Remove = false, want true")
		}
	})
	// Report a workspace the removal left changed
	if diff := cmp.Diff(template, removed); diff != "" {
		t.Errorf("Remove mismatch (-want +got):\n%s", diff)
	}
}

// Comments and a trailing comma the user wrote stay where they were.
func TestAddKeepsCommentsAndTrailingCommas(t *testing.T) {
	in := `{
  // my repos
  "folders": [
    { "path": "/home/ubuntu/o3s" }, // the home
  ],
}
`
	got := roundTrip(t, in, func(w *Workspace) {
		// Stop when the folder is turned away
		if err := w.Add(Folder{Name: "app", Path: "/home/ubuntu/projects/app"}); err != nil {
			t.Fatalf("Add = %v", err)
		}
	})
	want := `{
  // my repos
  "folders": [
    { "path": "/home/ubuntu/o3s" }, // the home
    {
      "name": "app",
      "path": "/home/ubuntu/projects/app"
    },
  ],
}
`
	// Report a comment or comma the folder lost
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Add mismatch (-want +got):\n%s", diff)
	}
}

// A workspace that lacks folders gets its list, and a path listed already is turned away.
func TestAddStartsTheListAndTurnsAwayADuplicate(t *testing.T) {
	got := roundTrip(t, "{}\n", func(w *Workspace) {
		// Stop when the folder is turned away
		if err := w.Add(Folder{Path: "/home/ubuntu/projects/app"}); err != nil {
			t.Fatalf("Add = %v", err)
		}
		// Report a duplicate the workspace took
		if err := w.Add(Folder{Path: "/home/ubuntu/projects/app"}); !errors.Is(err, ErrListed) {
			t.Errorf("Add(duplicate) = %v, want %v", err, ErrListed)
		}
		// Report a folder list read wrong
		if diff := cmp.Diff([]Folder{{Path: "/home/ubuntu/projects/app"}}, w.Folders()); diff != "" {
			t.Errorf("Folders mismatch (-want +got):\n%s", diff)
		}
	})
	want := "{\n\t\"folders\": [\n\t\t{\n\t\t\t\"path\": \"/home/ubuntu/projects/app\"\n\t\t}\n\t]\n}\n"
	// Report a list laid out wrong
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Add mismatch (-want +got):\n%s", diff)
	}
}
