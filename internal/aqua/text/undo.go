package text

// DefaultUndoLimit is the number of undo groups an Editor keeps until
// SetUndoLimit changes it; the oldest group goes first.
const DefaultUndoLimit = 1000

// mergeMax caps the text of an edit that consecutive typing or deletes grow
// in place; past it a group gets a new edit, so typing stays O(1) a key.
const mergeMax = 256

// editKind says how an edit coalesces with the open undo group.
type editKind uint8

const (
	kindOther          editKind = iota // a group of its own: paste, cut, a typed newline
	kindTyping                         // Insert
	kindDeleteBackward                 // ⌫ and its Option and Primary variants
	kindDeleteForward                  // ⌦ and its variants
)

// edit is one replace: [start, start+len(old)) held old and holds new.
type edit struct {
	start    int
	old, new string
}

// group is what one Undo reverts: its edits in order, the selection before
// the first and the caret after the last.
type group struct {
	edits        []edit
	anchor, head int
	after        int
}

// history is the undo and redo stacks of an Editor. The top undo group is
// open while open is not kindOther: an edit of that kind at its caret joins
// it. Any move or selection change seals it (seal).
type history struct {
	undo, redo []group
	open       editKind
	limit      int // the most groups kept; 0 is no limit
}

func (h *history) clear() {
	clear(h.undo)
	clear(h.redo)
	h.undo, h.redo, h.open = h.undo[:0], h.redo[:0], kindOther
}

func (h *history) seal() { h.open = kindOther }

// begin is called before an edit of kind k with the selection (anchor,
// head): it keeps the open group when the edit coalesces with it (the same
// kind, a caret where the group left it), else it starts a new one. Either
// way the redo stack is gone.
func (h *history) begin(k editKind, anchor, head int) {
	clear(h.redo)
	h.redo = h.redo[:0]
	if k != kindOther && h.open == k && anchor == head && head == h.undo[len(h.undo)-1].after {
		return
	}
	h.undo = append(h.undo, group{anchor: anchor, head: head})
	h.trim()
	h.open = k
}

// record adds ed to the top group, which then leaves the caret at after.
// Typing at the end of the last edit, or deleting right before it (⌫) or
// at it (⌦), grows that edit.
func (h *history) record(ed edit, after int) {
	g := &h.undo[len(h.undo)-1]
	g.after = after
	if n := len(g.edits); n > 0 {
		l := &g.edits[n-1]
		switch {
		case ed.old == "" && l.start+len(l.new) == ed.start && len(l.new)+len(ed.new) <= mergeMax:
			l.new += ed.new
			return
		case ed.new == "" && l.new == "" && ed.start+len(ed.old) == l.start && len(l.old)+len(ed.old) <= mergeMax:
			l.start, l.old = ed.start, ed.old+l.old
			return
		case ed.new == "" && l.new == "" && ed.start == l.start && len(l.old)+len(ed.old) <= mergeMax:
			l.old += ed.old
			return
		}
	}
	g.edits = append(g.edits, ed)
}

// trim drops the oldest groups past the limit. It reslices rather than
// copies, so dropping one group a keystroke stays O(1): the next growth of
// the slice leaves the dropped front behind.
func (h *history) trim() {
	if h.limit <= 0 {
		return
	}
	for _, s := range []*[]group{&h.undo, &h.redo} {
		if d := len(*s) - h.limit; d > 0 {
			clear((*s)[:d])
			*s = (*s)[d:]
		}
	}
}

// Undo reverts the last undo group and restores the selection from before
// it; false when there is nothing to undo.
func (e *Editor) Undo() bool {
	h := &e.hist
	h.seal()
	if len(h.undo) == 0 {
		return false
	}
	g := h.undo[len(h.undo)-1]
	h.undo[len(h.undo)-1] = group{}
	h.undo = h.undo[:len(h.undo)-1]
	for i := len(g.edits) - 1; i >= 0; i-- {
		ed := g.edits[i]
		e.buf.Replace(ed.start, ed.start+len(ed.new), ed.old)
	}
	e.setRange(g.anchor, g.head)
	h.redo = append(h.redo, g)
	return true
}

// Redo applies the last undone group again, the caret where it left it;
// false when there is nothing to redo. Any new edit clears the redo stack.
func (e *Editor) Redo() bool {
	h := &e.hist
	h.seal()
	if len(h.redo) == 0 {
		return false
	}
	g := h.redo[len(h.redo)-1]
	h.redo[len(h.redo)-1] = group{}
	h.redo = h.redo[:len(h.redo)-1]
	for _, ed := range g.edits {
		e.buf.Replace(ed.start, ed.start+len(ed.old), ed.new)
	}
	e.setCaret(g.after, false)
	h.undo = append(h.undo, g)
	return true
}

// BreakUndoGroup seals the open undo group without changing the text or
// the selection: the next edit starts a group of its own even where it
// would coalesce. A text field calls it around an IME commit, so the commit
// undoes on its own, apart from the typing before and after it (S5.3).
func (e *Editor) BreakUndoGroup() { e.hist.seal() }

// CanUndo reports whether Undo would do something (for the Edit menu).
func (e *Editor) CanUndo() bool { return len(e.hist.undo) > 0 }

// CanRedo reports whether Redo would do something.
func (e *Editor) CanRedo() bool { return len(e.hist.redo) > 0 }

// SetUndoLimit sets the most undo groups kept (DefaultUndoLimit at first;
// 0 keeps all), dropping the oldest ones past it.
func (e *Editor) SetUndoLimit(n int) {
	e.hist.limit = max(n, 0)
	e.hist.trim()
}
