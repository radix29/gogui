package gtk

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// boundStructs are the binding structs, by name, with the C prefix of
// their functions and the library those come from.
var boundStructs = []struct {
	name, prefix, lib string
	v                 any
}{
	{"Gtk", "gtk_", GTKLib, &Gtk},
	{"Gdk", "gdk_", GTKLib, &Gdk},
	{"Gsk", "gsk_", GTKLib, &Gsk},
	{"GObject", "g_", GObjectLib, &GObject},
	{"GLib", "g_", GLibLib, &GLib},
	{"Pango", "pango_", PangoLib, &Pango},
	{"PangoCairo", "pango_cairo_", PangoCairoLib, &PangoCairo},
	{"Cairo", "cairo_", CairoLib, &Cairo},
	{"PangoFc", "pango_fc_", PangoFT2Lib, &PangoFc},
	{"Fc", "Fc", FontconfigLib, &Fc},
}

// goName turns a C function name without its prefix into the field name
// the tables use: snake_case to CamelCase, with "css", "id" and "im" as
// initialisms.
func goName(c string) string {
	var b strings.Builder
	for _, p := range strings.Split(c, "_") {
		switch p {
		case "css", "id", "im":
			b.WriteString(strings.ToUpper(p))
		default:
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

// TestBindingTables checks, on every host, that the tables in libraries()
// bind every func field of the binding structs exactly once, from the
// library its struct belongs to, under the C name the field name spells.
func TestBindingTables(t *testing.T) {
	type field struct{ strct, name, prefix, lib string }
	fields := map[uintptr]field{}
	for _, s := range boundStructs {
		v := reflect.ValueOf(s.v).Elem()
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if f.Type.Kind() != reflect.Func {
				t.Errorf("%s.%s is a %s, not a func", s.name, f.Name, f.Type)
				continue
			}
			fields[v.Field(i).Addr().Pointer()] = field{s.name, f.Name, s.prefix, s.lib}
		}
	}

	seen := map[uintptr]bool{}
	names := map[string]bool{}
	graphene := false
	for _, l := range libraries() {
		if l.soname == GrapheneLib {
			graphene = true
		}
		for _, b := range slices.Concat(l.bindings, l.optional) {
			if names[b.name] {
				t.Errorf("%s is bound twice", b.name)
			}
			names[b.name] = true
			p := reflect.ValueOf(b.fptr).Pointer()
			f, ok := fields[p]
			if !ok {
				t.Errorf("%s binds into %T, which is not a field of the binding structs", b.name, b.fptr)
				continue
			}
			seen[p] = true
			if l.soname != f.lib {
				t.Errorf("%s.%s is bound from %s, want %s", f.strct, f.name, l.soname, f.lib)
			}
			c, ok := strings.CutPrefix(b.name, f.prefix)
			if !ok || goName(c) != f.name {
				t.Errorf("%s.%s is bound to %s, which does not start with %s or spells %s",
					f.strct, f.name, b.name, f.prefix, goName(c))
			}
		}
	}
	for p, f := range fields {
		if !seen[p] {
			t.Errorf("%s.%s is not in libraries(), so Load never sets it", f.strct, f.name)
		}
	}
	if !graphene {
		t.Errorf("libraries() does not open %s", GrapheneLib)
	}
	if l := libraries(); len(l) == 0 || l[0].soname != GTKLib {
		t.Errorf("libraries() does not open %s first, so a missing GTK is not what the error names", GTKLib)
	}
}

func TestGoName(t *testing.T) {
	for c, want := range map[string]string{
		"window_new":                     "WindowNew",
		"widget_class_set_css_name":      "WidgetClassSetCSSName",
		"main_context_find_source_by_id": "MainContextFindSourceByID",
		"version":                        "Version",
	} {
		if got := goName(c); got != want {
			t.Errorf("goName(%q) = %q, want %q", c, got, want)
		}
	}
}

// TestFloorBindings checks the S4.2a floor (GTK 4.14): what was optional
// on GTK 4.10 (gdk_surface_get_scale, 4.12) is required now, the integer
// gtk_widget_get_scale_factor path is gone, and the only optional binding
// is Pango 1.56's pango_font_map_add_font_file (S4.2j; above the 1.52
// floor).
func TestFloorBindings(t *testing.T) {
	required := map[string]bool{}
	var optional []string
	for _, l := range libraries() {
		for _, b := range l.optional {
			optional = append(optional, l.soname+": "+b.name)
		}
		for _, b := range l.bindings {
			required[b.name] = true
		}
	}
	if want := []string{PangoLib + ": pango_font_map_add_font_file"}; !slices.Equal(optional, want) {
		t.Errorf("optional bindings %q, want %q", optional, want)
	}
	if !required["gdk_surface_get_scale"] {
		t.Error("gdk_surface_get_scale is not a required binding")
	}
	if required["gtk_widget_get_scale_factor"] {
		t.Error("gtk_widget_get_scale_factor is still bound")
	}
}

// TestAppFontBindings checks the S4.2j bindings: the app font needs
// fontconfig's current config and app-font call, the PangoFc config calls
// of libpangoft2 and g_error_free, all required.
func TestAppFontBindings(t *testing.T) {
	required := map[string]string{}
	for _, l := range libraries() {
		for _, b := range l.bindings {
			required[b.name] = l.soname
		}
	}
	for name, lib := range map[string]string{
		"FcConfigGetCurrent":               FontconfigLib,
		"FcConfigAppFontAddFile":           FontconfigLib,
		"pango_fc_font_map_get_config":     PangoFT2Lib,
		"pango_fc_font_map_config_changed": PangoFT2Lib,
		"g_error_free":                     GLibLib,
	} {
		if required[name] != lib {
			t.Errorf("%s is not a required binding of %s", name, lib)
		}
	}
}

// TestStrokeBindings checks the S4.2b bindings: the gradient stroke of a
// rounded rect needs the path builder, the stroke and the stroke push, all
// required (GTK 4.14 has them).
func TestStrokeBindings(t *testing.T) {
	required := map[string]bool{}
	for _, l := range libraries() {
		if l.soname != GTKLib {
			continue
		}
		for _, b := range l.bindings {
			required[b.name] = true
		}
	}
	for _, name := range []string{
		"gsk_path_builder_new", "gsk_path_builder_add_rect", "gsk_path_builder_add_rounded_rect", "gsk_path_builder_free_to_path",
		"gsk_path_unref", "gsk_stroke_new", "gsk_stroke_free", "gtk_snapshot_push_stroke",
	} {
		if !required[name] {
			t.Errorf("%s is not a required binding of %s", name, GTKLib)
		}
	}
}

// TestPathBindings checks the S4.2c bindings: FillPath and StrokePath need
// the rest of the path builder, the GskStroke setters and the fill and
// stroke calls of GtkSnapshot, all required (GTK 4.14 has them).
func TestPathBindings(t *testing.T) {
	required := map[string]bool{}
	for _, l := range libraries() {
		if l.soname != GTKLib {
			continue
		}
		for _, b := range l.bindings {
			required[b.name] = true
		}
	}
	for _, name := range []string{
		"gsk_path_builder_move_to", "gsk_path_builder_line_to", "gsk_path_builder_quad_to",
		"gsk_path_builder_cubic_to", "gsk_path_builder_close",
		"gsk_stroke_set_line_cap", "gsk_stroke_set_line_join", "gsk_stroke_set_miter_limit",
		"gsk_stroke_set_dash", "gsk_stroke_set_dash_offset",
		"gtk_snapshot_append_fill", "gtk_snapshot_push_fill", "gtk_snapshot_append_stroke",
	} {
		if !required[name] {
			t.Errorf("%s is not a required binding of %s", name, GTKLib)
		}
	}
}

// TestShadowBindings checks the S4.2d bindings: DrawShadow needs the
// outset and inset shadow calls of GtkSnapshot, both required (GTK 4.0
// has them).
func TestShadowBindings(t *testing.T) {
	required := map[string]bool{}
	for _, l := range libraries() {
		if l.soname != GTKLib {
			continue
		}
		for _, b := range l.bindings {
			required[b.name] = true
		}
	}
	for _, name := range []string{"gtk_snapshot_append_outset_shadow", "gtk_snapshot_append_inset_shadow"} {
		if !required[name] {
			t.Errorf("%s is not a required binding of %s", name, GTKLib)
		}
	}
}

// TestImageBindings checks the S4.2e bindings: the canvas's images need
// the scaled-texture append and the memory texture of GTK, and GBytes of
// GLib, all required (GTK 4.10 has the scaled append).
func TestImageBindings(t *testing.T) {
	required := map[string]string{}
	for _, l := range libraries() {
		for _, b := range l.bindings {
			required[b.name] = l.soname
		}
	}
	for name, lib := range map[string]string{
		"gtk_snapshot_append_scaled_texture": GTKLib,
		"gdk_memory_texture_new":             GTKLib,
		"g_bytes_new":                        GLibLib,
		"g_bytes_unref":                      GLibLib,
	} {
		if required[name] != lib {
			t.Errorf("%s is not a required binding of %s", name, lib)
		}
	}
}

// TestBlurBindings checks the S4.2f bindings: the backdrop blur's
// segments need gtk_snapshot_new, free_to_node, append_node and push_blur
// of GTK, and gsk_render_node_unref, all required (GTK 4.0 has them).
func TestBlurBindings(t *testing.T) {
	required := map[string]bool{}
	for _, l := range libraries() {
		if l.soname != GTKLib {
			continue
		}
		for _, b := range l.bindings {
			required[b.name] = true
		}
	}
	for _, name := range []string{"gtk_snapshot_new", "gtk_snapshot_free_to_node", "gtk_snapshot_append_node",
		"gtk_snapshot_push_blur", "gsk_render_node_unref"} {
		if !required[name] {
			t.Errorf("%s is not a required binding of %s", name, GTKLib)
		}
	}
}

// TestTextDrawBindings checks the S4.2i binding: DrawText needs
// gtk_snapshot_append_layout of GTK, required (GTK 4.0 has it).
func TestTextDrawBindings(t *testing.T) {
	required := map[string]bool{}
	for _, l := range libraries() {
		if l.soname != GTKLib {
			continue
		}
		for _, b := range l.bindings {
			required[b.name] = true
		}
	}
	if !required["gtk_snapshot_append_layout"] {
		t.Errorf("gtk_snapshot_append_layout is not a required binding of %s", GTKLib)
	}
}
