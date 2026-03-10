package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	k8sclient "github.com/bobbydeveaux/coo-cli/internal/k8s"
)

const (
	cooAPIGroup     = "coo.itsacoo.com"
	k8sProbeTimeout = 5 * time.Second
	createTimeout   = 120 * time.Second
	pollInterval    = 2 * time.Second
)

var cooWorkspaceGVR = schema.GroupVersionResource{
	Group:    "coo.itsacoo.com",
	Version:  "v1alpha1",
	Resource: "cooworkspaces",
}

// K8sRuntime implements Runtime using the itsacoo operator and COOWorkspace CRs.
type K8sRuntime struct {
	cfg    Config
	client *k8sclient.Client
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

	// Build the full operations client (no short timeout).
	c, err := k8sclient.New(k8sclient.Config{
		Kubeconfig: cfg.Kubeconfig,
		Context:    cfg.KubeContext,
	})
	if err != nil {
		return nil, fmt.Errorf("build k8s operations client: %w", err)
	}

	return &K8sRuntime{cfg: cfg, client: c}, nil
}

// buildDiscoveryClient constructs a discovery client with a short timeout for probing.
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

// ListWorkspaces implements Runtime.
// Returns all COOWorkspaces in the configured namespace with a non-Terminated phase.
func (r *K8sRuntime) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	list, err := r.client.Dynamic.Resource(cooWorkspaceGVR).
		Namespace(r.cfg.Namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list COOWorkspaces in %s: %w", r.cfg.Namespace, err)
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
	name := fmt.Sprintf("ws-%d", time.Now().Unix())

	mode := "freestyle"
	if opts.Concept != "" {
		mode = "handoff"
	}

	image := opts.Image
	if image == "" {
		image = "ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest"
	}

	ws := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "coo.itsacoo.com/v1alpha1",
			"kind":       "COOWorkspace",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": r.cfg.Namespace,
			},
			"spec": map[string]interface{}{
				"mode":            mode,
				"repo":            opts.Repo,
				"conceptRef":      opts.Concept,
				"model":           opts.Model,
				"ttl":             opts.TTL,
				"image":           image,
				"imagePullPolicy": "IfNotPresent",
			},
		},
	}

	_, err := r.client.Dynamic.Resource(cooWorkspaceGVR).
		Namespace(r.cfg.Namespace).
		Create(ctx, ws, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create COOWorkspace %s: %w", name, err)
	}

	fmt.Fprintf(os.Stderr, "Workspace %s created. Waiting for pod to be ready", name)
	if err := r.waitForReady(ctx, name); err != nil {
		fmt.Fprintln(os.Stderr)
		return name, fmt.Errorf("workspace %s did not become ready: %w", name, err)
	}
	fmt.Fprintln(os.Stderr, " ready.")

	return name, nil
}

// waitForReady polls status.phase until "Ready" or the 120s deadline is exceeded.
func (r *K8sRuntime) waitForReady(ctx context.Context, name string) error {
	deadline := time.Now().Add(createTimeout)
	spinner := []string{"|", "/", "-", "\\"}
	tick := 0

	for time.Now().Before(deadline) {
		ws, err := r.client.Dynamic.Resource(cooWorkspaceGVR).
			Namespace(r.cfg.Namespace).
			Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get workspace status: %w", err)
		}

		phase, _, _ := unstructured.NestedString(ws.Object, "status", "phase")
		if strings.EqualFold(phase, "Ready") {
			return nil
		}

		fmt.Fprintf(os.Stderr, "\r  %s waiting (phase: %s)...   ", spinner[tick%len(spinner)], phase)
		tick++
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timed out after %s", createTimeout)
}

// ExecWorkspace implements Runtime.
// Resolves the pod name and drops the user into Claude Code in the workspace container.
func (r *K8sRuntime) ExecWorkspace(ctx context.Context, name string) error {
	podName, err := r.getPodName(ctx, name)
	if err != nil {
		return err
	}

	command := []string{
		"bash", "-c",
		`git config --global --add safe.directory '*' && cd /workspace && claude --dangerously-skip-permissions`,
	}
	return r.execIntoPod(ctx, podName, command)
}

// ResumeWorkspace implements Runtime.
// Finds the last Claude Code session ID in the pod and execs in with --resume.
func (r *K8sRuntime) ResumeWorkspace(ctx context.Context, name string) error {
	podName, err := r.getPodName(ctx, name)
	if err != nil {
		return err
	}

	sessionID, err := r.findLastSessionID(ctx, podName)
	if err != nil {
		// No session found — fall back to a fresh exec.
		fmt.Fprintf(os.Stderr, "No previous session found (%v); starting fresh.\n", err)
		return r.ExecWorkspace(ctx, name)
	}

	command := []string{
		"bash", "-c",
		fmt.Sprintf(`cd /workspace && claude --dangerously-skip-permissions --resume %s`, sessionID),
	}
	return r.execIntoPod(ctx, podName, command)
}

// DeleteWorkspace implements Runtime.
func (r *K8sRuntime) DeleteWorkspace(ctx context.Context, name string) error {
	err := r.client.Dynamic.Resource(cooWorkspaceGVR).
		Namespace(r.cfg.Namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete COOWorkspace %s: %w", name, err)
	}
	return nil
}

// getPodName reads status.podName from the named COOWorkspace CR.
func (r *K8sRuntime) getPodName(ctx context.Context, name string) (string, error) {
	ws, err := r.client.Dynamic.Resource(cooWorkspaceGVR).
		Namespace(r.cfg.Namespace).
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
// listing JSONL files under /tmp/.claude/projects/ and extracting the filename stem.
func (r *K8sRuntime) findLastSessionID(ctx context.Context, podName string) (string, error) {
	var outBuf, errBuf bytes.Buffer
	err := r.execCapture(ctx, podName, []string{
		"bash", "-c", `ls -t /tmp/.claude/projects/*/*.jsonl 2>/dev/null | head -1`,
	}, &outBuf, &errBuf)
	if err != nil {
		return "", fmt.Errorf("list session files: %w", err)
	}

	path := strings.TrimSpace(outBuf.String())
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

// execIntoPod runs an interactive (TTY) exec into the workspace container.
func (r *K8sRuntime) execIntoPod(ctx context.Context, podName string, command []string) error {
	req := r.client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(r.cfg.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "workspace",
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, clientgoscheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(r.client.RestConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("create SPDY executor: %w", err)
	}

	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
	if streamErr != nil && !isRemoteExitError(streamErr) {
		return fmt.Errorf("exec stream: %w", streamErr)
	}
	return nil
}

// execCapture runs a non-interactive command and captures stdout/stderr into buffers.
func (r *K8sRuntime) execCapture(ctx context.Context, podName string, command []string, stdout, stderr *bytes.Buffer) error {
	req := r.client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(r.cfg.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "workspace",
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, clientgoscheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(r.client.RestConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("create SPDY executor: %w", err)
	}

	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
}

// isRemoteExitError returns true when err represents a non-zero remote process exit,
// which we treat as a normal exit rather than a transport failure.
func isRemoteExitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "command terminated with exit code")
}
