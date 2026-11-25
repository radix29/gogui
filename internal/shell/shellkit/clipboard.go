package shellkit

import (
	"strings"
	"unicode/utf8"
)

// ClipboardText returns text as every shell's clipboard takes it (review
// V4): cut at the first NUL (CF_UNICODETEXT and C strings end there), and
// each byte of invalid UTF-8 replaced with U+FFFD, as Go decodes it (range,
// []rune), so Win32's UTF-16 conversion gives the same text.
func ClipboardText(text string) string {
	text, _, _ = strings.Cut(text, "\x00")
	if utf8.ValidString(text) {
		return text
	}
	return string([]rune(text))
}
