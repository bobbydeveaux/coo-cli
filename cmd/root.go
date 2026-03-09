package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfig  string
	kubecontext string
	namespace   string
	localMode   bool
)

var rootCmd = &cobra.Command{
	Use:   "coo",
	Short: "CLI for itsacoo — the Kubernetes-native AI software development operator",
	Long: `coo is the command-line interface for itsacoo (Code Orchestrator Operator).

Manage workspaces, concepts, sprints, and workers from your terminal.

Examples:
  coo workspace create --repo owner/repo
  coo workspace create --concept my-concept
  coo workspace list
  coo workspace resume ws-1234567890`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&kubecontext, "context", "", "Kubernetes context to use")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "coo-system", "Namespace where the COO operator is installed")
	rootCmd.PersistentFlags().BoolVar(&localMode, "local", false, "Force local Docker mode (skip k8s auto-detection)")

	rootCmd.AddCommand(workspaceCmd)
}
