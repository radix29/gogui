package shellkit

import "testing"

func TestClipboardText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"a\r\nb\nc", "a\r\nb\nc"},
		{"ünï 😀", "ünï 😀"},
		{"cut\x00here", "cut"},
		{"\x00", ""},
		{"bad\xffbyte", "bad�byte"},
		{"a\xff\xfeb", "a��b"},    // one U+FFFD per bad byte
		{"\xe2\x82", "��"},        // a cut-off sequence
		{"ok\xff\x00\xff", "ok�"}, // cut before the replacement
	} {
		if got := ClipboardText(c.in); got != c.want {
			t.Errorf("ClipboardText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
