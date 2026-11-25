package text

import (
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// The boundary functions work on one paragraph s (Buffer.ParagraphAt gives
// its range; crossing into the neighbouring paragraph is the Editor's job)
// and take and return byte offsets into s. Grapheme clusters are the
// extended grapheme clusters and words the word segments of UAX #29, both
// from rivo/uniseg; a word boundary that is not a grapheme boundary is
// dropped, so every offset returned is a grapheme boundary. An offset inside
// a cluster, even inside a UTF-8 sequence, counts as that cluster's.
// Offsets outside [0, len(s)] panic.
//
// The functions scan from a nearby offset that is provably a boundary (see
// graphemeSafe and wordSafe) rather than from the start of s, so a step in
// Latin, CJK or spaced text costs the same in a 1 MB paragraph as in a short
// one. Only long runs without such an offset (one huge word, Thai, Katakana,
// emoji) scan from their start.

// NextGrapheme returns the first grapheme cluster boundary after off, or
// len(s) at the end.
func NextGrapheme(s string, off int) int {
	checkOff(s, off)
	if off == len(s) {
		return off
	}
	p, state := graphemeSafe(s, off), -1
	for p <= off {
		c, _, _, st := uniseg.FirstGraphemeClusterInString(s[p:], state)
		p, state = p+len(c), st
	}
	return p
}

// PrevGrapheme returns the last grapheme cluster boundary before off, or 0
// at the start.
func PrevGrapheme(s string, off int) int {
	checkOff(s, off)
	if off == 0 {
		return 0
	}
	p := graphemeSafe(s, off-1)
	state := -1
	for {
		c, _, _, st := uniseg.FirstGraphemeClusterInString(s[p:], state)
		if p+len(c) >= off {
			return p
		}
		p, state = p+len(c), st
	}
}

// ClusterStart clamps off to the start of the grapheme cluster it is in: off
// itself when it is a boundary (len(s) included), else the boundary before.
func ClusterStart(s string, off int) int {
	checkOff(s, off)
	if off == len(s) {
		return off
	}
	p := graphemeSafe(s, off)
	state := -1
	for {
		c, _, _, st := uniseg.FirstGraphemeClusterInString(s[p:], state)
		if p+len(c) > off {
			return p
		}
		p, state = p+len(c), st
	}
}

// NextWordEnd is Option-→: the end of the word off is in, or else of the
// next word, skipping spaces and punctuation. Without a word after off it
// returns len(s) and false; the caller then moves into the next paragraph.
func NextWordEnd(s string, off int) (int, bool) {
	checkOff(s, off)
	end, found := len(s), false
	wordSegs(s, wordSafe(s, off), func(a, b int, word bool) bool {
		if word && b > off {
			end, found = b, true
			return false
		}
		return true
	})
	return end, found
}

// PrevWordStart is Option-←: the start of the word before off (off inside a
// word goes to that word's start), skipping spaces and punctuation. Without
// a word before off it returns 0 and false.
func PrevWordStart(s string, off int) (int, bool) {
	checkOff(s, off)
	for hi := off; hi > 0; {
		p := wordSafe(s, hi-1)
		start := -1
		wordSegs(s, p, func(a, b int, word bool) bool {
			if a >= hi {
				return false
			}
			if word {
				start = a
			}
			return true
		})
		if start >= 0 {
			return start, true
		}
		hi = p
	}
	return 0, false
}

// WordAt is the range a double-click at off selects: the word off is in; a
// run of whitespace clusters; else (punctuation, symbols, emoji) the one
// cluster at off. At len(s) it is the range of the cluster before; an empty
// s gives (0, 0).
func WordAt(s string, off int) (start, end int) {
	checkOff(s, off)
	if len(s) == 0 {
		return 0, 0
	}
	if off == len(s) {
		off = PrevGrapheme(s, off)
	}
	c := ClusterStart(s, off)
	if isSpaceCluster(s[c:]) {
		start, end = c, NextGrapheme(s, c)
		for start > 0 {
			q := PrevGrapheme(s, start)
			if !isSpaceCluster(s[q:]) {
				break
			}
			start = q
		}
		for end < len(s) && isSpaceCluster(s[end:]) {
			end = NextGrapheme(s, end)
		}
		return start, end
	}
	start, end = c, NextGrapheme(s, c)
	wordSegs(s, wordSafe(s, c), func(a, b int, word bool) bool {
		if b <= c {
			return true
		}
		if word {
			start, end = a, b
		}
		return false
	})
	return start, end
}

// wordSegs calls yield with the word segments of s from p on, p being a word
// and grapheme boundary, until yield returns false. A segment is a word when
// it holds a letter or a digit. Word boundaries inside a grapheme cluster
// are dropped, merging the segments around them.
func wordSegs(s string, p int, yield func(a, b int, word bool) bool) {
	a, q, g := p, p, p
	wstate, gstate := -1, -1
	for q < len(s) {
		w, _, st := uniseg.FirstWordInString(s[q:], wstate)
		q, wstate = q+len(w), st
		for g < q {
			c, _, _, gs := uniseg.FirstGraphemeClusterInString(s[g:], gstate)
			g, gstate = g+len(c), gs
		}
		if g != q {
			continue
		}
		if !yield(a, q, isWordText(s[a:q])) {
			return
		}
		a = q
	}
}

// isWordText reports whether a segment holds a letter or a digit (UAX #29
// keeps "don't" and "3.14" in one segment; ideographs are letters).
func isWordText(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// isSpaceCluster reports whether the cluster at the start of s is
// whitespace (its first rune is).
func isSpaceCluster(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}

// graphemeSafe returns the largest p ≤ off that is provably a grapheme
// boundary: 0, len(s), or a rune start between two "plain" runes (ASCII
// other than "\r\n", CJK ideographs, hiragana), which no rule of UAX #29
// joins and around which no rule looks further. uniseg can start afresh at
// such a p and segments the rest exactly as it would from 0.
func graphemeSafe(s string, off int) int {
	if off == len(s) {
		return off
	}
	for p := off; p > 0; p-- {
		if !utf8.RuneStart(s[p]) {
			continue
		}
		prev, _ := utf8.DecodeLastRuneInString(s[:p])
		cur, _ := utf8.DecodeRuneInString(s[p:])
		if plainGrapheme(prev) && plainGrapheme(cur) && (prev != '\r' || cur != '\n') {
			return p
		}
	}
	return 0
}

func plainGrapheme(r rune) bool { return r < utf8.RuneSelf || isCJK(r) }

// wordSafe returns the largest p ≤ off that is provably a word boundary (and
// a grapheme boundary): 0, len(s), or a rune start after a space, tab, CJK
// ideograph or hiragana and before ASCII, a CJK ideograph or hiragana, not
// between two spaces. Those runes are Word_Break Other or WSegSpace, which
// break before anything but Extend, Format and ZWJ (none of the runes after)
// and which no look-behind rule (WB7, WB12, WB15/16) reaches past.
func wordSafe(s string, off int) int {
	if off == len(s) {
		return off
	}
	for p := off; p > 0; p-- {
		if wordSafeAt(s, p) {
			return p
		}
	}
	return 0
}

// wordSafeAt reports whether 0 < p < len(s) is a safe point of wordSafe. It
// looks only at the runes on either side of p, and a rune cut by the start
// or end of s decodes as utf8.RuneError, which is never safe; so a safe point
// of a substring is one of the whole string too (the Editor's spans rely on
// it).
func wordSafeAt(s string, p int) bool {
	if !utf8.RuneStart(s[p]) {
		return false
	}
	prev, _ := utf8.DecodeLastRuneInString(s[:p])
	cur, _ := utf8.DecodeRuneInString(s[p:])
	return (prev == ' ' || prev == '\t' || isCJK(prev)) &&
		(cur < utf8.RuneSelf || isCJK(cur)) && (prev != ' ' || cur != ' ')
}

// isCJK reports the CJK Unified Ideographs block and the hiragana letters
// (not the combining and sound marks U+3099–U+309C): Grapheme_Cluster_Break
// Other and Word_Break Other, with no joining rule in either.
func isCJK(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF || r >= 0x3041 && r <= 0x3096
}

func checkOff(s string, off int) {
	if off < 0 || off > len(s) {
		panic("text: offset out of range")
	}
}
