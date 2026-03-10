package handoff

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderTemplateMinimal(t *testing.T) {
	hc := &HandoffContext{
		Concept: makeConcept("my-concept", "Planned", "M", "Build an API", []string{"owner/repo"}),
	}

	out, err := RenderTemplate(hc)
	if err != nil {
		t.Fatalf("RenderTemplate error: %v", err)
	}

	mustContain(t, out, "COO HANDOFF WORKSPACE")
	mustContain(t, out, "my-concept")
	mustContain(t, out, "owner/repo")
	mustContain(t, out, "Build an API")
	mustContain(t, out, "Planned")
}

func TestRenderTemplateWithPlan(t *testing.T) {
	hc := &HandoffContext{
		Concept: makeConcept("test-concept", "Executing", "L", "Big project", []string{"acme/widget"}),
		Plan:    makePlan("plan-1", "coo-test-concept", "https://github.com/org/repo/pull/99", 3, 8, 20),
	}

	out, err := RenderTemplate(hc)
	if err != nil {
		t.Fatalf("RenderTemplate error: %v", err)
	}

	mustContain(t, out, "https://github.com/org/repo/pull/99")
	mustContain(t, out, "docs/PRD.md")
	mustContain(t, out, "docs/HLD.md")
	mustContain(t, out, "Executing")
}

func TestRenderTemplateWithTasks(t *testing.T) {
	ns := "coo-tasks-concept"
	hc := &HandoffContext{
		Concept: makeConcept("tasks-concept", "Executing", "M", "Task concept", nil),
		Tasks: []unstructured.Unstructured{
			*makeTask("task-done", ns, "worker-a", "high", "Completed", 7),
			*makeTask("task-open", ns, "worker-b", "low", "InProgress", 0),
		},
	}

	out, err := RenderTemplate(hc)
	if err != nil {
		t.Fatalf("RenderTemplate error: %v", err)
	}

	mustContain(t, out, "task-done")
	mustContain(t, out, "task-open")
	mustContain(t, out, "✅")
	mustContain(t, out, "⏳")
	mustContain(t, out, "#7")
}

func TestRenderTemplatePlannedPhaseGuidance(t *testing.T) {
	hc := &HandoffContext{
		Concept: makeConcept("planned-c", "Planned", "S", "Plan me", nil),
	}

	out, err := RenderTemplate(hc)
	if err != nil {
		t.Fatalf("RenderTemplate error: %v", err)
	}

	mustContain(t, out, "Review the planning PR")
}

func TestRenderTemplateExecutingPhaseGuidance(t *testing.T) {
	hc := &HandoffContext{
		Concept: makeConcept("exec-c", "Executing", "L", "Execute me", nil),
	}

	out, err := RenderTemplate(hc)
	if err != nil {
		t.Fatalf("RenderTemplate error: %v", err)
	}

	mustContain(t, out, "open tasks")
}

func TestBuildTemplateDataConcept(t *testing.T) {
	concept := makeConcept("my-c", "Executing", "XL", "build it", []string{"org/proj"})
	hc := &HandoffContext{Concept: concept}

	td := buildTemplateData(hc)

	if td.ConceptName != "my-c" {
		t.Errorf("ConceptName = %q; want %q", td.ConceptName, "my-c")
	}
	if td.Phase != "Executing" {
		t.Errorf("Phase = %q; want %q", td.Phase, "Executing")
	}
	if td.Tier != "XL" {
		t.Errorf("Tier = %q; want %q", td.Tier, "XL")
	}
	if td.Repo != "org/proj" {
		t.Errorf("Repo = %q; want %q", td.Repo, "org/proj")
	}
	if td.RawConcept != "build it" {
		t.Errorf("RawConcept = %q; want %q", td.RawConcept, "build it")
	}
}

func TestBuildTemplateDataNilConcept(t *testing.T) {
	hc := &HandoffContext{}
	td := buildTemplateData(hc)
	if td.ConceptName != "" {
		t.Errorf("ConceptName = %q; want empty", td.ConceptName)
	}
}

func TestBuildTemplateDataPlanCounts(t *testing.T) {
	hc := &HandoffContext{
		Plan: makePlan("p", "coo-x", "http://pr.url", 3, 10, 25),
	}
	td := buildTemplateData(hc)

	if td.EpicCount != 3 {
		t.Errorf("EpicCount = %d; want 3", td.EpicCount)
	}
	if td.FeatureCount != 10 {
		t.Errorf("FeatureCount = %d; want 10", td.FeatureCount)
	}
	if td.IssueCount != 25 {
		t.Errorf("IssueCount = %d; want 25", td.IssueCount)
	}
}

func mustContain(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("output missing %q\n\nFull output:\n%s", substr, s)
	}
}
