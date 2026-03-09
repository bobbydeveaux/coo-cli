package cmd

import (
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage COO workspaces",
	Long:  `Create, list, exec into, resume, and delete COO interactive workspaces.`,
}

func init() {
	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceExecCmd)
	workspaceCmd.AddCommand(workspaceResumeCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new workspace and exec into it",
	Long: `Create a COOWorkspace and drop into Claude Code inside the cluster.

Freestyle mode (--repo): clone a repo and start from a blank slate.
Handoff mode (--concept): pick up where AI workers left off, with full
COO context injected into the workspace CLAUDE.md.`,
	Example: `  # Freestyle
  coo workspace create --repo bobbydeveaux/my-project

  # Handoff — auto-detects repo from COOConcept
  coo workspace create --concept my-concept`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		cmd.Println("workspace create — not yet implemented")
		return nil
	},
}

var workspaceListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List active workspaces",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		cmd.Println("workspace list — not yet implemented")
		return nil
	},
}

var workspaceExecCmd = &cobra.Command{
	Use:   "exec <name>",
	Short: "Exec into an existing workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		cmd.Printf("workspace exec %s — not yet implemented\n", args[0])
		return nil
	},
}

var workspaceResumeCmd = &cobra.Command{
	Use:   "resume <name>",
	Short: "Resume the last Claude Code session in a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		cmd.Printf("workspace resume %s — not yet implemented\n", args[0])
		return nil
	},
}

var workspaceDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Short:   "Delete a workspace",
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		cmd.Printf("workspace delete %s — not yet implemented\n", args[0])
		return nil
	},
}

func init() {
	workspaceCreateCmd.Flags().String("repo", "", "Repository to clone (owner/repo) — freestyle mode")
	workspaceCreateCmd.Flags().String("concept", "", "COO concept name — handoff mode (auto-detects repo, k8s only)")
	workspaceCreateCmd.Flags().String("model", "claude-sonnet-4-5", "AI model to use")
	workspaceCreateCmd.Flags().String("ttl", "4h", "Workspace TTL (e.g. 4h, 24h) — k8s mode only")
	workspaceCreateCmd.Flags().String("image", "", "Worker image override (default: ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest)")
	workspaceCreateCmd.Flags().String("token", "", "Claude Code OAuth token (default: $CLAUDE_CODE_OAUTH_TOKEN)")
	workspaceCreateCmd.Flags().String("github-token", "", "GitHub token for private repos (default: $GITHUB_TOKEN)")
}
