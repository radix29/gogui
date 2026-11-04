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
// drawn controls (Controls) and those of the scroll view (Scroll). Lengths
// are in logical points. Colors with an
// increased-contrast variant use it when Appearance.IncreaseContrast is
// set.
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
	ToggleDisabledAlpha float64

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
	SpinnerSpokes, SpinnerTailAlpha float64

	// ValueDisabledAlpha is the opacity a disabled slider or progress
	// indicator is drawn with; ValueFill applies it to the filled part.
	ValueDisabledAlpha float64

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

// LineHeightFor returns the line height of a font of size size, in points.
func (t *Typography) LineHeightFor(size float64) float64 { return t.LineHeight * size }

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

// TrackingPoints returns the tracking at the font size size in points
// (TrackingAt × size / 1000): the spacing to add after each character.
func (t *Typography) TrackingPoints(size float64) float64 {
	return t.TrackingAt(size) * size / 1000
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

	// SelectionAlpha is the alpha of the accent over Base in the selection
	// colors (ColorSelectedTextBackground, ColorSelectedControl);
	// FocusRingAlpha the alpha of the keyboard focus ring.
	SelectionAlpha, FocusRingAlpha float32

	// PressedShade is the factor a pressed accent fill scales the color
	// channels by.
	PressedShade float32

	// InactiveAlpha is the alpha of the accent's grey (its luminance) over
	// Base: accent fills in an inactive window.
	InactiveAlpha float32
}

// Animation holds the animation tokens (PLAN.md §5.5): durations, timing
// curves and springs. The animation engine (internal/aqua/anim, S3.2) runs
// them; with Appearance.ReduceMotion every duration is zero (Duration).
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

// Duration returns d for the appearance a: zero with Reduce Motion, so the
// change happens at once.
func (an *Animation) Duration(d time.Duration, a driver.Appearance) time.Duration {
	if a.ReduceMotion {
		return 0
	}
	return d
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
