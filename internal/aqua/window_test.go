package aqua

import (
	"errors"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/theme"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/fake"
)

// winLog is a driver.WindowHandler that logs its calls.
type winLog struct {
	log      []string
	refuse   bool // ShouldClose result is !refuse
	resizes  []driver.Size
	onResize func(driver.Size)
}

func (h *winLog) ContentResized(s driver.Size) {
	h.resizes = append(h.resizes, s)
	h.log = append(h.log, "resized")
	if h.onResize != nil {
		h.onResize(s)
	}
}
func (h *winLog) ShouldClose() bool { h.log = append(h.log, "shouldClose"); return !h.refuse }
func (h *winLog) Closed()           { h.log = append(h.log, "closed") }
func (h *winLog) KeyChanged(k bool) {
	if k {
		h.log = append(h.log, "key")
	} else {
		h.log = append(h.log, "unkey")
	}
}

func (h *winLog) take() string {
	s := strings.Join(h.log, " ")
	h.log = nil
	return s
}

type shellApp struct{}

func (shellApp) Ready()                              {}
func (shellApp) AppearanceChanged(driver.Appearance) {}

// startShell returns a fake shell started on the test goroutine.
func startShell(t *testing.T) *fake.Shell {
	t.Helper()
	s := fake.New()
	if err := s.Start(shellApp{}); err != nil {
		t.Fatal(err)
	}
	return s
}

const allStyles = driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable

// openWindow creates and shows a window, paints it once and clears the log.
func openWindow(t *testing.T, s *fake.Shell, cfg driver.WindowConfig) (*Window, *fake.Surface, *winLog) {
	t.Helper()
	h := &winLog{}
	w, err := NewWindow(s, cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	sf := w.Surface().(*fake.Surface)
	w.Show()
	s.FakeClock().Flush()
	if err := sf.PaintErr(); err != nil {
		t.Fatal(err)
	}
	h.take()
	return w, sf, h
}

func paint(t *testing.T, s *fake.Shell, sf *fake.Surface) string {
	t.Helper()
	s.FakeClock().Flush()
	if err := sf.PaintErr(); err != nil {
		t.Fatalf("paint errors: %v\n%s", err, sf.Dump())
	}
	return sf.Dump()
}

func pt(x, y float64) driver.Point { return driver.Point{X: x, Y: y} }

// Centres of the traffic lights in surface coordinates.
var (
	closeAt = pt(14, 14)
	minAt   = pt(34, 14)
	zoomAt  = pt(54, 14)
)

func TestWindowContentCoordinates(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{
		Title: "W", Size: driver.Size{W: 300, H: 200},
		MinSize: driver.Size{W: 100, H: 50}, MaxSize: driver.Size{W: 400, H: 300},
		Style: allStyles, Center: true,
	})
	if got := sf.Size(); got != (driver.Size{W: 300, H: 228}) {
		t.Errorf("surface size %v", got)
	}
	if got := w.ContentSize(); got != (driver.Size{W: 300, H: 200}) {
		t.Errorf("content size %v", got)
	}
	if sf.MinSize() != (driver.Size{W: 100, H: 78}) || sf.MaxSize() != (driver.Size{W: 400, H: 328}) {
		t.Errorf("surface limits %v %v", sf.MinSize(), sf.MaxSize())
	}
	if sf.Centered() != 1 || sf.Title() != "W" || sf.Style() != allStyles {
		t.Errorf("config not passed: centered %d title %q style %v", sf.Centered(), sf.Title(), sf.Style())
	}
	if got := w.content.Frame(); got != (driver.Rect{Y: 28, W: 300, H: 200}) {
		t.Errorf("content frame %v", got)
	}

	// SetContentSize clamps in content coordinates and reports once.
	w.SetContentSize(driver.Size{W: 1000, H: 10})
	if got := w.ContentSize(); got != (driver.Size{W: 400, H: 50}) {
		t.Errorf("clamped content size %v", got)
	}
	if h.take() != "resized" || h.resizes[len(h.resizes)-1] != (driver.Size{W: 400, H: 50}) {
		t.Errorf("resizes %v", h.resizes)
	}
	// The same size again still reports ContentResized (the driver contract).
	w.SetContentSize(driver.Size{W: 400, H: 50})
	if got := h.take(); got != "resized" {
		t.Errorf("same size: %q", got)
	}

	// The user resizes the surface: ContentResized in content coordinates.
	sf.UserResize(driver.Size{W: 250, H: 128})
	if got := h.resizes[len(h.resizes)-1]; got != (driver.Size{W: 250, H: 100}) || h.take() != "resized" {
		t.Errorf("user resize: %v", got)
	}
	// A scale change alone does not change the content size.
	sf.SetScale(2)
	if got := h.take(); got != "" {
		t.Errorf("scale change: %q", got)
	}

	// New limits; the current size violates the new minimum.
	w.SetMinSize(driver.Size{W: 260, H: 120})
	if w.MinSize() != (driver.Size{W: 260, H: 120}) || sf.MinSize() != (driver.Size{W: 260, H: 148}) {
		t.Errorf("min %v surface %v", w.MinSize(), sf.MinSize())
	}
	if got := w.ContentSize(); got != (driver.Size{W: 260, H: 120}) || h.take() != "resized" {
		t.Errorf("after SetMinSize: %v", got)
	}
	w.SetMaxSize(driver.Size{W: 0, H: 0}) // no limit
	if w.MaxSize() != (driver.Size{}) || sf.MaxSize() != (driver.Size{}) {
		t.Errorf("max %v surface %v", w.MaxSize(), sf.MaxSize())
	}
	w.SetMinSize(driver.Size{})
	if sf.MinSize() != (driver.Size{H: 28}) {
		t.Errorf("zero min keeps the title bar: %v", sf.MinSize())
	}

	w.Center()
	if sf.Centered() != 2 {
		t.Errorf("Center not passed on")
	}
	w.SetTitle("New")
	if sf.Title() != "New" || w.Title() != "New" {
		t.Errorf("title %q %q", sf.Title(), w.Title())
	}
}

func TestWindowUntitled(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Size: driver.Size{W: 120, H: 80}, Style: driver.StyleResizable})
	if sf.Size() != (driver.Size{W: 120, H: 80}) || w.ContentSize() != sf.Size() {
		t.Errorf("sizes %v %v", sf.Size(), w.ContentSize())
	}
	if sf.ChromeRegions() != nil || sf.HitTest(pt(10, 10)) != shell.RegionClient {
		t.Errorf("untitled window has regions %v", sf.ChromeRegions())
	}
	if f, txt := w.TitleFrame(); txt != "" || f != (driver.Rect{}) {
		t.Errorf("title %v %q", f, txt)
	}
	checkDump(t, sf.Dump(), "fillRRect (0,0,120,80) solid #ececec")
	// OS close is refused without StyleClosable, and ShouldClose is not asked.
	if sf.UserClose() || h.take() != "" {
		t.Errorf("untitled, unclosable window closed")
	}
	w.SetMinSize(driver.Size{W: 10, H: 10})
	if sf.MinSize() != (driver.Size{W: 10, H: 10}) {
		t.Errorf("min %v", sf.MinSize())
	}
}

func TestChromeRegions(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	want := []shell.ChromeRegion{
		{Rect: driver.Rect{W: 300, H: 28}, Kind: shell.RegionCaption},
		{Rect: driver.Rect{X: 8, Y: 8, W: 12, H: 12}, Kind: shell.RegionClose},
		{Rect: driver.Rect{X: 28, Y: 8, W: 12, H: 12}, Kind: shell.RegionMinimize},
		{Rect: driver.Rect{X: 48, Y: 8, W: 12, H: 12}, Kind: shell.RegionMaximize},
	}
	checkRegions(t, sf.ChromeRegions(), want)
	for _, c := range []struct {
		p    driver.Point
		want shell.RegionKind
	}{
		{closeAt, shell.RegionClose}, {minAt, shell.RegionMinimize}, {zoomAt, shell.RegionMaximize},
		{pt(24, 14), shell.RegionCaption}, {pt(150, 5), shell.RegionCaption}, {pt(299, 27), shell.RegionCaption},
		{pt(150, 28), shell.RegionClient}, {pt(5, 100), shell.RegionClient},
	} {
		if got := sf.HitTest(c.p); got != c.want {
			t.Errorf("HitTest(%v) = %v, want %v", c.p, got, c.want)
		}
	}

	// Recomputed on resize.
	sf.UserResize(driver.Size{W: 500, H: 300})
	want[0].Rect.W = 500
	checkRegions(t, sf.ChromeRegions(), want)
	w.SetContentSize(driver.Size{W: 90, H: 40})
	want[0].Rect.W = 90
	checkRegions(t, sf.ChromeRegions(), want)

	// Lights whose style flag is missing stay caption.
	_, sf2, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: driver.StyleTitled | driver.StyleClosable})
	checkRegions(t, sf2.ChromeRegions(), shell.ChromeRegion{Rect: driver.Rect{W: 300, H: 28}, Kind: shell.RegionCaption}, want[1])
	if sf2.HitTest(zoomAt) != shell.RegionCaption {
		t.Errorf("disabled zoom light is not caption")
	}
}

func checkRegions(t *testing.T, got []shell.ChromeRegion, want ...any) {
	t.Helper()
	var flat []shell.ChromeRegion
	for _, w := range want {
		switch w := w.(type) {
		case []shell.ChromeRegion:
			flat = append(flat, w...)
		case shell.ChromeRegion:
			flat = append(flat, w)
		}
	}
	if len(got) != len(flat) {
		t.Fatalf("regions %v, want %v", got, flat)
	}
	for i := range got {
		if got[i] != flat[i] {
			t.Fatalf("regions %v, want %v", got, flat)
		}
	}
}

func TestTrafficLightActions(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})

	// Green zooms and restores.
	sf.Click(zoomAt)
	if !sf.Zoomed() || w.ContentSize() != (driver.Size{W: 1440, H: 872}) || h.take() != "resized" {
		t.Errorf("zoom: zoomed %v size %v", sf.Zoomed(), w.ContentSize())
	}
	if got := sf.ChromeRegions()[0].Rect.W; got != 1440 {
		t.Errorf("regions not recomputed after zoom: %v", got)
	}
	s.FakeClock().Advance(fake.DoubleClickTime) // no double click
	sf.Click(zoomAt)
	if sf.Zoomed() || w.ContentSize() != (driver.Size{W: 300, H: 200}) {
		t.Errorf("unzoom: %v", w.ContentSize())
	}
	h.take()

	// Dragging off a light and releasing outside does nothing.
	s.FakeClock().Advance(fake.DoubleClickTime)
	sf.Drag(closeAt, pt(150, 100), pt(150, 100))
	if sf.Closed() || h.take() != "" {
		t.Errorf("drag-off closed the window")
	}
	// Dragging off and back on acts.
	sf.Drag(minAt, minAt, pt(100, 100))
	if !sf.Minimized() {
		t.Errorf("yellow did not minimize")
	}
	if h.take() != "unkey" {
		t.Errorf("minimize log")
	}
	sf.Activate()
	h.take()

	// A secondary click does nothing.
	sf.ClickButton(closeAt, shell.ButtonSecondary)
	if sf.Closed() || h.take() != "" {
		t.Errorf("secondary click acted")
	}

	// Red asks ShouldClose; refused keeps the window.
	h.refuse = true
	s.FakeClock().Advance(fake.DoubleClickTime)
	sf.Click(closeAt)
	if sf.Closed() || h.take() != "shouldClose" {
		t.Errorf("refused close")
	}
	h.refuse = false
	s.FakeClock().Advance(fake.DoubleClickTime)
	sf.Click(closeAt)
	if !sf.Closed() || !w.Closed() || h.take() != "shouldClose closed" {
		t.Errorf("close: %v", h.log)
	}
	// Everything is inert afterwards.
	w.SetTitle("x")
	w.SetContentSize(driver.Size{W: 1, H: 1})
	w.SetMinSize(driver.Size{W: 1, H: 1})
	w.SetMaxSize(driver.Size{W: 1, H: 1})
	w.SetContent(nil)
	w.SetAppearance(driver.Appearance{Dark: true})
	w.Center()
	w.Show()
	w.Close()
	if h.take() != "" || w.Title() != "W" || w.IsKey() {
		t.Errorf("closed window reacted")
	}
}

func TestDisabledLights(t *testing.T) {
	s := startShell(t)
	_, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: driver.StyleTitled})
	sf.Click(closeAt)
	sf.Click(minAt)
	sf.Click(zoomAt)
	if sf.Closed() || sf.Minimized() || sf.Zoomed() || h.take() != "" {
		t.Errorf("disabled lights acted")
	}
	// Disabled lights are caption, so the presses went to the OS; without
	// StyleClosable an OS close is refused.
	if sf.UserClose() {
		t.Errorf("OS close of an unclosable window")
	}
}

func TestOSCloseAsksShouldClose(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	h.refuse = true
	if sf.UserClose() || h.take() != "shouldClose" {
		t.Errorf("refused OS close")
	}
	h.refuse = false
	if !sf.UserClose() || h.take() != "shouldClose closed" || !w.Closed() {
		t.Errorf("OS close")
	}

	// Close does not ask; a ShouldClose handler that closes the window itself
	// does not close twice.
	w2, _, h2 := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	w2.Close()
	if h2.take() != "closed" {
		t.Errorf("Close log")
	}
	w3, sf3, h3 := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	sf3.Click(closeAt)
	_ = w3
	if h3.take() != "shouldClose closed" {
		t.Errorf("close log")
	}
}

func TestKeyState(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 300, H: 200}, Style: allStyles})
	if !w.IsKey() {
		t.Fatalf("shown window not key")
	}
	s.Deactivate()
	if w.IsKey() || h.take() != "unkey" {
		t.Errorf("deactivate")
	}
	if !sf.Invalid() {
		t.Errorf("key change does not repaint")
	}
	sf.Activate()
	if !w.IsKey() || h.take() != "key" {
		t.Errorf("activate")
	}
}

func TestChromeDump(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "Hello", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	const (
		content = "fillRRect (0,28,200,100) solid #ececec"
	)
	checkDump(t, paint(t, s, sf),
		content,
		"fillRRect (0,0,200,28) solid #e5e5e5",
		"fillRRect (0,27,200,1) solid #0000001a",
		`text (82.5,6,35,16) "Hello" 13pt w600 #000000d9`,
		"fillRRect (8,8,12,12) r=6 solid #ff5f57",
		"strokeRRect (8.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #e0443e",
		"fillRRect (28,8,12,12) r=6 solid #febc2e",
		"strokeRRect (28.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #dea123",
		"fillRRect (48,8,12,12) r=6 solid #28c840",
		"strokeRRect (48.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #1aab29",
	)

	// Hovering the group (a gap between two lights counts) shows the glyphs.
	sf.Move(pt(24, 14))
	checkDump(t, paint(t, s, sf),
		content,
		"fillRRect (0,0,200,28) solid #e5e5e5",
		"fillRRect (0,27,200,1) solid #0000001a",
		`text (82.5,6,35,16) "Hello" 13pt w600 #000000d9`,
		"fillRRect (8,8,12,12) r=6 solid #ff5f57",
		"strokeRRect (8.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #e0443e",
		"strokePath [M(11.5,11.5) L(16.5,16.5) M(16.5,11.5) L(11.5,16.5)] w=1.2 cap=round solid #4d0000",
		"fillRRect (28,8,12,12) r=6 solid #febc2e",
		"strokeRRect (28.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #dea123",
		"strokePath [M(31,14) L(37,14)] w=1.2 cap=round solid #995700",
		"fillRRect (48,8,12,12) r=6 solid #28c840",
		"strokeRRect (48.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #1aab29",
		"strokePath [M(51,14) L(57,14) M(54,11) L(54,17)] w=1.2 cap=round solid #006500",
	)

	// Pressed is darker; dragging off un-presses.
	sf.Press(closeAt, shell.ButtonPrimary)
	d := paint(t, s, sf)
	if !strings.Contains(d, "fillRRect (8,8,12,12) r=6 solid #d14e47") {
		t.Errorf("pressed close:\n%s", d)
	}
	sf.Move(pt(150, 60))
	d = paint(t, s, sf)
	if !strings.Contains(d, "fillRRect (8,8,12,12) r=6 solid #ff5f57") || !strings.Contains(d, "strokePath") {
		t.Errorf("dragged off (unpressed, glyphs kept during the capture):\n%s", d)
	}
	sf.Release(pt(150, 60), shell.ButtonPrimary)
	if d := paint(t, s, sf); strings.Contains(d, "strokePath") {
		t.Errorf("glyphs after release outside:\n%s", d)
	}

	// Inactive: grey lights, tertiary title, lighter bar.
	s.Deactivate()
	checkDump(t, paint(t, s, sf),
		content,
		"fillRRect (0,0,200,28) solid #f3f3f3",
		"fillRRect (0,27,200,1) solid #0000001a",
		`text (82.5,6,35,16) "Hello" 13pt w600 #00000042`,
		"fillRRect (8,8,12,12) r=6 solid #dcdcdc",
		"strokeRRect (8.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #c9c9c9",
		"fillRRect (28,8,12,12) r=6 solid #dcdcdc",
		"strokeRRect (28.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #c9c9c9",
		"fillRRect (48,8,12,12) r=6 solid #dcdcdc",
		"strokeRRect (48.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #c9c9c9",
	)
	// Hovering an inactive window's lights colours them, as on macOS.
	sf.Move(minAt)
	d = paint(t, s, sf)
	if !strings.Contains(d, "fillRRect (28,8,12,12) r=6 solid #febc2e") || !strings.Contains(d, "solid #995700") {
		t.Errorf("hover in inactive window:\n%s", d)
	}
	sf.Move(pt(150, 60))

	// Dark, key.
	sf.Activate()
	w.SetAppearance(driver.Appearance{Dark: true, Accent: driver.DefaultAccent})
	if !w.Appearance().Dark {
		t.Errorf("appearance not kept")
	}
	checkDump(t, paint(t, s, sf),
		"fillRRect (0,28,200,100) solid #323232",
		"fillRRect (0,0,200,28) solid #383838",
		"fillRRect (0,27,200,1) solid #000000",
		`text (82.5,6,35,16) "Hello" 13pt w600 #ffffffd9`,
		"fillRRect (8,8,12,12) r=6 solid #ff5f57",
		"strokeRRect (8.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #e0443e",
		"fillRRect (28,8,12,12) r=6 solid #febc2e",
		"strokeRRect (28.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #dea123",
		"fillRRect (48,8,12,12) r=6 solid #28c840",
		"strokeRRect (48.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #1aab29",
	)
	// Dark, inactive.
	s.Deactivate()
	d = paint(t, s, sf)
	for _, want := range []string{
		"fillRRect (0,0,200,28) solid #2b2b2b", `#ffffff40`, "fillRRect (8,8,12,12) r=6 solid #545454",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("dark inactive lacks %q:\n%s", want, d)
		}
	}
	// The same appearance again does not repaint.
	paints := sf.Paints()
	w.SetAppearance(w.Appearance())
	s.FakeClock().Flush()
	if sf.Paints() != paints {
		t.Errorf("same appearance repainted")
	}
}

func TestDisabledLightsDump(t *testing.T) {
	s := startShell(t)
	_, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "", Size: driver.Size{W: 200, H: 100}, Style: driver.StyleTitled | driver.StyleClosable})
	sf.Move(closeAt)
	checkDump(t, paint(t, s, sf),
		"fillRRect (0,28,200,100) solid #ececec",
		"fillRRect (0,0,200,28) solid #e5e5e5",
		"fillRRect (0,27,200,1) solid #0000001a",
		"fillRRect (8,8,12,12) r=6 solid #ff5f57",
		"strokeRRect (8.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #e0443e",
		"strokePath [M(11.5,11.5) L(16.5,16.5) M(16.5,11.5) L(11.5,16.5)] w=1.2 cap=round solid #4d0000",
		"fillRRect (28,8,12,12) r=6 solid #dcdcdc80",
		"strokeRRect (28.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #c9c9c980",
		"fillRRect (48,8,12,12) r=6 solid #dcdcdc80",
		"strokeRRect (48.25,8.25,11.5,11.5) r=5.75 w=0.5 solid #c9c9c980",
	)
}

func TestTitleTruncation(t *testing.T) {
	s := startShell(t)
	// Title font: 7 pt per rune (fake metrics), the margin is 8+52+8 = 68 pt
	// on each side.
	for _, c := range []struct {
		width float64
		title string
		want  string
		x     float64
	}{
		{300, "Hello", "Hello", 132.5},
		{136 + 35, "Hello", "Hello", 68},     // exactly fits
		{136 + 34, "Hello", "Hel…", 71},      // "Hel…" is 28 pt
		{136 + 27, "Hello", "He…", 71},       // 21 pt
		{136 + 7, "Hello", "…", 68},          // only the ellipsis
		{136 + 6, "Hello", "", 0},            // hidden
		{100, "Hello", "", 0},                // no room at all
		{136 + 35, "Ab   cdefgh", "Ab…", 75}, // trailing spaces dropped before the ellipsis
		{300, "Two\nlines", "Two lines", 118.5},
		{136 + 41, "Grüße aus Köln", "Grüß…", 71},
		{300, "", "", 0},
	} {
		w, _, _ := openWindow(t, s, driver.WindowConfig{Title: c.title, Size: driver.Size{W: c.width, H: 50}, Style: allStyles})
		f, txt := w.TitleFrame()
		if txt != c.want || (txt != "" && (f.X != c.x || f.Y != 6 || f.H != 16)) {
			t.Errorf("width %g %q: got %q at %v, want %q at x=%g", c.width, c.title, txt, f, c.want, c.x)
		}
		w.Close()
	}

	// The title follows SetTitle and resizes.
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "Hello", Size: driver.Size{W: 300, H: 50}, Style: allStyles})
	w.SetTitle("A much longer title")
	if _, txt := w.TitleFrame(); txt != "A much longer title" {
		t.Errorf("SetTitle: %q", txt)
	}
	w.SetTitle("A much longer title") // no change
	sf.UserResize(driver.Size{W: 200, H: 78})
	if _, txt := w.TitleFrame(); txt != "A much l…" {
		t.Errorf("resized: %q", txt)
	}
	if d := paint(t, s, sf); !strings.Contains(d, `"A much l…"`) {
		t.Errorf("dump lacks truncated title:\n%s", d)
	}
}

func TestWindowContent(t *testing.T) {
	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	if w.Content() != nil || w.Root() == nil || w.Router() == nil {
		t.Fatalf("initial content")
	}
	var log []string
	p := newProbe(&log, "p", 10, 10, 50, 20, "down")
	p.SetFocusable(true)
	p.node = a11y.Node{Role: a11y.RoleButton, Label: "OK"}
	w.SetContent(contentWidget{p})
	if w.Content() != Element(p) {
		t.Fatalf("content not set")
	}
	// Content coordinates: the probe at (10,10) in the content is at
	// (10,38) on the surface.
	sf.Click(pt(15, 45))
	checkLog(t, &log, "p enter 5,7", "p move 5,7", "p down 5,7 x1", "p up 5,7 x1")
	// Content is clipped to the content area: a part above it is not hit.
	p.SetFrame(driver.Rect{X: 100, Y: -10, W: 50, H: 30})
	sf.Move(pt(120, 20))
	checkLog(t, &log, "p leave 20,2")
	// Key events reach the focused content.
	p.Focus()
	checkLog(t, &log, "p focus true")
	sf.Key(driver.KeyReturn, 0)
	checkLog(t, &log, "p key 13 true", "p key 13 false")
	_ = log

	// The a11y tree: window, title bar with its lights, then the content.
	tree := AccessibilityTree(w.Root()).Dump()
	want := strings.Join([]string{
		`window "W" [focused] @0,0 200x128`,
		`  button "OK" [focusable focused] {focus} @100,18 50x30`,
		`  titlebar "W" @0,0 200x28`,
		`    button "Close" {press} @8,8 12x12`,
		`    button "Minimize" {press} @28,8 12x12`,
		`    button "Zoom" {press} @48,8 12x12`,
	}, "\n")
	if tree != want {
		t.Errorf("a11y:\n%s\nwant:\n%s", tree, want)
	}

	// A widget that is not an element shows nothing; nil removes.
	w.SetContent(stubWidget{})
	if w.Content() != nil {
		t.Errorf("non-element content kept")
	}
	w.SetContent(contentWidget{p})
	w.SetContent(nil)
	if w.Content() != nil || p.Parent() != nil {
		t.Errorf("nil content")
	}
	if err := sf.PaintErr(); err != nil {
		t.Error(err)
	}
}

// contentWidget is a probe that also is a driver.Widget.
type contentWidget struct{ *probe }

func (contentWidget) SetEnabled(bool)                   {}
func (contentWidget) SetHidden(bool)                    {}
func (contentWidget) SetControlSize(driver.ControlSize) {}
func (contentWidget) SetTooltip(string)                 {}
func (contentWidget) Focus()                            {}
func (contentWidget) Destroy()                          {}

// stubWidget is a driver.Widget that is no element.
type stubWidget struct{}

func (stubWidget) SetFrame(driver.Rect)                {}
func (stubWidget) FittingSize(driver.Size) driver.Size { return driver.Size{} }
func (stubWidget) SetEnabled(bool)                     {}
func (stubWidget) SetHidden(bool)                      {}
func (stubWidget) SetControlSize(driver.ControlSize)   {}
func (stubWidget) SetTooltip(string)                   {}
func (stubWidget) Focus()                              {}
func (stubWidget) Destroy()                            {}

func TestResizeReentrancy(t *testing.T) {
	s := startShell(t)
	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	// A ContentResized handler that closes the window.
	h.onResize = func(driver.Size) { w.Close() }
	w.SetContentSize(driver.Size{W: 300, H: 100})
	if !w.Closed() || h.take() != "resized closed" {
		t.Errorf("close during resize: %v", h.log)
	}
	_ = sf
}

func TestThemeColors(t *testing.T) {
	light := driver.DefaultAppearance()
	dark := driver.Appearance{Dark: true, Accent: driver.RGB8(1, 2, 3)}
	if c := resolveColor(driver.RGB8(1, 2, 3), light); c != driver.RGB8(1, 2, 3) {
		t.Errorf("concrete color changed: %v", c)
	}
	if c := semantic(driver.ColorControlAccent, dark); c != dark.Accent {
		t.Errorf("accent %v", c)
	}
	if c := semantic(driver.ColorKeyboardFocusIndicator, light); c.A != 0.5 || c.R != light.Accent.R {
		t.Errorf("focus %v", c)
	}
	if semantic(driver.ColorText, light) == semantic(driver.ColorText, dark) {
		t.Errorf("text same in light and dark")
	}
	if c := semantic(driver.SemanticColor(driver.NumSemanticColors+1), light); c != theme.Unknown {
		t.Errorf("unknown %v", c)
	}
	if c := semantic(driver.ColorWindowBackground, dark); c != driver.RGB8(50, 50, 50) {
		t.Errorf("window background %v", c)
	}
	if tokens != &theme.Sequoia {
		t.Errorf("the driver does not draw with theme.Sequoia")
	}
}

// failShell is a shell whose NewSurface fails.
type failShell struct{ *fake.Shell }

var errNoSurface = errors.New("no surface")

func (failShell) NewSurface(shell.SurfaceConfig, shell.SurfaceHandler) (shell.Surface, error) {
	return nil, errNoSurface
}

func TestWindowEdgeCases(t *testing.T) {
	s := startShell(t)
	if _, err := NewWindow(failShell{s}, driver.WindowConfig{}, &winLog{}); err != errNoSurface {
		t.Errorf("NewSurface error: %v", err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Errorf("nil handler did not panic")
			}
		}()
		NewWindow(s, driver.WindowConfig{}, nil)
	}()

	w, sf, h := openWindow(t, s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	sh := surfaceHandler{w}
	sh.KeyChanged(true) // already key
	if h.take() != "" {
		t.Errorf("repeated key state reported")
	}
	// Non-pointer events and other buttons on a light.
	l := w.bar.lights[lightClose]
	if l.HandleEvent(FocusEvent{Focused: true}) || l.HandleEvent(PointerEvent{Kind: PointerMove}) {
		t.Errorf("light handled an idle event")
	}
	sf.Press(closeAt, shell.ButtonPrimary)
	if !l.HandleEvent(PointerEvent{Kind: PointerDown, Button: shell.ButtonSecondary}) ||
		!l.HandleEvent(PointerEvent{Kind: PointerUp, Button: shell.ButtonSecondary}) {
		t.Errorf("captured light dropped a second button")
	}
	sf.CancelCapture() // PointerUp with Over: the close is asked
	h.take()
	l.HandleEvent(PointerEvent{Kind: PointerDown, Button: shell.ButtonPrimary})
	l.HandleEvent(PointerEvent{Kind: PointerCancel})
	if l.press.Down() || l.press.Pressed() {
		t.Errorf("cancel kept the press")
	}
	w.bar.group.setHovered(w.bar.group.hovered)

	w.Close()
	h.take()
	sh.Resized(driver.Size{W: 1, H: 1}, 1)
	sh.Closed()
	sh.KeyChanged(true)
	if sh.Event(shell.KeyEvent{Down: true, Key: driver.KeyTab}) || sh.CloseRequested() || h.take() != "" {
		t.Errorf("closed window reacted")
	}
}

// TestChromeReadsTheme draws a window with altered chrome metrics and title
// weight and checks the title bar, the lights, their regions and the title
// follow the tokens (S3.1c), not constants.
func TestChromeReadsTheme(t *testing.T) {
	alt := theme.Sequoia
	alt.Chrome.TitleBarHeight = 40
	alt.Chrome.LightSize = 16
	alt.Chrome.LightStep = 24
	alt.Chrome.LightInset = 10
	alt.Chrome.TitleGap = 4
	alt.Chrome.LightBorderWidth = 1
	alt.Typography.WindowTitleWeight = driver.FontWeightBold
	old := tokens
	tokens = &alt
	defer func() { tokens = old }()

	s := startShell(t)
	w, sf, _ := openWindow(t, s, driver.WindowConfig{Title: "Hello", Size: driver.Size{W: 200, H: 100}, Style: allStyles})
	if TitleBarHeight() != 40 || sf.Size() != (driver.Size{W: 200, H: 140}) || w.content.Frame() != (driver.Rect{Y: 40, W: 200, H: 100}) {
		t.Fatalf("title bar %v, surface %v, content %v", TitleBarHeight(), sf.Size(), w.content.Frame())
	}
	// Lights at x = 10, 34, 58, y = (40-16)/2 = 12.
	checkRegions(t, sf.ChromeRegions(),
		shell.ChromeRegion{Rect: driver.Rect{W: 200, H: 40}, Kind: shell.RegionCaption},
		shell.ChromeRegion{Rect: driver.Rect{X: 10, Y: 12, W: 16, H: 16}, Kind: shell.RegionClose},
		shell.ChromeRegion{Rect: driver.Rect{X: 34, Y: 12, W: 16, H: 16}, Kind: shell.RegionMinimize},
		shell.ChromeRegion{Rect: driver.Rect{X: 58, Y: 12, W: 16, H: 16}, Kind: shell.RegionMaximize},
	)
	checkDump(t, paint(t, s, sf),
		"fillRRect (0,40,200,100) solid #ececec",
		"fillRRect (0,0,200,40) solid #e5e5e5",
		"fillRRect (0,39,200,1) solid #0000001a",
		`text (82.5,12,35,16) "Hello" 13pt w700 #000000d9`,
		"fillRRect (10,12,16,16) r=8 solid #ff5f57",
		"strokeRRect (10.5,12.5,15,15) r=7.5 w=1 solid #e0443e",
		"fillRRect (34,12,16,16) r=8 solid #febc2e",
		"strokeRRect (34.5,12.5,15,15) r=7.5 w=1 solid #dea123",
		"fillRRect (58,12,16,16) r=8 solid #28c840",
		"strokeRRect (58.5,12.5,15,15) r=7.5 w=1 solid #1aab29",
	)
	// The title margin is LightInset + 2*LightStep + LightSize + TitleGap =
	// 10 + 48 + 16 + 4 = 78 from both edges: 44 pt are left at width 200.
	sf.UserResize(driver.Size{W: 156 + 35, H: 140})
	if _, text := w.TitleFrame(); text != "Hello" {
		t.Errorf("title %q at margin 78 and width 191", text)
	}
	sf.UserResize(driver.Size{W: 156 + 34, H: 140})
	if _, text := w.TitleFrame(); text == "Hello" {
		t.Errorf("title not truncated at width 190")
	}
}

// TestInitialFocus covers S3.2e's window side: Show focuses the first Tab
// stop unless something is focused, SetContent drops a focus in the old
// content, and the focus survives key changes.
func TestInitialFocus(t *testing.T) {
	s := startShell(t)
	newWin := func() (*Window, *fake.Surface) {
		t.Helper()
		w, err := NewWindow(s, driver.WindowConfig{Title: "W", Size: driver.Size{W: 200, H: 100}, Style: allStyles}, &winLog{})
		if err != nil {
			t.Fatal(err)
		}
		return w, w.Surface().(*fake.Surface)
	}
	var log []string
	// content: box{ a (control, disabled), b (control), t (text) }
	content := func(prefix string) (box, a, b, tx *probe) {
		box = newProbe(&log, prefix+"box", 0, 0, 200, 72)
		a = newProbe(&log, prefix+"a", 0, 0, 10, 10)
		b = newProbe(&log, prefix+"b", 0, 20, 10, 10)
		tx = newProbe(&log, prefix+"t", 0, 40, 10, 10)
		box.SetChildren(a, b, tx)
		for _, e := range []*probe{a, b, tx} {
			e.SetFocusable(true)
		}
		a.SetEnabled(false)
		tx.SetFocusKind(FocusText)
		return
	}
	name := func(r *Router) string {
		if f := r.Focused(); f != nil {
			return f.(*probe).name
		}
		return "<nil>"
	}

	// Show focuses the first enabled control (full keyboard access), before
	// the window is key, so the focused element sees both events in order.
	w, sf := newWin()
	box, _, b, tx := content("")
	w.SetContent(contentWidget{box})
	log = nil
	w.Show()
	if name(w.Router()) != "b" || !w.IsKey() || w.Router().FocusState(b) != FocusedActive {
		t.Fatalf("initial focus %s", name(w.Router()))
	}
	checkLog(t, &log, "b focus true", "box state key=true dark=false", "a state key=true dark=false",
		"b state key=true dark=false", "t state key=true dark=false")

	// The focus survives losing and regaining the key state.
	s.Deactivate()
	if !b.HasFocus() || w.Router().FocusState(b) != FocusedInactive {
		t.Fatal("focus lost with the key state")
	}
	sf.Activate()
	if !b.HasFocus() || w.Router().FocusState(b) != FocusedActive {
		t.Fatal("focus not back with the key state")
	}
	// A second Show keeps the focus.
	tx.Focus()
	w.Show()
	if name(w.Router()) != "t" {
		t.Fatalf("Show again moved the focus to %s", name(w.Router()))
	}
	log = nil

	// Replacing the content drops a focus inside the old one; the next Tab
	// starts at the first element of the new content.
	box2, _, b2, _ := content("n")
	w.SetContent(contentWidget{box2})
	checkLog(t, &log, "t focus false")
	if w.Router().Focused() != nil {
		t.Fatal("focus kept in the replaced content")
	}
	sf.Key(driver.KeyTab, 0)
	if name(w.Router()) != "nb" {
		t.Fatalf("Tab in the new content: %s", name(w.Router()))
	}
	// Setting the same content again keeps its focus.
	w.SetContent(contentWidget{box2})
	if !b2.HasFocus() {
		t.Fatal("same content dropped its focus")
	}
	// Removing the content drops the focus too; Show finds nothing.
	w.SetContent(nil)
	w.Show()
	if w.Router().Focused() != nil {
		t.Fatal("focus without content")
	}
	w.Close()

	// A Focus before Show wins.
	w, _ = newWin()
	box, _, _, tx = content("")
	w.SetContent(contentWidget{box})
	tx.Focus()
	w.Show()
	if name(w.Router()) != "t" {
		t.Fatalf("Focus before Show lost to %s", name(w.Router()))
	}
	w.Close()

	// Without full keyboard access Show focuses the first text element.
	w, _ = newWin()
	box, _, _, _ = content("")
	w.SetContent(contentWidget{box})
	w.Router().SetFullKeyboardAccess(false)
	w.Show()
	if name(w.Router()) != "t" {
		t.Fatalf("limited initial focus %s", name(w.Router()))
	}
	w.Close()

	// Nothing focusable: Show focuses nothing; a closed window's Show does
	// nothing either.
	w, _ = newWin()
	box, a, b, tx := content("")
	for _, e := range []*probe{a, b, tx} {
		e.SetFocusable(false)
	}
	w.SetContent(contentWidget{box})
	w.Show()
	if w.Router().Focused() != nil {
		t.Fatal("focus without focusable elements")
	}
	w.Close()
	b.SetFocusable(true)
	w.Show()
	if w.Router().Focused() != nil {
		t.Fatal("Show after Close focused")
	}
}
