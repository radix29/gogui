//go:build windows

package win32

import (
	"bytes"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/radix29/gogui/internal/shell/win32/w32"
)

func TestClipboardCodec(t *testing.T) {
	for _, c := range []struct {
		in, out string
		data    []byte
	}{
		{"", "", []byte{0, 0}},
		{"Ab", "Ab", []byte{'A', 0, 'b', 0, 0, 0}},
		{"a\r\nb", "a\r\nb", []byte{'a', 0, '\r', 0, '\n', 0, 'b', 0, 0, 0}},
		{"日", "日", []byte{0xE5, 0x65, 0, 0}},
		{"👍", "👍", []byte{0x3D, 0xD8, 0x4D, 0xDC, 0, 0}}, // a surrogate pair
		{"ab\x00cd", "ab", []byte{'a', 0, 'b', 0, 0, 0}}, // cut at the NUL
		{"\xff", "�", []byte{0xFD, 0xFF, 0, 0}},          // invalid UTF-8
	} {
		data := encodeClipboardText(c.in)
		if !bytes.Equal(data, c.data) {
			t.Errorf("encode(%q) = % x, want % x", c.in, data, c.data)
		}
		if got := decodeClipboardText(data); got != c.out {
			t.Errorf("decode(encode(%q)) = %q, want %q", c.in, got, c.out)
		}
	}
	// Blocks as other apps leave them: padding after the NUL, no NUL, an
	// odd last byte, an unpaired surrogate.
	for _, c := range []struct {
		data []byte
		want string
	}{
		{[]byte{'x', 0, 0, 0, 'y', 0, 0, 0}, "x"},
		{[]byte{'x', 0, 'y', 0}, "xy"},
		{[]byte{'x', 0, 'y'}, "x"},
		{[]byte{0x3D, 0xD8, 'z', 0, 0, 0}, "�z"},
		{nil, ""},
	} {
		if got := decodeClipboardText(c.data); got != c.want {
			t.Errorf("decode(% x) = %q, want %q", c.data, got, c.want)
		}
	}
}

// TestClipboardRoundTrip goes through the real Windows clipboard: text
// written by the shell reads back unchanged (ASCII, CJK, emoji with
// modifiers and joiners, "\r\n" kept), an empty clipboard reads as no
// text, and reads are delivered from the queue. The user's clipboard text
// is put back afterwards (other formats on it are lost).
func TestClipboardRoundTrip(t *testing.T) {
	s := newTestShell()
	type read struct {
		text string
		ok   bool
	}
	cases := []string{
		"hello, clipboard",
		"日本語のテキスト 中文 한국어",
		"👍🏽 👨‍👩‍👧 🇨🇭",
		"line 1\r\nline 2\nline 3",
		"",
	}
	var got []read
	err := run(t, s, &handler{ready: func() {
		mustPanic(t, "ReadClipboardText with a nil func", func() { s.ReadClipboardText(nil) })
		saved, savedErr := s.readClipboard()
		defer func() {
			if savedErr == nil {
				if err := s.setClipboard(saved); err != nil {
					t.Errorf("restoring the clipboard: %v", err)
				}
			}
		}()

		deliver := func(text string, ok bool) { got = append(got, read{text, ok}) }
		for _, c := range cases {
			if err := s.setClipboard(c); err != nil {
				t.Fatalf("setClipboard(%q): %v", c, err)
			}
			s.ReadClipboardText(deliver)
			if len(got) != 0 {
				t.Fatal("ReadClipboardText delivered inside the call")
			}
		}
		s.SetClipboardText("through the interface")
		s.ReadClipboardText(deliver)

		// A clipboard without text.
		if err := openClipboard(s.listener); err != nil {
			t.Fatal(err)
		}
		if err := w32.EmptyClipboard(); err != nil {
			t.Error(err)
		}
		if err := w32.CloseClipboard(); err != nil {
			t.Error(err)
		}
		if _, err := s.readClipboard(); !errors.Is(err, errNoText) {
			t.Errorf("readClipboard of an empty clipboard = %v, want errNoText", err)
		}
		s.ReadClipboardText(deliver)

		// Held by another thread for longer than the retries: the write
		// and the read fail, and the clipboard stays as it was.
		if err := s.setClipboard("kept"); err != nil {
			t.Fatal(err)
		}
		release := holdClipboard(t)
		if err := s.setClipboard("lost"); err == nil {
			t.Error("setClipboard while the clipboard is open elsewhere succeeded")
		}
		if _, err := s.readClipboard(); err == nil {
			t.Error("readClipboard while the clipboard is open elsewhere succeeded")
		}
		release()
		if text, err := s.readClipboard(); text != "kept" || err != nil {
			t.Errorf("after a failed write: (%q, %v), want (\"kept\", nil)", text, err)
		}
		s.Post(s.Quit) // after the reads
	}})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	want := make([]read, 0, len(cases)+2)
	for _, c := range cases {
		want = append(want, read{c, true})
	}
	want = append(want, read{"through the interface", true}, read{"", false})
	if len(got) != len(want) {
		t.Fatalf("reads = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("read %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// holdClipboard opens the clipboard on another OS thread, as another
// process would, and returns the func that closes it there.
func holdClipboard(t *testing.T) (release func()) {
	t.Helper()
	opened := make(chan error)
	done := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(closed)
		// A clipboard listener (clipboard history, a sync tool) may still
		// hold the clipboard after the test's last write: retry for up to
		// about 1 s, as openClipboard does for a shorter time.
		var err error
		for range 100 {
			if err = w32.OpenClipboard(0); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		opened <- err
		if err != nil {
			return
		}
		<-done
		_ = w32.CloseClipboard()
	}()
	if err := <-opened; err != nil {
		t.Fatalf("holding the clipboard: %v", err)
	}
	return func() {
		close(done)
		<-closed
	}
}
