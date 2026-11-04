package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

// TestColorNamesMatchDriver checks ColorNames against the SemanticColor
// constants of internal/driver/color.go, in declaration order.
func TestColorNamesMatchDriver(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../../../driver/color.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			for _, n := range sp.(*ast.ValueSpec).Names {
				names = append(names, n.Name)
			}
		}
	}
	start, end := -1, -1
	for i, n := range names {
		switch n {
		case "SemanticNone":
			start = i + 1
		case "semanticColorCount":
			end = i
		}
	}
	if start < 0 || end < start {
		t.Fatalf("no SemanticNone … semanticColorCount block in color.go: %v", names)
	}
	got := names[start:end]
	if strings.Join(got, ",") != strings.Join(ColorNames, ",") {
		t.Errorf("driver colors\n%v\nColorNames\n%v", got, ColorNames)
	}
}

// The default tokens of spec.
const (
	okColor    = `{"light": [1, 2, 3], "dark": [4, 5, 6, 7], "source": "memory"}`
	okNumber   = `{"value": 0.5, "source": "memory"}`
	okSizes    = `{"regular": 1, "small": 2, "mini": 3, "large": 4.5, "source": "memory"}`
	okWeight   = `{"value": 600, "source": "memory"}`
	okTracking = `{"steps": [[9, 19], [13, -6]], "source": "memory"}`
	okDuration = `{"ms": 250, "source": "memory"}`
	okCurve    = `{"value": [0.42, 0, 0.58, 1], "source": "memory"}`
	okSpring   = `{"mass": 1, "stiffness": 300, "damping": 30, "source": "memory"}`
)

// spec builds a spec: every semantic color and every group color gets def,
// and the other tokens the ok defaults, unless entries overrides them; an
// empty override drops the entry. Keys name semantic colors, or
// "<group key>.<kind>:<field>" a group token (kind colors, numbers, sizes,
// weights, durations, curves or springs; e.g. "chrome.numbers:TitleGap"), or "<group key>.tracking" a
// tracking table; unknown names are added. extra is appended inside the
// top-level object.
func spec(def string, entries map[string]string, extra string) []byte {
	var b strings.Builder
	section := func(prefix string, names []string, def string) {
		first := true
		add := func(name, body string) {
			if !first {
				b.WriteString(",")
			}
			first = false
			b.WriteString(`"` + name + `": ` + body)
		}
		for _, n := range names {
			body, ok := entries[prefix+n]
			if !ok {
				body = def
			}
			if body != "" {
				add(n, body)
			}
		}
		for k, body := range entries {
			n, ok := strings.CutPrefix(k, prefix)
			if !ok || (prefix == "" && strings.ContainsAny(k, ":.")) || contains(names, n) {
				continue
			}
			add(n, body)
		}
	}
	b.WriteString(`{"name": "Test", "colors": {`)
	section("", ColorNames, def)
	b.WriteString("}")
	for _, g := range Groups {
		fmt.Fprintf(&b, `, %q: {"colors": {`, g.Key)
		section(g.Key+".colors:", g.Colors, def)
		b.WriteString(`}, "numbers": {`)
		section(g.Key+".numbers:", g.Numbers, okNumber)
		b.WriteString(`}, "sizes": {`)
		section(g.Key+".sizes:", g.Sizes, okSizes)
		b.WriteString(`}, "weights": {`)
		section(g.Key+".weights:", g.Weights, okWeight)
		b.WriteString(`}, "durations": {`)
		section(g.Key+".durations:", g.Durations, okDuration)
		b.WriteString(`}, "curves": {`)
		section(g.Key+".curves:", g.Curves, okCurve)
		b.WriteString(`}, "springs": {`)
		section(g.Key+".springs:", g.Springs, okSpring)
		b.WriteString("}")
		tr, ok := entries[g.Key+".tracking"]
		if !ok && g.Tracking != "" {
			tr = okTracking
		}
		if tr != "" {
			b.WriteString(`, "tracking": ` + tr)
		}
		b.WriteString("}")
	}
	b.WriteString(extra + "}")
	return []byte(b.String())
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// allTokens is the number of tokens of a complete spec.
func allTokens() int {
	n := len(ColorNames)
	for _, g := range Groups {
		n += len(g.Colors) + len(g.Numbers) + len(g.Sizes) + len(g.Weights) + len(g.Durations) + len(g.Curves) + len(g.Springs)
		if g.Tracking != "" {
			n++
		}
	}
	return n
}

func TestParseValid(t *testing.T) {
	s, err := Parse(spec(okColor, map[string]string{
		"ColorLabel": `{"light": [0, 0, 0, 255], "dark": [255, 255, 255, 0], "contrastLight": [9, 9, 9], "contrastDark": [8, 8, 8, 8], "source": "measured", "note": "a | b"}`,
	}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Colors) != len(ColorNames) {
		t.Fatalf("%d colors, want %d", len(s.Colors), len(ColorNames))
	}
	for i, c := range s.Colors {
		if c.Name != ColorNames[i] {
			t.Errorf("color %d is %s, want %s", i, c.Name, ColorNames[i])
		}
	}
	l := s.Colors[0]
	want := Color{
		Name: "ColorLabel", Light: RGBA8{0, 0, 0, 255}, Dark: RGBA8{255, 255, 255, 0},
		HasContrast: true, ContrastLight: RGBA8{9, 9, 9, 255}, ContrastDark: RGBA8{8, 8, 8, 8},
		Source: SourceMeasured, Note: "a | b",
	}
	if l != want {
		t.Errorf("ColorLabel = %+v, want %+v", l, want)
	}
	if c := s.Colors[1]; c.Light != (RGBA8{1, 2, 3, 255}) || c.Dark != (RGBA8{4, 5, 6, 7}) || c.HasContrast {
		t.Errorf("ColorSecondaryLabel = %+v", c)
	}
	if len(s.Groups) != len(Groups) {
		t.Fatalf("%d groups, want %d", len(s.Groups), len(Groups))
	}
	for i, gv := range s.Groups {
		g := &Groups[i]
		if gv.Group != g || len(gv.Colors) != len(g.Colors) || len(gv.Numbers) != len(g.Numbers) ||
			len(gv.Sizes) != len(g.Sizes) || len(gv.Weights) != len(g.Weights) || len(gv.Durations) != len(g.Durations) ||
			len(gv.Curves) != len(g.Curves) || len(gv.Springs) != len(g.Springs) || (gv.Tracking != nil) != (g.Tracking != "") {
			t.Fatalf("group %s = %+v", g.Field, gv)
		}
		for j, c := range gv.Colors {
			if c.Name != g.Colors[j] || c.Light != (RGBA8{1, 2, 3, 255}) {
				t.Errorf("%s color %d = %+v", g.Field, j, c)
			}
		}
		for j, v := range gv.Numbers {
			if v != (Number{Name: g.Numbers[j], Value: "0.5", Source: SourceMemory}) {
				t.Errorf("%s number %d = %+v", g.Field, j, v)
			}
		}
		for j, v := range gv.Sizes {
			if v != (Sizes{Name: g.Sizes[j], Values: [4]string{"1", "2", "3", "4.5"}, Source: SourceMemory}) {
				t.Errorf("%s size %d = %+v", g.Field, j, v)
			}
		}
		for j, v := range gv.Weights {
			if v != (Number{Name: g.Weights[j], Value: "600", Source: SourceMemory}) {
				t.Errorf("%s weight %d = %+v", g.Field, j, v)
			}
		}
		for j, v := range gv.Durations {
			if v != (Number{Name: g.Durations[j], Value: "250", Source: SourceMemory}) {
				t.Errorf("%s duration %d = %+v", g.Field, j, v)
			}
		}
		for j, v := range gv.Curves {
			if v.Name != g.Curves[j] || strings.Join(v.Values, ",") != "0.42,0,0.58,1" || v.Source != SourceMemory {
				t.Errorf("%s curve %d = %+v", g.Field, j, v)
			}
		}
		for j, v := range gv.Springs {
			if v.Name != g.Springs[j] || strings.Join(v.Values, ",") != "1,300,30" || v.Source != SourceMemory {
				t.Errorf("%s spring %d = %+v", g.Field, j, v)
			}
		}
		if tr := gv.Tracking; tr != nil {
			if tr.Name != g.Tracking || len(tr.Steps) != 2 || tr.Steps[0] != [2]string{"9", "19"} || tr.Steps[1] != [2]string{"13", "-6"} {
				t.Errorf("%s tracking = %+v", g.Field, tr)
			}
		}
	}
	if s.Group("Typography") != &s.Groups[2] || s.Group("Bogus") != nil {
		t.Error("Spec.Group")
	}
	if n, all := s.Provisional(), s.Tokens(); all != allTokens() || n != all-1 {
		t.Errorf("Provisional = %d of %d, want %d of %d", n, all, allTokens()-1, allTokens())
	}
	if _, _, err := Generate(s, "spec/test.json"); err != nil {
		t.Errorf("Generate: %v", err)
	}
}

func TestParseRejects(t *testing.T) {
	type m = map[string]string
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"missing color", spec(okColor, m{"ColorGrid": ""}, ""), "ColorGrid is missing"},
		{"unknown color", spec(okColor, m{"ColorBogus": okColor}, ""), `unknown color "ColorBogus"`},
		{"missing light", spec(okColor, m{"ColorLink": `{"dark": [1, 2, 3], "source": "memory"}`}, ""), `ColorLink: "light" is missing`},
		{"missing dark", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "source": "memory"}`}, ""), `ColorLink: "dark" is missing`},
		{"contrast without pair", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3], "contrastLight": [1, 2, 3], "source": "memory"}`}, ""), "must come together"},
		{"channel above 255", spec(okColor, m{"ColorLink": `{"light": [1, 256, 3], "dark": [1, 2, 3], "source": "memory"}`}, ""), `"light" channel 1 is 256`},
		{"negative channel", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3, -1], "source": "memory"}`}, ""), `"dark" channel 3 is -1`},
		{"fractional channel", spec(okColor, m{"ColorLink": `{"light": [1, 2.5, 3], "dark": [1, 2, 3], "source": "memory"}`}, ""), `"light" channel 1 is 2.5`},
		{"contrast channel", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3], "contrastLight": [1, 2, 3], "contrastDark": [300, 2, 3], "source": "memory"}`}, ""), `"contrastDark" channel 0 is 300`},
		{"too few channels", spec(okColor, m{"ColorLink": `{"light": [1, 2], "dark": [1, 2, 3], "source": "memory"}`}, ""), "2 channels"},
		{"too many channels", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3, 4, 5], "dark": [1, 2, 3], "source": "memory"}`}, ""), "5 channels"},
		{"bad source", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3], "source": "guess"}`}, ""), `bad "source" "guess"`},
		{"empty source", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3], "source": ""}`}, ""), `bad "source" ""`},
		{"missing source", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3]}`}, ""), `"source" is missing`},
		{"unknown color field", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3], "source": "memory", "lite": [1, 2, 3]}`}, ""), `unknown field "lite"`},
		{"unknown spec field", spec(okColor, nil, `, "motion": {}`), `unknown field "motion"`},
		{"multi-line note", spec(okColor, m{"ColorLink": `{"light": [1, 2, 3], "dark": [1, 2, 3], "source": "memory", "note": "a\nb"}`}, ""), "one line"},

		{"missing chrome color", spec(okColor, m{"chrome.colors:ZoomGlyph": ""}, ""), "chrome color ZoomGlyph is missing"},
		{"unknown chrome color", spec(okColor, m{"chrome.colors:Bogus": okColor}, ""), `unknown chrome color "Bogus"`},
		{"bad chrome color", spec(okColor, m{"chrome.colors:TitleBar": `{"light": [1, 2, 3], "source": "memory"}`}, ""), `chrome color TitleBar: "dark" is missing`},
		{"missing chrome number", spec(okColor, m{"chrome.numbers:TitleGap": ""}, ""), "chrome number TitleGap is missing"},
		{"unknown chrome number", spec(okColor, m{"chrome.numbers:Bogus": okNumber}, ""), `unknown chrome number "Bogus"`},
		{"missing value", spec(okColor, m{"chrome.numbers:LightPressedShade": `{"source": "memory"}`}, ""), `"value" is missing`},
		{"value out of range", spec(okColor, m{"chrome.numbers:LightPressedShade": `{"value": 1e50, "source": "memory"}`}, ""), "not a finite float32"},
		{"number source", spec(okColor, m{"chrome.numbers:LightPressedShade": `{"value": 1, "source": "guess"}`}, ""), `bad "source" "guess"`},
		{"number note", spec(okColor, m{"chrome.numbers:LightPressedShade": `{"value": 1, "source": "memory", "note": "a\nb"}`}, ""), "one line"},
		{"unknown number field", spec(okColor, m{"chrome.numbers:LightPressedShade": `{"value": 1, "source": "memory", "unit": "x"}`}, ""), `unknown field "unit"`},
		{"sizes in chrome", spec(okColor, m{"chrome.sizes:Bogus": okSizes}, ""), `unknown chrome size "Bogus"`},

		{"missing metrics color", spec(okColor, m{"metrics.colors:BezelShadow": ""}, ""), "metrics color BezelShadow is missing"},
		{"missing metrics number", spec(okColor, m{"metrics.numbers:FocusRingWidth": ""}, ""), "metrics number FocusRingWidth is missing"},
		{"missing size row", spec(okColor, m{"metrics.sizes:ControlHeight": ""}, ""), "metrics size ControlHeight is missing"},
		{"unknown size row", spec(okColor, m{"metrics.sizes:Bogus": okSizes}, ""), `unknown metrics size "Bogus"`},
		{"missing control size", spec(okColor, m{"metrics.sizes:ControlHeight": `{"regular": 1, "small": 2, "large": 4, "source": "memory"}`}, ""), `ControlHeight: "mini" is missing`},
		{"unknown control size", spec(okColor, m{"metrics.sizes:ControlHeight": `{"regular": 1, "small": 2, "mini": 3, "large": 4, "huge": 5, "source": "memory"}`}, ""), `unknown field "huge"`},
		{"size out of range", spec(okColor, m{"metrics.sizes:ControlHeight": `{"regular": 1e40, "small": 2, "mini": 3, "large": 4, "source": "memory"}`}, ""), `"regular" 1e40 is not a finite float32`},
		{"size source", spec(okColor, m{"metrics.sizes:ControlHeight": `{"regular": 1, "small": 2, "mini": 3, "large": 4}`}, ""), `"source" is missing`},
		{"size note", spec(okColor, m{"metrics.sizes:ControlHeight": `{"regular": 1, "small": 2, "mini": 3, "large": 4, "source": "memory", "note": "a\tb"}`}, ""), "one line"},

		{"missing weight", spec(okColor, m{"typography.weights:EmphasizedWeight": ""}, ""), "typography weight EmphasizedWeight is missing"},
		{"fractional weight", spec(okColor, m{"typography.weights:EmphasizedWeight": `{"value": 600.5, "source": "memory"}`}, ""), "not an integer weight"},
		{"weight zero", spec(okColor, m{"typography.weights:EmphasizedWeight": `{"value": 0, "source": "memory"}`}, ""), "not an integer weight"},
		{"weight above 1000", spec(okColor, m{"typography.weights:EmphasizedWeight": `{"value": 1001, "source": "memory"}`}, ""), "not an integer weight"},
		{"missing weight value", spec(okColor, m{"typography.weights:EmphasizedWeight": `{"source": "memory"}`}, ""), `"value" is missing`},
		{"missing tracking", spec(okColor, m{"typography.tracking": ""}, ""), "typography tracking is missing"},
		{"tracking in chrome", spec(okColor, m{"chrome.tracking": okTracking}, ""), "chrome has no tracking table"},
		{"empty tracking", spec(okColor, m{"typography.tracking": `{"steps": [], "source": "memory"}`}, ""), `"steps" is missing or empty`},
		{"tracking step length", spec(okColor, m{"typography.tracking": `{"steps": [[9, 1, 2]], "source": "memory"}`}, ""), "step 0 has 3 values"},
		{"tracking not ascending", spec(okColor, m{"typography.tracking": `{"steps": [[9, 1], [9, 2]], "source": "memory"}`}, ""), "step 1: font size 9 is not positive"},
		{"tracking size zero", spec(okColor, m{"typography.tracking": `{"steps": [[0, 1]], "source": "memory"}`}, ""), "step 0: font size 0 is not positive"},
		{"tracking value", spec(okColor, m{"typography.tracking": `{"steps": [[9, 1e50]], "source": "memory"}`}, ""), `step 0: "tracking" 1e50 is not a finite float32`},
		{"tracking source", spec(okColor, m{"typography.tracking": `{"steps": [[9, 1]], "source": "guess"}`}, ""), `bad "source" "guess"`},
		{"tracking field", spec(okColor, m{"typography.tracking": `{"steps": [[9, 1]], "source": "memory", "unit": "em"}`}, ""), `unknown field "unit"`},

		{"missing accent color", spec(okColor, m{"accent.colors:Base": ""}, ""), "accent color Base is missing"},
		{"missing accent number", spec(okColor, m{"accent.numbers:SelectionAlpha": ""}, ""), "accent number SelectionAlpha is missing"},
		{"missing duration", spec(okColor, m{"animation.durations:DurationNormal": ""}, ""), "animation duration DurationNormal is missing"},
		{"unknown duration", spec(okColor, m{"animation.durations:Bogus": okDuration}, ""), `unknown animation duration "Bogus"`},
		{"durations in chrome", spec(okColor, m{"chrome.durations:Bogus": okDuration}, ""), `unknown chrome duration "Bogus"`},
		{"missing ms", spec(okColor, m{"animation.durations:DurationNormal": `{"source": "memory"}`}, ""), `"ms" is missing`},
		{"fractional ms", spec(okColor, m{"animation.durations:DurationNormal": `{"ms": 2.5, "source": "memory"}`}, ""), "not an integer in 0..60000"},
		{"negative ms", spec(okColor, m{"animation.durations:DurationNormal": `{"ms": -1, "source": "memory"}`}, ""), "not an integer in 0..60000"},
		{"ms too large", spec(okColor, m{"animation.durations:DurationNormal": `{"ms": 60001, "source": "memory"}`}, ""), "not an integer in 0..60000"},
		{"duration source", spec(okColor, m{"animation.durations:DurationNormal": `{"ms": 1}`}, ""), `"source" is missing`},
		{"duration field", spec(okColor, m{"animation.durations:DurationNormal": `{"ms": 1, "s": 1, "source": "memory"}`}, ""), `unknown field "s"`},
		{"missing curve", spec(okColor, m{"animation.curves:EaseIn": ""}, ""), "animation curve EaseIn is missing"},
		{"missing curve value", spec(okColor, m{"animation.curves:EaseIn": `{"source": "memory"}`}, ""), `"value" is missing`},
		{"short curve", spec(okColor, m{"animation.curves:EaseIn": `{"value": [0.42, 0, 1], "source": "memory"}`}, ""), "has 3 numbers"},
		{"curve x1 out of range", spec(okColor, m{"animation.curves:EaseIn": `{"value": [1.5, 0, 1, 1], "source": "memory"}`}, ""), "x1 1.5 is not in 0..1"},
		{"curve x2 out of range", spec(okColor, m{"animation.curves:EaseIn": `{"value": [0, 0, -0.1, 1], "source": "memory"}`}, ""), "x2 -0.1 is not in 0..1"},
		{"curve y not finite", spec(okColor, m{"animation.curves:EaseIn": `{"value": [0, 1e50, 1, 1], "source": "memory"}`}, ""), "not a finite float32"},
		{"curve note", spec(okColor, m{"animation.curves:EaseIn": `{"value": [0, 0, 1, 1], "source": "memory", "note": "a\nb"}`}, ""), "one line"},
		{"missing spring", spec(okColor, m{"animation.springs:ControlSpring": ""}, ""), "animation spring ControlSpring is missing"},
		{"spring no damping", spec(okColor, m{"animation.springs:ControlSpring": `{"mass": 1, "stiffness": 300, "source": "memory"}`}, ""), `"damping" is missing`},
		{"spring zero mass", spec(okColor, m{"animation.springs:ControlSpring": `{"mass": 0, "stiffness": 300, "damping": 30, "source": "memory"}`}, ""), `"mass" 0 is not positive`},
		{"spring negative stiffness", spec(okColor, m{"animation.springs:ControlSpring": `{"mass": 1, "stiffness": -3, "damping": 30, "source": "memory"}`}, ""), `"stiffness" -3 is not positive`},
		{"spring source", spec(okColor, m{"animation.springs:ControlSpring": `{"mass": 1, "stiffness": 3, "damping": 3, "source": "guess"}`}, ""), `bad "source" "guess"`},
		{"spring field", spec(okColor, m{"animation.springs:ControlSpring": `{"mass": 1, "stiffness": 3, "damping": 3, "velocity": 0, "source": "memory"}`}, ""), `unknown field "velocity"`},

		{"no chrome", []byte(`{"name": "Test", "colors": {}}`), `no "chrome"`},
		{"no metrics", []byte(`{"name": "Test", "colors": {}, "chrome": {}}`), `no "metrics"`},
		{"no typography", []byte(`{"name": "Test", "colors": {}, "chrome": {}, "metrics": {}}`), `no "typography"`},
		{"no accent", []byte(`{"name": "Test", "colors": {}, "chrome": {}, "metrics": {}, "typography": {}}`), `no "accent"`},
		{"no animation", []byte(`{"name": "Test", "colors": {}, "chrome": {}, "metrics": {}, "typography": {}, "accent": {}}`), `no "animation"`},
		{"no colors", []byte(`{"name": "Test"}`), `no "colors"`},
		{"bad name", []byte(`{"name": "test", "colors": {}}`), "not an exported Go identifier"},
		{"trailing data", append(spec(okColor, nil, ""), "{}"...), "trailing data"},
		{"not JSON", []byte(`{`), "spec:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.data)
			if err == nil {
				t.Fatalf("Parse succeeded, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateMarksProvisional(t *testing.T) {
	const measured = `{"light": [1, 2, 3], "dark": [4, 5, 6], "source": "measured"}`
	entries := map[string]string{
		"ColorGrid":                           `{"light": [255, 0, 16, 128], "dark": [4, 5, 6], "source": "memory", "note": "x | y"}`,
		"chrome.colors:TitleBar":              `{"light": [1, 2, 3], "dark": [4, 5, 6], "contrastLight": [7, 8, 9], "contrastDark": [10, 11, 12], "source": "memory"}`,
		"chrome.numbers:LightPressedShade":    `{"value": 0.82, "source": "measured", "note": "n"}`,
		"metrics.sizes:ControlHeight":         `{"regular": 22, "small": 19, "mini": 16, "large": 28, "source": "memory", "note": "h"}`,
		"typography.weights:EmphasizedWeight": `{"value": 700, "source": "measured"}`,
		"typography.tracking":                 `{"steps": [[9, 19], [13, -6]], "source": "memory", "note": "t | u"}`,
		"animation.durations:DurationNormal":  `{"ms": 250, "source": "memory", "note": "d"}`,
		"animation.curves:EaseInOut":          `{"value": [0.42, 0, 0.58, 1], "source": "memory", "note": "c"}`,
		"animation.springs:ControlSpring":     `{"mass": 1, "stiffness": 300, "damping": 30, "source": "memory", "note": "s"}`,
	}
	// Everything else measured, so only the seven entries above are memory.
	for _, g := range Groups {
		for _, n := range g.Numbers {
			if _, ok := entries[g.Key+".numbers:"+n]; !ok {
				entries[g.Key+".numbers:"+n] = `{"value": 0.5, "source": "measured"}`
			}
		}
		for _, n := range g.Sizes {
			if _, ok := entries[g.Key+".sizes:"+n]; !ok {
				entries[g.Key+".sizes:"+n] = `{"regular": 1, "small": 2, "mini": 3, "large": 4, "source": "measured"}`
			}
		}
		for _, n := range g.Weights {
			if _, ok := entries[g.Key+".weights:"+n]; !ok {
				entries[g.Key+".weights:"+n] = `{"value": 600, "source": "measured"}`
			}
		}
		for _, n := range g.Durations {
			if _, ok := entries[g.Key+".durations:"+n]; !ok {
				entries[g.Key+".durations:"+n] = `{"ms": 100, "source": "measured"}`
			}
		}
		for _, n := range g.Curves {
			if _, ok := entries[g.Key+".curves:"+n]; !ok {
				entries[g.Key+".curves:"+n] = `{"value": [0, 0, 1, 1], "source": "measured"}`
			}
		}
		for _, n := range g.Springs {
			if _, ok := entries[g.Key+".springs:"+n]; !ok {
				entries[g.Key+".springs:"+n] = `{"mass": 2, "stiffness": 3, "damping": 4, "source": "measured"}`
			}
		}
	}
	s, err := Parse(spec(measured, entries, ""))
	if err != nil {
		t.Fatal(err)
	}
	goSrc, doc, err := Generate(s, "spec/test.json")
	if err != nil {
		t.Fatal(err)
	}
	// gofmt aligns the keyed elements; compare with single spaces.
	squeezed := regexp.MustCompile(` +`).ReplaceAllString(string(goSrc), " ")
	if n := strings.Count(string(goSrc), "// provisional"); n != 7 {
		t.Errorf("%d provisional marks in the Go file, want 7", n)
	}
	for _, want := range []string{
		fmt.Sprintf("7 of its %d tokens are provisional", allTokens()), "DO NOT EDIT", "driver.ColorGrid:",
		"import (\n\t\"time\"\n", "Accent: Accent{", "Animation: Animation{",
		"DurationNormal: 250 * time.Millisecond, // provisional\n",
		"DurationFast: 100 * time.Millisecond,\n",
		"EaseInOut: Curve{0.42, 0, 0.58, 1}, // provisional\n",
		"EaseIn: Curve{0, 0, 1, 1},\n",
		"ControlSpring: Spring{Mass: 1, Stiffness: 300, Damping: 30}, // provisional\n",
		"RubberBandSpring: Spring{Mass: 2, Stiffness: 3, Damping: 4},\n",
		"Chrome: Chrome{", "Metrics: Metrics{", "Typography: Typography{",
		"TitleBar: ColorToken{Light: rgba8(1, 2, 3, 255), Dark: rgba8(4, 5, 6, 255), HasContrast: true, ContrastLight: rgba8(7, 8, 9, 255), ContrastDark: rgba8(10, 11, 12, 255)}, // provisional",
		"LightPressedShade: 0.82,\n",
		"ControlHeight: SizeTable{22, 19, 16, 28}, // provisional\n",
		"ControlPaddingX: SizeTable{1, 2, 3, 4},\n",
		"EmphasizedWeight: 700,\n",
		"Tracking: []TrackingStep{ // provisional; font size (pt), tracking (1/1000 em)\n",
		"{13, -6},\n",
	} {
		if !strings.Contains(squeezed, want) {
			t.Errorf("Go file lacks %q", want)
		}
	}
	for _, want := range []string{
		fmt.Sprintf("**7 of %d tokens are provisional**", allTokens()),
		"## Window chrome", "## Control metrics", "## Typography", "## Accent rules", "## Animation",
		"| `DurationNormal` | 250 | **memory** (to be measured) | d |",
		"| `EaseInOut` | 0.42 | 0 | 0.58 | 1 | **memory** (to be measured) | c |",
		"| `ControlSpring` | 1 | 300 | 30 | **memory** (to be measured) | s |",
		"| `RubberBandSpring` | 2 | 3 | 4 | measured |  |",
		"| `TitleBar` | `#010203` | `#040506` | `#070809` | `#0a0b0c` | **memory** (to be measured) |  |",
		"| `LightPressedShade` | 0.82 | measured | n |",
		"| `ColorGrid` | `#ff001080` | `#040506` |  |  | **memory** (to be measured) | x \\| y |",
		"| `ColorLink` | `#010203` | `#040506` |  |  | measured |  |",
		"| `ControlHeight` | 22 | 19 | 16 | 28 | **memory** (to be measured) | h |",
		"| `EmphasizedWeight` | 700 | measured |  |",
		"`Tracking`, **memory** (to be measured): t \\| u\n",
		"| 13 | -6 |\n",
	} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("listing lacks %q", want)
		}
	}
}

// TestGroupsMatchTheme checks Groups against ../theme.go: the Theme fields
// after Name and Colors are the groups in order, each of a struct type of
// the same name, whose fields are the group's names by kind, in
// declaration order.
func TestGroupsMatchTheme(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../theme.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	structs := map[string]*ast.StructType{}
	ast.Inspect(f, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSpec); ok {
			if st, ok := ts.Type.(*ast.StructType); ok {
				structs[ts.Name.Name] = st
			}
		}
		return true
	})
	th := structs["Theme"]
	if th == nil {
		t.Fatal("no type Theme in theme.go")
	}
	var fields []string
	for _, fld := range th.Fields.List {
		for _, n := range fld.Names {
			fields = append(fields, n.Name)
		}
	}
	var want []string
	want = append(want, "Name", "Colors")
	for _, g := range Groups {
		want = append(want, g.Field)
	}
	if strings.Join(fields, ",") != strings.Join(want, ",") {
		t.Errorf("Theme fields %v, want %v", fields, want)
	}
	typeName := func(e ast.Expr) string {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			return x.X.(*ast.Ident).Name + "." + x.Sel.Name
		case *ast.ArrayType:
			if id, ok := x.Elt.(*ast.Ident); ok && x.Len == nil {
				return "[]" + id.Name
			}
		}
		return "?"
	}
	for _, g := range Groups {
		st := structs[g.Field]
		if st == nil {
			t.Errorf("no struct type %s in theme.go", g.Field)
			continue
		}
		var colors, numbers, sizes, weights, durations, curves, springs []string
		tracking := ""
		for _, fld := range st.Fields.List {
			for _, n := range fld.Names {
				switch typ := typeName(fld.Type); typ {
				case "ColorToken":
					colors = append(colors, n.Name)
				case "float32", "float64":
					numbers = append(numbers, n.Name)
				case "SizeTable":
					sizes = append(sizes, n.Name)
				case "driver.FontWeight":
					weights = append(weights, n.Name)
				case "time.Duration":
					durations = append(durations, n.Name)
				case "Curve":
					curves = append(curves, n.Name)
				case "Spring":
					springs = append(springs, n.Name)
				case "[]TrackingStep":
					tracking = n.Name
				default:
					t.Errorf("%s.%s has type %s, which the generator does not know", g.Field, n.Name, typ)
				}
			}
		}
		for _, c := range []struct {
			kind      string
			got, want []string
		}{{"colors", colors, g.Colors}, {"numbers", numbers, g.Numbers}, {"sizes", sizes, g.Sizes}, {"weights", weights, g.Weights},
			{"durations", durations, g.Durations}, {"curves", curves, g.Curves}, {"springs", springs, g.Springs}} {
			if strings.Join(c.got, ",") != strings.Join(c.want, ",") {
				t.Errorf("%s %s in theme.go\n%v\nin Groups\n%v", g.Field, c.kind, c.got, c.want)
			}
		}
		if tracking != g.Tracking {
			t.Errorf("%s tracking %q in theme.go, %q in Groups", g.Field, tracking, g.Tracking)
		}
	}
}
