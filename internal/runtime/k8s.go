package runtime

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	cooAPIGroup     = "coo.itsacoo.com"
	k8sProbeTimeout = 5 * time.Second
)

var cooWorkspaceGVR = schema.GroupVersionResource{
	Group:    cooAPIGroup,
	Version:  "v1alpha1",
	Resource: "cooworkspaces",
}

// K8sRuntime implements Runtime using the itsacoo operator and COOWorkspace CRs.
type K8sRuntime struct {
	cfg             Config
	discoveryClient discovery.DiscoveryInterface
	dynamicClient   dynamic.Interface
}

// newK8sRuntime creates a K8sRuntime after verifying the k8s API is reachable
// and the COO operator CRD is installed.
func newK8sRuntime(ctx context.Context, cfg Config) (*K8sRuntime, error) {
	dc, dynClient, err := buildClients(cfg)
	if err != nil {
		return nil, fmt.Errorf("build k8s clients: %w", err)
	}

	if err := probeCOOCRD(dc); err != nil {
		return nil, fmt.Errorf("COO operator not detected: %w", err)
	}

	return &K8sRuntime{cfg: cfg, discoveryClient: dc, dynamicClient: dynClient}, nil
}

// buildClients constructs both a discovery client and a dynamic client from the
// provided config. A short timeout is applied so auto-detection fails fast when
// no cluster is present.
func buildClients(cfg Config) (discovery.DiscoveryInterface, dynamic.Interface, error) {
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
		return nil, nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	// Short timeout so auto-detection fails fast when no cluster is present.
	restCfg.Timeout = k8sProbeTimeout

	dc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create discovery client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return dc, dynClient, nil
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

// ListWorkspaces implements Runtime. It returns all COOWorkspaces in the
// configured namespace that are not in a Terminated phase.
func (r *K8sRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	ns := r.cfg.Namespace
	if ns == "" {
		ns = "coo-system"
	}

	list, err := r.dynamicClient.Resource(cooWorkspaceGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list COOWorkspaces in %s: %w", ns, err)
	}

	var workspaces []WorkspaceInfo
	for _, item := range list.Items {
		status, _ := item.Object["status"].(map[string]interface{})
		phase, _ := status["phase"].(string)

		// Skip terminated workspaces — they are no longer usable.
		if phase == "Terminated" {
			continue
		}

		spec, _ := item.Object["spec"].(map[string]interface{})
		mode, _ := spec["mode"].(string)
		repo, _ := spec["repo"].(string)
		ttl, _ := spec["ttl"].(string)
		podName, _ := status["podName"].(string)

		workspaces = append(workspaces, WorkspaceInfo{
			Name:      item.GetName(),
			Mode:      mode,
			Phase:     phase,
			PodName:   podName,
			TTLExpiry: ttl,
			Repo:      repo,
		})
	}

	return workspaces, nil
}

// CreateWorkspace implements Runtime.
func (r *K8sRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) error {
	return fmt.Errorf("k8s CreateWorkspace: not yet implemented")
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
