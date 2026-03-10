The LLD has been written to `docs/concepts/coo-cli-workspace/LLD.md`. Here's a summary of what it covers:

**Key design decisions captured:**

- **File structure** — 11 new files under `internal/` (runtime, k8s, workspace, handoff packages), with `cmd/workspace.go` as the only modified existing file
- **Component designs** — detailed flows for both k8s and local runtimes, including the clone-into-volume strategy, token resolution priority chains, and SPDY exec-based CLAUDE.md injection
- **Function signatures** — complete signatures for all exported and key unexported functions across every package
- **State management** — k8s mode is stateless (API server is source of truth); local mode uses atomic JSON file writes to `~/.coo/workspaces.json`
- **Error handling** — specific user-facing error messages for each failure mode, `SilenceUsage: true` on cobra root, exit code propagation from container exec
- **Test plan** — unit tests for token resolution/state logic, integration tests with fake k8s client and real Docker (CI DinD), E2E tests gated behind `//go:build e2e`
- **Migration** — purely additive to the existing skeleton; subcommands can be shipped one at a time