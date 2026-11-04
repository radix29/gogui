package driver

import "testing"

func TestModifiersResolve(t *testing.T) {
	m := ModPrimary | ModShift
	if got := m.Resolve(true); got != ModCommand|ModShift {
		t.Errorf("mac: %b", got)
	}
	if got := m.Resolve(false); got != ModControl|ModShift {
		t.Errorf("other: %b", got)
	}
	if got := ModOption.Resolve(true); got != ModOption {
		t.Errorf("no primary: %b", got)
	}
}

func TestNormalizeKey(t *testing.T) {
	for in, want := range map[Key]Key{
		'a': 'A', 'Z': 'Z', '+': '+', 'é': 'É', KeyUp: KeyUp, KeyReturn: KeyReturn,
	} {
		if got := NormalizeKey(in); got != want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
	if s := NewShortcut(ModPrimary, 'n'); s.Key != 'N' {
		t.Errorf("NewShortcut did not normalize: %q", s.Key)
	}
}

func TestShortcutDisplay(t *testing.T) {
	for _, tc := range []struct {
		s          Shortcut
		mac, other string
	}{
		{Shortcut{}, "", ""},
		{Shortcut{ModPrimary, 'c'}, "⌘C", "Ctrl+C"},
		{Shortcut{ModPrimary | ModShift, 'D'}, "⇧⌘D", "Ctrl+Shift+D"},
		{Shortcut{ModPrimary | ModOption, 'H'}, "⌥⌘H", "Ctrl+Alt+H"},
		{Shortcut{ModControl | ModOption | ModShift | ModCommand, 'K'}, "⌃⌥⇧⌘K", "Ctrl+Alt+Shift+Super+K"},
		{Shortcut{ModPrimary, '+'}, "⌘+", "Ctrl++"},
		{Shortcut{0, KeyF1 + 9}, "F10", "F10"},
		{Shortcut{ModPrimary, KeyUp}, "⌘↑", "Ctrl+Up"},
		{Shortcut{0, KeyEscape}, "⎋", "Esc"},
		{Shortcut{ModShift, KeyPageDown}, "⇧⇟", "Shift+PgDn"},
	} {
		if got := tc.s.Display(true); got != tc.mac {
			t.Errorf("%+v mac = %q, want %q", tc.s, got, tc.mac)
		}
		if got := tc.s.Display(false); got != tc.other {
			t.Errorf("%+v other = %q, want %q", tc.s, got, tc.other)
		}
	}
}

func TestFunctionKeys(t *testing.T) {
	if !KeyF12.IsFunctionKey() || KeyF12-KeyF1 != 11 || KeyUp.IsFunctionKey() {
		t.Error("function key range is wrong")
	}
}
