// Package handoff implements CLAUDE.md context injection for handoff-mode
// workspaces. It fetches operator CRD state (COOConcept, COOPlan, COOSprints,
// COOFeatures, COOTasks, COOWorkers) and renders them into a structured
// CLAUDE.md that is prepended to /workspace/CLAUDE.md inside the pod.
package handoff

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	cooAPIGroup      = "coo.itsacoo.com"
	cooAPIVersion    = "v1alpha1"
	workspaceContainer = "workspace"
)

var (
	cooConceptGVR = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cooconcepts"}
	cooPlanGVR    = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cooplans"}
	cooSprintGVR  = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "coosprints"}
	cooFeatureGVR = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "coofeatures"}
	cooTaskGVR    = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cootasks"}
	cooWorkerGVR  = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cooworkers"}
)

// ArtifactPaths holds file paths to the planning documents from COOPlan.spec.artifacts.
type ArtifactPaths struct {
	PRD   string
	HLD   string
	LLD   string
	Epic  string
	Tasks string
}

// SprintInfo is a summary of a single COOSprint.
type SprintInfo struct {
	Name       string
	Phase      string
	Iteration  int64
	SprintType string
}

// FeatureInfo is a summary of a single COOFeature.
type FeatureInfo struct {
	Name  string
	Phase string
}

// TaskInfo is a summary of a single COOTask.
type TaskInfo struct {
	Name     string
	Worker   string
	Priority string
	Phase    string
	PRNumber int64
}

// WorkerInfo is a summary of a single COOWorker.
type WorkerInfo struct {
	Name      string
	AgentType string
	Phase     string
}

// HandoffData is the complete data set passed to the CLAUDE.md template.
// It is populated by FetchHandoffData from the operator CRDs.
type HandoffData struct {
	// Concept information (from COOConcept in coo-system namespace).
	ConceptName     string
	RawConcept      string
	AffectedProjects []string
	ConceptPhase    string
	ComplexityTier  string
	Repo            string // first entry from AffectedProjects, if available

	// Plan information (from COOPlan in coo-<concept> namespace).
	Artifacts        ArtifactPaths
	PlanningPRURL    string
	PlanningPRNumber int64
	EpicCount        int64
	FeatureCount     int64
	IssueCount       int64

	// Workload summaries (from coo-<concept> namespace).
	Sprints  []SprintInfo
	Features []FeatureInfo
	Tasks    []TaskInfo
	Workers  []WorkerInfo
}

// extractInt64 retrieves an integer from an unstructured object field,
// handling both int64 (from typed serialisation) and float64 (from standard
// encoding/json — all JSON numbers decode as float64) gracefully.
func extractInt64(obj map[string]interface{}, fields ...string) int64 {
	val, found, _ := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int32:
		return int64(v)
	case int:
		return int64(v)
	}
	return 0
}

// FetchHandoffData queries the Kubernetes API for all CRD objects needed to
// render the handoff CLAUDE.md. systemNS is the namespace where COOConcept
// lives (typically "coo-system"); conceptName is the COOConcept name.
//
// Non-critical fetch errors (plan, sprints, features, tasks, workers) are
// silently swallowed so that partial data still produces a useful template
// rather than failing the entire workspace creation.
func FetchHandoffData(ctx context.Context, client dynamic.Interface, systemNS, conceptName string) (*HandoffData, error) {
	data := &HandoffData{ConceptName: conceptName}
	conceptNS := "coo-" + conceptName

	// COOConcept — required; fail the whole operation if missing.
	concept, err := client.Resource(cooConceptGVR).Namespace(systemNS).Get(ctx, conceptName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get COOConcept %s/%s: %w", systemNS, conceptName, err)
	}

	data.RawConcept, _, _ = unstructured.NestedString(concept.Object, "spec", "rawConcept")
	data.ConceptPhase, _, _ = unstructured.NestedString(concept.Object, "status", "phase")
	data.ComplexityTier, _, _ = unstructured.NestedString(concept.Object, "status", "complexityAssessment", "tier")
	data.AffectedProjects, _, _ = unstructured.NestedStringSlice(concept.Object, "spec", "affectedProjects")
	if len(data.AffectedProjects) > 0 {
		data.Repo = data.AffectedProjects[0]
	}

	// COOPlan — best-effort; take the first plan found in the concept namespace.
	plans, err := client.Resource(cooPlanGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err == nil && len(plans.Items) > 0 {
		plan := plans.Items[0]
		prd, _, _ := unstructured.NestedString(plan.Object, "spec", "artifacts", "prd")
		hld, _, _ := unstructured.NestedString(plan.Object, "spec", "artifacts", "hld")
		lld, _, _ := unstructured.NestedString(plan.Object, "spec", "artifacts", "lld")
		epic, _, _ := unstructured.NestedString(plan.Object, "spec", "artifacts", "epic")
		tasks, _, _ := unstructured.NestedString(plan.Object, "spec", "artifacts", "tasks")
		data.Artifacts = ArtifactPaths{PRD: prd, HLD: hld, LLD: lld, Epic: epic, Tasks: tasks}

		data.PlanningPRURL, _, _ = unstructured.NestedString(plan.Object, "status", "planningPRURL")
		data.PlanningPRNumber = extractInt64(plan.Object, "status", "planningPRNumber")
		data.EpicCount = extractInt64(plan.Object, "status", "epicCount")
		data.FeatureCount = extractInt64(plan.Object, "status", "featureCount")
		data.IssueCount = extractInt64(plan.Object, "status", "issueCount")
	}

	// COOSprints — best-effort.
	sprints, err := client.Resource(cooSprintGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range sprints.Items {
			phase, _, _ := unstructured.NestedString(s.Object, "status", "phase")
			iter := extractInt64(s.Object, "status", "iteration")
			sType, _, _ := unstructured.NestedString(s.Object, "spec", "type")
			data.Sprints = append(data.Sprints, SprintInfo{
				Name:       s.GetName(),
				Phase:      phase,
				Iteration:  iter,
				SprintType: sType,
			})
		}
	}

	// COOFeatures — best-effort.
	features, err := client.Resource(cooFeatureGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, f := range features.Items {
			phase, _, _ := unstructured.NestedString(f.Object, "status", "phase")
			data.Features = append(data.Features, FeatureInfo{
				Name:  f.GetName(),
				Phase: phase,
			})
		}
	}

	// COOTasks — best-effort.
	taskList, err := client.Resource(cooTaskGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, t := range taskList.Items {
			phase, _, _ := unstructured.NestedString(t.Object, "status", "phase")
			worker, _, _ := unstructured.NestedString(t.Object, "spec", "worker")
			priority, _, _ := unstructured.NestedString(t.Object, "spec", "priority")
			prNum := extractInt64(t.Object, "status", "prNumber")
			data.Tasks = append(data.Tasks, TaskInfo{
				Name:     t.GetName(),
				Worker:   worker,
				Priority: priority,
				Phase:    phase,
				PRNumber: prNum,
			})
		}
	}

	// COOWorkers — best-effort.
	workers, err := client.Resource(cooWorkerGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, w := range workers.Items {
			phase, _, _ := unstructured.NestedString(w.Object, "status", "phase")
			agentType, _, _ := unstructured.NestedString(w.Object, "spec", "agentType")
			data.Workers = append(data.Workers, WorkerInfo{
				Name:      w.GetName(),
				AgentType: agentType,
				Phase:     phase,
			})
		}
	}

	return data, nil
}

// InjectCLAUDEMD writes content to /workspace/CLAUDE.md inside the named pod.
//
// Injection flow:
//  1. Stream content to /tmp/coo-handoff.md via kubectl exec stdin.
//  2. Run a shell script that:
//     - Moves /workspace/CLAUDE.md → /workspace/CLAUDE.md.original (if present).
//     - Writes /tmp/coo-handoff.md as the new /workspace/CLAUDE.md.
//     - Appends a separator + historical-reference note + original content.
//     - Removes /tmp/coo-handoff.md.
func InjectCLAUDEMD(content, podName, namespace, kubeconfig, kubeContext string) error {
	// Step 1: stream the rendered content into a temp file in the pod.
	writeArgs := kubectlExecArgs(podName, namespace, kubeconfig, kubeContext,
		"bash", "-c", "cat > /tmp/coo-handoff.md",
	)
	writeCmd := exec.Command("kubectl", writeArgs...)
	writeCmd.Stdin = strings.NewReader(content)
	if out, err := writeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stream handoff content to pod: %w\n%s", err, string(out))
	}

	// Step 2: atomically merge the handoff front-matter with any existing CLAUDE.md.
	const mergeScript = `
set -e
if [ -f /workspace/CLAUDE.md ]; then
    mv /workspace/CLAUDE.md /workspace/CLAUDE.md.original
    cat /tmp/coo-handoff.md > /workspace/CLAUDE.md
    printf '\n---\n\n> **Note: Historical Reference Only** — The following is the original CLAUDE.md from the repository.\n\n' >> /workspace/CLAUDE.md
    cat /workspace/CLAUDE.md.original >> /workspace/CLAUDE.md
else
    cat /tmp/coo-handoff.md > /workspace/CLAUDE.md
fi
rm -f /tmp/coo-handoff.md
`
	mergeArgs := kubectlExecArgs(podName, namespace, kubeconfig, kubeContext,
		"bash", "-c", mergeScript,
	)
	mergeCmd := exec.Command("kubectl", mergeArgs...)
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("merge handoff CLAUDE.md in pod: %w\n%s", err, string(out))
	}

	return nil
}

// kubectlExecArgs builds the argument slice for a non-interactive kubectl exec
// call into the workspace container. Global flags (--kubeconfig, --context) are
// prepended when set.
func kubectlExecArgs(podName, namespace, kubeconfig, kubeContext string, command ...string) []string {
	args := []string{"exec", "-i", podName, "-n", namespace, "-c", workspaceContainer, "--"}
	args = append(args, command...)

	// Prepend global flags so they appear before the subcommand.
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	if kubeContext != "" {
		args = append([]string{"--context", kubeContext}, args...)
	}

	return args
}
