package release

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Channel is the releases a binary updates to, named by its file.
type Channel int

const (
	// Stable follows the latest stable release, the channel of o3s-latest and of a plain o3s.
	Stable Channel = iota
	// Candidates follows the newest release, a release candidate or the stable release it leads to, the channel of o3s-rc.
	Candidates
	// Pinned keeps its version, the channel of o3s-<version>.
	Pinned
)

// String names a channel by its binary's suffix.
func (c Channel) String() string {
	// Name the channel
	switch c {
	case Candidates:
		return "rc"
	case Pinned:
		return "pinned"
	case Stable:
	}
	return "latest"
}

// ChannelOf reads a binary's channel from its file's name, and the version a pinned binary keeps.
func ChannelOf(path string) (Channel, string) {
	name := filepath.Base(path)
	// Read the channel the name ends in
	switch suffix := strings.TrimPrefix(name, "o3s-"); {
	case name == "o3s-rc":
		return Candidates, ""
	case suffix != name && IsRelease(suffix):
		return Pinned, suffix
	}
	return Stable, ""
}

// tagged matches a release's tag in the releases feed.
var tagged = regexp.MustCompile(`/releases/tag/v([0-9A-Za-z.\-]+)"`)

// Newest reads the highest release in the releases feed, release candidates included.
func Newest(ctx context.Context) (string, error) {
	data, err := get(ctx, base()+".atom")
	// Hand back a feed out of reach
	if err != nil {
		return "", err
	}
	newest := ""
	// Take every tag that names a release, keeping the highest
	for _, m := range tagged.FindAllStringSubmatch(string(data), -1) {
		// Keep a release above the highest so far
		if IsRelease(m[1]) && (newest == "" || Newer(m[1], newest)) {
			newest = m[1]
		}
	}
	// Turn away a feed that lacks a release
	if newest == "" {
		return "", fmt.Errorf("the releases feed names no release")
	}
	return newest, nil
}
