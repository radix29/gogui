package gogui

import (
	"log"
	"os"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// The debug frame-time log (PLAN.md S6.2c). With GOGUI_DEBUG=1, an app on
// a drawn driver (aqua, over the Win32 and GTK 4 shells) logs every frame
// whose CPU work on the UI thread is over frameBudget, with its split: the
// frame function (the animations), the layout passes run since the
// previous frame (they run on the UI queue before the frame they cause),
// the painting on the canvas and the submit (setting up and finishing the
// canvas, D2D's EndDraw). The present (Present and the composition's
// commit on Win32, which may wait for the display) is shown but not
// counted: it is the pacing, not the frame's work.

// debugFrames turns the log on (GOGUI_DEBUG=1). Unlike debugChecks the
// tests do not set it.
var debugFrames = os.Getenv("GOGUI_DEBUG") == "1"

// frameBudget is the budget of a frame's work: a 120 Hz frame.
var frameBudget = time.Second / 120

// frameLogf prints the log's lines (tests replace it).
var frameLogf = log.Printf

// frameLog is the log of one running app. UI thread only.
type frameLog struct {
	// now is the frame clock's Now: time.Now moves only every 0.5 ms on
	// Windows (the interrupt timer), the clock is QPC there.
	now    func() time.Duration
	layout time.Duration // the layout passes since the last frame
}

// startFrameLog starts the log for app if debugFrames is on and its
// driver has a frame clock (driver.Driver.ObserveFrames). It runs in
// Ready.
func startFrameLog(app *App) {
	if !debugFrames {
		return
	}
	l := &frameLog{}
	if l.now = app.drv.ObserveFrames(l.observe); l.now == nil {
		return
	}
	app.frames = l
}

// addLayout counts a layout pass that started at start and ends now
// (deferred by Window.layout).
func (l *frameLog) addLayout(start time.Duration) { l.layout += l.now() - start }

// observe is the clock's frame observer: it logs ft if it is over budget.
func (l *frameLog) observe(ft driver.FrameTiming) {
	lay := l.layout
	l.layout = 0
	work := ft.FrameFunc + lay + ft.Paint + ft.Submit
	if work <= frameBudget {
		return
	}
	what := "frame at " + ft.Time.String()
	if ft.Time < 0 {
		what = "paint outside a frame"
	}
	frameLogf("gogui: %s over budget (%v): %v: frame function %v, layout %v, paint %v, submit %v (present %v, %d surfaces)",
		what, frameBudget, work, ft.FrameFunc, lay, ft.Paint, ft.Submit, ft.Present, ft.Surfaces)
}
