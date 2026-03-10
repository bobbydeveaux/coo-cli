package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// newTestClient builds a Client pointed at the given test server URL.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	restCfg := &rest.Config{Host: serverURL}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}
	discClient, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		t.Fatalf("create discovery client: %v", err)
	}
	return &Client{
		RestConfig: restCfg,
		Dynamic:    dynClient,
		Discovery:  discClient,
		Clientset:  fake.NewSimpleClientset(),
	}
}

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"major":"1","minor":"31","gitVersion":"v1.31.1"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.Ping(); err != nil {
		t.Fatalf("Ping returned unexpected error: %v", err)
	}
}

func TestPing_Unreachable(t *testing.T) {
	// Point at a port that is not listening.
	c := newTestClient(t, "http://127.0.0.1:1")
	if err := c.Ping(); err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

func TestHasCOOCRD_Present(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis/coo.itsacoo.com/v1alpha1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"kind":"APIResourceList",
				"apiVersion":"v1",
				"groupVersion":"coo.itsacoo.com/v1alpha1",
				"resources":[
					{"name":"cooworkspaces","singularName":"cooworkspace","namespaced":true,"kind":"COOWorkspace","verbs":["get","list","watch"]}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ok, err := c.HasCOOCRD(context.Background())
	if err != nil {
		t.Fatalf("HasCOOCRD returned unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected HasCOOCRD to return true when CRD is present")
	}
}

func TestHasCOOCRD_Missing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for everything — simulates cluster without COO installed.
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ok, err := c.HasCOOCRD(context.Background())
	if err != nil {
		t.Fatalf("HasCOOCRD returned unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected HasCOOCRD to return false when CRD is absent")
	}
}

func TestIsNotFoundErr(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"nil", "", false},
		{"not found lowercase", "the server could not find the requested resource", true},
		{"404 phrasing", "not found", true},
		{"unrelated error", "connection refused", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.msg != "" {
				err = &errString{tc.msg}
			}
			if got := isNotFoundErr(err); got != tc.want {
				t.Errorf("isNotFoundErr(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

// errString is a minimal error implementation for testing.
type errString struct{ s string }

func (e *errString) Error() string { return e.s }
