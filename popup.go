package gogui

import (
	"slices"

	"github.com/radix29/gogui/internal/driver"
)

// PopUpButton is a pop-up menu button (NSPopUpButton). Its menu opens over
// the selected item. It doesn't expand.
type PopUpButton struct {
	common[*PopUpButton]
	p        driver.PopUpButton
	items    []string
	selected int
	onChange subscribers[func(int)]
}

// NewPopUpButton returns a pop-up button with items. The first item is
// selected (none if there are no items).
func NewPopUpButton(items ...string) *PopUpButton {
	b := &PopUpButton{selected: -1}
	p := driverFor("NewPopUpButton").NewPopUpButton(popUpEvents{b})
	b.p = p
	b.init(b, kindPopUp, p, nil)
	b.setItems(items)
	return b
}

// SetItems replaces the items. The selection is kept if it is still in
// range, and otherwise reset to the first item (or none when there are no
// items). It doesn't call the OnChange handlers.
func (b *PopUpButton) SetItems(items ...string) *PopUpButton {
	b.check("SetItems")
	b.setItems(items)
	b.invalidate()
	return b
}

func (b *PopUpButton) setItems(items []string) {
	b.items = slices.Clone(items)
	switch {
	case len(b.items) == 0:
		b.selected = -1
	case b.selected < 0 || b.selected >= len(b.items):
		b.selected = 0
	}
	if b.live() {
		b.p.SetItems(slices.Clone(b.items))
		b.p.SetSelected(b.selected)
	}
}

// Items returns a copy of the items.
func (b *PopUpButton) Items() []string { b.check("Items"); return slices.Clone(b.items) }

// Item returns the item at index i, or "" if i is out of range (such as
// -1 for no selection).
func (b *PopUpButton) Item(i int) string {
	b.check("Item")
	if i < 0 || i >= len(b.items) {
		return ""
	}
	return b.items[i]
}

// SetSelected selects the item at index i. An index outside the items
// (such as -1) selects none. It doesn't call the OnChange handlers.
func (b *PopUpButton) SetSelected(i int) *PopUpButton {
	b.check("SetSelected")
	if i < 0 || i >= len(b.items) {
		i = -1
	}
	b.selected = i
	if b.live() {
		b.p.SetSelected(i)
	}
	return b
}

// Selected returns the index of the selected item, or -1 if none is.
func (b *PopUpButton) Selected() int { b.check("Selected"); return b.selected }

// OnChange calls fn with the index of the item the user picked, when the
// selection changes. Picking the selected item again (which AppKit
// reports), SetSelected and a disabled pop-up button don't call it.
func (b *PopUpButton) OnChange(fn func(i int)) Unsubscribe {
	b.check("OnChange")
	return b.onChange.add(fn)
}

// popUpEvents implements driver.PopUpHandler for a PopUpButton.
type popUpEvents struct{ b *PopUpButton }

func (e popUpEvents) SelectionChanged(i int) {
	b := e.b
	if b.disabled || b.dead || i < 0 || i >= len(b.items) {
		return
	}
	changed := b.selected != i
	b.selected = i
	if changed {
		b.onChange.each(func(fn func(int)) { fn(i) })
	}
}
