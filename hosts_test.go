package gogui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/driver/stub"
	"github.com/radix29/gogui/internal/layout"
)

func TestBox(t *testing.T) {
	runApp(t, func(app *App) {
		b := NewBox("Buttons")
		p := b.peer.(*stub.Box)
		if p.Title != "Buttons" || b.Title() != "Buttons" || b.Content() != nil {
			t.Fatalf("new box: %+v", p)
		}
		if b.node().Expands(layout.Horizontal) || b.node().Expands(layout.Vertical) {
			t.Error("an empty box expands")
		}
		ok := NewButton("OK")
		var got *Box = b.SetContent(ok)
		if got != b || p.Content != ok.peer || b.Content() != ok || ok.parent != Widget(b) {
			t.Fatalf("SetContent: peer content %v", p.Content)
		}
		field := NewTextField()
		b.SetContent(field) // replaces; the old content is not destroyed
		if p.Content != field.peer || ok.parent != nil || stubBase(ok).Destroyed {
			t.Error("replacing the content")
		}
		if !b.node().Expands(layout.Horizontal) {
			t.Error("a box doesn't expand with its content")
		}

		// The content's frame follows the insets, which follow the title.
		win, _ := newTestWindow(app, WindowConfig{Size: Size{W: 200, H: 100}})
		win.SetContent(VStack(0, b))
		drain(app)
		wantFrame(t, "titled content", field, Rect{X: 0, Y: 0, W: 184, H: 22})
		wantFrame(t, "titled box", b, Rect{W: 200, H: 22 + 32})
		passes := win.passes
		b.SetTitle("")
		if !win.pending {
			t.Error("SetTitle didn't mark the window dirty")
		}
		drain(app)
		if win.passes != passes+1 || p.Title != "" {
			t.Errorf("%d passes after SetTitle, title %q", win.passes-passes, p.Title)
		}
		wantFrame(t, "untitled box", b, Rect{W: 200, H: 22 + 16})

		// Moving the content into a stack takes it out of the box.
		VStack(0, field)
		if p.Content != nil || b.Content() != nil || b.n.Content() != nil {
			t.Error("moved content stays in the box")
		}
		b.SetContent(field)
		b.SetContent(nil)
		if p.Content != nil || field.parent != nil {
			t.Error("SetContent(nil)")
		}
		b.SetContent(field)
		win.Close() // destroys content first (destroyTree via childWidgets)
		if !stubBase(field).Destroyed || !p.Destroyed {
			t.Error("closing the window didn't destroy the box and its content")
		}
		mustPanic(t, "Box.SetTitle on a destroyed widget", func() { b.SetTitle("after") })
		mustPanic(t, "Box.SetContent on a destroyed widget", func() { b.SetContent(nil) })
		if p.Title != "" || b.Title() != "" {
			t.Error("a destroyed box called its peer or lost its title")
		}

		func() {
			defer func() {
				if recover() == nil {
					t.Error("a box inside its own content didn't panic")
				}
			}()
			outer := NewBox("outer")
			outer.SetContent(VStack(0, NewBox("inner")))
			outer.Content().(*Stack).children[0].(*Box).SetContent(outer)
		}()
	})
}

// notesTree builds examples/notes (without the menus) in a window.
func notesTree(app *App) (win *Window, sidebar *ScrollView, settings *Stack, editor *TextView) {
	editor = NewTextView().SetFont(SystemFont(13))
	settings = VStack(8)
	for i := 1; i <= 30; i++ {
		settings.Add(NewCheckbox(fmt.Sprintf("Option %d", i)))
	}
	sidebar = NewScrollView(settings.SetPadding(12)).SetHasVerticalScroller(true).SetMinWidth(180)
	win = app.NewWindow(WindowConfig{Size: Size{W: 720, H: 480}, MinSize: Size{W: 480, H: 320}})
	win.SetContent(VStack(0,
		HStack(0, sidebar, NewSeparator().SetVertical(true), editor),
		NewSeparator(),
		HStack(8, NewLabel("16 words, 89 characters"), Spacer()).SetPadding(6),
	))
	return win, sidebar, settings, editor
}

func TestScrollViewNotes(t *testing.T) {
	runApp(t, func(app *App) {
		win, sidebar, settings, editor := notesTree(app)
		p := sidebar.peer.(*stub.ScrollView)
		if p.Document != settings.peer || !p.HasVertical || p.HasHorizontal {
			t.Fatalf("sidebar peer: %+v", p)
		}
		drain(app)
		// The S0.4h numbers (internal/layout TestExampleNotes).
		wantFrame(t, "sidebar", sidebar, Rect{W: 180, H: 451})
		wantFrame(t, "editor", editor, Rect{X: 181, W: 539, H: 451})
		wantFrame(t, "document", settings, Rect{W: 180, H: 796})
		if p.DocumentSize != (Size{W: 180, H: 796}) {
			t.Errorf("document size %v, want 180 × 796", p.DocumentSize)
		}

		// Legacy scrollers appear: the viewport shrinks without SetFrame,
		// and the next pass fits the document to it.
		p.UserResizeViewport(driver.Insets{Right: 15})
		if !win.pending {
			t.Fatal("ViewportResized didn't mark the window dirty")
		}
		drain(app)
		if p.DocumentSize != (Size{W: 165, H: 796}) {
			t.Errorf("document size after the viewport change %v, want 165 × 796", p.DocumentSize)
		}
		wantFrame(t, "document after the viewport change", settings, Rect{W: 165, H: 796})

		// Scrolling: user scrolls fire, SetOffset doesn't; both clamp.
		var log []Point
		un := sidebar.OnScroll(func(o Point) { log = append(log, o) })
		p.UserScroll(Point{Y: 100})
		sidebar.SetOffset(Point{X: 50, Y: 1000})
		if want := (Point{Y: 796 - 451}); sidebar.Offset() != want || p.Off != want {
			t.Errorf("Offset after SetOffset = %v, want %v", sidebar.Offset(), want)
		}
		un()
		un()
		p.UserScroll(Point{Y: 10})
		if !slices.Equal(log, []Point{{Y: 100}}) {
			t.Errorf("scroll events %v", log)
		}

		// Closing destroys the document before the scroll view.
		win.Close()
		if !stubBase(settings).Destroyed || !p.Destroyed || sidebar.Offset() != (Point{Y: 10}) {
			t.Error("closing the window: document, scroll view or last offset")
		}
		scrollEvents{sidebar}.Scrolled(Point{Y: 5})
		if len(log) != 1 || sidebar.Offset() != (Point{Y: 10}) {
			t.Error("a destroyed scroll view reported a scroll")
		}
	})
}

func TestScrollViewFlags(t *testing.T) {
	runApp(t, func(*App) {
		s := NewScrollView(nil)
		p := s.peer.(*stub.ScrollView)
		h := &countingHost{}
		s.host = h
		expands := func() (bool, bool) {
			return s.node().Expands(layout.Horizontal), s.node().Expands(layout.Vertical)
		}
		if x, y := expands(); x || y || p.Document != nil || s.Document() != nil {
			t.Fatal("a new scroll view expands or has a document")
		}
		var got *ScrollView = s.SetHasHorizontalScroller(true)
		if x, y := expands(); got != s || !x || y || !p.HasHorizontal || !s.n.Scrolls(layout.Horizontal) {
			t.Error("horizontal scroller")
		}
		s.SetHasVerticalScroller(true).SetHasHorizontalScroller(false)
		if x, y := expands(); x || !y || p.HasHorizontal || !p.HasVertical ||
			s.n.Scrolls(layout.Horizontal) || !s.n.Scrolls(layout.Vertical) {
			t.Error("vertical scroller only")
		}
		s.SetMinWidth(100).SetMinHeight(50)
		if w, _ := s.n.Props().Min(layout.Horizontal); w != 100 {
			t.Errorf("min width %v", w)
		}
		if hh, _ := s.n.Props().Min(layout.Vertical); hh != 50 {
			t.Errorf("min height %v", hh)
		}
		if h.n != 5 {
			t.Errorf("%d invalidations, want 5", h.n)
		}
		l := NewLabel("doc")
		s.SetDocument(l)
		s.SetDocument(nil)
		if p.Document != nil || l.parent != nil || s.n.Document() != nil {
			t.Error("SetDocument(nil)")
		}
	})
}

func TestTextView(t *testing.T) {
	runApp(t, func(*App) {
		v := NewTextView()
		p := v.peer.(*stub.TextView)
		if x, y := v.node().Expands(layout.Horizontal), v.node().Expands(layout.Vertical); !x || !y {
			t.Error("a text view doesn't expand on both axes")
		}
		h := &countingHost{}
		v.host = h
		var got *TextView = v.SetText("héllo").SetFont(SystemFont(14)).
			SetTextColor(ColorSecondaryLabel).SetEditable(false)
		if got != v || p.Value != "héllo" || v.Text() != "héllo" || p.Font != SystemFont(14) ||
			v.Font() != SystemFont(14) || p.TextColor != ColorSecondaryLabel || !p.ReadOnly {
			t.Fatalf("setters: %+v", p)
		}
		if h.n != 0 {
			t.Errorf("%d invalidations; a text view's size doesn't depend on its text", h.n)
		}
		if s, e := v.Selection(); s != 6 || e != 6 {
			t.Errorf("caret after SetText at %d…%d, want 6", s, e)
		}
		// Offsets are clamped and moved back to a character boundary.
		if s, e := v.SetSelection(9, 2).Selection(); s != 1 || e != 6 {
			t.Errorf("SetSelection(9, 2) = %d…%d, want 1…6", s, e)
		}
		v.SetEditable(true).SetSelection(6, 6)

		var log []string
		v.OnChange(func(s string) { log = append(log, "a:"+s) })
		un := v.OnChange(func(s string) { log = append(log, "b:"+s) })
		p.UserType("!")
		un()
		un()
		p.UserType("?")
		v.SetText("x") // setters don't fire
		if want := []string{"a:héllo!", "b:héllo!", "a:héllo!?"}; !slices.Equal(log, want) {
			t.Errorf("events %v, want %v", log, want)
		}
		if s, e := v.SetText("héllo").SetSelection(0, 1).Selection(); s != 0 || e != 1 {
			t.Errorf("selection %d…%d", s, e)
		}
		p.UserType("H")
		if v.Text() != "Héllo" || log[len(log)-1] != "a:Héllo" {
			t.Errorf("Text after typing = %q", v.Text())
		}
		if s, e := v.Selection(); s != 1 || e != 1 {
			t.Errorf("caret after typing %d…%d, want 1", s, e)
		}

		v.SetEnabled(false)
		p.Value = "Hi"                  // a native edit while disabled
		textViewEvents{v}.TextChanged() // reported anyway
		if v.Text() != "Hi" {           // B14: the cache follows, only the handlers are skipped
			t.Errorf("disabled view: Text %q, want the reported Hi", v.Text())
		}
		v.destroy()
		textViewEvents{v}.TextChanged()
		s0, e0 := v.Selection()
		mustPanic(t, "TextView.SetText on a destroyed widget", func() { v.SetText("after") })
		mustPanic(t, "TextView.SetSelection on a destroyed widget", func() { v.SetSelection(0, 0) })
		if len(log) != 4 || p.Value != "Hi" || v.Text() != "Hi" {
			t.Errorf("disabled or destroyed view: events %v, peer %q, Text %q", log, p.Value, v.Text())
		}
		if s, e := v.Selection(); s != s0 || e != e0 {
			t.Errorf("selection of a destroyed view %d…%d, want the last one %d…%d", s, e, s0, e0)
		}
	})
}
