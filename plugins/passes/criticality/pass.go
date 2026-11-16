// Package criticality provides an analysis pass that ranks the components of a
// Blueprint application by how much of the application depends on them.
//
// The pass reads the compiled IR, builds the call graph (service -> the services
// and backends passed to its constructor), and scores every component with a
// dependency-criticality score:
//
//	score = alpha * C + beta * H + gamma * R
//
// C is a composite centrality on the call graph (0.35 PageRank + 0.30 betweenness
// + 0.20 degree + 0.15 closeness, divided by its maximum). H is the normalised
// Shannon entropy of the component's demand history, if one is supplied. R is an
// operator-declared criticality in [0, 1], if one is supplied. The defaults
// (0.40, 0.35, 0.25) are those of the Centrality-Entropy Index (Pokharel, FTC 2026).
// With no demand histories or declared criticality the score is the scaled
// centrality alone, so the report also lists the plain graph measures (betweenness,
// degree, PageRank, closeness, fan-out, downstream reach and upstream dependents)
// for comparison.
//
// Usage, in a wiring main:
//
//	passes := []analysis.IRAnalysisPass{criticality.NewCriticalityPass(criticality.Options{OutputPath: "criticality.json"})}
//	cmdbuilder.MakeAndExecuteWithPasses(name, passes, specs.Docker)
package criticality

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/analysis"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/ir"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
	"github.com/blueprint-uservices/blueprint/plugins/workflow"
)

// Options configure the pass. The zero value is valid.
type Options struct {
	// If non-empty, the report is written to this path as JSON.
	OutputPath string
	// Score weights. The zero value means DefaultWeights.
	Weights Weights
	// Optional demand history per component (values in [0, 1], for example
	// requests per time bin divided by the peak bin).
	Histories map[string][]float64
	// Optional operator-declared criticality per component, in [0, 1].
	Governance map[string]float64
}

// Report is the output of the pass.
type Report struct {
	Application string      `json:"application"`
	Weights     Weights     `json:"weights"`
	Components  []Component `json:"components"`
	Edges       [][2]string `json:"edges"`
}

// CriticalityPass implements analysis.IRAnalysisPass.
type CriticalityPass struct {
	opts   Options
	report *Report
}

// NewCriticalityPass returns the pass with the given options.
func NewCriticalityPass(opts Options) *CriticalityPass {
	if opts.Weights == (Weights{}) {
		opts.Weights = DefaultWeights
	}
	return &CriticalityPass{opts: opts}
}

var _ analysis.IRAnalysisPass = (*CriticalityPass)(nil)

// Name implements analysis.IRAnalysisPass.
func (p *CriticalityPass) Name() string { return "CriticalityPass" }

// ImplementsAnalysisPass implements analysis.IRAnalysisPass.
func (p *CriticalityPass) ImplementsAnalysisPass() {}

// Report returns the report from the last call to Analyze, or nil.
func (p *CriticalityPass) Report() *Report { return p.report }

// Analyze implements analysis.IRAnalysisPass. It never modifies the IR.
func (p *CriticalityPass) Analyze(spec wiring.WiringSpec, app *ir.ApplicationNode) (bool, error) {
	g, kinds := CallGraph(app)
	rows := Rank(g, kinds, p.opts.Histories, p.opts.Governance, p.opts.Weights)
	p.report = &Report{Application: app.Name(), Weights: p.opts.Weights, Components: rows, Edges: g.Edges()}

	for i, r := range rows {
		if i == 10 {
			break
		}
		slog.Info(fmt.Sprintf("[%v] %2d. %-28s %-8s score=%.3f betweenness=%.3f upstream=%v",
			p.Name(), i+1, r.Name, r.Kind, r.Score, r.Betweenness, r.UpstreamDependents))
	}
	if p.opts.OutputPath != "" {
		data, err := json.MarshalIndent(p.report, "", "  ")
		if err != nil {
			return false, err
		}
		if err := os.WriteFile(p.opts.OutputPath, data, 0o644); err != nil {
			return false, err
		}
		slog.Info(fmt.Sprintf("[%v] wrote %d components to %v", p.Name(), len(rows), p.opts.OutputPath))
	}
	return false, nil
}

// CallGraph builds the component graph of an application: one node per workflow
// service and per backend it is given, and an edge from each service to every
// service or backend passed to its constructor. Hard-coded constructor values are
// skipped. A component is named by its instance name in the wiring spec: an
// argument such as "user_db.client" belongs to "user_db".
//
// kinds maps each node to "entry" (a service no other service calls), "service"
// or "backend".
func CallGraph(app *ir.ApplicationNode) (*Graph, map[string]string) {
	g := NewGraph()
	kinds := map[string]string{}
	handlers := workflow.FilterWorkflowNodes(app.GetAllIRNodes())
	for _, h := range handlers {
		g.AddNode(h.Name())
		kinds[h.Name()] = "service"
	}
	for _, h := range handlers {
		for _, arg := range h.Args {
			if _, isValue := arg.(*ir.IRValue); isValue || arg == nil {
				continue
			}
			dep := componentName(arg.Name())
			g.AddEdge(h.Name(), dep)
			if _, known := kinds[dep]; !known {
				kinds[dep] = "backend"
			}
		}
	}
	for _, n := range g.Nodes() {
		if kinds[n] == "service" && len(g.in[g.index[n]]) == 0 {
			kinds[n] = "entry"
		}
	}
	return g, kinds
}

func componentName(argName string) string {
	return strings.SplitN(argName, ".", 2)[0]
}
