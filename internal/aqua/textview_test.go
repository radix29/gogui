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

// viewLog counts the events of a text view.
type viewLog struct{ changed int }

func (l *viewLog) TextChanged() { l.changed++ }

// viewWindow opens a 200×100 window with a text view at content
// (10, 10, 180, 80), surface (10, 38, 180, 80), without the focus. Its
// viewport wraps after 24 fake runes (180 − 2·5 = 170 pt, 7 pt each).
func viewWindow(t *testing.T, h driver.TextViewHandler) (*TextView, *Window, *fake.Surface, *fake.Shell) {
	t.Helper()
	s := startShell(t)
	d := newDriver(s)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	v := d.NewTextView(h).(*TextView)
	w.SetContent(v)
	v.SetFrame(driver.Rect{X: 10, Y: 10, W: 180, H: 80})
	w.Router().SetFocus(nil)
	return v, w, sf, s
}

// numberedText is n paragraphs "Line 0" … "Line n−1".
func numberedText(n int) string {
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "Line %d", i)
	}
	return b.String()
}

func TestTextViewIsReal(t *testing.T) {
	d := startDriver(t)
	tv := d.NewTextView(nil).(*TextView)
	if e := ElementOf(tv); e == nil || e != Element(tv.view) || tv.StubPeer() != tv.TextView || tv.Stub() != &tv.TextView.Base {
		t.Error("ElementOf, StubPeer or Stub")
	}
	if e := ElementOf(tv).base(); !e.Focusable() || e.FocusKind() != FocusText {
		t.Error("not focusable as text")
	}
	if tv.Editor().SingleLine() {
		t.Error("the editor is single-line")
	}
	// FittingSize stays the stub's, so the example frames do not move.
	want := tv.TextView.FittingSize(infSize)
	if got := ElementOf(tv).FittingSize(infSize); got != want || tv.FittingSize(infSize) != want {
		t.Errorf("FittingSize %v, want the stub's %v", got, want)
	}
	if !tv.sv.HasVertical || tv.sv.HasHorizontal || tv.sv.view.Parent() != Element(tv.view) {
		t.Error("the internal scroll view")
	}
	// Made before the shell runs: no text system needed until it shows.
	if tv.lines != nil {
		t.Error("layouts made before the view showed")
	}
}

func TestTextViewNodes(t *testing.T) {
	v, w, _, _ := viewWindow(t, nil)
	if got := nodeOf(v); got != `textarea ""` {
		t.Errorf("node %q", got)
	}
	v.SetText("first\nsecond")
	v.SetEditable(false)
	if got := nodeOf(v); got != "textarea \"\" value=\"first\\nsecond\" [readonly]" {
		t.Errorf("node %q", got)
	}
	v.SetEditable(true)
	v.Focus()
	// The scroll view inside is not exposed: the text area has no children.
	tree := AccessibilityTree(w.Root())
	n := tree.Find(a11y.RoleTextArea, "")
	if n == nil || n.Frame != (driver.Rect{X: 10, Y: 38, W: 180, H: 80}) || n.States&a11y.StateFocused == 0 ||
		len(n.Children) != 0 || tree.Find(a11y.RoleScrollArea, "") != nil {
		t.Errorf("a11y tree:\n%s", tree.Dump())
	}
}

// TestTextViewText: SetText (caret at the end, no event, newlines
// normalized), Selection and SetSelection (ordered, clamped, at cluster
// starts), UserType with TextChanged, refused while read-only or disabled.
func TestTextViewText(t *testing.T) {
	h := &viewLog{}
	v, _, _, _ := viewWindow(t, h)
	v.SetText("ab\r\ncé")
	if got := v.Text(); got != "ab\ncé" {
		t.Errorf("Text %q", got)
	}
	if s, e := v.Selection(); s != 6 || e != 6 || h.changed != 0 {
		t.Errorf("after SetText: selection %d %d, %d events", s, e, h.changed)
	}
	v.SetSelection(5, 1)
	if s, e := v.Selection(); s != 1 || e != 4 { // 5 is inside "é"
		t.Errorf("SetSelection(5, 1): %d %d", s, e)
	}
	v.SetSelection(-3, 99)
	if s, e := v.Selection(); s != 0 || e != 6 {
		t.Errorf("SetSelection(-3, 99): %d %d", s, e)
	}
	v.SetSelection(2, 3)
	v.UserType("X")
	if v.Text() != "abXcé" || h.changed != 1 {
		t.Errorf("UserType: %q, %d events", v.Text(), h.changed)
	}
	if s, e := v.Selection(); s != 3 || e != 3 {
		t.Errorf("caret after UserType: %d %d", s, e)
	}
	v.SetEditable(false)
	v.UserType("Y")
	v.SetEditable(true)
	v.SetEnabled(false)
	v.UserType("Z")
	if v.Text() != "abXcé" || h.changed != 1 {
		t.Errorf("refused UserType: %q, %d events", v.Text(), h.changed)
	}
	// The stub keeps no copy of the text.
	if v.TextView.Value != "" {
		t.Errorf("stub Value %q", v.TextView.Value)
	}
}

// TestTextViewStates pins the paint of the view states (empty, wrapped
// text, caret, a selection across paragraphs, inactive, unfocused,
// read-only, disabled, a text color, scrolled to the middle, legacy
// scrollers) in light and dark, as record dumps in
// testdata/textview.states.
func TestTextViewStates(t *testing.T) {
	long := "The quick brown fox jumps over the lazy dog.\n\nSecond paragraph."
	var out strings.Builder
	for _, dark := range []bool{false, true} {
		for _, st := range []struct {
			name     string
			text     string
			sel      [2]int // -1: the caret at the end
			focused  bool
			inactive bool
			disabled bool
			readOnly bool
			color    driver.Color
			legacy   bool
			preedit  string // composed at the caret, its first clause selected
			contrast bool
		}{
			{name: "empty", sel: [2]int{-1}},
			{name: "empty focused", sel: [2]int{-1}, focused: true},
			{name: "wrapped", text: long, sel: [2]int{-1}},
			{name: "wrapped focused", text: long, sel: [2]int{-1}, focused: true},
			{name: "caret at a soft break", text: long, sel: [2]int{20, 20}, focused: true},
			{name: "selection", text: long, sel: [2]int{36, 50}, focused: true},
			{name: "selection inactive", text: long, sel: [2]int{36, 50}, focused: true, inactive: true},
			{name: "selection unfocused", text: long, sel: [2]int{36, 50}},
			{name: "read-only focused", text: long, sel: [2]int{4, 9}, focused: true, readOnly: true},
			{name: "read-only caret", text: long, sel: [2]int{-1}, focused: true, readOnly: true},
			{name: "disabled", text: long, sel: [2]int{4, 9}, disabled: true},
			{name: "text color", text: "Red text", sel: [2]int{-1}, color: driver.RGBA(1, 0, 0, 1)},
			{name: "scrolled to the middle", text: numberedText(40), sel: [2]int{150, 150}, focused: true},
			{name: "legacy scrollers", text: numberedText(40), sel: [2]int{-1}, legacy: true},
			{name: "composing", text: long, sel: [2]int{36, 36}, focused: true, preedit: "日本語です"},
			{name: "increased contrast, focused", text: long, sel: [2]int{-1}, focused: true, contrast: true},
			{name: "increased contrast, selection", text: long, sel: [2]int{36, 50}, focused: true, contrast: true},
			{name: "increased contrast, selection inactive", text: long, sel: [2]int{36, 50}, focused: true, inactive: true, contrast: true},
			{name: "increased contrast, legacy scrollers", text: numberedText(40), sel: [2]int{-1}, legacy: true, contrast: true},
		} {
			v, w, sf, s := viewWindow(t, nil)
			w.SetAppearance(driver.Appearance{Dark: dark, Accent: driver.DefaultAccent, LegacyScrollers: st.legacy, IncreaseContrast: st.contrast})
			v.SetText(st.text)
			v.SetTextColor(st.color)
			v.SetEditable(!st.readOnly)
			if st.sel[0] >= 0 {
				v.SetSelection(st.sel[0], st.sel[1])
			}
			if st.focused && !v.view.Focus() {
				t.Fatal("focus")
			}
			v.SetEnabled(!st.disabled)
			if st.inactive {
				s.Deactivate()
			}
			if st.preedit != "" {
				sf.Preedit(st.preedit, 6, shell.PreeditSegment{Start: 0, End: 6, Selected: true}, shell.PreeditSegment{Start: 6, End: len(st.preedit)})
			}
			fmt.Fprintf(&out, "## %s, %s, offset %v\n%s\n", st.name, v.view.ControlState(), v.Offset().Y, fieldLines(paint(t, s, sf)))
		}
	}
	got := out.String()
	path := filepath.Join("testdata", "textview.states")
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
		t.Errorf("text view states differ from %s (rerun with -update if the change is intended):\n%s", path, got)
	}
}

// TestTextViewFocusRing: the ring goes around the frame, drawn before the
// text (the element paints it, its children follow).
func TestTextViewFocusRing(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText("Hi")
	v.Focus()
	lines := strings.Split(fieldLines(paint(t, s, sf)), "\n")
	want := []string{
		"fillRRect (10,38,180,80) solid #ffffff",
		"strokeRRect (8.5,36.5,183,83) w=3 solid #007aff80",
	}
	if len(lines) < 3 || lines[0] != want[0] || lines[1] != want[1] || !strings.HasPrefix(lines[2], `text (15,43,`) {
		t.Errorf("paint:\n%s", strings.Join(lines, "\n"))
	}
}

// TestTextViewScrolling: SetSelection scrolls the range into view by the
// least amount, SetText scrolls to the top, and the document is as tall
// as the text plus the insets (at least the viewport).
func TestTextViewScrolling(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	if got := v.sv.DocumentSize; got != (driver.Size{W: 180, H: 80}) {
		t.Errorf("empty document %v", got)
	}
	v.SetText(numberedText(40)) // 40 lines of 16 pt
	if got := v.sv.DocumentSize; got != (driver.Size{W: 180, H: 650}) {
		t.Errorf("document %v", got)
	}
	if v.Offset().Y != 0 {
		t.Errorf("offset after SetText %v", v.Offset())
	}
	start := func(i int) int { return v.Editor().Buffer().ParagraphStart(i) }
	v.SetSelection(start(20), start(20)) // line 20: 325..341
	if got := v.Offset().Y; got != 341-80 {
		t.Errorf("caret on line 20: offset %v", got)
	}
	v.SetSelection(start(18), start(18)+1) // above the bottom, in view: no scroll
	if got := v.Offset().Y; got != 341-80 {
		t.Errorf("visible selection: offset %v", got)
	}
	v.SetSelection(start(2), start(3)) // above: its top
	if got := v.Offset().Y; got != 37 {
		t.Errorf("selection above: offset %v", got)
	}
	v.SetSelection(start(5), start(30)) // taller than the viewport: its start
	if got := v.Offset().Y; got != 85 {
		t.Errorf("tall selection: offset %v", got)
	}
	if dump := fieldLines(paint(t, s, sf)); !strings.Contains(dump, `"Line 5"`) || strings.Contains(dump, `"Line 4"`) {
		t.Errorf("painted:\n%s", dump)
	}
	v.SetText("short")
	if v.Offset().Y != 0 || v.sv.DocumentSize.H != 80 {
		t.Errorf("SetText: offset %v, document %v", v.Offset(), v.sv.DocumentSize)
	}
}

// TestTextViewWrapAnchor: a narrower frame wraps the text again and keeps
// the line at the top of the viewport where it was; so does a font change.
func TestTextViewWrapAnchor(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	para := strings.Repeat("word ", 12) // 60 runes: 3 lines at 24, 5 at 14
	v.SetText(strings.Repeat(para+"\n", 30))
	buf := v.Editor().Buffer()
	target := buf.ParagraphStart(15) + 25 // the second line of paragraph 15
	v.SetSelection(target, target)
	paint(t, s, sf)
	top := func() (int, float64) {
		a := v.anchor()
		if !a.ok {
			t.Fatal("no anchor")
		}
		return a.off, a.dy
	}
	off0, dy0 := top()
	made := v.lines.Made()
	v.SetFrame(driver.Rect{X: 10, Y: 10, W: 110, H: 80}) // 100 pt: 14 runes
	if off, dy := top(); off != off0 || dy != dy0 {
		t.Errorf("after the resize the top line starts at %d (dy %v), want %d (dy %v)", off, dy, off0, dy0)
	}
	if v.lines.Width() != 100 {
		t.Errorf("wrap width %v", v.lines.Width())
	}
	// Only the anchor's paragraph and the visible ones were laid out.
	if n := v.lines.Made() - made; n > 8 {
		t.Errorf("%d layouts made by the resize", n)
	}
	v.SetFont(driver.Font{Size: 26}) // twice the size: the line moves on, but stays on top
	if off, _ := top(); off > off0 || buf.ParagraphIndex(off) != buf.ParagraphIndex(off0) {
		t.Errorf("after the font change the top line starts at %d, before %d", off, off0)
	}
	if got := fieldLines(paint(t, s, sf)); !strings.Contains(got, " 26pt ") {
		t.Errorf("font not used:\n%s", got)
	}
}

// TestTextViewMegaPaint: showing, scrolling and resizing a 1 MB text lays
// out only the visible paragraphs (and the anchor's), never all of them.
func TestTextViewMegaPaint(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetFrame(driver.Rect{X: 10, Y: 10, W: 180, H: 600})
	before := v.lines.Made() // the empty paragraph
	v.SetText(megaText(1))
	paint(t, s, sf)
	idx := v.lines.Index()
	// The paragraphs that overlap the viewport.
	visible := func() int {
		top, bottom, _ := v.visible()
		n := 0
		for i := idx.At(max(top, 0)); i < idx.Len() && idx.Y(i) < bottom; i++ {
			if !idx.Exact(i) {
				t.Fatalf("visible paragraph %d not laid out", i)
			}
			n++
		}
		return n
	}
	if n, made := visible(), v.lines.Made()-before; made != n {
		t.Errorf("load: %d layouts made, %d paragraphs visible", made, n)
	}
	if ex := idx.Len() - idx.Inexact(); ex > 2*visible() {
		t.Errorf("%d of %d heights exact", ex, idx.Len())
	}
	made := v.lines.Made()
	mid := v.Editor().Len() / 2
	v.SetSelection(mid, mid)
	paint(t, s, sf)
	if n := v.lines.Made() - made; n > visible()+1 {
		t.Errorf("jump to the middle: %d layouts made, %d visible", n, visible())
	}
	made = v.lines.Made()
	v.SetFrame(driver.Rect{X: 10, Y: 10, W: 300, H: 600})
	paint(t, s, sf)
	if n := v.lines.Made() - made; n > visible()+1 {
		t.Errorf("resize: %d layouts made, %d visible", n, visible())
	}
	if v.lines.Cached() > v.lines.Bound() {
		t.Errorf("%d layouts cached, bound %d", v.lines.Cached(), v.lines.Bound())
	}
	t.Logf("%d paragraphs, %d visible, %d layouts made, %d cached", idx.Len(), visible(), v.lines.Made(), v.lines.Cached())
}

// TestTextViewSettled: once painted, a paint changes nothing (no extra
// frame), and a hidden view lays nothing out.
func TestTextViewSettled(t *testing.T) {
	v, _, sf, s := viewWindow(t, nil)
	v.SetText(numberedText(100))
	paint(t, s, sf)
	if sf.Invalid() {
		t.Error("the surface is invalid after a paint")
	}
	made := v.lines.Made()
	paint(t, s, sf)
	if v.lines.Made() != made || sf.Invalid() {
		t.Error("a second paint laid out or invalidated")
	}
}
