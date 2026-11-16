# criticality

```go
import "github.com/blueprint-uservices/blueprint/plugins/passes/criticality"
```

Package criticality is an analysis pass that ranks the components of a Blueprint
application by how much of the application depends on them. It needs no running
system: it reads the call graph from the compiled IR.

## What it computes

The pass builds a directed graph with one node per workflow service and per backend
given to a service, and an edge from each service to every service or backend passed
to its constructor. For every node it reports:

- `betweenness`, `degree`, `pagerank` and `closeness` (the same definitions as networkx
  for directed graphs),
- `fan_out` (direct dependencies), `downstream_reach` (everything the node depends on)
  and `upstream_dependents` (everything that depends on the node: its static blast radius),
- a combined score `alpha*C + beta*H + gamma*R`. C is a composite centrality
  (0.35 PageRank + 0.30 betweenness + 0.20 degree + 0.15 closeness, divided by the
  maximum), H is the normalised entropy of an optional demand history, and R is an
  optional operator-declared criticality in [0, 1]. The default weights (0.40, 0.35,
  0.25) are those of the Centrality-Entropy Index (Pokharel, FTC 2026). Without
  histories or declared criticality, the score orders components exactly as C does.

Components are marked `entry` (a service nothing else calls), `service` or `backend`.

## Usage

Register the pass in a wiring main:

```go
passes := []analysis.IRAnalysisPass{
	criticality.NewCriticalityPass(criticality.Options{OutputPath: "criticality.json"}),
}
cmdbuilder.MakeAndExecuteWithPasses(name, passes, specs.Docker)
```

The pass logs the top components and, if `OutputPath` is set, writes the full report
as JSON (`components`, sorted by score, and `edges`). Demand histories
(`Options.Histories`) and declared criticality (`Options.Governance`) are optional.

The pass never modifies the IR.

## Caveats

- The graph comes from constructor arguments, so it shows which components a service
  *can* call, not how often each call happens. A dependency used by one rare request
  counts the same as one used by every request.
- Components are named by their instance name in the wiring spec. An argument such as
  `user_db.client` belongs to `user_db`, so instance names should not contain dots.
