// Package theme holds the design tokens of the drawn aqua driver (PLAN.md
// §5.2): the values that make a drawn control look like its AppKit
// counterpart. The Theme type is hand-written; tokens.go is generated from a
// JSON spec (spec/sequoia.json) by the gen package and fills one value of
// it, Sequoia (macOS 14/15). A second token set (D2) is a second spec and a
// second generated value, with no change to the code that reads a Theme.
//
// Besides the semantic colors (resolved by Theme.Resolve), a theme holds the
// colors and metrics of the drawn window chrome (Chrome), the control
// metrics per control size and the focus ring (Metrics), the typography
// (Typography), the parameters of the accent rules (Accent; the colors that
// follow Appearance.Accent are computed, not stored, accent.go), the
// animation durations, curves and springs (Animation), the tokens of the
// drawn controls (Controls), those of the scroll view (Scroll), those of
// the text fields (TextField), those of the text view (TextView) and those
// of the menus (Menu; their highlight follows the accent, menu.go) and
// those of the pop-up button (PopUp; its indicator follows the accent,
// popup.go), those of the title-bar menu strip (MenuBar) and those of the
// tooltips (Tooltip) and those of the alerts (Alert).
// Lengths are in logical points. Colors with an increased-contrast variant
// use it when Appearance.IncreaseContrast is set; the outline tokens
// (Controls.BezelOutline, Controls.SliderTrackOutline) are transparent
// otherwise, as Increase Contrast adds outlines the normal look does not
// have (S6.1d).
//
// The values are provisional while phase 1 waits for a Mac: the spec marks
// each one as "memory" (from memory, to be measured) or "measured", and
// docs/tokens.md lists them for S3.1t.
//
// Run "go generate ./internal/aqua/theme" after editing a spec; the
// generated files are committed, so a build never needs go generate.
//
// Besides the standard library, the package imports only internal/driver.
package theme

//go:generate go run ./gen/gentokens -spec spec/sequoia.json -out tokens.go -doc ../../../docs/tokens.md

import (
	"math"
	"time"

	"github.com/radix29/gogui/internal/driver"
)

// Theme is one complete token set.
type Theme struct {
	// Name is the name of the token set ("Sequoia").
	Name string

	// Colors holds the values of the semantic colors.
	Colors Colors

	// Chrome holds the tokens of the drawn title bar and traffic lights.
	Chrome Chrome

	// Metrics holds the control metrics per control size and the focus
	// ring.
	Metrics Metrics

	// Typography holds the font sizes, weights, line height and tracking.
	Typography Typography

	// Accent holds the parameters of the rules that derive colors from
	// Appearance.Accent (accent.go).
	Accent Accent

	// Animation holds the durations, timing curves and springs.
	Animation Animation

	// Controls holds the tokens of the controls: push button, checkbox,
	// radio, Box and separator (S3.3b), slider, progress bar and spinner
	// (S3.4a; controls.go).
	Controls Controls

	// Scroll holds the tokens of the scroll view: its scrollers and its
	// physics (S3.5a).
	Scroll Scroll

	// TextField holds the tokens of the text field's bezel and caret
	// (S5.2e; textfield.go).
	TextField TextField

	// TextView holds the geometry of the text view's text container
	// (S5.4f).
	TextView TextView

	// Menu holds the tokens of the drawn menus (S5.5h; menu.go).
	Menu Menu

	// PopUp holds the tokens of the drawn pop-up button (S5.5j; popup.go).
	PopUp PopUp

	// MenuBar holds the tokens of the title-bar menu strip (S5.6c).
	MenuBar MenuBar

	// Tooltip holds the tokens of the drawn tooltips (S5.5l).
	Tooltip Tooltip

	// Alert holds the tokens of the drawn alerts (S5.7a).
	Alert Alert
}

// Alert holds the tokens of the drawn alerts (PLAN.md S5.7a), modelled on
// the macOS 11+ NSAlert: a fixed-width rounded panel with an icon centred
// at the top, the message in the bold system font (Typography
// EmphasizedWeight) and the detail in a smaller one, both centred and
// wrapped below it, and the buttons under them (regular push buttons). The
// icon is procedural (no Apple assets, §3.1): a neutral app placeholder,
// or for a critical alert a caution triangle with the placeholder as a
// badge. Lengths are in points.
type Alert struct {
	// Background fills the panel (opaque), Border is its hairline, drawn
	// inside it, and Shadow the color of the shadow the panel draws itself
	// where nothing draws one behind it (a sheet; offset by ShadowOffsetY
	// with ShadowBlur, render.Shadow semantics). The texts are in
	// ColorLabel, as NSAlert's.
	Background, Border, Shadow ColorToken

	// IconTop and IconBottom are the ends of the app placeholder's
	// vertical gradient, IconBorder its hairline and IconGlyph the window
	// glyph drawn on it. CautionFill fills the caution triangle and
	// CautionMark is its exclamation mark.
	IconTop, IconBottom, IconBorder, IconGlyph, CautionFill, CautionMark ColorToken

	// Width is the panel's width; CornerRadius its corner radius and
	// BorderWidth its border's (whole device pixels, at least one).
	Width, CornerRadius, BorderWidth float64

	// ShadowOffsetY and ShadowBlur shape the panel's own shadow.
	ShadowOffsetY, ShadowBlur float64

	// PaddingX is the space between the panel's sides and the texts and
	// buttons, PaddingTop the space above the icon and PaddingBottom the
	// space below the last button.
	PaddingX, PaddingTop, PaddingBottom float64

	// IconSize is the icon's side; IconCornerRadius the placeholder's
	// corner radius at that size; BadgeSize the side of the placeholder
	// badge over a critical alert's caution triangle; IconGap the space
	// between the icon and the message.
	IconSize, IconCornerRadius, BadgeSize, IconGap float64

	// MessageFontSize and DetailFontSize are the sizes of the two texts'
	// system fonts; DetailGap is the space between the message and the
	// detail (none without a detail).
	MessageFontSize, DetailFontSize, DetailGap float64

	// ButtonsGap is the space between the texts and the first row of
	// buttons, ButtonGap the space between two buttons side by side or
	// stacked.
	ButtonsGap, ButtonGap float64
}

// MenuBar holds the tokens of the menu strip the aqua driver draws in the
// title bar on Windows and Linux (PLAN.md §5.4.1, S5.6c), right of the
// traffic lights. The titles are in the regular system font (Typography),
// the app menu's in EmphasizedWeight. Lengths are in points.
type MenuBar struct {
	// Text colors the titles of a key window, InactiveText those of an
	// inactive one; Highlight is the rounded fill behind the open or
	// keyboard-focused title (accent-free, as macOS's menu bar).
	Text, InactiveText, Highlight ColorToken

	// StartGap is the space between the traffic lights and the first
	// title's slot. Each slot is its title's width plus TitlePaddingX on
	// both sides; the highlight covers the slot, HighlightHeight high and
	// centred in the bar, with HighlightRadius corners.
	StartGap, TitlePaddingX, HighlightHeight, HighlightRadius float64

	// MinDragWidth is the least caption width kept right of the last shown
	// title (the titles that do not fit are dropped from the right), and
	// WindowTitleGap the least space between the strip and the window
	// title.
	MinDragWidth, WindowTitleGap float64
}

// Tooltip holds the tokens of the drawn tooltips (PLAN.md S5.5l): a small
// rounded panel with the text of Widget.SetTooltip, in a non-grab popup
// below the pointer. Lengths are in points.
type Tooltip struct {
	// Background fills the panel (opaque: a tooltip is never blurred
	// behind), Border is its hairline, drawn inside it, and Text colors the
	// text. Shadow is the color of the shadow a tooltip draws itself where
	// the OS draws none behind its popup (offset by ShadowOffsetY with
	// ShadowBlur, render.Shadow semantics).
	Background, Border, Text, Shadow ColorToken

	// BorderWidth is the border's width (rounded to whole device pixels,
	// at least one); CornerRadius is the panel's corner radius.
	BorderWidth, CornerRadius float64

	// ShadowOffsetY and ShadowBlur shape the tooltip's own shadow.
	ShadowOffsetY, ShadowBlur float64

	// PaddingX and PaddingY are the space between the panel's edges and the
	// text; FontSize is the size of the text's system font
	// (NSFont.toolTipsFont); a text that would make the panel wider than
	// MaxWidth wraps.
	PaddingX, PaddingY, FontSize, MaxWidth float64

	// OffsetX and OffsetY place the panel's top-left corner from the
	// pointer's hot spot: below the arrow cursor.
	OffsetX, OffsetY float64

	// Delay is how long the pointer has to rest on a widget with a tooltip
	// before it shows; for Grace after a tooltip was hidden by moving off
	// its widget, the next one shows at once.
	Delay, Grace time.Duration
}

// PopUp holds the tokens of the drawn PopUpButton (PLAN.md S5.5j). It is a
// push-button bezel (Metrics.Control, Controls.BezelBorder, ButtonFill)
// with the selected title on the left, in the system font of the control
// size, and on the right the indicator: a rounded rect inset into the
// bezel, filled with the accent in a key window (PopUpIndicatorFill,
// popup.go), with two procedural chevrons, one pointing up and one down,
// stacked in its centre. Lengths are in points.
type PopUp struct {
	// IndicatorInset is the space between the bezel's top, right and
	// bottom edges and the indicator; its corner radius is the bezel's
	// minus this. IndicatorGap is the space between the title's
	// truncation edge and the indicator.
	IndicatorInset, IndicatorGap float64

	// ChevronStroke is the chevrons' stroke width (round caps and joins);
	// ChevronGap the space between the open sides of the up and the down
	// chevron.
	ChevronStroke, ChevronGap float64

	// PaddingX is the space between the bezel's left edge and the
	// left-aligned title, per control size.
	PaddingX SizeTable

	// IndicatorWidth is the indicator's width per control size; its
	// height is the bezel's minus twice IndicatorInset.
	IndicatorWidth SizeTable

	// ChevronWidth is the width of each chevron per control size; a
	// chevron is a right angle, half as high as wide.
	ChevronWidth SizeTable
}

// Menu holds the tokens of the drawn menus (PLAN.md S5.5h): a pop-up
// button's menu, and the title-bar menus from S5.6. A menu is a rounded
// panel with a column of items: titles with a checkmark column on their
// left, and separators. The highlighted item is a rounded fill in the
// accent (Theme.MenuHighlight, menu.go), inset from the panel's sides.
// Lengths are in points.
type Menu struct {
	// Background fills a menu whose popup the OS blurs behind
	// (shell.Popup.Translucent): a translucent tint over the blur.
	// OpaqueBackground fills it everywhere else (no blur: Windows 10, high
	// contrast, transparency effects off, Linux). Border is the panel's
	// hairline, drawn inside it. Shadow is the color of the shadow a menu
	// draws itself where the OS draws none (offset by ShadowOffsetY with
	// ShadowBlur, render.Shadow semantics).
	Background, OpaqueBackground, Border, Shadow ColorToken

	// Text colors an enabled item's title and checkmark, DisabledText a
	// disabled item's, HighlightedText the highlighted item's (on the
	// accent fill).
	Text, DisabledText, HighlightedText ColorToken

	// Separator colors a separator item's line.
	Separator ColorToken

	// BorderWidth is the border's width (rounded to whole device pixels,
	// at least one); CornerRadius is the panel's corner radius.
	BorderWidth, CornerRadius float64

	// ShadowOffsetY and ShadowBlur shape the menu's own shadow.
	ShadowOffsetY, ShadowBlur float64

	// PaddingY is the space above the first item and below the last.
	PaddingY float64

	// HighlightInsetX is the space between the panel's sides and the
	// highlight, HighlightRadius the highlight's corner radius.
	HighlightInsetX, HighlightRadius float64

	// CheckColumnWidth is the column between the highlight's left edge and
	// the titles; a checkmark is centred in it, CheckmarkSize wide and
	// high, stroked CheckmarkStroke wide. TitlePaddingRight is the space
	// between the widest title and the panel's right side.
	CheckColumnWidth, CheckmarkSize, CheckmarkStroke, TitlePaddingRight float64

	// SeparatorHeight is the height of a separator item, whose line is
	// SeparatorThickness thick (whole device pixels, at least one),
	// centred in it and SeparatorInsetX from the panel's sides.
	SeparatorHeight, SeparatorThickness, SeparatorInsetX float64

	// ScrollArrowHeight is the height of the strip at the top or bottom of
	// a menu lower than its items (S5.5i), shown while the items can
	// scroll that way; its arrow, a triangle in Text pointing up or down,
	// is ScrollArrowSize wide and half as high, centred in it.
	ScrollArrowHeight, ScrollArrowSize float64

	// ShortcutGap is the least space between the widest title and the
	// shortcut column (S5.6b), which holds the items' shortcuts and submenu
	// chevrons right-aligned TitlePaddingRight from the panel's right side;
	// a menu without either has no column. A submenu chevron points right,
	// SubmenuChevronSize high and half as wide, stroked
	// SubmenuChevronStroke wide in the title's color.
	ShortcutGap, SubmenuChevronSize, SubmenuChevronStroke float64

	// ItemHeight is the height of a title item per control size; the
	// title, in the system font of that size, is centred in it.
	ItemHeight SizeTable
}

// TextView holds the tokens of the drawn TextView (PLAN.md S5.4f): the
// geometry of its text container, as NSTextView's defaults. The text view
// draws its text with the TextField's colors and caret (Caret,
// TextSelection, CaretWidth) on ColorTextBackground. Lengths are in points.
type TextView struct {
	// ContainerInsetY is the space above the first line and below the
	// last (NSTextView.textContainerInset.height).
	ContainerInsetY float64

	// LineFragmentPadding is the space left and right of every line
	// (NSTextContainer.lineFragmentPadding): the text wraps at the
	// viewport width minus twice this.
	LineFragmentPadding float64
}

// TextField holds the tokens of the drawn TextField and SecureField
// (PLAN.md S5.2e). The bezel is the field's whole frame (S5.2f, as an
// NSTextField's square bezel) and is filled with ColorTextBackground; the text is ColorText (ColorDisabledControlText
// when disabled), the placeholder ColorPlaceholderText and selected text
// ColorSelectedText. The caret and the selection colors follow the accent
// and the focus, so they are computed (Caret and TextSelection,
// textfield.go), not stored. Lengths are in points.
type TextField struct {
	// FieldBorder is the bezel's border, FieldBorderWidth wide and drawn
	// inside the bezel; FieldInsetShadow is the inset shadow under its top
	// edge, offset by FieldInsetShadowOffsetY with FieldInsetShadowBlur
	// (render.Shadow semantics).
	FieldBorder, FieldInsetShadow ColorToken

	FieldBorderWidth, FieldInsetShadowOffsetY, FieldInsetShadowBlur float64

	// CaretWidth is the width of the insertion point.
	CaretWidth float64

	// FieldCornerRadius is the bezel's corner radius; FieldPaddingX is the
	// space between the bezel's edge (its border included) and the text on
	// either side. The text is centred vertically in the bezel.
	FieldCornerRadius, FieldPaddingX SizeTable
}

// Scroll holds the tokens of the drawn scroll view (PLAN.md §5.6, S3.5a):
// the overlay and legacy scrollers and the scroll physics. The fade timing
// and the spring-back stay in Animation (ScrollerFadeDelay,
// ScrollerFadeDuration, RubberBandSpring). Lengths are in points, velocities
// in points per second.
type Scroll struct {
	// ScrollerKnob colors the knob of an overlay or a legacy scroller;
	// LegacyScrollerTrack fills a legacy scroller's track and
	// LegacyScrollerBorder is the hairline between it and the content.
	ScrollerKnob, LegacyScrollerTrack, LegacyScrollerBorder ColorToken

	// ScrollerThickness is the width of an idle overlay knob,
	// ScrollerHoverThickness that of a hovered one (and of a legacy knob);
	// ScrollerInset is the space between the knob and the viewport's edge
	// (or the edges of a legacy track); ScrollerMinKnobLength is the least
	// knob length before rubber-banding shortens it.
	ScrollerThickness, ScrollerHoverThickness, ScrollerInset, ScrollerMinKnobLength float64

	// LegacyScrollerThickness is the width of a legacy scroller's track,
	// taken from the viewport.
	LegacyScrollerThickness float64

	// WheelLineHeight is how many points one wheel line scrolls.
	WheelLineHeight float64

	// MomentumDecayRate is the rate k (per second) at which momentum
	// decays, v(t) = v0·e^(−k·t); momentum stops below
	// MomentumStopVelocity.
	MomentumDecayRate, MomentumStopVelocity float64

	// RubberBandFraction is the rubber-band constant c (§5.6) as a fraction
	// of the viewport's length on that axis: the overscroll never reaches
	// it.
	RubberBandFraction float64

	// PageOverlap is how much of the old page stays visible on Page Up and
	// Page Down.
	PageOverlap float64

	// ScrollerHoverDuration is how long an overlay knob takes to widen on
	// hover; WheelStepDuration is the smooth scroll of one wheel notch or
	// key step; VelocityWindow is how far back the precise samples before a
	// release count for the momentum velocity.
	ScrollerHoverDuration, WheelStepDuration, VelocityWindow time.Duration
}

// Controls holds the tokens of the drawn controls (PLAN.md S3.3b, S3.4a).
// The fills that follow the accent (a default button, a checked checkbox,
// an on radio, the filled part of a slider or progress bar) are not
// stored: ButtonFill, ToggleFill and ValueFill compute them with the accent
// rules (controls.go). Sizes per control size are of the visible part (the
// bezel, the box, the circle, the track, the knob), without the focus ring.
type Controls struct {
	// BezelFill and BezelPressedFill fill an off-state bezel (a push
	// button, a checkbox's box, a radio's circle) normally and while
	// pressed; BezelBorder is its 1 pt border.
	BezelFill, BezelPressedFill, BezelBorder ColorToken

	// BezelOutline is the solid outline Increase Contrast draws around a
	// bezel whose normal look has no border: an accent-filled one (a
	// default button, a checked or mixed checkbox's box, an on radio's
	// circle, the pop-up button's indicator). It is transparent without
	// Increase Contrast; the off-state bezels darken BezelBorder instead
	// (S6.1d; painted from S6.1e).
	BezelOutline ColorToken

	// BoxFill and BoxBorder draw a Box's rounded group.
	BoxFill, BoxBorder ColorToken

	// BoxCornerRadius and BoxBorderWidth shape the group; BoxTitleSize is
	// the font size of the title drawn above it, BoxTitleGap the space
	// between the title and the group; BoxContentInsetX and
	// BoxContentInsetY inset the content from the group's edges.
	BoxCornerRadius, BoxBorderWidth, BoxTitleSize, BoxTitleGap float64
	BoxContentInsetX, BoxContentInsetY                         float64

	// SeparatorThickness is the width of a separator's line.
	SeparatorThickness float64

	// ToggleDisabledAlpha is the opacity a disabled checkbox's box (with
	// its mark) and a disabled radio's circle are drawn with.
	ToggleDisabledAlpha float32

	// ButtonMinWidth is the least width of a push button's bezel.
	ButtonMinWidth SizeTable

	// CheckboxSize is the side of a checkbox's box, CheckboxRadius its
	// corner radius, CheckboxMarkStroke the width of the check and the
	// dash, CheckboxGap the space between the box and the title.
	CheckboxSize, CheckboxRadius, CheckboxMarkStroke, CheckboxGap SizeTable

	// RadioSize is the diameter of a radio's circle, RadioDotSize that of
	// the dot of an on radio, RadioGap the space before the title.
	RadioSize, RadioDotSize, RadioGap SizeTable

	// ButtonTitleWeight is the weight of a push button's title (on a
	// default button too), BoxTitleWeight that of a Box's title.
	ButtonTitleWeight, BoxTitleWeight driver.FontWeight

	// The value controls (S3.4a): slider, progress bar and spinner.

	// SliderTrack is the empty part of a slider's track (the filled part
	// is ValueFill); SliderKnobFill, SliderKnobBorder and SliderKnobShadow
	// draw the round knob; SliderTickMark colors the tick marks.
	SliderTrack, SliderKnobFill, SliderKnobBorder, SliderKnobShadow, SliderTickMark ColorToken

	// SliderTrackOutline is the solid outline Increase Contrast draws
	// around a slider's whole track, the filled part included; transparent
	// without Increase Contrast, as BezelOutline (S6.1d; painted from
	// S6.1e).
	SliderTrackOutline ColorToken

	// ProgressTrack is the empty track of a progress bar (the filled part
	// and the indeterminate band are ValueFill); SpinnerSpoke is the color
	// of a spinner's leading spoke.
	ProgressTrack, SpinnerSpoke ColorToken

	// SliderKnobBorderWidth is the width of the knob's border, drawn inside
	// it.
	SliderKnobBorderWidth float64

	// ProgressBandFraction is the width of an indeterminate progress bar's
	// band as a fraction of the track's width.
	ProgressBandFraction float64

	// SpinnerSpokes is the number of a spinner's spokes (a whole number);
	// SpinnerTailAlpha is the opacity of the faintest spoke relative to
	// SpinnerSpoke, the spokes between fading linearly.
	SpinnerSpokes    float64
	SpinnerTailAlpha float32

	// ValueDisabledAlpha is the opacity a disabled slider or progress
	// indicator is drawn with; ValueFill applies it to the filled part.
	ValueDisabledAlpha float32

	// SliderTrackThickness and SliderTrackRadius shape a horizontal
	// slider's track (its height; a vertical one turns it); SliderKnobSize
	// is the knob's diameter, SliderKnobShadowOffsetY and
	// SliderKnobShadowBlur its shadow (in SliderKnobShadow).
	SliderTrackThickness, SliderTrackRadius, SliderKnobSize SizeTable
	SliderKnobShadowOffsetY, SliderKnobShadowBlur           SizeTable

	// SliderTickLength and SliderTickWidth are the size of a tick mark,
	// SliderTickGap the space between the knob's edge and the marks.
	SliderTickLength, SliderTickWidth, SliderTickGap SizeTable

	// ProgressThickness and ProgressRadius shape a horizontal progress
	// bar's track.
	ProgressThickness, ProgressRadius SizeTable

	// SpinnerSize is a spinner's diameter; each spoke is SpinnerSpokeLength
	// long from the outer edge inwards and SpinnerSpokeWidth wide, with
	// round caps.
	SpinnerSize, SpinnerSpokeLength, SpinnerSpokeWidth SizeTable

	// ProgressBandPeriod is how long an indeterminate progress bar's band
	// takes to cross the track once; SpinnerPeriod is one turn of a
	// spinner. Both loop until the control stops; with Reduce Motion they
	// stand still (S3.4b).
	ProgressBandPeriod, SpinnerPeriod time.Duration
}

// Chrome holds the tokens of the drawn title bar and its traffic lights
// (PLAN.md §5.4) that are not semantic colors. All colors are concrete.
type Chrome struct {
	// TitleBar and TitleBarInactive fill the title bar of a key and of an
	// inactive window; TitleBarSeparator is the hairline at its bottom.
	TitleBar, TitleBarInactive, TitleBarSeparator ColorToken

	// The fill, border and glyph of an active close, minimize and zoom
	// light.
	CloseFill, CloseBorder, CloseGlyph          ColorToken
	MinimizeFill, MinimizeBorder, MinimizeGlyph ColorToken
	ZoomFill, ZoomBorder, ZoomGlyph             ColorToken

	// LightInactiveFill and LightInactiveBorder draw a grey light: in an
	// inactive window, or disabled because its style flag is missing.
	LightInactiveFill, LightInactiveBorder ColorToken

	// LightPressedShade is the factor a pressed light scales the color
	// channels of its fill, border and glyph by; LightDisabledAlpha is the
	// alpha of a disabled light.
	LightPressedShade, LightDisabledAlpha float32

	// TitleBarHeight is the height of the title bar. The traffic lights are
	// circles of LightSize, LightStep apart (centre to centre), the close
	// light LightInset from the left edge, vertically centred in the bar;
	// the title never comes closer to them than TitleGap.
	TitleBarHeight, LightSize, LightStep, LightInset, TitleGap float64

	// LightBorderWidth is the width of a light's border, stroked just inside
	// the circle, and LightGlyphStroke that of its ×, − and +.
	LightBorderWidth, LightGlyphStroke float64
}

// NumControlSizes is the number of driver.ControlSize values (regular,
// small, mini, large), the length of a SizeTable.
const NumControlSizes = 4

// SizeTable holds one number per control size, indexed by
// driver.ControlSize.
type SizeTable [NumControlSizes]float64

// At returns the entry of control size cs; an unknown size gets the
// regular entry.
func (t *SizeTable) At(cs driver.ControlSize) float64 {
	if int(cs) >= NumControlSizes {
		cs = driver.ControlSizeRegular
	}
	return t[cs]
}

// Metrics holds the control metrics that are not tied to one control kind
// (PLAN.md §5.2): the bezel of a push button, pop-up button or text field
// per control size, and the keyboard focus ring.
type Metrics struct {
	// ControlHeight is the height of the visible bezel, ControlPaddingX
	// the horizontal space between its edge and its content, and
	// ControlCornerRadius its corner radius.
	ControlHeight, ControlPaddingX, ControlCornerRadius SizeTable

	// The bezel shadow with render.Shadow (CSS box-shadow) semantics: its
	// vertical offset, blur and spread per control size, and its color.
	BezelShadowOffsetY, BezelShadowBlur, BezelShadowSpread SizeTable
	BezelShadow                                            ColorToken

	// FocusRingWidth is the width of the keyboard focus ring, drawn
	// FocusRingOffset outside the control's bezel (negative: inside).
	FocusRingWidth, FocusRingOffset float64
}

// ControlMetrics are the bezel metrics of one control size (Metrics.Control).
type ControlMetrics struct {
	Height, PaddingX, CornerRadius          float64
	ShadowOffsetY, ShadowBlur, ShadowSpread float64
}

// Control returns the bezel metrics of control size cs; an unknown size
// gets the regular ones.
func (m *Metrics) Control(cs driver.ControlSize) ControlMetrics {
	return ControlMetrics{
		Height:        m.ControlHeight.At(cs),
		PaddingX:      m.ControlPaddingX.At(cs),
		CornerRadius:  m.ControlCornerRadius.At(cs),
		ShadowOffsetY: m.BezelShadowOffsetY.At(cs),
		ShadowBlur:    m.BezelShadowBlur.At(cs),
		ShadowSpread:  m.BezelShadowSpread.At(cs),
	}
}

// Typography holds the text tokens. The UI font of Windows and Linux is
// Inter (PLAN.md §3.1); the tracking table spaces it like SF Pro.
type Typography struct {
	// FontSize is the system font size per control size. It equals
	// driver.Font.EffectiveSize for a zero Size (a test pins it).
	FontSize SizeTable

	// LineHeight is the line height as a multiple of the font size.
	LineHeight float64

	// WindowTitleWeight is the weight of the drawn window title, and
	// EmphasizedWeight that of emphasized control text (AppKit's bold
	// system font).
	WindowTitleWeight, EmphasizedWeight driver.FontWeight

	// Tracking is the SF-like tracking table, by strictly ascending Size.
	Tracking []TrackingStep
}

// TrackingStep is one row of the tracking table: at the font size Size
// (points) the letter spacing is Tracking thousandths of an em (negative
// tightens).
type TrackingStep struct {
	Size, Tracking float64
}

// TrackingAt returns the tracking at the font size size in thousandths of
// an em: linear between two rows of the table, the first or the last row's
// value outside it, and 0 for an empty table.
func (t *Typography) TrackingAt(size float64) float64 {
	tr := t.Tracking
	if len(tr) == 0 {
		return 0
	}
	if size <= tr[0].Size {
		return tr[0].Tracking
	}
	for i := 1; i < len(tr); i++ {
		if size <= tr[i].Size {
			a, b := tr[i-1], tr[i]
			return a.Tracking + (b.Tracking-a.Tracking)*(size-a.Size)/(b.Size-a.Size)
		}
	}
	return tr[len(tr)-1].Tracking
}

// Accent holds the parameters of the accent rules (accent.go): the colors
// that depend on Appearance.Accent are computed from it, never stored, so an
// OS or app accent changes them all.
type Accent struct {
	// Base is the opaque backdrop the derived selection and inactive colors
	// are composited over (white in light, a mid grey in dark).
	Base ColorToken

	// DefaultButtonText is the label color on an accent fill (a default
	// button, a checked checkbox's mark).
	DefaultButtonText ColorToken

	// AccentDarkText replaces DefaultButtonText on a light accent, one
	// that DefaultButtonText has a contrast ratio below
	// AccentTextMinContrast on (S6.1g; Windows lets the user pick any
	// accent).
	AccentDarkText ColorToken

	// SelectionAlpha is the alpha of the accent over Base in the selection
	// colors (ColorSelectedTextBackground, ColorSelectedControl);
	// FocusRingAlpha the alpha of the keyboard focus ring.
	SelectionAlpha, FocusRingAlpha float32

	// SelectionContrastAlpha replaces SelectionAlpha with
	// Appearance.IncreaseContrast: a stronger selection that keeps the
	// selected text readable (S6.1e).
	SelectionContrastAlpha float32

	// PressedShade is the factor a pressed accent fill scales the color
	// channels by.
	PressedShade float32

	// InactiveAlpha is the alpha of the accent's grey (its luminance) over
	// Base: accent fills in an inactive window.
	InactiveAlpha float32

	// AccentTextMinContrast is the WCAG 2 contrast ratio DefaultButtonText
	// needs on the accent to stay the label of an accent fill (AccentText).
	AccentTextMinContrast float32
}

// Animation holds the animation tokens (PLAN.md §5.5): durations, timing
// curves and springs. The animation engine (internal/aqua/anim, S3.2) runs
// them; with Appearance.ReduceMotion the Timeline makes them jump
// (anim.Timeline.SetReduceMotion).
type Animation struct {
	// DurationFast is for hover and press feedback, DurationNormal the
	// default (0.2–0.25 s), DurationSlow for larger transitions.
	DurationFast, DurationNormal, DurationSlow time.Duration

	// FocusRingDuration is how long the focus ring takes to animate in.
	FocusRingDuration time.Duration

	// An overlay scroller fades out ScrollerFadeDelay after scrolling
	// stops, over ScrollerFadeDuration.
	ScrollerFadeDelay, ScrollerFadeDuration time.Duration

	// The cubic-bezier timing curves (CSS / CAMediaTimingFunction):
	// EaseDefault is Core Animation's default.
	EaseIn, EaseOut, EaseInOut, EaseDefault Curve

	// ControlSpring animates control state (a switch's knob, a moving
	// selection); RubberBandSpring brings a rubber-banded scroll view back
	// to its edge.
	ControlSpring, RubberBandSpring Spring
}

// Curve is a cubic-bezier timing curve from (0, 0) to (1, 1) with the
// control points (X1, Y1) and (X2, Y2); X1 and X2 are in 0..1.
type Curve struct {
	X1, Y1, X2, Y2 float64
}

// Spring is a damped spring: Mass, Stiffness and Damping as in
// CASpringAnimation (all positive).
type Spring struct {
	Mass, Stiffness, Damping float64
}

// DampingRatio returns the spring's damping ratio: 1 is critically damped
// (no overshoot), below 1 it overshoots.
func (s Spring) DampingRatio() float64 {
	return s.Damping / (2 * math.Sqrt(s.Stiffness*s.Mass))
}

// Colors holds one ColorToken per semantic color, indexed by
// driver.SemanticColor. Index 0 (SemanticNone) is unused and zero.
type Colors [driver.NumSemanticColors + 1]ColorToken

// ColorToken is the value of one semantic color in each appearance. All
// colors are concrete (never semantic).
type ColorToken struct {
	Light, Dark driver.Color

	// HasContrast reports whether the color has its own increased-contrast
	// values; without them it keeps Light and Dark when
	// Appearance.IncreaseContrast is set.
	HasContrast                 bool
	ContrastLight, ContrastDark driver.Color
}

// Value returns the token's color for the appearance a: the dark or light
// value, or its increased-contrast variant when a asks for one and the token
// has it. The accent rules of the aqua driver are not applied here.
func (t ColorToken) Value(a driver.Appearance) driver.Color {
	switch {
	case a.IncreaseContrast && t.HasContrast && a.Dark:
		return t.ContrastDark
	case a.IncreaseContrast && t.HasContrast:
		return t.ContrastLight
	case a.Dark:
		return t.Dark
	}
	return t.Light
}

// Color returns the value of the semantic color s for the appearance a. It
// reports false for SemanticNone and for unknown colors.
func (t *Theme) Color(s driver.SemanticColor, a driver.Appearance) (driver.Color, bool) {
	if s == driver.SemanticNone || !s.Valid() {
		return driver.Color{}, false
	}
	return t.Colors[s].Value(a), true
}

// Unknown is what Resolve returns for a semantic color it does not know:
// opaque magenta, so a missing token shows in dumps and on screen instead of
// failing the paint.
var Unknown = driver.RGB8(255, 0, 255)

// Resolve returns c as a concrete color for the appearance a. A concrete c
// is returned unchanged. The semantic colors that follow the accent are
// computed by the accent rules (accent.go; their tokens hold the
// default-accent values for reference): ColorControlAccent,
// ColorSelectedContentBackground and ColorSystemBlue resolve to a.Accent,
// ColorKeyboardFocusIndicator to FocusRing, ColorSelectedTextBackground and
// ColorSelectedControl to Selection. The other semantic colors resolve to
// their token (with its increased-contrast variant when a asks for one),
// unknown ones to Unknown.
func (t *Theme) Resolve(c driver.Color, a driver.Appearance) driver.Color {
	if !c.IsSemantic() {
		return c
	}
	switch c.Semantic {
	case driver.ColorControlAccent, driver.ColorSelectedContentBackground, driver.ColorSystemBlue:
		return a.Accent
	case driver.ColorKeyboardFocusIndicator:
		return t.FocusRing(a)
	case driver.ColorSelectedTextBackground, driver.ColorSelectedControl:
		return t.Selection(a)
	}
	v, ok := t.Color(c.Semantic, a)
	if !ok {
		return Unknown
	}
	return v
}

// rgba8 returns a concrete sRGB color from 8-bit components with straight
// alpha. The generated tokens use it; an alpha of 255 gives the same color as
// driver.RGB8.
func rgba8(r, g, b, a uint8) driver.Color {
	c := driver.RGB8(r, g, b)
	c.A = float32(a) / 255
	return c
}
