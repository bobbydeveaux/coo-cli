package runtime

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	cooAPIGroup    = "coo.itsacoo.com"
	k8sProbeTimeout = 5 * time.Second
)

// K8sRuntime implements Runtime using the itsacoo operator and COOWorkspace CRs.
type K8sRuntime struct {
	cfg             Config
	discoveryClient discovery.DiscoveryInterface
}

// newK8sRuntime creates a K8sRuntime after verifying the k8s API is reachable
// and the COO operator CRD is installed.
func newK8sRuntime(ctx context.Context, cfg Config) (*K8sRuntime, error) {
	dc, err := buildDiscoveryClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("build k8s discovery client: %w", err)
	}

	if err := probeCOOCRD(dc); err != nil {
		return nil, fmt.Errorf("COO operator not detected: %w", err)
	}

	return &K8sRuntime{cfg: cfg, discoveryClient: dc}, nil
}

// buildDiscoveryClient constructs a discovery client from the provided config.
func buildDiscoveryClient(cfg Config) (discovery.DiscoveryInterface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if cfg.KubeContext != "" {
		overrides.CurrentContext = cfg.KubeContext
	}

	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		overrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	// Short timeout so auto-detection fails fast when no cluster is present.
	restCfg.Timeout = k8sProbeTimeout

	return discovery.NewDiscoveryClientForConfig(restCfg)
}

// probeCOOCRD checks that the k8s API server is reachable and that the
// coo.itsacoo.com API group (COO operator CRDs) is registered.
func probeCOOCRD(dc discovery.DiscoveryInterface) error {
	groups, err := dc.ServerGroups()
	if err != nil {
		return fmt.Errorf("contact k8s API server: %w", err)
	}

	for _, g := range groups.Groups {
		if g.Name == cooAPIGroup {
			return nil
		}
	}

	return fmt.Errorf("API group %q not found; is the itsacoo operator installed?", cooAPIGroup)
}

// Type implements Runtime.
func (r *K8sRuntime) Type() RuntimeType { return RuntimeK8s }

// CreateWorkspace implements Runtime.
func (r *K8sRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) error {
	return fmt.Errorf("k8s CreateWorkspace: not yet implemented")
}

// ListWorkspaces implements Runtime.
func (r *K8sRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	return nil, fmt.Errorf("k8s ListWorkspaces: not yet implemented")
}

// ExecWorkspace implements Runtime.
func (r *K8sRuntime) ExecWorkspace(ctx context.Context, name string) error {
	return fmt.Errorf("k8s ExecWorkspace: not yet implemented")
}

// ResumeWorkspace implements Runtime.
func (r *K8sRuntime) ResumeWorkspace(ctx context.Context, name string) error {
	return fmt.Errorf("k8s ResumeWorkspace: not yet implemented")
}

// DeleteWorkspace implements Runtime.
func (r *K8sRuntime) DeleteWorkspace(ctx context.Context, name string) error {
	return fmt.Errorf("k8s DeleteWorkspace: not yet implemented")
}
