// Package workspace edits the folders of an o3s home's VS Code workspace and keeps every comment and the layout the user gave it.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/hansehart/o3s/internal/atomicfile"
)

// ErrListed marks a folder the workspace lists already.
var ErrListed = errors.New("already in the workspace")

// Folder is one folder of the workspace.
type Folder struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

// Workspace is a workspace file as it holds it.
type Workspace struct {
	v hujson.Value
}

// Load reads a workspace file.
func Load(file string) (*Workspace, error) {
	data, err := os.ReadFile(file)
	// Hand back a workspace that fails to read
	if err != nil {
		return nil, err
	}
	v, err := hujson.Parse(data)
	// Hand back a workspace that holds no JSON with comments
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	// Turn away a workspace that holds no object
	if _, ok := v.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("%s holds no object", file)
	}
	return &Workspace{v: v}, nil
}

// Save writes the workspace back in a single step.
func (w *Workspace) Save(file string) error {
	return atomicfile.Write(file, w.v.Pack(), 0o644)
}

// Folders lists the workspace's folders that name a path, in its order.
func (w *Workspace) Folders() []Folder {
	var out []Folder
	// Read every folder
	for _, e := range w.find().Elements {
		o, ok := e.Value.(*hujson.Object)
		// Skip an entry that holds no object
		if !ok {
			continue
		}
		f := Folder{Name: field(o, "name"), Path: field(o, "path")}
		// Take a folder that names a path, leaving folders by URI aside
		if f.Path != "" {
			out = append(out, f)
		}
	}
	return out
}

// Add lists a folder at the end of the workspace, in the layout its folders carry.
func (w *Workspace) Add(f Folder) error {
	// Turn away a folder the workspace lists already
	for _, have := range w.Folders() {
		// Name the folder that holds the path
		if have.Path == f.Path {
			return fmt.Errorf("%s is %w", f.Path, ErrListed)
		}
	}
	unit := w.unit()
	arr := w.folders()
	indent := unit + unit
	// Take the indent the first folder carries
	if len(arr.Elements) > 0 {
		// Measure the indent on the folder's own line
		if _, have, ok := strings.CutLast(string(arr.Elements[0].BeforeExtra), "\n"); ok {
			indent = have
		}
	}
	data, err := json.MarshalIndent(f, indent, unit)
	// Hand back a folder JSON cannot hold, which strings never are
	if err != nil {
		return err
	}
	v, err := hujson.Parse(data)
	// Hand back JSON that fails to parse back, which JSON from the encoder never is
	if err != nil {
		return err
	}
	// Close the list on its own line where it held nothing
	if len(arr.Elements) == 0 && len(arr.AfterExtra) == 0 {
		arr.AfterExtra = hujson.Extra("\n" + strings.TrimSuffix(indent, unit))
	}
	v.BeforeExtra = hujson.Extra("\n" + indent)
	// Leave what follows the last folder on its line, such as a comment, with that folder
	if line, rest, ok := strings.CutLast(string(arr.AfterExtra), "\n"); ok {
		v.BeforeExtra = hujson.Extra(line + "\n" + indent)
		arr.AfterExtra = hujson.Extra("\n" + rest)
	}
	// Keep a trailing comma where the list ends in one
	if n := len(arr.Elements); n > 0 && arr.Elements[n-1].AfterExtra != nil {
		v.AfterExtra = hujson.Extra{}
	}
	arr.Elements = append(arr.Elements, v)
	return nil
}

// Remove drops the folder of a path and reports whether the workspace listed it.
func (w *Workspace) Remove(p string) bool {
	arr := w.find()
	// Find the folder of the path
	for i, e := range arr.Elements {
		o, ok := e.Value.(*hujson.Object)
		// Drop it, and close the list where it held nothing else
		if ok && field(o, "path") == p {
			arr.Elements = append(arr.Elements[:i], arr.Elements[i+1:]...)
			// Close an emptied list on the line it opens
			if len(arr.Elements) == 0 {
				arr.AfterExtra = nil
			}
			return true
		}
	}
	return false
}

// folders finds the workspace's list of folders and adds it where the workspace lacks it.
func (w *Workspace) folders() *hujson.Array {
	root := w.v.Value.(*hujson.Object)
	// Take the list the workspace already holds
	for _, m := range root.Members {
		// Take the list named folders
		if a, ok := m.Value.Value.(*hujson.Array); ok && name(m) == "folders" {
			return a
		}
	}
	a := &hujson.Array{}
	m := hujson.ObjectMember{
		Name:  hujson.Value{BeforeExtra: hujson.Extra("\n" + w.unit()), Value: hujson.String("folders")},
		Value: hujson.Value{BeforeExtra: hujson.Extra(" "), Value: a},
	}
	// Close the workspace on its own line where it held nothing
	if len(root.Members) == 0 && len(root.AfterExtra) == 0 {
		root.AfterExtra = hujson.Extra("\n")
	}
	root.Members = append(root.Members, m)
	return a
}

// find hands back the workspace's list of folders, and an empty one where it lacks it.
func (w *Workspace) find() *hujson.Array {
	// Look at every top-level member
	for _, m := range w.v.Value.(*hujson.Object).Members {
		// Take the list named folders
		if a, ok := m.Value.Value.(*hujson.Array); ok && name(m) == "folders" {
			return a
		}
	}
	return &hujson.Array{}
}

// unit is the indent the workspace's top-level members carry, a tab where it shows none, as VS Code writes it.
func (w *Workspace) unit() string {
	root := w.v.Value.(*hujson.Object)
	// Take the indent of the first top-level member
	if len(root.Members) > 0 {
		// Measure the indent on the member's own line
		if _, indent, ok := strings.CutLast(string(root.Members[0].Name.BeforeExtra), "\n"); ok && indent != "" {
			return indent
		}
	}
	return "\t"
}

// field reads a string member of an object, and empty where it holds none.
func field(o *hujson.Object, key string) string {
	// Find the member of that name
	for _, m := range o.Members {
		// Take its value where it is a string
		if l, ok := m.Value.Value.(hujson.Literal); ok && name(m) == key && l.Kind() == '"' {
			return l.String()
		}
	}
	return ""
}

// name reads a member's name.
func name(m hujson.ObjectMember) string {
	l, ok := m.Name.Value.(hujson.Literal)
	// Take a name that holds no string as empty
	if !ok {
		return ""
	}
	return l.String()
}
