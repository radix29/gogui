package gtk4

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
)

// The render seams and the readback (readback.go, S4.3f) over the fake
// window, and the fixed appearance of the golden renders.

// gallerySource is the appearance source of the golden renders (S4.3g),
// the same on every machine, as Win32's gallerySys: light (color-scheme 0,
// no preference), the default accent (no accent-color), normal contrast,
// Reduce Motion (reduced-motion 1, and gtk-enable-animations off) and
// legacy scrollers (gtk-overlay-scrolling off). Set its open as the
// shell's openSettings before Run.
func gallerySource() *fakeAppSource {
	f := newFakeAppSource()
	f.portal[keyColorScheme] = uint32(0)
	f.portal[keyContrast] = uint32(0)
	f.portal[keyReducedMotion] = uint32(1)
	f.gtk[gtkEnableAnimations] = false
	f.gtk[gtkOverlayScrolling] = false
	f.gtk[gtkPreferDark] = false
	return f
}

// galleryAppearance is what gallerySource reads as.
func galleryAppearance() driver.Appearance {
	a := driver.DefaultAppearance()
	a.Dark, a.IncreaseContrast, a.ReduceTransparency = false, false, false
	a.Accent = driver.DefaultAccent
	a.ReduceMotion, a.LegacyScrollers = true, true
	return a
}

// TestGallerySource: the golden renders' source reads as light, the
// default accent, normal contrast, Reduce Motion and legacy scrollers,
// and a shell opened over it reports that before Ready.
func TestGallerySource(t *testing.T) {
	want := galleryAppearance()
	if !want.ReduceMotion || !want.LegacyScrollers || want.Dark || want.Accent != driver.DefaultAccent {
		t.Fatalf("galleryAppearance() = %+v", want)
	}
	src := gallerySource()
	if a := readAppearance(src); a != want {
		t.Errorf("readAppearance(gallerySource()) = %+v, want %+v", a, want)
	}
	s, _ := newTestShell(t)
	s.openSettings = src.open
	err := run(t, s, &handler{ready: func() {
		defer s.Quit()
		if a := s.Appearance(); a != want {
			t.Errorf("Appearance() = %+v, want %+v", a, want)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if opens, closes := src.counts(); opens != 1 || closes != 1 {
		t.Errorf("the source was opened %d and closed %d times, want once each", opens, closes)
	}
}

// TestSurfaceFixedScale: a fixed scale is reported as a scale change
// (Resized, invalidated) and holds against notify::scale; 0 returns to the
// GdkSurface's scale; an unchanged one reports nothing; a bad one panics;
// a closed surface ignores it.
func TestSurfaceFixedScale(t *testing.T) {
	s, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	sf.invalid = false
	sf.setFixedScale(2)
	wantLog(t, "fixed 2", h.take(), "resized 400x300@2")
	if sf.Scale() != 2 || !sf.invalid || !s.clock.requested {
		t.Errorf("after setFixedScale(2): Scale %v, invalid %v, requested %v", sf.Scale(), sf.invalid, s.clock.requested)
	}
	sf.setFixedScale(2)
	fw.sc = 1.5
	fw.notify(kindScale)
	wantLog(t, "unchanged and native changes while fixed", h.take())
	if sf.Scale() != 2 {
		t.Errorf("Scale() = %v while fixed at 2", sf.Scale())
	}
	sf.setFixedScale(0)
	wantLog(t, "back to native", h.take(), "resized 400x300@1.5")
	fw.sc = 1.25
	fw.notify(kindScale)
	wantLog(t, "native again", h.take(), "resized 400x300@1.25")
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		mustPanicWith(t, "setFixedScale of a bad scale", panicsWith("setFixedScale"), func() { sf.setFixedScale(bad) })
	}
	sf.setFixedScale(1)
	wantLog(t, "fixed 1", h.take(), "resized 400x300@1")
	h.onPaint = func(render.Canvas) {
		mustPanicWith(t, "setFixedScale during Paint", panicsWith("during SurfaceHandler.Paint"), func() { sf.setFixedScale(3) })
	}
	fw.snap()
	if sf.Scale() != 1 {
		t.Errorf("Scale() = %v after a setFixedScale during Paint", sf.Scale())
	}
	h.onPaint = nil
	h.take()
	sf.Close()
	h.take()
	sf.setFixedScale(2)
	wantLog(t, "after Close", h.take())
}

// TestSurfaceFixedKey: a fixed key state is reported as notify::is-active
// would (KeyChanged, the input method's focus, with the text input on)
// and holds against it;
// keyNative returns to gtk_window_is_active; an unchanged one reports
// nothing; a bad one panics.
func TestSurfaceFixedKey(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	sf.SetTextInput(textInputOn)
	fw.ims = nil
	sf.setFixedKey(keyOn)
	wantLog(t, "fixed on", h.take(), "key true")
	wantLog(t, "fixed on: the input method", fw.ims, "focus true")
	fw.ims = nil
	sf.setFixedKey(keyOn)
	fw.notify(kindActive) // the window manager says inactive
	wantLog(t, "unchanged and native changes while fixed", h.take())
	sf.setFixedKey(keyOff)
	wantLog(t, "fixed off", h.take(), "key false")
	wantLog(t, "fixed off: the input method", fw.ims, "reset", "focus false")
	fw.isActive = true
	fw.notify(kindActive)
	wantLog(t, "native change while fixed off", h.take())
	sf.setFixedKey(keyNative)
	wantLog(t, "back to native", h.take(), "key true")
	fw.isActive = false
	fw.notify(kindActive)
	wantLog(t, "native again", h.take(), "key false")
	mustPanicWith(t, "setFixedKey of a bad state", panicsWith("setFixedKey"), func() { sf.setFixedKey(keyOff + 1) })
	sf.Close()
	h.take()
	sf.setFixedKey(keyOn)
	wantLog(t, "after Close", h.take())
}

// TestSurfaceReadback: a readback needs a shown surface with no paint
// pending; it asks the window at the surface's scale (the fixed one
// included) for the allocation × scale pixels and checks what comes back;
// the window's errors pass through, a Paint the native readback runs is
// guarded and its panic re-raised; a closed surface fails.
func TestSurfaceReadback(t *testing.T) {
	_, f := newSurfaceTest(t)
	h := &sfHandler{}
	sf, fw := f.open(t, sfResizable, h)
	if _, err := sf.readback(); !errors.Is(err, errReadbackHidden) {
		t.Errorf("readback of a hidden surface = %v, want errReadbackHidden", err)
	}
	sf.Show()
	fw.layout()
	if _, err := sf.readback(); !errors.Is(err, errReadbackPending) {
		t.Errorf("readback before the paint = %v, want errReadbackPending", err)
	}
	fw.snap()
	fr, err := sf.readback()
	if err != nil {
		t.Fatal(err)
	}
	if fr.w != 400 || fr.h != 300 || fr.stride != 1600 || len(fr.pix) != 1600*300 || fr.renderer != "FakeRenderer" {
		t.Errorf("readback at 1×: %d×%d, stride %d, %d bytes, renderer %q", fr.w, fr.h, fr.stride, len(fr.pix), fr.renderer)
	}
	sf.setFixedScale(2)
	if _, err := sf.readback(); !errors.Is(err, errReadbackPending) {
		t.Errorf("readback after the scale change = %v, want errReadbackPending", err)
	}
	fw.snap()
	if fr, err = sf.readback(); err != nil || fr.w != 800 || fr.h != 600 {
		t.Errorf("readback at 2× = %d×%d, %v; want 800×600", fr.w, fr.h, err)
	}
	if !slices.Equal(fw.readbacks, []float64{1, 2}) {
		t.Errorf("the window was asked at the scales %v, want [1 2]", fw.readbacks)
	}
	for name, bad := range map[string]readbackFrame{
		"size":   {w: 800, h: 599, stride: 3200, pix: make([]byte, 3200*599)},
		"stride": {w: 800, h: 600, stride: 3199, pix: make([]byte, 3200*600)},
		"bytes":  {w: 800, h: 600, stride: 3200, pix: make([]byte, 3200*600-1)},
	} {
		fw.rbFrame = &bad
		if _, err := sf.readback(); err == nil || !strings.Contains(err.Error(), "readback:") {
			t.Errorf("readback of a frame with a bad %s = %v, want an error", name, err)
		}
	}
	fw.rbFrame = &readbackFrame{w: 800, h: 600, stride: 3300, pix: make([]byte, 3300*600)}
	if _, err := sf.readback(); err != nil {
		t.Errorf("readback of padded rows = %v", err)
	}
	fw.rbFrame = nil
	fw.rbErr = errors.New("no renderer")
	if _, err := sf.readback(); err != fw.rbErr {
		t.Errorf("readback = %v, want the window's error", err)
	}
	fw.rbErr = nil
	fw.rbPaint = true
	h.onPaint = func(render.Canvas) { panic("boom") }
	mustPanicWith(t, "a Paint panicking inside the readback", func(v any) bool {
		pe, ok := v.(*PanicError)
		return ok && pe.Value == "boom"
	}, func() { _, _ = sf.readback() })
	fw.rbPaint = false
	h.onPaint = func(render.Canvas) {
		mustPanicWith(t, "readback during Paint", panicsWith("during SurfaceHandler.Paint"), func() { _, _ = sf.readback() })
	}
	fw.snap()
	h.onPaint = nil
	sf.Close()
	if _, err := sf.readback(); !errors.Is(err, errReadbackClosed) {
		t.Errorf("readback of a closed surface = %v, want errReadbackClosed", err)
	}
}
