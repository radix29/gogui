//go:build windows

package dwrite

// Constants and mirrored structs from dwrite.h. The names are the SDK's
// without the DWRITE_ prefix; the layouts are pinned by the tests.

// DWRITE_FACTORY_TYPE.
const (
	FACTORY_TYPE_SHARED   = 0
	FACTORY_TYPE_ISOLATED = 1
)

// FontWeight is DWRITE_FONT_WEIGHT (1–999; the named values below).
type FontWeight uint32

// Font weights.
const (
	FONT_WEIGHT_THIN        FontWeight = 100
	FONT_WEIGHT_EXTRA_LIGHT FontWeight = 200
	FONT_WEIGHT_LIGHT       FontWeight = 300
	FONT_WEIGHT_SEMI_LIGHT  FontWeight = 350
	FONT_WEIGHT_NORMAL      FontWeight = 400
	FONT_WEIGHT_MEDIUM      FontWeight = 500
	FONT_WEIGHT_SEMI_BOLD   FontWeight = 600
	FONT_WEIGHT_BOLD        FontWeight = 700
	FONT_WEIGHT_EXTRA_BOLD  FontWeight = 800
	FONT_WEIGHT_BLACK       FontWeight = 900
	FONT_WEIGHT_EXTRA_BLACK FontWeight = 950
)

// FontStyle is DWRITE_FONT_STYLE.
type FontStyle uint32

// Font styles.
const (
	FONT_STYLE_NORMAL  FontStyle = 0
	FONT_STYLE_OBLIQUE FontStyle = 1
	FONT_STYLE_ITALIC  FontStyle = 2
)

// FontStretch is DWRITE_FONT_STRETCH.
type FontStretch uint32

// Font stretches.
const (
	FONT_STRETCH_UNDEFINED       FontStretch = 0
	FONT_STRETCH_ULTRA_CONDENSED FontStretch = 1
	FONT_STRETCH_EXTRA_CONDENSED FontStretch = 2
	FONT_STRETCH_CONDENSED       FontStretch = 3
	FONT_STRETCH_SEMI_CONDENSED  FontStretch = 4
	FONT_STRETCH_NORMAL          FontStretch = 5
	FONT_STRETCH_SEMI_EXPANDED   FontStretch = 6
	FONT_STRETCH_EXPANDED        FontStretch = 7
	FONT_STRETCH_EXTRA_EXPANDED  FontStretch = 8
	FONT_STRETCH_ULTRA_EXPANDED  FontStretch = 9
)

// WordWrapping is DWRITE_WORD_WRAPPING. EMERGENCY_BREAK, WHOLE_WORD and
// CHARACTER need IDWriteTextFormat1-era DirectWrite (Windows 8.1+), which
// Factory5 implies.
type WordWrapping uint32

// Word wrapping modes.
const (
	WORD_WRAPPING_WRAP            WordWrapping = 0
	WORD_WRAPPING_NO_WRAP         WordWrapping = 1
	WORD_WRAPPING_EMERGENCY_BREAK WordWrapping = 2
	WORD_WRAPPING_WHOLE_WORD      WordWrapping = 3
	WORD_WRAPPING_CHARACTER       WordWrapping = 4
)

// TextAlignment is DWRITE_TEXT_ALIGNMENT (horizontal, in reading direction).
type TextAlignment uint32

// Text alignments.
const (
	TEXT_ALIGNMENT_LEADING   TextAlignment = 0
	TEXT_ALIGNMENT_TRAILING  TextAlignment = 1
	TEXT_ALIGNMENT_CENTER    TextAlignment = 2
	TEXT_ALIGNMENT_JUSTIFIED TextAlignment = 3
)

// ParagraphAlignment is DWRITE_PARAGRAPH_ALIGNMENT (vertical, in flow
// direction).
type ParagraphAlignment uint32

// Paragraph alignments.
const (
	PARAGRAPH_ALIGNMENT_NEAR   ParagraphAlignment = 0
	PARAGRAPH_ALIGNMENT_FAR    ParagraphAlignment = 1
	PARAGRAPH_ALIGNMENT_CENTER ParagraphAlignment = 2
)

// TextMetrics is DWRITE_TEXT_METRICS, in DIPs. Left and Top are the
// formatted text's offset in the layout box (non-zero with centered or
// trailing alignment). Width leaves out trailing whitespace;
// WidthIncludingTrailingWhitespace keeps it.
type TextMetrics struct {
	Left                             float32
	Top                              float32
	Width                            float32
	WidthIncludingTrailingWhitespace float32
	Height                           float32
	LayoutWidth                      float32
	LayoutHeight                     float32
	MaxBidiReorderingDepth           uint32
	LineCount                        uint32
}

// LineMetrics is DWRITE_LINE_METRICS. Length counts UTF-16 units including
// the trailing whitespace and the newline; Baseline is the distance from
// the line's top. IsTrimmed is a BOOL.
type LineMetrics struct {
	Length                   uint32
	TrailingWhitespaceLength uint32
	NewlineLength            uint32
	Height                   float32
	Baseline                 float32
	IsTrimmed                int32
}

// HitTestMetrics is DWRITE_HIT_TEST_METRICS, in DIPs: the box of a text
// range (TextPosition, Length in UTF-16 units). BidiLevel is odd for
// right-to-left text; IsText is false for the box at a line's end that
// holds no text; IsText and IsTrimmed are BOOLs.
type HitTestMetrics struct {
	TextPosition uint32
	Length       uint32
	Left         float32
	Top          float32
	Width        float32
	Height       float32
	BidiLevel    uint32
	IsText       int32
	IsTrimmed    int32
}

// ClusterMetrics is DWRITE_CLUSTER_METRICS: a cluster's advance Width in
// DIPs, its Length in UTF-16 units, and five one-bit flags packed in Flags
// (the C bit fields, LSB first, as MSVC allocates them in a UINT16); read
// them through the methods.
type ClusterMetrics struct {
	Width  float32
	Length uint16
	Flags  uint16
}

// The bits of ClusterMetrics.Flags in declaration order; the other 11 bits
// are padding.
const (
	clusterCanWrapLineAfter = 1 << iota
	clusterIsWhitespace
	clusterIsNewline
	clusterIsSoftHyphen
	clusterIsRightToLeft
)

// IsRightToLeft is whether the cluster is laid out right to left.
func (c ClusterMetrics) IsRightToLeft() bool { return c.Flags&clusterIsRightToLeft != 0 }
