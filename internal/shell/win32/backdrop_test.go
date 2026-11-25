//go:build windows

package win32

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// dwmLog is a fake backdropSys recording the DWM calls; fail makes the
// named call ("corner", "backdrop", "extend") fail.
type dwmLog struct {
	calls []string
	fail  map[string]bool
}

var errDWM = errors.New("fake DWM failure")

func (l *dwmLog) sys(build uint32) backdropSys {
	return backdropSys{
		build: build,
		set: func(_ windows.HWND, attr uint32, v int32) error {
			name := map[uint32]string{w32.DWMWA_WINDOW_CORNER_PREFERENCE: "corner", w32.DWMWA_SYSTEMBACKDROP_TYPE: "backdrop", w32.DWMWA_USE_IMMERSIVE_DARK_MODE: "dark"}[attr]
			l.calls = append(l.calls, fmt.Sprintf("%s=%d", name, v))
			if l.fail[name] {
				return errDWM
			}
			return nil
		},
		extend: func(_ windows.HWND, m *w32.Margins) error {
			l.calls = append(l.calls, fmt.Sprintf("extend=%d", m.CxLeftWidth))
			if l.fail["extend"] {
				return errDWM
			}
			return nil
		},
	}
}

// TestPopupBackdrop: the decision over the seam, both branches whatever
// the host runs: the build, high contrast, transparency effects and the
// calls' results.
func TestPopupBackdrop(t *testing.T) {
	const (
		w10  = 19045
		w11a = 22000 // 21H2: corners, no backdrop attribute
		w11  = acrylicBuild
	)
	hc := driver.Appearance{IncreaseContrast: true}
	rt := driver.Appearance{ReduceTransparency: true}
	tests := []struct {
		name        string
		build       uint32
		a           driver.Appearance
		fail        string
		translucent bool
		calls       string
		corner, bd  bool // errors expected
	}{
		{"windows 10", w10, driver.Appearance{}, "corner", false, "[corner=3 dark=0]", true, false},
		{"windows 11 21H2", w11a, driver.Appearance{}, "", false, "[corner=3 dark=0]", false, false},
		{"acrylic", w11, driver.Appearance{}, "", true, "[corner=3 dark=0 extend=-1 backdrop=3]", false, false},
		{"acrylic, later build", 26200, driver.Appearance{}, "", true, "[corner=3 dark=0 extend=-1 backdrop=3]", false, false},
		{"acrylic, dark", w11, driver.Appearance{Dark: true}, "", true, "[corner=3 dark=1 extend=-1 backdrop=3]", false, false},
		{"high contrast", w11, hc, "", false, "[corner=0 dark=0 backdrop=1 extend=0]", false, false},
		{"transparency off", w11, rt, "", false, "[corner=3 dark=0 backdrop=1 extend=0]", false, false},
		{"backdrop fails", w11, driver.Appearance{}, "backdrop", false, "[corner=3 dark=0 extend=-1 backdrop=3 backdrop=1 extend=0]", false, true},
		{"extend fails", w11, driver.Appearance{}, "extend", false, "[corner=3 dark=0 extend=-1 backdrop=1 extend=0]", false, true},
		{"corner fails only", w11, driver.Appearance{}, "corner", true, "[corner=3 dark=0 extend=-1 backdrop=3]", true, false},
		{"opaque, reset fails", w11, rt, "backdrop", false, "[corner=3 dark=0 backdrop=1 extend=0]", false, true},
	}
	for _, tt := range tests {
		l := &dwmLog{fail: map[string]bool{tt.fail: tt.fail != ""}}
		p := &popup{swapTarget: swapTarget{s: &Shell{backdrop: l.sys(tt.build), appearance: tt.a}}}
		p.translucent = !tt.translucent // must be overwritten
		corner, bd := p.applyBackdrop()
		if p.translucent != tt.translucent || fmt.Sprint(l.calls) != tt.calls ||
			(corner != nil) != tt.corner || (bd != nil) != tt.bd {
			t.Errorf("%s: translucent %v, calls %v, errors %v, %v; want %v, %s, errors %v, %v",
				tt.name, p.translucent, l.calls, corner, bd, tt.translucent, tt.calls, tt.corner, tt.bd)
		}
		if got := wantsAcrylic(tt.build, tt.a); got != (tt.build >= acrylicBuild && !tt.a.IncreaseContrast && !tt.a.ReduceTransparency) {
			t.Errorf("%s: wantsAcrylic = %v", tt.name, got)
		}
	}
}

// overrideSys is the live sysReader with high contrast and the
// transparency setting replaced.
type overrideSys struct {
	liveSys
	hc, noTransparency bool
}

func (o *overrideSys) highContrast() bool { return o.hc }
func (o *overrideSys) regDWORD(path, name string) (uint32, bool) {
	if path == regPersonalize && name == "EnableTransparency" {
		if o.noTransparency {
			return 0, true
		}
		return 1, true
	}
	return o.liveSys.regDWORD(path, name)
}

// TestShownPopupBackdrop: on the host's real DWM, the branch its Windows
// decides (Acrylic from build 22621 on, the attributes accepted), and
// WM_SETTINGCHANGE turning it off and on again for high contrast and
// transparency effects, each time before the popup's repaint.
func TestShownPopupBackdrop(t *testing.T) {
	sys := &overrideSys{}
	s := newShell(sys)
	s.q.SetChecks(true)
	s.debug = true
	build := s.backdrop.build
	t.Logf("host build %d: Acrylic %v", build, build >= acrylicBuild)
	err := run(t, s, &handler{ready: func() {
		defer s.Quit()
		sf := shownParent(t, s, &recorder{})
		var r popRec
		p := newPopup(t, s, sf, shell.PopupConfig{Size: driver.Size{W: 120, H: 80}, Grab: true}, &r)
		want := build >= acrylicBuild
		if p.Translucent() != want {
			t.Errorf("Translucent() = %v on build %d, want %v", p.Translucent(), build, want)
		}
		corner, bd := p.applyBackdrop()
		if bd != nil || (build >= 22000 && corner != nil) || p.Translucent() != want {
			t.Errorf("applyBackdrop on build %d = %v, %v, translucent %v", build, corner, bd, p.Translucent())
		}
		settings := func(hc, noTr bool, wantTr bool) {
			t.Helper()
			sys.hc, sys.noTransparency = hc, noTr
			p.invalid = false
			w32.SendMessage(s.listener, w32.WM_SETTINGCHANGE, 0, 0)
			rethrow()
			if p.Translucent() != wantTr || !p.invalid {
				t.Errorf("high contrast %v, transparency off %v: Translucent() = %v, invalid %v; want %v, invalid",
					hc, noTr, p.Translucent(), p.invalid, wantTr)
			}
		}
		settings(false, true, false)
		settings(false, false, want)
		settings(true, false, false)
		settings(false, false, want)
		p.Close()
		sf.Close()
	}})
	if err != nil {
		t.Fatal(err)
	}
}
