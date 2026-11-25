// Package text is the editing core shared by the aqua TextField and TextView
// (PLAN.md §5.5, §5.6, S5.1). It has no widget, layout or clipboard code and
// no build tags; everything runs on the UI thread of its owner and nothing is
// safe for concurrent use.
//
// Offsets are UTF-8 byte offsets into the text, as in driver.TextView's
// selection and render.TextLayout.
//
// Since S5.1a the package holds Buffer, a piece table over an immutable
// original buffer and an append-only add buffer, with paragraph lookup in
// logarithmic time and a revision counter for layout caches. Since S5.1b it
// has the grapheme and word boundaries over one paragraph (NextGrapheme,
// PrevGrapheme, ClusterStart, NextWordEnd, PrevWordStart, WordAt) over
// rivo/uniseg. Since S5.1c the Editor holds the selection (anchor, head and
// the upstream affinity at soft line breaks), the moves by grapheme, word,
// line, paragraph, document, vertical line and page (the geometry through
// the Lines interface), the edits and the clipboard text. Since S5.1d it
// keeps an undo history: typing and deletes in one direction at the caret
// coalesce into groups, Undo restores the selection, Redo the caret. Since
// S5.1e the keybinding map: Lookup turns a key and its physical modifiers
// into a Command (macOS bindings on every OS, Primary as Ctrl) and Editor.Do
// runs it, leaving the clipboard commands to the caller. S5.1f completed the
// core; ARCHITECTURE.md §9.5 lists its contracts.
package text
