package gogui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
)

// withFrameLog turns the debug frame-time log on with budget for the
// test and returns the lines it logs.
func withFrameLog(t *testing.T, budget time.Duration) *[]string {
	t.Helper()
	oldOn, oldBudget, oldLogf := debugFrames, frameBudget, frameLogf
	var lines []string
	debugFrames, frameBudget = true, budget
	frameLogf = func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { debugFrames, frameBudget, frameLogf = oldOn, oldBudget, oldLogf })
	return &lines
}

// TestFrameLog: with GOGUI_DEBUG=1 a frame over budget is logged with its
// split, the layout passes since the previous frame included and counted
// once; a frame within budget is not (S6.2c).
func TestFrameLog(t *testing.T) {
	lines := withFrameLog(t, -1) // every frame is over a negative budget
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		win := exampleHello(t, app)
		if app.frames == nil {
			t.Fatal("no frame log on aqua with debugFrames")
		}
		*lines = nil
		sf := win.p.(*aqua.Window).Surface().(*fakeshell.Surface)
		win.layout()
		if app.frames.layout < 0 {
			t.Fatalf("layout time %v", app.frames.layout)
		}
		sf.Invalidate()
		sh.FakeClock().Advance(fakeshell.FrameInterval)
		if len(*lines) != 1 {
			t.Fatalf("one frame: %d lines %q", len(*lines), *lines)
		}
		l := (*lines)[0]
		for _, part := range []string{"gogui: frame at ", "over budget (-1ns)", "frame function ", "layout ", "paint ", "submit ", "present 0s", "1 surfaces"} {
			if !strings.Contains(l, part) {
				t.Errorf("line %q lacks %q", l, part)
			}
		}
		if app.frames.layout != 0 {
			t.Errorf("layout time %v left after the frame", app.frames.layout)
		}
		frameBudget = time.Hour
		*lines = nil
		sf.Invalidate()
		sh.FakeClock().Advance(fakeshell.FrameInterval)
		if len(*lines) != 0 {
			t.Errorf("a frame within budget logged %q", *lines)
		}
	})
}

// TestFrameLogOff: without GOGUI_DEBUG=1 no observer is set.
func TestFrameLogOff(t *testing.T) {
	withFrameLog(t, -1)
	debugFrames = false
	runAqua(t, func(app *App, sh *fakeshell.Shell) {
		if app.frames != nil {
			t.Error("a frame log without debugFrames")
		}
	})
}
