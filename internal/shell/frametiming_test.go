package shell

import (
	"reflect"
	"testing"
	"time"
)

// TestFrameTimer: a frame's record collects its frame function and its
// paints and is reported at End, or after the paints Expect announced; a
// record whose draws never came is reported at the next Begin; a paint
// outside a frame is reported on its own; without an observer nothing is
// reported.
func TestFrameTimer(t *testing.T) {
	var got []FrameTiming
	var ft FrameTimer
	ft.Begin(1)
	ft.Painted(1, 1, 1)
	ft.End()
	if ft.Active() {
		t.Fatal("active without an observer")
	}
	ft.SetObserver(func(x FrameTiming) { got = append(got, x) })
	if !ft.Active() {
		t.Fatal("not active with an observer")
	}
	ms := time.Millisecond

	// Win32 and the fake: the paints inside the frame.
	ft.Begin(16 * ms)
	ft.FrameFunc(1 * ms)
	ft.Painted(2*ms, 3*ms, 4*ms)
	ft.Painted(5*ms, 6*ms, 0)
	if len(got) != 0 {
		t.Fatalf("reported before End: %+v", got)
	}
	ft.End()
	want := []FrameTiming{{Time: 16 * ms, FrameFunc: 1 * ms, Paint: 7 * ms, Submit: 9 * ms, Present: 4 * ms, Surfaces: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frame with two paints: %+v, want %+v", got, want)
	}

	// A frame without paints, and a paint outside a frame.
	got = nil
	ft.Begin(32 * ms)
	ft.End()
	ft.Painted(1*ms, 2*ms, 3*ms)
	want = []FrameTiming{{Time: 32 * ms}, {Time: -1, Paint: 1 * ms, Submit: 2 * ms, Present: 3 * ms, Surfaces: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("empty frame and a lone paint: %+v, want %+v", got, want)
	}

	// gtk4: two draws queued, reported after the second.
	got = nil
	ft.Begin(48 * ms)
	ft.FrameFunc(1 * ms)
	ft.Expect(1)
	ft.Expect(1)
	ft.End()
	ft.Painted(2*ms, 1*ms, 0)
	if len(got) != 0 {
		t.Fatalf("reported before the last draw: %+v", got)
	}
	ft.Painted(2*ms, 1*ms, 0)
	ft.Painted(1*ms, 0, 0) // a redraw of GTK's own, after the frame
	want = []FrameTiming{
		{Time: 48 * ms, FrameFunc: 1 * ms, Paint: 4 * ms, Submit: 2 * ms, Surfaces: 2},
		{Time: -1, Paint: 1 * ms, Surfaces: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queued draws: %+v, want %+v", got, want)
	}

	// A draw that never came: the record goes out at the next Begin.
	got = nil
	ft.Begin(64 * ms)
	ft.Expect(1)
	ft.End()
	ft.Begin(80 * ms)
	ft.End()
	want = []FrameTiming{{Time: 64 * ms}, {Time: 80 * ms}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("a missing draw: %+v, want %+v", got, want)
	}

	// Removing the observer drops an open record.
	got = nil
	ft.Begin(96 * ms)
	ft.SetObserver(nil)
	ft.End()
	ft.Painted(1, 1, 1)
	if len(got) != 0 || ft.Active() {
		t.Fatalf("after SetObserver(nil): %+v, active %v", got, ft.Active())
	}
}
