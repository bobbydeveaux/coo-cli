package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// newTestK8sRuntime builds a K8sRuntime pointed at the given test server.
func newTestK8sRuntime(t *testing.T, serverURL string) *K8sRuntime {
	t.Helper()
	restCfg := &rest.Config{Host: serverURL}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}
	return &K8sRuntime{
		dynamicClient: dynClient,
		restConfig:    restCfg,
		namespace:     defaultNamespace,
	}
}

// mustMarshal serialises v to JSON, fatally failing the test on error.
func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// unstructuredWorkspace builds a minimal COOWorkspace object for test responses.
func unstructuredWorkspace(name, phase, podName string) map[string]interface{} {
	obj := map[string]interface{}{
		"apiVersion": "coo.itsacoo.com/v1alpha1",
		"kind":       "COOWorkspace",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": defaultNamespace,
		},
		"spec": map[string]interface{}{
			"mode":  "freestyle",
			"repo":  "owner/repo",
			"model": "claude-sonnet-4-5",
			"ttl":   "4h",
		},
	}
	if phase != "" || podName != "" {
		status := map[string]interface{}{}
		if phase != "" {
			status["phase"] = phase
		}
		if podName != "" {
			status["podName"] = podName
		}
		obj["status"] = status
	}
	return obj
}

// TestBuildCOOWorkspace verifies the CR spec is assembled correctly.
func TestBuildCOOWorkspace(t *testing.T) {
	opts := CreateOptions{
		Repo:  "owner/repo",
		Model: "claude-sonnet-4-5",
		TTL:   "4h",
	}
	ws := buildCOOWorkspace("ws-test", defaultNamespace, "freestyle", opts, defaultWorkerImage)

	if ws.GetName() != "ws-test" {
		t.Errorf("name = %q, want ws-test", ws.GetName())
	}
	if ws.GetNamespace() != defaultNamespace {
		t.Errorf("namespace = %q, want %s", ws.GetNamespace(), defaultNamespace)
	}

	mode, _, _ := unstructured.NestedString(ws.Object, "spec", "mode")
	if mode != "freestyle" {
		t.Errorf("spec.mode = %q, want freestyle", mode)
	}

	repo, _, _ := unstructured.NestedString(ws.Object, "spec", "repo")
	if repo != "owner/repo" {
		t.Errorf("spec.repo = %q, want owner/repo", repo)
	}

	image, _, _ := unstructured.NestedString(ws.Object, "spec", "image")
	if image != defaultWorkerImage {
		t.Errorf("spec.image = %q, want %s", image, defaultWorkerImage)
	}

	policy, _, _ := unstructured.NestedString(ws.Object, "spec", "imagePullPolicy")
	if policy != "IfNotPresent" {
		t.Errorf("spec.imagePullPolicy = %q, want IfNotPresent", policy)
	}
}

// TestBuildCOOWorkspace_Handoff verifies handoff mode sets conceptRef.
func TestBuildCOOWorkspace_Handoff(t *testing.T) {
	opts := CreateOptions{Concept: "my-concept"}
	ws := buildCOOWorkspace("ws-test", defaultNamespace, "handoff", opts, defaultWorkerImage)

	mode, _, _ := unstructured.NestedString(ws.Object, "spec", "mode")
	if mode != "handoff" {
		t.Errorf("spec.mode = %q, want handoff", mode)
	}

	ref, _, _ := unstructured.NestedString(ws.Object, "spec", "conceptRef")
	if ref != "my-concept" {
		t.Errorf("spec.conceptRef = %q, want my-concept", ref)
	}
}

// TestBuildCOOWorkspace_Defaults verifies empty model/ttl get default values.
func TestBuildCOOWorkspace_Defaults(t *testing.T) {
	opts := CreateOptions{Repo: "owner/repo"} // no model or TTL
	ws := buildCOOWorkspace("ws-test", defaultNamespace, "freestyle", opts, defaultWorkerImage)

	model, _, _ := unstructured.NestedString(ws.Object, "spec", "model")
	if model != "claude-sonnet-4-5" {
		t.Errorf("spec.model = %q, want claude-sonnet-4-5", model)
	}

	ttl, _, _ := unstructured.NestedString(ws.Object, "spec", "ttl")
	if ttl != "4h" {
		t.Errorf("spec.ttl = %q, want 4h", ttl)
	}
}

// TestWaitForReady_ImmediateReady verifies that waitForReady returns immediately
// when the workspace is already in Ready phase on the first poll.
func TestWaitForReady_ImmediateReady(t *testing.T) {
	wsObj := unstructuredWorkspace("ws-test", "Ready", "pod-abc123")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ws-test") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(t, wsObj))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL)
	// Use a very short poll interval for the test.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	podName, err := rt.waitForReadyWithInterval(ctx, "ws-test", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForReady returned unexpected error: %v", err)
	}
	if podName != "pod-abc123" {
		t.Errorf("podName = %q, want pod-abc123", podName)
	}
}

// TestWaitForReady_EventuallyReady verifies the poll loop keeps going until Ready.
func TestWaitForReady_EventuallyReady(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ws-test") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			count := atomic.AddInt32(&callCount, 1)
			phase := "Pending"
			podName := ""
			if count >= 3 {
				phase = "Ready"
				podName = "pod-xyz"
			}
			_, _ = w.Write(mustMarshal(t, unstructuredWorkspace("ws-test", phase, podName)))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	podName, err := rt.waitForReadyWithInterval(ctx, "ws-test", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForReady returned unexpected error: %v", err)
	}
	if podName != "pod-xyz" {
		t.Errorf("podName = %q, want pod-xyz", podName)
	}
	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 polls, got %d", callCount)
	}
}

// TestWaitForReady_ContextCancelled verifies cancellation is respected.
func TestWaitForReady_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return Pending.
		if strings.Contains(r.URL.Path, "ws-test") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(t, unstructuredWorkspace("ws-test", "Pending", "")))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately.
	cancel()

	_, err := rt.waitForReadyWithInterval(ctx, "ws-test", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// TestWaitForReady_FailedPhase verifies that a Failed phase returns an error.
func TestWaitForReady_FailedPhase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ws-test") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(t, unstructuredWorkspace("ws-test", "Failed", "")))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rt.waitForReadyWithInterval(ctx, "ws-test", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for Failed phase")
	}
	if !strings.Contains(err.Error(), "Failed") {
		t.Errorf("error should mention 'Failed', got: %v", err)
	}
}

// TestCreateWorkspace_MissingRepoAndConcept verifies validation.
func TestCreateWorkspace_MissingRepoAndConcept(t *testing.T) {
	rt := &K8sRuntime{namespace: defaultNamespace}
	_, err := rt.CreateWorkspace(context.Background(), CreateOptions{})
	if err == nil {
		t.Fatal("expected error when neither --repo nor --concept is provided")
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Errorf("error should mention --repo, got: %v", err)
	}
}

// TestListActiveWorkspaces verifies filtering out Terminated workspaces.
func TestListActiveWorkspaces(t *testing.T) {
	items := []map[string]interface{}{
		unstructuredWorkspace("ws-running", "Ready", "pod-1"),
		unstructuredWorkspace("ws-pending", "Pending", ""),
		unstructuredWorkspace("ws-done", "Terminated", ""),
		unstructuredWorkspace("ws-uninit", "", ""), // phase not set yet
	}
	listResp := map[string]interface{}{
		"apiVersion": "coo.itsacoo.com/v1alpha1",
		"kind":       "COOWorkspaceList",
		"metadata":   map[string]interface{}{"resourceVersion": ""},
		"items":      items,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "cooworkspaces") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(t, listResp))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL)
	active, err := rt.listActiveWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("listActiveWorkspaces returned unexpected error: %v", err)
	}

	// Terminated and uninitialised workspaces should be excluded.
	if len(active) != 2 {
		t.Errorf("got %d active workspaces, want 2", len(active))
	}
	for _, ws := range active {
		name := ws.GetName()
		if name == "ws-done" || name == "ws-uninit" {
			t.Errorf("unexpected workspace in active list: %s", name)
		}
	}
}
