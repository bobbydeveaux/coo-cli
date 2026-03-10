// Package handoff implements the COO handoff context injection for workspace pods.
// It fetches all relevant CRD resources from the itsacoo operator, renders a
// CLAUDE.md header, and prepends it to the existing /workspace/CLAUDE.md in
// the target pod.
package handoff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	cooAPIGroup   = "coo.itsacoo.com"
	cooAPIVersion = "v1alpha1"
	cooSystem     = "coo-system"
)

// GVRs for all COO resource types used during handoff.
var (
	conceptGVR = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cooconcepts"}
	planGVR    = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cooplans"}
	sprintGVR  = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "coosprints"}
	featureGVR = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "coofeatures"}
	taskGVR    = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cootasks"}
	workerGVR  = schema.GroupVersionResource{Group: cooAPIGroup, Version: cooAPIVersion, Resource: "cooworkers"}
)

// ClientConfig holds the parameters needed to build a Kubernetes client.
type ClientConfig struct {
	Kubeconfig  string
	KubeContext string
	Namespace   string
}

// HandoffContext holds all CRD data fetched for a given concept.
type HandoffContext struct {
	Concept  *unstructured.Unstructured
	Plan     *unstructured.Unstructured
	Sprints  []unstructured.Unstructured
	Features []unstructured.Unstructured
	Tasks    []unstructured.Unstructured
	Workers  []unstructured.Unstructured
}

// Injector fetches CRD data and injects the rendered CLAUDE.md into a pod.
type Injector struct {
	dynClient   dynamic.Interface
	cfg         ClientConfig
	namespace   string
}

// NewInjector creates an Injector using the provided client configuration.
func NewInjector(cfg ClientConfig) (*Injector, error) {
	dynClient, err := buildDynamicClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("build k8s client for handoff: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = cooSystem
	}

	return &Injector{
		dynClient: dynClient,
		cfg:       cfg,
		namespace: ns,
	}, nil
}

// FetchContext fetches all CRD resources for the named concept and returns a
// HandoffContext populated with the fetched data.
func (inj *Injector) FetchContext(ctx context.Context, conceptName string) (*HandoffContext, error) {
	concept, err := inj.fetchConcept(ctx, conceptName)
	if err != nil {
		return nil, err
	}

	conceptNS := "coo-" + conceptName

	plan, err := inj.fetchPlan(ctx, conceptNS)
	if err != nil {
		// Plan may not exist yet; proceed without it.
		fmt.Fprintf(os.Stderr, "warning: could not fetch COOPlan in %s: %v\n", conceptNS, err)
	}

	sprints, err := inj.dynClient.Resource(sprintGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list COOSprints in %s: %v\n", conceptNS, err)
	}

	features, err := inj.dynClient.Resource(featureGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list COOFeatures in %s: %v\n", conceptNS, err)
	}

	tasks, err := inj.dynClient.Resource(taskGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list COOTasks in %s: %v\n", conceptNS, err)
	}

	workers, err := inj.dynClient.Resource(workerGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list COOWorkers in %s: %v\n", conceptNS, err)
	}

	hc := &HandoffContext{
		Concept: concept,
		Plan:    plan,
	}
	if sprints != nil {
		hc.Sprints = sprints.Items
	}
	if features != nil {
		hc.Features = features.Items
	}
	if tasks != nil {
		hc.Tasks = tasks.Items
	}
	if workers != nil {
		hc.Workers = workers.Items
	}

	return hc, nil
}

// InjectIntoPod renders the CLAUDE.md header and prepends it to
// /workspace/CLAUDE.md inside the named pod/container via kubectl exec.
func (inj *Injector) InjectIntoPod(ctx context.Context, podName, containerName, conceptName string) error {
	hc, err := inj.FetchContext(ctx, conceptName)
	if err != nil {
		return fmt.Errorf("fetch handoff context: %w", err)
	}

	rendered, err := RenderTemplate(hc)
	if err != nil {
		return fmt.Errorf("render handoff template: %w", err)
	}

	return inj.prependCLAUDEMD(podName, containerName, rendered)
}

// fetchConcept retrieves the named COOConcept from coo-system.
func (inj *Injector) fetchConcept(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	obj, err := inj.dynClient.Resource(conceptGVR).Namespace(cooSystem).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get COOConcept %q in %s: %w", name, cooSystem, err)
	}
	return obj, nil
}

// fetchPlan retrieves the first COOPlan found in the given concept namespace.
// The COO operator creates exactly one COOPlan per concept namespace.
func (inj *Injector) fetchPlan(ctx context.Context, conceptNS string) (*unstructured.Unstructured, error) {
	list, err := inj.dynClient.Resource(planGVR).Namespace(conceptNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list COOPlans in %s: %w", conceptNS, err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no COOPlan found in %s", conceptNS)
	}
	return &list.Items[0], nil
}

// prependCLAUDEMD prepends the rendered header to /workspace/CLAUDE.md inside
// the pod by running a short shell script via kubectl exec.
//
// Shell script logic:
//  1. If /workspace/CLAUDE.md exists, copy it to CLAUDE.md.original
//  2. Write the new header as CLAUDE.md
//  3. Append a separator and the original content (if any)
//
// The single-quoted heredoc (<<'HANDOFF_EOF') is used so that the rendered
// header is written literally without any shell variable expansion.
func (inj *Injector) prependCLAUDEMD(podName, containerName, header string) error {
	shellCmd := fmt.Sprintf(`
set -e
ORIG=/workspace/CLAUDE.md
if [ -f "$ORIG" ]; then
  cp "$ORIG" /workspace/CLAUDE.md.original
fi
cat > /workspace/CLAUDE.md << 'HANDOFF_EOF'
%s
HANDOFF_EOF
if [ -f /workspace/CLAUDE.md.original ]; then
  printf '\n\n---\n\n> **Historical Reference Only** — The section below is the original CLAUDE.md preserved for context. The section above supersedes it.\n\n' >> /workspace/CLAUDE.md
  cat /workspace/CLAUDE.md.original >> /workspace/CLAUDE.md
fi
`, header)

	args := inj.kubectlArgs([]string{
		"exec", podName,
		"-n", inj.namespace,
		"-c", containerName,
		"--", "bash", "-c", shellCmd,
	})

	cmd := exec.Command("kubectl", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("inject CLAUDE.md into pod %s: %w\n%s", podName, err, out.String())
	}
	return nil
}

// kubectlArgs prepends --kubeconfig / --context flags if set.
func (inj *Injector) kubectlArgs(args []string) []string {
	var prefix []string
	if inj.cfg.Kubeconfig != "" {
		prefix = append(prefix, "--kubeconfig", inj.cfg.Kubeconfig)
	}
	if inj.cfg.KubeContext != "" {
		prefix = append(prefix, "--context", inj.cfg.KubeContext)
	}
	return append(prefix, args...)
}

// buildDynamicClient constructs a dynamic Kubernetes client from the given config.
func buildDynamicClient(cfg ClientConfig) (dynamic.Interface, error) {
	restCfg, err := buildRESTConfig(cfg)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(restCfg)
}

// buildRESTConfig builds a *rest.Config honouring kubeconfig / context overrides.
func buildRESTConfig(cfg ClientConfig) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		rules.ExplicitPath = cfg.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if cfg.KubeContext != "" {
		overrides.CurrentContext = cfg.KubeContext
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	return restCfg, nil
}
