package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopoSort_NoDependencies(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a"},
		{ID: "b", AgentID: "agent-b"},
		{ID: "c", AgentID: "agent-c"},
	}
	waves, err := topoSortSubtasks(subtasks)
	require.NoError(t, err)
	require.Len(t, waves, 1, "all independent tasks should be in a single wave")
	assert.Len(t, waves[0], 3)
}

func TestTopoSort_LinearChain(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a"},
		{ID: "b", AgentID: "agent-b", DependsOn: []string{"a"}},
		{ID: "c", AgentID: "agent-c", DependsOn: []string{"b"}},
	}
	waves, err := topoSortSubtasks(subtasks)
	require.NoError(t, err)
	require.Len(t, waves, 3, "linear chain should produce 3 waves")
	assert.Equal(t, []string{"a"}, waves[0])
	assert.Equal(t, []string{"b"}, waves[1])
	assert.Equal(t, []string{"c"}, waves[2])
}

func TestTopoSort_DiamondDependency(t *testing.T) {
	// a -> b, a -> c, b -> d, c -> d
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a"},
		{ID: "b", AgentID: "agent-b", DependsOn: []string{"a"}},
		{ID: "c", AgentID: "agent-c", DependsOn: []string{"a"}},
		{ID: "d", AgentID: "agent-d", DependsOn: []string{"b", "c"}},
	}
	waves, err := topoSortSubtasks(subtasks)
	require.NoError(t, err)
	require.Len(t, waves, 3, "diamond dependency should produce 3 waves")

	assert.Equal(t, []string{"a"}, waves[0])

	// Wave 1 should contain b and c (order doesn't matter)
	assert.Len(t, waves[1], 2)
	assert.ElementsMatch(t, []string{"b", "c"}, waves[1])

	assert.Equal(t, []string{"d"}, waves[2])
}

func TestTopoSort_CycleDetection(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a", DependsOn: []string{"c"}},
		{ID: "b", AgentID: "agent-b", DependsOn: []string{"a"}},
		{ID: "c", AgentID: "agent-c", DependsOn: []string{"b"}},
	}
	_, err := topoSortSubtasks(subtasks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestTopoSort_SelfDependency(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a", DependsOn: []string{"a"}},
	}
	_, err := topoSortSubtasks(subtasks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestTopoSort_UnknownDependency(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a", DependsOn: []string{"nonexistent"}},
	}
	_, err := topoSortSubtasks(subtasks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task")
}

func TestTopoSort_DuplicateID(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a"},
		{ID: "a", AgentID: "agent-b"},
	}
	_, err := topoSortSubtasks(subtasks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestTopoSort_Empty(t *testing.T) {
	waves, err := topoSortSubtasks(nil)
	require.NoError(t, err)
	assert.Empty(t, waves)
}

func TestTopoSort_PartialDependencies(t *testing.T) {
	// a and b are independent; c depends on a only; d depends on b and c
	subtasks := []OrchestrationSubtask{
		{ID: "a", AgentID: "agent-a"},
		{ID: "b", AgentID: "agent-b"},
		{ID: "c", AgentID: "agent-c", DependsOn: []string{"a"}},
		{ID: "d", AgentID: "agent-d", DependsOn: []string{"b", "c"}},
	}
	waves, err := topoSortSubtasks(subtasks)
	require.NoError(t, err)
	require.Len(t, waves, 3)

	// Wave 0: a and b (no deps)
	assert.Len(t, waves[0], 2)
	assert.ElementsMatch(t, []string{"a", "b"}, waves[0])

	// Wave 1: c (depends on a, which is in wave 0)
	assert.Equal(t, []string{"c"}, waves[1])

	// Wave 2: d (depends on b from wave 0 and c from wave 1)
	assert.Equal(t, []string{"d"}, waves[2])
}

func TestCascadeSkip(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", Status: "failed"},
		{ID: "b", Status: "pending", DependsOn: []string{"a"}},
		{ID: "c", Status: "pending", DependsOn: []string{"b"}},
		{ID: "d", Status: "pending"},
	}

	taskByID := make(map[string]*OrchestrationSubtask, len(subtasks))
	for i := range subtasks {
		taskByID[subtasks[i].ID] = &subtasks[i]
	}
	deps := buildDependentsMap(subtasks)

	cascadeSkip(taskByID, "a", deps)

	assert.Equal(t, "skipped", taskByID["b"].Status, "direct dependent should be skipped")
	assert.Equal(t, "skipped", taskByID["c"].Status, "transitive dependent should be skipped")
	assert.Equal(t, "pending", taskByID["d"].Status, "unrelated task should remain pending")
}

func TestCascadeSkip_AlreadySkippedNotReprocessed(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a", Status: "failed"},
		{ID: "b", Status: "skipped", DependsOn: []string{"a"}},
		{ID: "c", Status: "pending", DependsOn: []string{"b"}},
	}

	taskByID := make(map[string]*OrchestrationSubtask, len(subtasks))
	for i := range subtasks {
		taskByID[subtasks[i].ID] = &subtasks[i]
	}
	deps := buildDependentsMap(subtasks)

	cascadeSkip(taskByID, "a", deps)

	// b was already skipped, so c should remain pending (cascade stops at already-skipped nodes)
	assert.Equal(t, "skipped", taskByID["b"].Status)
	assert.Equal(t, "pending", taskByID["c"].Status,
		"cascade should stop at already-skipped nodes to avoid reprocessing")
}

func TestBuildDependentsMap(t *testing.T) {
	subtasks := []OrchestrationSubtask{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}
	deps := buildDependentsMap(subtasks)

	assert.ElementsMatch(t, []string{"b", "c"}, deps["a"])
	assert.Equal(t, []string{"d"}, deps["b"])
	assert.Equal(t, []string{"d"}, deps["c"])
	assert.Empty(t, deps["d"])
}

func TestTopoSort_WideGraph(t *testing.T) {
	// One root task with 5 independent children
	subtasks := []OrchestrationSubtask{
		{ID: "root", AgentID: "agent-root"},
		{ID: "c1", AgentID: "agent-1", DependsOn: []string{"root"}},
		{ID: "c2", AgentID: "agent-2", DependsOn: []string{"root"}},
		{ID: "c3", AgentID: "agent-3", DependsOn: []string{"root"}},
		{ID: "c4", AgentID: "agent-4", DependsOn: []string{"root"}},
		{ID: "c5", AgentID: "agent-5", DependsOn: []string{"root"}},
	}
	waves, err := topoSortSubtasks(subtasks)
	require.NoError(t, err)
	require.Len(t, waves, 2)
	assert.Equal(t, []string{"root"}, waves[0])
	assert.Len(t, waves[1], 5)
}
