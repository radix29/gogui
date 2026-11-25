package fake

import (
	"unicode"

	"github.com/radix29/gogui/internal/driver"
)

// usShifted is the Shift level of the US layout's character keys other
// than the letters (S5.6h).
var usShifted = map[driver.Key]driver.Key{
	'1': '!', '2': '@', '3': '#', '4': '$', '5': '%', '6': '^', '7': '&', '8': '*', '9': '(',
	'0': ')', '-': '_', '=': '+', '[': '{', ']': '}', '\\': '|', ';': ':', '\'': '"',
	',': '<', '.': '>', '/': '?', '`': '~',
}

// SetShiftLayout sets the Shift level of the fake keyboard layout, which
// gives KeyEvent.Shifted on key-downs (S5.6h): m maps an unshifted
// character key to the character it gives with Shift; a letter not in m
// gives itself (its upper case), any other key KeyNone. nil restores the
// US layout, the default. Before Start it may be called from any
// goroutine; afterwards only on the UI goroutine.
func (s *Shell) SetShiftLayout(m map[driver.Key]driver.Key) {
	if s.started.Load() {
		s.check("Shell.SetShiftLayout")
	}
	if m == nil {
		s.shiftLayout = nil
		return
	}
	s.shiftLayout = make(map[driver.Key]driver.Key, len(m))
	for k, v := range m {
		s.shiftLayout[driver.NormalizeKey(k)] = driver.NormalizeKey(v)
	}
}

// shifted returns KeyEvent.Shifted of a key-down of k (normalized).
func (s *Shell) shifted(k driver.Key) driver.Key {
	m := s.shiftLayout
	if m == nil {
		m = usShifted
	}
	if v, ok := m[k]; ok {
		return v
	}
	if k < driver.KeyUp && unicode.IsLetter(rune(k)) {
		return k
	}
	return driver.KeyNone
}
