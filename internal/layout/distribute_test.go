package layout

import (
	"math"
	"testing"
)

func sp(natural, min, max float64, expand bool) span {
	return span{natural: natural, min: min, max: max, expand: expand}
}

// fix is a non-expanding span with min = natural and no max.
func fix(v float64) span { return sp(v, v, inf, false) }

// exp is an expanding span with min = natural and no max.
func exp(v float64) span { return sp(v, v, inf, true) }

func sum(sizes []float64) float64 {
	var s float64
	for _, v := range sizes {
		s += v
	}
	return s
}

func TestDistribute(t *testing.T) {
	nan := math.NaN()
	cases := []struct {
		name  string
		items []span
		space float64
		want  []float64
	}{
		{"empty", nil, 100, []float64{}},
		{"empty inf", nil, inf, []float64{}},
		{"no expanders", []span{fix(10), fix(20)}, 100, []float64{10, 20}},
		{"exact fit", []span{fix(10), exp(20)}, 30, []float64{10, 20}},
		{"one expander", []span{fix(10), exp(20), fix(5)}, 100, []float64{10, 85, 5}},
		{"two expanders", []span{exp(10), fix(20), exp(30)}, 100, []float64{30, 20, 50}},
		{"spacer expanders", []span{exp(0), fix(40), exp(0)}, 100, []float64{30, 40, 30}},
		{"cap reshares", []span{sp(10, 10, 20, true), exp(10)}, 100, []float64{20, 80}},
		{"cap cascade", []span{sp(0, 0, 5, true), sp(0, 0, 30, true), exp(0)}, 90,
			[]float64{5, 30, 55}},
		{"all capped", []span{sp(10, 10, 20, true), sp(10, 10, 30, true)}, 100,
			[]float64{20, 30}},
		{"max 0 expander", []span{sp(0, 0, 0, true), fix(10)}, 100, []float64{0, 10}},
		{"max 0 and expander", []span{sp(0, 0, 0, true), exp(10)}, 100, []float64{0, 100}},
		{"inf max", []span{sp(10, 10, inf, true)}, 1e6, []float64{1e6}},
		{"inf space", []span{exp(10), fix(20)}, inf, []float64{10, 20}},
		{"inf space clamps", []span{sp(50, 0, 30, true), sp(5, 10, inf, false)}, inf,
			[]float64{30, 10}},
		{"shrink part-way", []span{sp(100, 50, inf, false), sp(60, 40, inf, true)}, 130,
			// deficit 30, flex 50 and 20: shares 30·50/70 and 30·20/70
			[]float64{100 - 150.0/7, 60 - 60.0/7}},
		{"shrink zero flex", []span{fix(40), sp(60, 20, inf, false)}, 80,
			[]float64{40, 40}},
		{"shrink to minima", []span{sp(100, 50, inf, false), sp(60, 40, inf, true)}, 90,
			[]float64{50, 40}},
		{"overflow", []span{sp(100, 50, inf, false), fix(40)}, 30, []float64{50, 40}},
		{"zero space", []span{sp(10, 0, inf, true), sp(10, 5, inf, false)}, 0,
			[]float64{0, 5}},
		{"negative space", []span{sp(10, 0, inf, false), fix(5)}, -50, []float64{0, 5}},
		{"NaN space", []span{sp(10, 2, inf, true)}, nan, []float64{2}},
		{"min > max", []span{sp(5, 30, 10, true), exp(0)}, 100, []float64{30, 70}},
		{"natural above max", []span{sp(50, 0, 20, false)}, 100, []float64{20}},
		{"natural below min", []span{sp(5, 20, inf, false)}, 100, []float64{20}},
		{"bad inputs", []span{sp(nan, -3, nan, true), sp(inf, inf, -1, false)}, 50,
			// first: natural 0, min 0, max 0; second: natural 0, min 0, max 0
			[]float64{0, 0}},
		{"bad natural inf", []span{sp(inf, 0, inf, true)}, 40, []float64{40}},
	}
	for _, c := range cases {
		got := distribute(c.items, c.space)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if !near(got[i], c.want[i]) {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestDistributeDoesNotModifyInput(t *testing.T) {
	items := []span{sp(50, 0, 20, true), exp(5)}
	before := append([]span(nil), items...)
	distribute(items, 10)
	for i := range items {
		if items[i] != before[i] {
			t.Fatalf("item %d changed: %v -> %v", i, before[i], items[i])
		}
	}
}

// When the space is used up, the sizes add up to it exactly (==, not
// within eps), and repeated runs give bit-identical results.
func TestDistributeExactSum(t *testing.T) {
	cases := []struct {
		name  string
		items []span
		space float64
	}{
		{"100 three ways", []span{exp(0), exp(0), exp(0)}, 100},
		{"thirds of 1", []span{exp(0), exp(0), exp(0)}, 1},
		{"sevenths", []span{exp(0.1), fix(0.2), exp(0.3), exp(0), exp(0), exp(0), exp(0), exp(0)}, 10.3},
		{"capped then three", []span{sp(0, 0, 0.1, true), exp(0), exp(0), exp(0)}, 100.7},
		{"shrink odd", []span{sp(33.3, 0, inf, false), sp(66.7, 10.1, inf, true), sp(1e-3, 0, inf, false)}, 77.7},
		{"shrink many", []span{sp(0.3, 0.1, inf, false), sp(0.7, 0, inf, false), sp(1.1, 0.2, inf, false)}, 1.3},
	}
	for _, c := range cases {
		first := distribute(c.items, c.space)
		if s := sum(first); s != c.space {
			t.Errorf("%s: sum = %v, want exactly %v (%v)", c.name, s, c.space, first)
		}
		for range 10 {
			again := distribute(c.items, c.space)
			for i := range again {
				if math.Float64bits(again[i]) != math.Float64bits(first[i]) {
					t.Fatalf("%s: run differs: %v vs %v", c.name, again, first)
				}
			}
		}
	}
	// 100 split three ways: the leftover goes to the last expander.
	got := distribute([]span{exp(0), exp(0), exp(0)}, 100)
	if third := 100.0 / 3; got[0] != third || got[1] != third || !near(got[2], third) {
		t.Errorf("100/3: %v", got)
	}
}

// Many random-ish inputs: the sizes always stay within [min, max] (up to
// eps), and when the space is used up they sum to it exactly, or (in a
// rounding gap) to the float just below it.
func TestDistributeInvariants(t *testing.T) {
	seed := uint64(1)
	rnd := func(n float64) float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11) / (1 << 53) * n
	}
	gaps := 0
	for run := range 2000 {
		items := make([]span, 1+run%7)
		var minSum, natSum float64
		anyGrows := false
		for i := range items {
			lo := math.Floor(rnd(50)*10) / 10
			nat := lo + rnd(80)
			hi := inf
			if run%3 == 0 {
				hi = nat + rnd(40)
			}
			items[i] = sp(nat, lo, hi, rnd(1) < 0.5)
			minSum += lo
			natSum += nat
			anyGrows = anyGrows || items[i].expand && math.IsInf(hi, 1)
		}
		space := rnd(natSum * 2)
		got := distribute(items, space)
		for i, it := range items {
			if got[i] < it.min-eps || got[i] > it.max+eps {
				t.Fatalf("run %d: size %d = %v outside [%v, %v]", run, i, got[i], it.min, it.max)
			}
		}
		usedUp := space > minSum && (space <= natSum || anyGrows)
		if s := sum(got); usedUp && s != space {
			if s < space && math.Nextafter(s, inf) >= space {
				gaps++ // rounding gap: one ulp below, never above
				continue
			}
			t.Fatalf("run %d: sum %v != space %v (%v)", run, sum(got), space, got)
		}
	}
	if gaps > 2000/10 {
		t.Errorf("%d of 2000 runs hit a rounding gap; expected a few", gaps)
	}
}
