package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultLocalWorkerImage = "ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest"

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

// ListWorkspaces implements Runtime. It reads workspace state from
// ~/.coo/workspaces.json and returns non-terminated entries.
func (r *LocalRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	entries, err := LoadWorkspaces()
	if err != nil {
		return nil, err
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
// It generates a workspace name (ws-<unix-timestamp>), creates the persistent
// volume directories under ~/.coo/volumes/<name>/, records the entry in
// ~/.coo/workspaces.json, and returns the workspace name.
// ExecWorkspace must be called separately to launch the container.
func (r *LocalRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) (string, error) {
	if opts.Repo == "" && opts.Concept == "" {
		return "", fmt.Errorf("one of --repo or --concept is required")
	}

	name := fmt.Sprintf("ws-%d", time.Now().Unix())

	volumePath, err := NewVolumePath(name)
	if err != nil {
		return "", err
	}

	// workspace/ holds the git repository; claude-data/ is mounted as
	// /tmp/.claude inside the container so Claude Code session files survive
	// container restarts (containers are launched with --rm).
	workspaceDir := filepath.Join(volumePath, "workspace")
	claudeDataDir := filepath.Join(volumePath, "claude-data")
	if err := os.MkdirAll(workspaceDir, 0700); err != nil {
		return "", fmt.Errorf("create workspace directory: %w", err)
	}
	if err := os.MkdirAll(claudeDataDir, 0700); err != nil {
		return "", fmt.Errorf("create claude-data directory: %w", err)
	}

	image := opts.Image
	if image == "" {
		image = defaultLocalWorkerImage
	}

	entry := WorkspaceEntry{
		Name:       name,
		Repo:       opts.Repo,
		Concept:    opts.Concept,
		CreatedAt:  time.Now(),
		VolumePath: volumePath,
		Phase:      "Pending",
		Image:      image,
	}

	if err := AddWorkspace(entry); err != nil {
		return "", fmt.Errorf("save workspace entry: %w", err)
	}

	fmt.Printf("Created local workspace %s\n", name)
	return name, nil
}

// ExecWorkspace implements Runtime.
// Launches a Docker container for the workspace, mounting the persistent volume.
// On the first run (empty workspace directory) the container entrypoint is
// invoked without a command override so it can clone GIT_REPO and bootstrap
// Claude Code settings. On subsequent runs the command is overridden to start
// Claude Code directly against the already-cloned repository.
func (r *LocalRuntime) ExecWorkspace(ctx context.Context, name string) error {
	return r.runContainer(name, "")
}

// ResumeWorkspace implements Runtime.
// Locates the most recently modified Claude Code session file under the
// workspace's claude-data volume and starts the container with --resume <id>.
// Falls back to a plain ExecWorkspace if no session file is found.
func (r *LocalRuntime) ResumeWorkspace(ctx context.Context, name string) error {
	entry, err := FindWorkspace(name)
	if err != nil {
		return err
	}

	claudeDataDir := filepath.Join(entry.VolumePath, "claude-data")
	sessionID, err := findLatestSessionID(claudeDataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No previous session found (%v); starting fresh.\n", err)
		return r.ExecWorkspace(ctx, name)
	}

	resumeCmd := fmt.Sprintf("cd /workspace && claude --dangerously-skip-permissions --resume %s", sessionID)
	return r.runContainer(name, resumeCmd)
}

// DeleteWorkspace implements Runtime.
// Removes the workspace entry from ~/.coo/workspaces.json and deletes the
// volume directory from ~/.coo/volumes/<name>.
func (r *LocalRuntime) DeleteWorkspace(ctx context.Context, name string) error {
	entry, err := FindWorkspace(name)
	if err != nil {
		return err
	}

	if err := RemoveWorkspace(name); err != nil {
		return fmt.Errorf("remove workspace entry: %w", err)
	}

	if entry.VolumePath != "" {
		if err := os.RemoveAll(entry.VolumePath); err != nil {
			// Non-fatal: the state entry is already removed; warn and continue.
			fmt.Fprintf(os.Stderr, "Warning: could not remove volume at %s: %v\n", entry.VolumePath, err)
		}
	}

	fmt.Printf("Deleted workspace %s\n", name)
	return nil
}

// runContainer starts a Docker container for the named workspace.
//
// If shellCmd is non-empty it is passed to the container as:
//
//	bash -c "<shellCmd>"
//
// overriding the image's default entrypoint.  This is used for
// ExecWorkspace (fresh re-attach) and ResumeWorkspace (--resume <id>).
//
// If shellCmd is empty the image's default entrypoint is used.  On the first
// run the entrypoint detects GIT_REPO and clones the repository into
// /workspace before launching Claude Code.  On subsequent runs (non-empty
// workspace directory) the command is automatically set to run Claude directly.
func (r *LocalRuntime) runContainer(name, shellCmd string) error {
	entry, err := FindWorkspace(name)
	if err != nil {
		return err
	}

	token, err := ResolveClaudeToken("")
	if err != nil {
		return fmt.Errorf("resolve claude token: %w", err)
	}
	githubToken, err := ResolveGitHubToken("")
	if err != nil {
		return fmt.Errorf("resolve github token: %w", err)
	}

	workspaceDir := filepath.Join(entry.VolumePath, "workspace")
	claudeDataDir := filepath.Join(entry.VolumePath, "claude-data")

	image := entry.Image
	if image == "" {
		image = defaultLocalWorkerImage
	}

	args := []string{
		"run", "--rm", "-it",
		"-v", workspaceDir + ":/workspace",
		"-v", claudeDataDir + ":/tmp/.claude",
	}

	if token != "" {
		args = append(args, "-e", "CLAUDE_CODE_OAUTH_TOKEN="+token)
	}
	if githubToken != "" {
		args = append(args, "-e", "GITHUB_TOKEN="+githubToken)
	}

	// Determine the effective shell command when the caller didn't specify one.
	if shellCmd == "" {
		fresh, err := isDirEmpty(workspaceDir)
		if err != nil {
			return fmt.Errorf("check workspace directory: %w", err)
		}

		if fresh {
			// First run: let the container entrypoint handle git clone + bootstrap.
			if entry.Repo != "" {
				args = append(args, "-e", "GIT_REPO="+entry.Repo)
			}
			// No command override — entrypoint takes full control.
		} else {
			// Subsequent run: repository already cloned, start Claude directly.
			shellCmd = "cd /workspace && claude --dangerously-skip-permissions"
		}
	}

	args = append(args, image)
	if shellCmd != "" {
		args = append(args, "bash", "-c", shellCmd)
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isDirEmpty returns true if path does not exist or contains no directory entries.
func isDirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// findLatestSessionID scans <claudeDataDir>/projects/*/*.jsonl for Claude Code
// session files and returns the session ID of the most recently modified one.
// The session ID is the filename without the .jsonl extension.
func findLatestSessionID(claudeDataDir string) (string, error) {
	pattern := filepath.Join(claudeDataDir, "projects", "*", "*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob session files: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no session files found")
	}

	var latestPath string
	var latestMod time.Time
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().After(latestMod) {
			latestMod = info.ModTime()
			latestPath = p
		}
	}

	if latestPath == "" {
		return "", fmt.Errorf("no accessible session files found")
	}

	file := filepath.Base(latestPath)
	sessionID := strings.TrimSuffix(file, ".jsonl")
	if sessionID == "" {
		return "", fmt.Errorf("could not extract session ID from path %q", latestPath)
	}
	return sessionID, nil
}
