package fake

import (
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/shellkit"
)

// TextSystem is the fake's render.TextSystem: shellkit.FixedText, the
// fixed-width metrics the native shells fall back to (review R6), so the
// fake lays out text exactly as their fallback does. It has no state and
// may be used from any goroutine.
type TextSystem = shellkit.FixedText

// NewTextSystem returns a fake text system.
func NewTextSystem() *TextSystem { return &TextSystem{} }

// RuneWidth returns the fake width of one rune in f (7 pt at 13 pt).
func RuneWidth(f render.Font) float64 { return shellkit.FixedRuneWidth(f) }

// LineHeight returns the fake line height of f (16 pt at 13 pt).
func LineHeight(f render.Font) float64 { return shellkit.FixedLineHeight(f) }
