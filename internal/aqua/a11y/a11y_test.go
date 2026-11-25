package a11y

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
)

func TestStrings(t *testing.T) {
	for r, want := range map[Role]string{RoleNone: "none", RoleButton: "button", RoleTitleBar: "titlebar", 200: "role(200)"} {
		if got := r.String(); got != want {
			t.Errorf("Role(%d) = %q, want %q", r, got, want)
		}
	}
	if got := (StateFocusable | StateChecked | 1<<12).String(); got != "focusable checked bit12" {
		t.Errorf("States %q", got)
	}
	if States(0).String() != "" || Actions(0).String() != "" {
		t.Error("empty sets not empty")
	}
	if got := (ActionPress | ActionShowMenu).String(); got != "press showmenu" {
		t.Errorf("Actions %q", got)
	}
	// Every named bit has a name.
	if len(stateNames) != 8 || len(actionNames) != 5 || len(roleNames) != int(RoleDialog)+1 {
		t.Error("name tables out of date")
	}
}

func TestNode(t *testing.T) {
	ok := &Node{Role: RoleButton, Label: "OK", States: StateFocused, Actions: ActionPress,
		Frame: driver.Rect{X: 1.5, Y: 2, W: 80, H: 22}}
	field := &Node{Role: RoleTextField, Label: `say "hi"`, Value: "x"}
	inner := &Node{Role: RoleStaticText, Label: "Hi"}
	group := &Node{Role: RoleGroup, Children: []*Node{inner}}
	root := &Node{Role: RoleWindow, Label: "W", Children: []*Node{ok, group, field}}

	if got := ok.String(); got != `button "OK" [focused] {press} @1.5,2 80x22` {
		t.Errorf("String %q", got)
	}
	want := "window \"W\" @0,0 0x0\n" +
		"  button \"OK\" [focused] {press} @1.5,2 80x22\n" +
		"  group \"\" @0,0 0x0\n" +
		"    text \"Hi\" @0,0 0x0\n" +
		"  textfield \"say \\\"hi\\\"\" value=\"x\" @0,0 0x0"
	if got := root.Dump(); got != want {
		t.Errorf("Dump:\n%s\nwant:\n%s", got, want)
	}
	if root.Find(RoleStaticText, "Hi") != inner || root.Find(RoleButton, "Cancel") != nil {
		t.Error("Find")
	}
	// Walk skips the children of a node when f returns false.
	var seen []Role
	root.Walk(func(n *Node, depth int) bool {
		seen = append(seen, n.Role)
		return n.Role != RoleGroup
	})
	if len(seen) != 4 {
		t.Errorf("Walk visited %v", seen)
	}
}
