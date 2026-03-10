package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withTempCooDir overrides the home directory used by cooDir() for the duration
// of a test by setting $HOME to a temporary directory.
func withTempCooDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Also set USERPROFILE for Windows compatibility if needed.
	t.Setenv("USERPROFILE", tmp)
	return tmp
}

func TestLoadWorkspaces_Empty(t *testing.T) {
	withTempCooDir(t)

	entries, err := LoadWorkspaces()
	if err != nil {
		t.Fatalf("LoadWorkspaces on missing file: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestSaveAndLoadWorkspaces(t *testing.T) {
	withTempCooDir(t)

	now := time.Now().UTC().Truncate(time.Second)
	want := []WorkspaceEntry{
		{
			Name:        "ws-1000000000",
			ContainerID: "abc123",
			Repo:        "owner/repo",
			CreatedAt:   now,
			VolumePath:  "/home/user/.coo/volumes/ws-1000000000",
		},
		{
			Name:        "ws-1000000001",
			ContainerID: "def456",
			Concept:     "my-concept",
			CreatedAt:   now,
			VolumePath:  "/home/user/.coo/volumes/ws-1000000001",
		},
	}

	if err := SaveWorkspaces(want); err != nil {
		t.Fatalf("SaveWorkspaces: %v", err)
	}

	got, err := LoadWorkspaces()
	if err != nil {
		t.Fatalf("LoadWorkspaces: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		if g.Name != w.Name || g.ContainerID != w.ContainerID || g.Repo != w.Repo ||
			g.Concept != w.Concept || g.VolumePath != w.VolumePath {
			t.Errorf("entry[%d] mismatch: got %+v, want %+v", i, g, w)
		}
		if !g.CreatedAt.Equal(w.CreatedAt) {
			t.Errorf("entry[%d] CreatedAt mismatch: got %v, want %v", i, g.CreatedAt, w.CreatedAt)
		}
	}
}

func TestSaveWorkspaces_CreatesDirectory(t *testing.T) {
	home := withTempCooDir(t)

	cooPath := filepath.Join(home, ".coo")
	if _, err := os.Stat(cooPath); !os.IsNotExist(err) {
		t.Fatal("~/.coo should not exist before first save")
	}

	if err := SaveWorkspaces([]WorkspaceEntry{}); err != nil {
		t.Fatalf("SaveWorkspaces: %v", err)
	}

	info, err := os.Stat(cooPath)
	if err != nil {
		t.Fatalf("~/.coo not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("~/.coo is not a directory")
	}
	// Verify directory permissions are 0700.
	if info.Mode().Perm() != 0700 {
		t.Errorf("~/.coo perm = %v, want 0700", info.Mode().Perm())
	}
}

func TestSaveWorkspaces_FilePermissions(t *testing.T) {
	home := withTempCooDir(t)

	if err := SaveWorkspaces([]WorkspaceEntry{}); err != nil {
		t.Fatalf("SaveWorkspaces: %v", err)
	}

	wsFile := filepath.Join(home, ".coo", "workspaces.json")
	info, err := os.Stat(wsFile)
	if err != nil {
		t.Fatalf("stat workspaces.json: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("workspaces.json perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestAddWorkspace(t *testing.T) {
	withTempCooDir(t)

	e1 := WorkspaceEntry{Name: "ws-1", ContainerID: "ctr1", Repo: "a/b", CreatedAt: time.Now().UTC()}
	e2 := WorkspaceEntry{Name: "ws-2", ContainerID: "ctr2", Repo: "c/d", CreatedAt: time.Now().UTC()}

	if err := AddWorkspace(e1); err != nil {
		t.Fatalf("AddWorkspace(e1): %v", err)
	}
	if err := AddWorkspace(e2); err != nil {
		t.Fatalf("AddWorkspace(e2): %v", err)
	}

	entries, err := LoadWorkspaces()
	if err != nil {
		t.Fatalf("LoadWorkspaces: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Name != "ws-1" || entries[1].Name != "ws-2" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestRemoveWorkspace(t *testing.T) {
	withTempCooDir(t)

	for _, name := range []string{"ws-1", "ws-2", "ws-3"} {
		if err := AddWorkspace(WorkspaceEntry{Name: name, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("AddWorkspace(%s): %v", name, err)
		}
	}

	if err := RemoveWorkspace("ws-2"); err != nil {
		t.Fatalf("RemoveWorkspace: %v", err)
	}

	entries, err := LoadWorkspaces()
	if err != nil {
		t.Fatalf("LoadWorkspaces: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Name == "ws-2" {
			t.Error("ws-2 should have been removed")
		}
	}
}

func TestRemoveWorkspace_NotExist(t *testing.T) {
	withTempCooDir(t)

	// Removing a non-existent workspace should not error.
	if err := RemoveWorkspace("ws-nonexistent"); err != nil {
		t.Fatalf("RemoveWorkspace on missing entry: %v", err)
	}
}

func TestFindWorkspace(t *testing.T) {
	withTempCooDir(t)

	want := WorkspaceEntry{Name: "ws-42", ContainerID: "xyz", Repo: "foo/bar", CreatedAt: time.Now().UTC()}
	if err := AddWorkspace(want); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}

	got, err := FindWorkspace("ws-42")
	if err != nil {
		t.Fatalf("FindWorkspace: %v", err)
	}
	if got.Name != want.Name || got.ContainerID != want.ContainerID || got.Repo != want.Repo {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFindWorkspace_NotFound(t *testing.T) {
	withTempCooDir(t)

	_, err := FindWorkspace("ws-missing")
	if err == nil {
		t.Fatal("expected error for missing workspace, got nil")
	}
}

func TestNewVolumePath(t *testing.T) {
	home := withTempCooDir(t)

	path, err := NewVolumePath("ws-test")
	if err != nil {
		t.Fatalf("NewVolumePath: %v", err)
	}

	expected := filepath.Join(home, ".coo", "volumes", "ws-test")
	if path != expected {
		t.Errorf("got %q, want %q", path, expected)
	}
}
