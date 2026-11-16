package criticality

import (
	"math"
	"sort"
)

// Graph is a small directed graph of application components.
// An edge a -> b means component a calls (depends on) component b.
type Graph struct {
	nodes []string
	index map[string]int
	out   []map[int]bool
	in    []map[int]bool
}

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{index: map[string]int{}}
}

// AddNode adds a node if it is not already present and returns its index.
func (g *Graph) AddNode(name string) int {
	if i, ok := g.index[name]; ok {
		return i
	}
	g.index[name] = len(g.nodes)
	g.nodes = append(g.nodes, name)
	g.out = append(g.out, map[int]bool{})
	g.in = append(g.in, map[int]bool{})
	return len(g.nodes) - 1
}

// AddEdge adds the edge from -> to, adding either node if needed. Self loops are ignored.
func (g *Graph) AddEdge(from, to string) {
	a, b := g.AddNode(from), g.AddNode(to)
	if a == b {
		return
	}
	g.out[a][b] = true
	g.in[b][a] = true
}

// Nodes returns the node names in insertion order.
func (g *Graph) Nodes() []string { return append([]string(nil), g.nodes...) }

// Edges returns every edge as a [from, to] pair, sorted.
func (g *Graph) Edges() [][2]string {
	var es [][2]string
	for a := range g.nodes {
		for b := range g.out[a] {
			es = append(es, [2]string{g.nodes[a], g.nodes[b]})
		}
	}
	sort.Slice(es, func(i, j int) bool {
		if es[i][0] != es[j][0] {
			return es[i][0] < es[j][0]
		}
		return es[i][1] < es[j][1]
	})
	return es
}

func (g *Graph) sortedOut(a int) []int {
	s := make([]int, 0, len(g.out[a]))
	for b := range g.out[a] {
		s = append(s, b)
	}
	sort.Ints(s)
	return s
}

func (g *Graph) sortedIn(a int) []int {
	s := make([]int, 0, len(g.in[a]))
	for b := range g.in[a] {
		s = append(s, b)
	}
	sort.Ints(s)
	return s
}

func (g *Graph) byName(v []float64) map[string]float64 {
	m := make(map[string]float64, len(v))
	for i, x := range v {
		m[g.nodes[i]] = x
	}
	return m
}

// DegreeCentrality is (in-degree + out-degree) / (n - 1), as in networkx for directed graphs.
func (g *Graph) DegreeCentrality() map[string]float64 {
	n := len(g.nodes)
	v := make([]float64, n)
	if n <= 1 {
		return g.byName(v)
	}
	for i := range g.nodes {
		v[i] = float64(len(g.in[i])+len(g.out[i])) / float64(n-1)
	}
	return g.byName(v)
}

// FanOut is the out-degree of each node.
func (g *Graph) FanOut() map[string]float64 {
	v := make([]float64, len(g.nodes))
	for i := range g.nodes {
		v[i] = float64(len(g.out[i]))
	}
	return g.byName(v)
}

// bfs returns hop distances from src following out-edges (forward) or in-edges.
func (g *Graph) bfs(src int, forward bool) []int {
	dist := make([]int, len(g.nodes))
	for i := range dist {
		dist[i] = -1
	}
	dist[src] = 0
	queue := []int{src}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		var next []int
		if forward {
			next = g.sortedOut(u)
		} else {
			next = g.sortedIn(u)
		}
		for _, w := range next {
			if dist[w] < 0 {
				dist[w] = dist[u] + 1
				queue = append(queue, w)
			}
		}
	}
	return dist
}

// DownstreamReach counts the components each node depends on, directly or transitively.
func (g *Graph) DownstreamReach() map[string]float64 {
	v := make([]float64, len(g.nodes))
	for i := range g.nodes {
		for j, d := range g.bfs(i, true) {
			if j != i && d > 0 {
				v[i]++
			}
		}
	}
	return g.byName(v)
}

// UpstreamDependents counts the components that depend on each node, directly or
// transitively. This is the static blast radius of the node.
func (g *Graph) UpstreamDependents() map[string]float64 {
	v := make([]float64, len(g.nodes))
	for i := range g.nodes {
		for j, d := range g.bfs(i, false) {
			if j != i && d > 0 {
				v[i]++
			}
		}
	}
	return g.byName(v)
}

// ClosenessCentrality uses incoming distances with the Wasserman-Faust scaling, as in
// networkx.closeness_centrality for directed graphs.
func (g *Graph) ClosenessCentrality() map[string]float64 {
	n := len(g.nodes)
	v := make([]float64, n)
	for i := range g.nodes {
		reach, total := 0, 0
		for _, d := range g.bfs(i, false) {
			if d >= 0 {
				reach++
				total += d
			}
		}
		if total > 0 && n > 1 {
			v[i] = float64(reach-1) / float64(total) * float64(reach-1) / float64(n-1)
		}
	}
	return g.byName(v)
}

// BetweennessCentrality is Brandes' algorithm for unweighted directed graphs,
// normalised by 1 / ((n-1)(n-2)) as in networkx.
func (g *Graph) BetweennessCentrality() map[string]float64 {
	n := len(g.nodes)
	cb := make([]float64, n)
	for s := 0; s < n; s++ {
		var stack []int
		pred := make([][]int, n)
		sigma := make([]float64, n)
		dist := make([]int, n)
		for i := range dist {
			dist[i] = -1
		}
		sigma[s], dist[s] = 1, 0
		queue := []int{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)
			for _, w := range g.sortedOut(v) {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}
		delta := make([]float64, n)
		for len(stack) > 0 {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, v := range pred[w] {
				delta[v] += sigma[v] / sigma[w] * (1 + delta[w])
			}
			if w != s {
				cb[w] += delta[w]
			}
		}
	}
	if n > 2 {
		scale := 1 / float64((n-1)*(n-2))
		for i := range cb {
			cb[i] *= scale
		}
	}
	return g.byName(cb)
}

// PageRank with damping alpha, uniform teleport and dangling mass spread uniformly,
// iterated until the L1 change is below n*tol, as in networkx.pagerank.
func (g *Graph) PageRank(alpha float64, maxIter int, tol float64) map[string]float64 {
	n := len(g.nodes)
	if n == 0 {
		return map[string]float64{}
	}
	x := make([]float64, n)
	for i := range x {
		x[i] = 1 / float64(n)
	}
	for iter := 0; iter < maxIter; iter++ {
		next := make([]float64, n)
		dangling := 0.0
		for u := 0; u < n; u++ {
			k := len(g.out[u])
			if k == 0 {
				dangling += x[u]
				continue
			}
			share := x[u] / float64(k)
			for w := range g.out[u] {
				next[w] += share
			}
		}
		errSum := 0.0
		for i := range next {
			next[i] = alpha*(next[i]+dangling/float64(n)) + (1-alpha)/float64(n)
			errSum += math.Abs(next[i] - x[i])
		}
		x = next
		if errSum < float64(n)*tol {
			break
		}
	}
	return g.byName(x)
}
