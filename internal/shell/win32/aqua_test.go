//go:build windows

package win32

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
	"golang.org/x/sys/windows"
)

// The S2.4d_3 tests: an aqua window (the title bar with its traffic lights
// and a push button in the content) on a real surface, painted through the
// D2D canvas and read back from the back buffer between EndDraw and
// Present, in light and dark, key and inactive, at scale 1 and 2. With
// GOGUI_SCREENSHOTS set to a directory, every frame checked is also
// written there as a PNG.

// winHandler is a driver.WindowHandler that does nothing.
type winHandler struct{}

func (winHandler) ContentResized(driver.Size) {}
func (winHandler) ShouldClose() bool          { return true }
func (winHandler) Closed()                    {}
func (winHandler) KeyChanged(bool)            {}

// okButton is a push button's handler.
type okButton struct{}

func (okButton) Clicked() {}

// framePx paints sf through WM_PAINT and returns the whole frame, read
// back before Present. The frame must draw without rendering or recorder
// errors.
func framePx(t *testing.T, win window) (w, h int, pix []byte) {
	t.Helper()
	sf := win.swap()
	drawn := false
	sf.s.hooks.onDrawn = func(d *swapTarget) {
		if d == sf {
			w, h, pix = readAll(t, sf)
			drawn = true
		}
	}
	defer func() { sf.s.hooks.onDrawn = nil }()
	w32.SendMessage(sf.hwnd, w32.WM_PAINT, 0, 0)
	rethrow()
	if !drawn || sf.renderErr != nil || sf.paintErr != nil {
		t.Fatalf("not drawn (%v): render error %v, paint error %v", drawn, sf.renderErr, sf.paintErr)
	}
	return w, h, pix
}

// savePNG writes BGRA pixels (premultiplied, as read back) to
// $GOGUI_SCREENSHOTS/name.png if the variable is set.
func savePNG(t *testing.T, name string, w, h int, bgra []byte) {
	t.Helper()
	dir := os.Getenv("GOGUI_SCREENSHOTS")
	if dir == "" {
		return
	}
	writePNG(t, filepath.Join(dir, name+".png"), w, h, bgra)
}

// writePNG writes BGRA pixels to path as an RGBA PNG.
func writePNG(t *testing.T, path string, w, h int, bgra []byte) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h)) // premultiplied, like the buffer
	for i := 0; i < w*h*4; i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bgra[i+2], bgra[i+1], bgra[i], bgra[i+3]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

// framePxAt returns the BGRA pixel under the logical point (x, y) at scale.
func framePxAt(w, h int, pix []byte, scale, x, y float64) [4]byte {
	px, py := min(int(x*scale), w-1), min(int(y*scale), h-1)
	o := (py*w + px) * 4
	return [4]byte(pix[o : o+4])
}

func rgb(r, g, b byte) [4]byte { return [4]byte{b, g, r, 255} }

// nearPx reports whether two BGRA pixels differ by at most tol per channel.
func nearPx(a, b [4]byte, tol int) bool {
	for c := range 4 {
		if d := int(a[c]) - int(b[c]); d < -tol || d > tol {
			return false
		}
	}
	return true
}

// setDPI sends a synthetic WM_DPICHANGED to dpi that keeps the logical
// size (as TestSurfaceDPIChanged).
func setDPI(t testing.TB, sf *surface, dpi uint32) {
	t.Helper()
	size := sf.Size()
	wr, err := w32.GetWindowRect(sf.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	nw, nh := sf.nonClient(w32.GetDpiForWindow(sf.hwnd))
	scale := float64(dpi) / 96
	rc := w32.Rect{Left: wr.Left, Top: wr.Top,
		Right: wr.Left + shellkit.ToPx(size.W, scale) + nw, Bottom: wr.Top + shellkit.ToPx(size.H, scale) + nh}
	var pin runtime.Pinner // rc is read by the window procedure through ffi.At
	pin.Pin(&rc)
	defer pin.Unpin()
	w32.SendMessage(sf.hwnd, w32.WM_DPICHANGED, uintptr(dpi)<<16|uintptr(dpi), uintptr(unsafe.Pointer(&rc)))
	rethrow()
	if sf.Scale() != scale || sf.bitmapDPI != dpi {
		t.Fatalf("WM_DPICHANGED to %d: scale %v, bitmap DPI %d", dpi, sf.Scale(), sf.bitmapDPI)
	}
}

// TestSurfaceAquaWindow: an aqua window's title bar, a traffic light, the
// content background and a push button's bezel read back from the frame,
// key and inactive, light and dark, at scale 1 and 2, with no recorder
// errors; inShell checks that no references are left.
func TestSurfaceAquaWindow(t *testing.T) {
	inShell(t, func(s *Shell) {
		sv := shell.Services{Text: s.TextSystem(), Clock: s.Clock()}
		d := aqua.NewWithServices(s, sv) // only for the widget factory; never run
		win, err := aqua.NewWindow(s, sv, driver.WindowConfig{Title: "Hello", Size: driver.Size{W: 360, H: 180},
			Style: driver.StyleTitled | driver.StyleClosable | driver.StyleMiniaturizable | driver.StyleResizable},
			winHandler{})
		if err != nil {
			t.Fatal(err)
		}
		sf := win.Surface().(*surface)
		defer win.Close()
		// The real button (S3.3e): its 20 pt bezel is centred in the 30 pt
		// frame (content y 25 to 45) and its "OK" in the middle, so the right
		// part of the 160 pt bezel is fill and its top row the 1 pt border.
		btn := d.NewButton(okButton{})
		btn.SetTitle("OK")
		btn.SetFrame(driver.Rect{X: 20, Y: 20, W: 160, H: 30})
		win.SetContent(btn)
		bx, by := 20+150.0, aqua.TitleBarHeight()+20+15.0

		for _, dpi := range []uint32{96, 192} {
			if sf.dpi != dpi {
				setDPI(t, sf, dpi)
			}
			scale := float64(dpi) / 96
			for _, dark := range []bool{false, true} {
				s.appearance.Dark = dark
				a := driver.DefaultAppearance()
				a.Dark = dark
				win.SetAppearance(a)
				for _, key := range []bool{false, true} {
					wa := uintptr(w32.WA_INACTIVE)
					if key {
						wa = w32.WA_ACTIVE
					}
					w32.SendMessage(sf.hwnd, w32.WM_ACTIVATE, wa, 0)
					rethrow()
					if win.IsKey() != key {
						t.Fatalf("IsKey %v after WM_ACTIVATE", win.IsKey())
					}
					name := fmt.Sprintf("aqua-%s-%s-x%g", map[bool]string{false: "light", true: "dark"}[dark],
						map[bool]string{false: "inactive", true: "key"}[key], scale)
					w, h, pix := framePx(t, sf)
					savePNG(t, name, w, h, pix)
					if ww, wh := int(shellkit.ToPx(360, scale)), int(shellkit.ToPx(180+aqua.TitleBarHeight(), scale)); w != ww || h != wh {
						t.Errorf("%s: frame %d×%d px, want %d×%d", name, w, h, ww, wh)
					}
					// The theme tokens (internal/aqua/theme/spec/sequoia.json);
					// the dark bezel is white at 64/255 over 50 grey.
					var bar, light, bg, fill [4]byte
					switch {
					case dark && key:
						bar, light, bg, fill = rgb(56, 56, 56), rgb(255, 95, 87), rgb(50, 50, 50), rgb(101, 101, 101)
					case dark:
						bar, light, bg, fill = rgb(43, 43, 43), rgb(84, 84, 84), rgb(50, 50, 50), rgb(101, 101, 101)
					case key:
						bar, light, bg, fill = rgb(229, 229, 229), rgb(255, 95, 87), rgb(236, 236, 236), rgb(255, 255, 255)
					default:
						bar, light, bg, fill = rgb(243, 243, 243), rgb(220, 220, 220), rgb(236, 236, 236), rgb(255, 255, 255)
					}
					for _, c := range []struct {
						what string
						x, y float64
						want [4]byte
					}{
						{"title bar", 300, 4, bar},
						{"title bar left edge", 2, 14, bar},
						{"between the lights", 24, 14, bar},
						{"close light", 14, 14, light},
						{"close light near its edge", 10, 14, light},
						{"content", 300, 150, bg},
						{"content corner", 359.5, 207.5, bg},
						{"bezel fill", bx, by, fill},
						{"bezel fill near its top", bx, by - 8, fill},
						{"above the bezel", bx, by - 12, bg},
						{"left of the bezel", 10, by, bg},
					} {
						if got := framePxAt(w, h, pix, scale, c.x, c.y); !nearPx(got, c.want, 2) {
							t.Errorf("%s: %s at (%g, %g) = BGRA %v, want %v", name, c.what, c.x, c.y, got, c.want)
						}
					}
				}
			}
		}
	})
}

// TestCaptureWindow is the PrintWindow cross-check of the screenshots
// (S2.4d_3): with GOGUI_CAPTURE=title=file.png it captures the client area
// of the visible gogui window of that title (in another process, such as a
// running examples/hello) with PW_RENDERFULLCONTENT and writes it as a
// PNG. It is skipped otherwise.
func TestCaptureWindow(t *testing.T) {
	spec := os.Getenv("GOGUI_CAPTURE")
	if spec == "" {
		t.Skip("GOGUI_CAPTURE not set")
	}
	i := strings.LastIndexByte(spec, '=')
	if i <= 0 || i == len(spec)-1 {
		t.Fatalf("GOGUI_CAPTURE=%q, want title=file.png", spec)
	}
	title, file := spec[:i], spec[i+1:]
	hwnd := w32.FindWindowEx(0, className, title)
	if hwnd == 0 {
		t.Fatalf("no %s window titled %q", className, title)
	}
	w, h, pix := printWindow(t, hwnd)
	writePNG(t, file, w, h, pix)
}

// printWindow captures hwnd's client area through PrintWindow into a
// top-down 32-bit DIB and returns its BGRA pixels with opaque alpha.
func printWindow(t *testing.T, hwnd windows.HWND) (w, h int, pix []byte) {
	t.Helper()
	rc, err := w32.GetClientRect(hwnd)
	if err != nil {
		t.Fatal(err)
	}
	w, h = int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	dc, err := w32.CreateCompatibleDC(0)
	if err != nil {
		t.Fatal(err)
	}
	defer w32.DeleteDC(dc)
	bi := w32.BitmapInfoHeader{BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32,
		BiCompression: w32.BI_RGB}
	bi.BiSize = uint32(unsafe.Sizeof(bi))
	bmp, bits, err := w32.CreateDIBSection(dc, &bi)
	if err != nil {
		t.Fatal(err)
	}
	defer w32.DeleteObject(bmp)
	old := w32.SelectObject(dc, bmp)
	defer w32.SelectObject(dc, old)
	if err := w32.PrintWindow(hwnd, dc, w32.PW_CLIENTONLY|w32.PW_RENDERFULLCONTENT); err != nil {
		t.Fatal(err)
	}
	w32.GdiFlush()
	pix = ffi.CopyBytes(bits, w*h*4)
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 255 // GDI leaves alpha undefined
	}
	return w, h, pix
}

// countButton counts a push button's clicks.
type countButton struct{ n *int }

func (b countButton) Clicked() { *b.n++ }

// TestSurfaceAquaFirstClick (S6.1c): an aqua window on a real surface, the
// press Windows activates it with (WM_MOUSEACTIVATE, WM_ACTIVATE, then the
// button messages) only activates it: the push button under it does not
// click; the next click does.
func TestSurfaceAquaFirstClick(t *testing.T) {
	inShell(t, func(s *Shell) {
		sv := shell.Services{Text: s.TextSystem(), Clock: s.Clock()}
		d := aqua.NewWithServices(s, sv) // only for the widget factory; never run
		win, err := aqua.NewWindow(s, sv, driver.WindowConfig{Title: "First click", Size: driver.Size{W: 360, H: 180},
			Style: driver.StyleTitled | driver.StyleClosable}, winHandler{})
		if err != nil {
			t.Fatal(err)
		}
		sf := win.Surface().(*surface)
		defer win.Close()
		clicks := 0
		btn := d.NewButton(countButton{&clicks})
		btn.SetTitle("OK")
		btn.SetFrame(driver.Rect{X: 20, Y: 20, W: 160, H: 30})
		win.SetContent(btn)
		at := pxAt(sf, 100, aqua.TitleBarHeight()+35)
		click := func() {
			sendTo(sf.hwnd, w32.WM_MOUSEMOVE, 0, at)
			sendTo(sf.hwnd, w32.WM_LBUTTONDOWN, w32.MK_LBUTTON, at)
			sendTo(sf.hwnd, w32.WM_LBUTTONUP, 0, at)
		}

		sendTo(sf.hwnd, w32.WM_ACTIVATE, w32.WA_INACTIVE, 0)
		sendTo(sf.hwnd, w32.WM_MOUSEACTIVATE, uintptr(sf.hwnd), w32.HTCLIENT|w32.WM_LBUTTONDOWN<<16)
		sendTo(sf.hwnd, w32.WM_ACTIVATE, w32.WA_CLICKACTIVE, 0)
		click()
		if !win.IsKey() || clicks != 0 {
			t.Fatalf("activating click: key %v, clicks %d, want key and 0", win.IsKey(), clicks)
		}
		click()
		if clicks != 1 {
			t.Errorf("second click: clicks %d, want 1", clicks)
		}
	})
}
