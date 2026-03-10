package runtime

import (
	"context"
	"fmt"
)

// LocalRuntime implements Runtime using a local Docker/Podman daemon.
// It tracks workspaces in ~/.coo/workspaces.json and persists volumes in
// ~/.coo/volumes/<name>.
type LocalRuntime struct {
	cfg Config
}

// newLocalRuntime creates a LocalRuntime. No network calls are made; local mode
// is always available as long as Docker/Podman is installed.
func newLocalRuntime(cfg Config) *LocalRuntime {
	return &LocalRuntime{cfg: cfg}
}

// Type implements Runtime.
func (r *LocalRuntime) Type() RuntimeType { return RuntimeLocal }

// CreateWorkspace implements Runtime.
func (r *LocalRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) (string, error) {
	return "", fmt.Errorf("local CreateWorkspace: not yet implemented")
}

// ListWorkspaces implements Runtime.
func (r *LocalRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	return nil, fmt.Errorf("local ListWorkspaces: not yet implemented")
}

// ExecWorkspace implements Runtime.
func (r *LocalRuntime) ExecWorkspace(ctx context.Context, name string) error {
	return fmt.Errorf("local ExecWorkspace: not yet implemented")
}

// ResumeWorkspace implements Runtime.
func (r *LocalRuntime) ResumeWorkspace(ctx context.Context, name string) error {
	return fmt.Errorf("local ResumeWorkspace: not yet implemented")
}

// DeleteWorkspace implements Runtime.
func (r *LocalRuntime) DeleteWorkspace(ctx context.Context, name string) error {
	return fmt.Errorf("local DeleteWorkspace: not yet implemented")
}
