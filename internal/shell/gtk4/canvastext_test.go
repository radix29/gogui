package gtk4

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
)

// The canvas's text host tests (S4.2i): DrawText over a fakeSnapshot,
// whose "text #n" is the nth native layout the fake pangoLayouter
// (txLayouter, text_test.go) made, with a text system over that layouter
// as the canvas's. Covered: the ops and their nesting, the ignored cases
// (none makes a native layout), the per-paint cache and its releases, the
// arguments the native layout is made with, and a failing layout.

var ctFont = render.Font{Size: 13}

// ctText returns a text system over a fresh txLayouter, the layouter and
// the system's log.
func ctText() (*textSystem, *txLayouter, *txLogs) {
	pl := &txLayouter{ui: "Cantarell", has: map[string]bool{"DejaVu Serif": true}, cw: 7, lh: 16}
	ts, lg := txSystem(pl, nil, nil)
	return ts, pl, lg
}

// ctLayout returns ts's layout of text, which must be a native-backed one
// (not a fake fallback).
func ctLayout(t *testing.T, ts *textSystem, text string) *textLayout {
	t.Helper()
	l, ok := ts.NewLayout(text, ctFont, 0).(*textLayout)
	if !ok {
		t.Fatalf("NewLayout(%q) is not a *textLayout", text)
	}
	return l
}

// ctPaint paints once into a 100×50 surface at scale 1 with ts as the
// canvas's text system (nil for none), finishes the canvas and returns the
// snapshot, which must be balanced. Without a recorder finish must return
// nil (with one it reports contract violations, which some cases make on
// purpose).
func ctPaint(t *testing.T, ts *textSystem, debug bool, paint func(c *canvas)) *fakeSnapshot {
	t.Helper()
	sn := &fakeSnapshot{}
	c := newCanvas(sn, render.Size{W: 100, H: 50}, 1, debug)
	c.text = ts
	paint(c)
	if err := c.finish(); !debug && err != nil {
		t.Errorf("finish without a recorder = %v, want nil", err)
	}
	if c.texts != nil {
		t.Errorf("finish left %d cached native layouts", len(c.texts))
	}
	if err := sn.balanced(); err != nil {
		t.Errorf("debug=%v: snapshot not balanced: %v\nops:\n%s", debug, err, strings.Join(sn.ops, "\n"))
	}
	return sn
}

// ctReleased checks that every native layout pl made was released n times.
func ctReleased(t *testing.T, what string, pl *txLayouter, n int) {
	t.Helper()
	for _, nl := range pl.layouts {
		if nl.released != n {
			t.Errorf("%s: native layout #%d released %d times, want %d", what, nl.id, nl.released, n)
		}
	}
}

// TestCanvasTextOps: a layout draws as a save, a translate to the origin,
// the native layout in the color and a restore, nested in whatever state
// is open; its native layout is released by finish.
func TestCanvasTextOps(t *testing.T) {
	text := func(x, y, col string) []string {
		return []string{"save", "translate " + x + "," + y, "text #1 " + col, "restore"}
	}
	cat := func(parts ...[]string) []string {
		var ops []string
		for _, p := range parts {
			ops = append(ops, p...)
		}
		return ops
	}
	for _, tc := range []struct {
		name  string
		paint func(c *canvas, l render.TextLayout)
		want  []string
	}{
		{"drawn", func(c *canvas, l render.TextLayout) { c.DrawText(l, render.Point{X: 5, Y: 6}, cvBlue) },
			cvOps(text("5", "6", cvBlueOp)...)},
		{"at the origin", func(c *canvas, l render.TextLayout) { c.DrawText(l, render.Point{}, cvRed) },
			cvOps(text("0", "0", cvRedOp)...)},
		{"negative and fractional origin", func(c *canvas, l render.TextLayout) {
			c.DrawText(l, render.Point{X: -1.5, Y: 0.25}, cvRed)
		}, cvOps(text("-1.5", "0.25", cvRedOp)...)},
		{"outside the surface", func(c *canvas, l render.TextLayout) { c.DrawText(l, render.Point{X: 500, Y: -90}, cvRed) },
			cvOps(text("500", "-90", cvRedOp)...)},
		{"translucent color", func(c *canvas, l render.TextLayout) { c.DrawText(l, render.Point{X: 1, Y: 2}, cvShade) },
			cvOps(text("1", "2", cvShadeOp)...)},
		{"under transforms", func(c *canvas, l render.TextLayout) {
			c.Translate(10, 20)
			c.Scale(2, 2)
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvRed)
		}, cvOps(cat([]string{"translate 10,20", "scale 2,2"}, text("1", "2", cvRedOp))...)},
		{"in a save with a clip", func(c *canvas, l render.TextLayout) {
			c.Save()
			c.ClipRoundedRect(cvRect, render.UniformRadii(5))
			c.DrawText(l, render.Point{X: 12, Y: 22}, cvRed)
			c.Restore()
		}, cvOps(cat([]string{"save", "clip (10,20,30,40) r=5"}, text("12", "22", cvRedOp), []string{"pop", "restore"})...)},
		{"in an opacity", func(c *canvas, l render.TextLayout) {
			c.PushOpacity(0.5)
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvRed)
			c.Pop()
		}, cvOps(cat([]string{"opacity 0.5", "save"}, text("1", "2", cvRedOp), []string{"restore", "pop"})...)},
		{"state left open", func(c *canvas, l render.TextLayout) {
			c.Save()
			c.Translate(3, 4)
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvRed)
		}, cvOps(cat([]string{"save", "translate 3,4"}, text("1", "2", cvRedOp), []string{"restore"})...)},
		{"between other drawing", func(c *canvas, l render.TextLayout) {
			c.FillRoundedRect(render.Rect{W: 1, H: 1}, render.Radii{}, render.Solid(cvRed))
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvBlue)
			c.FillRoundedRect(render.Rect{W: 2, H: 2}, render.Radii{}, render.Solid(cvRed))
		}, cvOps(cat([]string{"color (0,0,1,1) " + cvRedOp}, text("1", "2", cvBlueOp), []string{"color (0,0,2,2) " + cvRedOp})...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, debug := range []bool{false, true} {
				ts, pl, _ := ctText()
				l := ctLayout(t, ts, "abc")
				sn := ctPaint(t, ts, debug, func(c *canvas) { tc.paint(c, l) })
				cvWantOps(t, sn.ops, tc.want)
				if len(pl.layouts) != 1 {
					t.Errorf("debug=%v: %d native layouts, want 1", debug, len(pl.layouts))
				}
				ctReleased(t, "after the paint", pl, 1)
			}
		})
	}
}

// TestCanvasTextIgnored: the cases DrawText ignores draw nothing and make
// no native layout: nil and typed-nil layouts, fake ones, another text
// system's, empty text, semantic and transparent colors, origins that are
// not finite as float32, a canvas without a text system, a released text
// system, and calls after finish.
func TestCanvasTextIgnored(t *testing.T) {
	other, otherPL, _ := ctText()
	otherL := ctLayout(t, other, "abc")
	for _, tc := range []struct {
		name    string
		noText  bool // the canvas has no text system
		noDebug bool // the recorder can't take the case (see below)
		paint   func(c *canvas, ts *textSystem, l *textLayout)
	}{
		{"nil layout", false, false, func(c *canvas, _ *textSystem, _ *textLayout) {
			c.DrawText(nil, render.Point{X: 1, Y: 2}, cvRed)
		}},
		// The recorder asks a layout its Size, which a typed nil can't
		// answer: that contract violation is only checked without one.
		{"typed-nil layout", false, true, func(c *canvas, _ *textSystem, _ *textLayout) {
			c.DrawText((*textLayout)(nil), render.Point{X: 1, Y: 2}, cvRed)
		}},
		{"fake layout", false, false, func(c *canvas, _ *textSystem, _ *textLayout) {
			c.DrawText(fake.NewTextSystem().NewLayout("abc", ctFont, 0), render.Point{X: 1, Y: 2}, cvRed)
		}},
		{"another text system's layout", false, false, func(c *canvas, _ *textSystem, _ *textLayout) {
			c.DrawText(otherL, render.Point{X: 1, Y: 2}, cvRed)
		}},
		{"empty text", false, false, func(c *canvas, ts *textSystem, _ *textLayout) {
			c.DrawText(ctLayout(t, ts, ""), render.Point{X: 1, Y: 2}, cvRed)
		}},
		{"semantic color", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvLabel)
		}},
		{"transparent color", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvClear)
		}},
		{"NaN x", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: cvNaN, Y: 2}, cvRed)
		}},
		{"NaN y", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: 1, Y: cvNaN}, cvRed)
		}},
		{"Inf x", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: cvInf, Y: 2}, cvRed)
		}},
		{"-Inf y", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: 1, Y: -cvInf}, cvRed)
		}},
		{"x overflows float32", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: cvBigF32, Y: 2}, cvRed)
		}},
		{"y overflows float32", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: 1, Y: -cvBigF32}, cvRed)
		}},
		{"no text system", true, false, func(c *canvas, _ *textSystem, l *textLayout) {
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvRed)
		}},
		{"text system released", false, false, func(c *canvas, ts *textSystem, l *textLayout) {
			ts.release()
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvRed)
			c.DrawText(l, render.Point{X: 3, Y: 4}, cvRed)
		}},
		{"after finish", false, false, func(c *canvas, _ *textSystem, l *textLayout) {
			if err := c.finish(); err != nil && c.rec == nil {
				t.Errorf("finish = %v", err)
			}
			c.DrawText(l, render.Point{X: 1, Y: 2}, cvRed)
		}},
	} {
		for _, debug := range []bool{false, true} {
			if debug && tc.noDebug {
				continue
			}
			ts, pl, lg := ctText()
			l := ctLayout(t, ts, "abc")
			cts := ts
			if tc.noText {
				cts = nil
			}
			sn := ctPaint(t, cts, debug, func(c *canvas) { tc.paint(c, ts, l) })
			if want := cvOps(); strings.Join(sn.ops, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s debug=%v: drew:\n%s", tc.name, debug, strings.Join(sn.ops, "\n"))
			}
			if pl.layoutCalls != 0 || len(otherPL.layouts) != 0 {
				t.Errorf("%s debug=%v: %d native layouts made (another system's: %d), want none",
					tc.name, debug, pl.layoutCalls, len(otherPL.layouts))
			}
			if pl.afterRelease != 0 {
				t.Errorf("%s debug=%v: %d layouter calls after its release", tc.name, debug, pl.afterRelease)
			}
			if len(lg.msgs) != 0 {
				t.Errorf("%s debug=%v: logged %q, want nothing", tc.name, debug, lg.msgs)
			}
		}
	}
}

// TestCanvasTextCache: a paint makes one native layout per layout, at its
// first draw, whatever the colors and origins it is drawn with; none is
// released before finish, and finish releases each exactly once; the
// next paint makes new ones.
func TestCanvasTextCache(t *testing.T) {
	ts, pl, _ := ctText()
	a, b := ctLayout(t, ts, "abc"), ctLayout(t, ts, "de\nf")
	sn := ctPaint(t, ts, false, func(c *canvas) {
		c.DrawText(a, render.Point{X: 1, Y: 2}, cvRed)
		c.DrawText(a, render.Point{X: 3, Y: 4}, cvBlue)
		c.DrawText(b, render.Point{X: 5, Y: 6}, cvRed)
		c.Translate(10, 0)
		c.DrawText(a, render.Point{X: 1, Y: 2}, cvShade)
		c.DrawText(b, render.Point{X: 7, Y: 8}, cvBlue)
		if len(pl.layouts) != 2 || len(c.texts) != 2 {
			t.Errorf("in the paint: %d native layouts, %d cached, want 2", len(pl.layouts), len(c.texts))
		}
		ctReleased(t, "in the paint", pl, 0)
	})
	cvWantOps(t, sn.ops, cvOps(
		"save", "translate 1,2", "text #1 "+cvRedOp, "restore",
		"save", "translate 3,4", "text #1 "+cvBlueOp, "restore",
		"save", "translate 5,6", "text #2 "+cvRedOp, "restore",
		"translate 10,0",
		"save", "translate 1,2", "text #1 "+cvShadeOp, "restore",
		"save", "translate 7,8", "text #2 "+cvBlueOp, "restore",
	))
	if len(pl.layouts) != 2 || pl.layouts[0].args.text != "abc" || pl.layouts[1].args.text != "de\nf" {
		t.Fatalf("native layouts %v, want one of each layout, in the order first drawn", pl.layouts)
	}
	ctReleased(t, "after the paint", pl, 1)

	// The next paint: new native layouts, in its own order; the old ones
	// stay released once.
	sn = ctPaint(t, ts, true, func(c *canvas) {
		c.DrawText(b, render.Point{X: 1, Y: 1}, cvRed)
		c.DrawText(a, render.Point{X: 2, Y: 2}, cvRed)
		c.DrawText(b, render.Point{X: 3, Y: 3}, cvRed)
	})
	cvWantOps(t, sn.ops, cvOps(
		"save", "translate 1,1", "text #3 "+cvRedOp, "restore",
		"save", "translate 2,2", "text #4 "+cvRedOp, "restore",
		"save", "translate 3,3", "text #3 "+cvRedOp, "restore",
	))
	if len(pl.layouts) != 4 || pl.layouts[2].args.text != "de\nf" || pl.layouts[3].args.text != "abc" {
		t.Errorf("native layouts %v after the second paint, want two more", pl.layouts)
	}
	ctReleased(t, "after the second paint", pl, 1)

	// A paint that draws no text makes none.
	ctPaint(t, ts, false, func(c *canvas) {
		c.FillRoundedRect(cvRect, render.Radii{}, render.Solid(cvRed))
	})
	if len(pl.layouts) != 4 {
		t.Errorf("a paint without text made %d native layouts", len(pl.layouts)-4)
	}
}

// TestCanvasTextLayoutArgs: the native layout is made with exactly what
// Pango measured: the text with the stand-ins for NUL and invalid bytes,
// the resolved font and the wrap width.
func TestCanvasTextLayoutArgs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		font     render.Font
		maxWidth float64
		want     txMeasure
	}{
		{"plain", "Hello", render.Font{Size: 13}, 0,
			txMeasure{"Hello", pangoFont{family: "Cantarell", size: 13 * pangoScale, weight: 400}, -1}},
		{"stand-ins", "a\x00b\xffc", render.Font{Size: 13}, 0,
			txMeasure{"a?b?c", pangoFont{family: "Cantarell", size: 13 * pangoScale, weight: 400}, -1}},
		{"known family, bold italic, wrapped", "wrap me", render.Font{Family: "DejaVu Serif", Size: 20, Weight: 700, Italic: true}, 100,
			txMeasure{"wrap me", pangoFont{family: "DejaVu Serif", size: 20 * pangoScale, weight: 700, italic: true}, wrapWidth(100)}},
		{"unknown family, monospaced", "x", render.Font{Monospaced: true, Size: 11}, math.Inf(1),
			txMeasure{"x", pangoFont{family: familyMono, size: 11 * pangoScale, weight: 400}, -1}},
		{"unknown family", "x", render.Font{Family: "Nope", Size: 12}, 50.5,
			txMeasure{"x", pangoFont{family: "Cantarell", size: 12 * pangoScale, weight: 400}, wrapWidth(50.5)}},
	} {
		ts, pl, _ := ctText()
		l, ok := ts.NewLayout(tc.text, tc.font, tc.maxWidth).(*textLayout)
		if !ok || len(pl.measures) == 0 {
			t.Fatalf("%s: NewLayout made no Pango layout", tc.name)
		}
		measured := pl.measures[len(pl.measures)-1]
		ctPaint(t, ts, false, func(c *canvas) { c.DrawText(l, render.Point{}, cvRed) })
		if len(pl.layouts) != 1 {
			t.Fatalf("%s: %d native layouts, want 1", tc.name, len(pl.layouts))
		}
		got := pl.layouts[0].args
		if got != measured {
			t.Errorf("%s: native layout made of %+v, measured %+v", tc.name, got, measured)
		}
		if got != tc.want {
			t.Errorf("%s: native layout made of %+v, want %+v", tc.name, got, tc.want)
		}
	}
	if wrapWidth(100) <= 0 || wrapWidth(50.5) <= 0 {
		t.Fatal("the wrapped cases above don't wrap")
	}
}

// TestCanvasTextLayoutFails: when Pango can't make the native layout the
// text is not drawn; each layout is tried once per paint (the failure is
// cached), the first failure is logged once for the text system's life,
// and a later success draws again without logging.
func TestCanvasTextLayoutFails(t *testing.T) {
	ts, pl, lg := ctText()
	a, b := ctLayout(t, ts, "abc"), ctLayout(t, ts, "def")
	pl.layoutErr = errors.New("no layout")
	draw := func(c *canvas) {
		for i := range 3 {
			c.DrawText(a, render.Point{X: float64(i)}, cvRed)
			c.DrawText(b, render.Point{Y: float64(i)}, cvBlue)
		}
	}
	for paint := 1; paint <= 3; paint++ {
		sn := ctPaint(t, ts, paint == 2, draw)
		cvWantOps(t, sn.ops, cvOps())
		if pl.layoutCalls != 2*paint {
			t.Errorf("paint %d: %d layout calls in all, want %d (one per layout and paint)", paint, pl.layoutCalls, 2*paint)
		}
	}
	if len(lg.msgs) != 1 || !strings.Contains(lg.msgs[0], "no layout") {
		t.Errorf("logged %q, want the first failure once", lg.msgs)
	}
	if !ts.drawFailed || ts.failed || ts.closed {
		t.Errorf("drawFailed %v, failed %v, closed %v: want only drawFailed", ts.drawFailed, ts.failed, ts.closed)
	}

	// Pango recovers: the next paint draws, and nothing more is logged.
	pl.layoutErr = nil
	sn := ctPaint(t, ts, false, func(c *canvas) { c.DrawText(a, render.Point{X: 1, Y: 2}, cvRed) })
	cvWantOps(t, sn.ops, cvOps("save", "translate 1,2", "text #1 "+cvRedOp, "restore"))
	if len(lg.msgs) != 1 {
		t.Errorf("logged %q after a success", lg.msgs)
	}
	ctReleased(t, "after the paint", pl, 1)

	// A layout failing within a paint whose other layout succeeds: only
	// the failure is skipped.
	fails := &txLayouter{ui: "Cantarell", cw: 7, lh: 16}
	ts2, _ := txSystem(fails, nil, nil)
	ok := ctLayout(t, ts2, "ok")
	bad := ctLayout(t, ts2, "bad")
	sn = ctPaint(t, ts2, false, func(c *canvas) {
		c.DrawText(ok, render.Point{X: 1, Y: 1}, cvRed)
		fails.layoutErr = errors.New("no layout")
		c.DrawText(bad, render.Point{X: 2, Y: 2}, cvRed)
		fails.layoutErr = nil
		c.DrawText(bad, render.Point{X: 3, Y: 3}, cvRed) // cached as failed
		c.DrawText(ok, render.Point{X: 4, Y: 4}, cvRed)
	})
	cvWantOps(t, sn.ops, cvOps(
		"save", "translate 1,1", "text #1 "+cvRedOp, "restore",
		"save", "translate 4,4", "text #1 "+cvRedOp, "restore",
	))
	if fails.layoutCalls != 2 {
		t.Errorf("%d layout calls, want 2", fails.layoutCalls)
	}
	ctReleased(t, "mixed paint", fails, 1)
}

// TestCanvasTextBlur: text drawn before, inside and after a backdrop
// blur's cut keeps one native layout per layout for the whole paint (the
// cut's node holds the drawing, it is not drawn again), and the snapshot
// stays balanced.
func TestCanvasTextBlur(t *testing.T) {
	ts, pl, _ := ctText()
	a, b := ctLayout(t, ts, "abc"), ctLayout(t, ts, "def")
	shape := render.Rect{X: 30, Y: 10, W: 20, H: 20}
	sn := ctPaint(t, ts, true, func(c *canvas) {
		cvBlurFill(c)
		c.DrawText(a, render.Point{X: 1, Y: 2}, cvRed)
		c.PushBackdropBlur(shape, render.Radii{}, 4)
		c.DrawText(a, render.Point{X: 31, Y: 12}, cvBlue)
		c.Pop()
		c.DrawText(b, render.Point{X: 5, Y: 6}, cvRed)
	})
	cvWantOps(t, sn.ops, []string{
		cvBaseOp, cvBlurFillOp,
		"save", "translate 1,2", "text #1 " + cvRedOp, "restore",
		"pop", "cut #1",
		cvBaseOp,
		"save", "clip (30,10,20,20)",
		"save", "blur 4", "clip (24,4,32,32)", "node #1", "pop", "pop", "restore",
		"save", "translate 31,12", "text #1 " + cvBlueOp, "restore",
		"pop", "restore",
		"save", "translate 5,6", "text #2 " + cvRedOp, "restore",
		"pop",
	})
	if len(pl.layouts) != 2 {
		t.Errorf("%d native layouts, want 2", len(pl.layouts))
	}
	ctReleased(t, "after the paint", pl, 1)
}
