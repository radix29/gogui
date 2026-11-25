package text

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

const (
	combAcute = "e\u0301"                                    // é as e + combining acute (3 bytes)
	family    = "\U0001F468\u200d\U0001F469\u200d\U0001F467" // ZWJ family, one cluster (18 bytes)
	flagDE    = "\U0001F1E9\U0001F1EA"                       // regional indicator pair (8 bytes)
	flagFR    = "\U0001F1EB\U0001F1F7"
	jamo      = "\u1100\u1161\u11a8" // 각 as conjoining jamo L V T, one cluster (9 bytes)
	arabicNum = "\u0600"             // a Prepend: joins the next rune's cluster but not its word
)

func TestGraphemes(t *testing.T) {
	tests := []struct {
		name string
		s    string
		// want lists the grapheme boundaries of s, 0 and len(s) included.
		want []int
	}{
		{"empty", "", []int{0}},
		{"ascii", "abc", []int{0, 1, 2, 3}},
		{"combining", "a" + combAcute + "b", []int{0, 1, 4, 5}},
		{"two combining", "o\u0308\u0304x", []int{0, 5, 6}},
		{"zwj family", "a" + family + "b", []int{0, 1, 19, 20}},
		{"flags", flagDE + flagFR, []int{0, 8, 16}},
		{"odd flags", flagDE + "\U0001F1EB" + "x", []int{0, 8, 12, 13}},
		{"emoji modifier", "\U0001F44D\U0001F3FD!", []int{0, 8, 9}},
		{"hangul jamo", jamo + "x", []int{0, 9, 10}},
		{"hangul syllables", "한국", []int{0, 3, 6}},
		{"crlf", "a\r\nb", []int{0, 1, 3, 4}},
		{"cr cr", "\r\r", []int{0, 1, 2}},
		{"cjk", "日本語の", []int{0, 3, 6, 9, 12}},
		{"prepend", arabicNum + "1", []int{0, 3}},
		{"space mark", " \u0301x", []int{0, 3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refGraphemes(tt.s); !slices.Equal(got, tt.want) {
				t.Fatalf("uniseg boundaries = %v, want %v", got, tt.want)
			}
			checkGraphemes(t, tt.s)
		})
	}
}

// checkGraphemes checks the grapheme functions at every offset of s
// against the boundaries of a full scan.
func checkGraphemes(t *testing.T, s string) {
	t.Helper()
	bs := refGraphemes(s)
	for off := 0; off <= len(s); off++ {
		i, ok := slices.BinarySearch(bs, off) // bs[i] is the first boundary ≥ off
		start := off
		if !ok {
			start = bs[i-1]
		}
		next := len(s)
		if j := i; j < len(bs) && bs[j] == off {
			j++
			if j < len(bs) {
				next = bs[j]
			}
		} else if j < len(bs) {
			next = bs[j]
		}
		prev := 0
		if i > 0 {
			prev = bs[i-1]
		}
		if got := ClusterStart(s, off); got != start {
			t.Fatalf("ClusterStart(%q, %d) = %d, want %d", s, off, got, start)
		}
		if got := IsGraphemeBoundary(s, off); got != ok {
			t.Fatalf("IsGraphemeBoundary(%q, %d) = %v, want %v", s, off, got, ok)
		}
		if got := NextGrapheme(s, off); got != next {
			t.Fatalf("NextGrapheme(%q, %d) = %d, want %d", s, off, got, next)
		}
		if got := PrevGrapheme(s, off); got != prev {
			t.Fatalf("PrevGrapheme(%q, %d) = %d, want %d", s, off, got, prev)
		}
	}
}

func TestWords(t *testing.T) {
	tests := []struct {
		name string
		s    string
		// want is s split into its segments, words marked with [].
		want string
	}{
		{"plain", "foo bar", "[foo]· ·[bar]"},
		{"punctuation", "foo, bar.", "[foo]·,· ·[bar]·."},
		{"apostrophe", "don't stop", "[don't]· ·[stop]"},
		{"typographic apostrophe", "don’t", "[don’t]"},
		{"leading quote", "'tis", "'·[tis]"},
		{"number", "pi is 3.14.", "[pi]· ·[is]· ·[3.14]·."},
		{"thousands", "1,000,000 x", "[1,000,000]· ·[x]"},
		{"underscore", "foo_bar __", "[foo_bar]· ·__"},
		{"spaces", "a   b", "[a]·   ·[b]"},
		{"tab", "a\t b", "[a]·\t· ·[b]"},
		{"combining", "caf" + combAcute + " au", "[caf" + combAcute + "]· ·[au]"},
		{"hangul", "안녕 세상", "[안녕]· ·[세상]"},
		{"cjk", "日本語のテキスト", "[日]·[本]·[語]·[の]·[テキスト]"},
		{"cjk latin", "日本Go", "[日]·[本]·[Go]"},
		{"emoji", "a " + family + " b", "[a]· ·" + family + "· ·[b]"},
		{"flags", flagDE + flagFR, flagDE + "·" + flagFR},
		{"prepend merged", arabicNum + ".", arabicNum + "."},
		{"space mark", " \u0301x", " \u0301·[x]"},
		{"crlf", "a\r\nb", "[a]·\r\n·[b]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parts []string
			for _, sg := range refWordSegs(tt.s) {
				if sg.word {
					parts = append(parts, "["+tt.s[sg.a:sg.b]+"]")
				} else {
					parts = append(parts, tt.s[sg.a:sg.b])
				}
			}
			if got := strings.Join(parts, "·"); got != tt.want {
				t.Fatalf("segments = %q, want %q", got, tt.want)
			}
			checkWords(t, tt.s)
		})
	}
}

func TestWordMoves(t *testing.T) {
	tests := []struct {
		s            string
		off          int
		next         int
		nextFound    bool
		prev         int
		prevFound    bool
		wStart, wEnd int
	}{
		{"foo, bar", 0, 3, true, 0, false, 0, 3},
		{"foo, bar", 1, 3, true, 0, true, 0, 3},
		{"foo, bar", 3, 8, true, 0, true, 3, 4},
		{"foo, bar", 4, 8, true, 0, true, 4, 5},
		{"foo, bar", 5, 8, true, 0, true, 5, 8},
		{"foo, bar", 8, 8, false, 5, true, 5, 8},
		{"  hello  ", 0, 7, true, 0, false, 0, 2},
		{"  hello  ", 7, 9, false, 2, true, 7, 9},
		{"  hello  ", 9, 9, false, 2, true, 7, 9},
		{"don't stop", 2, 5, true, 0, true, 0, 5},
		{"pi is 3.14.", 7, 10, true, 6, true, 6, 10},
		{"pi is 3.14.", 10, 11, false, 6, true, 10, 11},
		{"foo  bar", 4, 8, true, 0, true, 3, 5},
		{"a,,b", 1, 4, true, 0, true, 1, 2},
		{"a " + family + " b", 2, 22, true, 0, true, 2, 20},
		{"a " + family + " b", 5, 22, true, 0, true, 2, 20},
		{"日本語のテキスト", 13, 24, true, 12, true, 12, 24},
		{"日本語のテキスト", 3, 6, true, 0, true, 3, 6},
		{"caf" + combAcute + " au", 4, 6, true, 0, true, 0, 6},
		{"...", 1, 3, false, 0, false, 1, 2},
		{"", 0, 0, false, 0, false, 0, 0},
	}
	for _, tt := range tests {
		next, nf := NextWordEnd(tt.s, tt.off)
		prev, pf := PrevWordStart(tt.s, tt.off)
		ws, we := WordAt(tt.s, tt.off)
		if next != tt.next || nf != tt.nextFound {
			t.Errorf("NextWordEnd(%q, %d) = %d, %v, want %d, %v", tt.s, tt.off, next, nf, tt.next, tt.nextFound)
		}
		if prev != tt.prev || pf != tt.prevFound {
			t.Errorf("PrevWordStart(%q, %d) = %d, %v, want %d, %v", tt.s, tt.off, prev, pf, tt.prev, tt.prevFound)
		}
		if ws != tt.wStart || we != tt.wEnd {
			t.Errorf("WordAt(%q, %d) = %d, %d, want %d, %d", tt.s, tt.off, ws, we, tt.wStart, tt.wEnd)
		}
	}
}

// checkWords checks the word functions at every offset of s against a model
// built on a full scan, and that each result is a grapheme boundary.
func checkWords(t *testing.T, s string) {
	t.Helper()
	segs := refWordSegs(s)
	bs := refGraphemes(s)
	isB := func(off int) bool { _, ok := slices.BinarySearch(bs, off); return ok }
	for _, sg := range segs {
		if !isB(sg.a) || !isB(sg.b) {
			t.Fatalf("%q: segment [%d, %d) not on grapheme boundaries", s, sg.a, sg.b)
		}
	}
	for off := 0; off <= len(s); off++ {
		next, nf := len(s), false
		for _, sg := range segs {
			if sg.word && sg.b > off {
				next, nf = sg.b, true
				break
			}
		}
		prev, pf := 0, false
		for _, sg := range segs {
			if sg.word && sg.a < off {
				prev, pf = sg.a, true
			}
		}
		ws, we := refWordAt(s, segs, bs, off)
		if got, f := NextWordEnd(s, off); got != next || f != nf {
			t.Fatalf("NextWordEnd(%q, %d) = %d, %v, want %d, %v", s, off, got, f, next, nf)
		}
		if got, f := PrevWordStart(s, off); got != prev || f != pf {
			t.Fatalf("PrevWordStart(%q, %d) = %d, %v, want %d, %v", s, off, got, f, prev, pf)
		}
		if gs, ge := WordAt(s, off); gs != ws || ge != we {
			t.Fatalf("WordAt(%q, %d) = %d, %d, want %d, %d", s, off, gs, ge, ws, we)
		}
	}
}

// refGraphemes returns every grapheme boundary of s from a scan from 0.
func refGraphemes(s string) []int {
	bs := []int{0}
	for rest, st, off := s, -1, 0; rest != ""; {
		var c string
		c, rest, _, st = uniseg.FirstGraphemeClusterInString(rest, st)
		off += len(c)
		bs = append(bs, off)
	}
	return bs
}

type seg struct {
	a, b int
	word bool
}

// refWordSegs splits s at the UAX #29 word boundaries of a scan from 0 that
// are also grapheme boundaries.
func refWordSegs(s string) []seg {
	bs := refGraphemes(s)
	var segs []seg
	a := 0
	for rest, st, off := s, -1, 0; rest != ""; {
		var w string
		w, rest, st = uniseg.FirstWordInString(rest, st)
		off += len(w)
		if _, ok := slices.BinarySearch(bs, off); !ok {
			continue
		}
		segs = append(segs, seg{a, off, isWordText(s[a:off])})
		a = off
	}
	return segs
}

func refWordAt(s string, segs []seg, bs []int, off int) (int, int) {
	if s == "" {
		return 0, 0
	}
	// The cluster at off, or before it at the end.
	i, _ := slices.BinarySearch(bs, off+1)
	ci := i - 1
	if off == len(s) {
		ci = len(bs) - 2
	}
	space := func(k int) bool { return isSpaceCluster(s[bs[k]:]) }
	if space(ci) {
		lo, hi := ci, ci+1
		for lo > 0 && space(lo-1) {
			lo--
		}
		for hi < len(bs)-1 && space(hi) {
			hi++
		}
		return bs[lo], bs[hi]
	}
	for _, sg := range segs {
		if sg.a <= bs[ci] && bs[ci] < sg.b && sg.word {
			return sg.a, sg.b
		}
	}
	return bs[ci], bs[ci+1]
}

// pieces are the tricky bits the random and fuzz tests build strings from.
var pieces = []string{
	"a", "Z", "7", " ", "  ", "\t", ".", ",", "'", "’", "_", "-", "!", "\r", "\n", "\r\n",
	"\u0301", "\u200d", "\u200b", combAcute, family, "\U0001F600", "\U0001F3FD",
	"\U0001F1E9", "\U0001F1EA", flagFR, "\u1100", "\u1161", "\u11a8", "한", jamo,
	"日", "本", "の", "テ", "キ", "\u3099", "ก", "\u0e33", arabicNum, "\u0903",
	"don't", "3.14", "\xff", "\x80",
}

func randPieces(r *rand.Rand, n int) string {
	var sb strings.Builder
	for range n {
		sb.WriteString(pieces[r.IntN(len(pieces))])
	}
	return sb.String()
}

func TestBoundariesRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 1))
	for range 3000 {
		s := randPieces(r, 1+r.IntN(12))
		checkGraphemes(t, s)
		checkWords(t, s)
	}
}

func FuzzBoundaries(f *testing.F) {
	for _, s := range []string{"", "foo, bar", "don't 3.14", family + flagDE + jamo, "日本語のテキスト", "a\r\nb", arabicNum + ".", "\xff\x80a"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 256 {
			s = s[:256]
		}
		checkGraphemes(t, s)
		checkWords(t, s)
	})
}

func TestBoundaryPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"NextGrapheme":  func() { NextGrapheme("ab", 3) },
		"PrevGrapheme":  func() { PrevGrapheme("ab", -1) },
		"ClusterStart":  func() { ClusterStart("", 1) },
		"NextWordEnd":   func() { NextWordEnd("ab", 3) },
		"PrevWordStart": func() { PrevWordStart("ab", 3) },
		"WordAt":        func() { WordAt("ab", -1) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			fn()
		}()
	}
}

// TestSafePointsAreLocal pins that a step in a long spaced or CJK paragraph
// scans from nearby: it must not see a broken prefix far before off.
func TestSafePointsAreLocal(t *testing.T) {
	for _, s := range []string{strings.Repeat("lorem ipsum ", 1000), strings.Repeat("日本語の", 1000)} {
		mid := len(s) / 2
		mid = ClusterStart(s, mid)
		if p := graphemeSafe(s, mid); mid-p > 8 {
			t.Errorf("graphemeSafe walked back %d bytes", mid-p)
		}
		if p := wordSafe(s, mid); mid-p > 16 {
			t.Errorf("wordSafe walked back %d bytes", mid-p)
		}
	}
}

// paragraph1MB returns a 1 MB paragraph (no newline) of repeated unit.
func paragraph1MB(unit string) string {
	s := strings.Repeat(unit, (1<<20)/len(unit)+1)[:1<<20]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func benchBoundaries(b *testing.B, s string) {
	mid := ClusterStart(s, len(s)/2)
	b.Run("NextGrapheme", func(b *testing.B) {
		for b.Loop() {
			NextGrapheme(s, mid)
		}
	})
	b.Run("PrevGrapheme", func(b *testing.B) {
		for b.Loop() {
			PrevGrapheme(s, mid)
		}
	})
	b.Run("NextWordEnd", func(b *testing.B) {
		for b.Loop() {
			NextWordEnd(s, mid)
		}
	})
	b.Run("PrevWordStart", func(b *testing.B) {
		for b.Loop() {
			PrevWordStart(s, mid)
		}
	})
	b.Run("WordAt", func(b *testing.B) {
		for b.Loop() {
			WordAt(s, mid)
		}
	})
}

// BenchmarkBoundariesLatin1MB steps in the middle of a 1 MB paragraph of
// spaced Latin text with accents.
func BenchmarkBoundariesLatin1MB(b *testing.B) {
	benchBoundaries(b, paragraph1MB("Grüße, the café's naïve résumé costs 3.14 € today. "))
}

// BenchmarkBoundariesCJK1MB steps in the middle of a 1 MB paragraph of
// kanji and hiragana without spaces.
func BenchmarkBoundariesCJK1MB(b *testing.B) {
	benchBoundaries(b, paragraph1MB("日本語の文章を書きます。"))
}

// BenchmarkBoundariesOneWord64KB is the worst case: one 64 KB word, where
// nothing short of its start is provably a boundary.
func BenchmarkBoundariesOneWord64KB(b *testing.B) {
	benchBoundaries(b, strings.Repeat("x", 1<<16))
}
