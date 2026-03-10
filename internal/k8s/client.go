package k8s

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// COOGroup is the API group for all COO custom resources.
	COOGroup = "coo.itsacoo.com"
	// COOVersion is the API version for COO custom resources.
	COOVersion = "v1alpha1"
)

// Config holds options for building the Kubernetes client.
type Config struct {
	// Kubeconfig is an explicit path to the kubeconfig file.
	// Defaults to $KUBECONFIG env var, then ~/.kube/config.
	Kubeconfig string
	// Context is the kubeconfig context to activate.
	// Defaults to the current context.
	Context string
}

// Client bundles a REST config, dynamic client, discovery client, and typed clientset.
type Client struct {
	RestConfig *rest.Config
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
	Clientset  kubernetes.Interface
}

// New creates a Kubernetes Client from the given Config.
//
// Kubeconfig resolution order:
//  1. cfg.Kubeconfig  (from --kubeconfig flag)
//  2. KUBECONFIG environment variable
//  3. ~/.kube/config  (standard default)
func New(cfg Config) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		overrides.CurrentContext = cfg.Context
	}

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restCfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	discClient, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}

	kclient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes clientset: %w", err)
	}

	return &Client{
		RestConfig: restCfg,
		Dynamic:    dynClient,
		Discovery:  discClient,
		Clientset:  kclient,
	}, nil
}

// Ping verifies the API server is reachable by fetching the server version.
func (c *Client) Ping() error {
	_, err := c.Discovery.ServerVersion()
	if err != nil {
		return fmt.Errorf("ping k8s API server: %w", err)
	}
	return nil
}

// HasCOOCRD reports whether the itsacoo operator CRDs are registered in the
// cluster by probing the coo.itsacoo.com/v1alpha1 API group via discovery.
//
// A missing API group returns (false, nil). Other errors are propagated.
func (c *Client) HasCOOCRD(_ context.Context) (bool, error) {
	_, err := c.Discovery.ServerResourcesForGroupVersion(COOGroup + "/" + COOVersion)
	if err != nil {
		if isNotFoundErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("probe COO CRD group %s: %w", COOGroup, err)
	}
	return true, nil
}

// isNotFoundErr returns true when the error indicates the API group/version
// does not exist on the server. client-go returns a *errors.StatusError (404)
// for missing groups; we also guard against string-matching for older clusters
// that may use different error paths.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "the server could not find the requested resource") ||
		strings.Contains(msg, "no kind is registered")
}
