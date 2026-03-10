package workspace

import (
	"context"
	"fmt"

	"github.com/bobbydeveaux/coo-cli/internal/runtime"
)

// DeleteConfig holds the parsed flag values for the delete command.
type DeleteConfig struct {
	Kubeconfig  string
	KubeContext string
	Namespace   string
	LocalMode   bool
}

// Delete removes a workspace and its associated resources.
func Delete(ctx context.Context, name string, cfg DeleteConfig) error {
	rt, err := runtime.Detect(ctx, runtime.Config{
		LocalMode:   cfg.LocalMode,
		Kubeconfig:  cfg.Kubeconfig,
		KubeContext: cfg.KubeContext,
		Namespace:   cfg.Namespace,
	})
	if err != nil {
		return fmt.Errorf("detect runtime: %w", err)
	}
	if err := rt.DeleteWorkspace(ctx, name); err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	fmt.Printf("Workspace %s deleted.\n", name)
	return nil
}
