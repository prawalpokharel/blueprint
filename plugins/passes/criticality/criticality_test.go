package criticality

import (
	"math"
	"testing"
)

func graphOf(edges ...[2]string) *Graph {
	g := NewGraph()
	for _, e := range edges {
		g.AddEdge(e[0], e[1])
	}
	return g
}

// a -> b -> c -> d
func chain() *Graph {
	return graphOf([2]string{"a", "b"}, [2]string{"b", "c"}, [2]string{"c", "d"})
}

// gw calls x, y and z; x and y share db; z uses cache.
func fan() *Graph {
	return graphOf([2]string{"gw", "x"}, [2]string{"gw", "y"}, [2]string{"gw", "z"},
		[2]string{"x", "db"}, [2]string{"y", "db"}, [2]string{"z", "cache"})
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %.12f, want %.12f", what, got, want)
	}
}

func check(t *testing.T, name string, got map[string]float64, want map[string]float64) {
	t.Helper()
	for n, w := range want {
		near(t, name+"["+n+"]", got[n], w)
	}
}

func TestChainMeasures(t *testing.T) {
	g := chain()
	check(t, "degree", g.DegreeCentrality(), map[string]float64{"a": 1.0 / 3, "b": 2.0 / 3, "c": 2.0 / 3, "d": 1.0 / 3})
	check(t, "betweenness", g.BetweennessCentrality(), map[string]float64{"a": 0, "b": 1.0 / 3, "c": 1.0 / 3, "d": 0})
	check(t, "closeness", g.ClosenessCentrality(), map[string]float64{"a": 0, "b": 1.0 / 3, "c": 4.0 / 9, "d": 0.5})
	check(t, "downstream", g.DownstreamReach(), map[string]float64{"a": 3, "b": 2, "c": 1, "d": 0})
	check(t, "upstream", g.UpstreamDependents(), map[string]float64{"a": 0, "b": 1, "c": 2, "d": 3})
	check(t, "fan-out", g.FanOut(), map[string]float64{"a": 1, "b": 1, "c": 1, "d": 0})
}

func TestFanMeasures(t *testing.T) {
	g := fan()
	check(t, "degree", g.DegreeCentrality(), map[string]float64{"gw": 0.6, "x": 0.4, "db": 0.4, "cache": 0.2})
	// x and y each carry half of the two gw->db shortest paths; z carries gw->cache.
	check(t, "betweenness", g.BetweennessCentrality(), map[string]float64{"gw": 0, "x": 0.025, "y": 0.025, "z": 0.05, "db": 0})
	check(t, "closeness", g.ClosenessCentrality(), map[string]float64{"gw": 0, "x": 0.2, "db": 0.45, "cache": 4.0 / 15})
	check(t, "upstream", g.UpstreamDependents(), map[string]float64{"gw": 0, "x": 1, "db": 3, "cache": 2})
}

func TestPageRankSumsToOneAndFollowsCalls(t *testing.T) {
	pr := chain().PageRank(0.85, 100, 1e-6)
	sum := 0.0
	for _, v := range pr {
		sum += v
	}
	near(t, "sum", sum, 1)
	if !(pr["a"] < pr["b"] && pr["b"] < pr["c"] && pr["c"] < pr["d"]) {
		t.Errorf("PageRank should grow along the call chain, got %v", pr)
	}
}

func TestSelfLoopsAndDuplicatesIgnored(t *testing.T) {
	g := graphOf([2]string{"a", "a"}, [2]string{"a", "b"}, [2]string{"a", "b"})
	if es := g.Edges(); len(es) != 1 || es[0] != [2]string{"a", "b"} {
		t.Errorf("edges = %v, want [[a b]]", es)
	}
}

func TestDemandEntropy(t *testing.T) {
	near(t, "empty", DemandEntropy(nil), 0)
	near(t, "constant", DemandEntropy([]float64{1, 1, 1, 1}), 0)
	near(t, "one per bin", DemandEntropy([]float64{0.05, 0.15, 0.25, 0.35, 0.45, 0.55, 0.65, 0.75, 0.85, 0.95}), 1)
	// numpy puts 0.3 below the edge 3*0.1 = 0.30000000000000004, so 0.3 and 0.29 share a bin.
	near(t, "edge values", DemandEntropy([]float64{0.3, 0.29}), 0)
	// Two values in one bin and five alone: (2/7 log2 3.5 + 5/7 log2 7) / log2 10.
	want := (2.0/7*math.Log2(3.5) + 5.0/7*math.Log2(7)) / math.Log2(10)
	near(t, "mixed", DemandEntropy([]float64{0.3, 0.3, 0.6, 0.7, 1.0, 0.0, 0.1}), want)
	near(t, "clipped", DemandEntropy([]float64{-2, 5}), 1/math.Log2(10))
}

func TestCompositeCentralityIsScaledToOne(t *testing.T) {
	c := CompositeCentrality(fan())
	top := 0.0
	for _, v := range c {
		top = math.Max(top, v)
	}
	near(t, "max", top, 1)
}

func TestRankUsesWeightsAndOrdersByScore(t *testing.T) {
	g := fan()
	kinds := map[string]string{"gw": "entry", "x": "service", "y": "service", "z": "service", "db": "backend", "cache": "backend"}
	rows := Rank(g, kinds, nil, nil, DefaultWeights)
	for i := 1; i < len(rows); i++ {
		if rows[i].Score > rows[i-1].Score {
			t.Fatalf("rows not sorted at %d: %v", i, rows)
		}
	}
	cen := CompositeCentrality(g)
	for _, r := range rows {
		near(t, "score of "+r.Name, r.Score, 0.40*cen[r.Name])
	}

	// A declared criticality of 1 on cache adds gamma = 0.25.
	rows = Rank(g, kinds, nil, map[string]float64{"cache": 1}, DefaultWeights)
	for _, r := range rows {
		if r.Name == "cache" {
			near(t, "cache score", r.Score, 0.40*cen["cache"]+0.25)
			if r.Kind != "backend" {
				t.Errorf("cache kind = %q", r.Kind)
			}
		}
	}
}

func TestClassify(t *testing.T) {
	for score, want := range map[float64]string{0.8: "critical", 0.75: "critical", 0.5: "elevated", 0.3: "moderate", 0.1: "low"} {
		if got := Classify(score); got != want {
			t.Errorf("Classify(%v) = %q, want %q", score, got, want)
		}
	}
}
