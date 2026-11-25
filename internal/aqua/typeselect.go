package aqua

import (
	"strings"
	"time"
)

// typeSelect is the type-select of a menu and of the menu strip's titles
// (review R5: one rule for both, as in AppKit and the Windows menus). A
// character typed within menuTypeSelectReset of the previous one grows the
// prefix, and the first item from the highlighted one on (wrapping) that
// the grown prefix starts is highlighted; else the character starts a new
// prefix and highlights the next item after the highlighted one that it
// starts, so typing the same letter again steps through the items with
// that letter. A space only grows a prefix. Titles compare
// case-insensitively; items that cannot be highlighted are skipped. The
// zero value has no prefix.
type typeSelect struct {
	typed string        // the prefix, lower case ("" after a miss)
	at    time.Duration // the time of the last character
	any   bool          // a character was typed at at
}

// reset forgets the prefix: the next character starts a new one.
func (s *typeSelect) reset() { *s = typeSelect{} }

// growing reports whether a character typed at now still belongs to the
// last one's prefix (a Space then is typed, not a pick).
func (s *typeSelect) growing(now time.Duration) bool {
	return s.any && now-s.at < menuTypeSelectReset
}

// next types r at now and returns the item to highlight among n items
// with title(i), of which ok(i) can be highlighted, the highlighted one
// being cur (-1 for none); -1 when no item matches (the highlight stays).
func (s *typeSelect) next(r rune, now time.Duration, n, cur int, title func(int) string, ok func(int) bool) int {
	grow := s.growing(now)
	s.at, s.any = now, true
	if r == ' ' && !(grow && s.typed != "") {
		return -1 // a space only grows a prefix
	}
	c := strings.ToLower(string(r))
	find := func(prefix string, from int) int {
		for j := range n {
			i := ((from+j)%n + n) % n
			if ok(i) && strings.HasPrefix(strings.ToLower(title(i)), prefix) {
				return i
			}
		}
		return -1
	}
	if grow && s.typed != "" {
		if i := find(s.typed+c, max(cur, 0)); i >= 0 {
			s.typed += c
			return i
		}
	}
	s.typed = ""
	i := find(c, cur+1)
	if i >= 0 {
		s.typed = c
	}
	return i
}
