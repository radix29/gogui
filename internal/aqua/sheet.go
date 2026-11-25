package aqua

import (
	"math"
	"slices"
	"time"

	"github.com/radix29/gogui/internal/aqua/a11y"
	"github.com/radix29/gogui/internal/aqua/anim"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

// Sheets (PLAN.md S5.7c): an alert with a parent window is shown as a
// sheet of that window, an alert element (alert.go) in an alertRoot of its
// own, painted by the window's sheet layer.
//
// Geometry: the sheet layer covers the content area, above the content
// and below the title bar (the root's children are the content area, the
// layer and the title bar). The sheet's panel is centred horizontally in
// it (its left edge on a whole device pixel) and hangs from the title
// bar's bottom edge, its own shadow on (alert.SetShadow), clipped to the
// layer. It slides down from behind the title bar over sheetDuration
// (EaseInOut) and back up when it is answered; with Reduce Motion it fades
// in and out in place instead, and Reduce Motion turned on mid-slide puts
// the panel in place at once and fades the rest of the way (S6.1f). The
// slide runs on a Ticker of the window's Timeline (a fade is no motion,
// so Reduce Motion must not jump it); a window without a timeline (made
// with NewWindow directly) shows and removes the sheet at once.
//
// Modality: from the moment an alert is queued on a window until no
// unanswered alert is left (the answer, not the end of the slide-out) the
// window is modal (Window.modal):
//
//   - The content takes no pointer or key input. The layer is hit-testable
//     over the whole content area, so the window's router hovers it (the
//     arrow cursor; the content gets PointerLeave and its capture, hover
//     and scroll latch are cancelled when the modality begins), and it
//     consumes the presses and scrolls there. No dimming (decided in
//     S5.7c): the content draws as it is, its focus kept but drawn
//     inactive (the window's router is inactive, so a caret stops and the
//     IME is off), and active again afterwards. Open popups close when the
//     modality begins.
//   - The pointer events of the surface go to the window's router first
//     (the title bar still moves the window, the traffic lights and the
//     menu strip still work) and then, in the sheet's points, to the
//     router of the sheet shown (the sheet's buttons). Keys go to the menu
//     strip's keyboard access and the accelerators as before, then to the
//     sheet shown, never to the content nor its default and cancel
//     buttons.
//   - The close light is disabled (ShouldClose is not asked; minimize and
//     zoom still work), an OS close request is refused, and the menu's
//     Close role and the Edit roles are disabled (menuaction.go), the
//     accelerators too.
//
// Queue: the alerts of a window are shown one at a time in the order they
// came; the next one slides in once the one before has gone (its
// slide-out ended). An answered alert takes no more input while it slides
// out.
//
// Done: the answer (a click, a key, Alert.Dismiss) runs done at once,
// after the slide-out has started and the modality has ended (when no
// other alert waits), so done may show another alert or close the window.
//
// Closing: Window.Close with alerts shown or queued first answers each of
// them with its Cancel index (0 without one), in queue order, before the
// window goes (decided in S5.7c, AppKit leaves this undefined); an alert
// shown on the closing window from inside such a done is app-modal. A
// window closed otherwise (Quit) answers them the same way once it has
// closed.
//
// Small windows (decided in S5.7c, built in S5.7g): when its turn comes,
// a sheet whose window's content area is narrower or lower than the panel
// (sheet.fits) is shown in a modal surface owned by the window and
// centred on it (alertsurface.go) instead of sliding in; it keeps its
// place in the queue and the window stays modal as for a sheet, and the
// answer closes the surface at once (no slide-out), the next one showing
// after it. A window made smaller while its sheet is shown keeps the sheet,
// clipped to the content area (its top, the icon and the message, visible,
// its keys working), and so does a sheet whose surface the shell refuses.

// sheetDuration is how long a sheet slides (or fades) in and out
// (provisional, from memory of macOS: about 0.2 s).
const sheetDuration = 200 * time.Millisecond

// sheet is one alert of a window's queue.
type sheet struct {
	w      *Window
	alert  *alert
	root   *alertRoot
	ticker *anim.Ticker  // nil without a timeline
	surf   *alertSurface // shown in a modal surface instead (S5.7g), or nil

	started bool    // it is (or was) on screen: the first of the queue
	p       float64 // the slide's progress: 0 hidden, 1 shown
	target  float64 // where p goes
}

// sheetLayer is the window's layer of sheets (see the sheets above). It
// has no children: it paints the shown sheet's tree.
type sheetLayer struct {
	Base
	w *Window
}

// addSheet queues the alert a on w as a sheet: the window becomes modal,
// and the sheet slides in when it is the first of the queue. a's press
// func must call the sheet's answeredBy (Driver.RunAlert sets it).
func (w *Window) addSheet(a *alert) *sheet {
	s := &sheet{w: w, alert: a}
	a.SetShadow(true)
	s.root = newAlertRoot(a)
	s.root.w = w
	s.root.SetFrame(driver.RectOf(driver.Point{}, s.root.FittingSize(driver.Size{W: math.Inf(1), H: math.Inf(1)})))
	s.root.SetInvalidateFunc(func() {
		if !w.closed {
			w.surface.Invalidate()
		}
	})
	if !w.router.FullKeyboardAccess() {
		s.root.SetFullKeyboardAccess(false)
	}
	if w.timeline != nil {
		s.ticker = w.timeline.NewTicker(s.tick)
	}
	wasModal := w.modal()
	w.sheets = append(w.sheets, s)
	if !wasModal {
		w.beginModal()
	}
	s.stateChanged()
	if len(w.sheets) == 1 {
		s.slide(1)
	}
	return s
}

// modal reports whether the window has an unanswered alert (see sheets).
func (w *Window) modal() bool {
	for _, s := range w.sheets {
		if !s.answered() {
			return true
		}
	}
	return false
}

// answered reports whether the sheet's alert has been answered.
func (s *sheet) answered() bool {
	_, ok := s.alert.Result()
	return ok
}

// shown returns the sheet on screen (sliding in, shown or sliding out), or
// nil.
func (w *Window) shownSheet() *sheet {
	if len(w.sheets) > 0 && w.sheets[0].started {
		return w.sheets[0]
	}
	return nil
}

// layerSheet returns the shown sheet when the layer shows it (not in a
// modal surface, S5.7g), or nil.
func (w *Window) layerSheet() *sheet {
	if s := w.shownSheet(); s != nil && s.surf == nil {
		return s
	}
	return nil
}

// inputSheet returns the sheet the layer shows when it takes input
// (unanswered), or nil.
func (w *Window) inputSheet() *sheet {
	if s := w.layerSheet(); s != nil && !s.answered() {
		return s
	}
	return nil
}

// contentActive reports whether the content's router is active: the window
// is key, the menu strip does not have the keys (S5.6i) and no alert is
// up.
func (w *Window) contentActive() bool {
	s := w.menuStrip()
	return w.key && (s == nil || !s.kbd) && !w.modal()
}

// sheetActive reports whether a sheet's router is active: the window is
// key and the menu strip does not have the keys.
func (w *Window) sheetActive() bool {
	s := w.menuStrip()
	return w.key && (s == nil || !s.kbd)
}

// beginModal starts the modality (see sheets).
func (w *Window) beginModal() {
	w.tips.reset()
	w.closePopups()
	w.router.CancelPointer()
	w.modalChanged()
}

// endModal ends the modality once no unanswered alert is left.
func (w *Window) endModal() { w.modalChanged() }

// modalChanged applies the modality's state: the layer shown while a sheet
// is queued, the close light, the chrome regions, the content's router,
// the hover, a repaint with WindowStateEvents and the text input.
func (w *Window) modalChanged() {
	if w.closed {
		return
	}
	w.layer.SetHidden(len(w.sheets) == 0)
	if w.bar != nil {
		w.bar.lights[lightClose].SetEnabled(w.style&driver.StyleClosable != 0 && !w.modal())
		w.surface.SetChromeRegions(w.bar.regions())
	}
	w.router.SetActive(w.contentActive())
	w.router.RefreshHover()
	w.stateChanged()
	w.publishTextInput()
}

// slide starts the sheet's slide to target (1 in, 0 out). A sheet that
// does not fit its window opens in a modal surface instead and closes it
// on the way out (S5.7g).
func (s *sheet) slide(target float64) {
	s.started = true
	s.target = target
	if target == 1 && s.surf == nil && !s.w.closed && !s.fits() && s.w.driver != nil {
		if m, err := s.w.driver.openAlertSurface(s.root, s.w); err == nil {
			s.surf = m
		}
	}
	if s.surf != nil {
		if target == 0 {
			s.surf.close()
			s.arrived()
		}
		return
	}
	if s.ticker == nil || s.w.closed {
		s.setProgress(target)
		s.arrived()
		return
	}
	if s.p != target {
		s.ticker.Start()
	} else {
		s.arrived()
	}
}

// tick advances the slide by dt.
func (s *sheet) tick(dt time.Duration) {
	step := float64(dt) / float64(sheetDuration)
	if s.target > s.p {
		s.setProgress(min(s.target, s.p+step))
	} else {
		s.setProgress(max(s.target, s.p-step))
	}
	if s.p == s.target {
		s.ticker.Stop()
		s.arrived()
	}
}

// setProgress sets the slide's progress and repaints the layer.
func (s *sheet) setProgress(p float64) {
	if p != s.p {
		s.p = p
		s.w.layer.Invalidate()
	}
}

// arrived ends a slide: a sheet that has slid out (or whose surface has
// closed) leaves the queue, and the next one slides in, unless the window
// is closing.
func (s *sheet) arrived() {
	if s.target != 0 || (s.p != 0 && s.surf == nil) {
		return
	}
	s.remove()
	if next := s.w.shownSheet(); next == nil && len(s.w.sheets) > 0 && !s.w.closing {
		s.w.sheets[0].slide(1)
	}
}

// remove takes the sheet out of the queue; the layer hides with the last.
func (s *sheet) remove() {
	w := s.w
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.root.SetInvalidateFunc(nil)
	i := slices.Index(w.sheets, s)
	if i < 0 {
		return
	}
	w.sheets = slices.Delete(w.sheets, i, i+1)
	if !w.closed {
		w.layer.SetHidden(len(w.sheets) == 0)
		w.layer.Invalidate()
	}
}

// answeredBy is the sheet's press func: the shown sheet slides out, a
// queued one leaves the queue, and the modality ends when no unanswered
// alert is left. The caller runs done afterwards.
func (s *sheet) answeredBy() {
	w := s.w
	s.root.router.CancelPointer()
	if s.started {
		s.slide(0)
	} else {
		s.remove()
	}
	if !w.modal() {
		w.endModal()
	}
}

// dismissSheets answers every unanswered alert of the window with its
// Cancel index (0 without one), in queue order (see sheets).
func (w *Window) dismissSheets() {
	w.closing = true
	for _, s := range slices.Clone(w.sheets) {
		if !s.answered() {
			s.alert.answer(max(0, s.alert.cancelIndex()))
		}
	}
}

// dropSheets stops and forgets the sheets of a closed window.
func (w *Window) dropSheets() {
	for _, s := range slices.Clone(w.sheets) {
		s.remove()
	}
}

// stateChanged repaints the sheet's tree and sends its elements a
// WindowStateEvent, after a key or appearance change of the window, as a
// popup's.
func (s *sheet) stateChanged() {
	w := s.w
	if w.closed || s.surf != nil { // a surface follows the driver (S5.7g)
		return
	}
	s.root.router.SetActive(w.sheetActive())
	broadcastState(s.root, s.root.router, w.state, s.root.gone)
}

// origin returns the top-left of the sheet's root in the layer's points
// at the current progress: centred, the panel's top at the layer's top
// when shown, slid up by the panel's height (less nothing with Reduce
// Motion, which fades instead).
func (s *sheet) origin() driver.Point {
	lw := s.w.layer.Frame().W
	sz := s.root.Frame().Size()
	in := s.alert.ShadowMargin()
	x := snapPixel((lw-sz.W)/2, s.w.surface.Scale())
	y := -in.Top
	if !s.w.appearance.ReduceMotion {
		panel := sz.H - in.Top - in.Bottom
		y -= (1 - s.eased()) * (panel + in.Bottom)
	}
	return driver.Point{X: x, Y: y}
}

// eased returns the progress along the slide's curve.
func (s *sheet) eased() float64 { return anim.Curve(tokens.Animation.EaseInOut).At(s.p) }

// fits reports whether the panel fits the window's content area (see the
// small-window rule above).
func (s *sheet) fits() bool {
	c := s.w.layer.Frame().Size()
	return c.W >= tokens.Alert.Width && c.H >= s.alert.ContentHeight()
}

// event passes a pointer or scroll event of the surface to the sheet's
// router, in the sheet's points.
func (s *sheet) event(ev shell.Event) bool {
	o := s.origin().Add(s.w.layer.Frame().Origin())
	switch e := ev.(type) {
	case shell.PointerEvent:
		e.Pos = e.Pos.Sub(o)
		ev = e
	case shell.ScrollEvent:
		e.Pos = e.Pos.Sub(o)
		ev = e
	}
	return s.root.Event(ev)
}

// modalEvent handles an event of a modal window after the menu strip's
// keyboard access and the accelerators: pointer and scroll events go to
// the window's router (the title bar; the layer takes the rest) and then
// to the sheet shown, every other event to the sheet shown only.
func (w *Window) modalEvent(ev shell.Event) bool {
	switch ev.(type) {
	case shell.PointerEvent, shell.ScrollEvent:
		handled := w.router.Event(ev)
		if s := w.inputSheet(); s != nil && !w.closed {
			handled = s.event(ev) || handled
		}
		return handled
	}
	if s := w.inputSheet(); s != nil {
		return s.root.Event(ev)
	}
	return false
}

// HitSelf hits the whole content area while the window is modal.
func (l *sheetLayer) HitSelf(p driver.Point) bool { return l.w.modal() && l.Bounds().Contains(p) }

// HandleEvent consumes the pointer and scroll events over the content
// while the window is modal.
func (l *sheetLayer) HandleEvent(ev Event) bool {
	switch ev.(type) {
	case PointerEvent, ScrollEvent:
		return l.w.modal()
	}
	return false
}

// Paint paints the shown sheet's tree, clipped to the layer, slid or
// faded by its progress.
func (l *sheetLayer) Paint(c render.Canvas) {
	s := l.w.layerSheet()
	if s == nil || s.p <= 0 {
		return
	}
	c.ClipRoundedRect(l.Bounds(), render.Radii{})
	o := s.origin()
	c.Translate(o.X, o.Y)
	fade := l.w.appearance.ReduceMotion && s.p < 1
	if fade {
		c.PushOpacity(s.eased())
	}
	PaintTree(c, s.root)
	if fade {
		c.Pop()
	}
}

// overlayNodes implements accessibleOverlay: the shown, unanswered sheet's
// dialog in surface coordinates.
func (l *sheetLayer) overlayNodes() []*a11y.Node {
	s := l.w.inputSheet()
	if s == nil {
		return nil
	}
	t := AccessibilityTree(s.root)
	d := ToRoot(l, s.origin())
	for _, n := range t.Children {
		offsetNodes(n, d)
	}
	return t.Children
}

// offsetNodes moves the frames of n and its descendants by d.
func offsetNodes(n *a11y.Node, d driver.Point) {
	n.Frame = n.Frame.Offset(d)
	for _, c := range n.Children {
		offsetNodes(c, d)
	}
}

// SheetRoot returns the root element of the sheet the layer shows (not one
// in a modal surface: Alert.Root), in the
// sheet's points, or nil: for tests above the driver.
func (w *Window) SheetRoot() Element {
	if s := w.layerSheet(); s != nil {
		return s.root
	}
	return nil
}

// SheetOrigin returns where the root of the sheet the layer shows is, in
// surface points (the zero point without one).
func (w *Window) SheetOrigin() driver.Point {
	if s := w.layerSheet(); s != nil {
		return s.origin().Add(w.layer.Frame().Origin())
	}
	return driver.Point{}
}

// Modal reports whether the window has an unanswered alert.
func (w *Window) Modal() bool { return w.modal() }
