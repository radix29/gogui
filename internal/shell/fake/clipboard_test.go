package fake

import "testing"

type clipRead struct {
	text string
	ok   bool
}

func TestClipboard(t *testing.T) {
	pre := New()
	pre.SetClipboard("before start", true) // allowed before Start
	if err := pre.Start(&app{}); err != nil {
		t.Fatal(err)
	}
	if text, ok := pre.Clipboard(); text != "before start" || !ok {
		t.Errorf("Clipboard after a SetClipboard before Start = (%q, %v)", text, ok)
	}

	s := started(t)
	if text, ok := s.Clipboard(); text != "" || ok {
		t.Errorf("new clipboard = (%q, %v), want empty, no text", text, ok)
	}
	mustPanic(t, "nil func", func() { s.ReadClipboardText(nil) })

	var got []clipRead
	read := func() {
		s.ReadClipboardText(func(text string, ok bool) { got = append(got, clipRead{text, ok}) })
	}
	read() // no text yet
	s.SetClipboardText("a\r\nb 日本 👍🏽")
	read()
	if len(got) != 0 {
		t.Fatal("ReadClipboardText delivered inside the call")
	}
	// Each read sees the clipboard at the time of the call.
	s.SetClipboard("", false)
	read()
	s.SetClipboard("other app", true)
	read()
	s.SetClipboardText("")
	read()
	if n := s.Step(); n != 5 {
		t.Errorf("Step ran %d functions, want the 5 reads", n)
	}
	want := []clipRead{{"", false}, {"a\r\nb 日本 👍🏽", true}, {"", false}, {"other app", true}, {"", true}}
	if len(got) != len(want) {
		t.Fatalf("reads = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("read %d = %v, want %v", i, got[i], want[i])
		}
	}
	if n := s.ClipboardReads(); n != 5 {
		t.Errorf("ClipboardReads = %d, want 5", n)
	}
	s.SetClipboard("dropped text", false)
	if text, ok := s.Clipboard(); text != "" || ok {
		t.Errorf("SetClipboard(_, false) left (%q, %v)", text, ok)
	}
}

func TestClipboardChecks(t *testing.T) {
	s := New()
	mustPanic(t, "off the UI thread", func() { s.SetClipboardText("x") })
	mustPanic(t, "off the UI thread", func() { s.Clipboard() })
	bound := New()
	bound.q.Bind()
	mustPanic(t, "before Handler.Ready", func() { bound.ReadClipboardText(func(string, bool) {}) })
	mustPanic(t, "before Handler.Ready", func() { bound.SetClipboardText("x") })

	s = started(t)
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		s.SetClipboard("x", true)
	}()
	if r := <-done; r == nil {
		t.Error("SetClipboard off the UI goroutine after Start did not panic")
	}
}

// TestClipboardSanitized (review V4): the fake's clipboard takes text as the
// real shells' do, cut at a NUL and with invalid UTF-8 replaced, both from
// SetClipboardText and from another app (SetClipboard).
func TestClipboardSanitized(t *testing.T) {
	s := started(t)
	s.SetClipboardText("ab\x00cd")
	if text, ok := s.Clipboard(); text != "ab" || !ok {
		t.Errorf("after SetClipboardText with a NUL: (%q, %v), want (\"ab\", true)", text, ok)
	}
	s.SetClipboard("x\xff\xfey", true)
	if text, _ := s.Clipboard(); text != "x��y" {
		t.Errorf("after SetClipboard with bad UTF-8: %q, want \"x��y\"", text)
	}
}
