package service_test

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/proxy"
	"github.com/hansehart/o3s/internal/service"
)

// livePoll is how often a test's live view looks for changed files.
const livePoll = 10 * time.Millisecond

// syncWriter lets the service's goroutines write the log one at a time.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write writes the bytes while every other write waits.
func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}

// aHome is a home's files in a folder of their own, with the hosts the changes allow and the secrets given.
type aHome struct {
	files service.Files
	log   *bytes.Buffer
}

// homeWith writes a home whose allowlist takes the changes and whose secrets file holds the lines.
func homeWith(t *testing.T, changes []config.Change, secretLines ...string) aHome {
	t.Helper()
	dir := t.TempDir()
	h := aHome{files: service.Files{Config: filepath.Join(dir, config.File), Secrets: filepath.Join(dir, "secrets.env")}, log: &bytes.Buffer{}}
	h.allow(t, changes...)
	h.secrets(t, secretLines...)
	return h
}

// allow rewrites the allowlist to hold the hosts the changes name.
func (h aHome) allow(t *testing.T, changes ...config.Change) {
	t.Helper()
	c := config.New()
	// Add each host the test names
	for _, ch := range changes {
		// Stop when a change breaks the allowlist's rules
		if _, err := c.Allow(ch); err != nil {
			t.Fatalf("Setup: Allow(%s) = %v", ch.Name, err)
		}
	}
	// Stop when the allowlist stays unwritten
	if err := c.Save(filepath.Dir(h.files.Config)); err != nil {
		t.Fatalf("Setup: Save = %v", err)
	}
}

// secrets rewrites the secrets file to hold the lines.
func (h aHome) secrets(t *testing.T, lines ...string) {
	t.Helper()
	// Stop when the secrets file stays unwritten
	if err := os.WriteFile(h.files.Secrets, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("Setup: WriteFile = %v", err)
	}
}

// logger writes the service's log lines to the home's buffer.
func (h aHome) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&syncWriter{w: h.log}, nil))
}

// open is a host the allowlist holds on the HTTPS port.
func open(name string) config.Change {
	return config.Change{Name: name, Ports: []int{fixture.Port}}
}

// carrying is a host the allowlist holds on the HTTPS port with a secret in the sites.
func carrying(name, secret string, sites ...string) config.Change {
	return config.Change{Name: name, Ports: []int{fixture.Port}, Secret: secret, Inject: sites}
}

// A host whose end time has passed is refused by name and by port, while an open one stays allowed.
func TestSnapshotRefusesAnExpiredHost(t *testing.T) {
	expired := open(fixture.OtherHost)
	expired.Expires = fixture.LongAgoText
	timed := open(fixture.ThirdHost)
	timed.Expires = fixture.FarAheadText
	h := homeWith(t, []config.Change{open(fixture.Host), expired, timed})

	s := service.Load(h.files, h.logger())

	got := map[string][]bool{}
	// Ask about each host by port and by name
	for _, n := range []string{fixture.Host, fixture.OtherHost, fixture.ThirdHost} {
		got[n] = []bool{s.Allows(n, fixture.Port), s.Lists(n)}
	}
	want := map[string][]bool{fixture.Host: {true, true}, fixture.OtherHost: {false, false}, fixture.ThirdHost: {true, true}}
	// Require the expired host refused both ways
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("allowed and listed (-want +got):\n%s", diff)
	}
}

// A config that fails to parse refuses every host and says why in the log.
func TestSnapshotRefusesEverythingOnABrokenConfig(t *testing.T) {
	h := homeWith(t, []config.Change{open(fixture.Host)})
	_ = os.WriteFile(h.files.Config, []byte("{"), 0o600)

	s := service.Load(h.files, h.logger())

	// Require every host refused and an error in the log
	if s.Allows(fixture.Host, fixture.Port) || s.Lists(fixture.Host) || !strings.Contains(h.log.String(), `"level":"ERROR"`) {
		t.Errorf("broken config allows %s, or the log lacks an error:\n%s", fixture.Host, h.log.String())
	}
}

// A home whose config is missing refuses every host.
func TestSnapshotRefusesEverythingWithoutAConfig(t *testing.T) {
	h := homeWith(t, []config.Change{open(fixture.Host)})
	_ = os.Remove(h.files.Config)

	s := service.Load(h.files, h.logger())

	// Require every host refused
	if s.Allows(fixture.Host, fixture.Port) || s.Lists(fixture.Host) {
		t.Errorf("a home without config allows %s", fixture.Host)
	}
}

// A host's secret comes from its own entry first, then from the nearest wildcard above it, into the sites the entry
// names or the header while it leaves them empty.
func TestSnapshotFindsEachHostsSecret(t *testing.T) {
	deep := "*.api." + fixture.OtherHost
	expired := carrying(fixture.NewHost, fixture.Secret)
	expired.Expires = fixture.LongAgoText
	h := homeWith(t, []config.Change{
		carrying(fixture.Host, fixture.Secret, proxy.Query),
		carrying(fixture.OtherWildcard, fixture.OtherSecret),
		carrying(deep, fixture.NewSecret),
		open(fixture.ThirdHost),
		expired,
	}, fixture.Secret+"="+fixture.TokenValue, fixture.OtherSecret+"="+fixture.SecretValue)
	s := service.Load(h.files, h.logger())
	type found struct {
		Secret proxy.Secret
		OK     bool
	}

	got := map[string]found{}
	// Look up each host
	for _, n := range []string{fixture.Host, "www." + fixture.OtherHost, "eu.api." + fixture.OtherHost, "cdn.eu.api." + fixture.OtherHost, fixture.ThirdHost, fixture.NewHost} {
		sec, ok := s.Secret(n)
		got[n] = found{sec, ok}
	}

	want := map[string]found{
		fixture.Host:                      {proxy.Secret{Value: fixture.TokenValue, Sites: []string{proxy.Query}}, true},
		"www." + fixture.OtherHost:        {proxy.Secret{Value: fixture.SecretValue, Sites: []string{proxy.Header}}, true},
		"eu.api." + fixture.OtherHost:     {proxy.Secret{Value: "", Sites: []string{proxy.Header}}, true},
		"cdn.eu.api." + fixture.OtherHost: {proxy.Secret{Value: "", Sites: []string{proxy.Header}}, true},
		fixture.ThirdHost:                 {},
		fixture.NewHost:                   {},
	}
	// Require each host's own secret, an empty value for one still waiting, and a sealed answer for hosts free of
	// secrets or past their end
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("secrets by host (-want +got):\n%s", diff)
	}
}

// eventually waits for the condition to hold, failing the test once the wait runs out.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	// Look again until the condition holds or the time runs out
	for !cond() {
		// Stop once the time runs out
		if time.Now().After(deadline) {
			t.Fatalf("%s within 3s, want it to", what)
		}
		time.Sleep(livePoll)
	}
}

// live starts a live view of the home that looks for changes on every poll until the test ends.
func live(t *testing.T, h aHome) *service.Live {
	t.Helper()
	l := service.NewLive(h.files, h.logger())
	go l.Run(t.Context(), livePoll)
	return l
}

// A host added to the allowlist on disk is allowed once the live view looks again.
func TestLiveFollowsAnAllowlistChange(t *testing.T) {
	h := homeWith(t, nil)
	l := live(t, h)

	h.allow(t, open(fixture.Host))

	// Require the new host allowed
	eventually(t, fixture.Host+" allowed", func() bool { return l.Allows(fixture.Host, fixture.Port) })
}

// A secret set on disk reaches the host it belongs to once the live view looks again.
func TestLiveFollowsASecretChange(t *testing.T) {
	h := homeWith(t, []config.Change{carrying(fixture.Host, fixture.Secret)})
	l := live(t, h)

	h.secrets(t, fixture.Secret+"="+fixture.TokenValue)

	// Require the new value
	eventually(t, "the new secret value", func() bool {
		sec, _ := l.Secret(fixture.Host)
		return sec.Value == fixture.TokenValue
	})
}

// A config that breaks after start refuses every host from the next look on.
func TestLiveRefusesEverythingOnceTheConfigBreaks(t *testing.T) {
	h := homeWith(t, []config.Change{open(fixture.Host)})
	l := live(t, h)

	_ = os.WriteFile(h.files.Config, []byte("{"), 0o600)

	// Require the host refused
	eventually(t, fixture.Host+" refused", func() bool { return !l.Allows(fixture.Host, fixture.Port) })
}

// A secret written into the allowlist by hand with its sites left out goes into the header.
func TestSnapshotSendsAHandWrittenSecretInTheHeader(t *testing.T) {
	h := homeWith(t, nil, fixture.Secret+"="+fixture.TokenValue)
	c := config.New()
	c.Hosts = append(c.Hosts, config.Host{Name: fixture.Host, Ports: []int{fixture.Port}, Secret: fixture.Secret, By: []string{"user"}})
	// Stop when the allowlist stays unwritten
	if err := c.Save(filepath.Dir(h.files.Config)); err != nil {
		t.Fatalf("Setup: Save = %v", err)
	}

	got, ok := service.Load(h.files, h.logger()).Secret(fixture.Host)

	// Require the secret in the header
	if diff := cmp.Diff(proxy.Secret{Value: fixture.TokenValue, Sites: []string{proxy.Header}}, got); diff != "" || !ok {
		t.Errorf("Secret(%s) = %v (-want +got):\n%s", fixture.Host, ok, diff)
	}
}
