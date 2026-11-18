package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// view shows a group's tabs, each a list the user searches, picks from, marks or reorders, and takes the tab with the
// row or the key the user picks in it.
type view struct {
	q     menu.Question
	tab   int
	lists []list
	// searching marks a search box that takes what the user types.
	searching bool
}

// newView starts a view at the tab its default names, on the row it names, with every tab's marks in place.
func newView(q menu.Question) view {
	v := view{q: q, lists: make([]list, len(q.Tabs))}
	// Build the list of every tab
	for i, t := range q.Tabs {
		v.lists[i] = list{options: t.Options}
		// Start the marks of a tab the user marks
		if t.Multi {
			v.lists[i].marks = map[string]bool{}
			// Mark what the tab marks from the start
			for _, m := range t.Marked {
				v.lists[i].marks[m] = true
			}
		}
	}
	// Open at the tab the default names
	if len(q.Default) > 0 {
		v.tab = max(slices.IndexFunc(q.Tabs, func(t menu.Tab) bool { return t.Name == q.Default[0] }), 0)
	}
	// Start on the row the default names
	if len(q.Default) > 1 && len(q.Tabs) > 0 {
		v.lists[v.tab].cursor = max(slices.IndexFunc(q.Tabs[v.tab].Options, func(o menu.Option) bool { return o.Value == q.Default[1] }), 0)
	}
	return v
}

// current is the tab in view and its list.
func (v view) current() (menu.Tab, *list) {
	// Read a view without tabs as one empty tab
	if len(v.q.Tabs) == 0 {
		return menu.Tab{}, &list{}
	}
	return v.q.Tabs[v.tab], &v.lists[v.tab]
}

// answer ends the view with the tab in view and the values given.
func (v view) answer(values ...string) (field, result, tea.Cmd) {
	t, _ := v.current()
	return v, result{outcome: answered, values: append([]string{t.Name}, values...)}, nil
}

// Update follows a key: the arrows move and switch tabs, / searches, Space marks, ctrl+arrows move a row, a tab's own
// keys act, Enter picks or applies, Esc stops a search or steps back.
func (v view) Update(msg tea.Msg) (field, result, tea.Cmd) {
	k, isKey := msg.(tea.KeyPressMsg)
	// Keep the field as it is on anything but a key
	if !isKey {
		return v, result{}, nil
	}
	v.lists = slices.Clone(v.lists)
	t, l := v.current()
	// Type into the search while it takes what the user types
	if v.searching {
		// Follow the keys that end or edit the search
		switch k.String() {
		case "esc":
			v.searching, l.filter, l.cursor = false, "", 0
		case "enter", "down":
			v.searching = false
		default:
			l.move(k.String(), k.Text, true)
		}
		return v, result{}, nil
	}
	// Act on a key the tab takes
	for _, tk := range t.Keys {
		// Answer with the value of the key pressed
		if k.String() == tk.Key {
			return v.answer(tk.Value)
		}
	}
	// Follow the key the user pressed
	switch k.String() {
	case "esc":
		// Clear a search the user left, else step back
		if l.filter != "" {
			l.filter, l.cursor = "", 0
			return v, result{}, nil
		}
		return v, result{outcome: back}, nil
	case "left", "shift+tab":
		v.tab = (v.tab + len(v.q.Tabs) - 1) % max(len(v.q.Tabs), 1)
	case "right", "tab":
		v.tab = (v.tab + 1) % max(len(v.q.Tabs), 1)
	case "/":
		v.searching = len(t.Options) > 0
	case "space":
		// Mark or unmark a row where the tab takes marks
		if t.Multi {
			l.toggle()
		}
	case "ctrl+up", "K", "ctrl+down", "J":
		o, ok := l.highlighted()
		// Move a row where the tab lets rows move
		if t.Reorder && ok && l.filter == "" {
			direction := "down"
			// Move up where the key says so
			if k.String() == "ctrl+up" || k.String() == "K" {
				direction = "up"
			}
			return v.answer("+move", o.Value, direction)
		}
	case "enter":
		// Hand back the marks of a tab that takes them
		if t.Multi {
			return v.answer(l.marked()...)
		}
		// Take the highlighted row
		if o, ok := l.highlighted(); ok {
			return v.answer(o.Value)
		}
	default:
		l.move(k.String(), "", false)
	}
	return v, result{}, nil
}

// View draws the tabs, what the tab in view holds, the search box and the rows, or what the tab says while it holds
// nothing.
func (v view) View(width, height int) string {
	t, l := v.current()
	var lines []string
	// Draw the tab bar where the view has several tabs
	if len(v.q.Tabs) > 1 {
		var bar strings.Builder
		bar.WriteString(title.Render(v.q.Title))
		// Draw every tab, the one in view highlighted
		for i, tab := range v.q.Tabs {
			name := " " + tab.Name + " "
			// Highlight the tab in view
			if i == v.tab {
				name = tabOn.Render(name)
			} else {
				name = dim.Render(name)
			}
			bar.WriteString("  ")
			bar.WriteString(name)
		}
		lines = append(lines, cut(bar.String(), width), "")
	}
	// Say what the tab holds
	if t.About != "" {
		lines = append(lines, dim.Render(cut(t.About, width)))
	}
	// Say what the tab holds while it holds nothing
	if len(t.Options) == 0 {
		return strings.Join(append(lines, "", t.Empty), "\n")
	}
	lines = append(lines, v.search(l, width))
	lines = append(lines, l.view(width, height-len(lines)-2)...)
	return strings.Join(lines, "\n")
}

// search draws the framed search box, with what the user typed or the hint to search.
func (v view) search(l *list, width int) string {
	inside := dim.Render("⌕ Search…")
	// Show what the user typed, with the cursor while the box takes keys
	if l.filter != "" || v.searching {
		inside = "⌕ " + l.filter
		// Show the cursor while the box takes keys
		if v.searching {
			inside += pointer.Render("▏")
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Width(max(width-2, minFieldWidth)).Render(" " + inside)
}

// Keys are the keys the view takes now: the search's own while it takes keys, and the tab's otherwise.
func (v view) Keys() []key.Binding {
	t, l := v.current()
	// Name the keys of the search while it takes keys
	if v.searching {
		return []key.Binding{keyType, keyNavigate, keyContinue, keyClear}
	}
	var keys []key.Binding
	// Name moving through the rows where there are any
	if len(t.Options) > 0 {
		keys = append(keys, keyNavigate)
	}
	// Name switching where the view has several tabs
	if len(v.q.Tabs) > 1 {
		keys = append(keys, keySwitch)
	}
	// Name marking where the tab takes marks
	if t.Multi && len(t.Options) > 0 {
		keys = append(keys, keyToggle)
	}
	// Name moving a row where the tab lets rows move
	if t.Reorder && len(t.Options) > 1 {
		keys = append(keys, keyMove)
	}
	// Name every key the tab acts on
	for _, tk := range t.Keys {
		keys = append(keys, bind(tk.Key, tk.Help, tk.Key))
	}
	// Name searching where there are rows to search
	if len(t.Options) > 0 {
		keys = append(keys, keySearch)
	}
	// Name what Enter does on the tab
	switch {
	case t.Multi && len(t.Options) > 0:
		keys = append(keys, keyApply)
	case len(t.Options) > 0:
		keys = append(keys, keySelect)
	}
	// Name Esc as clearing a search the user left
	if l.filter != "" {
		return append(keys, keyClear)
	}
	return append(keys, keyBack)
}
