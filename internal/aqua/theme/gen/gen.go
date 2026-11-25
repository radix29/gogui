// Package gen turns a theme spec (internal/aqua/theme/spec/*.json) into the
// generated Go file of package theme and its Markdown listing (PLAN.md
// S3.1a, S3.1c, S3.1d, S3.3b, S3.4a, S3.5a, S5.2e, S5.4f, S5.5h, S5.5j, S5.5l, S6.1d). It uses
// only the standard library; the gentokens command wraps it for go
// generate.
//
// The spec is a JSON object:
//
//	{
//	  "name": "Sequoia",                  // Go identifier of the generated Theme
//	  "comment": "…",                     // optional, one line
//	  "colors": {
//	    "ColorLabel": {                   // a driver.SemanticColor constant
//	      "light": [0, 0, 0, 217],        // r, g, b and optional a, 0..255
//	      "dark": [255, 255, 255, 217],
//	      "contrastLight": [0, 0, 0, 255], // optional; both or neither
//	      "contrastDark": [255, 255, 255, 255],
//	      "source": "memory",             // "memory" (to be measured) or "measured"
//	      "note": "…"                     // optional, one line
//	    },
//	    …
//	  },
//	  "chrome": {                         // theme.Chrome
//	    "colors": {"TitleBar": {"light": […], "dark": […], "source": "memory"}, …},
//	    "numbers": {"TitleBarHeight": {"value": 28, "source": "memory", "note": "…"}, …}
//	  },
//	  "metrics": {                        // theme.Metrics
//	    "colors": {…}, "numbers": {…},
//	    "sizes": {                        // a theme.SizeTable: one value per control size
//	      "ControlHeight": {"regular": 22, "small": 19, "mini": 16, "large": 28, "source": "memory"},
//	      …
//	    }
//	  },
//	  "typography": {                     // theme.Typography
//	    "sizes": {…},
//	    "weights": {"WindowTitleWeight": {"value": 600, "source": "memory"}, …},
//	    "tracking": {"steps": [[9, 19], [13, -8], …], "source": "memory", "note": "…"}
//	  },
//	  "accent": {"colors": {…}, "numbers": {…}},  // theme.Accent
//	  "animation": {                      // theme.Animation
//	    "durations": {"DurationNormal": {"ms": 250, "source": "memory"}, …},
//	    "curves": {"EaseInOut": {"value": [0.42, 0, 0.58, 1], "source": "memory"}, …},
//	    "springs": {"ControlSpring": {"mass": 1, "stiffness": 300, "damping": 30, "source": "memory"}, …}
//	  },
//	  "controls": {                       // theme.Controls
//	    "colors": {…}, "numbers": {…}, "sizes": {…}, "weights": {…}, "durations": {…}
//	  },
//	  "scroll": {"colors": {…}, "numbers": {…}, "durations": {…}},  // theme.Scroll
//	  "textField": {"colors": {…}, "numbers": {…}, "sizes": {…}},    // theme.TextField
//	  "textView": {"numbers": {…}},                                   // theme.TextView
//	  "menu": {"colors": {…}, "numbers": {…}, "sizes": {…}},          // theme.Menu
//	  "popUp": {"numbers": {…}, "sizes": {…}},                        // theme.PopUp
//	  "menuBar": {"colors": {…}, "numbers": {…}},                     // theme.MenuBar
//	  "tooltip": {"colors": {…}, "numbers": {…}, "durations": {…}},   // theme.Tooltip
//	  "alert": {"colors": {…}, "numbers": {…}}                        // theme.Alert
//	}
//
// Every semantic color must be present exactly once, and each group must
// hold exactly the tokens its Group lists (each once, nothing else).
package gen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"math"
	"strconv"
	"strings"
)

// ColorNames lists the driver.SemanticColor constants in declaration order,
// SemanticNone excluded. A spec must define each of them; the generated file
// and the listing keep this order. A test checks it against the driver.
var ColorNames = []string{
	"ColorLabel",
	"ColorSecondaryLabel",
	"ColorTertiaryLabel",
	"ColorQuaternaryLabel",

	"ColorText",
	"ColorPlaceholderText",
	"ColorSelectedText",
	"ColorTextBackground",
	"ColorSelectedTextBackground",
	"ColorUnemphasizedSelectedTextBackground",

	"ColorWindowBackground",
	"ColorUnderPageBackground",
	"ColorControlBackground",
	"ColorSelectedContentBackground",
	"ColorUnemphasizedSelectedContentBackground",
	"ColorSeparator",
	"ColorGrid",
	"ColorLink",
	"ColorFindHighlight",

	"ColorControl",
	"ColorControlText",
	"ColorDisabledControlText",
	"ColorSelectedControl",
	"ColorSelectedControlText",
	"ColorControlAccent",
	"ColorKeyboardFocusIndicator",

	"ColorSystemRed",
	"ColorSystemOrange",
	"ColorSystemYellow",
	"ColorSystemGreen",
	"ColorSystemMint",
	"ColorSystemTeal",
	"ColorSystemCyan",
	"ColorSystemBlue",
	"ColorSystemIndigo",
	"ColorSystemPurple",
	"ColorSystemPink",
	"ColorSystemBrown",
	"ColorSystemGray",
}

// Group is the schema of one token group of a spec: the Theme field it
// fills (of a struct type of the same name in theme.go) and the names of
// that struct's fields by kind, each in declaration order. A test checks
// them against theme.go.
type Group struct {
	Field string // the Theme field and its type, e.g. "Chrome"
	Key   string // the spec's JSON key, e.g. "chrome"
	Title string // the listing's section heading

	Colors  []string // theme.ColorToken fields
	Numbers []string // float32 and float64 fields
	Sizes   []string // theme.SizeTable fields
	Weights []string // driver.FontWeight fields

	Durations []string // time.Duration fields
	Curves    []string // theme.Curve fields
	Springs   []string // theme.Spring fields

	// Tracking names the []theme.TrackingStep field, "" for none.
	Tracking string
}

// Groups lists the token groups in the order of the Theme fields.
var Groups = []Group{
	{
		Field: "Chrome", Key: "chrome", Title: "Window chrome",
		Colors: []string{
			"TitleBar", "TitleBarInactive", "TitleBarSeparator",
			"CloseFill", "CloseBorder", "CloseGlyph",
			"MinimizeFill", "MinimizeBorder", "MinimizeGlyph",
			"ZoomFill", "ZoomBorder", "ZoomGlyph",
			"LightInactiveFill", "LightInactiveBorder",
		},
		Numbers: []string{
			"LightPressedShade", "LightDisabledAlpha",
			"TitleBarHeight", "LightSize", "LightStep", "LightInset", "TitleGap",
			"LightBorderWidth", "LightGlyphStroke",
		},
	},
	{
		Field: "Metrics", Key: "metrics", Title: "Control metrics",
		Sizes: []string{
			"ControlHeight", "ControlPaddingX", "ControlCornerRadius",
			"BezelShadowOffsetY", "BezelShadowBlur", "BezelShadowSpread",
		},
		Colors:  []string{"BezelShadow"},
		Numbers: []string{"FocusRingWidth", "FocusRingOffset"},
	},
	{
		Field: "Typography", Key: "typography", Title: "Typography",
		Sizes:    []string{"FontSize"},
		Numbers:  []string{"LineHeight"},
		Weights:  []string{"WindowTitleWeight", "EmphasizedWeight"},
		Tracking: "Tracking",
	},
	{
		Field: "Accent", Key: "accent", Title: "Accent rules",
		Colors:  []string{"Base", "DefaultButtonText", "AccentDarkText"},
		Numbers: []string{"SelectionAlpha", "FocusRingAlpha", "SelectionContrastAlpha", "PressedShade", "InactiveAlpha", "AccentTextMinContrast"},
	},
	{
		Field: "Animation", Key: "animation", Title: "Animation",
		Durations: []string{
			"DurationFast", "DurationNormal", "DurationSlow",
			"FocusRingDuration", "ScrollerFadeDelay", "ScrollerFadeDuration",
		},
		Curves:  []string{"EaseIn", "EaseOut", "EaseInOut", "EaseDefault"},
		Springs: []string{"ControlSpring", "RubberBandSpring"},
	},
	{
		Field: "Controls", Key: "controls", Title: "Controls",
		Colors: []string{
			"BezelFill", "BezelPressedFill", "BezelBorder", "BezelOutline", "BoxFill", "BoxBorder",
			"SliderTrack", "SliderKnobFill", "SliderKnobBorder", "SliderKnobShadow", "SliderTickMark",
			"SliderTrackOutline",
			"ProgressTrack", "SpinnerSpoke",
		},
		Numbers: []string{
			"BoxCornerRadius", "BoxBorderWidth", "BoxTitleSize", "BoxTitleGap",
			"BoxContentInsetX", "BoxContentInsetY", "SeparatorThickness", "ToggleDisabledAlpha",
			"SliderKnobBorderWidth", "ProgressBandFraction", "SpinnerSpokes", "SpinnerTailAlpha",
			"ValueDisabledAlpha",
		},
		Sizes: []string{
			"ButtonMinWidth",
			"CheckboxSize", "CheckboxRadius", "CheckboxMarkStroke", "CheckboxGap",
			"RadioSize", "RadioDotSize", "RadioGap",
			"SliderTrackThickness", "SliderTrackRadius", "SliderKnobSize",
			"SliderKnobShadowOffsetY", "SliderKnobShadowBlur",
			"SliderTickLength", "SliderTickWidth", "SliderTickGap",
			"ProgressThickness", "ProgressRadius",
			"SpinnerSize", "SpinnerSpokeLength", "SpinnerSpokeWidth",
		},
		Weights:   []string{"ButtonTitleWeight", "BoxTitleWeight"},
		Durations: []string{"ProgressBandPeriod", "SpinnerPeriod"},
	},
	{
		Field: "Scroll", Key: "scroll", Title: "Scrolling",
		Colors: []string{"ScrollerKnob", "LegacyScrollerTrack", "LegacyScrollerBorder"},
		Numbers: []string{
			"ScrollerThickness", "ScrollerHoverThickness", "ScrollerInset", "ScrollerMinKnobLength",
			"LegacyScrollerThickness",
			"WheelLineHeight", "MomentumDecayRate", "MomentumStopVelocity",
			"RubberBandFraction", "PageOverlap",
		},
		Durations: []string{"ScrollerHoverDuration", "WheelStepDuration", "VelocityWindow"},
	},
	{
		Field: "TextField", Key: "textField", Title: "Text fields",
		Colors:  []string{"FieldBorder", "FieldInsetShadow"},
		Numbers: []string{"FieldBorderWidth", "FieldInsetShadowOffsetY", "FieldInsetShadowBlur", "CaretWidth"},
		Sizes:   []string{"FieldCornerRadius", "FieldPaddingX"},
	},
	{
		Field: "TextView", Key: "textView", Title: "Text views",
		Numbers: []string{"ContainerInsetY", "LineFragmentPadding"},
	},
	{
		Field: "Menu", Key: "menu", Title: "Menus",
		Colors: []string{
			"Background", "OpaqueBackground", "Border", "Shadow",
			"Text", "DisabledText", "HighlightedText", "Separator",
		},
		Numbers: []string{
			"BorderWidth", "CornerRadius", "ShadowOffsetY", "ShadowBlur", "PaddingY",
			"HighlightInsetX", "HighlightRadius",
			"CheckColumnWidth", "CheckmarkSize", "CheckmarkStroke", "TitlePaddingRight",
			"SeparatorHeight", "SeparatorThickness", "SeparatorInsetX",
			"ScrollArrowHeight", "ScrollArrowSize",
			"ShortcutGap", "SubmenuChevronSize", "SubmenuChevronStroke",
		},
		Sizes: []string{"ItemHeight"},
	},
	{
		Field: "PopUp", Key: "popUp", Title: "Pop-up buttons",
		Numbers: []string{"IndicatorInset", "IndicatorGap", "ChevronStroke", "ChevronGap"},
		Sizes:   []string{"PaddingX", "IndicatorWidth", "ChevronWidth"},
	},
	{
		Field: "MenuBar", Key: "menuBar", Title: "Menu bar",
		Colors: []string{"Text", "InactiveText", "Highlight"},
		Numbers: []string{
			"StartGap", "TitlePaddingX", "HighlightHeight", "HighlightRadius",
			"MinDragWidth", "WindowTitleGap",
		},
	},
	{
		Field: "Tooltip", Key: "tooltip", Title: "Tooltips",
		Colors: []string{"Background", "Border", "Text", "Shadow"},
		Numbers: []string{
			"BorderWidth", "CornerRadius", "ShadowOffsetY", "ShadowBlur",
			"PaddingX", "PaddingY", "FontSize", "MaxWidth", "OffsetX", "OffsetY",
		},
		Durations: []string{"Delay", "Grace"},
	},
	{
		Field: "Alert", Key: "alert", Title: "Alerts",
		Colors: []string{
			"Background", "Border", "Shadow",
			"IconTop", "IconBottom", "IconBorder", "IconGlyph", "CautionFill", "CautionMark",
		},
		Numbers: []string{
			"Width", "CornerRadius", "BorderWidth", "ShadowOffsetY", "ShadowBlur",
			"PaddingX", "PaddingTop", "PaddingBottom",
			"IconSize", "IconCornerRadius", "BadgeSize", "IconGap",
			"MessageFontSize", "DetailFontSize", "DetailGap",
			"ButtonsGap", "ButtonGap",
		},
	},
}

// CurveNames and SpringNames are the parts of a curve and a spring, in the
// order of the theme.Curve and theme.Spring fields.
var (
	CurveNames  = [4]string{"x1", "y1", "x2", "y2"}
	SpringNames = [3]string{"mass", "stiffness", "damping"}
)

// SizeNames are the JSON keys of a size row, in driver.ControlSize order
// (the index of theme.SizeTable).
var SizeNames = [4]string{"regular", "small", "mini", "large"}

// Source says where a token value comes from.
type Source string

const (
	// SourceMemory is a provisional value from memory, to be measured
	// against the macOS references (S3.1t).
	SourceMemory Source = "memory"
	// SourceMeasured is a value measured from the reference captures.
	SourceMeasured Source = "measured"
)

// RGBA8 is an sRGB color with straight alpha, 8 bits per channel.
type RGBA8 struct{ R, G, B, A uint8 }

// Color is one validated color token: a semantic color or a color field
// of a group.
type Color struct {
	Name        string // the driver constant or field, e.g. "ColorLabel"
	Light, Dark RGBA8

	HasContrast                 bool
	ContrastLight, ContrastDark RGBA8

	Source Source
	Note   string
}

// Number is one validated number or weight token.
type Number struct {
	Name  string // the field, e.g. "LightPressedShade"
	Value string // the number as written in the spec

	Source Source
	Note   string
}

// Sizes is one validated size row: a value per control size.
type Sizes struct {
	Name   string
	Values [4]string // in SizeNames order, as written in the spec

	Source Source
	Note   string
}

// Tuple is one validated curve or spring: its parts as written in the
// spec, in CurveNames or SpringNames order.
type Tuple struct {
	Name   string
	Values []string

	Source Source
	Note   string
}

// Tracking is a validated tracking table.
type Tracking struct {
	Name  string
	Steps [][2]string // font size and tracking, as written in the spec

	Source Source
	Note   string
}

// GroupValues holds the validated tokens of one group.
type GroupValues struct {
	Group     *Group
	Colors    []Color  // in Group.Colors order
	Numbers   []Number // in Group.Numbers order
	Sizes     []Sizes  // in Group.Sizes order
	Weights   []Number // in Group.Weights order
	Durations []Number // in Group.Durations order; Value in milliseconds
	Curves    []Tuple  // in Group.Curves order
	Springs   []Tuple  // in Group.Springs order
	Tracking  *Tracking
}

// Spec is a validated theme spec.
type Spec struct {
	Name    string
	Comment string
	Colors  []Color       // in ColorNames order
	Groups  []GroupValues // in Groups order
}

// Group returns the values of the group whose Theme field is field, or
// nil.
func (s *Spec) Group(field string) *GroupValues {
	for i := range s.Groups {
		if s.Groups[i].Group.Field == field {
			return &s.Groups[i]
		}
	}
	return nil
}

// sources returns the source of every token of s.
func (s *Spec) sources() []Source {
	var src []Source
	for _, c := range s.Colors {
		src = append(src, c.Source)
	}
	for _, g := range s.Groups {
		for _, c := range g.Colors {
			src = append(src, c.Source)
		}
		for _, v := range g.Numbers {
			src = append(src, v.Source)
		}
		for _, v := range g.Sizes {
			src = append(src, v.Source)
		}
		for _, v := range g.Weights {
			src = append(src, v.Source)
		}
		for _, v := range g.Durations {
			src = append(src, v.Source)
		}
		for _, v := range g.Curves {
			src = append(src, v.Source)
		}
		for _, v := range g.Springs {
			src = append(src, v.Source)
		}
		if g.Tracking != nil {
			src = append(src, g.Tracking.Source)
		}
	}
	return src
}

// Tokens returns the number of tokens of s. A size row, a curve, a spring
// and a tracking table count as one token each.
func (s *Spec) Tokens() int { return len(s.sources()) }

// Provisional returns the number of tokens whose source is SourceMemory.
func (s *Spec) Provisional() int {
	n := 0
	for _, src := range s.sources() {
		if src == SourceMemory {
			n++
		}
	}
	return n
}

type rawSpec struct {
	Name       string                     `json:"name"`
	Comment    string                     `json:"comment"`
	Colors     map[string]json.RawMessage `json:"colors"`
	Chrome     *rawGroup                  `json:"chrome"`
	Metrics    *rawGroup                  `json:"metrics"`
	Typography *rawGroup                  `json:"typography"`
	Accent     *rawGroup                  `json:"accent"`
	Animation  *rawGroup                  `json:"animation"`
	Controls   *rawGroup                  `json:"controls"`
	Scroll     *rawGroup                  `json:"scroll"`
	TextField  *rawGroup                  `json:"textField"`
	TextView   *rawGroup                  `json:"textView"`
	Menu       *rawGroup                  `json:"menu"`
	PopUp      *rawGroup                  `json:"popUp"`
	MenuBar    *rawGroup                  `json:"menuBar"`
	Tooltip    *rawGroup                  `json:"tooltip"`
	Alert      *rawGroup                  `json:"alert"`
}

func (r *rawSpec) group(key string) *rawGroup {
	switch key {
	case "chrome":
		return r.Chrome
	case "metrics":
		return r.Metrics
	case "typography":
		return r.Typography
	case "accent":
		return r.Accent
	case "controls":
		return r.Controls
	case "animation":
		return r.Animation
	case "scroll":
		return r.Scroll
	case "textField":
		return r.TextField
	case "textView":
		return r.TextView
	case "menu":
		return r.Menu
	case "popUp":
		return r.PopUp
	case "menuBar":
		return r.MenuBar
	case "tooltip":
		return r.Tooltip
	case "alert":
		return r.Alert
	}
	panic("gen: no group " + key)
}

type rawGroup struct {
	Colors    map[string]json.RawMessage `json:"colors"`
	Numbers   map[string]json.RawMessage `json:"numbers"`
	Sizes     map[string]json.RawMessage `json:"sizes"`
	Weights   map[string]json.RawMessage `json:"weights"`
	Durations map[string]json.RawMessage `json:"durations"`
	Curves    map[string]json.RawMessage `json:"curves"`
	Springs   map[string]json.RawMessage `json:"springs"`
	Tracking  json.RawMessage            `json:"tracking"`
}

type rawDuration struct {
	MS     *json.Number `json:"ms"`
	Source *string      `json:"source"`
	Note   string       `json:"note"`
}

type rawCurve struct {
	Value  []json.Number `json:"value"`
	Source *string       `json:"source"`
	Note   string        `json:"note"`
}

type rawSpring struct {
	Mass      *json.Number `json:"mass"`
	Stiffness *json.Number `json:"stiffness"`
	Damping   *json.Number `json:"damping"`
	Source    *string      `json:"source"`
	Note      string       `json:"note"`
}

type rawNumber struct {
	Value  *json.Number `json:"value"`
	Source *string      `json:"source"`
	Note   string       `json:"note"`
}

type rawSizes struct {
	Regular *json.Number `json:"regular"`
	Small   *json.Number `json:"small"`
	Mini    *json.Number `json:"mini"`
	Large   *json.Number `json:"large"`
	Source  *string      `json:"source"`
	Note    string       `json:"note"`
}

type rawTracking struct {
	Steps  [][]json.Number `json:"steps"`
	Source *string         `json:"source"`
	Note   string          `json:"note"`
}

type rawColor struct {
	Light         []json.Number `json:"light"`
	Dark          []json.Number `json:"dark"`
	ContrastLight []json.Number `json:"contrastLight"`
	ContrastDark  []json.Number `json:"contrastDark"`
	Source        *string       `json:"source"`
	Note          string        `json:"note"`
}

// Parse decodes and validates a spec. It rejects unknown fields; a missing
// group; a missing or unknown token; a color without both appearances, with
// a lone increased-contrast value or with a channel that is not an integer
// in 0..255; a number or size that is not a finite float32; a weight that
// is not an integer in 1..1000; a duration that is not an integer number of
// milliseconds in 0..60000; a curve that is not four finite numbers with
// x1 and x2 in 0..1; a spring whose mass, stiffness or damping is missing,
// not finite or not positive; a tracking table that is missing, empty, in
// a group without one, or not by strictly ascending positive font size; a
// source other than "memory" or "measured"; a name that is not an exported
// Go identifier; and comments or notes that are not a single line.
func Parse(data []byte) (*Spec, error) {
	var raw rawSpec
	if err := decodeStrict(data, &raw); err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	if !token.IsIdentifier(raw.Name) || !token.IsExported(raw.Name) {
		return nil, fmt.Errorf("spec: name %q is not an exported Go identifier", raw.Name)
	}
	if err := checkLine(raw.Comment); err != nil {
		return nil, fmt.Errorf("spec: comment: %w", err)
	}
	if raw.Colors == nil {
		return nil, errors.New(`spec: no "colors"`)
	}
	for _, g := range Groups {
		if raw.group(g.Key) == nil {
			return nil, fmt.Errorf("spec: no %q", g.Key)
		}
	}
	s := &Spec{Name: raw.Name, Comment: raw.Comment}
	var err error
	if s.Colors, err = parseAll("color", ColorNames, raw.Colors, parseColor); err != nil {
		return nil, err
	}
	for i := range Groups {
		g := &Groups[i]
		gv, err := parseGroup(g, raw.group(g.Key))
		if err != nil {
			return nil, err
		}
		s.Groups = append(s.Groups, gv)
	}
	return s, nil
}

func parseGroup(g *Group, rg *rawGroup) (GroupValues, error) {
	gv := GroupValues{Group: g}
	var err error
	if gv.Colors, err = parseAll(g.Key+" color", g.Colors, rg.Colors, parseColor); err != nil {
		return gv, err
	}
	if gv.Numbers, err = parseAll(g.Key+" number", g.Numbers, rg.Numbers, parseNumber); err != nil {
		return gv, err
	}
	if gv.Sizes, err = parseAll(g.Key+" size", g.Sizes, rg.Sizes, parseSizes); err != nil {
		return gv, err
	}
	if gv.Weights, err = parseAll(g.Key+" weight", g.Weights, rg.Weights, parseWeight); err != nil {
		return gv, err
	}
	if gv.Durations, err = parseAll(g.Key+" duration", g.Durations, rg.Durations, parseDuration); err != nil {
		return gv, err
	}
	if gv.Curves, err = parseAll(g.Key+" curve", g.Curves, rg.Curves, parseCurve); err != nil {
		return gv, err
	}
	if gv.Springs, err = parseAll(g.Key+" spring", g.Springs, rg.Springs, parseSpring); err != nil {
		return gv, err
	}
	switch {
	case g.Tracking == "" && rg.Tracking != nil:
		return gv, fmt.Errorf("spec: %s has no tracking table", g.Key)
	case g.Tracking != "" && rg.Tracking == nil:
		return gv, fmt.Errorf("spec: %s tracking is missing", g.Key)
	case g.Tracking != "":
		t, err := parseTracking(g.Tracking, rg.Tracking)
		if err != nil {
			return gv, fmt.Errorf("spec: %s tracking: %w", g.Key, err)
		}
		gv.Tracking = &t
	}
	return gv, nil
}

// parseAll parses the entries of m, which must hold each of names exactly
// once and nothing else, in the order of names.
func parseAll[T any](kind string, names []string, m map[string]json.RawMessage, parse func(string, json.RawMessage) (T, error)) ([]T, error) {
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	for n := range m {
		if !known[n] {
			return nil, fmt.Errorf("spec: unknown %s %q", kind, n)
		}
	}
	out := make([]T, 0, len(names))
	for _, n := range names {
		msg, ok := m[n]
		if !ok {
			return nil, fmt.Errorf("spec: %s %s is missing", kind, n)
		}
		v, err := parse(n, msg)
		if err != nil {
			return nil, fmt.Errorf("spec: %s %s: %w", kind, n, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseFloat checks that n is a finite float32 and returns it as written.
func parseFloat(field string, n *json.Number) (string, error) {
	if n == nil {
		return "", fmt.Errorf("%q is missing", field)
	}
	f, err := strconv.ParseFloat(n.String(), 32)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", fmt.Errorf("%q %s is not a finite float32", field, n)
	}
	return n.String(), nil
}

func parseNumber(name string, msg json.RawMessage) (Number, error) {
	var rn rawNumber
	if err := decodeStrict(msg, &rn); err != nil {
		return Number{}, err
	}
	v, err := parseFloat("value", rn.Value)
	if err != nil {
		return Number{}, err
	}
	return finishNumber(name, v, rn)
}

func parseWeight(name string, msg json.RawMessage) (Number, error) {
	var rn rawNumber
	if err := decodeStrict(msg, &rn); err != nil {
		return Number{}, err
	}
	if rn.Value == nil {
		return Number{}, errors.New(`"value" is missing`)
	}
	x, err := strconv.ParseInt(rn.Value.String(), 10, 64)
	if err != nil || x < 1 || x > 1000 {
		return Number{}, fmt.Errorf(`"value" %s is not an integer weight in 1..1000`, rn.Value)
	}
	return finishNumber(name, strconv.FormatInt(x, 10), rn)
}

func finishNumber(name, value string, rn rawNumber) (Number, error) {
	src, err := parseSource(rn.Source)
	if err != nil {
		return Number{}, err
	}
	if err := checkLine(rn.Note); err != nil {
		return Number{}, fmt.Errorf("note: %w", err)
	}
	return Number{Name: name, Value: value, Source: src, Note: rn.Note}, nil
}

func parseSizes(name string, msg json.RawMessage) (Sizes, error) {
	var rs rawSizes
	if err := decodeStrict(msg, &rs); err != nil {
		return Sizes{}, err
	}
	sz := Sizes{Name: name, Note: rs.Note}
	for i, n := range []*json.Number{rs.Regular, rs.Small, rs.Mini, rs.Large} {
		v, err := parseFloat(SizeNames[i], n)
		if err != nil {
			return Sizes{}, err
		}
		sz.Values[i] = v
	}
	var err error
	if sz.Source, err = parseSource(rs.Source); err != nil {
		return Sizes{}, err
	}
	if err := checkLine(rs.Note); err != nil {
		return Sizes{}, fmt.Errorf("note: %w", err)
	}
	return sz, nil
}

// maxDurationMS is the longest duration a spec may give.
const maxDurationMS = 60000

func parseDuration(name string, msg json.RawMessage) (Number, error) {
	var rd rawDuration
	if err := decodeStrict(msg, &rd); err != nil {
		return Number{}, err
	}
	if rd.MS == nil {
		return Number{}, errors.New(`"ms" is missing`)
	}
	x, err := strconv.ParseInt(rd.MS.String(), 10, 64)
	if err != nil || x < 0 || x > maxDurationMS {
		return Number{}, fmt.Errorf(`"ms" %s is not an integer in 0..%d`, rd.MS, maxDurationMS)
	}
	return finishNumber(name, strconv.FormatInt(x, 10), rawNumber{Source: rd.Source, Note: rd.Note})
}

func parseCurve(name string, msg json.RawMessage) (Tuple, error) {
	var rc rawCurve
	if err := decodeStrict(msg, &rc); err != nil {
		return Tuple{}, err
	}
	if rc.Value == nil {
		return Tuple{}, errors.New(`"value" is missing`)
	}
	if len(rc.Value) != len(CurveNames) {
		return Tuple{}, fmt.Errorf(`"value" has %d numbers, want x1, y1, x2, y2`, len(rc.Value))
	}
	t := Tuple{Name: name}
	for i := range rc.Value {
		v, err := parseFloat(CurveNames[i], &rc.Value[i])
		if err != nil {
			return Tuple{}, err
		}
		if f, _ := strconv.ParseFloat(v, 64); i%2 == 0 && (f < 0 || f > 1) {
			return Tuple{}, fmt.Errorf("%s %s is not in 0..1", CurveNames[i], v)
		}
		t.Values = append(t.Values, v)
	}
	return finishTuple(t, rc.Source, rc.Note)
}

func parseSpring(name string, msg json.RawMessage) (Tuple, error) {
	var rs rawSpring
	if err := decodeStrict(msg, &rs); err != nil {
		return Tuple{}, err
	}
	t := Tuple{Name: name}
	for i, n := range []*json.Number{rs.Mass, rs.Stiffness, rs.Damping} {
		v, err := parseFloat(SpringNames[i], n)
		if err != nil {
			return Tuple{}, err
		}
		if f, _ := strconv.ParseFloat(v, 64); f <= 0 {
			return Tuple{}, fmt.Errorf("%q %s is not positive", SpringNames[i], v)
		}
		t.Values = append(t.Values, v)
	}
	return finishTuple(t, rs.Source, rs.Note)
}

func finishTuple(t Tuple, src *string, note string) (Tuple, error) {
	var err error
	if t.Source, err = parseSource(src); err != nil {
		return Tuple{}, err
	}
	if err := checkLine(note); err != nil {
		return Tuple{}, fmt.Errorf("note: %w", err)
	}
	t.Note = note
	return t, nil
}

func parseTracking(name string, msg json.RawMessage) (Tracking, error) {
	var rt rawTracking
	if err := decodeStrict(msg, &rt); err != nil {
		return Tracking{}, err
	}
	if len(rt.Steps) == 0 {
		return Tracking{}, errors.New(`"steps" is missing or empty`)
	}
	t := Tracking{Name: name, Note: rt.Note}
	last := 0.0
	for i, st := range rt.Steps {
		if len(st) != 2 {
			return Tracking{}, fmt.Errorf("step %d has %d values, want a font size and a tracking", i, len(st))
		}
		size, err := parseFloat("size", &st[0])
		if err != nil {
			return Tracking{}, fmt.Errorf("step %d: %w", i, err)
		}
		tr, err := parseFloat("tracking", &st[1])
		if err != nil {
			return Tracking{}, fmt.Errorf("step %d: %w", i, err)
		}
		f, _ := strconv.ParseFloat(size, 64)
		if f <= last {
			return Tracking{}, fmt.Errorf("step %d: font size %s is not positive and above the previous one", i, size)
		}
		last = f
		t.Steps = append(t.Steps, [2]string{size, tr})
	}
	var err error
	if t.Source, err = parseSource(rt.Source); err != nil {
		return Tracking{}, err
	}
	if err := checkLine(rt.Note); err != nil {
		return Tracking{}, fmt.Errorf("note: %w", err)
	}
	return t, nil
}

func parseSource(s *string) (Source, error) {
	if s == nil {
		return "", errors.New(`"source" is missing`)
	}
	switch src := Source(*s); src {
	case SourceMemory, SourceMeasured:
		return src, nil
	}
	return "", fmt.Errorf(`bad "source" %q (want "memory" or "measured")`, *s)
}

func parseColor(name string, msg json.RawMessage) (Color, error) {
	var rc rawColor
	if err := decodeStrict(msg, &rc); err != nil {
		return Color{}, err
	}
	c := Color{Name: name, Note: rc.Note}
	var err error
	if c.Light, err = parseRGBA("light", rc.Light); err != nil {
		return Color{}, err
	}
	if c.Dark, err = parseRGBA("dark", rc.Dark); err != nil {
		return Color{}, err
	}
	if (rc.ContrastLight == nil) != (rc.ContrastDark == nil) {
		return Color{}, errors.New(`"contrastLight" and "contrastDark" must come together`)
	}
	if rc.ContrastLight != nil {
		c.HasContrast = true
		if c.ContrastLight, err = parseRGBA("contrastLight", rc.ContrastLight); err != nil {
			return Color{}, err
		}
		if c.ContrastDark, err = parseRGBA("contrastDark", rc.ContrastDark); err != nil {
			return Color{}, err
		}
	}
	if c.Source, err = parseSource(rc.Source); err != nil {
		return Color{}, err
	}
	if err := checkLine(rc.Note); err != nil {
		return Color{}, fmt.Errorf("note: %w", err)
	}
	return c, nil
}

// parseRGBA parses [r, g, b] or [r, g, b, a] with integer channels in
// 0..255; a missing alpha is 255.
func parseRGBA(field string, v []json.Number) (RGBA8, error) {
	if v == nil {
		return RGBA8{}, fmt.Errorf("%q is missing", field)
	}
	if len(v) != 3 && len(v) != 4 {
		return RGBA8{}, fmt.Errorf("%q has %d channels, want 3 or 4", field, len(v))
	}
	ch := [4]uint8{255, 255, 255, 255}
	for i, n := range v {
		x, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil || x < 0 || x > 255 {
			return RGBA8{}, fmt.Errorf("%q channel %d is %s, want an integer in 0..255", field, i, n)
		}
		ch[i] = uint8(x)
	}
	return RGBA8{ch[0], ch[1], ch[2], ch[3]}, nil
}

func decodeStrict(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.More() {
		return errors.New("trailing data after the JSON value")
	}
	return nil
}

func checkLine(s string) error {
	for _, r := range s {
		if r < ' ' || r == 0x7f {
			return fmt.Errorf("control character %U (one line of text only)", r)
		}
	}
	return nil
}

// Generate returns the Go source of the generated tokens file (package
// theme, gofmt-formatted) and the Markdown listing for s. specPath is the
// spec's path as shown in both headers, with forward slashes.
func Generate(s *Spec, specPath string) (goSrc, doc []byte, err error) {
	goSrc, err = generateGo(s, specPath)
	if err != nil {
		return nil, nil, err
	}
	return goSrc, generateDoc(s, specPath), nil
}

func generateGo(s *Spec, specPath string) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by gentokens from %s; DO NOT EDIT.\n\n", specPath)
	b.WriteString("package theme\n\n")
	if s.hasDurations() {
		b.WriteString("import (\n\"time\"\n\n\"github.com/radix29/gogui/internal/driver\"\n)\n\n")
	} else {
		b.WriteString("import \"github.com/radix29/gogui/internal/driver\"\n\n")
	}
	fmt.Fprintf(&b, "// %s is the token set of %s.", s.Name, specPath)
	if s.Comment != "" {
		fmt.Fprintf(&b, " %s", s.Comment)
	}
	fmt.Fprintf(&b, "\n// %d of its %d tokens are provisional (from memory, to be measured).\n",
		s.Provisional(), s.Tokens())
	fmt.Fprintf(&b, "var %s = Theme{\n", s.Name)
	fmt.Fprintf(&b, "Name: %q,\n", s.Name)
	b.WriteString("Colors: Colors{\n")
	for _, c := range s.Colors {
		goToken(&b, "driver."+c.Name+": {", c)
	}
	b.WriteString("},\n")
	for _, g := range s.Groups {
		fmt.Fprintf(&b, "%s: %s{\n", g.Group.Field, g.Group.Field)
		for _, c := range g.Colors {
			goToken(&b, c.Name+": ColorToken{", c)
		}
		for _, v := range g.Numbers {
			fmt.Fprintf(&b, "%s: %s,", v.Name, v.Value)
			provisionalMark(&b, v.Source)
		}
		for _, v := range g.Sizes {
			fmt.Fprintf(&b, "%s: SizeTable{%s},", v.Name, strings.Join(v.Values[:], ", "))
			provisionalMark(&b, v.Source)
		}
		for _, v := range g.Weights {
			fmt.Fprintf(&b, "%s: %s,", v.Name, v.Value)
			provisionalMark(&b, v.Source)
		}
		for _, v := range g.Durations {
			fmt.Fprintf(&b, "%s: %s * time.Millisecond,", v.Name, v.Value)
			provisionalMark(&b, v.Source)
		}
		for _, v := range g.Curves {
			fmt.Fprintf(&b, "%s: Curve{%s},", v.Name, strings.Join(v.Values, ", "))
			provisionalMark(&b, v.Source)
		}
		for _, v := range g.Springs {
			fmt.Fprintf(&b, "%s: Spring{Mass: %s, Stiffness: %s, Damping: %s},", v.Name, v.Values[0], v.Values[1], v.Values[2])
			provisionalMark(&b, v.Source)
		}
		if t := g.Tracking; t != nil {
			fmt.Fprintf(&b, "%s: []TrackingStep{ // ", t.Name)
			if t.Source == SourceMemory {
				b.WriteString("provisional; ")
			}
			b.WriteString("font size (pt), tracking (1/1000 em)\n")
			for _, st := range t.Steps {
				fmt.Fprintf(&b, "{%s, %s},\n", st[0], st[1])
			}
			b.WriteString("},\n")
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gen: formatting the generated code: %w", err)
	}
	return out, nil
}

func (s *Spec) hasDurations() bool {
	for _, g := range s.Groups {
		if len(g.Durations) > 0 {
			return true
		}
	}
	return false
}

// goToken writes one keyed ColorToken element; open is its key and opening
// brace.
func goToken(b *bytes.Buffer, open string, c Color) {
	b.WriteString(open)
	fmt.Fprintf(b, "Light: %s, Dark: %s", goColor(c.Light), goColor(c.Dark))
	if c.HasContrast {
		fmt.Fprintf(b, ", HasContrast: true, ContrastLight: %s, ContrastDark: %s",
			goColor(c.ContrastLight), goColor(c.ContrastDark))
	}
	b.WriteString("},")
	provisionalMark(b, c.Source)
}

func provisionalMark(b *bytes.Buffer, src Source) {
	if src == SourceMemory {
		b.WriteString(" // provisional")
	}
	b.WriteString("\n")
}

func goColor(c RGBA8) string {
	return fmt.Sprintf("rgba8(%d, %d, %d, %d)", c.R, c.G, c.B, c.A)
}

func generateDoc(s *Spec, specPath string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "<!-- Code generated by gentokens from %s; DO NOT EDIT. -->\n\n", specPath)
	fmt.Fprintf(&b, "# Theme tokens: %s\n\n", s.Name)
	fmt.Fprintf(&b, "Generated from [`%s`](../internal/aqua/theme/%s) by `go generate ./internal/aqua/theme` (PLAN.md §5.2, S3.1).", specPath, specPath)
	if s.Comment != "" {
		fmt.Fprintf(&b, " %s", s.Comment)
	}
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "**%d of %d tokens are provisional** (source `memory`: from memory, to be measured against the macOS references in S3.1t). ", s.Provisional(), s.Tokens())
	b.WriteString("Colors are sRGB with straight alpha, as `#rrggbb` or `#rrggbbaa`; lengths are in logical points. ")
	b.WriteString("A row per control size, a curve, a spring and the tracking table count as one token each.\n\n")
	b.WriteString("## Colors\n\n")
	mdColors(&b, s.Colors)
	for _, g := range s.Groups {
		fmt.Fprintf(&b, "\n## %s\n", g.Group.Title)
		if len(g.Colors) > 0 {
			b.WriteString("\n")
			mdColors(&b, g.Colors)
		}
		if nums := append(append([]Number(nil), g.Numbers...), g.Weights...); len(nums) > 0 {
			b.WriteString("\n| Number | Value | Source | Note |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, v := range nums {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", v.Name, v.Value, mdSource(v.Source), mdEscape(v.Note))
			}
		}
		if len(g.Sizes) > 0 {
			b.WriteString("\n| Per control size | Regular | Small | Mini | Large | Source | Note |\n")
			b.WriteString("|---|---|---|---|---|---|---|\n")
			for _, v := range g.Sizes {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s |\n", v.Name,
					v.Values[0], v.Values[1], v.Values[2], v.Values[3], mdSource(v.Source), mdEscape(v.Note))
			}
		}
		if len(g.Durations) > 0 {
			b.WriteString("\n| Duration | ms | Source | Note |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, v := range g.Durations {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", v.Name, v.Value, mdSource(v.Source), mdEscape(v.Note))
			}
		}
		if len(g.Curves) > 0 {
			b.WriteString("\n| Curve | x1 | y1 | x2 | y2 | Source | Note |\n")
			b.WriteString("|---|---|---|---|---|---|---|\n")
			for _, v := range g.Curves {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", v.Name, strings.Join(v.Values, " | "), mdSource(v.Source), mdEscape(v.Note))
			}
		}
		if len(g.Springs) > 0 {
			b.WriteString("\n| Spring | Mass | Stiffness | Damping | Source | Note |\n")
			b.WriteString("|---|---|---|---|---|---|\n")
			for _, v := range g.Springs {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", v.Name, strings.Join(v.Values, " | "), mdSource(v.Source), mdEscape(v.Note))
			}
		}
		if t := g.Tracking; t != nil {
			fmt.Fprintf(&b, "\n`%s`, %s", t.Name, mdSource(t.Source))
			if t.Note != "" {
				fmt.Fprintf(&b, ": %s", mdEscape(t.Note))
			}
			b.WriteString("\n\n| Font size (pt) | Tracking (1/1000 em) |\n|---|---|\n")
			for _, st := range t.Steps {
				fmt.Fprintf(&b, "| %s | %s |\n", st[0], st[1])
			}
		}
	}
	return b.Bytes()
}

func mdColors(b *bytes.Buffer, colors []Color) {
	b.WriteString("| Color | Light | Dark | Contrast light | Contrast dark | Source | Note |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, c := range colors {
		cl, cd := "", ""
		if c.HasContrast {
			cl, cd = mdColor(c.ContrastLight), mdColor(c.ContrastDark)
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s | %s |\n",
			c.Name, mdColor(c.Light), mdColor(c.Dark), cl, cd, mdSource(c.Source), mdEscape(c.Note))
	}
}

func mdSource(src Source) string {
	if src == SourceMemory {
		return "**memory** (to be measured)"
	}
	return "measured"
}

func mdColor(c RGBA8) string {
	if c.A == 255 {
		return fmt.Sprintf("`#%02x%02x%02x`", c.R, c.G, c.B)
	}
	return fmt.Sprintf("`#%02x%02x%02x%02x`", c.R, c.G, c.B, c.A)
}

func mdEscape(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
