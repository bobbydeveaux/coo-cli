package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeWorkspacesJSON writes entries to a temporary workspaces.json file and
// returns the path. The caller must ensure the file is cleaned up.
func writeWorkspacesJSON(t *testing.T, entries []localWorkspaceEntry) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "workspaces.json")
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal test data: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write test workspaces file: %v", err)
	}
	return path
}

// overrideLocalWorkspacesPath temporarily replaces the file path used by
// LocalRuntime.ListWorkspaces with the given path. It restores the original
// after the test by monkey-patching localWorkspacesPathFn.
//
// Note: the production code uses os.UserHomeDir() so tests must redirect via
// HOME override instead.
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
	entries := []localWorkspaceEntry{
		{Name: "ws-active", ContainerID: "abc", Repo: "owner/repo", Phase: "Running", CreatedAt: time.Now()},
		{Name: "ws-done", ContainerID: "def", Repo: "owner/other", Phase: "Terminated", CreatedAt: time.Now()},
	}

	home := t.TempDir()
	withTempHome(t, home)

	cooDir := filepath.Join(home, ".coo")
	if err := os.MkdirAll(cooDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data, _ := json.Marshal(entries)
	if err := os.WriteFile(filepath.Join(cooDir, "workspaces.json"), data, 0o600); err != nil {
		t.Fatalf("write workspaces: %v", err)
	}

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
	entries := []localWorkspaceEntry{
		{Name: "ws-1", ContainerID: "aaa", Repo: "owner/a", Phase: "Running", CreatedAt: time.Now()},
		{Name: "ws-2", ContainerID: "bbb", Repo: "owner/b", Phase: "Pending", CreatedAt: time.Now()},
		{Name: "ws-3", ContainerID: "ccc", Repo: "owner/c", Phase: "", CreatedAt: time.Now()},
	}

	home := t.TempDir()
	withTempHome(t, home)

	cooDir := filepath.Join(home, ".coo")
	if err := os.MkdirAll(cooDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data, _ := json.Marshal(entries)
	if err := os.WriteFile(filepath.Join(cooDir, "workspaces.json"), data, 0o600); err != nil {
		t.Fatalf("write workspaces: %v", err)
	}

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
