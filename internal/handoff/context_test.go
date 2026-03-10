package handoff

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// newTestInjector builds an Injector pointed at a test HTTP server.
func newTestInjector(t *testing.T, serverURL string) *Injector {
	t.Helper()
	restCfg := &rest.Config{Host: serverURL}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}
	return &Injector{
		dynClient: dynClient,
		cfg:       ClientConfig{},
		namespace: cooSystem,
	}
}

func marshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// conceptObj returns a minimal COOConcept map suitable for JSON responses.
func conceptObj(name, phase, tier, rawConcept string, projects []string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
		"kind":       "COOConcept",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": cooSystem,
		},
		"spec": map[string]interface{}{
			"rawConcept":       rawConcept,
			"affectedProjects": toIfaceSlice(projects),
		},
		"status": map[string]interface{}{
			"phase": phase,
			"complexityAssessment": map[string]interface{}{
				"tier": tier,
			},
		},
	}
}

// planObj returns a minimal COOPlan map suitable for JSON responses.
func planObj(name, ns, prURL string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
		"kind":       "COOPlan",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"artifacts": map[string]interface{}{
				"prd": "docs/PRD.md",
			},
		},
		"status": map[string]interface{}{
			"planningPRURL": prURL,
			"epicCount":     int64(2),
		},
	}
}

// listObj wraps items in a Kubernetes list response envelope.
func listObj(kind string, items []map[string]interface{}) map[string]interface{} {
	ifaces := make([]interface{}, len(items))
	for i, v := range items {
		ifaces[i] = v
	}
	return map[string]interface{}{
		"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"resourceVersion": ""},
		"items":      ifaces,
	}
}

func toIfaceSlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// TestFetchContextSuccess verifies that FetchContext populates HandoffContext
// when all resources are returned by the fake API server.
func TestFetchContextSuccess(t *testing.T) {
	conceptName := "my-concept"
	conceptNS := "coo-" + conceptName

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		// GET /apis/coo.../coo-system/cooconcepts/my-concept
		case strings.Contains(path, "cooconcepts/"+conceptName):
			w.Write(marshalJSON(t, conceptObj(conceptName, "Executing", "L", "Build it", []string{"owner/repo"})))

		// LIST /apis/coo.../coo-concept/cooplans
		case strings.Contains(path, conceptNS) && strings.Contains(path, "cooplans"):
			w.Write(marshalJSON(t, listObj("COOPlanList", []map[string]interface{}{
				planObj("plan-1", conceptNS, "https://github.com/pr/1"),
			})))

		// LIST any other resource in the concept namespace
		case strings.Contains(path, conceptNS):
			w.Write(marshalJSON(t, listObj("List", nil)))

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	inj := newTestInjector(t, srv.URL)
	hc, err := inj.FetchContext(context.Background(), conceptName)
	if err != nil {
		t.Fatalf("FetchContext returned unexpected error: %v", err)
	}

	if hc.Concept == nil {
		t.Fatal("expected Concept to be non-nil")
	}
	if hc.Concept.GetName() != conceptName {
		t.Errorf("concept name = %q; want %q", hc.Concept.GetName(), conceptName)
	}
	if hc.Plan == nil {
		t.Fatal("expected Plan to be non-nil")
	}
	prURL, _, _ := unstructured.NestedString(hc.Plan.Object, "status", "planningPRURL")
	if prURL != "https://github.com/pr/1" {
		t.Errorf("planningPRURL = %q; want https://github.com/pr/1", prURL)
	}
}

// TestFetchContextConceptNotFound verifies that a 404 concept causes an error.
func TestFetchContextConceptNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	inj := newTestInjector(t, srv.URL)
	_, err := inj.FetchContext(context.Background(), "missing-concept")
	if err == nil {
		t.Fatal("expected error for missing concept, got nil")
	}
}

// TestFetchPlanEmpty verifies that an empty plan list results in nil Plan.
func TestFetchPlanEmpty(t *testing.T) {
	conceptName := "no-plan"
	conceptNS := "coo-" + conceptName

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "cooconcepts/"+conceptName):
			w.Write(marshalJSON(t, conceptObj(conceptName, "Planned", "M", "do something", nil)))
		case strings.Contains(path, conceptNS):
			w.Write(marshalJSON(t, listObj("List", nil)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	inj := newTestInjector(t, srv.URL)
	hc, err := inj.FetchContext(context.Background(), conceptName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hc.Plan != nil {
		t.Error("expected Plan to be nil when no COOPlan exists")
	}
}

// TestKubectlArgsWithFlags verifies kubeconfig/context are prepended.
func TestKubectlArgsWithFlags(t *testing.T) {
	inj := &Injector{
		cfg: ClientConfig{
			Kubeconfig:  "/home/user/.kube/config",
			KubeContext: "my-ctx",
		},
	}

	got := inj.kubectlArgs([]string{"exec", "my-pod"})
	want := []string{"--kubeconfig", "/home/user/.kube/config", "--context", "my-ctx", "exec", "my-pod"}

	if len(got) != len(want) {
		t.Fatalf("kubectlArgs len = %d; want %d\ngot: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("kubectlArgs[%d] = %q; want %q", i, got[i], v)
		}
	}
}

// TestKubectlArgsNoFlags verifies no extra args when config is empty.
func TestKubectlArgsNoFlags(t *testing.T) {
	inj := &Injector{cfg: ClientConfig{}}
	got := inj.kubectlArgs([]string{"exec", "my-pod"})
	if len(got) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(got), got)
	}
}

// TestGVRValues confirms the package-level GVR variables have the correct values.
func TestGVRValues(t *testing.T) {
	cases := []struct {
		name     string
		resource string
		want     string
	}{
		{"concept", conceptGVR.Resource, "cooconcepts"},
		{"plan", planGVR.Resource, "cooplans"},
		{"sprint", sprintGVR.Resource, "coosprints"},
		{"feature", featureGVR.Resource, "coofeatures"},
		{"task", taskGVR.Resource, "cootasks"},
		{"worker", workerGVR.Resource, "cooworkers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.resource != tc.want {
				t.Errorf("resource = %q; want %q", tc.resource, tc.want)
			}
		})
	}

	for _, gvr := range []string{
		conceptGVR.Group, planGVR.Group, sprintGVR.Group,
		featureGVR.Group, taskGVR.Group, workerGVR.Group,
	} {
		if gvr != cooAPIGroup {
			t.Errorf("GVR group = %q; want %q", gvr, cooAPIGroup)
		}
	}
}

// TestNewInjectorBadKubeconfig verifies NewInjector fails with a bad kubeconfig.
func TestNewInjectorBadKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/kubeconfig")
	t.Setenv("HOME", "/nonexistent")

	_, err := NewInjector(ClientConfig{Kubeconfig: "/nonexistent/kubeconfig"})
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig")
	}
}
