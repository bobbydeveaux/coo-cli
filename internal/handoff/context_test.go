package handoff

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// fakeObject builds a minimal unstructured object suitable for use as a fake
// API server response item.
func fakeObject(apiVersion, kind, name, namespace string, spec, status map[string]interface{}) map[string]interface{} {
	obj := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}
	if spec != nil {
		obj["spec"] = spec
	}
	if status != nil {
		obj["status"] = status
	}
	return obj
}

// fakeList wraps items into a list response that the dynamic client can decode.
func fakeList(apiVersion, kind string, items []map[string]interface{}) map[string]interface{} {
	raw := make([]interface{}, len(items))
	for i, item := range items {
		raw[i] = item
	}
	return map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind + "List",
		"metadata":   map[string]interface{}{"resourceVersion": ""},
		"items":      raw,
	}
}

// mustJSON serialises v, fatally failing on error.
func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestFetchHandoffData_HappyPath verifies that FetchHandoffData correctly
// populates HandoffData from a fake Kubernetes API server.
func TestFetchHandoffData_HappyPath(t *testing.T) {
	const (
		systemNS    = "coo-system"
		conceptName = "test-concept"
		conceptNS   = "coo-test-concept"
	)

	concept := fakeObject("coo.itsacoo.com/v1alpha1", "COOConcept", conceptName, systemNS,
		map[string]interface{}{
			"rawConcept":       "Build a login page.",
			"affectedProjects": []interface{}{"owner/my-repo"},
		},
		map[string]interface{}{
			"phase": "Executing",
			"complexityAssessment": map[string]interface{}{
				"tier": "M",
			},
		},
	)

	plan := fakeObject("coo.itsacoo.com/v1alpha1", "COOPlan", "test-concept-plan", conceptNS,
		map[string]interface{}{
			"artifacts": map[string]interface{}{
				"prd":   "docs/PRD.md",
				"hld":   "docs/HLD.md",
				"lld":   "docs/LLD.md",
				"epic":  "docs/epic.yaml",
				"tasks": "docs/tasks.yaml",
			},
		},
		map[string]interface{}{
			"planningPRURL":    "https://github.com/owner/repo/pull/10",
			"planningPRNumber": int64(10),
			"epicCount":        int64(1),
			"featureCount":     int64(3),
			"issueCount":       int64(6),
		},
	)

	sprint := fakeObject("coo.itsacoo.com/v1alpha1", "COOSprint", "sprint-1", conceptNS,
		map[string]interface{}{"type": "feature"},
		map[string]interface{}{"phase": "Executing", "iteration": int64(1)},
	)

	feature := fakeObject("coo.itsacoo.com/v1alpha1", "COOFeature", "feat-login", conceptNS,
		nil,
		map[string]interface{}{"phase": "InProgress"},
	)

	task := fakeObject("coo.itsacoo.com/v1alpha1", "COOTask", "task-1", conceptNS,
		map[string]interface{}{"worker": "backend-worker", "priority": "high"},
		map[string]interface{}{"phase": "Pending", "prNumber": int64(0)},
	)

	worker := fakeObject("coo.itsacoo.com/v1alpha1", "COOWorker", "backend-worker", conceptNS,
		map[string]interface{}{"agentType": "backend-engineer"},
		map[string]interface{}{"phase": "Active"},
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case strings.Contains(path, "cooconcepts/"+conceptName):
			_, _ = w.Write(mustJSON(t, concept))
		case strings.Contains(path, "cooplans"):
			_, _ = w.Write(mustJSON(t, fakeList("coo.itsacoo.com/v1alpha1", "COOPlan", []map[string]interface{}{plan})))
		case strings.Contains(path, "coosprints"):
			_, _ = w.Write(mustJSON(t, fakeList("coo.itsacoo.com/v1alpha1", "COOSprint", []map[string]interface{}{sprint})))
		case strings.Contains(path, "coofeatures"):
			_, _ = w.Write(mustJSON(t, fakeList("coo.itsacoo.com/v1alpha1", "COOFeature", []map[string]interface{}{feature})))
		case strings.Contains(path, "cootasks"):
			_, _ = w.Write(mustJSON(t, fakeList("coo.itsacoo.com/v1alpha1", "COOTask", []map[string]interface{}{task})))
		case strings.Contains(path, "cooworkers"):
			_, _ = w.Write(mustJSON(t, fakeList("coo.itsacoo.com/v1alpha1", "COOWorker", []map[string]interface{}{worker})))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dynClient, err := dynamic.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	data, err := FetchHandoffData(context.Background(), dynClient, systemNS, conceptName)
	if err != nil {
		t.Fatalf("FetchHandoffData returned unexpected error: %v", err)
	}

	// Concept fields.
	if data.ConceptName != conceptName {
		t.Errorf("ConceptName = %q, want %q", data.ConceptName, conceptName)
	}
	if data.RawConcept != "Build a login page." {
		t.Errorf("RawConcept = %q", data.RawConcept)
	}
	if data.ConceptPhase != "Executing" {
		t.Errorf("ConceptPhase = %q, want Executing", data.ConceptPhase)
	}
	if data.ComplexityTier != "M" {
		t.Errorf("ComplexityTier = %q, want M", data.ComplexityTier)
	}
	if data.Repo != "owner/my-repo" {
		t.Errorf("Repo = %q, want owner/my-repo", data.Repo)
	}

	// Plan fields.
	if data.Artifacts.PRD != "docs/PRD.md" {
		t.Errorf("Artifacts.PRD = %q, want docs/PRD.md", data.Artifacts.PRD)
	}
	if data.PlanningPRURL != "https://github.com/owner/repo/pull/10" {
		t.Errorf("PlanningPRURL = %q", data.PlanningPRURL)
	}
	if data.FeatureCount != 3 {
		t.Errorf("FeatureCount = %d, want 3", data.FeatureCount)
	}

	// Sprints.
	if len(data.Sprints) != 1 {
		t.Fatalf("len(Sprints) = %d, want 1", len(data.Sprints))
	}
	if data.Sprints[0].Name != "sprint-1" || data.Sprints[0].Phase != "Executing" {
		t.Errorf("Sprints[0] = %+v", data.Sprints[0])
	}

	// Features.
	if len(data.Features) != 1 || data.Features[0].Name != "feat-login" {
		t.Errorf("Features = %+v", data.Features)
	}

	// Tasks.
	if len(data.Tasks) != 1 || data.Tasks[0].Worker != "backend-worker" {
		t.Errorf("Tasks = %+v", data.Tasks)
	}

	// Workers.
	if len(data.Workers) != 1 || data.Workers[0].AgentType != "backend-engineer" {
		t.Errorf("Workers = %+v", data.Workers)
	}
}

// TestFetchHandoffData_MissingConcept verifies that a missing COOConcept
// returns an error (it is a required resource).
func TestFetchHandoffData_MissingConcept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dynClient, _ := dynamic.NewForConfig(&rest.Config{Host: srv.URL})

	_, err := FetchHandoffData(context.Background(), dynClient, "coo-system", "missing-concept")
	if err == nil {
		t.Fatal("expected error for missing COOConcept, got nil")
	}
	if !strings.Contains(err.Error(), "COOConcept") {
		t.Errorf("error should mention COOConcept, got: %v", err)
	}
}

// TestFetchHandoffData_PartialData verifies that missing plan/sprints/etc. are
// silently tolerated — only the concept is required.
func TestFetchHandoffData_PartialData(t *testing.T) {
	const conceptName = "partial-concept"
	concept := fakeObject("coo.itsacoo.com/v1alpha1", "COOConcept", conceptName, "coo-system",
		map[string]interface{}{"rawConcept": "Partial concept."},
		map[string]interface{}{"phase": "Planned"},
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "cooconcepts/"+conceptName) {
			_, _ = w.Write(mustJSON(t, concept))
			return
		}
		// Return 404 for everything else (plan, sprints, etc.).
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dynClient, _ := dynamic.NewForConfig(&rest.Config{Host: srv.URL})

	data, err := FetchHandoffData(context.Background(), dynClient, "coo-system", conceptName)
	if err != nil {
		t.Fatalf("FetchHandoffData should not fail with partial data: %v", err)
	}
	if data.ConceptPhase != "Planned" {
		t.Errorf("ConceptPhase = %q, want Planned", data.ConceptPhase)
	}
	if len(data.Sprints) != 0 || len(data.Tasks) != 0 {
		t.Errorf("expected empty slices for missing resources")
	}
}

// TestKubectlExecArgs verifies the helper builds the expected argument order.
func TestKubectlExecArgs(t *testing.T) {
	cases := []struct {
		name       string
		kubeconfig string
		kubeCtx    string
		wantPrefix []string
	}{
		{
			name:       "no flags",
			wantPrefix: []string{"exec", "-i", "my-pod"},
		},
		{
			name:       "kubeconfig only",
			kubeconfig: "/home/user/.kube/config",
			wantPrefix: []string{"--kubeconfig", "/home/user/.kube/config", "exec"},
		},
		{
			name:    "context only",
			kubeCtx: "my-ctx",
			wantPrefix: []string{"--context", "my-ctx", "exec"},
		},
		{
			name:       "both flags",
			kubeconfig: "/tmp/kc",
			kubeCtx:    "ctx1",
			// context is prepended last so it appears first.
			wantPrefix: []string{"--context", "ctx1", "--kubeconfig", "/tmp/kc", "exec"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := kubectlExecArgs("my-pod", "coo-system", tc.kubeconfig, tc.kubeCtx, "bash", "-c", "echo hi")
			for i, want := range tc.wantPrefix {
				if i >= len(args) {
					t.Fatalf("args too short: %v", args)
				}
				if args[i] != want {
					t.Errorf("args[%d] = %q, want %q (full args: %v)", i, args[i], want, args)
				}
			}
		})
	}
}
