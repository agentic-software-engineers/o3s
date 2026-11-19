package tui

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/menu/menutest"
)

// fuzzKeys are the keys a fuzzed byte presses, every key the menu acts on and a few letters it types.
var fuzzKeys = []tea.Msg{
	up, down, left, right, enter, esc, space, clearAll,
	tea.KeyPressMsg{Code: tea.KeyTab},
	tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
	tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl},
	tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl},
	tea.KeyPressMsg{Code: tea.KeyPgDown},
	tea.KeyPressMsg{Code: tea.KeyBackspace},
	typed("m")[0], typed("p")[0], typed("/")[0], typed("y")[0], typed("n")[0],
	typed("1")[0], typed("2")[0], typed("3")[0], typed("e")[0], typed("x")[0],
}

// fuzzApp is the app on a home, every command it runs succeeding at once without output.
func fuzzApp(t *testing.T, h menu.Home) model {
	t.Helper()
	m := model{width: usualWidth, height: usualHeight, cfg: Config{
		Home:    func() menu.Home { return h },
		Command: func([]string) *exec.Cmd { return exec.CommandContext(t.Context(), "true") },
		Exec:    func(_ *exec.Cmd, fn tea.ExecCallback) tea.Cmd { return func() tea.Msg { return fn(nil) } },
	}}
	m, _ = settle(m, m.Init())
	return m
}

// Any keys pressed on any home keep the menu to its rules on every screen, and never break it: the first byte picks
// the home, every other byte a key.
func FuzzMenu(f *testing.F) {
	homes := menutest.Homes()
	// Seed every home with the walks into each group and back
	for i := range homes {
		f.Add([]byte{byte(i), 4, 4, 5, 1, 4, 5, 5})
		f.Add([]byte{byte(i), 1, 1, 4, 3, 1, 4, 6, 4, 17, 4})
	}

	f.Fuzz(func(t *testing.T, keys []byte) {
		// Pass over an input without a home
		if len(keys) == 0 {
			return
		}
		m := fuzzApp(t, homes[int(keys[0])%len(homes)].Home)
		// Press every key, checking the screen after each
		for _, b := range keys[1:] {
			var quit bool
			m, quit = drive(m, fuzzKeys[int(b)%len(fuzzKeys)])
			// End once the menu leaves
			if quit {
				return
			}
			// Report every break of the rules outside the known ones
			for _, br := range screenBreaks(m) {
				// Report a break no finding names
				if _, known := knownBreaks[br]; !known {
					t.Fatalf("%s, on:\n%s", br, strings.Join(screen(m), "\n"))
				}
			}
		}
	})
}
