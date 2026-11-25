//go:build windows

// Package dwrite holds the DirectWrite bindings of the Windows shell
// (PLAN.md S2.5). S2.5a binds the factory, text formats and layout metrics:
//
//   - CreateFactory: DWriteCreateFactory (shared) for IDWriteFactory, then
//     QueryInterface to IDWriteFactory5 (Windows 10 1703, inside D7); a
//     missing IDWriteFactory5 is an error naming that version. The Factory
//     holds the IDWriteFactory5 pointer, which serves every IDWriteFactory
//     method too.
//   - Factory: SystemFontCollection, CreateTextFormat (float path: the
//     size), CreateTextLayout (float path: the max width and height).
//   - FontCollection: FamilyCount.
//   - TextFormat: SetWordWrapping, SetTextAlignment, SetParagraphAlignment
//     and their getters, FontSize (a FLOAT result), FontWeight, FontStyle,
//     FontStretch, FontFamilyName.
//   - TextLayout: the TextFormat methods (it derives from it), MaxWidth,
//     MaxHeight (FLOAT results), Metrics (DWRITE_TEXT_METRICS by pointer)
//     and LineMetrics (an array of DWRITE_LINE_METRICS with the "buffer too
//     small" retry).
//
// S2.5b adds hit-testing and clusters on TextLayout: SetMaxWidth (float
// path), ClusterMetrics (DWRITE_CLUSTER_METRICS, its bit fields read by
// mask), HitTestPoint (float path), HitTestTextPosition (integer path; the
// FLOATs come back through pointers) and HitTestTextRange (float path, an
// array with the same retry), with DWRITE_HIT_TEST_METRICS.
//
// S2.5c adds fonts from memory (fonts.go) without a font file loader
// implemented in Go: IDWriteFactory5's CreateInMemoryFontFileLoader,
// Register/UnregisterFontFileLoader, the loader's
// CreateInMemoryFontFileReference (no owner object, so DirectWrite copies
// the bytes and no Go memory is held natively), IDWriteFontFile::Analyze,
// CreateFontSetBuilder (IDWriteFontSetBuilder1) with AddFontFile and
// CreateFontSet, CreateFontCollectionFromFontSet, and
// FontCollection.FindFamilyName. CreateMemoryFonts builds one collection
// from a list of files and MemoryFonts.Release gives it all back; the loader
// must stay registered while anything made from the collection is alive.
//
// The rules are d2d's: every interface embeds com.Ptr, returned interfaces
// carry one reference, the slot constants follow dwrite.h's declaration
// order and are checked by the tests against the full vtables. Text is
// passed as UTF-16 ([]uint16); the conversion from UTF-8 and the index map
// are the shell's (S2.5d). Everything is used from the UI thread, although
// the shared factory and its objects are thread-safe by themselves.
//
// The structs are passed by pointer, so the integer path (com.Ptr.Call)
// carries them; only FLOAT arguments and results take the float path. On
// windows/amd64 (com.DirectFloats, review P2) that is the integer path too:
// a FLOAT argument goes as its bits in the integer slot of its position
// (com.Float; the runtime mirrors the first four into XMM0–3), and a FLOAT
// result comes back with com.Ptr.CallFloat. On windows/arm64 the float-path
// methods go through com.Bind (purego) and keep their bound func in a
// com.Method; directFloats selects the path, and the tests run both. No
// method returning a struct is bound.
package dwrite
