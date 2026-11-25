package gtk4

import (
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/ffi"
)

// fakeClipboard is the clipboardBackend of the host tests: it keeps the
// text, records the reads it started (their handles, as GTK keeps the
// user_data until it calls asyncReady) and finishes them from its fields.
type fakeClipboard struct {
	text     string
	has      bool // the clipboard holds text
	failRead bool // readText starts nothing
	sets     []string
	reads    []ffi.Handle // started, in order
	finishes int
}

// The source and result asyncReady passes on, checked by finishText.
const (
	fakeClipSource uintptr = 0xc11b
	fakeClipResult uintptr = 0x7e5
)

func (f *fakeClipboard) setText(text string) {
	f.sets = append(f.sets, text)
	f.text, f.has = text, true
}

func (f *fakeClipboard) readText(data ffi.Handle) bool {
	if f.failRead {
		return false
	}
	f.reads = append(f.reads, data)
	return true
}

func (f *fakeClipboard) finishText(source, result uintptr) (string, bool) {
	f.finishes++
	if source != fakeClipSource || result != fakeClipResult {
		return "", false
	}
	return f.text, f.has
}

// complete calls asyncReady for the oldest started read, as GTK's main
// loop does when the read is done.
func (f *fakeClipboard) complete() {
	h := f.reads[0]
	f.reads = f.reads[1:]
	asyncReady(fakeClipSource, fakeClipResult, uintptr(h))
}

type clipResult struct {
	text string
	ok   bool
}

// TestClipboardNoBackend: without a backend (no display) writes are
// dropped and a read fails later, from the queue, never inside the call.
func TestClipboardNoBackend(t *testing.T) {
	s, l := newTestShell(t)
	base := dispatch.Len()
	mustPanic(t, "ReadClipboardText before Run", func() { s.ReadClipboardText(func(string, bool) {}) })
	mustPanic(t, "SetClipboardText before Run", func() { s.SetClipboardText("x") })
	calls := 0
	err := run(t, s, &handler{ready: func() {
		mustPanic(t, "ReadClipboardText with a nil func", func() { s.ReadClipboardText(nil) })
		s.SetClipboardText("dropped")
		s.ReadClipboardText(func(text string, ok bool) {
			calls++
			if text != "" || ok {
				t.Errorf("read = (%q, %v), want (\"\", false)", text, ok)
			}
			s.Quit()
		})
		if calls != 0 {
			t.Error("ReadClipboardText called fn inside the call")
		}
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want once", calls)
	}
	checkTornDown(t, s, l, base)
}

// TestClipboardRoundTrip: a write reaches the backend cleaned; a read
// starts an asynchronous read, finishes it in asyncReady and delivers the
// text from the queue; every read handle is freed once delivered.
func TestClipboardRoundTrip(t *testing.T) {
	s, l := newTestShell(t)
	f := &fakeClipboard{}
	s.clip.backend = f
	base := dispatch.Len()
	var got []clipResult
	read := func(text string, ok bool) { got = append(got, clipResult{text, ok}) }
	err := run(t, s, &handler{ready: func() {
		// Empty clipboard: the read fails.
		s.ReadClipboardText(read)
		if len(f.reads) != 1 || dispatch.Len() != base+1 {
			t.Fatalf("%d reads started, %d handles, want 1 and %d", len(f.reads), dispatch.Len(), base+1)
		}
		s.Post(func() {
			f.complete()
			if len(got) != 0 {
				t.Error("the result was delivered inside asyncReady, not from the queue")
			}
			s.SetClipboardText("line 1\r\nline 2\x00rest")
			s.ReadClipboardText(read)
			s.ReadClipboardText(read) // two in flight
			s.Post(func() {
				f.complete()
				f.complete()
				s.Post(s.Quit)
			})
		})
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	want := []clipResult{{"", false}, {"line 1\r\nline 2", true}, {"line 1\r\nline 2", true}}
	if !slices.Equal(got, want) {
		t.Errorf("reads = %+v, want %+v", got, want)
	}
	if !slices.Equal(f.sets, []string{"line 1\r\nline 2"}) {
		t.Errorf("sets = %q", f.sets)
	}
	if f.finishes != 3 {
		t.Errorf("finishText called %d times, want 3", f.finishes)
	}
	checkTornDown(t, s, l, base)
}

// TestClipboardReadNotStarted: a read the backend cannot start fails from
// the queue and keeps no handle.
func TestClipboardReadNotStarted(t *testing.T) {
	s, l := newTestShell(t)
	f := &fakeClipboard{failRead: true, text: "x", has: true}
	s.clip.backend = f
	base := dispatch.Len()
	var got []clipResult
	err := run(t, s, &handler{ready: func() {
		s.ReadClipboardText(func(text string, ok bool) {
			got = append(got, clipResult{text, ok})
			s.Quit()
		})
		if len(s.clip.pending) != 0 { // and checkTornDown finds no handle left
			t.Errorf("%d reads pending after a read that did not start", len(s.clip.pending))
		}
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if !slices.Equal(got, []clipResult{{"", false}}) {
		t.Errorf("reads = %+v, want one failed", got)
	}
	checkTornDown(t, s, l, base)
}

// TestClipboardReadAfterQuit: a read pending at Quit is dropped: its
// handle goes at shutdown, and a result GTK delivers later finds a stale
// handle (no finish, no fn). A read asked for after Quit is dropped too.
func TestClipboardReadAfterQuit(t *testing.T) {
	s, l := newTestShell(t)
	f := &fakeClipboard{text: "late", has: true}
	s.clip.backend = f
	base := dispatch.Len()
	calls := 0
	err := run(t, s, &handler{ready: func() {
		s.ReadClipboardText(func(string, bool) { calls++ })
		s.Quit()
		s.ReadClipboardText(func(string, bool) { calls++ })
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	checkTornDown(t, s, l, base)
	if len(f.reads) != 1 {
		t.Fatalf("%d reads started, want 1 (none after Quit)", len(f.reads))
	}
	f.complete() // GTK delivers after shutdown
	if calls != 0 || f.finishes != 0 {
		t.Errorf("a late result: fn called %d times, finishText %d, want 0 and 0", calls, f.finishes)
	}
	checkLen(t, base)
}

// TestClipboardFinishedAfterQuit: a result that arrives after Quit but
// before shutdown (in the final drain) is finished, so the native string
// is freed, but fn is not called.
func TestClipboardFinishedAfterQuit(t *testing.T) {
	s, l := newTestShell(t)
	f := &fakeClipboard{text: "late", has: true}
	s.clip.backend = f
	base := dispatch.Len()
	calls := 0
	err := run(t, s, &handler{ready: func() {
		s.ReadClipboardText(func(string, bool) { calls++ })
		s.Post(func() {
			s.Quit()
			f.complete()
		})
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if calls != 0 || f.finishes != 1 {
		t.Errorf("fn called %d times, finishText %d, want 0 and 1", calls, f.finishes)
	}
	checkTornDown(t, s, l, base)
}

// TestAsyncReady: the trampoline frees its handle after the peer, also
// when the peer panics (kept as the pending panic), and ignores a stale
// handle.
func TestAsyncReady(t *testing.T) {
	resetPanics(t)
	base := dispatch.Len()
	p := &asyncRec{}
	h := dispatch.New(entry{peer: p, kind: kindClipboardRead})
	asyncReady(1, 2, uintptr(h))
	if dispatch.Valid(h) {
		t.Error("asyncReady kept its handle")
	}
	asyncReady(1, 2, uintptr(h)) // stale
	asyncReady(1, 2, 0)
	if !slices.Equal(p.calls, [][2]uintptr{{1, 2}}) {
		t.Errorf("calls %v, want one with (1, 2)", p.calls)
	}

	p.panics = true
	h2 := dispatch.New(entry{peer: p, kind: kindClipboardRead})
	asyncReady(1, 2, uintptr(h2))
	if dispatch.Valid(h2) {
		dispatch.Free(h2)
		t.Error("a panicking peer kept its handle")
	}
	if pendingPanic == nil || pendingPanic.Value != "async" || pendingPanic.Callback != nameAsyncReady {
		t.Errorf("pendingPanic = %+v", pendingPanic)
	}
	checkLen(t, base)
}

type asyncRec struct {
	calls  [][2]uintptr
	panics bool
}

func (a *asyncRec) asyncReady(_ kind, source, result uintptr) {
	a.calls = append(a.calls, [2]uintptr{source, result})
	if a.panics {
		panic("async")
	}
}
