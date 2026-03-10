package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	cooAPIGroup        = "coo.itsacoo.com"
	cooAPIVersion      = "v1alpha1"
	k8sProbeTimeout    = 5 * time.Second
	createReadyTimeout = 120 * time.Second
	createPollInterval = 2 * time.Second
	defaultNamespace   = "coo-system"
	defaultWorkerImage = "ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest"
	workspaceContainer = "workspace"
)

var cooWorkspaceGVR = schema.GroupVersionResource{
	Group:    cooAPIGroup,
	Version:  cooAPIVersion,
	Resource: "cooworkspaces",
}

// K8sRuntime implements Runtime using the itsacoo operator and COOWorkspace CRs.
type K8sRuntime struct {
	cfg             Config
	discoveryClient discovery.DiscoveryInterface
	dynamicClient   dynamic.Interface
	restConfig      *rest.Config
	namespace       string
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

	dynClient, restCfg, err := buildRuntimeClients(cfg)
	if err != nil {
		return nil, fmt.Errorf("build k8s runtime clients: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = defaultNamespace
	}

	return &K8sRuntime{
		cfg:             cfg,
		discoveryClient: dc,
		dynamicClient:   dynClient,
		restConfig:      restCfg,
		namespace:       ns,
	}, nil
}

// buildDiscoveryClient constructs a short-timeout discovery client used for
// probing whether the k8s API and COO CRDs are present.
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

// buildRuntimeClients constructs a dynamic client and REST config for runtime
// operations (no short probe timeout).
func buildRuntimeClients(cfg Config) (dynamic.Interface, *rest.Config, error) {
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

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return dynClient, restCfg, nil
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

// CreateWorkspace implements Runtime. It lists existing non-terminated
// workspaces, optionally prompts to resume one, then creates a new
// COOWorkspace CR and polls until it is Ready before exec-ing in.
func (r *K8sRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) error {
	if opts.Repo == "" && opts.Concept == "" {
		return fmt.Errorf("one of --repo or --concept is required")
	}

	// 1. List non-terminated workspaces and offer to resume.
	active, err := r.listActiveWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("list existing workspaces: %w", err)
	}

	if len(active) > 0 {
		resumed, err := r.promptResume(ctx, active)
		if err != nil {
			return err
		}
		if resumed {
			return nil
		}
	}

	// 2. Generate workspace name.
	name := fmt.Sprintf("ws-%d", time.Now().Unix())

	// 3. Determine mode and resolve image.
	mode := "freestyle"
	if opts.Concept != "" {
		mode = "handoff"
	}
	image := opts.Image
	if image == "" {
		image = defaultWorkerImage
	}

	// 4. Build and create the COOWorkspace CR.
	wsObj := buildCOOWorkspace(name, r.namespace, mode, opts, image)
	fmt.Printf("Creating workspace %s...\n", name)

	_, err = r.dynamicClient.Resource(cooWorkspaceGVR).Namespace(r.namespace).Create(
		ctx, wsObj, metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("create COOWorkspace %s: %w", name, err)
	}

	// 5. Poll until Ready.
	podName, err := r.waitForReady(ctx, name)
	if err != nil {
		return fmt.Errorf("workspace %s did not become ready: %w", name, err)
	}

	// 6. Exec into the workspace pod.
	fmt.Printf("Exec-ing into workspace pod %s...\n", podName)
	if err := r.execIntoPod(podName); err != nil {
		return fmt.Errorf("exec into workspace: %w", err)
	}

	fmt.Printf("\nResume this session: coo workspace resume %s\n", name)
	return nil
}

// listActiveWorkspaces returns COOWorkspaces whose status.phase is not
// "Terminated" (and not empty, which means not yet initialised).
func (r *K8sRuntime) listActiveWorkspaces(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := r.dynamicClient.Resource(cooWorkspaceGVR).Namespace(r.namespace).List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		return nil, err
	}

	var active []unstructured.Unstructured
	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase != "" && phase != "Terminated" {
			active = append(active, item)
		}
	}
	return active, nil
}

// promptResume prints the active workspace list and asks whether to resume one.
// Returns (true, nil) if the user chose to resume, (false, nil) to create new.
func (r *K8sRuntime) promptResume(ctx context.Context, active []unstructured.Unstructured) (bool, error) {
	fmt.Println("Existing workspaces:")
	for i, ws := range active {
		phase, _, _ := unstructured.NestedString(ws.Object, "status", "phase")
		repo, _, _ := unstructured.NestedString(ws.Object, "spec", "repo")
		fmt.Printf("  [%d] %s  phase=%s  repo=%s\n", i+1, ws.GetName(), phase, repo)
	}
	fmt.Print("Press Enter to create a new workspace, or enter a number to resume: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return false, nil
	}

	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err != nil || idx < 1 || idx > len(active) {
		return false, fmt.Errorf("invalid selection %q", input)
	}

	return true, r.ResumeWorkspace(ctx, active[idx-1].GetName())
}

// waitForReady polls the COOWorkspace until status.phase == "Ready", returning
// the pod name. It times out after createReadyTimeout.
func (r *K8sRuntime) waitForReady(ctx context.Context, name string) (string, error) {
	return r.waitForReadyWithInterval(ctx, name, createPollInterval)
}

// waitForReadyWithInterval is the testable core of waitForReady, accepting a
// configurable poll interval.
func (r *K8sRuntime) waitForReadyWithInterval(ctx context.Context, name string, interval time.Duration) (string, error) {
	deadline := time.Now().Add(createReadyTimeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Print("Waiting for workspace to be ready")

	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			return "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				fmt.Println()
				return "", fmt.Errorf("timed out after %s waiting for workspace to be ready", createReadyTimeout)
			}

			ws, err := r.dynamicClient.Resource(cooWorkspaceGVR).Namespace(r.namespace).Get(
				ctx, name, metav1.GetOptions{},
			)
			if err != nil {
				fmt.Print(".")
				continue
			}

			phase, _, _ := unstructured.NestedString(ws.Object, "status", "phase")
			switch phase {
			case "Ready":
				fmt.Println(" Ready!")
				podName, _, _ := unstructured.NestedString(ws.Object, "status", "podName")
				return podName, nil
			case "Failed", "Error":
				fmt.Println()
				msg, _, _ := unstructured.NestedString(ws.Object, "status", "message")
				if msg != "" {
					return "", fmt.Errorf("workspace entered %s phase: %s", phase, msg)
				}
				return "", fmt.Errorf("workspace entered %s phase", phase)
			default:
				fmt.Print(".")
			}
		}
	}
}

// execIntoPod runs kubectl exec -it into the workspace container.
func (r *K8sRuntime) execIntoPod(podName string) error {
	args := []string{
		"exec", "-it", podName,
		"-n", r.namespace,
		"-c", workspaceContainer,
		"--",
		"bash", "-c", "cd /workspace && claude --dangerously-skip-permissions",
	}

	if r.cfg.Kubeconfig != "" {
		args = append([]string{"--kubeconfig", r.cfg.Kubeconfig}, args...)
	}
	if r.cfg.KubeContext != "" {
		args = append([]string{"--context", r.cfg.KubeContext}, args...)
	}

	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildCOOWorkspace constructs the unstructured COOWorkspace object for creation.
func buildCOOWorkspace(name, namespace, mode string, opts CreateOptions, image string) *unstructured.Unstructured {
	model := opts.Model
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	ttl := opts.TTL
	if ttl == "" {
		ttl = "4h"
	}

	spec := map[string]interface{}{
		"mode":            mode,
		"model":           model,
		"ttl":             ttl,
		"image":           image,
		"imagePullPolicy": "IfNotPresent",
	}
	if opts.Repo != "" {
		spec["repo"] = opts.Repo
	}
	if opts.Concept != "" {
		spec["conceptRef"] = opts.Concept
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
			"kind":       "COOWorkspace",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
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
