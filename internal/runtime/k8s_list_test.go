package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// newTestK8sRuntime builds a K8sRuntime backed by the given test server URL,
// bypassing the COO CRD probe. This allows unit testing of ListWorkspaces in
// isolation from a real cluster.
func newTestK8sRuntime(t *testing.T, serverURL string, ns string) *K8sRuntime {
	t.Helper()
	restCfg := &rest.Config{Host: serverURL}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}
	return &K8sRuntime{
		cfg:           Config{Namespace: ns},
		dynamicClient: dynClient,
	}
}

// listWorkspacesResponse is the JSON the test server returns for list requests.
const listWorkspacesResponse = `{
	"apiVersion": "coo.itsacoo.com/v1alpha1",
	"kind": "COOWorkspaceList",
	"metadata": {},
	"items": [
		{
			"apiVersion": "coo.itsacoo.com/v1alpha1",
			"kind": "COOWorkspace",
			"metadata": {"name": "ws-111", "namespace": "coo-system"},
			"spec": {"mode": "freestyle", "repo": "owner/repo", "ttl": "4h"},
			"status": {"phase": "Ready", "podName": "ws-111-pod"}
		},
		{
			"apiVersion": "coo.itsacoo.com/v1alpha1",
			"kind": "COOWorkspace",
			"metadata": {"name": "ws-222", "namespace": "coo-system"},
			"spec": {"mode": "handoff", "repo": "owner/other", "ttl": "8h"},
			"status": {"phase": "Terminated", "podName": "ws-222-pod"}
		},
		{
			"apiVersion": "coo.itsacoo.com/v1alpha1",
			"kind": "COOWorkspace",
			"metadata": {"name": "ws-333", "namespace": "coo-system"},
			"spec": {"mode": "freestyle", "repo": "owner/third"},
			"status": {"phase": "Pending"}
		}
	]
}`

// TestK8sRuntime_ListWorkspaces_FiltersTerminated verifies that workspaces in
// the Terminated phase are excluded from the returned list.
func TestK8sRuntime_ListWorkspaces_FiltersTerminated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis/coo.itsacoo.com/v1alpha1/namespaces/coo-system/cooworkspaces" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(listWorkspacesResponse))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL, "coo-system")
	got, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces returned unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 non-terminated workspaces, got %d", len(got))
	}

	for _, ws := range got {
		if ws.Phase == "Terminated" {
			t.Errorf("terminated workspace %q should have been filtered out", ws.Name)
		}
	}
}

// TestK8sRuntime_ListWorkspaces_MapsFields verifies that workspace fields are
// correctly mapped from the unstructured COOWorkspace CR.
func TestK8sRuntime_ListWorkspaces_MapsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis/coo.itsacoo.com/v1alpha1/namespaces/coo-system/cooworkspaces" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(listWorkspacesResponse))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL, "coo-system")
	got, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces returned unexpected error: %v", err)
	}

	// Find ws-111 (the Ready one).
	var found *WorkspaceInfo
	for i, ws := range got {
		if ws.Name == "ws-111" {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected ws-111 in results")
	}

	if found.Mode != "freestyle" {
		t.Errorf("Mode = %q, want %q", found.Mode, "freestyle")
	}
	if found.Phase != "Ready" {
		t.Errorf("Phase = %q, want %q", found.Phase, "Ready")
	}
	if found.PodName != "ws-111-pod" {
		t.Errorf("PodName = %q, want %q", found.PodName, "ws-111-pod")
	}
	if found.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", found.Repo, "owner/repo")
	}
	if found.TTLExpiry != "4h" {
		t.Errorf("TTLExpiry = %q, want %q", found.TTLExpiry, "4h")
	}
}

// TestK8sRuntime_ListWorkspaces_Empty verifies that an empty COOWorkspaceList
// returns a nil slice without error.
func TestK8sRuntime_ListWorkspaces_Empty(t *testing.T) {
	const emptyList = `{"apiVersion":"coo.itsacoo.com/v1alpha1","kind":"COOWorkspaceList","metadata":{},"items":[]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis/coo.itsacoo.com/v1alpha1/namespaces/coo-system/cooworkspaces" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(emptyList))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := newTestK8sRuntime(t, srv.URL, "coo-system")
	got, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces returned unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d entries", len(got))
	}
}

// TestK8sRuntime_ListWorkspaces_DefaultNamespace verifies that when cfg.Namespace
// is empty, the default "coo-system" namespace is used.
func TestK8sRuntime_ListWorkspaces_DefaultNamespace(t *testing.T) {
	const emptyList = `{"apiVersion":"coo.itsacoo.com/v1alpha1","kind":"COOWorkspaceList","metadata":{},"items":[]}`
	var requestedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyList))
	}))
	defer srv.Close()

	// Pass empty namespace to trigger default fallback.
	rt := newTestK8sRuntime(t, srv.URL, "")
	_, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces returned unexpected error: %v", err)
	}

	want := "/apis/coo.itsacoo.com/v1alpha1/namespaces/coo-system/cooworkspaces"
	if requestedPath != want {
		t.Errorf("requested path = %q, want %q", requestedPath, want)
	}
}
