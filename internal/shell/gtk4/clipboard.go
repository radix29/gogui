package gtk4

import (
	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// The clipboard (S5.2b): the GdkClipboard of the default display. A write
// is gdk_clipboard_set_text; a read is gdk_clipboard_read_text_async (no
// cancellable, so no gio), finished in the one-shot asyncReady trampoline
// and delivered to fn through the queue. The logic is here, over a small
// clipboardBackend that the host tests fake; the GDK calls are in
// clipboard_linux.go.

// clipboardBackend is the native side of the clipboard: the default
// display's GdkClipboard (clipboard_linux.go), a fake in the host tests.
// Every method runs on the GTK thread.
type clipboardBackend interface {
	// setText puts text (valid UTF-8 without NUL) on the clipboard.
	setText(text string)
	// readText starts an asynchronous read of the clipboard's text; GTK
	// calls asyncReady once, later, from the main loop, with data as
	// user_data. It returns false when no read was started (no display);
	// asyncReady is then never called.
	readText(data ffi.Handle) bool
	// finishText finishes the read whose result asyncReady got from
	// source: the text and true, or "" and false when the clipboard has no
	// text or the read failed. It frees the native string and error.
	finishText(source, result uintptr) (string, bool)
}

// clipboard is the shell's clipboard state: its backend (nil: none, every
// write dropped and every read failing) and the reads in flight, whose
// handles it frees at shutdown.
type clipboard struct {
	backend clipboardBackend
	pending map[ffi.Handle]*clipboardRead
}

// clipboardRead is the peer of one pending read: the shell to deliver to,
// the caller's fn and the read's handle.
type clipboardRead struct {
	s  *Shell
	fn func(text string, ok bool)
	h  ffi.Handle
}

// SetClipboardText implements shell.Shell. The text is sanitized as on
// every shell (shellkit.ClipboardText: cut at a NUL, bad UTF-8 → U+FFFD).
// Without a display the write is dropped.
func (s *Shell) SetClipboardText(text string) {
	s.checkReady("SetClipboardText")
	if s.clip.backend == nil {
		return
	}
	s.clip.backend.setText(shellkit.ClipboardText(text))
}

// ReadClipboardText implements shell.Shell: GTK reads asynchronously, and
// fn runs from the queue once the read is finished. A read that cannot
// start (no display) fails, also from the queue. A read still pending at
// Quit is dropped: its handle goes at shutdown and a late result finds
// nothing.
func (s *Shell) ReadClipboardText(fn func(text string, ok bool)) {
	s.checkReady("ReadClipboardText")
	if fn == nil {
		panic("gtk4: ReadClipboardText with a nil func")
	}
	if s.quit || s.clip.backend == nil {
		s.Post(func() { fn("", false) }) // dropped after Quit, like the result
		return
	}
	r := &clipboardRead{s: s, fn: fn}
	h := dispatch.New(entry{peer: r, kind: kindClipboardRead})
	r.h = h
	if !s.clip.backend.readText(h) {
		dispatch.Free(h)
		s.Post(func() { fn("", false) })
		return
	}
	if s.clip.pending == nil {
		s.clip.pending = map[ffi.Handle]*clipboardRead{}
	}
	s.clip.pending[h] = r
}

// asyncReady finishes the read (always, so the native string is freed) and
// posts the result; after Quit, Post drops it. The handle is freed by the
// trampoline.
func (r *clipboardRead) asyncReady(k kind, source, result uintptr) {
	if k != kindClipboardRead {
		return
	}
	s := r.s
	delete(s.clip.pending, r.h)
	text, ok := "", false
	if s.clip.backend != nil {
		text, ok = s.clip.backend.finishText(source, result)
	}
	if !ok {
		text = ""
	}
	fn := r.fn
	s.Post(func() { fn(text, ok) })
}

// close drops the reads in flight: their handles are freed, so a result
// that GTK delivers after shutdown finds a stale handle and is dropped.
// It runs in Shell.shutdown.
func (c *clipboard) close() {
	for h := range c.pending {
		dispatch.Free(h)
	}
	c.pending = nil
}
