package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WorkspaceEntry records the local state of a single Docker/Podman workspace.
// It is persisted in ~/.coo/workspaces.json.
type WorkspaceEntry struct {
	Name        string    `json:"name"`
	ContainerID string    `json:"containerID"`
	Repo        string    `json:"repo,omitempty"`
	Concept     string    `json:"concept,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	VolumePath  string    `json:"volumePath"`
	Phase       string    `json:"phase,omitempty"`
	Image       string    `json:"image,omitempty"`
}

// cooDir returns the path to the ~/.coo directory.
func cooDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".coo"), nil
}

// workspacesFile returns the path to ~/.coo/workspaces.json.
func workspacesFile() (string, error) {
	dir, err := cooDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspaces.json"), nil
}

// LoadWorkspaces reads all workspace entries from ~/.coo/workspaces.json.
// Returns an empty slice if the file does not exist.
func LoadWorkspaces() ([]WorkspaceEntry, error) {
	path, err := workspacesFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []WorkspaceEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspaces file: %w", err)
	}

	var entries []WorkspaceEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse workspaces file: %w", err)
	}
	return entries, nil
}

// SaveWorkspaces atomically writes all workspace entries to ~/.coo/workspaces.json.
// It creates the ~/.coo directory if needed and uses a write-to-temp-then-rename
// pattern to prevent partial writes corrupting the state file.
// The directory is created with 0700 and the file with 0600 permissions.
func SaveWorkspaces(entries []WorkspaceEntry) error {
	dir, err := cooDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create ~/.coo directory: %w", err)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspaces: %w", err)
	}

	path, err := workspacesFile()
	if err != nil {
		return err
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(dir, ".workspaces-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("set temp file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file to workspaces.json: %w", err)
	}
	return nil
}

// AddWorkspace appends a new entry to the local workspace store.
func AddWorkspace(entry WorkspaceEntry) error {
	entries, err := LoadWorkspaces()
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	return SaveWorkspaces(entries)
}

// RemoveWorkspace removes the entry with the given name from the local workspace store.
// It is not an error if the name does not exist.
func RemoveWorkspace(name string) error {
	entries, err := LoadWorkspaces()
	if err != nil {
		return err
	}
	filtered := make([]WorkspaceEntry, 0, len(entries))
	for _, e := range entries {
		if e.Name != name {
			filtered = append(filtered, e)
		}
	}
	return SaveWorkspaces(filtered)
}

// FindWorkspace returns the WorkspaceEntry for the given name, or an error if not found.
func FindWorkspace(name string) (WorkspaceEntry, error) {
	entries, err := LoadWorkspaces()
	if err != nil {
		return WorkspaceEntry{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e, nil
		}
	}
	return WorkspaceEntry{}, fmt.Errorf("workspace %q not found", name)
}

// NewVolumePath returns the canonical path for a workspace's local volume directory
// (~/.coo/volumes/<name>). It does not create the directory.
func NewVolumePath(name string) (string, error) {
	dir, err := cooDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "volumes", name), nil
}
