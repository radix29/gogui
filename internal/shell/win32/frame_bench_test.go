//go:build windows

package win32

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/gallery"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// BenchmarkGalleryFrame: a steady frame of the gallery over real Direct2D
// (PLAN.md S6.2c) at 96 and 192 DPI, light and dark: the driver's frame on
// the shell's clock (the two indeterminate indicators loop) and a
// WM_PAINT of the hidden, key window that draws the whole scene through
// the D2D canvas, timed to EndDraw as the TextView benchmarks
// (textBench.frame), on the default device (the GPU when there is one),
// with the recorder checks off as in an app. Each frame first waits,
// untimed, on the chain's frame-latency waitable, as the clock does:
// presented back to back, the gallery's frames block in the submit until
// the display takes the previous one (DXGI's frame latency; 14.7 ms per
// frame on the host, where the TextView benchmarks' frames did not
// block), and the benchmark would time the refresh period. Next to
// ns/op (b.StartTimer's clock, which moves every 0.5 ms on Windows) the
// frame clock's observer (QPC) gives work-ns, the frame function, the
// paint and the submit (setup, EndDraw), and paint-ns and submit-ns
// alone. The five examples over Direct2D, at the screen's scale, are
// gogui's BenchmarkFrameNative.
func BenchmarkGalleryFrame(b *testing.B) {
	for _, dpi := range []uint32{96, 192} {
		for _, dark := range []bool{false, true} {
			mode, name := driver.AppearanceLight, "light"
			if dark {
				mode, name = driver.AppearanceDark, "dark"
			}
			b.Run(fmt.Sprintf("%gx/%s", float64(dpi)/96, name), func(b *testing.B) {
				s := New()
				d := aqua.New(s)
				err := d.Run(driver.AppConfig{Name: "Gallery"}, galleryApp{ready: func() {
					defer d.Quit()
					w, err := d.NewWindow(driver.WindowConfig{Title: gallery.Title, Size: gallery.Size, Style: gallery.Style},
						winHandler{})
					if err != nil {
						b.Error(err)
						return
					}
					defer w.Close()
					gallery.Fill(d, w)
					sf := w.(*aqua.Window).Surface().(*surface)
					w32.SendMessage(sf.hwnd, w32.WM_ACTIVATE, w32.WA_ACTIVE, 0)
					rethrow()
					if sf.dpi != dpi {
						setDPI(b, sf, dpi)
					}
					d.SetAppearance(mode)
					tb := &textBench{b: b, s: s, sf: sf}
					tb.setupFrame()
					var work, paint, submit time.Duration
					on := false
					s.Clock().SetFrameObserver(func(ft shell.FrameTiming) {
						if on {
							work += ft.FrameFunc + ft.Paint + ft.Submit
							paint += ft.Paint
							submit += ft.Submit
						}
					})
					defer s.Clock().SetFrameObserver(nil)
					b.ReportAllocs()
					n := 0
					for b.Loop() {
						b.StopTimer()
						if sf.waitable != 0 {
							_, _ = windows.WaitForSingleObject(sf.waitable, 100)
						}
						on = true
						b.StartTimer()
						tb.frame()
						n++
					}
					per := func(d time.Duration) float64 { return float64(d) / float64(max(n, 1)) }
					b.ReportMetric(per(work), "work-ns/frame")
					b.ReportMetric(per(paint), "paint-ns/frame")
					b.ReportMetric(per(submit), "submit-ns/frame")
				}})
				if err != nil {
					b.Fatal(err)
				}
			})
		}
	}
}
