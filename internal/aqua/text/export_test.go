package text

// IsGraphemeBoundary reports whether off is a grapheme cluster boundary of s
// (0 and len(s) always are).
func IsGraphemeBoundary(s string, off int) bool {
	return ClusterStart(s, off) == off
}
