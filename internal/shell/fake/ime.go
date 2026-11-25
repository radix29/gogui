package fake

import (
	"unicode"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/shell"
)

// ime is the text input state of a surface.
type ime struct {
	input     shell.TextInput // the state set last
	sets      int             // SetTextInput calls
	cancels   int             // CancelComposition calls
	composing bool            // a non-empty preedit was sent and not ended
}

// SetTextInput implements shell.Surface: the fake records ti. Disabling
// the text input ends a composition without any event.
func (sf *Surface) SetTextInput(ti shell.TextInput) {
	if !sf.live("SetTextInput") {
		return
	}
	sf.ime.input = ti
	sf.ime.sets++
	if !ti.Enabled {
		sf.ime.composing = false
	}
}

// TextInput returns the text input state set last (disabled initially).
func (sf *Surface) TextInput() shell.TextInput { sf.check("TextInput"); return sf.ime.input }

// TextInputSets returns the number of SetTextInput calls so far, changes
// or not.
func (sf *Surface) TextInputSets() int { sf.check("TextInputSets"); return sf.ime.sets }

// CancelComposition implements shell.Surface: the fake counts the call and
// ends a composition without any event.
func (sf *Surface) CancelComposition() {
	if !sf.change("CancelComposition") {
		return
	}
	sf.ime.cancels++
	sf.ime.composing = false
}

// Cancels returns the number of CancelComposition calls so far.
func (sf *Surface) Cancels() int { sf.check("Cancels"); return sf.ime.cancels }

// Composing reports whether a composition is running: a Preedit was sent
// and no commit (Commit, CommitPreedit, a TextEvent through Send),
// EndPreedit, CancelComposition or disabling SetTextInput has ended it
// since.
func (sf *Surface) Composing() bool { sf.check("Composing"); return sf.ime.composing }

// imeInput checks like keyInput and that a text input is enabled: an IME
// sends nothing while it is off, so driving one then is a test bug.
func (sf *Surface) imeInput(what string) bool {
	if !sf.keyInput(what) {
		return false
	}
	if !sf.ime.input.Enabled {
		panic("fake: Surface." + what + " while no text input is enabled (SetTextInput)")
	}
	return true
}

// Preedit sends a PreeditEvent with text as the composition, the IME's
// caret at the byte offset cursor and the clauses segs, as an IME does on
// each key of a composition. It panics unless the surface is key and a
// text input is enabled, and for an event that breaks the PreeditEvent
// rules: empty text (use EndPreedit), invalid UTF-8, control characters,
// a cursor or segment bound outside text or inside a rune, and segments
// that are empty, overlapping or out of order.
func (sf *Surface) Preedit(text string, cursor int, segs ...shell.PreeditSegment) {
	checkPreedit(text, cursor, segs)
	if !sf.imeInput("Preedit") {
		return
	}
	sf.ime.composing = true
	sf.keyTo().Event(shell.PreeditEvent{
		Text: text, Cursor: cursor,
		Segments: append([]shell.PreeditSegment(nil), segs...), Time: sf.now(),
	})
}

// EndPreedit sends the empty PreeditEvent that ends a composition, as an
// IME does when the user cancels it (Escape, deleting the whole preedit)
// or after a commit. It panics unless the surface is key and a text input
// is enabled.
func (sf *Surface) EndPreedit() {
	if sf.imeInput("EndPreedit") {
		sf.ime.composing = false
		sf.keyTo().Event(shell.PreeditEvent{Time: sf.now()})
	}
}

// CommitPreedit commits text as an IME does on Return or on a choice in
// its candidate window: the TextEvent with text, then the empty
// PreeditEvent (the GTK order; Commit followed by Preedit gives the Win32
// order of a commit with the next composition). It panics unless the
// surface is key and a text input is enabled, and for text Commit
// rejects. The empty PreeditEvent is not sent when the handler closed the
// surface, moved the key or disabled the text input during the commit.
func (sf *Surface) CommitPreedit(text string) {
	if !sf.imeInput("CommitPreedit") {
		return
	}
	sf.Commit(text)
	if sf.dead || sf.shell.key != sf || !sf.ime.input.Enabled {
		return
	}
	sf.EndPreedit()
}

// checkPreedit panics when a non-empty preedit breaks the PreeditEvent
// rules.
func checkPreedit(text string, cursor int, segs []shell.PreeditSegment) {
	if text == "" || !utf8.ValidString(text) {
		panic("fake: Preedit with empty text or invalid UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			panic("fake: Preedit with a control character")
		}
	}
	at := func(i int) bool {
		return i >= 0 && i <= len(text) && (i == len(text) || utf8.RuneStart(text[i]))
	}
	if !at(cursor) {
		panic("fake: Preedit cursor outside the text or inside a rune")
	}
	prev := 0
	for _, sg := range segs {
		if !at(sg.Start) || !at(sg.End) || sg.Start >= sg.End || sg.Start < prev {
			panic("fake: Preedit segment empty, out of order, overlapping, outside the text or inside a rune")
		}
		prev = sg.End
	}
}
