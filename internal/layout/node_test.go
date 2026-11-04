package layout

import (
	"math"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

const eps = 1e-9

var inf = math.Inf(1)

func near(a, b float64) bool { return a == b || math.Abs(a-b) <= eps }

func sizeNear(a, b driver.Size) bool { return near(a.W, b.W) && near(a.H, b.H) }

func rectNear(a, b driver.Rect) bool {
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

// probe is an Item that counts calls and reports a fixed fitting size.
type probe struct {
	fit      driver.Size
	measures int
	frames   int
	frame    driver.Rect
}

func (p *probe) FittingSize(max driver.Size) driver.Size { p.measures++; return p.fit.Min(max) }
func (p *probe) SetFrame(r driver.Rect)                  { p.frames++; p.frame = r }

func button(title string) *stub.Button {
	b := stub.NewButton(nil)
	b.Title = title
	return b
}

func TestLeafMeasureAndArrange(t *testing.T) {
	b := button("Greet") // 5×7 + 28 = 63 × 22
	l := NewLeaf(b)
	if l.Item() != b {
		t.Fatal("Item")
	}
	var p Pass
	want := driver.Size{W: 63, H: 22}
	for _, max := range []driver.Size{driver.Unbounded(), {W: 100, H: 100}, {W: 63, H: 22}} {
		if got := p.Measure(l, max); !sizeNear(got, want) {
			t.Errorf("Measure(%v) = %v, want %v", max, got, want)
		}
	}
	r := driver.Rect{X: 3, Y: 4, W: 70, H: 22}
	l.Arrange(&p, r)
	if b.Frame != r {
		t.Errorf("frame = %v, want %v", b.Frame, r)
	}
	var nilPass *Pass
	if got := nilPass.Measure(l, driver.Unbounded()); !sizeNear(got, want) {
		t.Errorf("nil Pass: %v", got)
	}
}

func TestLeafDefaultMinOverflows(t *testing.T) {
	l := NewLeaf(button("Greet"))
	var p Pass
	// Without an app-set min, the fitting size is the min: the result
	// overflows max instead of shrinking.
	for _, max := range []driver.Size{{}, {W: 10, H: 5}, {W: -1, H: math.NaN()}} {
		if got := p.Measure(l, max); !sizeNear(got, driver.Size{W: 63, H: 22}) {
			t.Errorf("Measure(%v) = %v", max, got)
		}
	}
}

func TestLeafShrinksTowardsMin(t *testing.T) {
	l := NewLeaf(button("Greet"))
	l.Props().SetMin(Horizontal, 40).SetMin(Vertical, 10)
	var p Pass
	cases := []struct{ max, want driver.Size }{
		{driver.Unbounded(), driver.Size{W: 63, H: 22}},
		{driver.Size{W: 50, H: 15}, driver.Size{W: 50, H: 15}},
		{driver.Size{W: 40, H: 10}, driver.Size{W: 40, H: 10}},
		{driver.Size{W: 0, H: 0}, driver.Size{W: 40, H: 10}}, // the min
	}
	for _, c := range cases {
		if got := p.Measure(l, c.max); !sizeNear(got, c.want) {
			t.Errorf("Measure(%v) = %v, want %v", c.max, got, c.want)
		}
	}
}

func TestLeafMinMaxFixed(t *testing.T) {
	type setup func(*Props)
	cases := []struct {
		name string
		set  setup
		max  driver.Size
		want driver.Size
	}{
		{"max below fit has no effect alone", func(p *Props) { p.SetMax(Horizontal, 30) }, driver.Unbounded(), driver.Size{W: 63, H: 22}},
		{"max with small min", func(p *Props) { p.SetMin(Horizontal, 0).SetMax(Horizontal, 30) }, driver.Unbounded(), driver.Size{W: 30, H: 22}},
		{"min above natural", func(p *Props) { p.SetMin(Horizontal, 100) }, driver.Unbounded(), driver.Size{W: 100, H: 22}},
		{"min wins over max", func(p *Props) { p.SetMin(Horizontal, 80).SetMax(Horizontal, 50) }, driver.Unbounded(), driver.Size{W: 80, H: 22}},
		{"fixed larger", func(p *Props) { p.SetFixed(Horizontal, 120).SetFixed(Vertical, 30) }, driver.Unbounded(), driver.Size{W: 120, H: 30}},
		{"fixed smaller", func(p *Props) { p.SetFixed(Horizontal, 20) }, driver.Unbounded(), driver.Size{W: 20, H: 22}},
		{"fixed overflows max", func(p *Props) { p.SetFixed(Horizontal, 120) }, driver.Size{W: 50, H: 50}, driver.Size{W: 120, H: 22}},
		{"fixed 0", func(p *Props) { p.SetFixed(Horizontal, 0).SetFixed(Vertical, 0) }, driver.Unbounded(), driver.Size{}},
		{"negative min is 0", func(p *Props) { p.SetMin(Horizontal, -5) }, driver.Size{W: 10, H: 22}, driver.Size{W: 10, H: 22}},
		{"NaN min is 0", func(p *Props) { p.SetMin(Horizontal, math.NaN()) }, driver.Size{W: 10, H: 22}, driver.Size{W: 10, H: 22}},
		{"+Inf min is unset", func(p *Props) { p.SetMin(Horizontal, inf) }, driver.Size{W: 10, H: 22}, driver.Size{W: 63, H: 22}},
		{"negative max is 0", func(p *Props) { p.SetMin(Horizontal, 0).SetMax(Horizontal, -5) }, driver.Unbounded(), driver.Size{W: 0, H: 22}},
		{"NaN max is 0", func(p *Props) { p.SetMin(Horizontal, 0).SetMax(Horizontal, math.NaN()) }, driver.Unbounded(), driver.Size{W: 0, H: 22}},
		{"+Inf max is unset", func(p *Props) { p.SetMin(Horizontal, 0).SetMax(Horizontal, 30).SetMax(Horizontal, inf) }, driver.Unbounded(), driver.Size{W: 63, H: 22}},
		{"min 100 at max 0", func(p *Props) { p.SetMin(Horizontal, 100) }, driver.Size{}, driver.Size{W: 100, H: 22}},
	}
	for _, c := range cases {
		l := NewLeaf(button("Greet"))
		c.set(l.Props())
		if got := (&Pass{}).Measure(l, c.max); !sizeNear(got, c.want) {
			t.Errorf("%s: Measure(%v) = %v, want %v", c.name, c.max, got, c.want)
		}
	}
}

func TestPropsGetters(t *testing.T) {
	var p Props
	if v, ok := p.Min(Horizontal); ok || v != 0 {
		t.Errorf("zero Min = %v, %v", v, ok)
	}
	if !math.IsInf(p.Max(Vertical), 1) {
		t.Errorf("zero Max = %v", p.Max(Vertical))
	}
	if p.Align(Horizontal) != AlignDefault || p.Expand(Vertical) || p.Hidden() {
		t.Error("zero Props not default")
	}
	p.SetMin(Vertical, 7).SetMax(Vertical, 9).SetAlign(Horizontal, Trailing).
		SetExpand(Vertical, true).SetHidden(true)
	if v, ok := p.Min(Vertical); !ok || v != 7 {
		t.Errorf("Min = %v, %v", v, ok)
	}
	if p.Max(Vertical) != 9 || p.Align(Horizontal) != Trailing || !p.Expand(Vertical) || !p.Hidden() {
		t.Error("setters not recorded")
	}
	if _, ok := p.Min(Horizontal); ok {
		t.Error("axes not independent")
	}
	p.SetMin(Vertical, inf)
	if v, ok := p.Min(Vertical); ok || v != 0 {
		t.Errorf("Min after +Inf = %v, %v", v, ok)
	}
	if lo, hi := p.SetMin(Vertical, 12).limits(Vertical, 0); lo != 12 || hi != 12 {
		t.Errorf("limits with min > max = %v, %v", lo, hi)
	}
	if Horizontal.Cross() != Vertical || Vertical.Cross() != Horizontal {
		t.Error("Cross")
	}
}

func TestLeafExpands(t *testing.T) {
	l := NewLeaf(button("x"))
	l.Props().SetExpand(Horizontal, true)
	if !l.Expands(Horizontal) || l.Expands(Vertical) {
		t.Error("Expands should follow the own flag only")
	}
}

func TestLeafHeightForWidth(t *testing.T) {
	lb := stub.NewLabel()
	lb.Text = "0123456789" // 70 × 16 on one line
	lb.Wraps = true
	l := NewLeaf(lb)
	var p Pass
	if got := p.Measure(l, driver.Size{W: 30, H: inf}); !sizeNear(got, driver.Size{W: 70, H: 16}) {
		t.Errorf("default min keeps one line: %v", got)
	}
	l.Props().SetMin(Horizontal, 30)
	p = Pass{} // the cache is per pass; a changed tree needs a fresh one
	// At 30 pt the text needs ceil(70/30) = 3 lines.
	if got := p.Measure(l, driver.Size{W: 30, H: inf}); !sizeNear(got, driver.Size{W: 30, H: 48}) {
		t.Errorf("wrapped at 30: %v", got)
	}
	if got := p.Measure(l, driver.Size{W: 0, H: 0}); !sizeNear(got, driver.Size{W: 30, H: 48}) {
		t.Errorf("min height follows the wrapped width: %v", got)
	}
}

func TestLeafBadItemSizes(t *testing.T) {
	it := &probe{fit: driver.Size{W: math.NaN(), H: -3}}
	if got := (&Pass{}).Measure(NewLeaf(it), driver.Unbounded()); got != (driver.Size{}) {
		t.Errorf("NaN/negative fitting size = %v", got)
	}
	it = &probe{fit: driver.Unbounded()}
	if got := (&Pass{}).Measure(NewLeaf(it), driver.Unbounded()); got != (driver.Size{}) {
		t.Errorf("+Inf fitting size = %v", got)
	}
}

func TestLeafHidden(t *testing.T) {
	it := &probe{fit: driver.Size{W: 10, H: 10}}
	l := NewLeaf(it)
	l.Props().SetHidden(true)
	var p Pass
	if got := p.Measure(l, driver.Unbounded()); got != (driver.Size{}) {
		t.Errorf("hidden Measure = %v", got)
	}
	if got := l.Measure(&p, driver.Unbounded()); got != (driver.Size{}) {
		t.Errorf("hidden direct Measure = %v", got)
	}
	l.Arrange(&p, driver.Rect{W: 5, H: 5})
	if it.measures != 0 || it.frames != 0 {
		t.Errorf("hidden leaf: %d FittingSize, %d SetFrame calls", it.measures, it.frames)
	}
}

func TestNewLeafNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	NewLeaf(nil)
}

func TestPlace(t *testing.T) {
	slot := driver.Rect{X: 10, Y: 20, W: 100, H: 50}
	size := driver.Size{W: 40, H: 10}
	cases := []struct {
		h, v Align
		want driver.Rect
	}{
		{AlignDefault, AlignDefault, driver.Rect{X: 10, Y: 20, W: 40, H: 10}},
		{Leading, Leading, driver.Rect{X: 10, Y: 20, W: 40, H: 10}},
		{Center, Center, driver.Rect{X: 40, Y: 40, W: 40, H: 10}},
		{Trailing, Trailing, driver.Rect{X: 70, Y: 60, W: 40, H: 10}},
		{Fill, Fill, driver.Rect{X: 10, Y: 20, W: 100, H: 50}},
		{Fill, Trailing, driver.Rect{X: 10, Y: 60, W: 100, H: 10}},
		{Center, Fill, driver.Rect{X: 40, Y: 20, W: 40, H: 50}},
	}
	for _, c := range cases {
		if got := place(slot, size, c.h, c.v); !rectNear(got, c.want) {
			t.Errorf("place(%v, %v) = %v, want %v", c.h, c.v, got, c.want)
		}
	}
	// Larger than the slot: starts at the origin and overflows, whatever
	// the alignment.
	big := driver.Size{W: 150, H: 80}
	for _, al := range []Align{AlignDefault, Leading, Center, Trailing, Fill} {
		want := driver.Rect{X: 10, Y: 20, W: 150, H: 80}
		if got := place(slot, big, al, al); !rectNear(got, want) {
			t.Errorf("overflow place(%v) = %v, want %v", al, got, want)
		}
	}
	// Fractional sizes are not rounded.
	if got := place(driver.Rect{W: 10}, driver.Size{W: 3}, Center, Leading); !rectNear(got, driver.Rect{X: 3.5, W: 3}) {
		t.Errorf("fractional center = %v", got)
	}
}

func TestClampAndNorm(t *testing.T) {
	if clamp(5, 10, 3) != 10 || clamp(5, 0, 3) != 3 || clamp(-1, 0, 3) != 0 || clamp(2, 0, inf) != 2 {
		t.Error("clamp")
	}
	if norm(-1) != 0 || norm(math.NaN()) != 0 || !math.IsInf(norm(inf), 1) || norm(2.5) != 2.5 {
		t.Error("norm")
	}
	if finite(inf) != 0 || finite(math.Inf(-1)) != 0 || finite(4) != 4 {
		t.Error("finite")
	}
}
