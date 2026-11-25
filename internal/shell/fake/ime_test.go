package fake

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell"
)

func TestTextInputRecorded(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	if ti := f.TextInput(); ti != (shell.TextInput{}) || f.TextInputSets() != 0 || f.Cancels() != 0 {
		t.Fatalf("initial state %+v, %d sets, %d cancels", ti, f.TextInputSets(), f.Cancels())
	}
	on := shell.TextInput{Enabled: true, Caret: driver.Rect{X: 10, Y: 20, W: 1, H: 16}}
	f.SetTextInput(on)
	f.SetTextInput(on) // recorded even without a change
	f.CancelComposition()
	if f.TextInput() != on || f.TextInputSets() != 2 || f.Cancels() != 1 {
		t.Fatalf("state %+v, %d sets, %d cancels", f.TextInput(), f.TextInputSets(), f.Cancels())
	}
	if got := w.take(); got != "" {
		t.Fatalf("SetTextInput or CancelComposition called the handler: %s", got)
	}

	// SetTextInput is allowed during Paint, CancelComposition is not.
	var paintPanic any
	w.paint = func(render.Canvas) {
		f.SetTextInput(shell.TextInput{Enabled: true})
		func() {
			defer func() { paintPanic = recover() }()
			f.CancelComposition()
		}()
	}
	f.Invalidate()
	s.Flush()
	w.paint = nil
	if !f.TextInput().Enabled || f.TextInput().Caret != (driver.Rect{}) || paintPanic == nil || f.Cancels() != 1 {
		t.Fatalf("in Paint: state %+v, cancel panic %v, %d cancels", f.TextInput(), paintPanic, f.Cancels())
	}

	// A closed surface ignores both and keeps its state readable.
	f.SetTextInput(on)
	f.Close()
	f.SetTextInput(shell.TextInput{})
	f.CancelComposition()
	if f.TextInput() != on || f.TextInputSets() != 4 || f.Cancels() != 1 {
		t.Fatalf("closed: state %+v, %d sets, %d cancels", f.TextInput(), f.TextInputSets(), f.Cancels())
	}
}

func TestPreeditEvents(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	f.SetTextInput(shell.TextInput{Enabled: true})
	s.FakeClock().Advance(1000)
	var times []int64
	w.event = func(ev shell.Event) bool { times = append(times, int64(ev.EventTime())); return false }

	segs := []shell.PreeditSegment{{Start: 0, End: 6, Selected: true}, {Start: 6, End: 9}}
	f.Preedit("にほ", 6)
	if !f.Composing() {
		t.Fatal("not composing after Preedit")
	}
	f.Preedit("日本語", 9, segs...)
	segs[0].Selected = false // the event has its own copy
	f.EndPreedit()
	if f.Composing() {
		t.Fatal("composing after EndPreedit")
	}
	if got, want := w.take(), `preedit "にほ" @6; preedit "日本語" @9 [0,6)* [6,9); preedit end`; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	// CommitPreedit: the commit, then the end (the GTK order).
	f.Preedit("か", 3)
	f.CommitPreedit("課")
	if got, want := w.take(), `preedit "か" @3; text "課"; preedit end`; got != want || f.Composing() {
		t.Fatalf("got %s (composing %v)", got, f.Composing())
	}
	// Commit followed by Preedit: the Win32 order of a commit with the
	// next composition.
	f.Preedit("a", 1)
	f.Commit("あ")
	if f.Composing() {
		t.Fatal("composing after a commit")
	}
	f.Preedit("い", 3)
	if got, want := w.take(), `preedit "a" @1; text "あ"; preedit "い" @3`; got != want || !f.Composing() {
		t.Fatalf("got %s (composing %v)", got, f.Composing())
	}
	for i, tm := range times {
		if tm != 1000 {
			t.Fatalf("event %d time %d, want the clock's 1000", i, tm)
		}
	}

	// Cancelling and disabling end the composition without events.
	f.CancelComposition()
	if f.Composing() || w.take() != "" {
		t.Fatal("CancelComposition: still composing or an event")
	}
	f.Preedit("う", 3)
	w.take()
	f.SetTextInput(shell.TextInput{})
	if f.Composing() || w.take() != "" {
		t.Fatal("disabling: still composing or an event")
	}
}

func TestCommitPreeditStopsWhenHandlerReacts(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})
	f.SetTextInput(shell.TextInput{Enabled: true})
	// The handler disables the text input on the commit (the focus moved
	// to a button): no end follows.
	w.event = func(ev shell.Event) bool {
		if _, ok := ev.(shell.TextEvent); ok {
			f.SetTextInput(shell.TextInput{})
		}
		return false
	}
	f.Preedit("か", 3)
	f.CommitPreedit("課")
	if got, want := w.take(), `preedit "か" @3; text "課"`; got != want {
		t.Fatalf("got %s", got)
	}
	// The handler closes the surface on the commit.
	f.SetTextInput(shell.TextInput{Enabled: true})
	w.event = func(ev shell.Event) bool {
		if _, ok := ev.(shell.TextEvent); ok {
			f.Close()
		}
		return false
	}
	f.CommitPreedit("x")
	if got, want := w.take(), `text "x"; closed`; got != want {
		t.Fatalf("got %s", got)
	}
}

func TestPreeditPanics(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(100, 100)})

	// No text input enabled.
	mustPanic(t, "no text input", func() { f.Preedit("a", 0) })
	mustPanic(t, "no text input", func() { f.EndPreedit() })
	mustPanic(t, "no text input", func() { f.CommitPreedit("a") })

	f.SetTextInput(shell.TextInput{Enabled: true})
	seg := func(a, b int) shell.PreeditSegment { return shell.PreeditSegment{Start: a, End: b} }
	mustPanic(t, "empty text", func() { f.Preedit("", 0) })
	mustPanic(t, "invalid UTF-8", func() { f.Preedit("\xff", 0) })
	mustPanic(t, "control character", func() { f.Preedit("a\n", 0) })
	mustPanic(t, "cursor", func() { f.Preedit("ab", 3) })
	mustPanic(t, "cursor", func() { f.Preedit("ab", -1) })
	mustPanic(t, "cursor", func() { f.Preedit("日", 1) })
	mustPanic(t, "segment", func() { f.Preedit("abc", 0, seg(1, 1)) })
	mustPanic(t, "segment", func() { f.Preedit("abc", 0, seg(2, 1)) })
	mustPanic(t, "segment", func() { f.Preedit("abc", 0, seg(0, 4)) })
	mustPanic(t, "segment", func() { f.Preedit("abc", 0, seg(0, 2), seg(1, 3)) })
	mustPanic(t, "segment", func() { f.Preedit("日本", 0, seg(0, 2)) })
	mustPanic(t, "empty text", func() { f.CommitPreedit("") })
	if got := w.take(); got != "" {
		t.Fatalf("a rejected preedit was delivered: %s", got)
	}
	f.Preedit("abc", 3, seg(0, 1), seg(1, 3)) // adjacent segments are fine
	f.Preedit("日本", 0, seg(3, 6))             // segments need not cover the text
	if got := w.take(); got != `preedit "abc" @3 [0,1) [1,3); preedit "日本" @0 [3,6)` {
		t.Fatalf("got %s", got)
	}

	// IME input needs the key surface.
	s.Deactivate()
	mustPanic(t, "not key", func() { f.Preedit("a", 0) })
	mustPanic(t, "not key", func() { f.EndPreedit() })
	mustPanic(t, "not key", func() { f.Send(shell.PreeditEvent{Text: "a"}) })
}

func TestSendPreedit(t *testing.T) {
	s := started(t)
	var w win
	f := open(t, s, &w, shell.SurfaceConfig{Size: sz(10, 10)})
	s.FakeClock().Advance(1000)
	var times []int64
	w.event = func(ev shell.Event) bool { times = append(times, int64(ev.EventTime())); return true }
	// Send checks neither the event nor the text input, and tracks
	// Composing.
	if !f.Send(shell.PreeditEvent{Text: "a", Cursor: 1}) || !f.Composing() {
		t.Fatal("Send preedit: result or Composing")
	}
	f.Send(shell.TextEvent{Text: "b", Time: 5})
	if f.Composing() {
		t.Fatal("composing after a TextEvent through Send")
	}
	f.Send(shell.PreeditEvent{Text: "c"})
	f.Send(shell.PreeditEvent{})
	if f.Composing() {
		t.Fatal("composing after an empty preedit through Send")
	}
	if got := w.take(); got != `preedit "a" @1; text "b"; preedit "c" @0; preedit end` {
		t.Fatalf("got %s", got)
	}
	if len(times) != 4 || times[0] != 1000 || times[1] != 5 || times[3] != 1000 {
		t.Fatalf("times %v", times)
	}
}
