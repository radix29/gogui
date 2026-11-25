package record

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

func rc(x, y, w, h float64) render.Rect { return render.Rect{X: x, Y: y, W: w, H: h} }
func pt(x, y float64) render.Point      { return render.Point{X: x, Y: y} }

var (
	blue  = driver.RGB8(0x00, 0x7a, 0xff)
	white = driver.RGB8(0xff, 0xff, 0xff)
	shade = driver.RGBA(0, 0, 0, 0.3)
)

// layout is a fixed-size render.TextLayout.
type layout struct {
	text string
	font render.Font
	size render.Size
}

func (l layout) Text() string                      { return l.text }
func (l layout) Font() render.Font                 { return l.font }
func (l layout) Size() render.Size                 { return l.size }
func (l layout) Baseline() float64                 { return 12 }
func (l layout) LineCount() int                    { return 1 }
func (l layout) HitTest(render.Point) (int, bool)  { return 0, false }
func (l layout) CaretRect(int) render.Rect         { return render.Rect{} }
func (l layout) CaretRectUpstream(int) render.Rect { return render.Rect{} }
func (l layout) Line(int) render.LineMetrics {
	return render.LineMetrics{End: len(l.text), Height: l.size.H, Baseline: 12}
}
func (l layout) SelectionRects(int, int) []render.Rect {
	return nil
}

type image struct{ w, h float64 }

func (i image) Size() render.Size { return render.Size{W: i.w, H: i.h} }

func newCanvas() *Canvas { return New(render.Size{W: 200, H: 100}) }

func mustEnd(t *testing.T, c *Canvas) {
	t.Helper()
	if err := c.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
}

func checkDump(t *testing.T, c *Canvas, want ...string) {
	t.Helper()
	if got, w := c.Dump(), strings.Join(want, "\n"); got != w {
		t.Errorf("dump:\n%s\nwant:\n%s", got, w)
	}
}

// wantErrs checks that End reports exactly the given substrings, in order.
func wantErrs(t *testing.T, c *Canvas, subs ...string) {
	t.Helper()
	err := c.End()
	errs := c.Errors()
	if len(subs) == 0 {
		if err != nil {
			t.Fatalf("End: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("End: nil, want %d errors", len(subs))
	}
	if len(errs) != len(subs) {
		t.Fatalf("got %d errors, want %d:\n%v", len(errs), len(subs), err)
	}
	for i, s := range subs {
		if !strings.Contains(errs[i].Error(), s) {
			t.Errorf("error %d = %q, want it to contain %q", i, errs[i], s)
		}
		if !errors.Is(err, errs[i]) {
			t.Errorf("joined error does not wrap error %d", i)
		}
	}
}

func TestEmpty(t *testing.T) {
	c := newCanvas()
	if c.Dump() != "" || len(c.Ops()) != 0 || c.Depth() != 0 {
		t.Fatalf("new canvas: dump %q, %d ops, depth %d", c.Dump(), len(c.Ops()), c.Depth())
	}
	mustEnd(t, c)
	if err := c.End(); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("second End = %v", err)
	}
}

func TestDumpEveryOp(t *testing.T) {
	c := newCanvas()
	c.FillRoundedRect(rc(0, 0, 80, 22), render.UniformRadii(5), render.Solid(blue))
	c.StrokeRoundedRect(rc(0.5, 0.5, 79, 21), render.UniformRadii(4.5), 1, render.Solid(driver.RGBA(0, 0, 0, 0.15)))
	c.FillRoundedRect(rc(0, 30, 80, 22), render.Radii{TopLeft: 6, TopRight: 6}, render.LinearGradient(pt(0, 30), pt(0, 52),
		render.GradientStop{Offset: 1, Color: blue}, render.GradientStop{Offset: 0, Color: white}))
	var tri render.Path
	tri.MoveTo(pt(0, 0)).LineTo(pt(10, 0)).LineTo(pt(5, 8)).Close()
	c.FillPath(&tri, render.Solid(white))
	var check render.Path
	check.MoveTo(pt(2, 6)).LineTo(pt(5, 9)).LineTo(pt(10, 3))
	c.StrokePath(&check, 2, render.Solid(white), render.StrokeStyle{Cap: render.CapRound, Join: render.JoinRound})
	c.StrokePath(&check, 1, render.Solid(white), render.StrokeStyle{Cap: render.CapSquare, Join: render.JoinBevel, MiterLimit: 4, Dash: []float64{2}, DashOffset: 1})
	var curve render.Path
	curve.QuadTo(pt(0, 0), pt(4, 4)).CubicTo(pt(5, 5), pt(6, 6), pt(7, 7))
	c.FillPath(&curve, render.Solid(white))
	c.FillPath(nil, render.Solid(white))
	c.DrawShadow(rc(0, 0, 80, 22), render.UniformRadii(5), render.Shadow{Offset: pt(0, 0.5), Blur: 1, Color: shade})
	c.DrawShadow(rc(0, 0, 80, 22), render.Radii{}, render.Shadow{Blur: -3, Spread: -1, Color: shade, Inset: true})
	c.DrawText(layout{text: "Greet", size: render.Size{W: 35, H: 16}}, pt(10, 3), driver.RGBA(0, 0, 0, 0.85))
	c.DrawText(layout{text: `say "hi"`, font: render.Font{Family: "Inter", Size: 11, Weight: driver.FontWeightSemibold, Italic: true}, size: render.Size{W: 1, H: 2}}, pt(0, 0), white)
	c.DrawText(layout{text: "x", font: render.Font{Monospaced: true}}, pt(0, 0), white)
	c.DrawImage(image{16, 16}, rc(4, 4, 16, 16))
	c.PushOpacity(0.5)
	c.Pop()
	c.PushBackdropBlur(rc(0, 0, 200, 300), render.UniformRadii(10), 30)
	c.Pop()
	mustEnd(t, c)
	checkDump(t, c,
		"fillRRect (0,0,80,22) r=5 solid #007aff",
		"strokeRRect (0.5,0.5,79,21) r=4.5 w=1 solid #00000026",
		"fillRRect (0,30,80,22) r=6/6/0/0 linear (0,30)->(0,52) [0:#ffffff 1:#007aff]",
		"fillPath [M(0,0) L(10,0) L(5,8) Z] solid #ffffff",
		"strokePath [M(2,6) L(5,9) L(10,3)] w=2 cap=round join=round solid #ffffff",
		"strokePath [M(2,6) L(5,9) L(10,3)] w=1 cap=square join=bevel miter=4 dash=[2 2]@1 solid #ffffff",
		"fillPath [M(0,0) Q(0,0) (4,4) C(5,5) (6,6) (7,7)] solid #ffffff",
		"fillPath [] solid #ffffff",
		"shadow (0,0,80,22) r=5 off=(0,0.5) blur=1 #0000004d",
		"shadow (0,0,80,22) off=(0,0) blur=0 spread=-1 #0000004d inset",
		`text (10,3,35,16) "Greet" 13pt #000000d9`,
		`text (0,0,1,2) "say \"hi\"" 11pt "Inter" w600 italic #ffffff`,
		`text (0,0,0,0) "x" 13pt mono #ffffff`,
		"image (4,4,16,16) 16x16",
		"pushOpacity 0.5",
		"pop",
		"pushBlur (0,0,200,300) r=10 blur=30",
		"pop",
	)
}

func TestNormalization(t *testing.T) {
	c := newCanvas()
	c.FillRoundedRect(rc(0, 0, 20, 10), render.UniformRadii(50), render.Solid(blue))
	c.StrokeRoundedRect(rc(0, 0, 20, 10), render.Radii{TopLeft: -1}, math.NaN(), render.Solid(blue))
	c.PushOpacity(7)
	c.Pop()
	c.PushOpacity(math.NaN())
	c.Pop()
	c.PushBackdropBlur(rc(0, 0, 10, 10), render.Radii{}, math.Inf(1))
	c.Pop()
	mustEnd(t, c)
	checkDump(t, c,
		"fillRRect (0,0,20,10) r=5 solid #007aff",
		"strokeRRect (0,0,20,10) w=0 solid #007aff",
		"pushOpacity 1",
		"pop",
		"pushOpacity 0",
		"pop",
		"pushBlur (0,0,10,10) blur=0",
		"pop",
	)
}

func TestTransformTracking(t *testing.T) {
	c := newCanvas()
	c.Translate(10, 20)
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	c.Save()
	c.Translate(1, 2)
	c.Scale(2, 3)
	c.FillRoundedRect(rc(1, 1, 5, 5), render.Radii{}, render.Solid(blue))
	c.Concat(render.Translation(1, 0))
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(blue))
	c.Restore()
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	mustEnd(t, c)
	checkDump(t, c,
		"fillRRect (10,20,5,5) solid #007aff",
		"fillRRect (1,1,5,5) solid #007aff xf=(2,0,0,3,11,22)",
		"fillRRect (0,0,1,1) solid #007aff xf=(2,0,0,3,13,22)",
		"fillRRect (10,20,5,5) solid #007aff",
	)
	ops := c.Ops()
	if b := ops[1].Bounds(); b != rc(13, 25, 10, 15) {
		t.Errorf("scaled bounds = %v", b)
	}
	want := render.Identity().Pre(render.Translation(10, 20)).Pre(render.Translation(1, 2)).Pre(render.Scaling(2, 3))
	if ops[1].Transform != want {
		t.Errorf("transform = %v, want %v", ops[1].Transform, want)
	}
}

func TestTranslationFoldedIntoPathsAndGradients(t *testing.T) {
	c := newCanvas()
	c.Translate(5, 5)
	var p render.Path
	p.MoveTo(pt(0, 0)).LineTo(pt(1, 1))
	c.StrokePath(&p, 1, render.LinearGradient(pt(0, 0), pt(0, 10), render.GradientStop{Color: blue}), render.StrokeStyle{})
	mustEnd(t, c)
	checkDump(t, c, "strokePath [M(5,5) L(6,6)] w=1 linear (5,5)->(5,15) [0:#007aff]")
	if b := c.Ops()[0].Bounds(); b != rc(5, 5, 1, 1) {
		t.Errorf("path bounds = %v", b)
	}
}

func TestCurvePathBounds(t *testing.T) {
	c := newCanvas()
	c.Translate(10, 0)
	var p render.Path
	p.MoveTo(pt(0, 0)).QuadTo(pt(5, 10), pt(10, 0)).CubicTo(pt(10, -8), pt(0, -8), pt(0, 0)).Close()
	c.FillPath(&p, render.Solid(blue))
	mustEnd(t, c)
	want := c.Ops()[0].Transform.ApplyRect(p.Bounds())
	if got := c.Ops()[0].Bounds(); got != want {
		t.Errorf("bounds = %v, want %v", got, want)
	}
	if want.Y != -6 || want.H != 11 {
		t.Errorf("curve bounds not tight: %v", want)
	}
}

func TestRotationPrintsTransform(t *testing.T) {
	c := newCanvas()
	c.Translate(50, 50)
	c.Concat(rotation(math.Pi / 2))
	c.FillRoundedRect(rc(0, 0, 10, 4), render.Radii{}, render.Solid(blue))
	c.ClipRoundedRect(rc(0, 0, 10, 4), render.UniformRadii(2))
	c.DrawImage(image{1, 1}, rc(0, 0, 1, 1))
	mustEnd(t, c)
	checkDump(t, c,
		"fillRRect (0,0,10,4) solid #007aff xf=(0,1,-1,0,50,50)",
		"image (0,0,1,1) 1x1 xf=(0,1,-1,0,50,50) clip=(46,50,4,10)",
	)
	b := c.Ops()[0].Bounds()
	if math.Abs(b.X-46) > 1e-9 || math.Abs(b.Y-50) > 1e-9 || math.Abs(b.W-4) > 1e-9 || math.Abs(b.H-10) > 1e-9 {
		t.Errorf("rotated bounds = %v", b)
	}
}

func TestClipTracking(t *testing.T) {
	c := newCanvas()
	c.Save()
	c.Translate(10, 10)
	c.ClipRoundedRect(rc(0, 0, 50, 50), render.UniformRadii(4))
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	c.Save()
	c.ClipRoundedRect(rc(40, 40, 100, 100), render.Radii{})
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	c.ClipRoundedRect(rc(200, 200, 1, 1), render.Radii{}) // disjoint: empty clip
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	c.Restore()
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	c.Restore()
	c.FillRoundedRect(rc(0, 0, 5, 5), render.Radii{}, render.Solid(blue))
	mustEnd(t, c)
	checkDump(t, c,
		"fillRRect (10,10,5,5) solid #007aff clip=(10,10,50,50)",
		"fillRRect (10,10,5,5) solid #007aff clip=(50,50,10,10)",
		"fillRRect (10,10,5,5) solid #007aff clip=(0,0,0,0)",
		"fillRRect (10,10,5,5) solid #007aff clip=(10,10,50,50)",
		"fillRRect (0,0,5,5) solid #007aff",
	)
	if got := c.Ops()[0].String(); got != "fillRRect (10,10,5,5) solid #007aff" {
		t.Errorf("Op.String = %q", got)
	}
}

func TestLayersNest(t *testing.T) {
	c := newCanvas()
	c.PushOpacity(0.5)
	c.Translate(10, 0)
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(blue))
	c.Save()
	c.PushOpacity(0.5)
	c.PushBackdropBlur(rc(0, 0, 20, 20), render.Radii{}, 8)
	if c.Depth() != 4 {
		t.Errorf("depth = %d, want 4", c.Depth())
	}
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(blue))
	c.Pop()
	c.Pop()
	c.Restore()
	c.Pop()
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(blue))
	mustEnd(t, c)
	checkDump(t, c,
		"pushOpacity 0.5",
		"fillRRect (10,0,1,1) solid #007aff alpha=0.5",
		"pushOpacity 0.5 alpha=0.5",
		"pushBlur (10,0,20,20) blur=8 alpha=0.25",
		"fillRRect (10,0,1,1) solid #007aff clip=(10,0,20,20) alpha=0.25",
		"pop alpha=0.25",
		"pop alpha=0.5",
		"pop",
		"fillRRect (0,0,1,1) solid #007aff",
	)
}

func TestBalanceErrors(t *testing.T) {
	t.Run("restore without save", func(t *testing.T) {
		c := newCanvas()
		c.Restore()
		wantErrs(t, c, "call 1 (Restore): nothing open")
	})
	t.Run("pop without push", func(t *testing.T) {
		c := newCanvas()
		c.Pop()
		wantErrs(t, c, "call 1 (Pop): nothing open")
		if c.Dump() != "" {
			t.Errorf("an ignored Pop was recorded: %q", c.Dump())
		}
	})
	t.Run("pop closing a save", func(t *testing.T) {
		c := newCanvas()
		c.Save()
		c.Pop()
		c.Restore()
		wantErrs(t, c, "call 2 (Pop): innermost open call is Save of call 1")
	})
	t.Run("restore closing a layer", func(t *testing.T) {
		c := newCanvas()
		c.PushOpacity(0.5)
		c.Restore()
		c.Pop()
		wantErrs(t, c, "call 2 (Restore): innermost open call is PushOpacity of call 1")
		checkDump(t, c, "pushOpacity 0.5", "pop")
	})
	t.Run("crossed pairs", func(t *testing.T) {
		c := newCanvas()
		c.Save()
		c.PushBackdropBlur(rc(0, 0, 1, 1), render.Radii{}, 1)
		c.Restore() // closes nothing: the blur is innermost
		c.Pop()
		c.Restore()
		wantErrs(t, c, "call 3 (Restore): innermost open call is PushBackdropBlur of call 2")
	})
	t.Run("open at End", func(t *testing.T) {
		c := newCanvas()
		c.Save()
		c.PushOpacity(1)
		c.PushBackdropBlur(rc(0, 0, 1, 1), render.Radii{}, 1)
		wantErrs(t, c,
			"call 4 (End): PushBackdropBlur of call 3 not closed",
			"call 4 (End): PushOpacity of call 2 not closed",
			"call 4 (End): Save of call 1 not closed")
	})
	t.Run("after End", func(t *testing.T) {
		c := newCanvas()
		mustEnd(t, c)
		c.Save()
		c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(blue))
		if errs := c.Errors(); len(errs) != 2 || !strings.Contains(errs[0].Error(), "call 2 (Save): call after End") ||
			!strings.Contains(errs[1].Error(), "call 3 (FillRoundedRect): call after End") {
			t.Errorf("errors = %v", errs)
		}
		if c.Dump() != "" || c.Depth() != 0 {
			t.Errorf("calls after End changed the canvas: %q depth %d", c.Dump(), c.Depth())
		}
		// Every call is refused after End.
		calls := []func(){
			c.Restore, c.Pop,
			func() { c.Translate(1, 1) }, func() { c.Scale(2, 2) }, func() { c.Concat(render.Identity()) },
			func() { c.ClipRoundedRect(rc(0, 0, 1, 1), render.Radii{}) },
			func() { c.StrokeRoundedRect(rc(0, 0, 1, 1), render.Radii{}, 1, render.Solid(blue)) },
			func() { c.FillPath(nil, render.Solid(blue)) },
			func() { c.StrokePath(nil, 1, render.Solid(blue), render.StrokeStyle{}) },
			func() { c.DrawShadow(rc(0, 0, 1, 1), render.Radii{}, render.Shadow{}) },
			func() { c.DrawText(layout{}, pt(0, 0), blue) },
			func() { c.DrawImage(image{}, rc(0, 0, 1, 1)) },
			func() { c.PushOpacity(1) },
			func() { c.PushBackdropBlur(rc(0, 0, 1, 1), render.Radii{}, 1) },
		}
		for _, f := range calls {
			f()
		}
		if n := len(c.Errors()); n != 2+len(calls) {
			t.Errorf("%d errors, want %d", n, 2+len(calls))
		}
		if c.Dump() != "" || c.Depth() != 0 {
			t.Errorf("calls after End changed the canvas: %q depth %d", c.Dump(), c.Depth())
		}
	})
}

func TestRejectedCalls(t *testing.T) {
	sem := driver.Semantic(driver.ColorControlAccent)
	c := newCanvas()
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(sem))
	c.StrokeRoundedRect(rc(0, 0, 1, 1), render.Radii{}, 1, render.LinearGradient(pt(0, 0), pt(1, 1), render.GradientStop{Color: blue}, render.GradientStop{Offset: 1, Color: sem}))
	c.FillPath(nil, render.Solid(sem))
	c.StrokePath(nil, 1, render.Solid(sem), render.StrokeStyle{})
	c.DrawShadow(rc(0, 0, 1, 1), render.Radii{}, render.Shadow{Color: sem})
	c.DrawText(layout{text: "x"}, pt(0, 0), sem)
	c.DrawText(nil, pt(0, 0), blue)
	c.DrawImage(nil, rc(0, 0, 1, 1))
	c.Translate(math.NaN(), 0)
	c.Scale(1, math.Inf(1))
	c.Concat(render.Transform{A: math.NaN()})
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, render.Solid(blue))
	semantic := "semantic color reached the canvas"
	wantErrs(t, c,
		"call 1 (FillRoundedRect): "+semantic,
		"call 2 (StrokeRoundedRect): "+semantic,
		"call 3 (FillPath): "+semantic,
		"call 4 (StrokePath): "+semantic,
		"call 5 (DrawShadow): "+semantic,
		"call 6 (DrawText): "+semantic,
		"call 7 (DrawText): nil layout",
		"call 8 (DrawImage): nil image",
		"call 9 (Translate): non-finite argument",
		"call 10 (Scale): non-finite argument",
		"call 11 (Concat): non-finite transform",
	)
	// Only the last call was recorded, with the transform untouched.
	checkDump(t, c, "fillRRect (0,0,1,1) solid #007aff")
}

func TestRecordedValuesAreCopies(t *testing.T) {
	c := newCanvas()
	var p render.Path
	p.MoveTo(pt(0, 0)).LineTo(pt(1, 0))
	stops := []render.GradientStop{{Offset: 0, Color: blue}}
	paint := render.Paint{Kind: render.PaintLinear, End: pt(1, 0), Stops: stops}
	dash := []float64{1, 1}
	c.StrokePath(&p, 1, paint, render.StrokeStyle{Dash: dash})
	before := c.Dump()
	p.Reset()
	p.MoveTo(pt(9, 9))
	stops[0].Color = white
	dash[0] = 5
	if after := c.Dump(); after != before {
		t.Errorf("dump changed after mutating the inputs:\n%s\n%s", before, after)
	}
	ops := c.Ops()
	ops[0].Kind = Pop
	if c.Ops()[0].Kind != StrokePath {
		t.Error("Ops does not return a copy")
	}
}

func TestDumpStability(t *testing.T) {
	draw := func(c *Canvas, jitter float64) {
		c.Translate(0.1+0.2+jitter, 0) // 0.30000000000000004
		c.FillRoundedRect(rc(math.Copysign(0, -1), 1.0/3, 10.0/3, 10), render.UniformRadii(1.00049), render.Solid(driver.RGBA(0.5, 0.25, 1.2, -1)))
		c.StrokeRoundedRect(rc(0, 0, 1, 1), render.Radii{}, 1e-12, render.Solid(blue))
	}
	want := []string{
		"fillRRect (0.3,0.333,3.333,10) r=1 solid #8040ff00",
		"strokeRRect (0.3,0,1,1) w=0 solid #007aff",
	}
	for _, jitter := range []float64{0, 1e-9, -1e-9} {
		c := newCanvas()
		draw(c, jitter)
		mustEnd(t, c)
		checkDump(t, c, want...)
		if got := strings.Join(c.Lines(), "\n"); got != c.Dump() {
			t.Error("Lines and Dump disagree")
		}
	}
	// Repeated dumps of one canvas are identical.
	c := newCanvas()
	draw(c, 0)
	if first, second := c.Dump(), c.Dump(); first != second {
		t.Error("dump not deterministic")
	}
}

func TestNumberFormat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0"}, {math.Copysign(0, -1), "0"}, {-0.0004, "0"}, {1.5, "1.5"}, {-2.25, "-2.25"},
		{1.0005, "1.001"}, {1e6, "1000000"}, {math.NaN(), "NaN"}, {math.Inf(-1), "-Inf"},
	} {
		if got := num(tc.in); got != tc.want {
			t.Errorf("num(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestKindString(t *testing.T) {
	if Kind(200).String() != "unknown" {
		t.Error("unknown kind")
	}
	for k := FillRoundedRect; k <= Pop; k++ {
		if k.String() == "unknown" {
			t.Errorf("kind %d has no name", k)
		}
	}
}

// TestInvalidGeometry is B8: geometry the backends drop (not finite, a
// negative size) is reported and not recorded, so a NaN from layout fails
// the recorder tests instead of passing them and drawing nothing on screen.
func TestInvalidGeometry(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	c := newCanvas()
	solid := render.Solid(blue)
	var bad render.Path
	bad.MoveTo(pt(0, 0)).LineTo(pt(nan, 1))
	c.FillRoundedRect(rc(nan, 0, 1, 1), render.Radii{}, solid)
	c.StrokeRoundedRect(rc(0, 0, -1, 1), render.Radii{}, 1, solid)
	c.FillPath(&bad, solid)
	c.StrokePath(&bad, 1, solid, render.StrokeStyle{})
	c.DrawShadow(rc(0, 0, inf, 1), render.Radii{}, render.Shadow{Color: blue})
	c.DrawText(layout{text: "x"}, pt(0, inf), blue)
	c.DrawImage(image{1, 1}, rc(0, 0, 1, nan))
	c.Save()
	c.ClipRoundedRect(rc(0, 0, -5, 5), render.Radii{})
	c.FillRoundedRect(rc(0, 0, 1, 1), render.Radii{}, solid) // clipped away
	c.Restore()
	c.PushBackdropBlur(rc(nan, 0, 5, 5), render.Radii{}, 4)
	c.Pop()
	c.StrokeRoundedRect(rc(0, 0, 1, 1), render.Radii{}, inf, solid) // width 0
	invalid := "invalid rect"
	wantErrs(t, c,
		"call 1 (FillRoundedRect): "+invalid,
		"call 2 (StrokeRoundedRect): "+invalid,
		"call 3 (FillPath): path with a non-finite point",
		"call 4 (StrokePath): path with a non-finite point",
		"call 5 (DrawShadow): "+invalid,
		"call 6 (DrawText): non-finite origin",
		"call 7 (DrawImage): "+invalid,
		"call 9 (ClipRoundedRect): "+invalid,
		"call 12 (PushBackdropBlur): "+invalid,
	)
	ops := c.Ops()
	if len(ops) != 4 {
		t.Fatalf("ops %v", c.Lines())
	}
	if b := ops[0].Clip; b.W != 0 || b.H != 0 {
		t.Errorf("fill after an invalid clip: clip %v, want empty", b)
	}
	if ops[1].Kind != PushBackdropBlur || ops[1].Rect != (render.Rect{}) || ops[2].Kind != Pop {
		t.Errorf("blur layer %v", c.Lines())
	}
	if ops[3].Width != 0 {
		t.Errorf("+Inf stroke width recorded as %v, want 0", ops[3].Width)
	}
}

// TestRecordKeepsItsOwnCopies (review P16): render.Paint.Normalized and
// StrokeStyle.Normalized now return the caller's stops and dashes when they
// are already normal, and a caller reuses its path; a recording still
// keeps what was drawn. The arenas hand each op a capped slice, so a later
// op never writes into an earlier one.
func TestRecordKeepsItsOwnCopies(t *testing.T) {
	c := New(render.Size{W: 100, H: 100})
	stops := []render.GradientStop{{Offset: 0, Color: driver.RGBA(1, 0, 0, 1)}, {Offset: 1, Color: driver.RGBA(0, 0, 1, 1)}}
	grad := render.Paint{Kind: render.PaintLinear, End: render.Point{X: 0, Y: 10}, Stops: stops}
	dash := []float64{2, 1}
	var p render.Path
	p.MoveTo(render.Point{X: 1, Y: 1}).LineTo(render.Point{X: 9, Y: 1})
	c.StrokePath(&p, 1, grad, render.StrokeStyle{Dash: dash})
	stops[0].Offset, dash[0] = 0.5, 7
	p.Reset()
	p.MoveTo(render.Point{X: 50, Y: 50}).LineTo(render.Point{X: 60, Y: 60}).LineTo(render.Point{X: 70, Y: 50})
	c.FillPath(&p, grad)
	if err := c.End(); err != nil {
		t.Fatal(err)
	}
	ops := c.Ops()
	first := ops[0]
	if first.Paint.Stops[0].Offset != 0 || first.Stroke.Dash[0] != 2 || len(first.Path) != 2 || first.Path[1].Pts[0] != (render.Point{X: 9, Y: 1}) {
		t.Errorf("first op changed by later calls: %s", first)
	}
	if got := first.Bounds(); got != (render.Rect{X: 1, Y: 1, W: 8}) {
		t.Errorf("first op bounds %v", got)
	}
	if got := ops[1].Bounds(); got != (render.Rect{X: 50, Y: 50, W: 20, H: 10}) {
		t.Errorf("second op bounds %v", got)
	}
}

// rotation turns by angle radians around the origin, clockwise on screen
// (render has no Rotation; nothing but the tests rotates).
func rotation(angle float64) render.Transform {
	s, c := math.Sincos(angle)
	return render.Transform{A: c, B: s, C: -s, D: c}
}
