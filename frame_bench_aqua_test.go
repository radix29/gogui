package gogui

import (
	"fmt"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/aqua"
	"github.com/radix29/gogui/internal/aqua/gallery"
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	fakeshell "github.com/radix29/gogui/internal/shell/fake"
	"github.com/radix29/gogui/internal/shell/native"
)

// The frame benchmarks (PLAN.md S6.2c). A scene is one of the six
// examples or the golden scene, shown on the aqua driver and left as a user
// leaves it after opening it (the controls example's busy bar and the
// two indeterminate indicators of the gallery example and of the golden
// scene loop; the hello, form,
// converter and notes examples have a focused field or text view, whose
// caret blinks until its timeout).
//
// BenchmarkFrame runs on the fake shell, painting into its recording
// canvas (pure Go, the host's numbers for the aqua side alone), at 1× and
// 2×, light and dark: "steady" is a frame where nothing changed but an
// invalidation of the window (the frame function, the paint, the
// recorder's checks), "layout" the same frame after a layout pass of the
// whole window (gogui's, so not for the gallery, which has fixed frames).
// BenchmarkFrameNative runs the same scenes on the OS's own shell (Win32
// with Direct2D, GTK 4 with GSK), at the screen's scale, light and dark:
// one op is one presented frame on a shown window, paced by the display,
// so ns/op is the refresh period; the frame clock's observer gives the
// CPU work per frame: work-ns (the frame function, the paint and the
// submit, D2D's EndDraw on Win32), paint-ns, submit-ns, present-ns (not
// in work) and max-work-ns. TestFrameAllocs guards the allocations of a
// steady frame on the fake shell.

// frameScene is a benchmark scene: open shows it on app and returns its
// window and, for an example, the gogui Window (nil for the gallery).
type frameScene struct {
	name string
	app  string // AppConfig.Name
	open func(tb testing.TB, app *App) (*aqua.Window, *Window)
}

// frameScenes are the examples and the fixed golden-image scene of
// internal/aqua/gallery ("golden"; the gallery example lays out the same
// controls with stacks).
var frameScenes = func() []frameScene {
	var sc []frameScene
	for _, e := range examples {
		sc = append(sc, frameScene{e.name, e.app, func(tb testing.TB, app *App) (*aqua.Window, *Window) {
			w := e.build(tb, app)
			return w.p.(*aqua.Window), w
		}})
	}
	return append(sc, frameScene{"golden", "Gallery", func(tb testing.TB, app *App) (*aqua.Window, *Window) {
		w, err := gallery.Open(app.drv, nopWindowHandler{})
		if err != nil {
			tb.Fatal(err)
		}
		return w.(*aqua.Window), nil
	}})
}()

// nopWindowHandler is the gallery window's driver.WindowHandler.
type nopWindowHandler struct{}

func (nopWindowHandler) ContentResized(driver.Size) {}
func (nopWindowHandler) ShouldClose() bool          { return true }
func (nopWindowHandler) Closed()                    {}
func (nopWindowHandler) KeyChanged(bool)            {}

// fakeFrame is one frame of a scene on the fake shell: the window
// invalidated and the clock moved on by one frame interval.
func fakeFrame(sf *fakeshell.Surface, clk *fakeshell.Clock) {
	sf.Invalidate()
	clk.Advance(fakeshell.FrameInterval)
}

// openFake opens sc on the fake shell at scale and in the dark appearance
// or not, runs a second of frames (the first paints and the scrollers'
// fades are over), checks that it paints cleanly and turns the
// UI-goroutine checks off.
func openFake(tb testing.TB, app *App, sh *fakeshell.Shell, sc frameScene, scale float64, dark bool) (*fakeshell.Surface, *Window) {
	tb.Helper()
	aw, win := sc.open(tb, app)
	sf := aw.Surface().(*fakeshell.Surface)
	sf.SetScale(scale)
	if dark {
		app.SetAppearance(AppearanceDark)
	}
	drain(app)
	clk := sh.FakeClock()
	clk.Advance(time.Second)
	fakeFrame(sf, clk)
	if err := sf.PaintErr(); err != nil {
		tb.Fatalf("%s: %v", sc.name, err)
	}
	// As in an app: no UI-goroutine checks, neither gogui's and aqua's
	// (the queue's) nor the fake shell's, which would be most of the time.
	app.drv.Queue().SetChecks(false)
	sh.SetChecks(false)
	return sf, win
}

func appearanceName(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// BenchmarkFrame: the frame of every scene on the fake shell's recording
// canvas, at 1× and 2×, light and dark, steady and after a layout pass.
func BenchmarkFrame(b *testing.B) {
	for _, sc := range frameScenes {
		for _, scale := range []float64{1, 2} {
			for _, dark := range []bool{false, true} {
				name := fmt.Sprintf("%s/%gx/%s", sc.name, scale, appearanceName(dark))
				b.Run(name+"/steady", func(b *testing.B) {
					runAquaNamed(b, sc.app, func(app *App, sh *fakeshell.Shell) {
						sf, _ := openFake(b, app, sh, sc, scale, dark)
						clk := sh.FakeClock()
						b.ReportAllocs()
						for b.Loop() {
							fakeFrame(sf, clk)
						}
					})
				})
				b.Run(name+"/layout", func(b *testing.B) {
					runAquaNamed(b, sc.app, func(app *App, sh *fakeshell.Shell) {
						sf, win := openFake(b, app, sh, sc, scale, dark)
						if win == nil {
							b.Skip("fixed frames, no layout pass")
						}
						clk := sh.FakeClock()
						b.ReportAllocs()
						for b.Loop() {
							win.layout()
							fakeFrame(sf, clk)
						}
					})
				})
			}
		}
	}
}

// frameAllocBudget is the most allocations a steady frame of each scene
// may make on the fake shell (1×, light), about a quarter above what was
// measured in W15 (review P16: paths built in Paint are memoised per
// element, normal paints and strokes are not copied, the recorder keeps
// its paths in one arena), as a regression guard. What is left is the
// fake shell's recorder (a new record.Canvas per frame growing its op,
// segment and state slices) and mainthread.goid; the aqua
// side of a steady gallery frame allocates nothing. S6.2e measured hello
// 18, form 31, converter 19, controls 61, notes 31, gallery 134, golden
// 122; W13 12, 23, 12, 39, 15, 61, 61.
var frameAllocBudget = map[string]float64{
	"hello":     15, // 12 measured
	"form":      21, // 17
	"converter": 15, // 12
	"controls":  25, // 20
	"notes":     19, // 15
	"gallery":   26, // 21
	"golden":    26, // 21 (the fixed scene of internal/aqua/gallery)
}

// TestFrameAllocs: a steady frame of every scene allocates within its
// budget (S6.2c).
func TestFrameAllocs(t *testing.T) {
	for _, sc := range frameScenes {
		t.Run(sc.name, func(t *testing.T) {
			runAquaNamed(t, sc.app, func(app *App, sh *fakeshell.Shell) {
				sf, _ := openFake(t, app, sh, sc, 1, false)
				clk := sh.FakeClock()
				paints := sf.Paints()
				n := testing.AllocsPerRun(50, func() { fakeFrame(sf, clk) })
				if p := sf.Paints() - paints; p != 51 {
					t.Fatalf("%s: %d paints in 51 frames", sc.name, p)
				}
				t.Logf("%s: %.0f allocations per steady frame", sc.name, n)
				if budget := frameAllocBudget[sc.name]; n > budget {
					t.Errorf("%s: %.0f allocations per steady frame, budget %.0f", sc.name, n, budget)
				}
			})
		})
	}
}

// nativeFrames sums the observer's records of the counted frames.
type nativeFrames struct {
	n                                  int
	work, paint, submit, present, most time.Duration
}

func (f *nativeFrames) add(ft shell.FrameTiming) {
	w := ft.FrameFunc + ft.Paint + ft.Submit
	f.n++
	f.work += w
	f.paint += ft.Paint
	f.submit += ft.Submit
	f.present += ft.Present
	f.most = max(f.most, w)
}

func (f *nativeFrames) report(b *testing.B) {
	per := func(d time.Duration) float64 { return float64(d) / float64(max(f.n, 1)) }
	b.ReportMetric(per(f.work), "work-ns/frame")
	b.ReportMetric(per(f.paint), "paint-ns/frame")
	b.ReportMetric(per(f.submit), "submit-ns/frame")
	b.ReportMetric(per(f.present), "present-ns/frame")
	b.ReportMetric(float64(f.most), "max-work-ns")
}

// nativeWarmup is the number of painted frames run before the counted
// ones (the first paints fill the text and image caches).
const nativeWarmup = 10

// BenchmarkFrameNative: the frame of every scene on the OS's own shell,
// light and dark, timed by the frame clock's observer. It skips where the
// OS has no shell or no display.
func BenchmarkFrameNative(b *testing.B) {
	for _, sc := range frameScenes {
		for _, dark := range []bool{false, true} {
			b.Run(sc.name+"/"+appearanceName(dark), func(b *testing.B) {
				sh, err := native.New()
				if err != nil {
					b.Skip(err)
				}
				d := aqua.New(sh)
				setDriver(b, d)
				debugChecks = false // as in an app
				var f nativeFrames
				err = runTestNamed(b, sc.app, func(app *App) {
					if dark {
						app.SetAppearance(AppearanceDark)
					}
					aw, _ := sc.open(b, app)
					sf := aw.Surface()
					b.Logf("%s at scale %g", sc.name, sf.Scale())
					seen := 0
					d.Clock().SetFrameObserver(func(ft shell.FrameTiming) {
						if ft.Time < 0 || ft.Surfaces == 0 {
							return // a paint outside a frame, a frame without one
						}
						seen++
						switch {
						case seen == nativeWarmup:
							b.ResetTimer()
						case seen > nativeWarmup:
							f.add(ft)
						}
						if f.n >= b.N {
							b.StopTimer()
							Do(app.Quit)
							return
						}
						Do(sf.Invalidate)
					})
					sf.Invalidate()
				})
				if err != nil {
					b.Skip(err) // no display
				}
				f.report(b)
			})
		}
	}
}
