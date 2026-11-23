package host

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hansehart/o3s/internal/atomicfile"
	"github.com/hansehart/o3s/internal/paths"
)

// Marker reads the placeholder a home's cage sends in place of every secret, and empty for a home that lacks one.
func Marker(home string) string {
	return markerIn(envFile(home))
}

// envFile is where a home keeps the values compose reads.
func envFile(home string) string {
	return filepath.Join(home, ".devcontainer", ".env")
}

// Env writes the values compose reads at create time, the cage's AppArmor profile among them, keeping the marker the cage already holds.
func Env(home, appArmorProfile string) error {
	path := envFile(home)
	marker := markerIn(path)
	// Draw a new marker where the home lacks one
	if marker == "" {
		b := make([]byte, 32)
		// Hand back randomness that fails to draw
		if _, err := rand.Read(b); err != nil {
			return err
		}
		marker = "o3s-" + hex.EncodeToString(b)
	}
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "# Generated on every start. Put your own variables in devcontainer.json.")
	fmt.Fprintln(&buf)
	fmt.Fprintf(&buf, "O3S_MARKER=%s\n", marker)
	fmt.Fprintf(&buf, "O3S_PROXY_DIR=%s\nO3S_SOCKETS=%s\n", paths.Proxy(), paths.CageSockets(home))
	fmt.Fprintf(&buf, "O3S_APPARMOR_PROFILE=%s\n", appArmorProfile)
	return atomicfile.Write(path, buf.Bytes(), 0o644)
}

// markerIn reads the marker an env file holds, and empty where the file lacks one.
func markerIn(path string) string {
	data, err := os.ReadFile(path)
	// Read a missing file as one that lacks a marker
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	// Look at every line for the marker
	for sc.Scan() {
		// Take the marker's value
		if value, ok := strings.CutPrefix(sc.Text(), "O3S_MARKER="); ok {
			return value
		}
	}
	return ""
}
