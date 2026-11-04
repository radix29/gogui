package driver

import (
	"math"
	"testing"
)

func TestRectContainsEdges(t *testing.T) {
	r := Rect{10, 20, 30, 40}
	for _, tc := range []struct {
		p    Point
		want bool
	}{
		{Point{10, 20}, true}, // top-left corner is inside
		{Point{39.9, 59.9}, true},
		{Point{40, 30}, false}, // right edge is outside
		{Point{20, 60}, false}, // bottom edge is outside
		{Point{9.9, 30}, false},
	} {
		if got := r.Contains(tc.p); got != tc.want {
			t.Errorf("Contains(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

func TestRectInset(t *testing.T) {
	r := Rect{0, 0, 100, 50}
	if got, want := r.Inset(Insets{Top: 1, Left: 2, Bottom: 3, Right: 4}), (Rect{2, 1, 94, 46}); got != want {
		t.Errorf("Inset = %v, want %v", got, want)
	}
	// Insets larger than the rect collapse it to its center.
	if got, want := r.Inset(UniformInsets(60)), (Rect{50, 25, 0, 0}); got != want {
		t.Errorf("oversized Inset = %v, want %v", got, want)
	}
}

func TestRectIntersect(t *testing.T) {
	a := Rect{0, 0, 10, 10}
	if got, want := a.Intersect(Rect{5, 5, 10, 10}), (Rect{5, 5, 5, 5}); got != want {
		t.Errorf("Intersect = %v, want %v", got, want)
	}
	if got := a.Intersect(Rect{10, 0, 5, 5}); got != (Rect{}) {
		t.Errorf("touching Intersect = %v, want zero", got)
	}
}

func TestSizeHelpers(t *testing.T) {
	u := Unbounded()
	if !math.IsInf(u.W, 1) || !math.IsInf(u.H, 1) {
		t.Fatalf("Unbounded = %v", u)
	}
	s := Size{10, 20}
	if got := s.Min(u); got != s {
		t.Errorf("Min(Unbounded) = %v", got)
	}
	if got, want := s.Inset(UniformInsets(6)), (Size{0, 8}); got != want {
		t.Errorf("Inset = %v, want %v", got, want)
	}
	if got, want := s.Outset(Insets{1, 2, 3, 4}), (Size{16, 24}); got != want {
		t.Errorf("Outset = %v, want %v", got, want)
	}
	if !(Size{}).IsZero() || (Size{0, 1}).IsZero() {
		t.Error("IsZero is wrong")
	}
}

func TestColor(t *testing.T) {
	if (Color{}).IsSemantic() {
		t.Error("zero Color must be concrete")
	}
	c := Semantic(ColorLabel)
	if !c.IsSemantic() || !c.Semantic.Valid() {
		t.Errorf("Semantic(ColorLabel) = %+v", c)
	}
	if semanticColorCount.Valid() {
		t.Error("semanticColorCount must not be valid")
	}
	if got := RGB8(255, 0, 51); got != (Color{R: 1, G: 0, B: 0.2, A: 1}) {
		t.Errorf("RGB8 = %+v", got)
	}
	if NumSemanticColors != int(ColorSystemGray) {
		t.Errorf("NumSemanticColors = %d, want %d", NumSemanticColors, ColorSystemGray)
	}
}

func TestFontDefaults(t *testing.T) {
	var f Font
	if !f.IsSystem() || f.EffectiveWeight() != FontWeightRegular {
		t.Errorf("zero Font: system=%v weight=%v", f.IsSystem(), f.EffectiveWeight())
	}
	for cs, want := range map[ControlSize]float64{
		ControlSizeRegular: 13, ControlSizeSmall: 11, ControlSizeMini: 9, ControlSizeLarge: 13,
	} {
		if got := f.EffectiveSize(cs); got != want {
			t.Errorf("EffectiveSize(%d) = %v, want %v", cs, got, want)
		}
	}
	if got := (Font{Size: 20}).EffectiveSize(ControlSizeMini); got != 20 {
		t.Errorf("explicit size = %v", got)
	}
}

func TestControlSizeMatchesNSControlSize(t *testing.T) {
	// NSControlSizeRegular = 0, Small = 1, Mini = 2, Large = 3.
	if ControlSizeRegular != 0 || ControlSizeSmall != 1 || ControlSizeMini != 2 || ControlSizeLarge != 3 {
		t.Error("ControlSize values must match NSControlSize")
	}
}

func TestWindowStyleMatchesNSWindowStyleMask(t *testing.T) {
	if StyleTitled != 1 || StyleClosable != 2 || StyleMiniaturizable != 4 || StyleResizable != 8 {
		t.Error("WindowStyle bits must match NSWindowStyleMask")
	}
}

func TestCheckStateMatchesNSControlStateValue(t *testing.T) {
	if StateOff != 0 || StateOn != 1 || StateMixed != -1 {
		t.Error("CheckState values must match NSControlStateValue")
	}
}
