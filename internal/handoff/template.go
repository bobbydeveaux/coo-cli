package handoff

import (
	"bytes"
	"fmt"
	"text/template"
)

// taskIcon returns a status emoji for use in the task table.
func taskIcon(phase string) string {
	switch phase {
	case "Completed", "Done", "Merged":
		return "✅"
	default:
		return "⏳"
	}
}

var claudeMDTmpl = template.Must(
	template.New("handoff-claude-md").
		Funcs(template.FuncMap{"taskIcon": taskIcon}).
		Parse(rawTemplate),
)

// rawTemplate is the Go text/template source for the handoff CLAUDE.md header.
// It is separated from the function to keep claudeMDTmpl initialisation clean.
const rawTemplate = `# ⚠️ COO HANDOFF WORKSPACE — READ THIS SECTION FIRST

This workspace was pre-configured by the **itsacoo** (Code Orchestrator Operator) system.
Read this section carefully before taking any action in the repository.

---

## What is itsacoo?

**itsacoo** is a Kubernetes-native AI software development system. It orchestrates an AI
workforce — planners, architects, engineers, and reviewers — to build software end-to-end
from a high-level concept. The operator manages the full lifecycle: requirements gathering,
planning, sprint execution, PR creation, and review.

You are running inside a **handoff workspace** — a containerised environment where a
human (or AI) can review progress, pick up open tasks, or guide the system forward.

---

## Project

| Field              | Value |
|--------------------|-------|
| **Concept**        | {{.ConceptName}} |
{{- if .Repo}}
| **Repo**           | {{.Repo}} |
{{- end}}
| **Phase**          | {{.ConceptPhase}} |
| **Complexity Tier**| {{.ComplexityTier}} |

### Original Requirement

> {{.RawConcept}}

---

## Planning Artifacts
{{- if or .Artifacts.PRD .Artifacts.HLD .Artifacts.LLD .Artifacts.Epic .Artifacts.Tasks}}

| Artifact                        | Path |
|---------------------------------|------|
{{- if .Artifacts.PRD}}
| PRD (Product Requirements Doc)  | {{.Artifacts.PRD}} |
{{- end}}
{{- if .Artifacts.HLD}}
| HLD (High-Level Design)         | {{.Artifacts.HLD}} |
{{- end}}
{{- if .Artifacts.LLD}}
| LLD (Low-Level Design)          | {{.Artifacts.LLD}} |
{{- end}}
{{- if .Artifacts.Epic}}
| Epic                            | {{.Artifacts.Epic}} |
{{- end}}
{{- if .Artifacts.Tasks}}
| Tasks                           | {{.Artifacts.Tasks}} |
{{- end}}
{{- else}}

_No planning artifacts recorded yet._
{{- end}}
{{- if .PlanningPRURL}}

**Planning PR:** [#{{.PlanningPRNumber}}]({{.PlanningPRURL}})
{{- end}}

---

## Sprints
{{- if .Sprints}}

| Sprint | Type | Phase |
|--------|------|-------|
{{- range .Sprints}}
| {{.Name}} | {{.SprintType}} | {{.Phase}} |
{{- end}}
{{- else}}

_No sprints found._
{{- end}}

---

## Features
{{- if .Features}}

| Feature | Phase |
|---------|-------|
{{- range .Features}}
| {{.Name}} | {{.Phase}} |
{{- end}}
{{- else}}

_No features found._
{{- end}}

---

## Tasks
{{- if .Tasks}}

|   | Task | Worker | Priority | Phase |
|---|------|--------|----------|-------|
{{- range .Tasks}}
| {{taskIcon .Phase}} | {{.Name}} | {{.Worker}} | {{.Priority}} | {{.Phase}} |
{{- end}}
{{- else}}

_No tasks found._
{{- end}}

---

## Worker Roster
{{- if .Workers}}

| Worker | Agent Type | Phase |
|--------|------------|-------|
{{- range .Workers}}
| {{.Name}} | {{.AgentType}} | {{.Phase}} |
{{- end}}
{{- else}}

_No workers found._
{{- end}}

---

## What You Should Do
{{- if eq .ConceptPhase "Planned"}}

The project is currently in **Planned** phase — planning is complete and awaiting approval.

- Review the planning PR linked above and inspect all planning artifacts in the docs/ folder
- Check that any GitHub issues created during planning look correct
- When ready, approve the planning PR to trigger Sprint 1 execution
{{- else if eq .ConceptPhase "Executing"}}

The project is currently in **Executing** phase — sprint work is underway.

- Review the task list above and pick up any open (⏳) tasks
- Check worker PRs and provide review feedback where needed
- Ensure tests pass and code quality standards are met
- Use ` + "`coo workspace list`" + ` to see other active workspaces
{{- else}}

The project is currently in **{{.ConceptPhase}}** phase.

- Review the planning artifacts in the docs/ folder
- Check open GitHub issues to understand the current state
- Determine the appropriate next steps for this phase
{{- end}}

`

// Render executes the CLAUDE.md handoff template against data and returns the
// rendered string. An error is returned only if template execution fails, which
// should not happen with well-formed HandoffData.
func Render(data *HandoffData) (string, error) {
	var buf bytes.Buffer
	if err := claudeMDTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render handoff template: %w", err)
	}
	return buf.String(), nil
}
