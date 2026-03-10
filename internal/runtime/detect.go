// Package runtime handles detection and abstraction of the execution backend.
// coo supports two runtimes: k8s mode (using the itsacoo operator) and local
// mode (Docker/Podman, no Kubernetes required).
package runtime

import (
	"context"
	"fmt"
)

// RuntimeType identifies which execution backend is active.
type RuntimeType string

const (
	RuntimeK8s   RuntimeType = "k8s"
	RuntimeLocal RuntimeType = "local"
)

// Config holds the flags and environment settings used to resolve a Runtime.
// It is typically populated from cobra flags in the root command.
type Config struct {
	// LocalMode forces local Docker/Podman mode regardless of k8s availability.
	LocalMode bool
	// Kubeconfig is the path to a kubeconfig file; empty means use defaults.
	Kubeconfig string
	// KubeContext is the kubeconfig context to use; empty means current context.
	KubeContext string
	// Namespace is the namespace where the COO operator is installed.
	Namespace string
}

// CreateOptions holds the parameters for workspace creation.
type CreateOptions struct {
	// Repo is "owner/repo" for freestyle mode.
	Repo string
	// Concept is the COO concept name for handoff mode (k8s only).
	Concept string
	// Model is the Claude model to use.
	Model string
	// TTL is the workspace time-to-live (k8s mode only), e.g. "4h".
	TTL string
	// Image overrides the default worker image.
	Image string
	// Token is the Claude Code OAuth token (local mode; falls back to env/disk).
	Token string
	// GithubToken is used for cloning private repos.
	GithubToken string
}

// WorkspaceInfo is a summary of a single workspace returned by ListWorkspaces.
type WorkspaceInfo struct {
	Name      string
	Mode      string
	Phase     string
	PodName   string
	TTLExpiry string
	Repo      string
}

// Runtime is the interface implemented by both the k8s and local backends.
// All workspace commands delegate to this interface after runtime detection.
type Runtime interface {
	// Type returns which backend is active.
	Type() RuntimeType

	// CreateWorkspace creates a new workspace and returns its name.
	// The caller is responsible for calling ExecWorkspace afterwards.
	CreateWorkspace(ctx context.Context, opts CreateOptions) (string, error)

	// ListWorkspaces returns all known workspaces for this backend.
	ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error)

	// ExecWorkspace opens an interactive shell in the named workspace.
	ExecWorkspace(ctx context.Context, name string) error

	// ResumeWorkspace re-attaches to the last Claude Code session in the workspace.
	ResumeWorkspace(ctx context.Context, name string) error

	// DeleteWorkspace removes the named workspace and its associated resources.
	DeleteWorkspace(ctx context.Context, name string) error
}

// Detect resolves the Runtime to use based on cfg and the current environment.
//
// Resolution order:
//  1. cfg.LocalMode == true  → local Docker/Podman mode
//  2. cfg.KubeContext or cfg.Kubeconfig is set → k8s mode (error if unreachable)
//  3. k8s API reachable and COO CRD present → k8s mode
//  4. fallback → local Docker/Podman mode
func Detect(ctx context.Context, cfg Config) (Runtime, error) {
	// 1. Explicit local flag.
	if cfg.LocalMode {
		return newLocalRuntime(cfg), nil
	}

	// 2. Explicit k8s flags — must succeed or error out.
	if cfg.KubeContext != "" || cfg.Kubeconfig != "" {
		r, err := newK8sRuntime(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("k8s mode requested via flags but unavailable: %w", err)
		}
		return r, nil
	}

	// 3. Auto-detect: try k8s, fall back to local on any failure.
	if r, err := newK8sRuntime(ctx, cfg); err == nil {
		return r, nil
	}

	// 4. No k8s available — use local mode.
	return newLocalRuntime(cfg), nil
}
