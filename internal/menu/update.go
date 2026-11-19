package menu

import (
	"strings"

	"github.com/hansehart/o3s/internal/words"
)

// updateDialog shows every part with a newer version, all of them marked, then the confirmation of the marked ones.
func updateDialog(h Home, a Answers) (Question, bool) {
	updates, err := h.Lookup.Updates()
	// End the dialog where what is newer stays unknown
	if err != nil {
		return Question{}, true
	}
	// Show the parts with a newer version first, all of them marked
	if !a.has("view") {
		t := Tab{Name: "Update", About: "Newer versions, all chosen.", Empty: "Everything is up to date.", Multi: true}
		// Show every part with what changes, marked
		for _, u := range updates {
			t.Options = append(t.Options, Option{Value: u.Target, Label: u.Name, Columns: []string{u.Change}})
			t.Marked = append(t.Marked, u.Target)
		}
		return Question{Key: "view", Kind: View, Title: "Update", Tabs: []Tab{t}}, false
	}
	_, marked := view(a)
	// End where the user marked nothing
	if len(marked) == 0 {
		return Question{}, true
	}
	var lines []string
	// Show every marked part with what changes
	for _, u := range updates {
		// Take a marked part
		if strings.Contains(" "+strings.Join(marked, " ")+" ", " "+u.Target+" ") {
			lines = append(lines, "~ "+u.Name+"   "+u.Change)
		}
	}
	return ask(a, apply(lines, "Updates "+words.Count(len(lines), "part", "parts"), false))
}

// updatePlan brings the marked parts up to date.
func updatePlan(_ Home, a Answers) []Command {
	_, marked := view(a)
	// Go back where the user marked nothing
	if len(marked) == 0 {
		return []Command{{Note: "Everything is up to date."}}
	}
	// Leave everything as it was where the user declines
	if !confirmed(a) {
		return declined()
	}
	return []Command{{Args: append(append([]string{"update"}, marked...), "--yes")}}
}
