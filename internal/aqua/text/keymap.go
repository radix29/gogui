package text

import (
	"strconv"

	"github.com/radix29/gogui/internal/driver"
)

// Command is an editing command a key is bound to (Lookup) and the Editor
// runs (Do). The names follow AppKit's NSResponder actions.
type Command uint8

// The move commands come in pairs, the plain move and its Shift variant
// (Select…, the selection extended); see moveSpecs.
const (
	CmdNone Command = iota

	CmdMoveLeft              // ←: one cluster back, or collapse a selection to its start
	CmdSelectLeft            // Shift-←
	CmdMoveRight             // →
	CmdSelectRight           // Shift-→
	CmdMoveWordLeft          // Option-←: to the start of the word before
	CmdSelectWordLeft        // Shift-Option-←
	CmdMoveWordRight         // Option-→: to the end of the word after
	CmdSelectWordRight       // Shift-Option-→
	CmdMoveToLineStart       // Primary-←: to the start of the visual line
	CmdSelectToLineStart     // Shift-Primary-←
	CmdMoveToLineEnd         // Primary-→: to the end of the visual line
	CmdSelectToLineEnd       // Shift-Primary-→
	CmdMoveUp                // ↑: one visual line up at the goal x
	CmdSelectUp              // Shift-↑
	CmdMoveDown              // ↓
	CmdSelectDown            // Shift-↓
	CmdMoveParagraphBack     // Option-↑: to the paragraph start, or the previous one's
	CmdSelectParagraphBack   // Shift-Option-↑
	CmdMoveParagraphFwd      // Option-↓: to the paragraph end, or the next one's
	CmdSelectParagraphFwd    // Shift-Option-↓
	CmdMoveToDocumentStart   // Primary-↑, Home
	CmdSelectToDocumentStart // Shift-Primary-↑, Shift-Home
	CmdMoveToDocumentEnd     // Primary-↓, End
	CmdSelectToDocumentEnd   // Shift-Primary-↓, Shift-End
	CmdPageUp                // Page Up, Option-Page Up: Lines.PageLines lines up
	CmdSelectPageUp          // Shift-Page Up
	CmdPageDown              // Page Down, Option-Page Down
	CmdSelectPageDown        // Shift-Page Down

	CmdDeleteBackward     // ⌫ (and Shift-⌫)
	CmdDeleteForward      // ⌦ (and Shift-⌦)
	CmdDeleteWordBackward // Option-⌫
	CmdDeleteWordForward  // Option-⌦
	CmdDeleteToLineStart  // Primary-⌫
	CmdDeleteToLineEnd    // Primary-⌦
	CmdInsertNewline      // Return, Shift-Return (multi-line only)
	CmdInsertTab          // Tab (multi-line only)
	CmdSelectAll          // Primary-A
	CmdCopy               // Primary-C (the caller's: Do reports it back)
	CmdCut                // Primary-X (the caller's)
	CmdPaste              // Primary-V (the caller's)
	CmdUndo               // Primary-Z
	CmdRedo               // Shift-Primary-Z

	numCommands
)

// moveSpecs holds the unit and direction of each pair of move commands, in
// the order of the constants.
var moveSpecs = [...]struct {
	unit Unit
	dir  Direction
}{
	{Grapheme, Backward}, {Grapheme, Forward},
	{Word, Backward}, {Word, Forward},
	{Line, Backward}, {Line, Forward},
	{Vertical, Backward}, {Vertical, Forward},
	{Paragraph, Backward}, {Paragraph, Forward},
	{Document, Backward}, {Document, Forward},
	{Page, Backward}, {Page, Forward},
}

// move reports whether c is a move command and which: its unit, direction
// and whether it extends the selection (Shift).
func (c Command) move() (u Unit, d Direction, extend, ok bool) {
	i := int(c) - int(CmdMoveLeft)
	if i < 0 || i >= 2*len(moveSpecs) {
		return 0, 0, false, false
	}
	m := moveSpecs[i/2]
	return m.unit, m.dir, i%2 == 1, true
}

// IsClipboard reports whether c is Copy, Cut or Paste, which Do leaves to the
// caller: it owns the clipboard and calls Editor.Copy, Cut or Paste.
func (c Command) IsClipboard() bool { return c == CmdCopy || c == CmdCut || c == CmdPaste }

// Edits reports whether c can change the text (the deletes, InsertNewline,
// InsertTab, Undo, Redo, and Cut and Paste): what a read-only editor refuses.
// The moves, SelectAll and Copy only move or read the selection.
func (c Command) Edits() bool {
	return c >= CmdDeleteBackward && c <= CmdInsertTab || c == CmdCut || c == CmdPaste ||
		c == CmdUndo || c == CmdRedo
}

var commandNames = [numCommands]string{
	"None",
	"MoveLeft", "SelectLeft", "MoveRight", "SelectRight",
	"MoveWordLeft", "SelectWordLeft", "MoveWordRight", "SelectWordRight",
	"MoveToLineStart", "SelectToLineStart", "MoveToLineEnd", "SelectToLineEnd",
	"MoveUp", "SelectUp", "MoveDown", "SelectDown",
	"MoveParagraphBack", "SelectParagraphBack", "MoveParagraphFwd", "SelectParagraphFwd",
	"MoveToDocumentStart", "SelectToDocumentStart", "MoveToDocumentEnd", "SelectToDocumentEnd",
	"PageUp", "SelectPageUp", "PageDown", "SelectPageDown",
	"DeleteBackward", "DeleteForward", "DeleteWordBackward", "DeleteWordForward",
	"DeleteToLineStart", "DeleteToLineEnd",
	"InsertNewline", "InsertTab",
	"SelectAll", "Copy", "Cut", "Paste", "Undo", "Redo",
}

func (c Command) String() string {
	if c < numCommands {
		return commandNames[c]
	}
	return "Command(" + strconv.Itoa(int(c)) + ")"
}

type chord struct {
	key  driver.Key
	mods driver.Modifiers
}

const (
	shift   = driver.ModShift
	option  = driver.ModOption
	primary = driver.ModControl // Primary resolved: the aqua driver runs on Windows and Linux only
)

// keymap holds the bindings of both modes; multilineKeymap the ones only a
// multi-line editor has.
var keymap, multilineKeymap = buildKeymaps()

func buildKeymaps() (both, multi map[chord]Command) {
	both = make(map[chord]Command)
	// The moves: each binding with and without Shift.
	for _, b := range []struct {
		key  driver.Key
		mods driver.Modifiers
		cmd  Command
	}{
		{driver.KeyLeft, 0, CmdMoveLeft},
		{driver.KeyRight, 0, CmdMoveRight},
		{driver.KeyLeft, option, CmdMoveWordLeft},
		{driver.KeyRight, option, CmdMoveWordRight},
		{driver.KeyLeft, primary, CmdMoveToLineStart},
		{driver.KeyRight, primary, CmdMoveToLineEnd},
		{driver.KeyUp, 0, CmdMoveUp},
		{driver.KeyDown, 0, CmdMoveDown},
		{driver.KeyUp, option, CmdMoveParagraphBack},
		{driver.KeyDown, option, CmdMoveParagraphFwd},
		{driver.KeyUp, primary, CmdMoveToDocumentStart},
		{driver.KeyDown, primary, CmdMoveToDocumentEnd},
		{driver.KeyHome, 0, CmdMoveToDocumentStart},
		{driver.KeyEnd, 0, CmdMoveToDocumentEnd},
		{driver.KeyPageUp, 0, CmdPageUp},
		{driver.KeyPageDown, 0, CmdPageDown},
		{driver.KeyPageUp, option, CmdPageUp},
		{driver.KeyPageDown, option, CmdPageDown},
	} {
		both[chord{b.key, b.mods}] = b.cmd
		both[chord{b.key, b.mods | shift}] = b.cmd + 1
	}
	// The deletes ignore Shift, as on macOS.
	for _, b := range []struct {
		key  driver.Key
		mods driver.Modifiers
		cmd  Command
	}{
		{driver.KeyBackspace, 0, CmdDeleteBackward},
		{driver.KeyDelete, 0, CmdDeleteForward},
		{driver.KeyBackspace, option, CmdDeleteWordBackward},
		{driver.KeyDelete, option, CmdDeleteWordForward},
		{driver.KeyBackspace, primary, CmdDeleteToLineStart},
		{driver.KeyDelete, primary, CmdDeleteToLineEnd},
	} {
		both[chord{b.key, b.mods}] = b.cmd
		both[chord{b.key, b.mods | shift}] = b.cmd
	}
	both[chord{'A', primary}] = CmdSelectAll
	both[chord{'C', primary}] = CmdCopy
	both[chord{'X', primary}] = CmdCut
	both[chord{'V', primary}] = CmdPaste
	both[chord{'Z', primary}] = CmdUndo
	both[chord{'Z', primary | shift}] = CmdRedo

	multi = map[chord]Command{
		{driver.KeyReturn, 0}:     CmdInsertNewline,
		{driver.KeyReturn, shift}: CmdInsertNewline,
		{driver.KeyTab, 0}:        CmdInsertTab,
	}
	return both, multi
}

// Lookup returns the command key with mods is bound to in a text editor
// (PLAN.md §5.5, §5.6), and false when it is bound to none: the key then goes
// on (to a default or cancel button, the focus, a menu shortcut, an enclosing
// scroll view).
//
// mods are physical, as shell.KeyEvent delivers them; Primary is Ctrl
// (ModControl), as the aqua driver runs only on Windows and Linux. A
// ModPrimary that slips in is resolved to Ctrl. The bindings are macOS's on
// every OS: Option (Alt) moves by word or paragraph, Primary to the line or
// document ends, Shift extends; no Emacs Control bindings (they would collide
// with Primary). A chord holding ModCommand (the Windows or Super key) is
// never bound, nor is Ctrl+Alt (AltGr on Windows) with anything.
//
// Home, End, Page Up and Page Down move the caret (to the document ends, by a
// page), as their Shift and Option variants do on macOS; plain ones there
// only scroll. Return and Tab are bound only when multiline: in a TextField
// they go on to the default button and the focus. Shift-Tab is never bound.
// Letter keys are matched case-insensitively.
func Lookup(key driver.Key, mods driver.Modifiers, multiline bool) (Command, bool) {
	if key >= 'a' && key <= 'z' {
		key -= 'a' - 'A'
	}
	c := chord{key, mods.Resolve(false)}
	if cmd, ok := keymap[c]; ok {
		return cmd, true
	}
	if multiline {
		if cmd, ok := multilineKeymap[c]; ok {
			return cmd, true
		}
	}
	return CmdNone, false
}

// Do runs cmd on the editor, lines as for Move (nil: each paragraph is one
// line). It reports whether it handled cmd: false for CmdNone, for the
// clipboard commands (IsClipboard), which the caller does over Copy, Cut and
// Paste, and for CmdInsertNewline in single-line mode. Undo and Redo with
// nothing to do are handled (they change nothing).
func (e *Editor) Do(cmd Command, lines Lines) bool {
	if u, d, extend, ok := cmd.move(); ok {
		e.Move(u, d, extend, lines)
		return true
	}
	switch cmd {
	case CmdDeleteBackward:
		e.DeleteBackward(Grapheme, lines)
	case CmdDeleteForward:
		e.DeleteForward(Grapheme, lines)
	case CmdDeleteWordBackward:
		e.DeleteBackward(Word, lines)
	case CmdDeleteWordForward:
		e.DeleteForward(Word, lines)
	case CmdDeleteToLineStart:
		e.DeleteBackward(Line, lines)
	case CmdDeleteToLineEnd:
		e.DeleteForward(Line, lines)
	case CmdInsertNewline:
		if e.single {
			return false
		}
		e.Insert("\n")
	case CmdInsertTab:
		e.Insert("\t")
	case CmdSelectAll:
		e.SelectAll()
	case CmdUndo:
		e.Undo()
	case CmdRedo:
		e.Redo()
	default:
		return false
	}
	return true
}
