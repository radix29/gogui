package aqua

import (
	"sync/atomic"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Default and cancel buttons (PLAN.md S3.2f).
//
// An element is marked as a default or a cancel button with
// Base.SetDefaultButton and SetCancelButton (a Button peer's
// SetDefault and SetCancel do it). The mark stays on the element, so it
// works whether the button joins the window before or after it is marked.
// A window's default button is the marked element of its tree that was
// marked last (Window.DefaultButton): marking a second one replaces the
// first, and removing the second makes the first, still marked, the
// default again. The same holds for the cancel button.
//
// A key-down that no element handled (the focused element and its
// ancestors get it first, so a text view can take Return) presses the
// window's button with Activate, when the button is visible and enabled in
// the tree:
//
//   - Return without modifiers presses the default button (the shells
//     report keypad Enter as Return too);
//   - Escape without modifiers, and Primary+. (Ctrl+. on Windows and Linux,
//     as ⌘. on macOS), press the cancel button.
//
// An auto-repeated key-down presses nothing but is still consumed when the
// button could be pressed. A missing, hidden or disabled button leaves the
// key unhandled, and the shell's default handling follows.

// markSerial orders the marks: a later mark wins. Atomic because tests run
// several drivers, each on its own UI thread.
var markSerial atomic.Uint64

// IsDefaultButton reports whether the element is marked as a default button.
func (b *Base) IsDefaultButton() bool { return b.defaultMark != 0 }

// SetDefaultButton marks the element as a default button (or unmarks it).
// Marking an already marked element makes it the latest mark again.
func (b *Base) SetDefaultButton(on bool) { b.defaultMark = nextMark(on) }

// IsCancelButton reports whether the element is marked as a cancel button.
func (b *Base) IsCancelButton() bool { return b.cancelMark != 0 }

// SetCancelButton marks the element as a cancel button (or unmarks it).
func (b *Base) SetCancelButton(on bool) { b.cancelMark = nextMark(on) }

func nextMark(on bool) uint64 {
	if !on {
		return 0
	}
	return markSerial.Add(1)
}

// DefaultButton returns the window's default button: the element of its
// tree marked last with SetDefaultButton, or nil. It may be hidden or
// disabled.
func (w *Window) DefaultButton() Element {
	return latestMark(w.root, func(b *Base) uint64 { return b.defaultMark })
}

// CancelButton returns the window's cancel button (see DefaultButton).
func (w *Window) CancelButton() Element {
	return latestMark(w.root, func(b *Base) uint64 { return b.cancelMark })
}

// latestMark returns the element of root's tree with the highest mark.
func latestMark(root Element, mark func(*Base) uint64) Element {
	var best Element
	var bestMark uint64
	var walk func(b *Base)
	walk = func(b *Base) {
		if m := mark(b); m > bestMark {
			best, bestMark = b.self, m
		}
		for _, c := range b.children {
			walk(c.base())
		}
	}
	walk(root.base())
	return best
}

// keyEquivalent handles a key-down no element handled: Return presses the
// default button, Escape and Ctrl+. the cancel button. It reports whether
// the key was consumed.
func (w *Window) keyEquivalent(ev shell.KeyEvent) bool {
	if !ev.Down {
		return false
	}
	var target Element
	switch {
	case isDefaultKey(ev):
		target = w.DefaultButton()
	case isCancelKey(ev):
		target = w.CancelButton()
	}
	if target == nil || !w.router.usable(target) {
		return false
	}
	if ev.Repeat {
		return true
	}
	return activateByKey(target)
}

// isDefaultKey reports whether ev is the default button's key: Return
// without modifiers (the shells report keypad Enter as Return too).
func isDefaultKey(ev shell.KeyEvent) bool { return ev.Key == driver.KeyReturn && ev.Mods == 0 }

// isCancelKey reports whether ev is the cancel button's key: Escape
// without modifiers, or Primary+. (Ctrl+. on Windows and Linux, as ⌘. on
// macOS).
func isCancelKey(ev shell.KeyEvent) bool {
	return ev.Key == driver.KeyEscape && ev.Mods == 0 ||
		ev.Key == '.' && ev.Mods == driver.ModPrimary.Resolve(false)
}
