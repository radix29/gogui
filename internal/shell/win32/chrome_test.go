//go:build windows

package win32

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The regions of a 400 pt wide title bar as the aqua driver sets them:
// the bar, then the three lights (aqua/chrome.go).
var barRegions = []shell.ChromeRegion{
	{Rect: driver.Rect{W: 400, H: 28}, Kind: shell.RegionCaption},
	{Rect: driver.Rect{X: 8, Y: 8, W: 12, H: 12}, Kind: shell.RegionClose},
	{Rect: driver.Rect{X: 28, Y: 8, W: 12, H: 12}, Kind: shell.RegionMinimize},
	{Rect: driver.Rect{X: 48, Y: 8, W: 12, H: 12}, Kind: shell.RegionMaximize},
}

func TestHitTest(t *testing.T) {
	// A window at (100, 50) in screen pixels, 2 px scale: an 8 px side and
	// bottom frame, the client area reaching the top edge, an 8 px band.
	win := w32.Rect{Left: 100, Top: 50, Right: 916, Bottom: 658}
	cl := w32.Rect{Left: 108, Top: 50, Right: 908, Bottom: 650}
	base := hitInput{window: win, client: cl, band: 8, corner: 16, scale: 2, resize: true,
		outside: w32.HTNOWHERE, regions: barRegions}
	pt := func(x, y int32) w32.Point { return w32.Point{X: x, Y: y} }
	// px returns the screen pixel of a point of the drawable area.
	px := func(x, y float64) w32.Point { return pt(cl.Left+int32(x*2), cl.Top+int32(y*2)) }
	cases := []struct {
		name string
		in   func(hitInput) hitInput
		pt   w32.Point
		want uintptr
	}{
		{"outside the window", nil, pt(99, 300), w32.HTNOWHERE},
		{"below the window", nil, pt(300, 658), w32.HTNOWHERE},
		{"left border", nil, pt(100, 300), w32.HTLEFT},
		{"right border", nil, pt(915, 300), w32.HTRIGHT},
		{"bottom border", nil, pt(500, 650), w32.HTBOTTOM},
		{"top band inside the client", nil, pt(500, 50), w32.HTTOP},
		{"top band, last row", nil, pt(500, 57), w32.HTTOP},
		{"under the band: caption", nil, pt(500, 58), w32.HTCAPTION},
		{"top-left corner", nil, pt(101, 51), w32.HTTOPLEFT},
		{"top-left along the top edge", nil, pt(123, 52), w32.HTTOPLEFT},
		{"top-left along the left edge", nil, pt(103, 65), w32.HTTOPLEFT},
		{"left edge below the corner", nil, pt(103, 66), w32.HTLEFT},
		{"top edge right of the corner", nil, pt(124, 52), w32.HTTOP},
		{"top-right corner", nil, pt(914, 52), w32.HTTOPRIGHT},
		{"bottom-left corner", nil, pt(101, 657), w32.HTBOTTOMLEFT},
		{"bottom-right along the bottom", nil, pt(893, 652), w32.HTBOTTOMRIGHT},
		{"bottom-right along the right", nil, pt(910, 635), w32.HTBOTTOMRIGHT},
		{"close light", nil, px(9, 9), w32.HTCLOSE},
		{"minimize light", nil, px(30, 15), w32.HTMINBUTTON},
		{"maximize light", nil, px(59.5, 19.5), w32.HTMAXBUTTON},
		{"between two lights: caption", nil, px(22, 12), w32.HTCAPTION},
		{"right of the lights", nil, px(60, 12), w32.HTCAPTION},
		{"title bar's right end", nil, px(399.5, 20), w32.HTCAPTION},
		{"below the title bar", nil, px(200, 28), w32.HTCLIENT},
		{"content", nil, px(200, 200), w32.HTCLIENT},
		{"no regions", func(in hitInput) hitInput { in.regions = nil; return in }, px(9, 9), w32.HTCLIENT},
		{"border wins over a region", func(in hitInput) hitInput {
			in.regions = []shell.ChromeRegion{{Rect: driver.Rect{W: 1000, H: 1000}, Kind: shell.RegionCaption}}
			return in
		}, pt(500, 52), w32.HTTOP},
		{"unknown kind is client", func(in hitInput) hitInput {
			in.regions = []shell.ChromeRegion{{Rect: driver.Rect{W: 1000, H: 1000}, Kind: 200}}
			return in
		}, px(200, 200), w32.HTCLIENT},
		// No resize borders (fixed size or maximized): the band is caption
		// and the frame is HTNOWHERE.
		{"no resize: top edge", func(in hitInput) hitInput { in.resize = false; return in }, pt(500, 50), w32.HTCAPTION},
		{"no resize: corner light", func(in hitInput) hitInput { in.resize = false; return in }, px(8, 8), w32.HTCLOSE},
		{"no resize: frame", func(in hitInput) hitInput { in.resize = false; return in }, pt(100, 300), w32.HTNOWHERE},
		{"no resize: bottom frame", func(in hitInput) hitInput { in.resize = false; return in }, pt(500, 655), w32.HTNOWHERE},
		{"no resize: content", func(in hitInput) hitInput { in.resize = false; return in }, px(200, 200), w32.HTCLIENT},
		{"maximized: frame clipped away", func(in hitInput) hitInput {
			in.resize = false
			in.window = w32.Rect{Left: -8, Top: -8, Right: 1928, Bottom: 1048}
			in.client = w32.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
			return in
		}, pt(-3, 500), w32.HTNOWHERE},
		{"maximized: top row is caption", func(in hitInput) hitInput {
			in.resize = false
			in.window = w32.Rect{Left: -8, Top: -8, Right: 1928, Bottom: 1048}
			in.client = w32.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
			return in
		}, pt(400, 0), w32.HTCAPTION},
		{"maximized: light at the screen corner", func(in hitInput) hitInput {
			in.resize = false
			in.window = w32.Rect{Left: -8, Top: -8, Right: 1928, Bottom: 1048}
			in.client = w32.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
			return in
		}, pt(17, 17), w32.HTCLOSE},
		{"scale 1.5", func(in hitInput) hitInput { in.scale = 1.5; return in },
			pt(cl.Left+12, cl.Top+12), w32.HTCLOSE},
		{"scale 1.5, just left of the light", func(in hitInput) hitInput { in.scale = 1.5; return in },
			pt(cl.Left+11, cl.Top+12), w32.HTCAPTION},
	}
	for _, c := range cases {
		in := base
		if c.in != nil {
			in = c.in(in)
		}
		in.pt = c.pt
		if got := hitTest(in); got != c.want {
			t.Errorf("%s: hitTest(%v) = %d, want %d", c.name, c.pt, got, c.want)
		}
	}
}

func TestClientRect(t *testing.T) {
	f := frame{side: 8, bottom: 8, band: 8}
	work := w32.Rect{Right: 1920, Bottom: 1040}
	cases := []struct {
		name      string
		wr        w32.Rect
		f         frame
		maximized bool
		want      w32.Rect
	}{
		{"restored", w32.Rect{Left: 100, Top: 50, Right: 916, Bottom: 658}, f, false,
			w32.Rect{Left: 108, Top: 50, Right: 908, Bottom: 650}},
		{"no frame", w32.Rect{Left: 100, Top: 50, Right: 500, Bottom: 350}, frame{}, false,
			w32.Rect{Left: 100, Top: 50, Right: 500, Bottom: 350}},
		{"maximized", w32.Rect{Left: -8, Top: -8, Right: 1928, Bottom: 1048}, f, true,
			w32.Rect{Right: 1920, Bottom: 1040}},
		{"maximized, clipped to the work area", w32.Rect{Left: -8, Top: -8, Right: 1928, Bottom: 1088}, f, true,
			w32.Rect{Right: 1920, Bottom: 1040}},
		{"maximized on a second monitor", w32.Rect{Left: 1912, Top: -8, Right: 3848, Bottom: 1048}, f, true,
			w32.Rect{Left: 1920, Right: 1920, Bottom: 1040}}, // outside work: empty
		{"smaller than the frame", w32.Rect{Left: 0, Top: 0, Right: 10, Bottom: 5}, f, false,
			w32.Rect{Left: 8, Top: 0, Right: 8, Bottom: 0}},
	}
	for _, c := range cases {
		if got := clientRect(c.wr, c.f, c.maximized, work); got != c.want {
			t.Errorf("%s: clientRect = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestFrameAttrsFor(t *testing.T) {
	cases := []struct {
		a    driver.Appearance
		want frameAttrs
	}{
		{driver.Appearance{}, frameAttrs{dark: 0, corner: w32.DWMWCP_ROUND}},
		{driver.Appearance{Dark: true}, frameAttrs{dark: 1, corner: w32.DWMWCP_ROUND}},
		{driver.Appearance{IncreaseContrast: true}, frameAttrs{corner: w32.DWMWCP_DEFAULT}},
	}
	for _, c := range cases {
		if got := frameAttrsFor(c.a); got != c.want {
			t.Errorf("frameAttrsFor(%+v) = %+v, want %+v", c.a, got, c.want)
		}
	}
}

func TestSignedXY(t *testing.T) {
	cases := []struct {
		lp   uintptr
		want w32.Point
	}{
		{0, w32.Point{}},
		{0x0002_0001, w32.Point{X: 1, Y: 2}},
		{0xFFFF_FFF8, w32.Point{X: -8, Y: -1}}, // a second monitor left of the primary
		{0xFFFF_FFFF_8000_7FFF, w32.Point{X: 32767, Y: -32768}},
	}
	for _, c := range cases {
		if got := signedXY(c.lp); got != c.want {
			t.Errorf("signedXY(%#x) = %v, want %v", c.lp, got, c.want)
		}
	}
}

// lparamXY packs screen or client pixels into a mouse lParam.
func lparamXY(p w32.Point) uintptr { return uintptr(uint16(p.X)) | uintptr(uint16(p.Y))<<16 }

// screenOf returns the screen pixel of a drawable-area point of sf.
func screenOf(t *testing.T, sf *surface, x, y float64) w32.Point {
	t.Helper()
	cl, err := sf.screenClient()
	if err != nil {
		t.Fatal(err)
	}
	return w32.Point{X: cl.Left + shellkit.ToPx(x, sf.Scale()), Y: cl.Top + shellkit.ToPx(y, sf.Scale())}
}

// TestSurfaceNCCalcSize: on real windows, the client area is the window
// without the side and bottom frame (or the whole window without
// StyleResizable), at several sizes, and WM_NCCALCSIZE sent directly
// gives the same answer for both forms of lParam.
func TestSurfaceNCCalcSize(t *testing.T) {
	inShell(t, func(s *Shell) {
		for _, cfg := range []shell.SurfaceConfig{resizable,
			{Size: driver.Size{W: 300, H: 200}, Style: driver.StyleTitled | driver.StyleClosable}} {
			sf := newSurface(t, s, cfg, &recorder{})
			f := sf.frameAt(w32.GetDpiForWindow(sf.hwnd))
			if sf.resizable && (f.side <= 0 || f.bottom <= 0 || f.band != f.bottom) {
				t.Errorf("resizable frame %+v", f)
			}
			if !sf.resizable && f != (frame{}) {
				t.Errorf("fixed frame %+v", f)
			}
			for _, size := range []driver.Size{{W: 400, H: 300}, {W: 640.5, H: 480.25}, {W: 120, H: 90}, {W: 1000, H: 700}} {
				sf.SetSize(size)
				wr, _ := w32.GetWindowRect(sf.hwnd)
				cl, err := sf.screenClient()
				if err != nil {
					t.Fatal(err)
				}
				want := clientRect(wr, f, false, w32.Rect{})
				if cl != want {
					t.Errorf("%v: client %+v in window %+v, want %+v", size, cl, wr, want)
				}
				if w, h := clientPx(t, sf); w != shellkit.ToPx(size.W, sf.Scale()) || h != shellkit.ToPx(size.H, sf.Scale()) || sf.Size() != size {
					t.Errorf("%v: client %d × %d px, Size %v", size, w, h, sf.Size())
				}

				// Sent directly, both forms.
				var p w32.NCCalcSizeParams
				p.Rgrc[0] = wr
				var pin runtime.Pinner
				pin.Pin(&p)
				r := w32.SendMessage(sf.hwnd, w32.WM_NCCALCSIZE, 1, uintptr(unsafe.Pointer(&p)))
				rc := wr
				pin.Pin(&rc)
				w32.SendMessage(sf.hwnd, w32.WM_NCCALCSIZE, 0, uintptr(unsafe.Pointer(&rc)))
				pin.Unpin()
				rethrow()
				if r != 0 || p.Rgrc[0] != want || rc != want {
					t.Errorf("%v: WM_NCCALCSIZE = %d, %+v, %+v, want 0, %+v", size, r, p.Rgrc[0], rc, want)
				}
			}
			sf.Close()
		}
	})
}

// TestSurfaceNCHitTest: WM_NCHITTEST on real windows at several sizes
// answers the borders, the top band, the chrome regions and the content.
func TestSurfaceNCHitTest(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{})
		defer sf.Close()
		hit := func(p w32.Point) uintptr {
			r := w32.SendMessage(sf.hwnd, w32.WM_NCHITTEST, 0, lparamXY(p))
			rethrow()
			return r
		}
		if hit(screenOf(t, sf, 9, 9)) != w32.HTCLIENT {
			t.Error("a light without regions is not HTCLIENT")
		}
		sf.SetChromeRegions(barRegions)
		for _, size := range []driver.Size{{W: 400, H: 300}, {W: 777.5, H: 433.3}, {W: 150, H: 100}} {
			sf.SetSize(size)
			wr, _ := w32.GetWindowRect(sf.hwnd)
			f := sf.frameAt(w32.GetDpiForWindow(sf.hwnd))
			midX, midY := (wr.Left+wr.Right)/2, (wr.Top+wr.Bottom)/2
			cases := []struct {
				name string
				p    w32.Point
				want uintptr
			}{
				{"left border", w32.Point{X: wr.Left, Y: midY}, w32.HTLEFT},
				{"right border", w32.Point{X: wr.Right - 1, Y: midY}, w32.HTRIGHT},
				{"bottom border", w32.Point{X: midX, Y: wr.Bottom - 1}, w32.HTBOTTOM},
				{"top edge", w32.Point{X: midX, Y: wr.Top}, w32.HTTOP},
				{"top band's last row", w32.Point{X: midX, Y: wr.Top + f.band - 1}, w32.HTTOP},
				{"top-left", w32.Point{X: wr.Left, Y: wr.Top}, w32.HTTOPLEFT},
				{"bottom-right", w32.Point{X: wr.Right - 1, Y: wr.Bottom - 1}, w32.HTBOTTOMRIGHT},
				{"outside", w32.Point{X: wr.Right + 5, Y: midY}, w32.HTNOWHERE},
				{"close", screenOf(t, sf, 14, 14), w32.HTCLOSE},
				{"minimize", screenOf(t, sf, 34, 14), w32.HTMINBUTTON},
				{"maximize", screenOf(t, sf, 54, 14), w32.HTMAXBUTTON},
				{"caption", screenOf(t, sf, size.W/2, 20), w32.HTCAPTION},
				{"content", screenOf(t, sf, size.W/2, 60), w32.HTCLIENT},
			}
			for _, c := range cases {
				if got := hit(c.p); got != c.want {
					t.Errorf("%v %s: WM_NCHITTEST(%v) = %d, want %d (window %+v)", size, c.name, c.p, got, c.want, wr)
				}
			}
		}
		sf.SetChromeRegions(nil)
		if hit(screenOf(t, sf, 14, 14)) != w32.HTCLIENT {
			t.Error("regions not cleared")
		}

		// A fixed-size surface has no borders: its edge is the caption.
		fixed := newSurface(t, s, shell.SurfaceConfig{Size: driver.Size{W: 300, H: 200},
			Style: driver.StyleTitled | driver.StyleClosable}, &recorder{})
		defer fixed.Close()
		fixed.SetChromeRegions(barRegions[:2])
		wr, _ := w32.GetWindowRect(fixed.hwnd)
		for _, c := range []struct {
			p    w32.Point
			want uintptr
		}{
			{w32.Point{X: wr.Left, Y: wr.Top}, w32.HTCAPTION},
			{w32.Point{X: wr.Left, Y: wr.Bottom - 1}, w32.HTCLIENT},
			{screenOf(t, fixed, 14, 14), w32.HTCLOSE},
		} {
			got := w32.SendMessage(fixed.hwnd, w32.WM_NCHITTEST, 0, lparamXY(c.p))
			rethrow()
			if got != c.want {
				t.Errorf("fixed: WM_NCHITTEST(%v) = %d, want %d", c.p, got, c.want)
			}
		}
	})
}

// TestSurfaceDPIScaledSize: WM_GETDPISCALEDSIZE keeps the logical size and
// adds the frame at the new DPI.
func TestSurfaceDPIScaledSize(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{})
		defer sf.Close()
		for _, dpi := range []uint32{96, 120, 144, 192} {
			sz := w32.Size{Cx: 1, Cy: 1}
			var pin runtime.Pinner
			pin.Pin(&sz)
			r := w32.SendMessage(sf.hwnd, w32.WM_GETDPISCALEDSIZE, uintptr(dpi), uintptr(unsafe.Pointer(&sz)))
			pin.Unpin()
			rethrow()
			nw, nh := sf.nonClient(dpi)
			scale := float64(dpi) / 96
			if want := (w32.Size{Cx: shellkit.ToPx(400, scale) + nw, Cy: shellkit.ToPx(300, scale) + nh}); r != 1 || sz != want {
				t.Errorf("DPI %d: result %d, size %+v, want 1, %+v", dpi, r, sz, want)
			}
		}
	})
}

// TestSurfaceFrameAttrs: the DWM calls succeed on this host (the corner
// preference only on Windows 11) and follow the appearance.
func TestSurfaceFrameAttrs(t *testing.T) {
	inShell(t, func(s *Shell) {
		sf := newSurface(t, s, resizable, &recorder{})
		defer sf.Close()
		ext, dark, corner := sf.applyFrame()
		if ext != nil || dark != nil {
			t.Errorf("applyFrame: extend %v, dark %v", ext, dark)
		}
		t.Logf("corner preference: %v (fails on Windows 10)", corner)
		s.appearance.Dark = !s.appearance.Dark
		if _, dark, _ := sf.applyFrame(); dark != nil {
			t.Errorf("dark frame toggle: %v", dark)
		}
	})
}

// pointerEvents returns the recorded events, all PointerEvents, and
// clears them.
func pointerEvents(t *testing.T, r *recorder) []shell.PointerEvent {
	t.Helper()
	var pes []shell.PointerEvent
	for _, ev := range r.events {
		pe, ok := ev.(shell.PointerEvent)
		if !ok {
			t.Fatalf("unexpected event %T", ev)
		}
		pes = append(pes, pe)
	}
	r.events = nil
	return pes
}

// TestSurfaceNCPointer: non-client mouse messages over the lights become
// PointerEvents in surface coordinates; a light's press captures the mouse
// and suppresses the default action; the window is hidden, so the pointer
// is never really over it and every check for it ends in PointerLeave.
func TestSurfaceNCPointer(t *testing.T) {
	inShell(t, func(s *Shell) {
		r := &recorder{allow: true}
		sf := newSurface(t, s, resizable, r)
		defer sf.Close()
		sf.SetChromeRegions(barRegions)
		send := func(msg uint32, wp uintptr, p w32.Point) uintptr {
			res := w32.SendMessage(sf.hwnd, msg, wp, lparamXY(p))
			rethrow()
			return res
		}
		closeAt := screenOf(t, sf, 14, 14)
		at := driver.Point{X: 14, Y: 14}

		send(w32.WM_NCMOUSEMOVE, w32.HTCLOSE, closeAt)
		pes := pointerEvents(t, r)
		if len(pes) != 2 || pes[0].Kind != shell.PointerEnter || pes[1].Kind != shell.PointerMove || pes[1].Pos != at {
			t.Fatalf("hover: %+v", pes)
		}
		send(w32.WM_NCMOUSEMOVE, w32.HTCAPTION, screenOf(t, sf, 200, 14))
		if pes := pointerEvents(t, r); len(pes) != 1 || pes[0].Kind != shell.PointerMove || pes[0].Pos != (driver.Point{X: 200, Y: 14}) {
			t.Errorf("caption hover: %+v", pes)
		}

		// Press, move with the capture, release: no SC_CLOSE (the window
		// stays open, CloseRequested is not asked).
		send(w32.WM_NCMOUSEMOVE, w32.HTCLOSE, closeAt)
		pointerEvents(t, r)
		if res := send(w32.WM_NCLBUTTONDOWN, w32.HTCLOSE, closeAt); res != 0 {
			t.Errorf("WM_NCLBUTTONDOWN = %d", res)
		}
		if w32.GetCapture() != sf.hwnd {
			t.Error("press did not capture the mouse")
		}
		w32.SendMessage(sf.hwnd, w32.WM_MOUSEMOVE, w32.MK_LBUTTON, lparamXY(w32.Point{X: -4, Y: 2}))
		rethrow()
		send(w32.WM_NCLBUTTONUP, w32.HTCLOSE, closeAt) // swallowed
		w32.SendMessage(sf.hwnd, w32.WM_LBUTTONUP, 0, lparamXY(w32.Point{X: shellkit.ToPx(14, sf.Scale()), Y: shellkit.ToPx(14, sf.Scale())}))
		rethrow()
		pes = pointerEvents(t, r)
		if len(pes) != 4 {
			t.Fatalf("press: %+v", pes)
		}
		down, move, up, leave := pes[0], pes[1], pes[2], pes[3]
		if down.Kind != shell.PointerDown || down.Button != shell.ButtonPrimary || down.Buttons != shell.ButtonPrimary ||
			down.Clicks != 1 || down.Pos != at {
			t.Errorf("down %+v", down)
		}
		if move.Kind != shell.PointerMove || move.Buttons != shell.ButtonPrimary || move.Pos.X >= 0 {
			t.Errorf("captured move %+v", move)
		}
		if up.Kind != shell.PointerUp || up.Button != shell.ButtonPrimary || up.Buttons != 0 || up.Clicks != 1 || up.Pos != at {
			t.Errorf("up %+v", up)
		}
		if leave.Kind != shell.PointerLeave {
			t.Errorf("after the press %+v, want PointerLeave (hidden window)", leave)
		}
		if w32.GetCapture() == sf.hwnd || r.requests != 0 || sf.dead {
			t.Errorf("after the press: capture %v, %d close requests, dead %v", w32.GetCapture() == sf.hwnd, r.requests, sf.dead)
		}

		// A double click on a light counts 2; losing the capture ends it.
		send(w32.WM_NCLBUTTONDBLCLK, w32.HTMAXBUTTON, screenOf(t, sf, 54, 14))
		if err := w32.ReleaseCapture(); err != nil {
			t.Fatal(err)
		}
		rethrow()
		pes = pointerEvents(t, r)
		if len(pes) != 4 || pes[0].Kind != shell.PointerEnter || pes[1].Kind != shell.PointerDown || pes[1].Clicks != 2 ||
			pes[2].Kind != shell.PointerUp || pes[2].Clicks != 2 || pes[3].Kind != shell.PointerLeave || sf.Zoomed() {
			t.Errorf("double click, capture lost: %+v, zoomed %v", pes, sf.Zoomed())
		}

		// A press on the caption is not delivered (DefWindowProcW moves the
		// window; on a hidden window it returns at once), nor is one over a
		// border; a move over the border leaves.
		send(w32.WM_NCMOUSEMOVE, w32.HTCAPTION, screenOf(t, sf, 200, 14))
		pointerEvents(t, r)
		wr, _ := w32.GetWindowRect(sf.hwnd)
		send(w32.WM_NCMOUSEMOVE, w32.HTLEFT, w32.Point{X: wr.Left, Y: wr.Top + 100})
		if pes := pointerEvents(t, r); len(pes) != 1 || pes[0].Kind != shell.PointerLeave {
			t.Errorf("move onto the border: %+v", pes)
		}
		send(w32.WM_NCMOUSELEAVE, 0, w32.Point{})
		if pes := pointerEvents(t, r); len(pes) != 0 {
			t.Errorf("second leave: %+v", pes)
		}

		// A handler that closes the surface in PointerDown.
		r2 := &recorder{}
		sf2 := newSurface(t, s, resizable, r2)
		sf2.SetChromeRegions(barRegions)
		closer := &closingRecorder{recorder: r2, sf: sf2}
		sf2.h = closer
		w32.SendMessage(sf2.hwnd, w32.WM_NCLBUTTONDOWN, w32.HTCLOSE, lparamXY(screenOf(t, sf2, 14, 14)))
		rethrow()
		if !sf2.dead || r2.closed != 1 || w32.GetCapture() != 0 {
			t.Errorf("close in PointerDown: dead %v, closed %d, capture %v", sf2.dead, r2.closed, w32.GetCapture())
		}
	})
}

// closingRecorder closes its surface on PointerDown, as the aqua close
// button does on PointerUp.
type closingRecorder struct {
	*recorder
	sf *surface
}

func (c *closingRecorder) Event(ev shell.Event) bool {
	c.recorder.Event(ev)
	if pe, ok := ev.(shell.PointerEvent); ok && pe.Kind == shell.PointerDown {
		c.sf.Close()
	}
	return true
}
