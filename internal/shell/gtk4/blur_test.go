package gtk4

import (
	"testing"

	"github.com/radix29/gogui/internal/render"
)

// The backdrop blur's host tests (S4.2f): the canvas over the fake
// snapshotter, whose "cut #n" is a segment ended into node n and appended
// to the paint's snapshot, "node #n" the same node drawn again, and whose
// balanced check (cvDraw) also fails when the last segment is not ended,
// when a segment is cut or ended with a push open, or when a node is not
// released exactly once.

// cvBlurFill fills the whole 100×50 test surface red: something to blur.
func cvBlurFill(c *canvas) {
	c.FillRoundedRect(render.Rect{W: 100, H: 50}, render.Radii{}, render.Solid(cvRed))
}

var cvBlurFillOp = "color (0,0,100,50) " + cvRedOp

func TestCanvasBlur(t *testing.T) {
	shape := render.Rect{X: 30, Y: 10, W: 20, H: 20} // 3σ = 6 from every edge at radius 4
	cvRun(t, []cvCase{
		{"the backdrop appended twice, the blur under the clip", func(c *canvas) {
			cvBlurFill(c)
			c.PushBackdropBlur(shape, render.UniformRadii(4), 4)
			c.FillRoundedRect(render.Rect{W: 100, H: 50}, render.Radii{}, render.Solid(cvBlue))
			c.Pop()
			c.FillRoundedRect(render.Rect{W: 10, H: 10}, render.Radii{}, render.Solid(cvGreen))
		}, []string{
			cvBaseOp, cvBlurFillOp, "pop", // the segment, closed
			"cut #1", // ended and appended to the paint's snapshot
			cvBaseOp, // reopened
			"save", "clip (30,10,20,20) r=4",
			"save", "blur 4", "clip (24,4,32,32)", "node #1", "pop", "pop", "restore", // the node again, blurred
			"color (0,0,100,50) " + cvBlueOp, // clipped to the shape
			"pop", "restore",                 // Pop
			"color (0,0,10,10) rgba(0,1,0,1)",
			"pop",
		}},
		{"the reopened state is the one before", func(c *canvas) {
			c.Translate(20, 10)
			c.Save()
			c.ClipRoundedRect(cvRect, render.Radii{})
			c.PushOpacity(0.5)
			c.Scale(2, 2)
			c.FillRoundedRect(render.Rect{W: 10, H: 10}, render.Radii{}, render.Solid(cvRed))
			c.PushBackdropBlur(render.Rect{W: 10, H: 10}, render.Radii{}, 2)
			c.Pop()
			c.Pop()
			c.Restore()
		}, []string{
			cvBaseOp, "translate 20,10", "save", "clip (10,20,30,40)", "opacity 0.5", "save", "scale 2,2",
			"color (0,0,10,10) " + cvRedOp,
			"restore", "pop", "pop", "restore", "pop", // closed, innermost first
			"cut #1",
			cvBaseOp, "translate 20,10", "save", "clip (10,20,30,40)", "opacity 0.5", "save", "scale 2,2", // the same again
			"save", "clip (0,0,10,10)",
			// Back to the root (x → 20 + 2x inverted), the radius scaled by
			// √|det| = 2, the cover (20,10,20,20) padded by 3σ = 6.
			"save", "translate -10,-5", "scale 0.5,0.5", "blur 4", "clip (14,4,32,32)", "node #1", "pop", "pop", "restore",
			"pop", "restore", // the blur
			"restore", "pop", // the opacity
			"pop", "restore", // the save
			"pop",
		}},
		{"nested blurs blur every segment", func(c *canvas) {
			cvBlurFill(c)
			c.PushBackdropBlur(render.Rect{X: 10, Y: 10, W: 30, H: 30}, render.Radii{}, 2)
			c.FillRoundedRect(render.Rect{X: 10, Y: 10, W: 30, H: 30}, render.Radii{}, render.Solid(cvBlue))
			c.PushBackdropBlur(render.Rect{X: 20, Y: 20, W: 10, H: 10}, render.Radii{}, 2)
			c.Pop()
			c.Pop()
		}, []string{
			cvBaseOp, cvBlurFillOp, "pop", "cut #1",
			cvBaseOp, "save", "clip (10,10,30,30)",
			"save", "blur 2", "clip (7,7,36,36)", "node #1", "pop", "pop", "restore",
			"color (10,10,30,30) " + cvBlueOp,
			"pop", "restore", "pop", "cut #2", // the outer blur's frame closed too
			cvBaseOp, "save", "clip (10,10,30,30)", // and reopened
			"save", "clip (20,20,10,10)",
			"save", "blur 2", "clip (17,17,16,16)", "node #1", "node #2", "pop", "pop", "restore",
			"pop", "restore",
			"pop", "restore",
			"pop",
		}},
		{"a blur inside PushOpacity", func(c *canvas) {
			c.PushOpacity(0.5)
			cvBlurFill(c)
			c.PushBackdropBlur(shape, render.Radii{}, 4)
			c.FillRoundedRect(shape, render.Radii{}, render.Solid(cvBlue))
			c.Pop()
			c.Pop()
		}, []string{
			cvBaseOp, "opacity 0.5", "save", cvBlurFillOp,
			"restore", "pop", "pop", "cut #1", // the group so far composed on its own
			cvBaseOp, "opacity 0.5", "save",
			"save", "clip (30,10,20,20)",
			"save", "blur 4", "clip (24,4,32,32)", "node #1", "pop", "pop", "restore",
			"color (30,10,20,20) " + cvBlueOp,
			"pop", "restore",
			"restore", "pop",
			"pop",
		}},
		{"PushOpacity inside a blur", func(c *canvas) {
			cvBlurFill(c)
			c.PushBackdropBlur(shape, render.Radii{}, 4)
			c.PushOpacity(0.5)
			c.FillRoundedRect(shape, render.Radii{}, render.Solid(cvBlue))
			c.Pop()
			c.Pop()
		}, []string{
			cvBaseOp, cvBlurFillOp, "pop", "cut #1", cvBaseOp,
			"save", "clip (30,10,20,20)",
			"save", "blur 4", "clip (24,4,32,32)", "node #1", "pop", "pop", "restore",
			"opacity 0.5", "save", "color (30,10,20,20) " + cvBlueOp, "restore", "pop",
			"pop", "restore",
			"pop",
		}},
		{"a rotation is inverted with a matrix", func(c *canvas) {
			c.Concat(render.Transform{B: 1, C: -1, E: 60, F: 10}) // x' = 60 - y, y' = 10 + x
			c.FillRoundedRect(render.Rect{W: 20, H: 10}, render.Radii{}, render.Solid(cvRed))
			c.PushBackdropBlur(render.Rect{W: 20, H: 10}, render.Radii{}, 2)
			c.Pop()
		}, []string{
			cvBaseOp, "matrix 0,1,-1,0,60,10", "color (0,0,20,10) " + cvRedOp, "pop", "cut #1",
			cvBaseOp, "matrix 0,1,-1,0,60,10",
			"save", "clip (0,0,20,10)",
			"save", "matrix 0,-1,1,0,-10,60", "blur 2", "clip (47,7,16,26)", "node #1", "pop", "pop", "restore",
			"pop", "restore",
			"pop",
		}},
		{"a transform ended by Restore is not inverted", func(c *canvas) {
			c.Save()
			c.Scale(2, 2)
			c.Restore()
			cvBlurFill(c)
			c.PushBackdropBlur(shape, render.Radii{}, 4)
			c.Pop()
		}, []string{
			cvBaseOp, "save", "scale 2,2", "restore", cvBlurFillOp, "pop", "cut #1", cvBaseOp,
			"save", "clip (30,10,20,20)",
			"save", "blur 4", "clip (24,4,32,32)", "node #1", "pop", "pop", "restore",
			"pop", "restore",
			"pop",
		}},
		{"nothing drawn before: nothing to blur", func(c *canvas) {
			c.PushBackdropBlur(shape, render.Radii{}, 4)
			c.Pop()
		}, []string{cvBaseOp, "pop", "cut (empty)", cvBaseOp, "save", "clip (30,10,20,20)", "pop", "restore", "pop"}},
		{"a blur after a blur's Pop blurs both segments", func(c *canvas) {
			cvBlurFill(c)
			c.PushBackdropBlur(shape, render.Radii{}, 4)
			c.Pop()
			c.PushBackdropBlur(shape, render.Radii{}, 4)
			c.Pop()
		}, []string{
			cvBaseOp, cvBlurFillOp, "pop", "cut #1", cvBaseOp,
			"save", "clip (30,10,20,20)",
			"save", "blur 4", "clip (24,4,32,32)", "node #1", "pop", "pop", "restore",
			"pop", "restore",
			"pop", "cut #2", cvBaseOp,
			"save", "clip (30,10,20,20)",
			"save", "blur 4", "clip (24,4,32,32)", "node #1", "node #2", "pop", "pop", "restore",
			"pop", "restore",
			"pop",
		}},
	})
}

// TestCanvasBlurEdges: where the cover leaves the surface, the nodes are
// drawn again mirrored at that edge, and at the corner between two.
func TestCanvasBlurEdges(t *testing.T) {
	blurred := func(shape string, cover string, mirrors ...string) []string {
		ops := []string{cvBaseOp, cvBlurFillOp, "pop", "cut #1", cvBaseOp, "save", "clip " + shape,
			"save", "blur 4", "clip " + cover, "node #1"}
		for _, m := range mirrors {
			ops = append(ops, "save")
			ops = append(ops, splitOps(m)...)
			ops = append(ops, "node #1", "restore")
		}
		return append(ops, "pop", "pop", "restore", "pop", "restore", "pop")
	}
	blur := func(r render.Rect) func(c *canvas) {
		return func(c *canvas) {
			cvBlurFill(c)
			c.PushBackdropBlur(r, render.Radii{}, 4)
			c.Pop()
		}
	}
	cvRun(t, []cvCase{
		{"top left", blur(render.Rect{W: 20, H: 20}),
			blurred("(0,0,20,20)", "(-6,-6,32,32)", "scale -1,1", "scale 1,-1", "scale -1,-1")},
		{"bottom right", blur(render.Rect{X: 80, Y: 30, W: 20, H: 20}),
			blurred("(80,30,20,20)", "(74,24,32,32)",
				"translate 200,0|scale -1,1", "translate 0,100|scale 1,-1", "translate 200,100|scale -1,-1")},
		{"left only", blur(render.Rect{X: 2, Y: 15, W: 20, H: 20}),
			blurred("(2,15,20,20)", "(-4,9,32,32)", "scale -1,1")},
		{"wider than the surface: both sides", blur(render.Rect{X: -10, Y: 15, W: 120, H: 20}),
			blurred("(-10,15,120,20)", "(-6,9,112,32)", "scale -1,1", "translate 200,0|scale -1,1")},
		{"6 from the edge: none", blur(render.Rect{X: 6, Y: 6, W: 20, H: 20}),
			blurred("(6,6,20,20)", "(0,0,32,32)")},
	})
}

// splitOps splits "a|b" into its ops.
func splitOps(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '|' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// TestCanvasBlurClipOnly: the blurs that only push their clip, with no
// cut (as on Win32).
func TestCanvasBlurClipOnly(t *testing.T) {
	shape := render.Rect{X: 30, Y: 10, W: 20, H: 20}
	clipOnly := func(pre []string, clip string) []string {
		ops := append([]string{cvBaseOp, cvBlurFillOp}, pre...)
		if clip != "" {
			return append(ops, "save", "clip "+clip, "pop", "restore", "pop")
		}
		return append(ops, "save", "restore", "pop")
	}
	blur := func(pre func(c *canvas), r render.Rect, radius float64) func(c *canvas) {
		return func(c *canvas) {
			cvBlurFill(c)
			if pre != nil {
				pre(c)
			}
			c.PushBackdropBlur(r, render.Radii{}, radius)
			c.Pop()
		}
	}
	cvRun(t, []cvCase{
		{"radius 0", blur(nil, shape, 0), clipOnly(nil, "(30,10,20,20)")},
		{"negative radius", blur(nil, shape, -4), clipOnly(nil, "(30,10,20,20)")},
		{"NaN radius", blur(nil, shape, cvNaN), clipOnly(nil, "(30,10,20,20)")},
		{"Inf radius", blur(nil, shape, cvInf), clipOnly(nil, "(30,10,20,20)")},
		{"huge radius", blur(nil, shape, 2e6), clipOnly(nil, "(30,10,20,20)")},
		{"radius beyond the bound under a scale", blur(func(c *canvas) { c.Scale(10, 10) }, render.Rect{X: 1, Y: 1, W: 2, H: 2}, 2e5),
			clipOnly([]string{"scale 10,10"}, "(1,1,2,2)")},
		{"tiny radius under a tiny scale", blur(func(c *canvas) { c.Scale(1e-30, 1e-30) }, shape, 1e-30),
			clipOnly([]string{"scale 1e-30,1e-30"}, "(30,10,20,20)")},
		{"empty shape clips everything", blur(nil, render.Rect{X: 30, Y: 10, H: 20}, 4), clipOnly(nil, "(30,10,0,20)")},
		{"negative shape clips everything", blur(nil, render.Rect{X: 30, W: -1, H: 20}, 4), clipOnly(nil, "(0,0,0,0)")},
		{"Inf shape is ignored", blur(nil, render.Rect{X: cvInf, W: 1, H: 20}, 4), clipOnly(nil, "")},
		{"singular transform", blur(func(c *canvas) { c.Scale(0, 1) }, shape, 4),
			clipOnly([]string{"scale 0,1"}, "(30,10,20,20)")},
		{"outside the surface", blur(nil, render.Rect{X: 100, Y: 10, W: 20, H: 20}, 4), clipOnly(nil, "(100,10,20,20)")},
		{"outside after the transform", blur(func(c *canvas) { c.Translate(-60, 0) }, shape, 4),
			clipOnly([]string{"translate -60,0"}, "(30,10,20,20)")},
	})
}

// TestCanvasBlurEmptySurface: on a surface with no area a blur only clips.
func TestCanvasBlurEmptySurface(t *testing.T) {
	sn := &fakeSnapshot{}
	c := newCanvas(sn, render.Size{W: 0, H: 50}, 1, false)
	c.FillRoundedRect(render.Rect{W: 10, H: 10}, render.Radii{}, render.Solid(cvRed))
	c.PushBackdropBlur(render.Rect{W: 10, H: 10}, render.Radii{}, 4)
	c.Pop()
	if err := c.finish(); err != nil {
		t.Fatal(err)
	}
	if err := sn.balanced(); err != nil {
		t.Error(err)
	}
	cvWantOps(t, sn.ops, []string{"clip (0,0,0,50)", "color (0,0,10,10) " + cvRedOp,
		"save", "clip (0,0,10,10)", "pop", "restore", "pop"})
}

// TestCanvasBlurFinish: finish closes an open blur, ends the last segment
// and releases every node once, also with blurs left open and after a
// paint that broke off (the surface finishes after a panic); the canvas
// keeps no node.
func TestCanvasBlurFinish(t *testing.T) {
	for _, debug := range []bool{false, true} {
		sn := &fakeSnapshot{}
		c := newCanvas(sn, render.Size{W: 100, H: 50}, 1, debug)
		cvBlurFill(c)
		c.PushBackdropBlur(render.Rect{X: 30, Y: 10, W: 20, H: 20}, render.Radii{}, 4)
		c.PushOpacity(0.5)
		c.PushBackdropBlur(render.Rect{X: 35, Y: 15, W: 10, H: 10}, render.Radii{}, 2)
		err := c.finish()
		if (err != nil) != debug {
			t.Errorf("debug=%v: finish = %v", debug, err)
		}
		if err := sn.balanced(); err != nil {
			t.Errorf("debug=%v: %v", debug, err)
		}
		if len(sn.nodes) != 2 {
			t.Errorf("debug=%v: %d nodes, want 2", debug, len(sn.nodes))
		}
		if c.nodes != nil {
			t.Errorf("debug=%v: the canvas keeps %d nodes after finish", debug, len(c.nodes))
		}
	}
}
