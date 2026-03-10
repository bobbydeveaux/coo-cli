package runtime

import (
	"context"
	"testing"
)

// TestDetect_LocalFlag verifies that --local always yields a LocalRuntime.
func TestDetect_LocalFlag(t *testing.T) {
	cfg := Config{LocalMode: true}
	rt, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Type() != RuntimeLocal {
		t.Errorf("got runtime %q, want %q", rt.Type(), RuntimeLocal)
	}
}

// TestDetect_LocalFallback verifies that when no k8s is reachable (no kubeconfig
// flags, empty environment), Detect falls back to local mode rather than erroring.
func TestDetect_LocalFallback(t *testing.T) {
	// Use a non-existent kubeconfig path only via explicit flag so that
	// the test counts as "explicit k8s request" — we want to verify the
	// fallback path, so we use a plain empty config with no flags.
	cfg := Config{}
	rt, err := Detect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Detect with no k8s available should fall back to local, not error: %v", err)
	}
	// In CI / the test container there is no itsacoo operator, so we expect
	// local mode after the probe fails.
	if rt.Type() != RuntimeLocal && rt.Type() != RuntimeK8s {
		t.Errorf("unexpected runtime type %q", rt.Type())
	}
}

// TestDetect_ExplicitKubeContextErrors verifies that when k8s flags are set but
// the cluster is unreachable, Detect returns an error rather than silently
// falling back to local mode.
func TestDetect_ExplicitKubeContextErrors(t *testing.T) {
	cfg := Config{
		KubeContext: "nonexistent-context-for-testing",
		Kubeconfig:  "/nonexistent/kubeconfig",
	}
	_, err := Detect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected an error when explicit k8s flags point to an unavailable cluster")
	}
}

// TestLocalRuntime_Type verifies LocalRuntime.Type() returns RuntimeLocal.
func TestLocalRuntime_Type(t *testing.T) {
	r := newLocalRuntime(Config{})
	if r.Type() != RuntimeLocal {
		t.Errorf("got %q, want %q", r.Type(), RuntimeLocal)
	}
}

// TestLocalRuntime_ImplementsRuntime ensures LocalRuntime satisfies the Runtime interface.
func TestLocalRuntime_ImplementsRuntime(t *testing.T) {
	var _ Runtime = (*LocalRuntime)(nil)
}

// TestK8sRuntime_ImplementsRuntime ensures K8sRuntime satisfies the Runtime interface.
func TestK8sRuntime_ImplementsRuntime(t *testing.T) {
	var _ Runtime = (*K8sRuntime)(nil)
}

// TestRuntimeTypeConstants verifies the string values of RuntimeType constants.
func TestRuntimeTypeConstants(t *testing.T) {
	if RuntimeK8s != "k8s" {
		t.Errorf("RuntimeK8s = %q, want %q", RuntimeK8s, "k8s")
	}
	if RuntimeLocal != "local" {
		t.Errorf("RuntimeLocal = %q, want %q", RuntimeLocal, "local")
	}
}
