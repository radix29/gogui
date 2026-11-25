package aqua

import (
	"slices"

	"github.com/radix29/gogui/internal/driver"
)

// Test-only accessors (review §4: nothing but the tests reads them).

// LineHeight is the font's line height, the height of an empty paragraph.
func (c *paraLayouts) LineHeight() float64 { return c.lineH }

// Bound is the most layouts the cache keeps.
func (c *paraLayouts) Bound() int { return c.bound }

// Cached is the number of layouts held.
func (c *paraLayouts) Cached() int { return len(c.m) }

// Items returns a copy of the items.
func (m *menu) Items() []menuItem { return slices.Clone(m.items) }

// TitleOrigin returns the top-left corner of item i's title layout in the
// element's points (S5.5k aligns it with the button's title), or the
// zero point for a separator, an empty title or an unknown index.
func (m *menu) TitleOrigin(i int) driver.Point {
	l := m.layout(i)
	if l == nil {
		return driver.Point{}
	}
	r := m.ItemRect(i)
	return driver.Point{X: r.X + titleX(), Y: r.Y + (r.H-l.Size().H)/2}
}

// Stacked reports whether the buttons are stacked full-width.
func (a *alert) Stacked() bool { return a.stacked }

// MenuPopup returns the open menu's popup, or nil.
func (s *menuStrip) MenuPopup() *popupRoot { return s.open }
