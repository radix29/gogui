package gtk4

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// The canvas's host tests (S4.1g, S4.2b–S4.2d; images, S4.2e, the blur,
// S4.2f, and text, S4.2i, are in image_test.go, blur_test.go and
// canvastext_test.go): the geometry as pure functions, and the canvas over
// a fakeSnapshot, whose ops are compared line by line and whose nesting
// must be balanced after finish.

var (
	cvRed     = driver.RGBA(1, 0, 0, 1)
	cvGreen   = driver.RGBA(0, 1, 0, 1)
	cvBlue    = driver.RGBA(0, 0, 1, 1)
	cvClear   = driver.RGBA(0, 0, 0, 0)
	cvShade   = driver.RGBA(0, 0, 0, 0.25)
	cvLabel   = driver.Semantic(driver.ColorLabel)
	cvNaN     = math.NaN()
	cvInf     = math.Inf(1)
	cvRect    = render.Rect{X: 10, Y: 20, W: 30, H: 40}
	cvBigF32  = 1e39 // finite as a float64, +Inf as a float32
	cvBaseOp  = "clip (0,0,100,50)"
	cvRedOp   = "rgba(1,0,0,1)"
	cvBlueOp  = "rgba(0,0,1,1)"
	cvShadeOp = "rgba(0,0,0,0.25)"
)

// cvImage is a render.Image.
type cvImage struct{}

func (cvImage) Size() render.Size { return render.Size{W: 8, H: 8} }

// cvLine is the path from (x0, y0) to (x1, y1).
func cvLine(x0, y0, x1, y1 float64) *render.Path {
	return new(render.Path).MoveTo(render.Point{X: x0, Y: y0}).LineTo(render.Point{X: x1, Y: y1})
}

// cvStop is a gradient stop.
func cvStop(off float64, c render.Color) render.GradientStop {
	return render.GradientStop{Offset: off, Color: c}
}

// cvNear compares colors with a float32 tolerance.
func cvNear(a, b render.Color) bool {
	n := func(x, y float32) bool { return math.Abs(float64(x-y)) < 1e-6 }
	return a.Semantic == b.Semantic && n(a.R, b.R) && n(a.G, b.G) && n(a.B, b.B) && n(a.A, b.A)
}

// cvOps returns the base clip, body and the base clip's pop: the ops of a
// paint of the 100×50 test surface whose body leaves the base frame as it
// found it.
func cvOps(body ...string) []string {
	return append(append([]string{cvBaseOp}, body...), "pop")
}

// cvDraw paints into a 100×50 surface at scale, finishes the canvas and
// returns the ops. It checks that the snapshot is balanced, that finish
// without a recorder returns nil, and that the ops are the same with the
// recorder on (the recorder only checks).
func cvDraw(t *testing.T, scale float64, paint func(c *canvas)) []string {
	t.Helper()
	var ops [2][]string
	for i, debug := range []bool{false, true} {
		sn := &fakeSnapshot{}
		c := newCanvas(sn, render.Size{W: 100, H: 50}, scale, debug)
		paint(c)
		err := c.finish()
		if !debug && err != nil {
			t.Errorf("finish without a recorder = %v, want nil", err)
		}
		if err := sn.balanced(); err != nil {
			t.Errorf("debug=%v: snapshot not balanced: %v\nops:\n%s", debug, err, strings.Join(sn.ops, "\n"))
		}
		ops[i] = sn.ops
	}
	if !slices.Equal(ops[0], ops[1]) {
		t.Errorf("ops differ with the recorder:\nwithout:\n%s\nwith:\n%s",
			strings.Join(ops[0], "\n"), strings.Join(ops[1], "\n"))
	}
	return ops[0]
}

// cvCase is one paint and the ops it must make.
type cvCase struct {
	name  string
	paint func(c *canvas)
	want  []string
}

// cvRun runs the cases at scale 1.
func cvRun(t *testing.T, cases []cvCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cvWantOps(t, cvDraw(t, 1, tc.paint), tc.want)
		})
	}
}

func cvWantOps(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("ops:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// --- Pure functions ----------------------------------------------------------

func TestCanvasBoxOK(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    render.Rect
		ok   bool
	}{
		{"area", cvRect, true},
		{"negative origin", render.Rect{X: -5, Y: -6, W: 1, H: 2}, true},
		{"zero width", render.Rect{W: 0, H: 5}, false},
		{"zero height", render.Rect{W: 5, H: 0}, false},
		{"negative width", render.Rect{W: -1, H: 5}, false},
		{"negative height", render.Rect{W: 5, H: -1}, false},
		{"NaN width", render.Rect{W: cvNaN, H: 5}, false},
		{"NaN height", render.Rect{W: 5, H: cvNaN}, false},
		{"NaN x", render.Rect{X: cvNaN, W: 5, H: 5}, false},
		{"Inf y", render.Rect{Y: -cvInf, W: 5, H: 5}, false},
		{"Inf width", render.Rect{W: cvInf, H: 5}, false},
		{"float32 overflow", render.Rect{X: cvBigF32, W: 5, H: 5}, false},
	} {
		got, ok := boxOK(tc.r)
		if ok != tc.ok {
			t.Errorf("%s: boxOK(%v) ok = %v, want %v", tc.name, tc.r, ok, tc.ok)
			continue
		}
		want := render.Rect{}
		if ok {
			want = tc.r
		}
		if got != want {
			t.Errorf("%s: boxOK(%v) = %v, want %v", tc.name, tc.r, got, want)
		}
	}
}

// cvPath builds a path: each op is "M", "L", "Q", "C" or "Z" followed by
// the coordinates it takes.
func cvPath(ops ...any) *render.Path {
	p := new(render.Path)
	pt := func(i int) render.Point { return render.Point{X: ops[i].(float64), Y: ops[i+1].(float64)} }
	for i := 0; i < len(ops); {
		switch ops[i] {
		case "M":
			p.MoveTo(pt(i + 1))
			i += 3
		case "L":
			p.LineTo(pt(i + 1))
			i += 3
		case "Q":
			p.QuadTo(pt(i+1), pt(i+3))
			i += 5
		case "C":
			p.CubicTo(pt(i+1), pt(i+3), pt(i+5))
			i += 7
		case "Z":
			p.Close()
			i++
		default:
			panic(fmt.Sprintf("cvPath: %v at %d", ops[i], i))
		}
	}
	return p
}

func TestCanvasPathCmds(t *testing.T) {
	for _, tc := range []struct {
		name         string
		path         *render.Path
		fill, stroke string // "" when ok is false
	}{
		{"nil", nil, "", ""},
		{"empty", new(render.Path), "", ""},
		{"lone move", cvPath("M", 1.0, 2.0), "", ""},
		{"lone move closed", cvPath("M", 1.0, 2.0, "Z"), "", ""},
		{"two lone moves", cvPath("M", 1.0, 2.0, "M", 3.0, 4.0), "", ""},
		{"line", cvLine(1, 2, 11, 22), "M1,2 L11,22 Z", "M1,2 L11,22"},
		{"closed triangle", cvPath("M", 0.0, 0.0, "L", 10.0, 0.0, "L", 5.0, 8.0, "Z"),
			"M0,0 L10,0 L5,8 Z", "M0,0 L10,0 L5,8 Z"},
		{"quad", cvPath("M", 0.0, 0.0, "Q", 5.0, 10.0, 10.0, 0.0), "M0,0 Q5,10 10,0 Z", "M0,0 Q5,10 10,0"},
		{"cubic", cvPath("M", 0.0, 0.0, "C", 0.0, 10.0, 10.0, 10.0, 10.0, 0.0),
			"M0,0 C0,10 10,10 10,0 Z", "M0,0 C0,10 10,10 10,0"},
		{"quad without a move", cvPath("Q", 5.0, 10.0, 10.0, 0.0), "M5,10 Q5,10 10,0 Z", "M5,10 Q5,10 10,0"},
		{"line without a move is a move", cvPath("L", 1.0, 2.0, "L", 3.0, 4.0), "M1,2 L3,4 Z", "M1,2 L3,4"},
		{"two open subpaths", cvPath("M", 0.0, 0.0, "L", 1.0, 1.0, "M", 5.0, 5.0, "L", 6.0, 7.0),
			"M0,0 L1,1 Z M5,5 L6,7 Z", "M0,0 L1,1 M5,5 L6,7"},
		{"open then closed", cvPath("M", 0.0, 0.0, "L", 1.0, 1.0, "M", 5.0, 5.0, "L", 6.0, 7.0, "L", 5.0, 7.0, "Z"),
			"M0,0 L1,1 Z M5,5 L6,7 L5,7 Z", "M0,0 L1,1 M5,5 L6,7 L5,7 Z"},
		{"drawing after a close starts at the start", cvPath("M", 0.0, 0.0, "L", 10.0, 0.0, "Z", "L", 10.0, 10.0),
			"M0,0 L10,0 Z M0,0 L10,10 Z", "M0,0 L10,0 Z M0,0 L10,10"},
		{"lone moves dropped", cvPath("M", 9.0, 9.0, "M", 1.0, 2.0, "L", 3.0, 4.0, "M", 7.0, 7.0, "Z", "M", 8.0, 8.0),
			"M1,2 L3,4 Z", "M1,2 L3,4"},
		{"zero-length line kept", cvLine(3, 3, 3, 3), "M3,3 L3,3 Z", "M3,3 L3,3"},
		{"Inf point", cvLine(0, 0, cvInf, 5), "", ""},
		{"NaN control point", cvPath("M", 0.0, 0.0, "C", 0.0, cvNaN, 10.0, 10.0, 10.0, 0.0), "", ""},
		{"NaN in a lone move", cvPath("M", cvNaN, 0.0, "M", 1.0, 2.0, "L", 3.0, 4.0), "", ""},
		{"float32 overflow", cvLine(0, 0, cvBigF32, 5), "", ""},
		{"float32 overflow in a quad", cvPath("M", 0.0, 0.0, "Q", 1.0, -cvBigF32, 10.0, 0.0), "", ""},
	} {
		for _, fill := range []bool{true, false} {
			want := tc.stroke
			if fill {
				want = tc.fill
			}
			cmds, ok := pathCmds(tc.path, fill)
			if ok != (want != "") || (ok && fpath(cmds) != want) {
				t.Errorf("%s: pathCmds(fill=%v) = %q, %v, want %q, %v", tc.name, fill, fpath(cmds), ok, want, want != "")
			}
		}
	}
}

func TestCanvasStrokeArgs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		w     float64
		s     render.StrokeStyle
		want  string // "" when ok is false
		reach float64
	}{
		{"default", 2, render.StrokeStyle{}, "w=2 cap=0 join=0 miter=10", 10},
		{"round", 2, render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound}, "w=2 cap=1 join=1 miter=10", 1},
		{"square, bevel", 2, render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinBevel}, "w=2 cap=2 join=2 miter=10", math.Sqrt2},
		{"square, round", 4, render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinRound}, "w=4 cap=2 join=1 miter=10", 2 * math.Sqrt2},
		{"miter limit", 2, render.StrokeStyle{MiterLimit: 4}, "w=2 cap=0 join=0 miter=4", 4},
		{"miter limit 1", 2, render.StrokeStyle{MiterLimit: 1, Cap: render.CapSquare}, "w=2 cap=2 join=0 miter=1", math.Sqrt2},
		{"miter limit below 1", 2, render.StrokeStyle{MiterLimit: 0.5}, "w=2 cap=0 join=0 miter=10", 10},
		{"unknown cap and join", 2, render.StrokeStyle{Cap: 9, Join: 9, MiterLimit: 2}, "w=2 cap=0 join=0 miter=2", 2},
		{"hairline", 0.5, render.StrokeStyle{Join: render.JoinRound}, "w=0.5 cap=0 join=1 miter=10", 0.25},
		{"dashes", 2, render.StrokeStyle{Join: render.JoinBevel, Dash: []float64{4, 2}, DashOffset: 1},
			"w=2 cap=0 join=2 miter=10 dash=4,2@1", 1},
		{"odd dashes repeat", 2, render.StrokeStyle{Join: render.JoinBevel, Dash: []float64{3}},
			"w=2 cap=0 join=2 miter=10 dash=3,3@0", 1},
		{"negative offset", 2, render.StrokeStyle{Join: render.JoinBevel, Dash: []float64{4, 2}, DashOffset: -3},
			"w=2 cap=0 join=2 miter=10 dash=4,2@-3", 1},
		{"negative and NaN dashes count as 0", 2, render.StrokeStyle{Join: render.JoinBevel, Dash: []float64{4, -1, cvNaN, 2}},
			"w=2 cap=0 join=2 miter=10 dash=4,0,0,2@0", 1},
		{"zero dashes are solid", 2, render.StrokeStyle{Join: render.JoinBevel, Dash: []float64{0, 0}, DashOffset: 5},
			"w=2 cap=0 join=2 miter=10", 1},
		{"NaN offset", 2, render.StrokeStyle{Join: render.JoinBevel, Dash: []float64{1, 1}, DashOffset: cvNaN},
			"w=2 cap=0 join=2 miter=10 dash=1,1@0", 1},
		{"width 0", 0, render.StrokeStyle{}, "", 0},
		{"negative width", -2, render.StrokeStyle{}, "", 0},
		{"NaN width", cvNaN, render.StrokeStyle{}, "", 0},
		{"Inf width", cvInf, render.StrokeStyle{}, "", 0},
		{"float32 overflow width", cvBigF32, render.StrokeStyle{}, "", 0},
		{"float32 overflow miter is the default", 2, render.StrokeStyle{MiterLimit: cvBigF32}, "w=2 cap=0 join=0 miter=10", 10},
		{"square caps, miter limit below the square root of 2", 2, render.StrokeStyle{Cap: render.CapSquare, MiterLimit: 1.2}, "w=2 cap=2 join=0 miter=1.2", math.Sqrt2},
		{"square caps, miter limit 3", 2, render.StrokeStyle{Cap: render.CapSquare, MiterLimit: 3}, "w=2 cap=2 join=0 miter=3", 3},
		{"float32 overflow dash", 2, render.StrokeStyle{Dash: []float64{cvBigF32, 1}}, "", 0},
		{"float32 overflow offset", 2, render.StrokeStyle{Dash: []float64{1, 1}, DashOffset: cvBigF32}, "", 0},
	} {
		ns := tc.s.Normalized()
		st, ok := strokeArgs(render.StrokeWidth(tc.w), ns)
		if ok != (tc.want != "") || (ok && fstroke(st) != tc.want) {
			t.Errorf("%s: strokeArgs = %q, %v, want %q, %v", tc.name, fstroke(st), ok, tc.want, tc.want != "")
		}
		if !ok {
			continue
		}
		if got := strokeReach(st); math.Abs(got-tc.reach) > 1e-6 {
			t.Errorf("%s: strokeReach = %v, want %v", tc.name, got, tc.reach)
		}
	}
}

// TestCanvasShadowArgs checks the shadow node DrawShadow makes (S4.2d):
// the outline with clamped radii, the color, and Shadow.Normalized's
// offset, spread and blur passed through as GSK takes them, and every
// shadow that draws nothing.
func TestCanvasShadowArgs(t *testing.T) {
	off := render.Point{X: 2, Y: 3}
	outline := func(r render.Rect, radii render.Radii) gtk.RoundedRect {
		rf, _ := rectF(r)
		return roundedF(rf, radii)
	}
	shade := rgba(cvShade)
	for _, tc := range []struct {
		name  string
		r     render.Rect
		radii render.Radii
		s     render.Shadow
		want  shadowCall
		ok    bool
	}{
		{"outer", cvRect, render.UniformRadii(4), render.Shadow{Offset: off, Blur: 6, Spread: 1, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.UniformRadii(4)), color: shade, dx: 2, dy: 3, spread: 1, blur: 6}, true},
		{"inset", cvRect, render.UniformRadii(4), render.Shadow{Offset: off, Blur: 6, Spread: 1, Color: cvShade, Inset: true},
			shadowCall{outline: outline(cvRect, render.UniformRadii(4)), color: shade, dx: 2, dy: 3, spread: 1, blur: 6, inset: true}, true},
		{"zero blur", cvRect, render.Radii{}, render.Shadow{Offset: off, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, dx: 2, dy: 3}, true},
		{"negative blur is none", cvRect, render.Radii{}, render.Shadow{Blur: -4, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade}, true},
		{"NaN blur is none", cvRect, render.Radii{}, render.Shadow{Blur: cvNaN, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade}, true},
		{"negative spread", cvRect, render.Radii{}, render.Shadow{Spread: -5, Blur: 2, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, spread: -5, blur: 2}, true},
		{"negative spread almost to nothing", cvRect, render.Radii{}, render.Shadow{Spread: -14.9, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, spread: -14.9}, true},
		{"negative spread shrinks a side to nothing", cvRect, render.Radii{}, render.Shadow{Spread: -15, Blur: 8, Color: cvShade}, shadowCall{}, false},
		{"negative spread past nothing", cvRect, render.Radii{}, render.Shadow{Spread: -100, Color: cvShade}, shadowCall{}, false},
		{"inset hole shrunk away fills the shape", cvRect, render.Radii{}, render.Shadow{Spread: 100, Color: cvShade, Inset: true},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, spread: 100, inset: true}, true},
		{"inset negative spread", cvRect, render.Radii{}, render.Shadow{Spread: -3, Color: cvShade, Inset: true},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, spread: -3, inset: true}, true},
		{"radii clamped", cvRect, render.UniformRadii(100), render.Shadow{Color: cvShade},
			shadowCall{outline: outline(cvRect, render.UniformRadii(15)), color: shade}, true},
		{"per-corner radii", cvRect, render.Radii{TopLeft: 1, TopRight: 2, BottomRight: 3, BottomLeft: 4}, render.Shadow{Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{TopLeft: 1, TopRight: 2, BottomRight: 3, BottomLeft: 4}), color: shade}, true},
		{"NaN offset is 0", cvRect, render.Radii{}, render.Shadow{Offset: render.Point{X: cvNaN, Y: 1}, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, dy: 1}, true},
		{"Inf spread is 0", cvRect, render.Radii{}, render.Shadow{Spread: cvInf, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade}, true},
		{"translucent", cvRect, render.Radii{}, render.Shadow{Color: driver.RGBA(1, 0, 0, 0.01)},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: rgba(driver.RGBA(1, 0, 0, 0.01))}, true},
		{"transparent", cvRect, render.Radii{}, render.Shadow{Spread: 2, Blur: 4, Color: cvClear}, shadowCall{}, false},
		{"semantic", cvRect, render.Radii{}, render.Shadow{Color: cvLabel}, shadowCall{}, false},
		{"zero width", render.Rect{W: 0, H: 5}, render.Radii{}, render.Shadow{Spread: 7, Color: cvShade}, shadowCall{}, false},
		{"zero height inset", render.Rect{W: 5, H: 0}, render.Radii{}, render.Shadow{Spread: 7, Color: cvShade, Inset: true}, shadowCall{}, false},
		{"negative size", render.Rect{W: -5, H: 5}, render.Radii{}, render.Shadow{Spread: 7, Color: cvShade}, shadowCall{}, false},
		{"NaN rect", render.Rect{X: cvNaN, W: 5, H: 5}, render.Radii{}, render.Shadow{Color: cvShade}, shadowCall{}, false},
		{"Inf rect", render.Rect{W: cvInf, H: 5}, render.Radii{}, render.Shadow{Color: cvShade}, shadowCall{}, false},
		{"float32-overflowing rect", render.Rect{X: cvBigF32, W: 5, H: 5}, render.Radii{}, render.Shadow{Color: cvShade}, shadowCall{}, false},
		{"float32-overflowing offset", cvRect, render.Radii{}, render.Shadow{Offset: render.Point{Y: cvBigF32}, Color: cvShade}, shadowCall{}, false},
		{"float32-overflowing spread", cvRect, render.Radii{}, render.Shadow{Spread: cvBigF32, Color: cvShade, Inset: true}, shadowCall{}, false},
		{"float32-overflowing blur", cvRect, render.Radii{}, render.Shadow{Blur: cvBigF32, Color: cvShade}, shadowCall{}, false},
		{"huge spread", cvRect, render.Radii{}, render.Shadow{Spread: 3e38, Color: cvShade}, shadowCall{}, false},
		{"huge inset spread", cvRect, render.Radii{}, render.Shadow{Spread: 1e6 + 1, Color: cvShade, Inset: true}, shadowCall{}, false},
		{"huge negative offset", cvRect, render.Radii{}, render.Shadow{Offset: render.Point{X: -3e38}, Color: cvShade}, shadowCall{}, false},
		{"huge blur", cvRect, render.Radii{}, render.Shadow{Blur: 1e7, Color: cvShade}, shadowCall{}, false},
		{"at the bound", cvRect, render.Radii{}, render.Shadow{Offset: render.Point{X: -1e6, Y: 1e6}, Spread: 1e6, Blur: 1e6, Color: cvShade},
			shadowCall{outline: outline(cvRect, render.Radii{}), color: shade, dx: -1e6, dy: 1e6, spread: 1e6, blur: 1e6}, true},
	} {
		got, ok := shadowArgs(tc.r, tc.radii, tc.s.Normalized())
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: shadowArgs = %+v, %v, want %+v, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestCanvasDrawShadow: DrawShadow appends one shadow node, outset or
// inset, in whatever state is open, and opens none.
func TestCanvasDrawShadow(t *testing.T) {
	cvRun(t, []cvCase{
		{"outer", func(c *canvas) {
			c.DrawShadow(cvRect, render.UniformRadii(4), render.Shadow{Offset: render.Point{X: 2, Y: 3}, Spread: 1, Blur: 6, Color: cvShade})
		}, cvOps("outset shadow (10,20,30,40) r=4 " + cvShadeOp + " d=2,3 spread=1 blur=6")},
		{"inset", func(c *canvas) {
			c.DrawShadow(cvRect, render.Radii{}, render.Shadow{Offset: render.Point{X: 2}, Spread: 9, Color: cvShade, Inset: true})
		}, cvOps("inset shadow (10,20,30,40) " + cvShadeOp + " d=2,0 spread=9 blur=0")},
		{"in save, clip and opacity", func(c *canvas) {
			c.Save()
			c.Translate(5, 6)
			c.ClipRoundedRect(cvRect, render.Radii{})
			c.PushOpacity(0.5)
			c.DrawShadow(cvRect, render.Radii{TopLeft: 3}, render.Shadow{Blur: 2, Color: cvRed})
			c.Pop()
			c.Restore()
		}, cvOps("save", "translate 5,6", "clip (10,20,30,40)", "opacity 0.5", "save",
			"outset shadow (10,20,30,40) r=3/0/0/0 "+cvRedOp+" d=0,0 spread=0 blur=2",
			"restore", "pop", "pop", "restore")},
		{"left open, closed by finish", func(c *canvas) {
			c.PushOpacity(0.5)
			c.DrawShadow(cvRect, render.Radii{}, render.Shadow{Color: cvShade, Inset: true})
		}, cvOps("opacity 0.5", "save", "inset shadow (10,20,30,40) "+cvShadeOp+" d=0,0 spread=0 blur=0", "restore", "pop")},
		{"transparent", func(c *canvas) { c.DrawShadow(cvRect, render.Radii{}, render.Shadow{Spread: 2}) }, cvOps()},
		{"semantic", func(c *canvas) { c.DrawShadow(cvRect, render.Radii{}, render.Shadow{Color: cvLabel}) }, cvOps()},
		{"shrunk away", func(c *canvas) { c.DrawShadow(cvRect, render.Radii{}, render.Shadow{Spread: -20, Color: cvShade}) }, cvOps()},
		{"empty", func(c *canvas) {
			c.DrawShadow(render.Rect{X: 5, Y: 5}, render.Radii{}, render.Shadow{Spread: 3, Color: cvShade})
		}, cvOps()},
	})
}

func TestCanvasStrokeOutline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		radii     render.Radii
		w         float64
		wantR     render.Rect
		wantRadii render.Radii
	}{
		{"sharp", render.Radii{}, 2, render.Rect{X: 9, Y: 19, W: 32, H: 42}, render.Radii{}},
		{"rounded", render.UniformRadii(5), 2, render.Rect{X: 9, Y: 19, W: 32, H: 42}, render.UniformRadii(6)},
		{"mixed corners", render.Radii{TopLeft: 4, BottomRight: 1}, 4,
			render.Rect{X: 8, Y: 18, W: 34, H: 44}, render.Radii{TopLeft: 6, BottomRight: 3}},
		{"each corner", render.Radii{TopLeft: 1, TopRight: 2, BottomRight: 3, BottomLeft: 4}, 1,
			render.Rect{X: 9.5, Y: 19.5, W: 31, H: 41}, render.Radii{TopLeft: 1.5, TopRight: 2.5, BottomRight: 3.5, BottomLeft: 4.5}},
		{"zero width", render.UniformRadii(5), 0, cvRect, render.UniformRadii(5)},
	} {
		r, radii := strokeOutline(cvRect, tc.radii, tc.w)
		if r != tc.wantR || radii != tc.wantRadii {
			t.Errorf("%s: strokeOutline = %v, %v, want %v, %v", tc.name, r, radii, tc.wantR, tc.wantRadii)
		}
	}
}

func TestCanvasGradientAt(t *testing.T) {
	two := []render.GradientStop{cvStop(0.2, cvRed), cvStop(0.8, cvBlue)}
	hard := []render.GradientStop{cvStop(0.5, cvRed), cvStop(0.5, cvGreen), cvStop(1, cvBlue)}
	late := []render.GradientStop{cvStop(0.2, cvRed), cvStop(0.5, cvGreen), cvStop(0.5, cvBlue)}
	fade := []render.GradientStop{cvStop(0, cvRed), cvStop(1, cvClear)}
	for _, tc := range []struct {
		name  string
		stops []render.GradientStop
		t     float64
		want  render.Color
		ok    bool
	}{
		{"no stops", nil, 0.5, render.Color{}, false},
		{"one stop before", []render.GradientStop{cvStop(0.5, cvGreen)}, 0, cvGreen, true},
		{"one stop after", []render.GradientStop{cvStop(0.5, cvGreen)}, 1, cvGreen, true},
		{"before the first", two, 0, cvRed, true},
		{"at the first", two, 0.2, cvRed, true},
		{"after the last", two, 1, cvBlue, true},
		{"at the last", two, 0.8, cvBlue, true},
		{"middle", two, 0.5, driver.RGBA(0.5, 0, 0.5, 1), true},
		{"quarter", two, 0.35, driver.RGBA(0.75, 0, 0.25, 1), true},
		{"three quarters", two, 0.65, driver.RGBA(0.25, 0, 0.75, 1), true},
		{"hard change at it", hard, 0.5, cvRed, true},
		{"hard change after it", hard, 0.6, driver.RGBA(0, 0.8, 0.2, 1), true},
		{"hard change at the end", late, 0.5, cvGreen, true},
		{"hard change past the end", late, 0.7, cvBlue, true},
		{"straight alpha", fade, 0.5, driver.RGBA(0.5, 0, 0, 0.5), true},
	} {
		got, ok := gradientAt(tc.stops, tc.t)
		if ok != tc.ok || !cvNear(got, tc.want) {
			t.Errorf("%s: gradientAt(%v) = %v, %v, want %v, %v", tc.name, tc.t, got, ok, tc.want, tc.ok)
		}
	}
}

// --- The canvas ----------------------------------------------------------------

func TestCanvasBase(t *testing.T) {
	cvWantOps(t, cvDraw(t, 1, func(*canvas) {}), cvOps())
	for _, tc := range []struct {
		name string
		size render.Size
		want []string
	}{
		{"fractional", render.Size{W: 10.5, H: 0.25}, []string{"clip (0,0,10.5,0.25)", "pop"}},
		{"empty", render.Size{}, []string{"clip (0,0,0,0)", "pop"}},
		{"negative clips everything", render.Size{W: -1, H: 5}, []string{"clip (0,0,0,0)", "pop"}},
		{"NaN clips everything", render.Size{W: 5, H: cvNaN}, []string{"clip (0,0,0,0)", "pop"}},
		{"Inf is no clip", render.Size{W: cvInf, H: 5}, nil},
	} {
		sn := &fakeSnapshot{}
		c := newCanvas(sn, tc.size, 1, false)
		if err := c.finish(); err != nil {
			t.Errorf("%s: finish = %v", tc.name, err)
		}
		if err := sn.balanced(); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		if !slices.Equal(sn.ops, tc.want) {
			t.Errorf("%s: ops %q, want %q", tc.name, sn.ops, tc.want)
		}
	}
}

func TestCanvasFill(t *testing.T) {
	fill := func(r render.Rect, radii render.Radii, p render.Paint) func(c *canvas) {
		return func(c *canvas) { c.FillRoundedRect(r, radii, p) }
	}
	red := render.Solid(cvRed)
	cvRun(t, []cvCase{
		{"sharp", fill(cvRect, render.Radii{}, red), cvOps("color (10,20,30,40) " + cvRedOp)},
		{"rounded", fill(cvRect, render.UniformRadii(5), red),
			cvOps("clip (10,20,30,40) r=5", "color (10,20,30,40) "+cvRedOp, "pop")},
		{"radii clamped", fill(cvRect, render.UniformRadii(100), red),
			cvOps("clip (10,20,30,40) r=15", "color (10,20,30,40) "+cvRedOp, "pop")},
		{"radii clamped per corner", fill(cvRect, render.Radii{TopLeft: 4, TopRight: -1, BottomRight: cvNaN, BottomLeft: 100}, red),
			cvOps("clip (10,20,30,40) r=4/0/0/15", "color (10,20,30,40) "+cvRedOp, "pop")},
		{"negative radii are sharp", fill(cvRect, render.UniformRadii(-3), red), cvOps("color (10,20,30,40) " + cvRedOp)},
		{"NaN radii are sharp", fill(cvRect, render.UniformRadii(cvNaN), red), cvOps("color (10,20,30,40) " + cvRedOp)},
		{"translucent", fill(cvRect, render.Radii{}, render.Solid(cvShade)), cvOps("color (10,20,30,40) " + cvShadeOp)},
		{"transparent", fill(cvRect, render.UniformRadii(5), render.Solid(cvClear)), cvOps()},
		{"zero paint", fill(cvRect, render.Radii{}, render.Paint{}), cvOps()},
		{"semantic", fill(cvRect, render.UniformRadii(5), render.Solid(cvLabel)), cvOps()},
		{"negative width", fill(render.Rect{W: -1, H: 5}, render.Radii{}, red), cvOps()},
		{"negative height", fill(render.Rect{W: 5, H: -1}, render.Radii{}, red), cvOps()},
		{"zero width", fill(render.Rect{W: 0, H: 5}, render.UniformRadii(1), red), cvOps()},
		{"zero height", fill(render.Rect{W: 5, H: 0}, render.Radii{}, red), cvOps()},
		{"NaN x", fill(render.Rect{X: cvNaN, W: 5, H: 5}, render.Radii{}, red), cvOps()},
		{"NaN width", fill(render.Rect{W: cvNaN, H: 5}, render.Radii{}, red), cvOps()},
		{"Inf width", fill(render.Rect{W: cvInf, H: 5}, render.Radii{}, red), cvOps()},
		{"float32 overflow", fill(render.Rect{Y: -cvBigF32, W: 5, H: 5}, render.Radii{}, red), cvOps()},
	})
}

func TestCanvasFillGradient(t *testing.T) {
	fill := func(radii render.Radii, start, end render.Point, stops ...render.GradientStop) func(c *canvas) {
		return func(c *canvas) { c.FillRoundedRect(cvRect, radii, render.LinearGradient(start, end, stops...)) }
	}
	o, x := render.Point{}, render.Point{X: 10}
	rgb := []render.GradientStop{cvStop(1.5, cvBlue), cvStop(-1, cvRed), cvStop(0.5, cvGreen)}
	const rgbOp = "linear (10,20,30,40) (0,0)->(10,0) [0:rgba(1,0,0,1) 0.5:rgba(0,1,0,1) 1:rgba(0,0,1,1)]"
	cvRun(t, []cvCase{
		{"stops sorted and clamped", fill(render.Radii{}, o, x, rgb...), cvOps(rgbOp)},
		{"rounded", fill(render.UniformRadii(5), o, x, rgb...),
			cvOps("clip (10,20,30,40) r=5", rgbOp, "pop")},
		{"points", fill(render.Radii{}, render.Point{X: 1.5, Y: 2}, render.Point{X: -3, Y: 40},
			cvStop(0, cvRed), cvStop(1, cvClear)),
			cvOps("linear (10,20,30,40) (1.5,2)->(-3,40) [0:rgba(1,0,0,1) 1:rgba(0,0,0,0)]")},
		{"one transparent stop is drawn", fill(render.Radii{}, o, x, cvStop(0, cvClear), cvStop(1, cvShade)),
			cvOps("linear (10,20,30,40) (0,0)->(10,0) [0:rgba(0,0,0,0) 1:" + cvShadeOp + "]")},
		{"one stop is a color", fill(render.UniformRadii(5), o, x, cvStop(0.3, cvBlue)),
			cvOps("clip (10,20,30,40) r=5", "color (10,20,30,40) "+cvBlueOp, "pop")},
		{"one transparent stop", fill(render.Radii{}, o, x, cvStop(0.3, cvClear)), cvOps()},
		{"no stops", fill(render.UniformRadii(5), o, x), cvOps()},
		{"start is end: the last stop", fill(render.Radii{}, x, x, cvStop(1, cvBlue), cvStop(0, cvRed)),
			cvOps("color (10,20,30,40) " + cvBlueOp)},
		{"start is end, last stop transparent", fill(render.Radii{}, x, x, cvStop(0, cvRed), cvStop(1, cvClear)), cvOps()},
		{"all transparent", fill(render.UniformRadii(5), o, x, cvStop(0, cvClear), cvStop(1, driver.RGBA(1, 1, 1, 0))), cvOps()},
		{"semantic stop", fill(render.Radii{}, o, x, cvStop(0, cvRed), cvStop(1, cvLabel)), cvOps()},
		// Normalized sets non-finite end points to 0: here start == end.
		{"NaN end point", fill(render.Radii{}, o, render.Point{X: cvNaN}, cvStop(0, cvRed), cvStop(1, cvBlue)),
			cvOps("color (10,20,30,40) " + cvBlueOp)},
		{"Inf start point", fill(render.Radii{}, render.Point{Y: cvInf}, x, cvStop(0, cvRed), cvStop(1, cvBlue)),
			cvOps("linear (10,20,30,40) (0,0)->(10,0) [0:rgba(1,0,0,1) 1:rgba(0,0,1,1)]")},
		{"end point overflows float32", fill(render.UniformRadii(5), o, render.Point{X: cvBigF32}, cvStop(0, cvRed), cvStop(1, cvBlue)), cvOps()},
		{"start point overflows float32", fill(render.Radii{}, render.Point{Y: -cvBigF32}, x, cvStop(0, cvRed), cvStop(1, cvBlue)), cvOps()},
	})
}

func TestCanvasStroke(t *testing.T) {
	stroke := func(r render.Rect, radii render.Radii, w float64, p render.Paint) func(c *canvas) {
		return func(c *canvas) { c.StrokeRoundedRect(r, radii, w, p) }
	}
	red := render.Solid(cvRed)
	cvRun(t, []cvCase{
		{"sharp", stroke(cvRect, render.Radii{}, 2, red), cvOps("border (9,19,32,42) w=2 " + cvRedOp)},
		{"rounded", stroke(cvRect, render.UniformRadii(5), 2, red), cvOps("border (9,19,32,42) r=6 w=2 " + cvRedOp)},
		{"sharp corners stay sharp", stroke(cvRect, render.Radii{TopLeft: 5, BottomRight: -2}, 1, red),
			cvOps("border (9.5,19.5,31,41) r=5.5/0/0/0 w=1 " + cvRedOp)},
		{"radii clamped", stroke(cvRect, render.UniformRadii(100), 2, red), cvOps("border (9,19,32,42) r=16 w=2 " + cvRedOp)},
		{"hairline", stroke(cvRect, render.Radii{}, 0.5, render.Solid(cvShade)), cvOps("border (9.75,19.75,30.5,40.5) w=0.5 " + cvShadeOp)},
		{"empty rect", stroke(render.Rect{X: 5, Y: 5}, render.UniformRadii(3), 2, red), cvOps("border (4,4,2,2) w=2 " + cvRedOp)},
		{"width 0", stroke(cvRect, render.Radii{}, 0, red), cvOps()},
		{"negative width", stroke(cvRect, render.Radii{}, -2, red), cvOps()},
		{"NaN width", stroke(cvRect, render.Radii{}, cvNaN, red), cvOps()},
		{"Inf width", stroke(cvRect, render.Radii{}, cvInf, red), cvOps()},
		{"width overflows float32", stroke(cvRect, render.Radii{}, cvBigF32, red), cvOps()},
		{"transparent", stroke(cvRect, render.Radii{}, 2, render.Solid(cvClear)), cvOps()},
		{"semantic", stroke(cvRect, render.Radii{}, 2, render.Solid(cvLabel)), cvOps()},
		{"gradient without stops", stroke(cvRect, render.Radii{}, 2, render.LinearGradient(render.Point{}, render.Point{X: 1})), cvOps()},
		{"negative size", stroke(render.Rect{W: -1, H: 5}, render.Radii{}, 2, red), cvOps()},
		{"NaN rect", stroke(render.Rect{X: cvNaN, W: 5, H: 5}, render.Radii{}, 2, red), cvOps()},
	})
}

// TestCanvasStrokeGradient: a gradient stroke of a rounded rect (S4.2b) is
// a stroke push of the rect's outline (radii clamped) with the gradient
// node over the stroke's outer bounds, popped at once, so it nests inside
// any open save, clip or layer; degenerate gradients stroke as border
// nodes in their one color, and an empty rect in the middle color.
func TestCanvasStrokeGradient(t *testing.T) {
	stroke := func(r render.Rect, radii render.Radii, w float64, p render.Paint) func(c *canvas) {
		return func(c *canvas) { c.StrokeRoundedRect(r, radii, w, p) }
	}
	x := render.Point{X: 10}
	grad := render.LinearGradient(render.Point{}, x, cvStop(0, cvRed), cvStop(1, cvBlue))
	stops := "[0:" + cvRedOp + " 1:" + cvBlueOp + "]"
	body := func(outline string, w string) []string {
		return []string{"stroke " + outline + " w=" + w, "linear (9,19,32,42) (0,0)->(10,0) " + stops, "pop"}
	}
	cvRun(t, []cvCase{
		{"sharp", stroke(cvRect, render.Radii{}, 2, grad), cvOps(body("(10,20,30,40)", "2")...)},
		{"rounded", stroke(cvRect, render.UniformRadii(5), 2, grad), cvOps(body("(10,20,30,40) r=5", "2")...)},
		{"radii clamped", stroke(cvRect, render.UniformRadii(100), 2, grad), cvOps(body("(10,20,30,40) r=15", "2")...)},
		{"per-corner radii", stroke(cvRect, render.Radii{TopLeft: 5, BottomRight: -2}, 2, grad),
			cvOps(body("(10,20,30,40) r=5/0/0/0", "2")...)},
		{"hairline", stroke(cvRect, render.Radii{}, 0.5, grad),
			cvOps("stroke (10,20,30,40) w=0.5", "linear (9.75,19.75,30.5,40.5) (0,0)->(10,0) "+stops, "pop")},
		{"unsorted stops sorted", stroke(cvRect, render.Radii{}, 2,
			render.LinearGradient(render.Point{}, x, cvStop(1, cvBlue), cvStop(0, cvRed))), cvOps(body("(10,20,30,40)", "2")...)},
		{"three stops", stroke(cvRect, render.Radii{}, 2,
			render.LinearGradient(render.Point{}, x, cvStop(0, cvRed), cvStop(0.5, cvClear), cvStop(1, cvBlue))),
			cvOps("stroke (10,20,30,40) w=2", "linear (9,19,32,42) (0,0)->(10,0) [0:"+cvRedOp+" 0.5:rgba(0,0,0,0) 1:"+cvBlueOp+"]", "pop")},
		{"inside save, clip and opacity", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(render.Rect{X: 1, Y: 2, W: 3, H: 4}, render.Radii{})
			c.PushOpacity(0.5)
			c.StrokeRoundedRect(cvRect, render.Radii{}, 2, grad)
			c.Pop()
			c.StrokeRoundedRect(cvRect, render.Radii{}, 2, grad)
			c.Restore()
		}, cvOps(slices.Concat([]string{"save", "clip (1,2,3,4)", "opacity 0.5", "save"}, body("(10,20,30,40)", "2"),
			[]string{"restore", "pop"}, body("(10,20,30,40)", "2"), []string{"pop", "restore"})...)},
		{"left open, closed by finish", func(c *canvas) {
			c.Save()
			c.PushOpacity(0.5)
			c.StrokeRoundedRect(cvRect, render.Radii{}, 2, grad)
		}, cvOps(slices.Concat([]string{"save", "opacity 0.5", "save"}, body("(10,20,30,40)", "2"),
			[]string{"restore", "pop", "restore"})...)},
		{"one stop: a border", stroke(cvRect, render.Radii{}, 2, render.LinearGradient(render.Point{}, x, cvStop(0.3, cvBlue))),
			cvOps("border (9,19,32,42) w=2 " + cvBlueOp)},
		{"zero-length line: the last stop", stroke(cvRect, render.UniformRadii(5), 2,
			render.LinearGradient(x, x, cvStop(0, cvRed), cvStop(1, cvBlue))), cvOps("border (9,19,32,42) r=6 w=2 " + cvBlueOp)},
		{"empty rect: the middle color", stroke(render.Rect{X: 5, Y: 5, W: 10}, render.Radii{}, 2, grad),
			cvOps("border (4,4,12,2) w=2 rgba(0.5,0,0.5,1)")},
		{"empty rect, transparent middle", stroke(render.Rect{X: 5, Y: 5}, render.Radii{}, 2,
			render.LinearGradient(render.Point{}, x, cvStop(0, cvRed), cvStop(0.5, cvClear), cvStop(1, cvBlue))), cvOps()},
		{"all stops transparent", stroke(cvRect, render.Radii{}, 2,
			render.LinearGradient(render.Point{}, x, cvStop(0, cvClear), cvStop(1, cvClear))), cvOps()},
		{"semantic stop", stroke(cvRect, render.Radii{}, 2,
			render.LinearGradient(render.Point{}, x, cvStop(0, cvRed), cvStop(1, cvLabel))), cvOps()},
		{"NaN start point normalized to 0", stroke(cvRect, render.Radii{}, 2,
			render.LinearGradient(render.Point{X: cvNaN}, x, cvStop(0, cvRed), cvStop(1, cvBlue))), cvOps(body("(10,20,30,40)", "2")...)},
		{"end point overflows float32", stroke(cvRect, render.Radii{}, 2,
			render.LinearGradient(render.Point{}, render.Point{X: cvBigF32}, cvStop(0, cvRed), cvStop(1, cvBlue))), cvOps()},
		{"width 0", stroke(cvRect, render.Radii{}, 0, grad), cvOps()},
		{"NaN width", stroke(cvRect, render.Radii{}, cvNaN, grad), cvOps()},
		{"width overflows float32", stroke(cvRect, render.Radii{}, cvBigF32, grad), cvOps()},
		{"NaN rect", stroke(render.Rect{Y: cvNaN, W: 5, H: 5}, render.Radii{}, 2, grad), cvOps()},
	})
}

func TestCanvasState(t *testing.T) {
	clipA := render.Rect{X: 1, Y: 2, W: 3, H: 4}
	clipB := render.Rect{X: 5, Y: 6, W: 70, H: 80}
	cvRun(t, []cvCase{
		{"save restore", func(c *canvas) { c.Save(); c.Restore() }, cvOps("save", "restore")},
		{"clips popped before restore", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(clipA, render.Radii{})
			c.ClipRoundedRect(clipB, render.UniformRadii(100))
			c.Restore()
		}, cvOps("save", "clip (1,2,3,4)", "clip (5,6,70,80) r=35", "pop", "pop", "restore")},
		{"clip in the base", func(c *canvas) { c.ClipRoundedRect(clipA, render.UniformRadii(1)) },
			[]string{cvBaseOp, "clip (1,2,3,4) r=1", "pop", "pop"}},
		{"clip radii per corner", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(clipB, render.Radii{TopLeft: 1, TopRight: 2, BottomRight: 3, BottomLeft: 4})
			c.Restore()
		}, cvOps("save", "clip (5,6,70,80) r=1/2/3/4", "pop", "restore")},
		{"nested saves", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(clipA, render.Radii{})
			c.Save()
			c.ClipRoundedRect(clipB, render.Radii{})
			c.Restore()
			c.FillRoundedRect(cvRect, render.Radii{}, render.Solid(cvRed))
			c.Restore()
		}, cvOps("save", "clip (1,2,3,4)", "save", "clip (5,6,70,80)", "pop", "restore",
			"color (10,20,30,40) "+cvRedOp, "pop", "restore")},
		{"opacity", func(c *canvas) {
			c.PushOpacity(0.5)
			c.ClipRoundedRect(clipA, render.Radii{})
			c.Pop()
		}, cvOps("opacity 0.5", "save", "clip (1,2,3,4)", "pop", "restore", "pop")},
		{"opacity clamped high", func(c *canvas) { c.PushOpacity(1.5); c.Pop() }, cvOps("opacity 1", "save", "restore", "pop")},
		{"opacity clamped low", func(c *canvas) { c.PushOpacity(-1); c.Pop() }, cvOps("opacity 0", "save", "restore", "pop")},
		{"opacity NaN", func(c *canvas) { c.PushOpacity(cvNaN); c.Pop() }, cvOps("opacity 0", "save", "restore", "pop")},
		{"backdrop blur, clip only (radius 0; the blur: TestCanvasBlur)", func(c *canvas) {
			c.PushBackdropBlur(cvRect, render.UniformRadii(5), 0)
			c.ClipRoundedRect(clipA, render.Radii{})
			c.Pop()
		}, cvOps("save", "clip (10,20,30,40) r=5", "clip (1,2,3,4)", "pop", "pop", "restore")},
		{"backdrop blur radii clamped", func(c *canvas) { c.PushBackdropBlur(cvRect, render.UniformRadii(50), 0); c.Pop() },
			cvOps("save", "clip (10,20,30,40) r=15", "pop", "restore")},
		{"backdrop blur empty", func(c *canvas) { c.PushBackdropBlur(render.Rect{X: 1, Y: 2, H: 5}, render.UniformRadii(5), 8); c.Pop() },
			cvOps("save", "clip (1,2,0,5)", "pop", "restore")},
		{"backdrop blur negative size", func(c *canvas) { c.PushBackdropBlur(render.Rect{X: 1, W: -5, H: 5}, render.Radii{}, 8); c.Pop() },
			cvOps("save", "clip (0,0,0,0)", "pop", "restore")},
		{"backdrop blur Inf", func(c *canvas) { c.PushBackdropBlur(render.Rect{X: cvInf, W: 5, H: 5}, render.Radii{}, 8); c.Pop() },
			cvOps("save", "clip (0,0,0,0)", "pop", "restore")},
		{"layers nest", func(c *canvas) {
			c.PushOpacity(0.5)
			c.Save()
			c.PushBackdropBlur(clipA, render.Radii{}, 0)
			c.Pop()
			c.Restore()
			c.Pop()
		}, cvOps("opacity 0.5", "save", "save", "save", "clip (1,2,3,4)",
			"pop", "restore", "restore", "restore", "pop")},
		{"negative-size clip clips everything", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(render.Rect{X: 1, Y: 2, W: -3, H: 4}, render.UniformRadii(1))
			c.ClipRoundedRect(render.Rect{X: 1, Y: 2, W: 3, H: cvNaN}, render.Radii{})
			c.Restore()
		}, cvOps("save", "clip (0,0,0,0)", "clip (0,0,0,0)", "pop", "pop", "restore")},
		{"non-finite clip clips everything", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(render.Rect{X: cvInf, W: 3, H: 4}, render.Radii{})
			c.ClipRoundedRect(render.Rect{Y: cvNaN, W: 3, H: 4}, render.Radii{})
			c.ClipRoundedRect(render.Rect{W: cvInf, H: 4}, render.Radii{})
			c.ClipRoundedRect(render.Rect{X: cvBigF32, W: 3, H: 4}, render.Radii{})
			c.Restore()
		}, cvOps("save", "clip (0,0,0,0)", "clip (0,0,0,0)", "clip (0,0,0,0)", "clip (0,0,0,0)", "pop", "pop", "pop", "pop", "restore")},
		{"zero-size clip", func(c *canvas) { c.ClipRoundedRect(render.Rect{X: 3, Y: 4}, render.UniformRadii(2)) },
			[]string{cvBaseOp, "clip (3,4,0,0)", "pop", "pop"}},
	})
}

func TestCanvasMismatched(t *testing.T) {
	cvRun(t, []cvCase{
		{"restore in the base", func(c *canvas) { c.Restore() }, cvOps()},
		{"pop in the base", func(c *canvas) { c.Pop() }, cvOps()},
		{"restore after clip in the base", func(c *canvas) {
			c.ClipRoundedRect(render.Rect{W: 1, H: 1}, render.Radii{})
			c.Restore()
			c.Pop()
		}, []string{cvBaseOp, "clip (0,0,1,1)", "pop", "pop"}},
		{"pop closes no save", func(c *canvas) { c.Save(); c.Pop(); c.Translate(1, 1); c.Restore() },
			cvOps("save", "translate 1,1", "restore")},
		{"restore closes no opacity", func(c *canvas) { c.PushOpacity(0.5); c.Restore(); c.Translate(1, 1); c.Pop() },
			cvOps("opacity 0.5", "save", "translate 1,1", "restore", "pop")},
		{"restore closes no blur", func(c *canvas) {
			c.PushBackdropBlur(render.Rect{W: 1, H: 1}, render.Radii{}, 0)
			c.Restore()
			c.Translate(1, 1)
			c.Pop()
		},
			cvOps("save", "clip (0,0,1,1)", "translate 1,1", "pop", "restore")},
		{"extra restore", func(c *canvas) { c.Save(); c.Restore(); c.Restore() }, cvOps("save", "restore")},
		{"extra pop", func(c *canvas) { c.PushOpacity(1); c.Pop(); c.Pop() }, cvOps("opacity 1", "save", "restore", "pop")},
		{"crossed pairs", func(c *canvas) {
			c.Save()
			c.PushOpacity(0.5)
			c.Restore() // ignored: closes the opacity
			c.Translate(1, 1)
			c.Pop()
			c.Pop() // ignored: closes the save
			c.Translate(2, 2)
			c.Restore()
		}, cvOps("save", "opacity 0.5", "save", "translate 1,1", "restore", "pop", "translate 2,2", "restore")},
	})
}

func TestCanvasFinishCloses(t *testing.T) {
	clip := render.Rect{X: 1, Y: 2, W: 3, H: 4}
	cvRun(t, []cvCase{
		{"save", func(c *canvas) { c.Save() }, cvOps("save", "restore")},
		{"save and clip", func(c *canvas) { c.Save(); c.ClipRoundedRect(clip, render.Radii{}) },
			cvOps("save", "clip (1,2,3,4)", "pop", "restore")},
		{"opacity", func(c *canvas) { c.PushOpacity(0.5) }, cvOps("opacity 0.5", "save", "restore", "pop")},
		{"blur", func(c *canvas) { c.PushBackdropBlur(clip, render.Radii{}, 0) },
			cvOps("save", "clip (1,2,3,4)", "pop", "restore")},
		{"everything", func(c *canvas) {
			c.ClipRoundedRect(clip, render.Radii{})
			c.Save()
			c.ClipRoundedRect(clip, render.Radii{})
			c.PushOpacity(0.25)
			c.ClipRoundedRect(clip, render.Radii{})
			c.ClipRoundedRect(clip, render.Radii{})
			c.PushBackdropBlur(clip, render.Radii{}, 0)
			c.Save()
		}, []string{cvBaseOp, "clip (1,2,3,4)", "save", "clip (1,2,3,4)", "opacity 0.25", "save",
			"clip (1,2,3,4)", "clip (1,2,3,4)", "save", "clip (1,2,3,4)", "save",
			// finish:
			"restore",        // the last save
			"pop", "restore", // the blur
			"pop", "pop", "restore", "pop", // the opacity
			"pop", "restore", // the first save
			"pop", "pop", // the base
		}},
	})
}

func TestCanvasAfterFinish(t *testing.T) {
	for _, debug := range []bool{false, true} {
		sn := &fakeSnapshot{}
		c := newCanvas(sn, render.Size{W: 100, H: 50}, 1, debug)
		c.Save()
		if err := c.finish(); (err != nil) != debug {
			t.Errorf("debug=%v: finish = %v", debug, err)
		}
		want := slices.Clone(sn.ops)
		img := cvImage{}
		layout := fake.NewTextSystem().NewLayout("abc", render.Font{Size: 13}, 0)
		red := render.Solid(cvRed)
		c.Save()
		c.Restore()
		c.Restore()
		c.Translate(1, 2)
		c.Scale(2, 2)
		c.Concat(rotation(1))
		c.Concat(render.Transform{A: 2, D: 2, E: 1, F: 1})
		c.ClipRoundedRect(cvRect, render.Radii{})
		c.ClipRoundedRect(render.Rect{W: -1, H: 1}, render.Radii{})
		c.FillRoundedRect(cvRect, render.UniformRadii(4), red)
		c.StrokeRoundedRect(cvRect, render.Radii{}, 1, red)
		c.FillPath(cvLine(0, 0, 5, 5), red)
		c.StrokePath(cvLine(0, 0, 5, 5), 1, red, render.StrokeStyle{})
		c.DrawShadow(cvRect, render.Radii{}, render.Shadow{Color: cvShade})
		c.DrawText(layout, render.Point{}, cvRed)
		c.DrawImage(img, cvRect)
		c.PushOpacity(0.5)
		c.Pop()
		c.PushBackdropBlur(cvRect, render.Radii{}, 2)
		c.Pop()
		c.Pop()
		if err := c.finish(); err != nil {
			t.Errorf("debug=%v: second finish = %v, want nil", debug, err)
		}
		if !slices.Equal(sn.ops, want) {
			t.Errorf("debug=%v: calls after finish drew:\n%s", debug, strings.Join(sn.ops[len(want):], "\n"))
		}
		if err := sn.balanced(); err != nil {
			t.Errorf("debug=%v: %v", debug, err)
		}
		if debug && len(c.rec.Errors()) < 2 {
			t.Errorf("recorder errors after finish = %v, want the calls after End", c.rec.Errors())
		}
	}
}

func TestCanvasDebug(t *testing.T) {
	for _, tc := range []struct {
		name    string
		paint   func(c *canvas)
		wantErr string // "" for none
	}{
		{"clean", func(c *canvas) {
			c.Save()
			c.FillRoundedRect(cvRect, render.Radii{}, render.Solid(cvRed))
			c.Restore()
		}, ""},
		{"unclosed save", func(c *canvas) { c.Save() }, "not closed"},
		{"unclosed opacity", func(c *canvas) { c.PushOpacity(1) }, "not closed"},
		{"stray restore", func(c *canvas) { c.Restore() }, "Restore"},
		{"semantic color", func(c *canvas) { c.FillRoundedRect(cvRect, render.Radii{}, render.Solid(cvLabel)) }, "semantic"},
	} {
		for _, debug := range []bool{false, true} {
			sn := &fakeSnapshot{}
			c := newCanvas(sn, render.Size{W: 100, H: 50}, 1, debug)
			if (c.rec != nil) != debug {
				t.Errorf("%s: debug=%v but recorder %v", tc.name, debug, c.rec)
			}
			tc.paint(c)
			err := c.finish()
			switch {
			case !debug || tc.wantErr == "":
				if err != nil {
					t.Errorf("%s debug=%v: finish = %v, want nil", tc.name, debug, err)
				}
			case err == nil || !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("%s: finish = %v, want an error with %q", tc.name, err, tc.wantErr)
			}
			if err := sn.balanced(); err != nil {
				t.Errorf("%s debug=%v: %v", tc.name, debug, err)
			}
		}
	}
}

func TestCanvasTransform(t *testing.T) {
	concat := func(m render.Transform) func(c *canvas) { return func(c *canvas) { c.Concat(m) } }
	cvRun(t, []cvCase{
		{"translate", func(c *canvas) { c.Translate(1.5, -2) }, cvOps("translate 1.5,-2")},
		{"translate NaN", func(c *canvas) { c.Translate(cvNaN, 1) }, cvOps()},
		{"translate Inf", func(c *canvas) { c.Translate(1, -cvInf) }, cvOps()},
		{"translate overflows float32", func(c *canvas) { c.Translate(cvBigF32, 1) }, cvOps()},
		{"scale", func(c *canvas) { c.Scale(2, 0.5) }, cvOps("scale 2,0.5")},
		{"scale NaN", func(c *canvas) { c.Scale(1, cvNaN) }, cvOps()},
		{"scale Inf", func(c *canvas) { c.Scale(cvInf, 1) }, cvOps()},
		{"scale overflows float32", func(c *canvas) { c.Scale(1, -cvBigF32) }, cvOps()},
		{"concat identity", concat(render.Identity()), cvOps()},
		{"concat translation", concat(render.Translation(3, 4)), cvOps("translate 3,4")},
		{"concat translation x", concat(render.Translation(3, 0)), cvOps("translate 3,0")},
		{"concat translation y", concat(render.Translation(0, 4)), cvOps("translate 0,4")},
		{"concat scaling", concat(render.Scaling(2, 3)), cvOps("scale 2,3")},
		{"concat scaling x", concat(render.Scaling(2, 1)), cvOps("scale 2,1")},
		{"concat scaling y", concat(render.Scaling(1, -1)), cvOps("scale 1,-1")},
		{"concat translate then scale", concat(render.Transform{A: 2, D: 3, E: 4, F: 5}), cvOps("translate 4,5", "scale 2,3")},
		{"concat matrix", concat(render.Transform{A: 1, B: 2, C: 3, D: 4, E: 5, F: 6}), cvOps("matrix 1,2,3,4,5,6")},
		{"concat skew b", concat(render.Transform{A: 1, B: 0.5, D: 1}), cvOps("matrix 1,0.5,0,1,0,0")},
		{"concat skew c", concat(render.Transform{A: 1, C: 0.5, D: 1, E: 7}), cvOps("matrix 1,0,0.5,1,7,0")},
		{"concat NaN", concat(render.Transform{A: 1, D: 1, E: cvNaN}), cvOps()},
		{"concat Inf", concat(render.Transform{A: cvInf, D: 1}), cvOps()},
		{"concat Inf rotation", concat(render.Transform{A: 1, B: cvInf, D: 1}), cvOps()},
		{"concat overflows float32", concat(render.Transform{A: 1, D: 1, F: cvBigF32}), cvOps()},
		{"concat tiny skew rounds to 0", concat(render.Transform{A: 1, B: 1e-50, D: 1, E: 2}), cvOps("translate 2,0")},
		{"transforms in a save", func(c *canvas) {
			c.Save()
			c.Translate(1, 2)
			c.Concat(render.Scaling(2, 2))
			c.Restore()
		}, cvOps("save", "translate 1,2", "scale 2,2", "restore")},
	})
}

// TestCanvasForeignContent: layouts and images the shell did not make draw
// nothing (DrawText's own cases are in canvastext_test.go, DrawImage's in
// image_test.go).
func TestCanvasForeignContent(t *testing.T) {
	layout := fake.NewTextSystem().NewLayout("abc", render.Font{Size: 13}, 0)
	cvRun(t, []cvCase{
		{"fake text layout", func(c *canvas) { c.DrawText(layout, render.Point{X: 5, Y: 6}, cvBlue) }, cvOps()},
		{"text nil", func(c *canvas) { c.DrawText(nil, render.Point{X: 5, Y: 6}, cvBlue) }, cvOps()},
		{"image (not the shell's: S4.2e, image_test.go)", func(c *canvas) { c.DrawImage(cvImage{}, render.Rect{X: 1, Y: 2, W: 3, H: 4}) }, cvOps()},
		{"image nil", func(c *canvas) { c.DrawImage(nil, render.Rect{X: 1, Y: 2, W: 3, H: 4}) }, cvOps()},
		{"image empty", func(c *canvas) { c.DrawImage(cvImage{}, render.Rect{X: 1, Y: 2, H: 4}) }, cvOps()},
		{"image Inf", func(c *canvas) { c.DrawImage(cvImage{}, render.Rect{W: cvInf, H: 4}) }, cvOps()},
	})
}

// TestCanvasHairline: hair is one device pixel in points, 1 for a scale
// that is not positive and finite.
func TestCanvasHairline(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
		want  float64
	}{
		{"1", 1, 1},
		{"2", 2, 0.5},
		{"1.25", 1.25, 0.8},
		{"3", 3, 1.0 / 3},
		{"0", 0, 1},
		{"negative", -2, 1},
		{"NaN", cvNaN, 1},
		{"Inf", cvInf, 1},
	} {
		sn := &fakeSnapshot{}
		c := newCanvas(sn, render.Size{W: 100, H: 50}, tc.scale, false)
		if c.hair != tc.want {
			t.Errorf("%s: hair = %v, want %v", tc.name, c.hair, tc.want)
		}
		if err := c.finish(); err != nil {
			t.Errorf("%s: finish = %v", tc.name, err)
		}
	}
}

// --- Paths (S4.2c) -------------------------------------------------------------

func TestCanvasFillPath(t *testing.T) {
	red, blue := render.Solid(cvRed), render.Solid(cvBlue)
	grad := render.LinearGradient(render.Point{}, render.Point{X: 10}, cvStop(1, cvBlue), cvStop(0, cvRed))
	gradOp := "(0,0)->(10,0) [0:" + cvRedOp + " 1:" + cvBlueOp + "]"
	tri := cvPath("M", 1.0, 2.0, "L", 11.0, 2.0, "L", 6.0, 12.0, "Z")
	open := cvPath("M", 1.0, 2.0, "L", 11.0, 2.0, "L", 6.0, 12.0)
	triOp := "[M1,2 L11,2 L6,12 Z]"
	cubic := cvPath("M", 0.0, 0.0, "C", 0.0, 10.0, 10.0, 10.0, 10.0, 0.0)
	cvRun(t, []cvCase{
		{"solid", func(c *canvas) { c.FillPath(tri, red) }, cvOps("fill " + triOp + " " + cvRedOp)},
		{"open subpath closed", func(c *canvas) { c.FillPath(open, red) }, cvOps("fill " + triOp + " " + cvRedOp)},
		{"two subpaths", func(c *canvas) {
			c.FillPath(cvPath("M", 0.0, 0.0, "L", 4.0, 0.0, "L", 4.0, 4.0, "M", 6.0, 6.0, "L", 9.0, 6.0, "L", 9.0, 9.0, "Z"), blue)
		}, cvOps("fill [M0,0 L4,0 L4,4 Z M6,6 L9,6 L9,9 Z] " + cvBlueOp)},
		{"gradient", func(c *canvas) { c.FillPath(tri, grad) },
			cvOps("push fill "+triOp, "linear (0,1,12,12) "+gradOp, "pop")},
		{"gradient over the curve's tight bounds", func(c *canvas) { c.FillPath(cubic, grad) },
			cvOps("push fill [M0,0 C0,10 10,10 10,0 Z]", "linear (-1,-1,12,9.5) "+gradOp, "pop")},
		{"gradient of one stop is solid", func(c *canvas) {
			c.FillPath(tri, render.LinearGradient(render.Point{}, render.Point{X: 10}, cvStop(0.3, cvBlue)))
		}, cvOps("fill " + triOp + " " + cvBlueOp)},
		{"gradient of zero length is its last stop", func(c *canvas) {
			c.FillPath(tri, render.LinearGradient(render.Point{X: 3}, render.Point{X: 3}, cvStop(0, cvRed), cvStop(1, cvBlue)))
		}, cvOps("fill " + triOp + " " + cvBlueOp)},
		{"in save, clip and opacity", func(c *canvas) {
			c.Save()
			c.ClipRoundedRect(cvRect, render.Radii{})
			c.PushOpacity(0.5)
			c.FillPath(tri, grad)
			c.FillPath(tri, red)
			c.Pop()
			c.Restore()
		}, cvOps("save", "clip (10,20,30,40)", "opacity 0.5", "save",
			"push fill "+triOp, "linear (0,1,12,12) "+gradOp, "pop", "fill "+triOp+" "+cvRedOp,
			"restore", "pop", "pop", "restore")},
		{"left open: finish closes the state, not the fill", func(c *canvas) {
			c.PushOpacity(0.25)
			c.FillPath(tri, grad)
		}, cvOps("opacity 0.25", "save", "push fill "+triOp, "linear (0,1,12,12) "+gradOp, "pop", "restore", "pop")},
		{"nil", func(c *canvas) { c.FillPath(nil, red) }, cvOps()},
		{"empty", func(c *canvas) { c.FillPath(new(render.Path), red) }, cvOps()},
		{"lone move", func(c *canvas) { c.FillPath(cvPath("M", 1.0, 1.0, "Z"), red) }, cvOps()},
		{"flat", func(c *canvas) { c.FillPath(cvLine(0, 5, 10, 5), red) }, cvOps()},
		{"flat gradient", func(c *canvas) { c.FillPath(cvLine(0, 5, 10, 5), grad) }, cvOps()},
		{"Inf point", func(c *canvas) { c.FillPath(cvPath("M", 0.0, 0.0, "L", 10.0, 0.0, "L", 5.0, cvInf), red) }, cvOps()},
		{"float32 overflow", func(c *canvas) { c.FillPath(cvPath("M", 0.0, 0.0, "L", 10.0, 0.0, "L", 5.0, cvBigF32), red) }, cvOps()},
		{"semantic", func(c *canvas) { c.FillPath(tri, render.Solid(cvLabel)) }, cvOps()},
		{"semantic stop", func(c *canvas) {
			c.FillPath(tri, render.LinearGradient(render.Point{}, render.Point{X: 10}, cvStop(0, cvRed), cvStop(1, cvLabel)))
		}, cvOps()},
		{"transparent", func(c *canvas) { c.FillPath(tri, render.Solid(cvClear)) }, cvOps()},
		{"transparent stops", func(c *canvas) {
			c.FillPath(tri, render.LinearGradient(render.Point{}, render.Point{X: 10}, cvStop(0, cvClear), cvStop(1, cvClear)))
		}, cvOps()},
		{"no stops", func(c *canvas) { c.FillPath(tri, render.LinearGradient(render.Point{}, render.Point{X: 10})) }, cvOps()},
	})
	// The gradient's cover grows by one device pixel: half a point at scale 2.
	t.Run("gradient at scale 2", func(t *testing.T) {
		cvWantOps(t, cvDraw(t, 2, func(c *canvas) { c.FillPath(tri, grad) }),
			cvOps("push fill "+triOp, "linear (0.5,1.5,11,11) "+gradOp, "pop"))
	})
}

func TestCanvasStrokePath(t *testing.T) {
	red := render.Solid(cvRed)
	grad := render.LinearGradient(render.Point{}, render.Point{X: 10}, cvStop(1, cvBlue), cvStop(0, cvRed))
	gradOp := "(0,0)->(10,0) [0:" + cvRedOp + " 1:" + cvBlueOp + "]"
	line := cvLine(1, 2, 11, 22)
	lineOp := "[M1,2 L11,22]"
	def := "w=2 cap=0 join=0 miter=10"
	roundJoin := render.StrokeStyle{Join: render.JoinRound}
	sq := math.Sqrt2
	cvRun(t, []cvCase{
		{"solid", func(c *canvas) { c.StrokePath(line, 2, red, render.StrokeStyle{}) },
			cvOps("stroke path " + lineOp + " " + def + " " + cvRedOp)},
		{"flat line", func(c *canvas) { c.StrokePath(cvLine(0, 5, 10, 5), 2, red, render.StrokeStyle{}) },
			cvOps("stroke path [M0,5 L10,5] " + def + " " + cvRedOp)},
		{"closed stays closed, open stays open", func(c *canvas) {
			c.StrokePath(cvPath("M", 1.0, 2.0, "L", 11.0, 2.0, "L", 6.0, 12.0, "Z", "M", 20.0, 20.0, "L", 30.0, 30.0), 1, red,
				render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinBevel, MiterLimit: 3})
		}, cvOps("stroke path [M1,2 L11,2 L6,12 Z M20,20 L30,30] w=1 cap=2 join=2 miter=3 " + cvRedOp)},
		{"style", func(c *canvas) {
			c.StrokePath(line, 3, red, render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound, Dash: []float64{4, 2}, DashOffset: 1})
		}, cvOps("stroke path " + lineOp + " w=3 cap=1 join=1 miter=10 dash=4,2@1 " + cvRedOp)},
		{"odd dashes repeat", func(c *canvas) { c.StrokePath(line, 2, red, render.StrokeStyle{Dash: []float64{3}}) },
			cvOps("stroke path " + lineOp + " " + def + " dash=3,3@0 " + cvRedOp)},
		{"the caller's style is not changed", func(c *canvas) {
			s := render.StrokeStyle{Dash: []float64{3, -1}}
			c.StrokePath(line, 2, red, s)
			if s.Dash[1] != -1 {
				t.Errorf("the caller's dashes changed: %v", s.Dash)
			}
		}, cvOps("stroke path " + lineOp + " " + def + " dash=3,0@0 " + cvRedOp)},
		{"gradient, round joins", func(c *canvas) { c.StrokePath(line, 2, grad, roundJoin) },
			cvOps("push stroke path "+lineOp+" w=2 cap=0 join=1 miter=10", "linear (-1,0,14,24) "+gradOp, "pop")},
		{"gradient, miter joins reach the limit", func(c *canvas) { c.StrokePath(line, 2, grad, render.StrokeStyle{}) },
			cvOps("push stroke path "+lineOp+" "+def, "linear (-10,-9,32,42) "+gradOp, "pop")},
		{"gradient, square caps", func(c *canvas) {
			c.StrokePath(line, 2, grad, render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinBevel})
		}, cvOps("push stroke path "+lineOp+" w=2 cap=2 join=2 miter=10",
			fmt.Sprintf("linear (%s,%s,%s,%s) %s", fnum(float32(-sq)), fnum(float32(1-sq)),
				fnum(float32(12+2*sq)), fnum(float32(22+2*sq)), gradOp), "pop")},
		{"gradient, flat line", func(c *canvas) { c.StrokePath(cvLine(0, 5, 10, 5), 2, grad, roundJoin) },
			cvOps("push stroke path [M0,5 L10,5] w=2 cap=0 join=1 miter=10", "linear (-2,3,14,4) "+gradOp, "pop")},
		{"gradient of one stop is solid", func(c *canvas) {
			c.StrokePath(line, 2, render.LinearGradient(render.Point{}, render.Point{X: 10}, cvStop(0.5, cvBlue)), render.StrokeStyle{})
		}, cvOps("stroke path " + lineOp + " " + def + " " + cvBlueOp)},
		{"in an opacity", func(c *canvas) {
			c.PushOpacity(0.5)
			c.StrokePath(line, 2, grad, roundJoin)
			c.Pop()
		}, cvOps("opacity 0.5", "save", "push stroke path "+lineOp+" w=2 cap=0 join=1 miter=10",
			"linear (-1,0,14,24) "+gradOp, "pop", "restore", "pop")},
		{"width 0", func(c *canvas) { c.StrokePath(line, 0, red, render.StrokeStyle{}) }, cvOps()},
		{"negative width", func(c *canvas) { c.StrokePath(line, -1, red, render.StrokeStyle{}) }, cvOps()},
		{"NaN width", func(c *canvas) { c.StrokePath(line, cvNaN, red, render.StrokeStyle{}) }, cvOps()},
		{"Inf width", func(c *canvas) { c.StrokePath(line, cvInf, red, render.StrokeStyle{}) }, cvOps()},
		{"float32 overflow width", func(c *canvas) { c.StrokePath(line, cvBigF32, red, render.StrokeStyle{}) }, cvOps()},
		{"float32 overflow dash", func(c *canvas) {
			c.StrokePath(line, 2, red, render.StrokeStyle{Dash: []float64{1, cvBigF32}})
		}, cvOps()},
		{"a huge reach is clamped", func(c *canvas) {
			c.StrokePath(cvLine(0, 0, 3e38, 0), 2, grad, render.StrokeStyle{MiterLimit: 1e38})
		}, cvOps("push stroke path [M0,0 L3e+38,0] w=2 cap=0 join=0 miter=1e+38", "linear (-1e+30,-1e+30,2e+30,2e+30) "+gradOp, "pop")},
		{"an overflowing miter limit is the default", func(c *canvas) {
			c.StrokePath(line, 2, grad, render.StrokeStyle{MiterLimit: cvBigF32})
		}, cvOps("push stroke path "+lineOp+" "+def, "linear (-10,-9,32,42) "+gradOp, "pop")},
		{"nil", func(c *canvas) { c.StrokePath(nil, 2, red, render.StrokeStyle{}) }, cvOps()},
		{"empty", func(c *canvas) { c.StrokePath(new(render.Path), 2, red, render.StrokeStyle{}) }, cvOps()},
		{"lone move", func(c *canvas) { c.StrokePath(cvPath("M", 1.0, 1.0), 2, red, render.StrokeStyle{Cap: render.CapRound}) }, cvOps()},
		{"NaN point", func(c *canvas) { c.StrokePath(cvLine(cvNaN, 0, 1, 1), 2, red, render.StrokeStyle{}) }, cvOps()},
		{"semantic", func(c *canvas) { c.StrokePath(line, 2, render.Solid(cvLabel), render.StrokeStyle{}) }, cvOps()},
		{"transparent", func(c *canvas) { c.StrokePath(line, 2, render.Solid(cvClear), render.StrokeStyle{}) }, cvOps()},
		{"no stops", func(c *canvas) {
			c.StrokePath(line, 2, render.LinearGradient(render.Point{}, render.Point{X: 10}), render.StrokeStyle{})
		}, cvOps()},
	})
}

// TestCanvasTextBoxes: with a recorder, textBoxes lists the device-space
// box of every drawn text (translated and scaled by the transform),
// clipped to the clip at the call, and leaves out text clipped away and
// the other ops; without a recorder it is nil.
func TestCanvasTextBoxes(t *testing.T) {
	l := fake.NewTextSystem().NewLayout("abc", render.Font{Size: 13}, 0)
	sz := l.Size()
	paint := func(c *canvas) {
		c.FillRoundedRect(render.Rect{W: 10, H: 10}, render.Radii{}, render.Solid(cvBlue))
		c.DrawText(l, render.Point{X: 5, Y: 6}, cvBlue)
		c.Save()
		c.Translate(10, 10)
		c.Scale(2, 2)
		c.DrawText(l, render.Point{}, cvBlue)
		c.Restore()
		c.Save()
		c.ClipRoundedRect(render.Rect{X: 0, Y: 0, W: 7, H: 50}, render.Radii{})
		c.DrawText(l, render.Point{X: 5, Y: 30}, cvBlue)
		c.DrawText(l, render.Point{X: 8, Y: 30}, cvBlue) // clipped away
		c.Restore()
	}
	want := []render.Rect{
		{X: 5, Y: 6, W: sz.W, H: sz.H},
		{X: 10, Y: 10, W: 2 * sz.W, H: 2 * sz.H},
		{X: 5, Y: 30, W: 2, H: sz.H},
	}
	if sz.W <= 2 || sz.H <= 0 || 10+2*sz.H > 50 || 10+2*sz.W > 100 {
		t.Fatalf("the fake layout's size %v does not fit the test", sz)
	}
	c := newCanvas(&fakeSnapshot{}, render.Size{W: 100, H: 50}, 1, true)
	paint(c)
	if err := c.finish(); err != nil {
		t.Fatal(err)
	}
	if got := c.textBoxes(); !slices.Equal(got, want) {
		t.Errorf("textBoxes = %v, want %v", got, want)
	}
	c = newCanvas(&fakeSnapshot{}, render.Size{W: 100, H: 50}, 1, false)
	paint(c)
	c.finish()
	if got := c.textBoxes(); got != nil {
		t.Errorf("textBoxes without a recorder = %v, want nil", got)
	}
}

// rotation turns by angle radians around the origin, clockwise on screen
// (render has no Rotation; nothing but the tests rotates).
func rotation(angle float64) render.Transform {
	s, c := math.Sincos(angle)
	return render.Transform{A: c, B: s, C: -s, D: c}
}
