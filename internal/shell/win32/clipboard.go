//go:build windows

package win32

import (
	"encoding/binary"
	"errors"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/radix29/gogui/internal/ffi"
	"github.com/radix29/gogui/internal/shell/shellkit"
	"github.com/radix29/gogui/internal/shell/win32/w32"
)

// The clipboard (S5.2a): CF_UNICODETEXT in movable global memory, the
// listener window as the owner. Windows reads synchronously; the result is
// delivered through Post so the shell behaves like GTK's asynchronous read.

// errNoText is readClipboard's error when the clipboard holds no text.
var errNoText = errors.New("win32: the clipboard holds no text")

// Opening the clipboard fails while another process has it open (a
// clipboard manager, a remote-desktop sync, the other end of a paste), so
// openClipboard retries a few times before giving up: at most about
// clipboardTries × clipboardRetry on the UI thread.
const (
	clipboardTries = 8
	clipboardRetry = 4 * time.Millisecond
)

// SetClipboardText implements shell.Shell. A failure is dropped (the
// interface has no way to report it); setClipboard returns it for tests.
func (s *Shell) SetClipboardText(text string) {
	s.checkReady("SetClipboardText")
	_ = s.setClipboard(text)
}

// ReadClipboardText implements shell.Shell: the clipboard is read now and
// fn runs from the queue.
func (s *Shell) ReadClipboardText(fn func(text string, ok bool)) {
	s.checkReady("ReadClipboardText")
	if fn == nil {
		panic("win32: ReadClipboardText with a nil func")
	}
	text, err := s.readClipboard()
	s.Post(func() { fn(text, err == nil) })
}

// setClipboard empties the clipboard and puts text on it as
// CF_UNICODETEXT, sanitized by encodeClipboardText (text after a NUL is
// cut off, invalid UTF-8 becomes U+FFFD).
func (s *Shell) setClipboard(text string) error {
	data := encodeClipboardText(text)
	mem, err := w32.GlobalAlloc(w32.GMEM_MOVEABLE, uintptr(len(data)))
	if err != nil {
		return err
	}
	owned := false // by the system, after SetClipboardData
	defer func() {
		if !owned {
			_ = w32.GlobalFree(mem)
		}
	}()
	p, err := w32.GlobalLock(mem)
	if err != nil {
		return err
	}
	ffi.WriteBytes(p, data)
	w32.GlobalUnlock(mem)

	if err := openClipboard(s.listener); err != nil {
		return err
	}
	defer func() { _ = w32.CloseClipboard() }()
	if err := w32.EmptyClipboard(); err != nil {
		return err
	}
	if err := w32.SetClipboardData(w32.CF_UNICODETEXT, mem); err != nil {
		return err
	}
	owned = true
	return nil
}

// readClipboard returns the clipboard's CF_UNICODETEXT (Windows converts
// CF_TEXT and CF_OEMTEXT to it), or errNoText.
func (s *Shell) readClipboard() (string, error) {
	if err := openClipboard(s.listener); err != nil {
		return "", err
	}
	defer func() { _ = w32.CloseClipboard() }()
	mem := w32.GetClipboardData(w32.CF_UNICODETEXT)
	if mem == 0 {
		return "", errNoText
	}
	n := w32.GlobalSize(mem)
	p, err := w32.GlobalLock(mem)
	if err != nil {
		return "", err
	}
	data := ffi.CopyBytes(p, int(n))
	w32.GlobalUnlock(mem)
	return decodeClipboardText(data), nil
}

// openClipboard opens the clipboard with owner, retrying while another
// window holds it.
func openClipboard(owner windows.HWND) error {
	var err error
	for i := range clipboardTries {
		if i > 0 {
			time.Sleep(clipboardRetry)
		}
		if err = w32.OpenClipboard(owner); err == nil {
			return nil
		}
	}
	return err
}

// encodeClipboardText returns text as NUL-terminated little-endian UTF-16,
// sanitized as on every shell (shellkit.ClipboardText: cut at the first
// NUL, each invalid UTF-8 byte U+FFFD).
func encodeClipboardText(text string) []byte {
	u := utf16.Encode([]rune(shellkit.ClipboardText(text)))
	data := make([]byte, 0, 2*len(u)+2)
	for _, c := range u {
		data = binary.LittleEndian.AppendUint16(data, c)
	}
	return binary.LittleEndian.AppendUint16(data, 0)
}

// decodeClipboardText reads little-endian UTF-16 up to the first NUL or the
// end of data (a block without a terminator, or an odd last byte, which is
// dropped). Unpaired surrogates become U+FFFD.
func decodeClipboardText(data []byte) string {
	u := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		c := binary.LittleEndian.Uint16(data[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
