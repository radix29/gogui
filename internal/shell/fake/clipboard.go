package fake

import "github.com/radix29/gogui/internal/shell/shellkit"

// The clipboard (S5.2a): a string the shell keeps. Reads are delivered
// through the queue, so they run on the next Step (or the Run loop), as on
// the real shells.

// SetClipboardText implements shell.Shell: the clipboard holds text,
// sanitized as on the real shells (shellkit.ClipboardText: cut at the
// first NUL, each invalid UTF-8 byte U+FFFD).
func (s *Shell) SetClipboardText(text string) {
	s.checkReady("Shell.SetClipboardText")
	s.clip, s.clipOK = shellkit.ClipboardText(text), true
}

// ReadClipboardText implements shell.Shell. The clipboard is read now; fn
// runs from the queue (Step), with ok false while the clipboard holds no
// text.
func (s *Shell) ReadClipboardText(fn func(text string, ok bool)) {
	s.checkReady("Shell.ReadClipboardText")
	if fn == nil {
		panic("fake: ReadClipboardText with a nil func")
	}
	s.clipReads++
	text, ok := s.clip, s.clipOK
	s.Post(func() { fn(text, ok) })
}

// SetClipboard simulates another app changing the clipboard: ok false
// empties it (no text, as after a copy of an image). The text is sanitized
// as SetClipboardText's, since a real read never returns a NUL or invalid
// UTF-8. Before Start it may be called from any goroutine; afterwards only
// on the UI goroutine.
func (s *Shell) SetClipboard(text string, ok bool) {
	if s.started.Load() {
		s.check("Shell.SetClipboard")
	}
	if !ok {
		text = ""
	}
	s.clip, s.clipOK = shellkit.ClipboardText(text), ok
}

// Clipboard returns the clipboard's text and whether it holds any.
func (s *Shell) Clipboard() (text string, ok bool) {
	s.check("Shell.Clipboard")
	return s.clip, s.clipOK
}

// ClipboardReads returns how many times ReadClipboardText was called.
func (s *Shell) ClipboardReads() int {
	s.check("Shell.ClipboardReads")
	return s.clipReads
}
