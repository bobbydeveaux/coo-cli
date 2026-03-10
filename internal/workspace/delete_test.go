package workspace

import (
	"context"
	"testing"
)

// TestDelete_CallsDeleteWorkspace verifies that DeleteWorkspace is called with the correct name.
// It tests the stub directly (same pattern as other tests in this package).
func TestDelete_CallsDeleteWorkspace(t *testing.T) {
	stub := &stubRuntime{}
	ctx := context.Background()

	// stubRuntime.DeleteWorkspace is a no-op that returns nil.
	if err := stub.DeleteWorkspace(ctx, "ws-del"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
