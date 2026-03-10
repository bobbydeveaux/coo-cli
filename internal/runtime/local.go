package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// localWorkspaceEntry is the schema for entries in ~/.coo/workspaces.json.
type localWorkspaceEntry struct {
	ContainerID string    `json:"containerID"`
	Name        string    `json:"name"`
	Repo        string    `json:"repo"`
	Concept     string    `json:"concept,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	VolumePath  string    `json:"volumePath"`
	Phase       string    `json:"phase"`
}

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

// localWorkspacesPath returns the path to ~/.coo/workspaces.json.
func localWorkspacesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".coo", "workspaces.json"), nil
}

// ListWorkspaces implements Runtime. It reads workspace state from
// ~/.coo/workspaces.json and returns non-terminated entries.
func (r *LocalRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	path, err := localWorkspacesPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// No workspaces file yet — that's fine, nothing to list.
		return []WorkspaceInfo{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var entries []localWorkspaceEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var workspaces []WorkspaceInfo
	for _, e := range entries {
		if e.Phase == "Terminated" {
			continue
		}
		workspaces = append(workspaces, WorkspaceInfo{
			Name:  e.Name,
			Mode:  "local",
			Phase: e.Phase,
			Repo:  e.Repo,
		})
	}
	return workspaces, nil
}

// CreateWorkspace implements Runtime.
func (r *LocalRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) (string, error) {
	return "", fmt.Errorf("local CreateWorkspace: not yet implemented")
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
