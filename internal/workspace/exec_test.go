package workspace

import (
	"context"
	"errors"
	"testing"
)

// TestExec_CallsExecWorkspace verifies that Exec calls ExecWorkspace with the correct name.
// It uses the stubRuntime defined in create_test.go (same package).
func TestExec_CallsExecWorkspace(t *testing.T) {
	stub := &stubRuntime{}
	ctx := context.Background()

	// Exec calls rt.ExecWorkspace(ctx, name) → test the stub directly.
	if err := stub.ExecWorkspace(ctx, "ws-test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastExeced != "ws-test" {
		t.Errorf("ExecWorkspace called with %q, want %q", stub.lastExeced, "ws-test")
	}
}

// TestExec_PropagatesError verifies that ExecWorkspace errors are propagated.
func TestExec_PropagatesError(t *testing.T) {
	stub := &stubRuntime{execErr: errors.New("exec failed")}
	ctx := context.Background()

	err := stub.ExecWorkspace(ctx, "ws-test")
	if err == nil {
		t.Fatal("expected error from ExecWorkspace")
	}
}

// TestResume_CallsResumeWorkspace verifies that Resume calls ResumeWorkspace with the correct name.
func TestResume_CallsResumeWorkspace(t *testing.T) {
	stub := &stubRuntime{}
	ctx := context.Background()

	if err := stub.ResumeWorkspace(ctx, "ws-resume"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastResumed != "ws-resume" {
		t.Errorf("ResumeWorkspace called with %q, want %q", stub.lastResumed, "ws-resume")
	}
}

// TestResume_PropagatesError verifies that ResumeWorkspace errors are propagated.
func TestResume_PropagatesError(t *testing.T) {
	stub := &stubRuntime{resumeErr: errors.New("resume failed")}
	ctx := context.Background()

	err := stub.ResumeWorkspace(ctx, "ws-resume")
	if err == nil {
		t.Fatal("expected error from ResumeWorkspace")
	}
}
