package aqua

import (
	"unicode"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/shell"
)

// Keyboard access to the menu strip (PLAN.md S5.6i), as the Windows menu
// bar and macOS's Ctrl+F2:
//
//   - Alt pressed and released alone, or F10, focuses the strip: its first
//     title is highlighted and the keys go to it. The content keeps its
//     focus but draws it inactive (the window's router is inactive, so a
//     focus ring goes and a caret stops, and the IME is off).
//   - In the focused strip Left and Right move the highlight, wrapping;
//     Down, Return and Space open the highlighted menu with its first
//     enabled item highlighted; a character highlights the next title it
//     starts, or the first one a prefix typed within menuTypeSelectReset
//     starts (as the menu's type-select); Escape leaves. Other keys are
//     eaten (their text too), except a key with Control, Alt or the
//     Windows key, which leaves the strip and goes on (a shortcut).
//   - In an open menu Left and Right switch to the neighbour menu,
//     wrapping, also in a menu opened by mouse (the menu hands them over:
//     menu.SetSideKeys); Escape closes it, and in a menu opened from the
//     focused strip the strip stays focused with its title highlighted.
//   - Alt alone or F10 again, a press (in the window or in the menu), a
//     pick, the window losing key, a new strip and a relayout dropping
//     the highlighted title leave the strip.
//
// A lone Alt is told by shell.KeyEvent.Modifier: the key-down of the Alt
// key with nothing else held, then its key-up with no other key-down nor
// press in between. Its key-down is consumed in a window with a strip, so
// that Win32 does not enter its system-menu mode on the release (nor on
// F10); Alt+Space still opens the system menu and Alt+F4 still closes,
// as those key-downs go on. A window without a strip (untitled, or before
// SetMenuBar) leaves Alt and F10 alone.

// menuAccess looks at an event of the window (p nil) or of its grab popup
// p before the routing and the accelerators: done says that it took the
// event, and handled is the result to return then.
func (w *Window) menuAccess(ev shell.Event, p *popupRoot) (handled, done bool) {
	s := w.menuStrip()
	mine := s != nil && (p == nil || s.open == p)
	switch ev := ev.(type) {
	case shell.PointerEvent:
		if ev.Kind == shell.PointerDown {
			w.altAlone = false
			if mine {
				s.leaveKeyboard() // the press goes on as a press
			}
		}
	case shell.TextEvent, shell.PreeditEvent:
		if mine && p == nil && s.kbd {
			return true, true // the strip has the keys: no text for the focus
		}
	case shell.KeyEvent:
		lone := w.loneAlt(ev)
		if !mine {
			return false, false
		}
		return s.key(ev, p != nil, lone)
	}
	return false, false
}

// loneAlt follows the lone-Alt detector with k and reports whether k ends
// a lone Alt (see menuAccess).
func (w *Window) loneAlt(k shell.KeyEvent) bool {
	switch {
	case k.Down && k.Repeat:
	case k.Down:
		w.altAlone = k.Modifier == driver.ModOption && k.Mods == driver.ModOption
	case k.Modifier == driver.ModOption:
		lone := w.altAlone && k.Mods == 0
		w.altAlone = false
		return lone
	}
	return false
}

// key handles a key event for the strip, from the window or (inMenu) the
// open menu's popup; lone says that it ends a lone Alt.
func (s *menuStrip) key(k shell.KeyEvent, inMenu, lone bool) (handled, done bool) {
	if !k.Down {
		if lone {
			s.toggleKeyboard()
			return true, true
		}
		return false, false
	}
	switch {
	case k.Modifier == driver.ModOption && k.Mods == driver.ModOption:
		return true, true // a lone Alt may begin: the key-up decides
	case k.Key == driver.KeyF1+9 && k.Mods == 0: // F10
		if !k.Repeat {
			s.toggleKeyboard()
		}
		return true, true
	case inMenu || !s.kbd:
		return false, false
	case k.Modifier != 0:
		return true, true // another modifier key alone does nothing
	case k.Mods&^driver.ModShift != 0:
		s.leaveKeyboard() // a shortcut: it goes on to the accelerators
		return false, false
	}
	switch k.Key {
	case driver.KeyLeft:
		s.moveKeyboard(-1)
	case driver.KeyRight:
		s.moveKeyboard(1)
	case driver.KeyDown, driver.KeyReturn, driver.KeySpace:
		s.openKeyboard(s.highlight)
	case driver.KeyEscape:
		s.leaveKeyboard()
	default:
		s.typeSelect(k)
	}
	return true, true
}

// KeyboardFocused reports whether the strip has the keyboard focus.
func (s *menuStrip) KeyboardFocused() bool { return s.kbd }

// toggleKeyboard is a lone Alt or F10: it closes an open menu and leaves
// the strip, leaves the focused strip, or focuses it.
func (s *menuStrip) toggleKeyboard() {
	switch {
	case s.open != nil:
		s.leaveKeyboard()
		s.CloseMenu()
	case s.kbd:
		s.leaveKeyboard()
	default:
		s.focusKeyboard()
	}
}

// focusKeyboard focuses the strip with its first title highlighted, in a
// key window with a shown title.
func (s *menuStrip) focusKeyboard() {
	w := s.tb.w
	if s.kbd || s.Shown() == 0 || w.closed || !w.key || !s.VisibleInTree() {
		return
	}
	s.stopFlash()
	s.kbd = true
	s.typed.reset()
	s.SetHighlighted(0)
	s.keyStateChanged()
}

// leaveKeyboard ends the strip's keyboard focus (an open menu stays open,
// as a menu opened by mouse, with its title highlighted).
func (s *menuStrip) leaveKeyboard() {
	if !s.kbd {
		return
	}
	s.kbd = false
	s.typed.reset()
	if s.open == nil {
		s.SetHighlighted(-1)
	}
	s.keyStateChanged()
}

// keyStateChanged makes the content's focus inactive while the strip has
// the keys, and active again after.
func (s *menuStrip) keyStateChanged() {
	w := s.tb.w
	if w.closed {
		return
	}
	w.router.SetActive(w.contentActive())
	w.stateChanged()
	w.publishTextInput()
}

// moveKeyboard moves the highlight to the next shown title in direction
// dir, wrapping.
func (s *menuStrip) moveKeyboard(dir int) {
	if n := s.Shown(); n > 0 {
		s.SetHighlighted(((s.highlight+dir)%n + n) % n)
		s.typed.reset()
	}
}

// openKeyboard opens menu i from the keyboard (or switches the open menu
// to it) with its first enabled item highlighted.
func (s *menuStrip) openKeyboard(i int) {
	s.typed.reset()
	if s.openMenu(i, menuOpening{}) {
		s.menu.keyHighlight(s.menu.step(-1, 1))
	}
}

// side is the open menu's Left (-1) and Right (1): the neighbour menu,
// wrapping over the shown titles.
func (s *menuStrip) side(dir int) {
	if n := s.Shown(); s.open != nil && n > 1 {
		s.openKeyboard(((s.openAt+dir)%n + n) % n)
	}
}

// typeSelect highlights a title by the character of k, by the menus'
// type-select (typeSelect; see the keyboard access). Keys that are no
// character do nothing.
func (s *menuStrip) typeSelect(k shell.KeyEvent) {
	r := rune(k.Key)
	if r <= ' ' || (k.Key >= driver.KeyUp && r <= 0xF8FF) || !unicode.IsPrint(r) {
		return
	}
	all := func(int) bool { return true }
	title := func(i int) string { return s.bar.Menus[i].Title }
	if i := s.typed.next(r, k.Time, s.Shown(), s.highlight, title, all); i >= 0 {
		s.SetHighlighted(i)
	}
}

// menuCancelled is the open menu's cancel: the menu closes; a cancel by a
// key in a menu opened from the focused strip leaves the strip focused on
// its title, any other leaves the strip.
func (s *menuStrip) menuCancelled() {
	if s.kbd && s.menu.CancelledByKey() {
		s.keepKbd = true
		defer func() { s.keepKbd = false }()
	}
	s.CloseMenu()
}
