package gogui

import (
	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/layout"
)

// CheckState is the state of a checkbox. The values match
// NSControlStateValue.
type CheckState = driver.CheckState

// Checkbox states. StateMixed shows a dash; the user can't choose it, but
// a click on a mixed checkbox turns it on.
const (
	StateOff   = driver.StateOff
	StateOn    = driver.StateOn
	StateMixed = driver.StateMixed
)

// Checkbox is a check box with a title (NSButton with the switch style).
// It doesn't expand.
type Checkbox struct {
	common[*Checkbox]
	p        driver.Checkbox
	title    string
	state    CheckState
	onChange subscribers[func(on bool)]
}

// NewCheckbox returns an unchecked checkbox with title.
func NewCheckbox(title string) *Checkbox {
	c := &Checkbox{title: title}
	p := driverFor("NewCheckbox").NewCheckbox(checkboxEvents{c})
	c.p = p
	c.init(c, kindCheckbox, p, nil)
	p.SetTitle(title)
	return c
}

// SetTitle sets the title shown next to the box.
func (c *Checkbox) SetTitle(title string) *Checkbox {
	c.check("SetTitle")
	c.title = title
	if c.live() {
		c.p.SetTitle(title)
	}
	c.invalidate()
	return c
}

// Title returns the title.
func (c *Checkbox) Title() string { c.get("Title"); return c.title }

// SetChecked turns the checkbox on or off. It doesn't call the OnChange
// handlers.
func (c *Checkbox) SetChecked(on bool) *Checkbox {
	c.check("SetChecked")
	if on {
		return c.setState(StateOn)
	}
	return c.setState(StateOff)
}

// Checked reports whether the checkbox is on. A mixed checkbox counts as
// on, as in OnChange.
func (c *Checkbox) Checked() bool { c.get("Checked"); return c.state != StateOff }

// SetState sets the state, including StateMixed. Values other than the
// three states count as StateOn. It doesn't call the OnChange handlers.
func (c *Checkbox) SetState(s CheckState) *Checkbox {
	c.check("SetState")
	return c.setState(s)
}

// State returns the state.
func (c *Checkbox) State() CheckState { c.get("State"); return c.state }

func (c *Checkbox) setState(s CheckState) *Checkbox {
	if s != StateOff && s != StateMixed {
		s = StateOn
	}
	c.state = s
	if c.live() {
		c.p.SetState(s)
	}
	return c
}

// OnChange calls fn with the new value when the user toggles the
// checkbox. A click on a mixed checkbox turns it on, so fn gets true. A
// disabled checkbox doesn't call it.
func (c *Checkbox) OnChange(fn func(on bool)) Unsubscribe {
	c.check("OnChange")
	return c.onChange.add(fn)
}

// checkboxEvents implements driver.ToggleHandler for a Checkbox.
type checkboxEvents struct{ c *Checkbox }

func (e checkboxEvents) StateChanged(s CheckState) {
	c := e.c
	if c.dead {
		return
	}
	c.state = s // the cache follows the native state even when disabled (B14)
	if c.disabled {
		return
	}
	on := s != StateOff
	c.onChange.eachLive(&c.widget, func(fn func(bool)) { fn(on) })
}

// RadioGroup is a column of radio buttons, 6 points apart, of which at
// most one is selected (NSButton radios that share an action). It doesn't
// expand. The group is one Tab stop, its selected radio; the arrow keys
// move the selection (and call OnChange).
type RadioGroup struct {
	common[*RadioGroup]
	c        driver.Container
	radios   []*radio
	selected int
	onChange subscribers[func(i int)]
}

// radio is one radio button of a group. It is not public; the group
// creates, places and destroys it.
type radio struct {
	widget
	p driver.Radio
}

// NewRadioGroup returns a group with one radio button per item, top to
// bottom. The first one is selected (none if there are no items).
func NewRadioGroup(items ...string) *RadioGroup {
	d := driverFor("NewRadioGroup")
	c := d.NewContainer()
	g := &RadioGroup{c: c, selected: -1}
	nodes := make([]layout.Node, len(items))
	peers := make([]driver.Widget, len(items))
	for i, item := range items {
		p := d.NewRadio(radioEvents{g, i})
		p.SetTitle(item)
		r := &radio{p: p}
		r.kind, r.peer, r.n, r.parent, r.self = kindRadio, p, layout.NewLeaf(p), g, r
		r.applyExpand()
		g.radios = append(g.radios, r)
		nodes[i], peers[i] = r.n, p
	}
	g.init(g, kindRadioGroup, c, layout.NewVStack(6, nodes...).SetPeer(c))
	c.SetChildren(peers)
	if len(items) > 0 {
		g.setSelected(0)
	}
	return g
}

// SetSelected selects the radio at index i and turns the others off. An
// index outside the items (such as -1) selects none. It doesn't call the
// OnChange handlers.
func (g *RadioGroup) SetSelected(i int) *RadioGroup {
	g.check("SetSelected")
	g.setSelected(i)
	return g
}

// Selected returns the index of the selected radio, or -1 if none is.
func (g *RadioGroup) Selected() int { g.get("Selected"); return g.selected }

func (g *RadioGroup) setSelected(i int) {
	if i < 0 || i >= len(g.radios) {
		i = -1
	}
	g.selected = i
	for j, r := range g.radios {
		if r.live() {
			r.p.SetOn(j == i)
		}
	}
}

// SetEnabled enables or disables every radio of the group.
func (g *RadioGroup) SetEnabled(enabled bool) *RadioGroup {
	g.common.SetEnabled(enabled)
	for _, r := range g.radios {
		r.disabled = !enabled
		if r.live() {
			r.p.SetEnabled(enabled)
		}
	}
	return g
}

// SetTooltip sets the tooltip of the group and of every radio in it, so it
// shows wherever the pointer rests on the group.
func (g *RadioGroup) SetTooltip(text string) *RadioGroup {
	g.common.SetTooltip(text)
	for _, r := range g.radios {
		if r.live() {
			r.p.SetTooltip(text)
		}
	}
	return g
}

// SetControlSize sets the size class of every radio of the group.
func (g *RadioGroup) SetControlSize(cs ControlSize) *RadioGroup {
	g.common.SetControlSize(cs)
	for _, r := range g.radios {
		if r.live() {
			r.p.SetControlSize(cs)
		}
	}
	return g
}

// Focus focuses the selected radio, or the first one if none is selected.
func (g *RadioGroup) Focus() *RadioGroup {
	g.check("Focus")
	if len(g.radios) == 0 {
		return g
	}
	r := g.radios[max(g.selected, 0)]
	if r.live() {
		r.p.Focus()
	}
	return g
}

// OnChange calls fn with the index of the radio the user selected, when
// the selection changes. A click on the selected radio, or on any radio of
// a disabled group, doesn't call it.
func (g *RadioGroup) OnChange(fn func(i int)) Unsubscribe {
	g.check("OnChange")
	return g.onChange.add(fn)
}

// childWidgets returns the radios, so the window destroys them before the
// group's container.
func (g *RadioGroup) childWidgets() []Widget {
	out := make([]Widget, len(g.radios))
	for i, r := range g.radios {
		out[i] = r
	}
	return out
}

// removeChild does nothing: the radios are private, so they can't be
// moved to another parent.
func (g *RadioGroup) removeChild(Widget) {}

// radioEvents implements driver.ToggleHandler for radio i of a group.
type radioEvents struct {
	g *RadioGroup
	i int
}

func (e radioEvents) StateChanged(s CheckState) {
	g := e.g
	if s != StateOn || g.dead {
		return
	}
	changed := g.selected != e.i
	g.setSelected(e.i) // also turns the others off, even when disabled (B14)
	if changed && !g.disabled {
		g.onChange.eachLive(&g.widget, func(fn func(int)) { fn(e.i) })
	}
}
