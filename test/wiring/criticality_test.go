package wiring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/blueprint-uservices/blueprint/plugins/goproc"
	"github.com/blueprint-uservices/blueprint/plugins/grpc"
	"github.com/blueprint-uservices/blueprint/plugins/passes/criticality"
	"github.com/blueprint-uservices/blueprint/plugins/simple"
	"github.com/blueprint-uservices/blueprint/plugins/workflow"
	"github.com/blueprint-uservices/blueprint/test/workflow/nosqldb"
	wf "github.com/blueprint-uservices/blueprint/test/workflow/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Tests that the criticality pass recovers the call graph from the IR, whether services
are plain workflow instances or deployed over gRPC in separate processes.
*/

func TestCriticalityCallGraphWithBackends(t *testing.T) {
	spec := newWiringSpec("TestCriticalityCallGraphWithBackends")

	leaf_cache := simple.Cache(spec, "leaf_cache")
	leaf_db := simple.NoSQLDB(spec, "leaf_db")
	leaf := workflow.Service[*nosqldb.TestLeafServiceImplWithDB](spec, "leaf", leaf_cache, leaf_db)
	nonleaf := workflow.Service[wf.TestNonLeafService](spec, "nonleaf", leaf)

	app := assertBuildSuccess(t, spec, leaf, leaf_db, nonleaf)

	g, kinds := criticality.CallGraph(app)
	assert.Equal(t, [][2]string{{"leaf", "leaf_cache"}, {"leaf", "leaf_db"}, {"nonleaf", "leaf"}}, g.Edges())
	assert.Equal(t, map[string]string{"nonleaf": "entry", "leaf": "service", "leaf_cache": "backend", "leaf_db": "backend"}, kinds)
}

func TestCriticalityPassOverGRPCProcesses(t *testing.T) {
	spec := newWiringSpec("TestCriticalityPassOverGRPCProcesses")

	leaf := workflow.Service[*wf.TestLeafServiceImpl](spec, "leaf")
	nonleaf := workflow.Service[wf.TestNonLeafService](spec, "nonleaf", leaf)
	grpc.Deploy(spec, leaf)
	grpc.Deploy(spec, nonleaf)
	leafproc := goproc.CreateProcess(spec, "leafproc", leaf)
	nonleafproc := goproc.CreateProcess(spec, "nonleafproc", nonleaf)

	app := assertBuildSuccess(t, spec, leafproc, nonleafproc)

	out := filepath.Join(t.TempDir(), "criticality.json")
	pass := criticality.NewCriticalityPass(criticality.Options{OutputPath: out})
	modified, err := pass.Analyze(spec, app)
	require.NoError(t, err)
	assert.False(t, modified, "the pass must not modify the IR")

	report := pass.Report()
	require.NotNil(t, report)
	assert.Equal(t, [][2]string{{"nonleaf", "leaf"}}, report.Edges)
	require.Len(t, report.Components, 2)

	// leaf is called by nonleaf, so it carries the higher centrality and one upstream dependent.
	assert.Equal(t, "leaf", report.Components[0].Name)
	assert.Equal(t, 1.0, report.Components[0].UpstreamDependents)
	assert.Equal(t, "entry", report.Components[1].Kind)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	var fromFile criticality.Report
	require.NoError(t, json.Unmarshal(data, &fromFile))
	assert.Equal(t, report.Components, fromFile.Components)
}
