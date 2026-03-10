package handoff

import (
	"strings"
	"testing"
)

// minimalData returns a HandoffData with the required fields populated so the
// template can be rendered without panicking on missing values.
func minimalData() *HandoffData {
	return &HandoffData{
		ConceptName:    "my-concept",
		RawConcept:     "Build a test-page feature for the marketing site.",
		ConceptPhase:   "Planned",
		ComplexityTier: "M",
		Repo:           "owner/repo",
	}
}

// TestRender_ContainsHeader verifies that the rendered output starts with the
// expected COO handoff warning header.
func TestRender_ContainsHeader(t *testing.T) {
	out, err := Render(minimalData())
	if err != nil {
		t.Fatalf("Render returned unexpected error: %v", err)
	}
	if !strings.Contains(out, "⚠️ COO HANDOFF WORKSPACE") {
		t.Errorf("rendered output missing header; got:\n%s", out)
	}
}

// TestRender_ContainsConceptName verifies concept metadata appears in the output.
func TestRender_ContainsConceptName(t *testing.T) {
	out, err := Render(minimalData())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "my-concept") {
		t.Errorf("concept name not found in rendered output")
	}
	if !strings.Contains(out, "owner/repo") {
		t.Errorf("repo not found in rendered output")
	}
	if !strings.Contains(out, "Build a test-page feature") {
		t.Errorf("rawConcept not found in rendered output")
	}
}

// TestRender_PlannedPhase verifies the "What You Should Do" section for the
// Planned phase mentions the planning PR review.
func TestRender_PlannedPhase(t *testing.T) {
	data := minimalData()
	data.ConceptPhase = "Planned"

	out, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "planning PR") {
		t.Errorf("expected 'planning PR' in Planned phase output; got:\n%s", out)
	}
}

// TestRender_ExecutingPhase verifies the "What You Should Do" section for the
// Executing phase mentions open tasks.
func TestRender_ExecutingPhase(t *testing.T) {
	data := minimalData()
	data.ConceptPhase = "Executing"

	out, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "open") {
		t.Errorf("expected 'open' tasks guidance in Executing phase output; got:\n%s", out)
	}
}

// TestRender_TaskIcons verifies ✅ for completed tasks and ⏳ for others.
func TestRender_TaskIcons(t *testing.T) {
	data := minimalData()
	data.Tasks = []TaskInfo{
		{Name: "task-done", Phase: "Completed", Worker: "w1", Priority: "high"},
		{Name: "task-open", Phase: "InProgress", Worker: "w2", Priority: "medium"},
	}

	out, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "✅") {
		t.Errorf("expected ✅ for completed task; got:\n%s", out)
	}
	if !strings.Contains(out, "⏳") {
		t.Errorf("expected ⏳ for in-progress task; got:\n%s", out)
	}
}

// TestRender_Sprints verifies that sprints are rendered in a table.
func TestRender_Sprints(t *testing.T) {
	data := minimalData()
	data.Sprints = []SprintInfo{
		{Name: "sprint-1", Phase: "Completed", Iteration: 1, SprintType: "feature"},
		{Name: "sprint-2", Phase: "Executing", Iteration: 2, SprintType: "feature"},
	}

	out, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "sprint-1") {
		t.Errorf("sprint-1 not found in rendered output")
	}
	if !strings.Contains(out, "sprint-2") {
		t.Errorf("sprint-2 not found in rendered output")
	}
}

// TestRender_ArtifactPaths verifies planning artifact paths appear in the table.
func TestRender_ArtifactPaths(t *testing.T) {
	data := minimalData()
	data.Artifacts = ArtifactPaths{
		PRD:   "docs/PRD.md",
		HLD:   "docs/HLD.md",
		LLD:   "docs/LLD.md",
		Epic:  "docs/epic.yaml",
		Tasks: "docs/tasks.yaml",
	}
	data.PlanningPRURL = "https://github.com/owner/repo/pull/42"
	data.PlanningPRNumber = 42

	out, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	for _, expected := range []string{"docs/PRD.md", "docs/HLD.md", "docs/LLD.md", "#42"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in rendered output; got:\n%s", expected, out)
		}
	}
}

// TestRender_EmptyLists verifies the "not found" fallback messages for empty slices.
func TestRender_EmptyLists(t *testing.T) {
	out, err := Render(minimalData())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, msg := range []string{
		"No sprints found.",
		"No features found.",
		"No tasks found.",
		"No workers found.",
	} {
		if !strings.Contains(out, msg) {
			t.Errorf("expected %q for empty list; got:\n%s", msg, out)
		}
	}
}

// TestRender_Workers verifies the worker roster table.
func TestRender_Workers(t *testing.T) {
	data := minimalData()
	data.Workers = []WorkerInfo{
		{Name: "worker-backend", AgentType: "backend-engineer", Phase: "Active"},
	}

	out, err := Render(data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "worker-backend") {
		t.Errorf("worker name not found in rendered output")
	}
	if !strings.Contains(out, "backend-engineer") {
		t.Errorf("agent type not found in rendered output")
	}
}

// TestTaskIcon verifies the taskIcon helper directly.
func TestTaskIcon(t *testing.T) {
	cases := []struct {
		phase string
		want  string
	}{
		{"Completed", "✅"},
		{"Done", "✅"},
		{"Merged", "✅"},
		{"InProgress", "⏳"},
		{"Pending", "⏳"},
		{"", "⏳"},
	}
	for _, c := range cases {
		got := taskIcon(c.phase)
		if got != c.want {
			t.Errorf("taskIcon(%q) = %q, want %q", c.phase, got, c.want)
		}
	}
}
