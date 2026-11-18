package vm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// IDFile is where a host folder keeps the ID of its home's VM, which travels with the folder.
var IDFile = filepath.Join(".o3s", "id")

// ErrNoVM marks a folder whose home has no VM.
var ErrNoVM = errors.New("no o3s home with a VM here, run o3s init")

// releaseDir is where the VM keeps the files that install o3s in it.
const releaseDir = "/tmp/o3s-release"

// Find names the VM of the home in a folder.
func Find(dir string) (string, error) {
	id, err := os.ReadFile(filepath.Join(dir, IDFile))
	// Point a folder without a VM at the command that makes one
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoVM
	}
	// Hand back any other failure to read
	if err != nil {
		return "", err
	}
	return "o3s-" + strings.TrimSpace(string(id)), nil
}

// Bind names the VM of the home in a folder, giving the folder a new ID where it holds none.
func Bind(dir string) (string, error) {
	name, err := Find(dir)
	// Keep the ID the folder already holds
	if !errors.Is(err, ErrNoVM) {
		return name, err
	}
	raw := make([]byte, 6)
	// Hand back an ID that fails to draw
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	path := filepath.Join(dir, IDFile)
	// Hand back a folder that fails to hold the ID's folder
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	// Hand back an ID that stays unwritten
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return "o3s-" + id, nil
}

// Install puts o3s into a home's VM from a folder of release files, with the installer a bare machine runs.
func (l Lima) Install(ctx context.Context, name, bundle string, out io.Writer) error {
	// Hand back release files that fail to reach the VM
	if err := l.run(ctx, nil, out, out, "copy", "-r", bundle, name+":"+filepath.ToSlash(filepath.Dir(releaseDir))+"/"); err != nil {
		return err
	}
	return l.Exec(ctx, name, nil, out, out, "O3S_RELEASE_URL=file://"+releaseDir, "sh", releaseDir+"/install.sh")
}

// Installed reports whether a home's VM runs o3s at a given version.
func (l Lima) Installed(ctx context.Context, name, version string) bool {
	var out strings.Builder
	err := l.Exec(ctx, name, nil, &out, io.Discard, "o3s", "--version")
	return err == nil && strings.TrimSpace(out.String()) == "o3s version "+version
}

// Init makes the home inside a home's VM, in the folder o3s suggests for a home, with its output on out.
func (l Lima) Init(ctx context.Context, name string, in io.Reader, out, errOut io.Writer) error {
	return l.Exec(ctx, name, in, out, errOut, "sh", "-c", `mkdir -p "$HOME/o3s" && cd "$HOME/o3s" && o3s init --guest`)
}
