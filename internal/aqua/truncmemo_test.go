package aqua

import (
	"testing"

	"github.com/radix29/gogui/internal/driver"
	"github.com/radix29/gogui/internal/render"
	"github.com/radix29/gogui/internal/shell/fake"
)

// countingText counts the layouts made by a text system.
type countingText struct {
	render.TextSystem
	n int
}

func (c *countingText) NewLayout(text string, font render.Font, maxWidth float64) render.TextLayout {
	c.n++
	return c.TextSystem.NewLayout(text, font, maxWidth)
}

// TestTruncMemo: the memo truncates once per text system, text, font and
// width (S6.2e) and gives the same result as truncate.
func TestTruncMemo(t *testing.T) {
	ts := &countingText{TextSystem: fake.NewTextSystem()}
	other := &countingText{TextSystem: fake.NewTextSystem()}
	var m truncMemo
	const text = "A rather long title"
	font := driver.Font{Size: 13}
	l := m.truncate(ts, text, font, 40)
	want := truncate(fake.NewTextSystem(), text, font, 40)
	if l == nil || want == nil || l.Text() != want.Text() {
		t.Fatalf("memo truncation %v, truncate %v", l, want)
	}
	made := ts.n
	if made == 0 {
		t.Fatal("no layout made")
	}
	for range 3 {
		if got := m.truncate(ts, text, font, 40); got != l {
			t.Error("a repeated truncation is not the memo's")
		}
	}
	if ts.n != made || m.made != 1 {
		t.Errorf("repeated: %d layouts made (want %d), %d truncations (want 1)", ts.n, made, m.made)
	}
	for i, c := range []struct {
		ts    render.TextSystem
		text  string
		font  driver.Font
		width float64
	}{
		{ts, text, font, 60},
		{ts, text + "!", font, 60},
		{ts, text + "!", driver.Font{Size: 11}, 60},
		{other, text + "!", driver.Font{Size: 11}, 60},
	} {
		m.truncate(c.ts, c.text, c.font, c.width)
		if m.made != i+2 {
			t.Errorf("change %d: %d truncations, want %d", i, m.made, i+2)
		}
	}
	// nil (not even the ellipsis fits) is remembered too.
	var n truncMemo
	first := n.truncate(ts, text, font, 1)
	second := n.truncate(ts, text, font, 1)
	if first != nil || second != nil || n.made != 1 {
		t.Errorf("too narrow: %v and %v, %d truncations; want nil, nil, 1", first, second, n.made)
	}
}

// TestBoxTitlePaintOnce: a Box's title is laid out once, not on every
// paint: it fits, or its truncation is the memo's (S6.2e).
func TestBoxTitlePaintOnce(t *testing.T) {
	d := startDriver(t)
	box := d.NewBox().(*Box)
	box.SetTitle("A group with a long title")
	box.SetFrame(driver.Rect{X: 10, Y: 10, W: 60, H: 60})
	e := ElementOf(box).(*group)
	for range 3 {
		dump(t, e, driver.Size{W: 400, H: 300})
	}
	if e.trunc.made != 1 {
		t.Errorf("narrow: %d truncations in 3 paints, want 1", e.trunc.made)
	}
	box.SetFrame(driver.Rect{X: 10, Y: 10, W: 300, H: 60})
	for range 3 {
		dump(t, e, driver.Size{W: 400, H: 300})
	}
	if e.trunc.made != 1 {
		t.Errorf("wide: %d truncations, want none more", e.trunc.made)
	}
}
