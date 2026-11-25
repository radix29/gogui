package gtk4

import (
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The IME (S5.3h–S5.3j). The surface's input method is a
// GtkIMMulticontext (window_linux.go), which hands the keys to the user's
// input method (IBus, Fcitx; GTK's simple context as its own fallback),
// with use-preedit on, so the input method leaves the preedit to us:
//
//   - "preedit-changed" (sigVoid) reads the preedit (window.preedit: its
//     text, its cursor in characters and its Pango attributes) into a
//     PreeditEvent (preeditOf); a preedit without text ends the
//     composition.
//   - "preedit-end" ends the composition with an empty PreeditEvent, sent
//     once per composition; "preedit-start" sends nothing (a composition
//     starts with its first text).
//   - "commit" stays the TextEvent of keyboard.go; during a composition it
//     is its commit and empties the preedit (shell.PreeditEvent), so the
//     "preedit-end" GTK sends after it sends nothing more.
//
// The preedit events reach the handler only while the text input is on
// (shell.TextInput.Enabled), at the time of the key event that caused
// them (now outside one, as the asynchronous input methods do).
//
// The text input (S5.3j). While it is off, the S4.1k order holds: the
// handler sees a key first and the input method filters it after (without
// the focus, IBus and Fcitx leave it to their simple fallback). Turning it
// on gives the key controller the input method
// (gtk_event_controller_key_set_im_context), so GTK filters every key
// through it before "key-pressed" and "key-released": a key it takes
// reaches neither (a composition's keys, and plain typing, which arrives
// as its commit), and the keys it declines come to keyPressed afterwards,
// which then does not filter them again. IBus's asynchronous replays work
// in this order: IBus takes the key at once and puts it back when the
// engine declines it. The input method has the focus only while the text
// input is on and the surface is the key window (syncIMFocus; the key
// controller also focuses it on its own crossings, which agrees). The
// cursor location is the caret, in GGSurface points. SetTextInput,
// CancelComposition (a reset) and the loss of the key window run muted:
// a commit or preedit the input method makes inside them is dropped, as
// aqua commits its own copy of the preedit (S5.3c). Closing takes the
// focus away and detaches the input method before the GGSurface goes; the
// last reference to it goes after the event being handled (releaseIM).
// "retrieve-surrounding" and "delete-surrounding" are not answered (v1).

// ime is the text input state of a surface.
type ime struct {
	input     shell.TextInput // the state set last
	composing bool            // a preedit with text was sent, and no end since
	attached  bool            // the key controller has the input method (input on)
	focused   bool            // the input method has the focus (syncIMFocus)
	mute      int             // > 0: commits and preedits are dropped (muted)
}

// preeditAttr is one Pango attribute of a preedit, as window.preedit reads
// it: its PangoAttrType, its UTF-8 byte range in the preedit's text
// (gtk.AttrIndexToTextEnd: to the end) and, for an underline, its
// PangoUnderline.
type preeditAttr struct {
	typ        int32
	start, end uint32
	value      int32
}

// clause reports whether a reaches the segments: an underline that draws
// one, or a background or foreground, the colors input methods mark the
// clause being converted with (IBus, Fcitx 5: both).
func (a preeditAttr) clause() (ok, selected bool) {
	switch a.typ {
	case gtk.AttrUnderline:
		return a.value != gtk.UnderlineNone, false
	case gtk.AttrBackground, gtk.AttrForeground:
		return true, true
	}
	return false, false
}

// preeditText returns s with the control characters dropped and each
// invalid byte replaced by U+FFFD (commitText's rules, per byte), and
// offs: for every byte index of s (len(s) included) the offset in the
// result of the rune that holds it, so an index inside a rune snaps to its
// start.
func preeditText(s string) (string, []int) {
	var b strings.Builder
	offs := make([]int, len(s)+1)
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		for j := i; j < i+n; j++ {
			offs[j] = b.Len()
		}
		switch {
		case r == utf8.RuneError && n == 1:
			b.WriteRune(utf8.RuneError)
		case !isControl(r):
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	offs[len(s)] = b.Len()
	return b.String(), offs
}

// preeditOf converts a preedit as GTK reports it, its UTF-8 text, its
// cursor in characters (out of range: the end) and its attributes, into a
// PreeditEvent: the text cleaned up (preeditText), the cursor as a byte
// offset, and the segments. The attributes that mark clauses (clause) cut
// the text at their bounds; each piece between two cuts that one of them
// covers is a segment, selected if a background or foreground covers it.
// Ranges past the text end there, empty ones are dropped. A preedit
// without text gives the empty event.
func preeditOf(s string, cursor int, attrs []preeditAttr) shell.PreeditEvent {
	text, offs := preeditText(s)
	if text == "" {
		return shell.PreeditEvent{}
	}
	at := len(s)
	if cursor >= 0 {
		for i := range s {
			if cursor == 0 {
				at = i
				break
			}
			cursor--
		}
	}
	ev := shell.PreeditEvent{Text: text, Cursor: offs[at]}
	off := func(i uint32) int { return offs[min(uint64(i), uint64(len(s)))] }
	type span struct {
		start, end int
		selected   bool
	}
	var spans []span
	var cuts []int
	for _, a := range attrs {
		ok, sel := a.clause()
		if start, end := off(a.start), off(a.end); ok && start < end {
			spans = append(spans, span{start, end, sel})
			cuts = append(cuts, start, end)
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	for i := 0; i+1 < len(cuts); i++ {
		seg := shell.PreeditSegment{Start: cuts[i], End: cuts[i+1]}
		covered := false
		for _, sp := range spans {
			if sp.start <= seg.Start && seg.End <= sp.end {
				covered = true
				seg.Selected = seg.Selected || sp.selected
			}
		}
		if covered {
			ev.Segments = append(ev.Segments, seg)
		}
	}
	return ev
}

// imPreedit handles the input method's preedit signals (kindIMPreedit*).
func (sf *surface) imPreedit(k kind) {
	if !sf.keyLive() || !sf.ime.input.Enabled || sf.ime.mute > 0 {
		return
	}
	switch k {
	case kindIMPreeditChanged:
		ev := preeditOf(sf.w.preedit())
		if ev.Text == "" {
			sf.endPreedit()
			return
		}
		sf.ime.composing = true
		ev.Time = sf.textTime()
		sf.keySink().Event(ev)
	case kindIMPreeditEnd:
		sf.endPreedit()
	}
}

// endPreedit ends a running composition with an empty PreeditEvent.
func (sf *surface) endPreedit() {
	if !sf.ime.composing {
		return
	}
	sf.ime.composing = false
	sf.keySink().Event(shell.PreeditEvent{Time: sf.textTime()})
}

// textTime is the time of a commit or preedit: that of the key event that
// caused it, the one keyPressed or keyReleased handles or the one the key
// controller filters through the input method, else now.
func (sf *surface) textTime() time.Duration {
	t := sf.kbd.evTime
	if t == 0 {
		t = sf.w.imEventTime()
	}
	return sf.s.eventTime(t)
}

// imCaret converts the caret (surface points) into the input method's
// cursor location, widened to whole points (GGSurface coordinates are
// points) and clamped to the C int range.
func imCaret(r driver.Rect) gtk.Allocation {
	c := func(v float64) int32 { return int32(max(min(v, math.MaxInt32), math.MinInt32)) }
	if math.IsNaN(r.X) || math.IsNaN(r.Y) || math.IsNaN(r.W) || math.IsNaN(r.H) {
		return gtk.Allocation{}
	}
	x, y := math.Floor(r.X), math.Floor(r.Y)
	return gtk.Allocation{X: c(x), Y: c(y),
		Width: c(max(math.Ceil(r.X+r.W)-x, 0)), Height: c(max(math.Ceil(r.Y+r.H)-y, 0))}
}

// muted runs fn with the input method's commits and preedits swallowed:
// they reach no handler, so SetTextInput and CancelComposition deliver no
// event, not even a commit the input method makes while it is reset or
// loses the focus.
func (sf *surface) muted(fn func()) {
	sf.ime.mute++
	defer func() { sf.ime.mute-- }()
	fn()
}

// syncIMFocus gives the input method the focus while the text input is on
// and the surface is the key window, and takes it away otherwise.
func (sf *surface) syncIMFocus() {
	if want := sf.ime.input.Enabled && sf.key; want != sf.ime.focused {
		sf.ime.focused = want
		sf.w.imFocus(want)
	}
}

// SetTextInput implements shell.Surface. Turning the input on gives the
// key controller the input method (so it sees every key first), sets the
// cursor location and, in the key window, the focus; a new caret moves the
// cursor location. Turning it off resets the input method, takes the focus
// and the key controller's context away and forgets the composition. All
// of it muted.
func (sf *surface) SetTextInput(ti shell.TextInput) {
	if !sf.live("SetTextInput") {
		return
	}
	was := sf.ime.input
	sf.ime.input = ti
	sf.muted(func() {
		switch {
		case ti.Enabled && !was.Enabled:
			sf.ime.attached = true
			sf.w.imAttach(true)
			sf.w.imCursor(imCaret(ti.Caret))
			sf.syncIMFocus()
		case ti.Enabled:
			if ti.Caret != was.Caret {
				sf.w.imCursor(imCaret(ti.Caret))
			}
		case was.Enabled:
			sf.ime.composing = false
			sf.w.imReset()
			sf.syncIMFocus()
			sf.ime.attached = false
			sf.w.imAttach(false)
		}
	})
	rethrow()
}

// CancelComposition implements shell.Surface: while the text input is on,
// the input method is reset, muted, so neither a commit it makes while it
// resets nor the end of the composition is delivered. An asynchronous
// input method (IBus) may still send a commit after the call; the field
// takes it as typing.
func (sf *surface) CancelComposition() {
	if !sf.change("CancelComposition") {
		return
	}
	sf.ime.composing = false
	if sf.ime.input.Enabled {
		sf.muted(sf.w.imReset)
		rethrow()
	}
}

// imTeardown is the input method's part of closing: it loses the focus,
// muted (the surface is dead already, so nothing is delivered anyway).
// destroy then detaches it from the key controller and the GGSurface.
func (sf *surface) imTeardown() {
	sf.ime.composing = false
	sf.muted(func() {
		if sf.ime.focused {
			sf.ime.focused = false
			sf.w.imFocus(false)
		}
	})
	sf.ime.attached = false
}

// releaseLater runs fn, a closed window's releaseIM, once the event being
// handled is over: from the queue, or at shutdown if the queue never runs
// it (Quit closes it right after the surfaces).
func (s *Shell) releaseLater(fn func()) {
	s.imReleases = append(s.imReleases, fn)
	if len(s.imReleases) == 1 {
		s.q.Do(s.releaseIMs)
	}
}

// releaseIMs runs the pending releases.
func (s *Shell) releaseIMs() {
	l := s.imReleases
	s.imReleases = nil
	for _, fn := range l {
		fn()
	}
}
