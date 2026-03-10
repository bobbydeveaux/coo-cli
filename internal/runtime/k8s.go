package runtime

import (
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

	"github.com/bobbydeveaux/coo-cli/internal/handoff"
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
		return nil, fmt.Errorf("build k8s clients: %w", err)
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

	dc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}

	return dc, nil
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

// ListWorkspaces implements Runtime.
// Returns all COOWorkspaces in the configured namespace with a non-Terminated phase.
func (r *K8sRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	list, err := r.dynamicClient.Resource(cooWorkspaceGVR).
		Namespace(r.namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list COOWorkspaces in %s: %w", r.namespace, err)
	}

	var workspaces []WorkspaceInfo
	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if strings.EqualFold(phase, "Terminated") {
			continue
		}

		mode, _, _ := unstructured.NestedString(item.Object, "spec", "mode")
		podName, _, _ := unstructured.NestedString(item.Object, "status", "podName")
		repo, _, _ := unstructured.NestedString(item.Object, "spec", "repo")
		ttl, _, _ := unstructured.NestedString(item.Object, "spec", "ttl")

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
// Generates a ws-<timestamp> name, creates the COOWorkspace CR, polls until Ready,
// and returns the workspace name.
func (r *K8sRuntime) CreateWorkspace(ctx context.Context, opts CreateOptions) (string, error) {
	if opts.Repo == "" && opts.Concept == "" {
		return "", fmt.Errorf("one of --repo or --concept is required")
	}

	name := fmt.Sprintf("ws-%d", time.Now().Unix())

	mode := "freestyle"
	if opts.Concept != "" {
		mode = "handoff"
	}

	image := opts.Image
	if image == "" {
		image = defaultWorkerImage
	}

	wsObj := buildCOOWorkspace(name, r.namespace, mode, opts, image)
	fmt.Printf("Creating workspace %s...\n", name)

	_, err := r.dynamicClient.Resource(cooWorkspaceGVR).Namespace(r.namespace).Create(
		ctx, wsObj, metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("create COOWorkspace %s: %w", name, err)
	}

	podName, err := r.waitForReady(ctx, name)
	if err != nil {
		return name, fmt.Errorf("workspace %s did not become ready: %w", name, err)
	}

	// In handoff mode, inject the COO context into /workspace/CLAUDE.md.
	if mode == "handoff" && opts.Concept != "" {
		if injectErr := r.injectHandoffContext(ctx, podName, opts.Concept); injectErr != nil {
			// Non-fatal: warn and continue; the workspace is still usable.
			fmt.Fprintf(os.Stderr, "Warning: handoff context injection failed: %v\n", injectErr)
		}
	}

	return name, nil
}

// injectHandoffContext fetches COO CRD data for the named concept, renders the
// handoff CLAUDE.md, and writes it into the workspace pod.
func (r *K8sRuntime) injectHandoffContext(ctx context.Context, podName, conceptName string) error {
	fmt.Printf("Injecting handoff context for concept %s...\n", conceptName)

	data, err := handoff.FetchHandoffData(ctx, r.dynamicClient, r.namespace, conceptName)
	if err != nil {
		return fmt.Errorf("fetch handoff data: %w", err)
	}

	content, err := handoff.Render(data)
	if err != nil {
		return fmt.Errorf("render handoff template: %w", err)
	}

	if err := handoff.InjectCLAUDEMD(content, podName, r.namespace, r.cfg.Kubeconfig, r.cfg.KubeContext); err != nil {
		return fmt.Errorf("inject CLAUDE.md: %w", err)
	}

	fmt.Println("Handoff context injected successfully.")
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

// ExecWorkspace implements Runtime.
// Resolves the pod name and drops the user into Claude Code in the workspace container.
func (r *K8sRuntime) ExecWorkspace(ctx context.Context, name string) error {
	podName, err := r.getPodName(ctx, name)
	if err != nil {
		return err
	}

	return r.execIntoPod(podName,
		"git config --global --add safe.directory '*' && cd /workspace && claude --dangerously-skip-permissions")
}

// ResumeWorkspace implements Runtime.
// Finds the last Claude Code session ID in the pod and execs in with --resume.
func (r *K8sRuntime) ResumeWorkspace(ctx context.Context, name string) error {
	podName, err := r.getPodName(ctx, name)
	if err != nil {
		return err
	}

	sessionID, err := r.findLastSessionID(podName)
	if err != nil {
		// No session found — fall back to a fresh exec.
		fmt.Fprintf(os.Stderr, "No previous session found (%v); starting fresh.\n", err)
		return r.ExecWorkspace(ctx, name)
	}

	return r.execIntoPod(podName,
		fmt.Sprintf("cd /workspace && claude --dangerously-skip-permissions --resume %s", sessionID))
}

// DeleteWorkspace implements Runtime.
func (r *K8sRuntime) DeleteWorkspace(ctx context.Context, name string) error {
	err := r.dynamicClient.Resource(cooWorkspaceGVR).
		Namespace(r.namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete COOWorkspace %s: %w", name, err)
	}
	return nil
}

// getPodName reads status.podName from the named COOWorkspace CR.
func (r *K8sRuntime) getPodName(ctx context.Context, name string) (string, error) {
	ws, err := r.dynamicClient.Resource(cooWorkspaceGVR).
		Namespace(r.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get workspace %s: %w", name, err)
	}

	podName, found, err := unstructured.NestedString(ws.Object, "status", "podName")
	if err != nil || !found || podName == "" {
		return "", fmt.Errorf("workspace %s has no pod assigned (status.podName is empty)", name)
	}
	return podName, nil
}

// findLastSessionID discovers the most recent Claude Code session in the pod by
// listing JSONL files under /tmp/.claude/projects/ via kubectl exec.
func (r *K8sRuntime) findLastSessionID(podName string) (string, error) {
	args := []string{
		"exec", podName,
		"-n", r.namespace,
		"-c", workspaceContainer,
		"--", "bash", "-c", "ls -t /tmp/.claude/projects/*/*.jsonl 2>/dev/null | head -1",
	}

	if r.cfg.Kubeconfig != "" {
		args = append([]string{"--kubeconfig", r.cfg.Kubeconfig}, args...)
	}
	if r.cfg.KubeContext != "" {
		args = append([]string{"--context", r.cfg.KubeContext}, args...)
	}

	out, err := exec.Command("kubectl", args...).Output()
	if err != nil {
		return "", fmt.Errorf("list session files: %w", err)
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("no session files found")
	}

	// The session ID is the filename without the .jsonl extension.
	parts := strings.Split(path, "/")
	file := parts[len(parts)-1]
	sessionID := strings.TrimSuffix(file, ".jsonl")
	if sessionID == "" {
		return "", fmt.Errorf("could not extract session ID from path %q", path)
	}
	return sessionID, nil
}

// execIntoPod runs kubectl exec -it into the workspace container with the given shell command.
func (r *K8sRuntime) execIntoPod(podName, shellCmd string) error {
	args := []string{
		"exec", "-it", podName,
		"-n", r.namespace,
		"-c", workspaceContainer,
		"--",
		"bash", "-c", shellCmd,
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
