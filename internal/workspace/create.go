// Package workspace implements orchestration logic for workspace subcommands.
// It sits between the cobra command layer and the runtime backends (k8s/local).
package workspace

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bobbydeveaux/coo-cli/internal/runtime"
)

// CreateConfig holds the parsed flag values for the create command.
type CreateConfig struct {
	// Kubeconfig, KubeContext, Namespace, LocalMode come from the root persistent flags.
	Kubeconfig  string
	KubeContext string
	Namespace   string
	LocalMode   bool

	// Workspace-specific flags.
	Repo        string
	Concept     string
	Model       string
	TTL         string
	Image       string
	Token       string
	GithubToken string
}

// Create executes the full `coo workspace create` flow:
//  1. Detect which runtime to use (k8s or local).
//  2. List existing non-terminated workspaces and prompt the user to resume one.
//  3. If the user chooses to create a new workspace, call runtime.CreateWorkspace.
//  4. Exec into the workspace with Claude Code.
//  5. Print "Resume this session: coo workspace resume <name>" on exit (always).
func Create(ctx context.Context, cfg CreateConfig) error {
	if cfg.Repo == "" && cfg.Concept == "" {
		return fmt.Errorf("one of --repo or --concept is required")
	}
	if cfg.Repo != "" && cfg.Concept != "" {
		return fmt.Errorf("--repo and --concept are mutually exclusive; choose one")
	}

	rt, err := runtime.Detect(ctx, runtime.Config{
		LocalMode:   cfg.LocalMode,
		Kubeconfig:  cfg.Kubeconfig,
		KubeContext: cfg.KubeContext,
		Namespace:   cfg.Namespace,
	})
	if err != nil {
		return fmt.Errorf("detect runtime: %w", err)
	}

	// Prompt to resume if existing workspaces exist.
	if wsName, err := promptResume(ctx, rt); err != nil {
		return err
	} else if wsName != "" {
		// User chose to resume an existing workspace.
		defer printResumeHint(wsName)
		return rt.ResumeWorkspace(ctx, wsName)
	}

	// Create a new workspace.
	opts := runtime.CreateOptions{
		Repo:        cfg.Repo,
		Concept:     cfg.Concept,
		Model:       cfg.Model,
		TTL:         cfg.TTL,
		Image:       cfg.Image,
		Token:       cfg.Token,
		GithubToken: cfg.GithubToken,
	}

	wsName, err := rt.CreateWorkspace(ctx, opts)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	// Always print the resume hint after the session ends (including on error).
	defer printResumeHint(wsName)

	return rt.ExecWorkspace(ctx, wsName)
}

// promptResume lists existing workspaces, prints a numbered menu, and reads the
// user's selection. It returns the name of the workspace to resume, or an empty
// string if the user wants to create a new one.
func promptResume(ctx context.Context, rt runtime.Runtime) (string, error) {
	workspaces, err := rt.ListWorkspaces(ctx)
	if err != nil {
		// Non-fatal: proceed to create rather than blocking on list errors.
		fmt.Fprintf(os.Stderr, "Warning: could not list existing workspaces: %v\n", err)
		return "", nil
	}

	if len(workspaces) == 0 {
		return "", nil
	}

	fmt.Fprintln(os.Stderr, "\nExisting workspaces:")
	for i, ws := range workspaces {
		repo := ws.Repo
		if repo == "" {
			repo = ws.Mode
		}
		fmt.Fprintf(os.Stderr, "  [%d] %s  phase=%-12s  %s\n", i+1, ws.Name, ws.Phase, repo)
	}
	fmt.Fprintln(os.Stderr, "  [0] Create a new workspace")
	fmt.Fprint(os.Stderr, "\nSelect a workspace to resume (press Enter to create new): ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		// EOF or read error — proceed with new workspace.
		fmt.Fprintln(os.Stderr)
		return "", nil
	}

	line = strings.TrimSpace(line)
	if line == "" || line == "0" {
		return "", nil
	}

	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(workspaces) {
		fmt.Fprintf(os.Stderr, "Invalid selection %q; creating a new workspace.\n", line)
		return "", nil
	}

	return workspaces[idx-1].Name, nil
}

// printResumeHint writes the post-session resume hint to stderr.
func printResumeHint(wsName string) {
	fmt.Fprintf(os.Stderr, "\nResume this session: coo workspace resume %s\n", wsName)
}
