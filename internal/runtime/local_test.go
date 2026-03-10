package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempHome temporarily sets $HOME to dir for the duration of the test.
func withTempHome(t *testing.T, dir string) {
	t.Helper()
	orig, set := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	t.Cleanup(func() {
		if set {
			_ = os.Setenv("HOME", orig)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
}

// writeWorkspacesJSON marshals entries to ~/.coo/workspaces.json under the
// given home directory (which must already exist).
func writeWorkspacesJSON(t *testing.T, home string, entries []WorkspaceEntry) {
	t.Helper()
	cooDir := filepath.Join(home, ".coo")
	if err := os.MkdirAll(cooDir, 0o755); err != nil {
		t.Fatalf("mkdir .coo: %v", err)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal test entries: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cooDir, "workspaces.json"), data, 0o600); err != nil {
		t.Fatalf("write workspaces.json: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListWorkspaces tests
// ---------------------------------------------------------------------------

// TestLocalRuntime_ListWorkspaces_Empty verifies that an absent workspaces file
// returns an empty, non-nil slice without error.
func TestLocalRuntime_ListWorkspaces_Empty(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	got, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

// TestLocalRuntime_ListWorkspaces_FiltersTerminated verifies that Terminated
// workspaces are excluded from the results.
func TestLocalRuntime_ListWorkspaces_FiltersTerminated(t *testing.T) {
	entries := []WorkspaceEntry{
		{Name: "ws-active", ContainerID: "abc", Repo: "owner/repo", Phase: "Running", CreatedAt: time.Now()},
		{Name: "ws-done", ContainerID: "def", Repo: "owner/other", Phase: "Terminated", CreatedAt: time.Now()},
	}

	home := t.TempDir()
	withTempHome(t, home)
	writeWorkspacesJSON(t, home, entries)

	rt := newLocalRuntime(Config{})
	got, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(got))
	}
	if got[0].Name != "ws-active" {
		t.Errorf("expected ws-active, got %q", got[0].Name)
	}
	if got[0].Mode != "local" {
		t.Errorf("expected mode=local, got %q", got[0].Mode)
	}
}

// TestLocalRuntime_ListWorkspaces_AllActive verifies that multiple active
// workspaces are all returned correctly.
func TestLocalRuntime_ListWorkspaces_AllActive(t *testing.T) {
	entries := []WorkspaceEntry{
		{Name: "ws-1", ContainerID: "aaa", Repo: "owner/a", Phase: "Running", CreatedAt: time.Now()},
		{Name: "ws-2", ContainerID: "bbb", Repo: "owner/b", Phase: "Pending", CreatedAt: time.Now()},
		{Name: "ws-3", ContainerID: "ccc", Repo: "owner/c", Phase: "", CreatedAt: time.Now()},
	}

	home := t.TempDir()
	withTempHome(t, home)
	writeWorkspacesJSON(t, home, entries)

	rt := newLocalRuntime(Config{})
	got, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 workspaces, got %d", len(got))
	}

	nameSet := map[string]bool{}
	for _, w := range got {
		nameSet[w.Name] = true
		if w.Mode != "local" {
			t.Errorf("workspace %s: expected mode=local, got %q", w.Name, w.Mode)
		}
	}
	for _, e := range entries {
		if !nameSet[e.Name] {
			t.Errorf("workspace %s missing from results", e.Name)
		}
	}
}

// TestLocalRuntime_ListWorkspaces_MalformedJSON verifies that a malformed
// workspaces file returns an error.
func TestLocalRuntime_ListWorkspaces_MalformedJSON(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	cooDir := filepath.Join(home, ".coo")
	if err := os.MkdirAll(cooDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cooDir, "workspaces.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rt := newLocalRuntime(Config{})
	_, err := rt.ListWorkspaces(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreateWorkspace tests
// ---------------------------------------------------------------------------

// TestLocalRuntime_CreateWorkspace_Basic verifies that CreateWorkspace creates
// the expected volume directories and persists the workspace entry.
func TestLocalRuntime_CreateWorkspace_Basic(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	name, err := rt.CreateWorkspace(context.Background(), CreateOptions{
		Repo: "owner/repo",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	if !strings.HasPrefix(name, "ws-") {
		t.Errorf("expected name to start with ws-, got %q", name)
	}

	// Volume directories must be created.
	volumePath := filepath.Join(home, ".coo", "volumes", name)
	for _, sub := range []string{"workspace", "claude-data"} {
		if _, err := os.Stat(filepath.Join(volumePath, sub)); err != nil {
			t.Errorf("expected %s directory to exist: %v", sub, err)
		}
	}

	// Entry must be persisted.
	entry, err := FindWorkspace(name)
	if err != nil {
		t.Fatalf("FindWorkspace: %v", err)
	}
	if entry.Repo != "owner/repo" {
		t.Errorf("entry.Repo = %q, want %q", entry.Repo, "owner/repo")
	}
	if entry.Phase != "Pending" {
		t.Errorf("entry.Phase = %q, want Pending", entry.Phase)
	}
	if entry.Image != defaultLocalWorkerImage {
		t.Errorf("entry.Image = %q, want default image", entry.Image)
	}
	if entry.VolumePath != volumePath {
		t.Errorf("entry.VolumePath = %q, want %q", entry.VolumePath, volumePath)
	}
}

// TestLocalRuntime_CreateWorkspace_ConceptMode verifies concept-mode creation.
func TestLocalRuntime_CreateWorkspace_ConceptMode(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	name, err := rt.CreateWorkspace(context.Background(), CreateOptions{
		Concept: "my-concept",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	entry, err := FindWorkspace(name)
	if err != nil {
		t.Fatalf("FindWorkspace: %v", err)
	}
	if entry.Concept != "my-concept" {
		t.Errorf("entry.Concept = %q, want %q", entry.Concept, "my-concept")
	}
	if entry.Repo != "" {
		t.Errorf("entry.Repo should be empty for concept mode, got %q", entry.Repo)
	}
}

// TestLocalRuntime_CreateWorkspace_CustomImage verifies that a custom image
// override is persisted in the workspace entry.
func TestLocalRuntime_CreateWorkspace_CustomImage(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	name, err := rt.CreateWorkspace(context.Background(), CreateOptions{
		Repo:  "owner/repo",
		Image: "my-custom-image:v1",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	entry, err := FindWorkspace(name)
	if err != nil {
		t.Fatalf("FindWorkspace: %v", err)
	}
	if entry.Image != "my-custom-image:v1" {
		t.Errorf("entry.Image = %q, want my-custom-image:v1", entry.Image)
	}
}

// TestLocalRuntime_CreateWorkspace_MissingRepoAndConcept verifies that omitting
// both --repo and --concept returns an error.
func TestLocalRuntime_CreateWorkspace_MissingRepoAndConcept(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	_, err := rt.CreateWorkspace(context.Background(), CreateOptions{})
	if err == nil {
		t.Fatal("expected error for missing repo and concept, got nil")
	}
}

// TestLocalRuntime_CreateWorkspace_AppearInList verifies that a newly created
// workspace is immediately visible in ListWorkspaces.
func TestLocalRuntime_CreateWorkspace_AppearInList(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	name, err := rt.CreateWorkspace(context.Background(), CreateOptions{
		Repo: "owner/listed",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	workspaces, err := rt.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}

	found := false
	for _, ws := range workspaces {
		if ws.Name == name {
			found = true
			if ws.Repo != "owner/listed" {
				t.Errorf("workspace Repo = %q, want owner/listed", ws.Repo)
			}
			if ws.Mode != "local" {
				t.Errorf("workspace Mode = %q, want local", ws.Mode)
			}
		}
	}
	if !found {
		t.Errorf("workspace %s not found in ListWorkspaces results", name)
	}
}

// ---------------------------------------------------------------------------
// DeleteWorkspace tests
// ---------------------------------------------------------------------------

// TestLocalRuntime_DeleteWorkspace removes an entry and its volume.
func TestLocalRuntime_DeleteWorkspace_RemovesEntryAndVolume(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	name, err := rt.CreateWorkspace(context.Background(), CreateOptions{Repo: "owner/del"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Verify volume exists before deletion.
	entry, err := FindWorkspace(name)
	if err != nil {
		t.Fatalf("FindWorkspace before delete: %v", err)
	}
	if _, err := os.Stat(entry.VolumePath); err != nil {
		t.Fatalf("volume should exist before delete: %v", err)
	}

	if err := rt.DeleteWorkspace(context.Background(), name); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Entry must be gone.
	if _, err := FindWorkspace(name); err == nil {
		t.Error("workspace should not exist after delete")
	}

	// Volume must be removed.
	if _, err := os.Stat(entry.VolumePath); !os.IsNotExist(err) {
		t.Errorf("volume should be removed after delete, stat err = %v", err)
	}
}

// TestLocalRuntime_DeleteWorkspace_NotFound returns an error for unknown name.
func TestLocalRuntime_DeleteWorkspace_NotFound(t *testing.T) {
	home := t.TempDir()
	withTempHome(t, home)

	rt := newLocalRuntime(Config{})
	err := rt.DeleteWorkspace(context.Background(), "ws-nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown workspace, got nil")
	}
}

// ---------------------------------------------------------------------------
// isDirEmpty tests
// ---------------------------------------------------------------------------

func TestIsDirEmpty_NonExistent(t *testing.T) {
	empty, err := isDirEmpty(filepath.Join(t.TempDir(), "no-such-dir"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !empty {
		t.Error("expected true for non-existent directory")
	}
}

func TestIsDirEmpty_Empty(t *testing.T) {
	dir := t.TempDir()
	empty, err := isDirEmpty(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !empty {
		t.Error("expected true for empty directory")
	}
}

func TestIsDirEmpty_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	empty, err := isDirEmpty(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if empty {
		t.Error("expected false for non-empty directory")
	}
}

// ---------------------------------------------------------------------------
// findLatestSessionID tests
// ---------------------------------------------------------------------------

func TestFindLatestSessionID_NoFiles(t *testing.T) {
	_, err := findLatestSessionID(t.TempDir())
	if err == nil {
		t.Fatal("expected error when no session files exist")
	}
}

func TestFindLatestSessionID_SingleFile(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects", "proj1")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sessionID := "abc123def456"
	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := findLatestSessionID(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sessionID {
		t.Errorf("got %q, want %q", got, sessionID)
	}
}

func TestFindLatestSessionID_PicksMostRecent(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects", "proj1")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	older := "session-older"
	newer := "session-newer"

	oldPath := filepath.Join(projDir, older+".jsonl")
	newPath := filepath.Join(projDir, newer+".jsonl")

	if err := os.WriteFile(oldPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write old: %v", err)
	}
	// Ensure newer file has a later mtime by using Chtimes.
	if err := os.WriteFile(newPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write new: %v", err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(newPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := findLatestSessionID(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != newer {
		t.Errorf("got %q, want %q", got, newer)
	}
}
