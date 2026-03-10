package workspace

import (
	"context"
	"fmt"

	"github.com/bobbydeveaux/coo-cli/internal/runtime"
)

// ExecConfig holds the parsed flag values for the exec and resume commands.
type ExecConfig struct {
	Kubeconfig  string
	KubeContext string
	Namespace   string
	LocalMode   bool
}

// Exec drops the user into Claude Code in an existing workspace (fresh session).
func Exec(ctx context.Context, name string, cfg ExecConfig) error {
	rt, err := detectRT(ctx, cfg)
	if err != nil {
		return err
	}
	defer printResumeHint(name)
	return rt.ExecWorkspace(ctx, name)
}

// Resume re-attaches to the last Claude Code session in an existing workspace.
func Resume(ctx context.Context, name string, cfg ExecConfig) error {
	rt, err := detectRT(ctx, cfg)
	if err != nil {
		return err
	}
	defer printResumeHint(name)
	return rt.ResumeWorkspace(ctx, name)
}

// detectRT is a small helper to build and return a Runtime from ExecConfig.
func detectRT(ctx context.Context, cfg ExecConfig) (runtime.Runtime, error) {
	rt, err := runtime.Detect(ctx, runtime.Config{
		LocalMode:   cfg.LocalMode,
		Kubeconfig:  cfg.Kubeconfig,
		KubeContext: cfg.KubeContext,
		Namespace:   cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("detect runtime: %w", err)
	}
	return rt, nil
}
