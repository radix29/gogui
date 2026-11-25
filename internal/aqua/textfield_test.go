package aqua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// fieldLog records the events of a text field.
type fieldLog struct{ events []string }

func (l *fieldLog) TextChanged(s string) { l.events = append(l.events, "changed "+s) }
func (l *fieldLog) Submitted(s string)   { l.events = append(l.events, "submitted "+s) }

// fieldWindow opens a 200×100 window with a text field at content
// (10, 10, 100, 22), surface (10, 38, 100, 22), without the focus.
func fieldWindow(t *testing.T, secure bool, h driver.TextFieldHandler) (*TextField, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	f := d.NewTextField(secure, h).(*TextField)
	w.SetContent(f)
	f.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 22})
	w.Router().SetFocus(nil)
	return f, w, sf, s
}

// fieldLines returns the field's lines of a window dump: those between
// the content background and the title bar, without the content clip.
func fieldLines(dump string) string {
	var out []string
	in := false
	for _, l := range strings.Split(dump, "\n") {
		switch {
		case strings.HasPrefix(l, "fillRRect (0,28,200,100)"):
			in = true
			continue
		case strings.HasPrefix(l, "fillRRect (0,0,200,28)"):
			in = false
		}
		if in {
			out = append(out, strings.TrimSuffix(l, " clip=(0,28,200,100)"))
		}
	}
	return strings.Join(out, "\n")
}

func TestTextFieldIsReal(t *testing.T) {
	d := startDriver(t)
	tf := d.NewTextField(false, nil).(*TextField)
	if e := ElementOf(tf); e == nil || e != Element(tf.view) || tf.StubPeer() != tf.TextField || tf.Stub() != &tf.TextField.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if e := ElementOf(tf).base(); !e.Focusable() || e.FocusKind() != FocusText {
		t.Error("not focusable as text")
	}
	if !tf.Editor().SingleLine() {
		t.Error("the editor is multi-line")
	}
	// FittingSize stays the stub's, so the example frames do not move.
	if got := tf.FittingSize(infSize); got != (driver.Size{W: 96, H: 22}) {
		t.Errorf("FittingSize %v", got)
	}
	if got := ElementOf(tf).FittingSize(infSize); got != (driver.Size{W: 96, H: 22}) {
		t.Errorf("element FittingSize %v", got)
	}
}

func TestTextFieldNodes(t *testing.T) {
	d := startDriver(t)
	tf := d.NewTextField(false, nil).(*TextField)
	for _, c := range []struct {
		do   func()
		want string
	}{
		{func() {}, `textfield ""`},
		{func() { tf.SetPlaceholder("Name") }, `textfield "Name"`},
		{func() { tf.SetText("Ann"); tf.SetEditable(false) }, `textfield "Name" value="Ann" [readonly]`},
		{func() { tf.SetEditable(true); tf.UserType("Bob") }, `textfield "Name" value="Bob"`},
	} {
		c.do()
		if got := nodeOf(tf); got != c.want {
			t.Errorf("node %q, want %q", got, c.want)
		}
	}
	sec := d.NewTextField(true, nil).(*TextField)
	sec.SetPlaceholder("Password")
	sec.SetText("pä!")
	if got := nodeOf(sec); got != `textfield "Password"` {
		t.Errorf("secure node %q", got)
	}
}

// TestTextFieldText: SetText replaces the editor's text (newlines as
// spaces, the caret at the end, no undo, no event), UserType simulates an
// edit with TextChanged, refused while read-only or disabled, and a
// SetText from inside TextChanged wins.
func TestTextFieldText(t *testing.T) {
	h := &fieldLog{}
	tf := startDriver(t).NewTextField(false, h).(*TextField)
	tf.SetText("a\nb\r\nc")
	ed := tf.Editor()
	if tf.Text() != "a b c" || ed.Text() != "a b c" || ed.Anchor() != 5 || ed.Head() != 5 || ed.CanUndo() {
		t.Errorf("SetText: %q %q selection %d,%d undo %v", tf.Text(), ed.Text(), ed.Anchor(), ed.Head(), ed.CanUndo())
	}
	if len(h.events) != 0 {
		t.Errorf("SetText fired %v", h.events)
	}
	tf.UserType("xy")
	if tf.Text() != "xy" || ed.Text() != "xy" || ed.Head() != 2 || strings.Join(h.events, ";") != "changed xy" {
		t.Errorf("UserType: %q %q %v", tf.Text(), ed.Text(), h.events)
	}
	tf.SetEditable(false)
	tf.UserType("no")
	tf.SetEditable(true)
	tf.SetEnabled(false)
	tf.UserType("no")
	tf.SetEnabled(true)
	if tf.Text() != "xy" || ed.Text() != "xy" || len(h.events) != 1 {
		t.Errorf("read-only or disabled typing: %q %v", ed.Text(), h.events)
	}

	// The converter case: the handler sets the text it was told about.
	conv := &convField{}
	cf := startDriver(t).NewTextField(false, conv).(*TextField)
	conv.f = cf
	cf.UserType("5")
	if cf.Text() != "5.0" || cf.Editor().Text() != "5.0" || conv.calls != 1 {
		t.Errorf("SetText in TextChanged: %q %q, %d calls", cf.Text(), cf.Editor().Text(), conv.calls)
	}
}

type convField struct {
	f     *TextField
	calls int
}

func (c *convField) TextChanged(s string) { c.calls++; c.f.SetText(s + ".0") }
func (c *convField) Submitted(string)     {}

// TestTextFieldLayoutCache: the layout is reused until the text or the
// font changes; a secure field lays out one bullet per grapheme cluster.
func TestTextFieldLayoutCache(t *testing.T) {
	d := startDriver(t)
	tf := d.NewTextField(false, nil).(*TextField)
	tf.SetText("Hello")
	l := tf.view.textLayout()
	if tf.view.textLayout() != l || l.Text() != "Hello" || l.Font().Size != 13 {
		t.Fatal("layout not cached")
	}
	tf.SetPlaceholder("p")
	tf.SetAlignment(driver.AlignCenter)
	if tf.view.textLayout() != l {
		t.Error("a new layout for a change that does not move the text")
	}
	tf.SetText("Hello")
	if l2 := tf.view.textLayout(); l2 == l || l2.Text() != "Hello" {
		t.Error("SetText kept the layout")
	}
	l = tf.view.textLayout()
	tf.SetFont(driver.Font{Size: 20})
	if l2 := tf.view.textLayout(); l2 == l || l2.Font().Size != 20 {
		t.Error("SetFont kept the layout")
	}
	tf.SetFont(driver.Font{})
	tf.SetControlSize(driver.ControlSizeSmall)
	if got := tf.view.textLayout().Font().Size; got != 11 {
		t.Errorf("small field font %v", got)
	}

	sec := d.NewTextField(true, nil).(*TextField)
	// "e" + combining acute, a flag (two regional indicators) and a ZWJ
	// family are one cluster each.
	sec.SetText("pé🇨🇭👨‍👩‍👧!")
	if got := sec.view.textLayout().Text(); got != "•••••" {
		t.Errorf("secure layout %q", got)
	}
	s := sec.Editor().Text()
	for _, c := range []struct{ off, want int }{
		{0, 0}, {1, 3}, {len("pé"), 6}, {len("pé🇨🇭"), 9}, {len(s) - 1, 12}, {len(s), 15},
	} {
		if got := sec.view.shownOffset(c.off); got != c.want {
			t.Errorf("shownOffset(%d) = %d, want %d", c.off, got, c.want)
		}
	}
}

// TestTextFieldRepaints: every setter that changes the look invalidates
// the window.
func TestTextFieldRepaints(t *testing.T) {
	tf, _, sf, s := fieldWindow(t, false, nil)
	for _, c := range []struct {
		name string
		do   func()
	}{
		{"SetText", func() { tf.SetText("a") }},
		{"SetPlaceholder", func() { tf.SetPlaceholder("b") }},
		{"SetFont", func() { tf.SetFont(driver.Font{Size: 14}) }},
		{"SetAlignment", func() { tf.SetAlignment(driver.AlignTrailing) }},
		{"SetEditable", func() { tf.SetEditable(false) }},
		{"SetControlSize", func() { tf.SetControlSize(driver.ControlSizeLarge) }},
		{"SetEnabled", func() { tf.SetEnabled(false) }},
		{"UserType", func() { tf.SetEnabled(true); tf.SetEditable(true); paint(t, s, sf); tf.UserType("c") }},
		{"Focus", func() { tf.view.Focus() }},
		{"Deactivate", func() { s.Deactivate() }},
	} {
		paint(t, s, sf)
		c.do()
		if !sf.Invalid() {
			t.Errorf("%s did not invalidate", c.name)
		}
	}
}

// TestTextFieldScroll: overflowing text is scrolled to keep the caret in
// view while focused, minimally and never past its end, and shown from its
// start without the focus; text that fits is aligned and not scrolled.
func TestTextFieldScroll(t *testing.T) {
	tf, w, sf, s := fieldWindow(t, false, nil)
	// 25 runes × 7 = 175 pt in a room of 100 − 2 × 4 = 92.
	tf.SetText("The quick brown fox jumps")
	ed := tf.Editor()
	// textAt repaints and returns where the text is drawn (the editor does
	// not invalidate the field before S5.2g, so the test does).
	textAt := func() string { tf.redraw(); return textRect(t, s, sf, "quick") }
	if got := textAt(); got != "(14,41,175,16)" || tf.view.ScrollX() != 0 {
		t.Errorf("unfocused: %s scroll %v", got, tf.view.ScrollX())
	}
	tf.view.Focus()
	if got := textAt(); got != "(-69,41,175,16)" || tf.view.ScrollX() != 83 {
		t.Errorf("caret at the end: %s scroll %v", got, tf.view.ScrollX())
	}
	// Moving left inside the visible part keeps the scroll; past its left
	// edge scrolls just far enough.
	ed.SetSelection(20, 20) // x 140, visible from 83 to 175
	if textAt(); tf.view.ScrollX() != 83 {
		t.Errorf("caret in view: scroll %v", tf.view.ScrollX())
	}
	ed.SetSelection(10, 10) // x 70
	if textAt(); tf.view.ScrollX() != 70 {
		t.Errorf("caret left of the view: scroll %v", tf.view.ScrollX())
	}
	ed.SetSelection(24, 24) // x 168, right of 70 + 92
	if textAt(); tf.view.ScrollX() != 76 {
		t.Errorf("caret right of the view: scroll %v", tf.view.ScrollX())
	}
	ed.SetSelection(0, 25) // the head at the end
	if textAt(); tf.view.ScrollX() != 83 {
		t.Errorf("selection: scroll %v", tf.view.ScrollX())
	}
	ed.SetSelection(25, 0) // the head at the start
	if textAt(); tf.view.ScrollX() != 0 {
		t.Errorf("reversed selection: scroll %v", tf.view.ScrollX())
	}
	ed.SetSelection(25, 25)
	textAt()
	w.Router().SetFocus(nil)
	if got := textAt(); got != "(14,41,175,16)" || tf.view.ScrollX() != 0 {
		t.Errorf("blur: %s scroll %v", got, tf.view.ScrollX())
	}
	// Text that fits is aligned and never scrolled.
	tf.view.Focus()
	tf.SetText("Hi")
	for _, c := range []struct {
		a    driver.TextAlignment
		want string
	}{
		{driver.AlignLeading, "(14,41,14,16)"},
		{driver.AlignJustified, "(14,41,14,16)"},
		{driver.AlignCenter, "(53,41,14,16)"},
		{driver.AlignTrailing, "(92,41,14,16)"},
	} {
		tf.SetAlignment(c.a)
		if got := textRect(t, s, sf, `"Hi"`); got != c.want {
			t.Errorf("alignment %v: text at %s, want %s", c.a, got, c.want)
		}
	}
}

// TestTextFieldCaretScale2: the caret and the bezel are pixel-aligned at
// scale 2.
func TestTextFieldCaretScale2(t *testing.T) {
	tf, _, sf, s := fieldWindow(t, false, nil)
	sf.SetScale(2)
	tf.SetFrame(driver.Rect{X: 10.3, Y: 10.1, W: 100, H: 22})
	tf.SetText("ab")
	tf.view.Focus()
	got := fieldLines(paint(t, s, sf))
	for _, want := range []string{
		"fillRRect (10.5,38,100,22) solid #ffffff",
		"strokeRRect (11,38.5,99,21) w=1 solid #0000002e",             // 1 pt is 2 px
		"fillRRect (27.5,41,2,16) solid #007aff clip=(11.5,39,98,20)", // 10.5 + 4 + 14 − 1
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}

// TestTextFieldStates pins the paint of the field states (empty,
// placeholder, text, selection, focused, unfocused, inactive, disabled,
// read-only, secure, overflow, alignment, Increase Contrast, IME
// compositions) in light and
// dark, as record dumps in testdata/textfield.states.
func TestTextFieldStates(t *testing.T) {
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, st := range []struct {
			name         string
			secure       bool
			text, ph     string
			sel          [2]int // anchor, head; -1 for the caret at the end
			focused      bool
			inactive     bool
			disabled     bool
			readOnly     bool
			align        driver.TextAlignment
			contrast     bool
			smallControl bool
			preedit      string // an IME composition at the caret (S5.3c)
			preCursor    int
			segs         []shell.PreeditSegment
		}{
			{name: "empty", sel: [2]int{-1}},
			{name: "placeholder", ph: "Name", sel: [2]int{-1}},
			{name: "placeholder focused", ph: "Name", sel: [2]int{-1}, focused: true},
			{name: "placeholder truncated", ph: "A very long placeholder text", sel: [2]int{-1}},
			{name: "text", text: "Hello", sel: [2]int{-1}},
			{name: "text focused", text: "Hello", sel: [2]int{-1}, focused: true},
			{name: "caret inside", text: "Hello", sel: [2]int{2, 2}, focused: true},
			{name: "selection", text: "Hello", sel: [2]int{1, 4}, focused: true},
			{name: "selection inactive", text: "Hello", sel: [2]int{1, 4}, focused: true, inactive: true},
			{name: "caret inactive", text: "Hello", sel: [2]int{-1}, focused: true, inactive: true},
			{name: "selection unfocused", text: "Hello", sel: [2]int{1, 4}},
			{name: "disabled", text: "Hello", sel: [2]int{-1}, disabled: true},
			{name: "disabled placeholder", ph: "Name", sel: [2]int{-1}, disabled: true},
			{name: "read-only focused", text: "Hello", sel: [2]int{-1}, focused: true, readOnly: true},
			{name: "read-only selection", text: "Hello", sel: [2]int{0, 5}, focused: true, readOnly: true},
			{name: "secure", secure: true, text: "pä!", sel: [2]int{-1}},
			{name: "secure focused", secure: true, text: "pä!", sel: [2]int{-1}, focused: true},
			{name: "secure selection", secure: true, text: "pä!", sel: [2]int{1, 3}, focused: true},
			{name: "overflow focused", text: "The quick brown fox jumps", sel: [2]int{-1}, focused: true},
			{name: "overflow caret start", text: "The quick brown fox jumps", sel: [2]int{0, 0}, focused: true},
			{name: "overflow unfocused", text: "The quick brown fox jumps", sel: [2]int{-1}},
			{name: "centred", text: "Hi", sel: [2]int{-1}, focused: true, align: driver.AlignCenter},
			{name: "trailing", text: "Hi", sel: [2]int{-1}, focused: true, align: driver.AlignTrailing},
			{name: "small", text: "Hello", sel: [2]int{-1}, focused: true, smallControl: true},
			{name: "increase contrast", text: "Hello", sel: [2]int{-1}, contrast: true},
			{name: "increase contrast, placeholder", ph: "Name", sel: [2]int{-1}, contrast: true},
			{name: "increase contrast, focused", text: "Hello", sel: [2]int{-1}, focused: true, contrast: true},
			{name: "increase contrast, selection", text: "Hello", sel: [2]int{1, 4}, focused: true, contrast: true},
			{name: "increase contrast, selection inactive", text: "Hello", sel: [2]int{1, 4}, focused: true, inactive: true, contrast: true},
			{name: "increase contrast, disabled", text: "Hello", sel: [2]int{-1}, disabled: true, contrast: true},
			{name: "composing", text: "ab", sel: [2]int{1, 1}, focused: true, preedit: "にほん", preCursor: 9},
			{name: "composing, selected clause", text: "ab", sel: [2]int{-1}, focused: true,
				preedit: "日本語", preCursor: 6, segs: []shell.PreeditSegment{{Start: 0, End: 6, Selected: true}, {Start: 6, End: 9}}},
			{name: "composing overflow", text: "The quick brown fox", sel: [2]int{-1}, focused: true,
				preedit: "じゃんぷした", preCursor: 18},
		} {
			tf, w, sf, s := fieldWindow(t, st.secure, nil)
			w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, IncreaseContrast: st.contrast})
			tf.SetPlaceholder(st.ph)
			tf.SetText(st.text)
			tf.SetAlignment(st.align)
			tf.SetEditable(!st.readOnly)
			if st.smallControl {
				tf.SetControlSize(driver.ControlSizeSmall)
				tf.SetFrame(driver.Rect{X: 10, Y: 10, W: 100, H: 19})
			}
			if st.sel[0] >= 0 {
				tf.Editor().SetSelection(st.sel[0], st.sel[1])
			}
			if st.focused && !tf.view.Focus() {
				t.Fatal("focus")
			}
			if st.preedit != "" {
				sf.SetTextInput(shell.TextInput{Enabled: true})
				sf.Preedit(st.preedit, st.preCursor, st.segs...)
			}
			tf.SetEnabled(!st.disabled)
			if st.inactive {
				s.Deactivate()
			}
			fmt.Fprintf(&out, "## %s, %s\n%s\n", st.name, tf.view.ControlState(), fieldLines(paint(t, s, sf)))
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "textfield.states")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to pin the dump)", err)
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("text field states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// TestTextFieldInWindow: the a11y tree of a focused field, hidden fields
// are not painted, Destroy leaves the parent.
func TestTextFieldInWindow(t *testing.T) {
	tf, w, sf, s := fieldWindow(t, false, nil)
	tf.SetPlaceholder("Name")
	tf.SetText("Ann")
	tf.Focus()
	tree := AccessibilityTree(w.Root())
	if n := tree.Find(a11y.RoleTextField, "Name"); n == nil || n.Value != "Ann" ||
		n.Frame != (driver.Rect{X: 10, Y: 38, W: 100, H: 22}) || n.States&a11y.StateFocused == 0 {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
	if tf.FocusCalls != 1 {
		t.Errorf("Focus calls %d", tf.FocusCalls)
	}
	tf.SetHidden(true)
	if got := paint(t, s, sf); strings.Contains(got, `"Ann"`) || tf.view.HasFocus() {
		t.Errorf("hidden field painted or focused:\n%s", got)
	}
	tf.Destroy()
	if !tf.Destroyed || ElementOf(tf).base().Parent() != nil {
		t.Error("Destroy")
	}
}

// textRect paints and returns the rect of the first text line of the dump
// that contains sub, or "".
func textRect(t *testing.T, s *fake.Shell, sf *fake.Surface, sub string) string {
	t.Helper()
	for _, l := range strings.Split(paint(t, s, sf), "\n") {
		if strings.HasPrefix(l, "text (") && strings.Contains(l, sub) {
			return strings.SplitN(l, " ", 3)[1]
		}
	}
	return ""
}
