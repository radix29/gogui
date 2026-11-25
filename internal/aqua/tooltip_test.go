package aqua

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// Tooltips (PLAN.md S5.5l), over the fake shell's popups.

// tipWindow is a key window at (100, 100) of the screen with a container
// as its content and two tipped children, a at (10,10) and b at (70,10),
// 50×20 each, in the content's points.
type tipWindow struct {
	s    *fake.Shell
	w    *Window
	sf   *fake.Surface
	root *Container
	a, b *Container
}

func newTipWindow(t *testing.T) *tipWindow {
	t.Helper()
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	sf.UserMove(pt(100, 100))
	tw := &tipWindow{s: s, w: w, sf: sf, root: NewContainer(), a: NewContainer(), b: NewContainer()}
	tw.root.SetFrame(driver.Rect{W: 200, H: 100})
	tw.a.SetFrame(driver.Rect{X: 10, Y: 10, W: 50, H: 20})
	tw.b.SetFrame(driver.Rect{X: 70, Y: 10, W: 50, H: 20})
	tw.root.SetChildren([]driver.Widget{tw.a, tw.b})
	w.SetContent(tw.root)
	tw.a.SetTooltip("Alpha")
	tw.b.SetTooltip("Beta")
	if !w.IsKey() {
		t.Fatal("the window is not key")
	}
	return tw
}

// at returns the surface point of the content point (x, y).
func (tw *tipWindow) at(x, y float64) driver.Point { return pt(x, y+TitleBarHeight()) }

// overA and overB are points over a and b; nowhere is over neither.
func (tw *tipWindow) overA() driver.Point   { return tw.at(20, 20) }
func (tw *tipWindow) overB() driver.Point   { return tw.at(80, 20) }
func (tw *tipWindow) nowhere() driver.Point { return tw.at(150, 70) }

func (tw *tipWindow) advance(d time.Duration) { tw.s.FakeClock().Advance(d) }

// tip returns the shown tooltip's popup and its text, or nil and "".
func (tw *tipWindow) tip(t *testing.T) (*fake.Popup, string) {
	t.Helper()
	o := tw.w.tips.open
	var pops []*fake.Popup
	for _, p := range tw.s.Popups() {
		if !p.Grab() {
			pops = append(pops, p)
		}
	}
	if o == nil {
		if len(pops) != 0 {
			t.Fatalf("no tip, but %d non-grab popups", len(pops))
		}
		return nil, ""
	}
	if len(pops) != 1 || o.Popup() != shell.Popup(pops[0]) || o.Content() != Element(tw.w.tips.view) {
		t.Fatalf("tip %v, non-grab popups %v", o, pops)
	}
	return pops[0], tw.w.tips.view.Text()
}

// wantTip checks the shown tip's text ("" for none).
func (tw *tipWindow) wantTip(t *testing.T, what, text string) *fake.Popup {
	t.Helper()
	p, got := tw.tip(t)
	if got != text {
		t.Fatalf("%s: tip %q, want %q", what, got, text)
	}
	return p
}

var (
	tipDelay = func() time.Duration { return tokens.Tooltip.Delay }
	tipGrace = func() time.Duration { return tokens.Tooltip.Grace }
)

func TestTooltipAfterDelay(t *testing.T) {
	tw := newTipWindow(t)
	tw.sf.Move(tw.overA())
	tw.advance(tipDelay() - time.Millisecond)
	tw.wantTip(t, "before the delay", "")
	// A move over the same element restarts the wait.
	tw.sf.Move(tw.overA().Add(pt(5, 0)))
	tw.advance(tipDelay() - time.Millisecond)
	tw.wantTip(t, "after a move", "")
	tw.advance(time.Millisecond)
	pp := tw.wantTip(t, "after the delay", "Alpha")

	cfg := pp.Config()
	at := tw.overA().Add(pt(5, 0))
	tt := &tokens.Tooltip
	if cfg.Grab || cfg.Anchor != (driver.Rect{X: at.X, Y: at.Y}) || cfg.Offset != pt(tt.OffsetX, tt.OffsetY) ||
		cfg.Adjust != shell.AdjustSlideX|shell.AdjustSlideY {
		t.Errorf("config %+v", cfg)
	}
	l := tw.s.TextSystem().NewLayout("Alpha", driver.Font{Size: tt.FontSize}, 0)
	want := driver.Size{W: math.Ceil(l.Size().W + 2*tt.PaddingX), H: math.Ceil(l.Size().H + 2*tt.PaddingY)}
	if cfg.Size != want || pp.Rect() != driver.RectOf(at.Add(cfg.Offset), want) {
		t.Errorf("size %v at %v, want %v", cfg.Size, pp.Rect(), want)
	}
	// It takes no input: the window still gets the pointer.
	if tw.w.GrabPopupOpen() || tw.s.GrabPopup() != nil {
		t.Error("the tip is a grab popup")
	}
	paint(t, tw.s, tw.sf)
	d := pp.Dump()
	for _, s := range []string{`"Alpha"`, "fillRRect (0,0,", "strokeRRect"} {
		if !strings.Contains(d, s) {
			t.Errorf("tip paint lacks %s:\n%s", s, d)
		}
	}
	if strings.Contains(d, "shadow") {
		t.Errorf("own shadow where the shell draws one:\n%s", d)
	}

	// Moving over the element keeps the tip where it is.
	tw.sf.Move(tw.overA().Add(pt(10, 2)))
	tw.advance(tipDelay())
	if p := tw.wantTip(t, "moved over a", "Alpha"); p != pp || pp.Placements() != 1 {
		t.Errorf("the tip moved: %v, %d placements", p, pp.Placements())
	}
}

func TestTooltipGrace(t *testing.T) {
	tw := newTipWindow(t)
	tw.sf.Move(tw.overA())
	tw.advance(tipDelay())
	first := tw.wantTip(t, "a", "Alpha")
	// Straight to b: a's tip goes, b's shows at once.
	tw.sf.Move(tw.overB())
	if p := tw.wantTip(t, "b", "Beta"); p == first || !first.Closed() {
		t.Error("the tip was not replaced")
	}
	// Off both, back within the grace: at once.
	tw.sf.Move(tw.nowhere())
	tw.wantTip(t, "nowhere", "")
	tw.advance(tipGrace() - time.Millisecond)
	tw.sf.Move(tw.overA())
	tw.wantTip(t, "back within the grace", "Alpha")
	// Off again and past the grace: the delay again.
	tw.sf.Move(tw.nowhere())
	tw.advance(tipGrace())
	tw.sf.Move(tw.overB())
	tw.wantTip(t, "after the grace", "")
	tw.advance(tipDelay())
	tw.wantTip(t, "after the grace and the delay", "Beta")
}

func TestTooltipHiddenByInput(t *testing.T) {
	cases := []struct {
		name string
		do   func(tw *tipWindow)
	}{
		{"press", func(tw *tipWindow) { tw.sf.Click(tw.overA()) }},
		{"key", func(tw *tipWindow) { tw.sf.KeyDown(driver.Key('a'), 0) }},
		{"text", func(tw *tipWindow) { tw.sf.Type("x") }},
		{"scroll", func(tw *tipWindow) { tw.sf.Wheel(tw.overA(), pt(0, -1)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tw := newTipWindow(t)
			tw.sf.Move(tw.overA())
			tw.advance(tipDelay())
			tw.wantTip(t, "shown", "Alpha")
			c.do(tw)
			tw.wantTip(t, "hidden", "")
			// Muted until the pointer leaves a: no grace, no new wait.
			tw.sf.Move(tw.overA().Add(pt(3, 1)))
			tw.advance(2 * tipDelay())
			tw.wantTip(t, "muted", "")
			// b is a new target, and the grace ended with the hiding.
			tw.sf.Move(tw.overB())
			tw.wantTip(t, "b at once", "")
			tw.advance(tipDelay())
			tw.wantTip(t, "b", "Beta")
			// And a again after leaving it.
			tw.sf.Move(tw.overA())
			tw.wantTip(t, "a again", "Alpha")
		})
	}
}

// TestTooltipPressBeforeDelay: a press during the wait cancels it.
func TestTooltipPressBeforeDelay(t *testing.T) {
	tw := newTipWindow(t)
	tw.sf.Move(tw.overA())
	tw.advance(tipDelay() / 2)
	tw.sf.Press(tw.overA(), shell.ButtonPrimary)
	tw.advance(tipDelay())
	tw.wantTip(t, "pressed", "")
	// A drag over b with the button held arms nothing.
	tw.sf.Move(tw.overB())
	tw.advance(tipDelay())
	tw.wantTip(t, "dragged", "")
	tw.sf.Release(tw.overB(), shell.ButtonPrimary)
	tw.sf.Move(tw.overB().Add(pt(1, 0)))
	tw.advance(tipDelay())
	tw.wantTip(t, "after the drag", "Beta")
	if n := len(tw.s.Timers()); n != 0 {
		t.Errorf("%d timers left", n)
	}
}

func TestTooltipLeaveAndKeyLoss(t *testing.T) {
	tw := newTipWindow(t)
	tw.sf.Move(tw.overA())
	tw.advance(tipDelay())
	tw.wantTip(t, "shown", "Alpha")
	tw.sf.Move(pt(-50, -50)) // out of the window
	tw.wantTip(t, "left", "")
	tw.sf.Move(tw.overA())
	tw.wantTip(t, "back: no grace", "")
	tw.advance(tipDelay())
	tw.wantTip(t, "back", "Alpha")

	tw.s.Deactivate()
	tw.wantTip(t, "deactivated", "")
	tw.sf.Move(tw.overB())
	tw.advance(2 * tipDelay())
	tw.wantTip(t, "inactive window", "")
	tw.sf.Activate()
	tw.sf.Move(tw.overB().Add(pt(1, 0)))
	tw.advance(tipDelay())
	tw.wantTip(t, "active again", "Beta")

	tw.w.Close()
	if n := len(tw.s.Popups()); n != 0 || tw.w.tips.open != nil {
		t.Errorf("after the close: %d popups", n)
	}
	if n := len(tw.s.Timers()); n != 0 {
		t.Errorf("after the close: %d timers", n)
	}
}

// TestTooltipTree: the nearest ancestor's tip, disabled elements, and
// changes of the text and the tree.
func TestTooltipTree(t *testing.T) {
	tw := newTipWindow(t)
	child := NewContainer()
	child.SetFrame(driver.Rect{X: 5, Y: 5, W: 10, H: 10})
	tw.a.SetChildren([]driver.Widget{child})
	tw.sf.Move(tw.at(20, 20)) // over child
	tw.advance(tipDelay())
	tw.wantTip(t, "child without a tip", "Alpha")
	child.SetTooltip("Child")
	tw.wantTip(t, "child's own tip", "Child")
	// A new text for the shown element: in place, resized.
	pp, _ := tw.tip(t)
	child.SetTooltip("Child, longer")
	if p := tw.wantTip(t, "changed", "Child, longer"); p != pp || pp.Config().Size != tw.w.tips.view.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)}) {
		t.Errorf("not updated in place: %v, size %v", p, pp.Config().Size)
	}
	if pp.Rect().Size() != pp.Config().Size {
		t.Errorf("not re-placed: %v", pp.Rect())
	}
	// No text: back to a's at once (within the grace).
	child.SetTooltip("")
	tw.wantTip(t, "child's tip removed", "Alpha")
	// Disabled elements show theirs.
	tw.a.SetEnabled(false)
	tw.wantTip(t, "disabled", "Alpha")
	tw.a.SetEnabled(true)
	// Hiding or removing the shown element hides the tip.
	tw.a.SetHidden(true)
	tw.wantTip(t, "hidden", "")
	tw.a.SetHidden(false)
	tw.sf.Move(tw.at(21, 20))
	tw.advance(tipDelay())
	tw.wantTip(t, "shown again", "Alpha")
	tw.root.SetChildren([]driver.Widget{tw.b})
	tw.wantTip(t, "removed", "")
	// A pending wait for a removed element is dropped.
	tw.root.SetChildren([]driver.Widget{tw.a, tw.b})
	tw.advance(tipGrace())
	tw.sf.Move(tw.at(22, 20))
	if len(tw.s.Timers()) != 1 {
		t.Fatalf("timers %v", tw.s.Timers())
	}
	tw.a.RemoveFromParent()
	if n := len(tw.s.Timers()); n != 0 {
		t.Errorf("%d timers after the removal", n)
	}
	tw.advance(tipDelay())
	tw.wantTip(t, "removed while waiting", "")
}

// TestTooltipOwnShadowAndWrap: where the shell draws no shadow the view
// draws its own and the popup grows by its margin, the panel still at the
// offset; a long text wraps within MaxWidth.
func TestTooltipOwnShadowAndWrap(t *testing.T) {
	tw := newTipWindow(t)
	tw.s.SetCaps(shell.Caps{})
	long := strings.Repeat("word ", 40)
	tw.a.SetTooltip(long)
	tw.sf.Move(tw.overA())
	tw.advance(tipDelay())
	pp := tw.wantTip(t, "long", long)
	v := tw.w.tips.view
	in := v.ShadowMargin()
	tt := &tokens.Tooltip
	if in.Left != tt.ShadowBlur || in.Bottom != tt.ShadowBlur+tt.ShadowOffsetY {
		t.Errorf("margin %+v", in)
	}
	panel := v.Panel()
	if got := pp.Rect().Origin().Add(panel.Origin()); !near(got, tw.overA().Add(pt(tt.OffsetX, tt.OffsetY))) {
		t.Errorf("panel at %v", got)
	}
	if panel.W > tt.MaxWidth || v.layout.LineCount() < 2 {
		t.Errorf("panel %v, %d lines", panel, v.layout.LineCount())
	}
	paint(t, tw.s, tw.sf)
	if d := pp.Dump(); !strings.Contains(d, "shadow") {
		t.Errorf("no own shadow:\n%s", d)
	}
}

// TestTooltipSlides: at the screen's bottom-right corner the tip slides
// back onto the screen.
func TestTooltipSlides(t *testing.T) {
	tw := newTipWindow(t)
	tw.s.SetWorkArea(driver.Rect{W: 300, H: 260})
	tw.b.SetFrame(driver.Rect{X: 70, Y: 10, W: 130, H: 90})
	tw.sf.Move(tw.at(190, 95))
	tw.advance(tipDelay())
	pp := tw.wantTip(t, "corner", "Beta")
	r := pp.Rect()
	r.X += 100
	r.Y += 100
	if r.X+r.W > 300 || r.Y+r.H > 260 {
		t.Errorf("tip at %v (screen) off the work area", r)
	}
}

// TestTooltipAppearance: the tip repaints with its window's appearance.
func TestTooltipAppearance(t *testing.T) {
	tw := newTipWindow(t)
	tw.sf.Move(tw.overA())
	tw.advance(tipDelay())
	pp := tw.wantTip(t, "shown", "Alpha")
	paint(t, tw.s, tw.sf)
	light := pp.Dump()
	a := tw.w.Appearance()
	a.Dark = true
	tw.w.SetAppearance(a)
	paint(t, tw.s, tw.sf)
	dark := pp.Dump()
	if light == dark || !strings.Contains(dark, "#2c2c2e") {
		t.Errorf("light:\n%s\ndark:\n%s", light, dark)
	}
}

// TestTooltipPopUpButton: a peer's tooltip (stub and element), hidden by
// the press that opens its menu, and none while the menu is open.
func TestTooltipPopUpButton(t *testing.T) {
	p, w, sf, s := popUpMenuWindow(t, nil, "Red", "Green")
	p.SetTooltip("Colour")
	if p.Stub().Tooltip != "Colour" || p.view.Tooltip() != "Colour" {
		t.Fatalf("stub %q, element %q", p.Stub().Tooltip, p.view.Tooltip())
	}
	sf.Move(popUpAt)
	s.FakeClock().Advance(tipDelay())
	if w.tips.open == nil || w.tips.view.Text() != "Colour" {
		t.Fatal("no tip over the button")
	}
	sf.Click(popUpAt)
	menuPopup(t, p, s)
	if w.tips.open != nil {
		t.Error("the tip stayed with the menu open")
	}
	sf.Move(popUpAt.Add(pt(2, 0)))
	s.FakeClock().Advance(2 * tipDelay())
	if w.tips.open != nil {
		t.Error("a tip during the menu")
	}
	// Space opens it from the keyboard too: the tip goes first.
	p.CloseMenu()
	sf.Move(popUpAt.Add(pt(-40, 30))) // off the button
	sf.Move(popUpAt)
	s.FakeClock().Advance(tipDelay())
	if w.tips.open == nil {
		t.Fatal("no tip after the menu")
	}
	p.view.Focus()
	sf.KeyDown(driver.KeySpace, 0)
	menuPopup(t, p, s)
	if w.tips.open != nil {
		t.Error("the tip stayed with the menu opened by Space")
	}
}
