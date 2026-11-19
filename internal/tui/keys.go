package tui

import (
	"strconv"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// bind builds a key binding with the help the footer shows for it.
func bind(keyHelp, desc string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keyHelp, desc))
}

// The keys the fields and the screens take, each with the help the footer shows.
var (
	keyNavigate = bind("↑/↓", "navigate", "up", "down")
	keySwitch   = bind("←/→", "switch", "left", "right", "tab", "shift+tab")
	keyType     = bind("type", "to search", "")
	keySearch   = bind("/", "search", "/")
	keyToggle   = bind("space", "toggle", "space")
	keyPick     = bind("m", "move", "m", "ctrl+up", "ctrl+down", "K", "J")
	keyCarry    = bind("↑/↓", "move", "up", "down", "ctrl+up", "ctrl+down", "K", "J")
	keyDrop     = bind("enter", "drop", "enter")
	keySelect   = bind("enter", "select", "enter")
	keyApply    = bind("enter", "apply", "enter")
	keyContinue = bind("enter", "continue", "enter")
	keyBack     = bind("esc", "back", "esc")
	keyClear    = bind("esc", "clear", "esc")
	keyCancel   = bind("esc", "cancel", "esc")
	keyQuit     = bind("esc", "quit", "esc")
	keyScroll   = bind("↑/↓/pgup/pgdn", "scroll", "up", "down", "pgup", "pgdown")
	keyMenu     = bind("esc", "menu", "esc")
	keyYesNo    = bind("y/n", "answer", "y", "n")
)

// digits is the binding of the digits that pick one of n numbered options.
func digits(n int) key.Binding {
	keys := make([]string, n)
	// Bind every digit up to the last option
	for i := range keys {
		keys[i] = strconv.Itoa(i + 1)
	}
	return bind("1-"+strconv.Itoa(n), "choose", keys...)
}

// footer draws the keys that apply now, cut at the width with an ellipsis where they run longer.
func footer(width int, bindings []key.Binding) string {
	h := help.New()
	h.ShortSeparator = " · "
	h.Styles.ShortKey, h.Styles.ShortDesc, h.Styles.ShortSeparator, h.Styles.Ellipsis = dim, dim, dim, dim
	h.SetWidth(width)
	return h.ShortHelpView(bindings)
}
