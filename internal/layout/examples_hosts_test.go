package layout

import (
	"fmt"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
)

// notesTree is examples/notes:
//
//	VStack(0,
//		HStack(0, sidebar, NewSeparator().SetVertical(true), editor),
//		NewSeparator(),
//		HStack(8, stats, Spacer()).Padding(6),
//	)
//
// where sidebar is a vertically scrolling ScrollView (min width 180) of
// VStack(8, 30 checkboxes).Padding(12).
type notesTree struct {
	root                       *Root
	vstack, split, doc, status *stub.Container
	sidebar                    *stub.ScrollView
	options                    [30]*stub.Checkbox
	vsep, hsep                 *stub.Separator
	editor                     *stub.TextView
	stats                      *stub.Label
}

// layoutNotes builds the notes tree and lays it out at size.
func layoutNotes(size driver.Size) notesTree {
	var t notesTree
	var opts []Node
	for i := range t.options {
		t.options[i] = checkbox(fmt.Sprintf("Option %d", i+1))
		opts = append(opts, ctl(t.options[i]))
	}
	doc, dpeer := withPeer(NewVStack(8, opts...))
	doc.SetPadding(driver.UniformInsets(12))
	t.sidebar = stub.NewScrollView(nil)
	t.sidebar.HasVertical = true
	sidebar := NewScrollView(t.sidebar, doc).SetScrolls(Vertical, true)
	h, v := defaultExpand(t.sidebar)
	sidebar.Props().SetMin(Horizontal, 180).SetExpand(Horizontal, h).SetExpand(Vertical, v)

	t.vsep, t.hsep = stub.NewSeparator(), stub.NewSeparator()
	t.vsep.Vertical = true
	t.editor = stub.NewTextView(nil)
	// The text of the welcome note: 16 words, 89 characters.
	t.stats = lbl("16 words, 89 characters")

	split, speer := withPeer(NewHStack(0, sidebar, ctl(t.vsep), ctl(t.editor)))
	status, stpeer := withPeer(NewHStack(8, ctl(t.stats), NewSpacer()))
	status.SetPadding(driver.UniformInsets(6))
	col, vpeer := withPeer(NewVStack(0, split, ctl(t.hsep), status))
	t.root, t.vstack, t.split, t.doc, t.status = NewRoot(col), vpeer, speer, dpeer, stpeer
	t.root.Layout(size)
	return t
}

func TestExampleNotes(t *testing.T) {
	n := layoutNotes(driver.Size{W: 720, H: 480})
	wantRect(t, "vstack", n.vstack.Frame, driver.Rect{W: 720, H: 480})
	// Only the split view expands vertically: it takes everything but the
	// separator (1) and the status bar (16 + 2 × 6 = 28).
	wantRect(t, "split", n.split.Frame, driver.Rect{W: 720, H: 451})
	wantRect(t, "separator", n.hsep.Frame, driver.Rect{Y: 451, W: 720, H: 1})
	wantRect(t, "status bar", n.status.Frame, driver.Rect{Y: 452, W: 720, H: 28})
	// The sidebar keeps its 180 pt min width, the vertical separator is
	// 1 pt, and the editor fills the rest; all three fill the height.
	wantRect(t, "sidebar", n.sidebar.Frame, driver.Rect{W: 180, H: 451})
	wantRect(t, "vertical separator", n.vsep.Frame, driver.Rect{X: 180, W: 1, H: 451})
	wantRect(t, "editor", n.editor.Frame, driver.Rect{X: 181, W: 539, H: 451})
	// The document is the viewport width (it doesn't scroll horizontally)
	// and 30 × 18 + 29 × 8 + 2 × 12 = 796 high, longer than the viewport.
	wantSize(t, "document size", n.sidebar.DocumentSize, driver.Size{W: 180, H: 796})
	wantRect(t, "document", n.doc.Frame, driver.Rect{W: 180, H: 796})
	for i, c := range n.options {
		w := 18.0 + 8*7 // "Option 1" … "Option 9"
		if i >= 9 {
			w += 7
		}
		wantRect(t, c.Title, c.Frame, driver.Rect{X: 12, Y: 12 + 26*float64(i), W: w, H: 18})
	}
	// The status label (23 runes) sits in the 6 pt padding; the spacer
	// takes the rest of the row.
	wantRect(t, "stats", n.stats.Frame, driver.Rect{X: 6, Y: 6, W: 161, H: 16})
}

// titledBox returns a box node with a titled stub peer.
func titledBox(title string, content Node) (*Box, *stub.Box) {
	peer := stub.NewBox()
	peer.Title = title
	return NewBox(peer, content), peer
}

// controlsTree is examples/controls:
//
//	VStack(14,
//		Box("Buttons", HStack(8, push, def, disabled, Spacer())),
//		Box("Toggles", HStack(20, VStack(6, check, mixed, checkOff), radios, Spacer())),
//		Box("Text", VStack(8, field, secure)),
//		Box("Values", VStack(10, slider, progress, HStack(8, busy, Spacer(), popup))),
//		NewSeparator(),
//		HStack(8, status, Spacer(), alert),
//	).Padding(20)
type controlsTree struct {
	root                       *Root
	vstack                     *stub.Container
	boxes                      [4]*stub.Box
	buttonRow, toggleRow       *stub.Container
	checks, radioGroup         *stub.Container
	textCol, valueCol, busyRow *stub.Container
	statusRow                  *stub.Container
	push, def, disabled, alert *stub.Button
	check, mixed, checkOff     *stub.Checkbox
	radios                     [3]*stub.Radio
	field, secure              *stub.TextField
	slider                     *stub.Slider
	progress, busy             *stub.Progress
	popup                      *stub.PopUpButton
	sep                        *stub.Separator
	status                     *stub.Label
}

// layoutControls builds the controls tree and lays it out at size.
func layoutControls(size driver.Size) controlsTree {
	var t controlsTree
	t.push, t.def, t.disabled = button("Push Button"), button("Default"), button("Disabled")
	t.def.IsDefault, t.disabled.Disabled = true, true
	t.check, t.mixed, t.checkOff = checkbox("Checkbox"), checkbox("Mixed"), checkbox("Disabled")
	var radios []Node
	for i, s := range []string{"Small", "Medium", "Large"} {
		t.radios[i] = stub.NewRadio(nil)
		t.radios[i].Title = s
		radios = append(radios, ctl(t.radios[i]))
	}
	t.field, t.secure = stub.NewTextField(false, nil), stub.NewTextField(true, nil)
	t.slider, t.progress, t.busy = stub.NewSlider(nil), stub.NewProgress(), stub.NewProgress()
	t.busy.Indeterminate = true
	t.popup = stub.NewPopUpButton(nil)
	t.popup.SetItems([]string{"Red", "Green", "Blue"})
	t.sep, t.status, t.alert = stub.NewSeparator(), lbl("Ready"), button("Show Alert…")

	var row1, row2, checks, group, text, values, busyRow, statusRow *Stack
	row1, t.buttonRow = withPeer(NewHStack(8, ctl(t.push), ctl(t.def), ctl(t.disabled), NewSpacer()))
	checks, t.checks = withPeer(NewVStack(6, ctl(t.check), ctl(t.mixed), ctl(t.checkOff)))
	group, t.radioGroup = withPeer(NewVStack(6, radios...))
	row2, t.toggleRow = withPeer(NewHStack(20, checks, group, NewSpacer()))
	text, t.textCol = withPeer(NewVStack(8, ctl(t.field), ctl(t.secure)))
	busyRow, t.busyRow = withPeer(NewHStack(8, ctl(t.busy), NewSpacer(), ctl(t.popup)))
	values, t.valueCol = withPeer(NewVStack(10, ctl(t.slider), ctl(t.progress), busyRow))
	statusRow, t.statusRow = withPeer(NewHStack(8, ctl(t.status), NewSpacer(), ctl(t.alert)))

	var nodes []Node
	for i, c := range []struct {
		title   string
		content Node
	}{{"Buttons", row1}, {"Toggles", row2}, {"Text", text}, {"Values", values}} {
		var b *Box
		b, t.boxes[i] = titledBox(c.title, c.content)
		nodes = append(nodes, b)
	}
	col, vpeer := withPeer(NewVStack(14, append(nodes, ctl(t.sep), statusRow)...))
	col.SetPadding(driver.UniformInsets(20))
	t.root, t.vstack = NewRoot(col), vpeer
	t.root.Layout(size)
	return t
}

func TestExampleControls(t *testing.T) {
	c := layoutControls(driver.Size{W: 520, H: 560})
	wantRect(t, "vstack", c.vstack.Frame, driver.Rect{W: 520, H: 560})
	// Every box fills the 480 pt inner width, because its content expands
	// (a spacer or a text field). A titled stub box adds 16 across and
	// 8 + 16 + 8 = 32 down. Nothing expands vertically, so the leftover
	// 520 − (54 + 98 + 84 + 118 + 1 + 22 + 5 × 14) = 73 stays at the bottom.
	boxes := []driver.Rect{
		{X: 20, Y: 20, W: 480, H: 54}, {X: 20, Y: 88, W: 480, H: 98},
		{X: 20, Y: 200, W: 480, H: 84}, {X: 20, Y: 298, W: 480, H: 118},
	}
	for i, want := range boxes {
		wantRect(t, c.boxes[i].Title, c.boxes[i].Frame, want)
	}
	// Each content is framed at the origin of its box's content area, in
	// the 464 pt inner width.
	wantRect(t, "button row", c.buttonRow.Frame, driver.Rect{W: 464, H: 22})
	wantRect(t, "push", c.push.Frame, driver.Rect{W: 105, H: 22})
	wantRect(t, "default", c.def.Frame, driver.Rect{X: 113, W: 77, H: 22})
	wantRect(t, "disabled", c.disabled.Frame, driver.Rect{X: 198, W: 84, H: 22})

	// Toggles: the checkbox column (74 × 66) and the radio group (60 × 66)
	// 20 pt apart, the spacer taking the rest.
	wantRect(t, "toggle row", c.toggleRow.Frame, driver.Rect{W: 464, H: 66})
	wantRect(t, "checkboxes", c.checks.Frame, driver.Rect{W: 74, H: 66})
	wantRect(t, "radio group", c.radioGroup.Frame, driver.Rect{X: 94, W: 60, H: 66})
	wantRect(t, "checkbox", c.check.Frame, driver.Rect{W: 74, H: 18})
	wantRect(t, "mixed", c.mixed.Frame, driver.Rect{Y: 24, W: 53, H: 18})
	wantRect(t, "checkbox off", c.checkOff.Frame, driver.Rect{Y: 48, W: 74, H: 18})
	radios := []driver.Rect{{W: 53, H: 18}, {Y: 24, W: 60, H: 18}, {Y: 48, W: 53, H: 18}}
	for i, want := range radios {
		wantRect(t, c.radios[i].Title, c.radios[i].Frame, want)
	}

	wantRect(t, "text column", c.textCol.Frame, driver.Rect{W: 464, H: 52})
	wantRect(t, "field", c.field.Frame, driver.Rect{W: 464, H: 22})
	wantRect(t, "secure", c.secure.Frame, driver.Rect{Y: 30, W: 464, H: 22})

	wantRect(t, "value column", c.valueCol.Frame, driver.Rect{W: 464, H: 86})
	wantRect(t, "slider", c.slider.Frame, driver.Rect{W: 464, H: 22})
	wantRect(t, "progress", c.progress.Frame, driver.Rect{Y: 32, W: 464, H: 22})
	wantRect(t, "busy row", c.busyRow.Frame, driver.Rect{Y: 64, W: 464, H: 22})
	// The indeterminate bar expands, so it shares the leftover
	// 464 − (96 + 67 + 2 × 8) = 285 equally with the spacer: 142.5 each.
	wantRect(t, "busy", c.busy.Frame, driver.Rect{W: 96 + 142.5, H: 22})
	wantRect(t, "popup", c.popup.Frame, driver.Rect{X: 397, W: 67, H: 22})

	wantRect(t, "separator", c.sep.Frame, driver.Rect{X: 20, Y: 430, W: 480, H: 1})
	// The status row: "Ready" (35 × 16, centered) and the trailing alert
	// button (105).
	wantRect(t, "status row", c.statusRow.Frame, driver.Rect{X: 20, Y: 445, W: 480, H: 22})
	wantRect(t, "status", c.status.Frame, driver.Rect{Y: 3, W: 35, H: 16})
	wantRect(t, "alert", c.alert.Frame, driver.Rect{X: 375, W: 105, H: 22})
}
