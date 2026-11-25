// Package a11y holds the accessibility node type of the aqua driver
// (PLAN.md §7.5): the role, label, value, states, actions, frame and
// children each drawn element exposes. aqua.AccessibilityTree builds a
// window's tree of nodes from its elements.
//
// There is no screen-reader bridge yet (decision D4): the tree is built and
// tested from day one, and the UI Automation / AT-SPI bridge of phase 7
// reads it.
//
// The package uses only the standard library and internal/driver.
package a11y

import (
	"strconv"
	"strings"

	"github.com/radix29/gogui/internal/driver"
)

// Role is what kind of user-interface object a node is. The roles follow
// the v0.1 controls; the bridge maps them to UIA control types and AT-SPI
// roles.
type Role uint8

const (
	// RoleNone marks an element that is not exposed (a decoration, a layout
	// helper). Its children are exposed as children of its nearest exposed
	// ancestor.
	RoleNone Role = iota
	RoleWindow
	RoleGroup // stacks, forms, Box
	RoleStaticText
	RoleButton
	RoleCheckBox
	RoleRadioButton
	RoleTextField
	RoleTextArea // TextView
	RoleSlider
	RoleProgressIndicator
	RolePopUpButton
	RoleSeparator
	RoleScrollArea
	RoleTitleBar
	RoleMenuBar     // the title-bar menu strip (S5.6c)
	RoleMenuBarItem // one title of the strip
	RoleMenu        // an open menu
	RoleMenuItem    // one item of a menu
	RoleDialog      // an alert (S5.7a)
)

var roleNames = [...]string{
	RoleNone:              "none",
	RoleWindow:            "window",
	RoleGroup:             "group",
	RoleStaticText:        "text",
	RoleButton:            "button",
	RoleCheckBox:          "checkbox",
	RoleRadioButton:       "radio",
	RoleTextField:         "textfield",
	RoleTextArea:          "textarea",
	RoleSlider:            "slider",
	RoleProgressIndicator: "progress",
	RolePopUpButton:       "popup",
	RoleSeparator:         "separator",
	RoleScrollArea:        "scrollarea",
	RoleTitleBar:          "titlebar",
	RoleMenuBar:           "menubar",
	RoleMenuBarItem:       "menubaritem",
	RoleMenu:              "menu",
	RoleMenuItem:          "menuitem",
	RoleDialog:            "dialog",
}

// String returns the role's short name ("button"), or "role(n)" for an
// unknown value.
func (r Role) String() string {
	if int(r) < len(roleNames) {
		return roleNames[r]
	}
	return "role(" + strconv.Itoa(int(r)) + ")"
}

// States is a set of node states.
type States uint16

const (
	StateDisabled  States = 1 << iota // the element or an ancestor is disabled
	StateFocusable                    // the element can take keyboard focus
	StateFocused                      // the element has the window's focus
	StateChecked                      // a checkbox or radio button is on
	StateMixed                        // a checkbox is in the mixed state
	StateSelected                     // the selected item of a group
	StateReadOnly                     // text that can be selected but not edited
	StateExpanded                     // a pop-up button's menu is open
)

var stateNames = [...]string{
	"disabled", "focusable", "focused", "checked", "mixed", "selected", "readonly", "expanded",
}

// String returns the states as space-separated names in bit order
// ("focusable focused"), or "" for the empty set.
func (s States) String() string { return bitNames(uint16(s), stateNames[:]) }

// Actions is a set of actions a node offers to assistive technology.
type Actions uint8

const (
	ActionPress     Actions = 1 << iota // click a button, toggle a checkbox
	ActionFocus                         // move the keyboard focus to the element
	ActionIncrement                     // a slider step up
	ActionDecrement                     // a slider step down
	ActionShowMenu                      // open a pop-up button's menu
)

var actionNames = [...]string{"press", "focus", "increment", "decrement", "showmenu"}

// String returns the actions as space-separated names in bit order
// ("press focus"), or "" for the empty set.
func (a Actions) String() string { return bitNames(uint16(a), actionNames[:]) }

func bitNames(v uint16, names []string) string {
	var parts []string
	for i := 0; v != 0; i++ {
		if v&1 != 0 {
			if i < len(names) {
				parts = append(parts, names[i])
			} else {
				parts = append(parts, "bit"+strconv.Itoa(i))
			}
		}
		v >>= 1
	}
	return strings.Join(parts, " ")
}

// Node is one accessible object.
//
// An element's AccessibleNode fills in Role, Label, Value and its own
// States (checked, mixed …) and Actions. aqua.AccessibilityTree adds
// Frame, Children and the states the tree knows (disabled, focusable,
// focused).
type Node struct {
	Role Role

	// Label is the name read out ("OK", the text of a label, a window's
	// title).
	Label string

	// Value is the current value as text: a text field's contents, a
	// slider's or progress indicator's number, a pop-up's selected item.
	Value string

	States  States
	Actions Actions

	// Frame is the node's rectangle in window coordinates (the coordinates
	// of the root element's parent: the surface's, for a window).
	Frame driver.Rect

	// Children are the exposed descendants in tree (paint) order.
	Children []*Node
}

// Walk calls f for n and its descendants in pre-order with their depth (0
// for n). When f returns false the children of that node are skipped.
func (n *Node) Walk(f func(n *Node, depth int) bool) { n.walk(f, 0) }

func (n *Node) walk(f func(*Node, int) bool, depth int) {
	if !f(n, depth) {
		return
	}
	for _, c := range n.Children {
		c.walk(f, depth+1)
	}
}

// Find returns the first node in pre-order with the role and label, or nil.
// Only tests call it (of aqua, its gallery and gogui), so it stays here
// rather than in four copies.
func (n *Node) Find(role Role, label string) *Node {
	var found *Node
	n.Walk(func(m *Node, _ int) bool {
		if found == nil && m.Role == role && m.Label == label {
			found = m
		}
		return found == nil
	})
	return found
}

// String returns the node itself on one line, without its children:
//
//	button "OK" value="1" [focusable focused] {press} @10,20 80x22
//
// The label is always quoted; value, states and actions appear only when
// not empty.
func (n *Node) String() string {
	var b strings.Builder
	b.WriteString(n.Role.String())
	b.WriteByte(' ')
	b.WriteString(strconv.Quote(n.Label))
	if n.Value != "" {
		b.WriteString(" value=" + strconv.Quote(n.Value))
	}
	if n.States != 0 {
		b.WriteString(" [" + n.States.String() + "]")
	}
	if n.Actions != 0 {
		b.WriteString(" {" + n.Actions.String() + "}")
	}
	f := n.Frame
	b.WriteString(" @" + num(f.X) + "," + num(f.Y) + " " + num(f.W) + "x" + num(f.H))
	return b.String()
}

// Dump returns the tree as one String line per node, indented by two
// spaces per level: a stable text form for tests.
func (n *Node) Dump() string {
	var lines []string
	n.Walk(func(m *Node, depth int) bool {
		lines = append(lines, strings.Repeat("  ", depth)+m.String())
		return true
	})
	return strings.Join(lines, "\n")
}

func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
