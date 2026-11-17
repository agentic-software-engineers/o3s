package release_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/release"
)

// Each binary's name is its channel: o3s-rc follows release candidates, o3s-<version> stays, and o3s-latest or a plain o3s follows stable releases.
func TestChannelOfReadsTheBinarysName(t *testing.T) {
	tests := []struct {
		path    string
		channel release.Channel
		pinned  string
	}{
		{"/home/ubuntu/.local/bin/o3s-latest", release.Stable, ""},
		{"/home/ubuntu/.local/bin/o3s", release.Stable, ""},
		{"/home/ubuntu/.local/bin/o3s-rc", release.Candidates, ""},
		{"/home/ubuntu/.local/bin/o3s-" + fixture.Version, release.Pinned, fixture.Version},
		{"/home/ubuntu/.local/bin/o3s-" + fixture.NextCandidate, release.Pinned, fixture.NextCandidate},
	}
	// Read every name
	for _, tt := range tests {
		channel, pinned := release.ChannelOf(tt.path)

		// Report a channel read otherwise
		if channel != tt.channel || pinned != tt.pinned {
			t.Errorf("ChannelOf(%s) = %v, %q, want %v, %q", tt.path, channel, pinned, tt.channel, tt.pinned)
		}
	}
}

// feed is a releases feed whose newest entry is not the highest release, with a tag of no release among them.
const feed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry><link rel="alternate" type="text/html" href="https://github.com/hansehart/o3s/releases/tag/v1.0.0"/></entry>
  <entry><link rel="alternate" type="text/html" href="https://github.com/hansehart/o3s/releases/tag/v1.1.0-rc.10"/></entry>
  <entry><link rel="alternate" type="text/html" href="https://github.com/hansehart/o3s/releases/tag/v1.1.0-rc.2"/></entry>
  <entry><link rel="alternate" type="text/html" href="https://github.com/hansehart/o3s/releases/tag/nightly"/></entry>
</feed>`

// The newest release counts release candidates, and orders them by semver rather than by the feed.
func TestNewestReadsTheFeed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /o3s/releases.atom", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(feed))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	t.Setenv("O3S_RELEASES_URL", s.URL+"/o3s/releases")

	got, err := release.Newest(t.Context())

	// Require the highest release in the feed
	if err != nil || got != "1.1.0-rc.10" {
		t.Errorf("Newest() = %q, %v, want 1.1.0-rc.10", got, err)
	}
}
