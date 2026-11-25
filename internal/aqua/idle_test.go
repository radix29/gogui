package aqua

import "testing"

// The idle inventory (PLAN.md S6.2a): every source of frames and timers of
// the aqua driver and the shells, with the state that starts it and what
// ends it. An app left alone runs none of them once they have ended, except
// the ones marked byDesign; the idle audit (gogui's TestIdle*OnAqua, one per
// example, and the gallery's TestIdleGallery) drives every example into
// every state a user can leave a window in and checks that after 11 s
// (the caret blink's timeout, and its last repaint) no frame is requested
// and no timer armed. TestIdleInventory keeps both lists complete.

// idleSource is one entry of the inventory.
type idleSource struct {
	starts, ends string
	byDesign     bool // runs as long as its state lasts (an indeterminate indicator)
}

// idleInventory lists the aqua sources: every animation and timer of
// motionInventory (the same keys), the timeline's frames and the Shell.After
// timers. A frame also comes from every Invalidate (an element that
// changes), which is not a source of its own: it follows an event or one
// of these.
var idleInventory = map[string]idleSource{
	"focusring.go:FocusRing.Track":                {starts: "a focus change", ends: "the ring's grow-in (FocusRingDuration)"},
	"scroller.go:scroller.armFade":                {starts: "a scroll shows the overlay scroller", ends: "its fade-out, after the delay"},
	"scroller.go:scroller.setHover":               {starts: "the pointer enters or leaves an overlay scroller", ends: "the widening or narrowing"},
	"scroller.go:scroller.HandleEvent":            {starts: "a knob drag ends", ends: "the narrowing"},
	"progress.go:progressView.animate":            {starts: "an indeterminate indicator shown in a shown window", ends: "made determinate, hidden, its window closed, Reduce Motion", byDesign: true},
	"scroll/axis.go:Axis.SetMax":                  {starts: "a wheel step retargeted to a new edge", ends: "at the edge"},
	"scroll/axis.go:Axis.ScrollTo":                {starts: "an animated ScrollTo (keys, ScrollRectToVisible)", ends: "at the target"},
	"scroll/axis.go:Axis.wheel":                   {starts: "a wheel step", ends: "at the target"},
	"scroll/axis.go:Axis.release":                 {starts: "a touchpad release with speed", ends: "the momentum's rest or an edge's rubber band"},
	"scroll/axis.go:Axis.springTo":                {starts: "a release past an edge (rubber band)", ends: "settled at the edge"},
	"sheet.go:Window.addSheet":                    {starts: "a sheet shown or dismissed", ends: "the slide (or fade) on its Ticker"},
	"menuinput.go:menu.pick":                      {starts: "a menu item picked", ends: "the blink's two timers, then the menu closes"},
	"menustrip.go:menuStrip.flash":                {starts: "a menu accelerator", ends: "the title flash's timer"},
	"menuinput.go:menu.setArrow":                  {starts: "the pointer on a long menu's scroll arrow", ends: "the pointer leaves it, or the menu's end (the arrow goes)"},
	"textview_input.go:TextView.updateAutoScroll": {starts: "a selection drag outside the text view", ends: "the release, the pointer back inside, or the document's end"},
	"textview_layout.go:TextView.scheduleIdle":    {starts: "a text change with paragraphs left to lay out", ends: "every paragraph laid out"},
	"textedit.go:textEdit.scheduleBlink":          {starts: "a focused editable field or text view in the key window (a restart at every edit or caret move)", ends: "the blink's timeout (CaretBlink.Timeout, 10 s by default), the focus or the key window lost, a selection"},
	"control.go:clickControl.flash":               {starts: "Return or Escape on a default or cancel button, Space on a focused button, checkbox or radio, an alert's keys", ends: "the flash's timer (keyFlash, 100 ms)"},
	"tooltip.go:tooltips.arm":                     {starts: "the pointer resting on an element with a tooltip", ends: "the tooltip shown (it stays without a timer), or the pointer moving off"},
	"tooltip.go:tooltips.hide":                    {starts: "the pointer leaving a shown tooltip's element", ends: "the grace timer"},
}

// shellIdleCalls are the calls of the shells that request a frame or arm
// an OS timer or source: RequestFrame, Win32's SetTimer and
// SetWaitableTimer, GLib's timeout and idle sources and GTK's tick
// callback (through the gtk4 loop), and the fake clock's timers.
var shellIdleCalls = map[string]int{
	"RequestFrame": 0, "SetTimer": 3, "SetWaitableTimer": 2,
	"timeout": 3, "idle": 2, "addTick": 3, "after": 2,
}

// shellIdleInventory lists the sources of the shells (internal/shell,
// keyed "pkg/file:Func").
var shellIdleInventory = map[string]idleSource{
	"fake/clock.go:Clock.invalidated":         {starts: "Invalidate outside the frame function", ends: "the frame that paints it"},
	"fake/popup.go:Shell.NewPopup":            {starts: "a popup opened", ends: "its first paint"},
	"fake/surface.go:Surface.Show":            {starts: "an invalid surface shown or restored", ends: "its paint"},
	"fake/timer.go:Shell.After":               {starts: "Shell.After", ends: "the timer fired or cancelled"},
	"gtk4/clock.go:clock.addTimeout":          {starts: "a requested frame (the fallback timeout without a pacing surface, else the watchdog)", ends: "the frame; no source without a request"},
	"gtk4/clock.go:clock.invalidated":         {starts: "Invalidate outside the frame function", ends: "the frame that paints it"},
	"gtk4/clock.go:clock.schedule":            {starts: "a requested frame (the tick callback on the pacing surface)", ends: "the first tick without a request removes it, so GDK's clock stops"},
	"gtk4/loop_linux.go:gtkLoop.addTick":      {starts: "clock.schedule", ends: "as clock.schedule"},
	"gtk4/popup.go:popup.deferLeave":          {starts: "the pointer leaving a popup", ends: "the idle source, once"},
	"gtk4/scroll.go:surface.decelerate":       {starts: "a touchpad release with speed (GTK's decelerate)", ends: "the momentum's end"},
	"gtk4/scroll.go:surface.stepMomentum":     {starts: "a running momentum", ends: "its end"},
	"gtk4/surface.go:surface.Show":            {starts: "an invalid surface shown", ends: "its paint"},
	"gtk4/surface.go:surface.pointerSignal":   {starts: "an invalid surface restored from minimized", ends: "its paint"},
	"gtk4/timer.go:afterTimers.arm":           {starts: "Shell.After", ends: "the last timer fired or cancelled"},
	"gtk4/wake.go:Shell.wake":                 {starts: "Shell.Post from another goroutine", ends: "the posted work run, once"},
	"win32/clock.go:clock.invalidated":        {starts: "Invalidate outside the frame function", ends: "the frame that paints it"},
	"win32/clockmodal.go:clock.scheduleModal": {starts: "a requested frame inside a modal size/move or menu loop", ends: "the frame, or the loop's end"},
	"win32/clockmodal.go:clock.pollModal":     {starts: "a requested frame inside a modal loop whose waitables have not signaled (S6.2g)", ends: "the waitable's signal or the stall deadline, then the frame"},
	"win32/clock.go:clock.setTimer":           {starts: "a requested frame within its hold", ends: "the hold; no timer without a request"},
	"win32/gpu.go:Shell.deviceLost":           {starts: "a lost GPU device", ends: "the repaint on the new one"},
	"win32/popup.go:Shell.NewPopup":           {starts: "a popup opened", ends: "its first paint"},
	"win32/surface.go:surface.Show":           {starts: "an invalid surface shown", ends: "its paint"},
	"win32/surfaceproc.go:surface.handle":     {starts: "an invalid surface restored from minimized (WM_SIZE)", ends: "its paint"},
	"win32/timer.go:Shell.armTimers":          {starts: "Shell.After", ends: "the last timer fired or cancelled"},
	"win32/wheel.go:swapTarget.wheel":         {starts: "a precise (touchpad) wheel message", ends: "the gesture timeout ends the gesture, once"},
}

// TestIdleInventory: every source of frames and timers in the aqua packages
// and the shells is in the idle inventory, with what starts and ends it,
// and every entry still has a call; only the indeterminate indicator runs
// by design.
func TestIdleInventory(t *testing.T) {
	found, err := scanCalls(".", motionCalls, func(path string) bool { return path == "anim" })
	if err != nil {
		t.Fatal(err)
	}
	checkInventory(t, found, idleInventory, "aqua frame or timer sources not in idleInventory", "idleInventory entries with no call left")
	found, err = scanCalls("../shell", shellIdleCalls, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	checkInventory(t, found, shellIdleInventory, "shell frame or timer sources not in shellIdleInventory", "shellIdleInventory entries with no call left")
	for _, inv := range []map[string]idleSource{idleInventory, shellIdleInventory} {
		for k, s := range inv {
			if s.starts == "" || s.ends == "" {
				t.Errorf("%s: no start or end", k)
			}
			if s.byDesign != (k == "progress.go:progressView.animate") {
				t.Errorf("%s: byDesign %v", k, s.byDesign)
			}
		}
	}
	for k := range motionInventory {
		if _, ok := idleInventory[k]; !ok {
			t.Errorf("%s is in motionInventory, not in idleInventory", k)
		}
	}
}
