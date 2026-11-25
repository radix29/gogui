//go:build linux

package gtk4

import "testing"

// TestClipboardRoundTripGTK: over the real GdkClipboard of the default
// display, a text written with SetClipboardText reads back unchanged
// (line breaks and non-ASCII included) through the asynchronous read and
// the asyncReady trampoline, delivered on the UI thread; the read's handle
// is gone after Run. GDK answers a read of the process's own content
// locally, so no clipboard manager is needed (xvfb and headless Wayland
// alike).
func TestClipboardRoundTripGTK(t *testing.T) {
	s, reports := newGTKShell(t)
	base := dispatch.Len()
	const text = "gogui clip ✓\r\nline 2\n"
	var (
		got   string
		ok    bool
		calls int
	)
	p, err := runGTK(t, s, &handler{ready: func() {
		s.SetClipboardText(text)
		s.ReadClipboardText(func(tx string, k bool) {
			calls++
			if !s.q.OnUIThread() {
				t.Error("ReadClipboardText's fn off the UI thread")
			}
			got, ok = tx, k
			s.Quit()
		})
		if calls != 0 {
			t.Error("ReadClipboardText called fn inside the call")
		}
		if len(s.clip.pending) != 1 {
			t.Errorf("%d reads pending, want 1", len(s.clip.pending))
		}
	}})
	if p != nil || err != nil {
		t.Fatalf("Run = %v, panicked with %v", err, p)
	}
	t.Logf("CLIPBOARD-RESULT: read (%q, %v), %d call(s)", got, ok, calls)
	if calls != 1 || !ok || got != text {
		t.Errorf("read %d time(s): (%q, %v), want once (%q, true)", calls, got, ok, text)
	}
	checkGTKTornDown(t, s, reports, base)
}
