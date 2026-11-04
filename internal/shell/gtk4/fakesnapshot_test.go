package gtk4

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/radix29/gogui/internal/shell/gtk4/gtk"
)

// fakeSnapshot is a snapshotter that records its calls as text, one line
// per call, and checks GtkSnapshot's nesting rules: a restore closes a save
// with no push left open above it, a pop closes a push with no save left
// open above it. Violations go to errs.
type fakeSnapshot struct {
	ops   []string
	stack []string // "save", "clip", "opacity", "stroke", "fill" or "blur"
	errs  []string

	textures     []*fakeTexture // made by newTexture, in order
	failTextures bool           // newTexture makes none

	// Segments (S4.2f): every call but newTexture must come inside one.
	open, drawn bool        // a segment is open; it drew something
	nodes       []*fakeNode // made by cut, in order
}

var _ snapshotter = (*fakeSnapshot)(nil)

func fnum(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }

func frect(r gtk.GrapheneRect) string {
	return "(" + fnum(r.Origin.X) + "," + fnum(r.Origin.Y) + "," + fnum(r.Size.Width) + "," + fnum(r.Size.Height) + ")"
}

func frounded(r gtk.RoundedRect) string {
	s := frect(r.Bounds)
	c := r.Corner
	if c != [4]gtk.GrapheneSize{} {
		var parts []string
		for _, v := range c {
			if v.Width == v.Height {
				parts = append(parts, fnum(v.Width))
			} else {
				parts = append(parts, fnum(v.Width)+"x"+fnum(v.Height))
			}
		}
		if parts[0] == parts[1] && parts[1] == parts[2] && parts[2] == parts[3] {
			s += " r=" + parts[0]
		} else {
			s += " r=" + strings.Join(parts, "/")
		}
	}
	return s
}

func fcolor(c gtk.RGBA) string {
	return "rgba(" + fnum(c.R) + "," + fnum(c.G) + "," + fnum(c.B) + "," + fnum(c.A) + ")"
}

func (f *fakeSnapshot) add(format string, args ...any) {
	op := fmt.Sprintf(format, args...)
	if !f.open {
		f.errs = append(f.errs, op+" outside a segment")
	}
	f.ops = append(f.ops, op)
}

func (f *fakeSnapshot) push(kind string) { f.stack = append(f.stack, kind) }

func (f *fakeSnapshot) close(op string, want func(string) bool) {
	if len(f.stack) == 0 {
		f.errs = append(f.errs, op+" with nothing open")
		return
	}
	top := f.stack[len(f.stack)-1]
	if !want(top) {
		f.errs = append(f.errs, fmt.Sprintf("%s closes a %s", op, top))
	}
	f.stack = f.stack[:len(f.stack)-1]
}

func (f *fakeSnapshot) save() { f.add("save"); f.push("save") }

func (f *fakeSnapshot) restore() {
	f.add("restore")
	f.close("restore", func(k string) bool { return k == "save" })
}

func (f *fakeSnapshot) pop() {
	f.add("pop")
	f.close("pop", func(k string) bool { return k != "save" })
}

func (f *fakeSnapshot) translate(dx, dy float32) { f.add("translate %s,%s", fnum(dx), fnum(dy)) }

func (f *fakeSnapshot) scale(sx, sy float32) { f.add("scale %s,%s", fnum(sx), fnum(sy)) }

func (f *fakeSnapshot) matrix(t [6]float32) {
	parts := make([]string, len(t))
	for i, v := range t {
		parts[i] = fnum(v)
	}
	f.add("matrix %s", strings.Join(parts, ","))
}

func (f *fakeSnapshot) pushRoundedClip(r gtk.RoundedRect) {
	f.add("clip %s", frounded(r))
	f.push("clip")
}

func (f *fakeSnapshot) pushOpacity(alpha float64) {
	f.add("opacity %s", strconv.FormatFloat(alpha, 'g', -1, 64))
	f.push("opacity")
}

func (f *fakeSnapshot) appendColor(c gtk.RGBA, r gtk.GrapheneRect) {
	f.add("color %s %s", frect(r), fcolor(c))
	f.drew()
}

func (f *fakeSnapshot) appendBorder(outline gtk.RoundedRect, width float32, c gtk.RGBA) {
	f.add("border %s w=%s %s", frounded(outline), fnum(width), fcolor(c))
	f.drew()
}

func (f *fakeSnapshot) appendLinearGradient(r gtk.GrapheneRect, start, end gtk.GraphenePoint, stops []gtk.ColorStop) {
	parts := make([]string, len(stops))
	for i, s := range stops {
		parts[i] = fnum(s.Offset) + ":" + fcolor(s.Color)
	}
	f.add("linear %s (%s,%s)->(%s,%s) [%s]", frect(r), fnum(start.X), fnum(start.Y), fnum(end.X), fnum(end.Y),
		strings.Join(parts, " "))
	f.drew()
}

// balanced reports the nesting errors, and anything left open.
func (f *fakeSnapshot) balanced() error {
	errs := append([]string(nil), f.errs...)
	if len(f.stack) > 0 {
		errs = append(errs, "left open: "+strings.Join(f.stack, ", "))
	}
	if f.open {
		errs = append(errs, "the last segment never ended")
	}
	for _, n := range f.nodes {
		if n.released != 1 {
			errs = append(errs, fmt.Sprintf("node #%d released %d times, want once", n.id, n.released))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

func (f *fakeSnapshot) pushStroke(outline gtk.RoundedRect, width float32) {
	f.add("stroke %s w=%s", frounded(outline), fnum(width))
	f.push("stroke")
}

// fpath prints path commands as SVG-like text: "M1,2 L3,4 Q… C… Z".
func fpath(cmds []pathCmd) string {
	parts := make([]string, len(cmds))
	for i, c := range cmds {
		s := c.op.String()
		for j := range segmentPoints(c.op) {
			if j > 0 {
				s += " "
			}
			s += fnum(c.pts[j].X) + "," + fnum(c.pts[j].Y)
		}
		parts[i] = s
	}
	return strings.Join(parts, " ")
}

// fstroke prints a pathStroke: width, cap, join, miter limit and, when
// dashed, the dashes and offset.
func fstroke(st pathStroke) string {
	s := fmt.Sprintf("w=%s cap=%d join=%d miter=%s", fnum(st.width), st.cap, st.join, fnum(st.miter))
	if st.dash != nil {
		parts := make([]string, len(st.dash))
		for i, d := range st.dash {
			parts[i] = fnum(d)
		}
		s += " dash=" + strings.Join(parts, ",") + "@" + fnum(st.offset)
	}
	return s
}

func (f *fakeSnapshot) appendFill(path []pathCmd, c gtk.RGBA) {
	f.add("fill [%s] %s", fpath(path), fcolor(c))
	f.drew()
}

func (f *fakeSnapshot) pushFill(path []pathCmd) {
	f.add("push fill [%s]", fpath(path))
	f.push("fill")
}

func (f *fakeSnapshot) appendStroke(path []pathCmd, st pathStroke, c gtk.RGBA) {
	f.add("stroke path [%s] %s %s", fpath(path), fstroke(st), fcolor(c))
	f.drew()
}

func (f *fakeSnapshot) pushPathStroke(path []pathCmd, st pathStroke) {
	f.add("push stroke path [%s] %s", fpath(path), fstroke(st))
	f.push("stroke")
}

func (f *fakeSnapshot) appendShadow(sh shadowCall) {
	kind := "outset"
	if sh.inset {
		kind = "inset"
	}
	f.add("%s shadow %s %s d=%s,%s spread=%s blur=%s", kind, frounded(sh.outline), fcolor(sh.color),
		fnum(sh.dx), fnum(sh.dy), fnum(sh.spread), fnum(sh.blur))
	f.drew()
}

// fakeTexture is a texture the fake made: its number (from 1, in the order
// of the fake's textures), its pixel size and pixels, and how often it was
// released.
type fakeTexture struct {
	id, w, h int
	pix      []byte
	released int
}

func (t *fakeTexture) release() { t.released++ }

// newTexture records "texture #id WxH" and returns a new fakeTexture, or
// nothing when failTextures is set.
func (f *fakeSnapshot) newTexture(pix []byte, w, h int) (texture, bool) {
	if f.failTextures {
		f.ops = append(f.ops, fmt.Sprintf("texture %dx%d failed", w, h))
		return nil, false
	}
	t := &fakeTexture{id: len(f.textures) + 1, w: w, h: h, pix: append([]byte(nil), pix...)}
	f.textures = append(f.textures, t)
	f.ops = append(f.ops, fmt.Sprintf("texture #%d %dx%d", t.id, w, h))
	return t, true
}

func (f *fakeSnapshot) appendTexture(t texture, r gtk.GrapheneRect) {
	id := 0
	if ft, ok := t.(*fakeTexture); ok {
		id = ft.id
	}
	f.add("draw texture #%d %s", id, frect(r))
	f.drew()
}

// --- Segments (S4.2f) ---------------------------------------------------------

// fakeNode is a node a segment of the fake ended in: its number (from 1,
// in the order of the fake's nodes) and how often it was released.
type fakeNode struct {
	id       int
	released int
}

func (n *fakeNode) release() { n.released++ }

// drew marks the current segment as having drawn something, so cut
// returns a node for it.
func (f *fakeSnapshot) drew() { f.drawn = true }

func (f *fakeSnapshot) begin() {
	if f.open {
		f.errs = append(f.errs, "begin with a segment open")
	}
	f.open, f.drawn = true, false
}

// endSegment checks that a segment is open and balanced and ends it.
func (f *fakeSnapshot) endSegment(op string) {
	if !f.open {
		f.errs = append(f.errs, op+" with no segment open")
	}
	if len(f.stack) > 0 {
		f.errs = append(f.errs, op+" with "+strings.Join(f.stack, ", ")+" open")
		f.stack = nil
	}
	f.open = false
}

// cut records "cut #id", or "cut (empty)" when the segment drew nothing.
func (f *fakeSnapshot) cut() (renderNode, bool) {
	f.endSegment("cut")
	drawn := f.drawn
	f.open, f.drawn = true, false
	if !drawn {
		f.ops = append(f.ops, "cut (empty)")
		return nil, false
	}
	n := &fakeNode{id: len(f.nodes) + 1}
	f.nodes = append(f.nodes, n)
	f.ops = append(f.ops, fmt.Sprintf("cut #%d", n.id))
	return n, true
}

// end is not recorded: balanced checks that the last segment ended.
func (f *fakeSnapshot) end() { f.endSegment("end") }

func (f *fakeSnapshot) appendNode(n renderNode) {
	id := 0
	if fn, ok := n.(*fakeNode); ok {
		id = fn.id
		if fn.released > 0 {
			f.errs = append(f.errs, fmt.Sprintf("node #%d drawn after its release", id))
		}
	}
	f.add("node #%d", id)
	f.drew()
}

func (f *fakeSnapshot) pushBlur(radius float64) {
	f.add("blur %s", strconv.FormatFloat(radius, 'g', -1, 64))
	f.push("blur")
}

// --- Text (S4.2i) -------------------------------------------------------------

// appendLayout records "text #id color", the id of the txLayout drawn (0
// for any other nativeLayout), and flags a layout drawn after its release.
func (f *fakeSnapshot) appendLayout(l nativeLayout, c gtk.RGBA) {
	id := 0
	if tl, ok := l.(*txNative); ok {
		id = tl.id
		if tl.released > 0 {
			f.errs = append(f.errs, fmt.Sprintf("layout #%d drawn after its release", id))
		}
	}
	f.add("text #%d %s", id, fcolor(c))
	f.drew()
}
