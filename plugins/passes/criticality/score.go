package criticality

import (
	"math"
	"sort"
)

// Weights of the three terms of the dependency-criticality score.
type Weights struct {
	Centrality float64 // alpha
	Entropy    float64 // beta
	Governance float64 // gamma
}

// DefaultWeights are the published defaults: 0.40, 0.35, 0.25.
var DefaultWeights = Weights{Centrality: 0.40, Entropy: 0.35, Governance: 0.25}

// Component is one row of the criticality report.
type Component struct {
	Name               string  `json:"name"`
	Kind               string  `json:"kind"` // "service", "entry" (a service nothing calls) or "backend"
	Score              float64 `json:"score"`
	Class              string  `json:"class"`
	Centrality         float64 `json:"centrality"`
	Entropy            float64 `json:"entropy"`
	Governance         float64 `json:"governance"`
	Betweenness        float64 `json:"betweenness"`
	Degree             float64 `json:"degree"`
	PageRank           float64 `json:"pagerank"`
	Closeness          float64 `json:"closeness"`
	FanOut             float64 `json:"fan_out"`
	DownstreamReach    float64 `json:"downstream_reach"`
	UpstreamDependents float64 `json:"upstream_dependents"`
}

// CompositeCentrality is 0.35 PageRank + 0.30 betweenness + 0.20 degree + 0.15 closeness,
// divided by its maximum over the graph so it lies in [0, 1].
func CompositeCentrality(g *Graph) map[string]float64 {
	pr := g.PageRank(0.85, 100, 1e-6)
	bw := g.BetweennessCentrality()
	dg := g.DegreeCentrality()
	cl := g.ClosenessCentrality()
	out := map[string]float64{}
	top := 0.0
	for _, n := range g.Nodes() {
		out[n] = 0.35*pr[n] + 0.30*bw[n] + 0.20*dg[n] + 0.15*cl[n]
		top = math.Max(top, out[n])
	}
	if top > 0 {
		for n := range out {
			out[n] /= top
		}
	}
	return out
}

// DemandEntropy is the Shannon entropy of a demand history with values in [0, 1],
// over 10 equal bins, divided by log2(10). An empty history scores 0.
func DemandEntropy(history []float64) float64 {
	const bins = 10
	counts := make([]float64, bins)
	total := 0.0
	// Bin edges are k*0.1 for k < 10 and exactly 1 for the last, the same edges as
	// numpy.histogram(range=(0, 1), bins=10), so values on an edge land in the same bin.
	edge := func(k int) float64 {
		if k == bins {
			return 1
		}
		return float64(k) * (1.0 / bins)
	}
	for _, v := range history {
		v = math.Min(1, math.Max(0, v))
		b := int(v * bins)
		if b >= bins {
			b = bins - 1
		}
		if v < edge(b) {
			b--
		} else if b < bins-1 && v >= edge(b+1) {
			b++
		}
		counts[b]++
		total++
	}
	if total == 0 {
		return 0
	}
	h := 0.0
	for _, c := range counts {
		if c > 0 {
			p := c / total
			h -= p * math.Log2(p)
		}
	}
	return math.Min(1, h/math.Log2(bins))
}

// Classify maps a score to critical (>= 0.75), elevated (>= 0.50), moderate (>= 0.25) or low.
func Classify(score float64) string {
	switch {
	case score >= 0.75:
		return "critical"
	case score >= 0.50:
		return "elevated"
	case score >= 0.25:
		return "moderate"
	}
	return "low"
}

// Rank scores every node of g and returns the rows sorted by score, highest first.
// kinds gives each node's kind, histories its demand history and governance its
// operator-declared criticality in [0, 1]. Missing entries count as zero.
func Rank(g *Graph, kinds map[string]string, histories map[string][]float64, governance map[string]float64, w Weights) []Component {
	cen := CompositeCentrality(g)
	bw := g.BetweennessCentrality()
	dg := g.DegreeCentrality()
	pr := g.PageRank(0.85, 100, 1e-6)
	cl := g.ClosenessCentrality()
	fo := g.FanOut()
	down := g.DownstreamReach()
	up := g.UpstreamDependents()
	var rows []Component
	for _, n := range g.Nodes() {
		h := DemandEntropy(histories[n])
		r := math.Min(1, math.Max(0, governance[n]))
		s := w.Centrality*cen[n] + w.Entropy*h + w.Governance*r
		rows = append(rows, Component{
			Name: n, Kind: kinds[n], Score: s, Class: Classify(s),
			Centrality: cen[n], Entropy: h, Governance: r,
			Betweenness: bw[n], Degree: dg[n], PageRank: pr[n], Closeness: cl[n],
			FanOut: fo[n], DownstreamReach: down[n], UpstreamDependents: up[n],
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}
