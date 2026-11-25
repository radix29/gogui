package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/shell"
)

// The routing of IME preedits (PLAN.md S5.3b).

// TestPreeditRouting: a PreeditEvent goes where a TextEvent goes (the
// focused element, else the root), bubbles until handled, skips disabled
// elements, and carries the shell event unchanged.
func TestPreeditRouting(t *testing.T) {
	w := newWindow(t)
	r := w.r
	pe := shell.PreeditEvent{Text: "にほん", Cursor: 6,
		Segments: []shell.PreeditSegment{{Start: 0, End: 9, Selected: true}}, Time: 42}

	// No focus: the root gets it, and nobody handles it.
	if r.Event(pe) {
		t.Fatal("unhandled preedit reported handled")
	}
	checkLog(t, &w.log, `root preedit "にほん" @6`)

	// Focused: it bubbles from the focus to the root, as text does.
	w.btn.SetFocusable(true)
	w.btn.Focus()
	checkLog(t, &w.log, "btn focus true")
	r.Event(pe)
	r.Event(shell.TextEvent{Text: "日本"})
	r.Event(shell.PreeditEvent{}) // the end of the composition routes the same way
	checkLog(t, &w.log,
		`btn preedit "にほん" @6`, `panel preedit "にほん" @6`, `root preedit "にほん" @6`,
		`btn text "日本"`, `panel text "日本"`, `root text "日本"`,
		`btn preedit "" @0`, `panel preedit "" @0`, `root preedit "" @0`)

	// A handler stops the bubbling and the result goes back to the shell;
	// the event arrives whole.
	var got shell.PreeditEvent
	w.panel.handles["preedit"] = true
	w.panel.onEvent = func(ev Event) {
		if p, ok := ev.(PreeditEvent); ok {
			got = p.PreeditEvent
		}
	}
	if !r.Event(pe) {
		t.Fatal("handled preedit reported unhandled")
	}
	checkLog(t, &w.log, `btn preedit "にほん" @6`, `panel preedit "にほん" @6`)
	if got.Text != pe.Text || got.Cursor != pe.Cursor || got.Time != pe.Time ||
		len(got.Segments) != 1 || got.Segments[0] != pe.Segments[0] {
		t.Errorf("preedit arrived as %+v", got)
	}

	// A disabled ancestor gets nothing: disabling the panel takes the focus
	// from the button inside it, so the root gets the preedit.
	w.panel.SetEnabled(false)
	w.log = nil
	if r.Event(pe) {
		t.Fatal("a disabled panel handled a preedit")
	}
	checkLog(t, &w.log, `root preedit "にほん" @6`)
}

// TestPreeditIgnored: in a real window, a preedit that reaches no text
// element (nothing focused) is unhandled and changes nothing.
func TestPreeditIgnored(t *testing.T) {
	h := &fieldLog{}
	tf, w, sf, _ := fieldWindow(t, false, h)
	tf.SetText("ab")
	if w.Router().Focused() != nil {
		t.Fatal("the field is focused")
	}
	if sf.Send(shell.PreeditEvent{Text: "か", Cursor: 3}) || sf.Send(shell.PreeditEvent{}) {
		t.Error("an unfocused window handled a preedit")
	}
	if tf.Text() != "ab" || len(h.events) != 0 || sf.Cancels() != 0 {
		t.Errorf("after the preedit: %q %q, %d cancels", tf.Text(), h.events, sf.Cancels())
	}
}
