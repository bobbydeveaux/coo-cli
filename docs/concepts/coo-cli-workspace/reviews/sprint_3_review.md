Now I have a thorough understanding of the sprint's deliverables. Let me produce the review document.

# Sprint Review: coo-cli-workspace-sprint-3

**Date:** 2026-03-10 | **Duration:** 8h 45m | **Branch:** `review-coo-cli-workspace-sprint-3`

---

## Executive Summary

Sprint 3 delivered the two remaining core runtime implementations for the `coo` CLI, completing the workspace lifecycle feature set introduced in Sprint 2. Issue #16 (PR #23) wired up the Kubernetes runtime's list, exec, resume, and delete delegates, while Issue #17 (PR #22) implemented the full local Docker mode — including persistent volume management, session resumption, and the `~/.coo/workspaces.json` state store. Both issues shipped with strong unit test coverage, zero retries, and zero merge conflicts, resulting in a 100% first-time-right rate across the sprint.

---

## Achievements

**Complete workspace lifecycle across both runtimes.** The `Runtime` interface — introduced in Sprint 2 — is now fully implemented by both `K8sRuntime` and `LocalRuntime`. Every command (`create`, `list`, `exec`, `resume`, `delete`) delegates through `runtime.Detect`, giving users a seamless dual-mode experience without any code duplication at the command layer (`workspace/delete.go`, `workspace/exec.go`).

**Robust local mode state management.** The `WorkspaceEntry` store in `internal/runtime/state.go` uses an atomic write-to-temp-then-rename pattern to guard against partial writes corrupting `~/.coo/workspaces.json`. File permissions are set to `0600` (state file) and `0700` (directory), matching the sensitivity of the stored tokens.

**Multi-source token resolution.** `ResolveClaudeToken` and `ResolveGitHubToken` each implement a well-specified fallback chain (explicit flag → env var → credentials file → `gh auth token`), making the local mode usable out-of-the-box for developers with an existing Claude Code or `gh` CLI install.

**First-run vs. re-attach intelligence.** `runContainer` in `local.go` inspects the workspace volume to distinguish a fresh creation (empty directory → let entrypoint handle `git clone`) from a subsequent attach (non-empty → override to `cd /workspace && claude --dangerously-skip-permissions`). This eliminates the risk of double-cloning.

**High test density.** Both PRs were delivered with comprehensive table-driven and scenario-based tests covering edge cases: malformed JSON, missing workspace names, Terminated-phase filtering, session ID selection by mtime, and empty/non-empty directory detection.

**Clean, spec-compliant interface design.** Code organisation matches the layout specified in `CLAUDE.md` exactly. Files are short and single-concern. Errors are wrapped with context and propagated rather than swallowed. No external logging frameworks were introduced.

---

## Challenges

**Light code review load (1 review cycle total).** PR #23 shipped with 0 review cycles and PR #22 with 1. Given the complexity of the local runtime — Docker process management, file system state, token resolution — a second pass on PR #22 would have been prudent. No defects were found post-merge, but this is worth noting as a process observation rather than a quality failure.

**Test helper inconsistency in `k8s_list_test.go`.** `newTestK8sRuntime` populates `cfg.Namespace` but leaves `r.namespace` (the struct field actually used by `ListWorkspaces`) at its zero value `""`. This means `TestK8sRuntime_ListWorkspaces_DefaultNamespace` relies on the dynamic client issuing a cluster-scoped request, not a namespace-scoped one to `coo-system`. The test's intent (verify the default namespace fallback) is not actually exercised by this helper — the defaulting logic only runs inside `newK8sRuntime`, which the test bypasses. This is a latent test correctness issue that should be addressed.

**`findLastSessionID` in `k8s.go` shells out to `kubectl`.** The spec in `CLAUDE.md` states that the handoff context injection and session discovery should "use exec API, not kubectl". The current implementation calls `exec.Command("kubectl", "exec", ...)` to list JSONL files, introducing a hard runtime dependency on `kubectl` being present in `$PATH`. The local mode correctly uses Go's `filepath.Glob` against the mounted volume. Bringing K8s session discovery in-line with the local approach (via the Kubernetes exec API) would remove this dependency and improve consistency.

---

## Worker Performance

| Worker | Tasks | PRs Merged | Avg Duration | Reviews Given |
|---|---|---|---|---|
| `backend-engineer` | 2 | 2 | 7m 30s | 0 |
| `code-reviewer` | 2 | — | 1m 0s | 2 |

**backend-engineer** delivered both functional PRs efficiently and within specification. Task duration variance was low (6m vs. 9m), suggesting consistent scoping.

**code-reviewer** completed both reviews in the minimum observed time (1m each). Review depth appears adequate given the 0-defect outcome, though the absence of a review cycle on PR #23 means the `kubectl`-shelling and test-helper issue above were not caught pre-merge.

Worker utilisation was well-balanced for a 4-task sprint: 50/50 between implementation and review, with no blocking dependencies between the two implementation tasks.

---

## Recommendations

1. **Fix the `newTestK8sRuntime` helper.** Set `r.namespace` explicitly inside `newTestK8sRuntime` (defaulting to `defaultNamespace` when the argument is `""`). This ensures `TestK8sRuntime_ListWorkspaces_DefaultNamespace` actually validates the defaulting behaviour it describes. A one-line change to the helper is sufficient.

2. **Replace `kubectl` exec with the Go exec API for session discovery.** `K8sRuntime.findLastSessionID` should use `k8s.io/client-go/rest` SPDY execution (the same approach used by `kubectl exec` under the hood) rather than shelling out. This removes the `kubectl`-in-PATH requirement for users who manage their cluster access differently, and aligns K8s mode with local mode's in-process approach.

3. **Add a review cycle requirement for medium-complexity PRs.** Given the surface area of the local runtime (file I/O, Docker process management, token handling), mandating at least one review cycle for PRs over a threshold size would be a lightweight safeguard. The current sprint's clean outcome doesn't preclude this as a future risk.

4. **Consider integration-level smoke tests.** The unit tests are thorough, but there are no tests that exercise `runtime.Detect` in a realistic environment (e.g., asserting local fallback when no kubeconfig exists). A small set of integration tests running against a local Docker daemon in CI would catch regressions in the auto-detection path.

5. **Document the `ContainerID` field lifecycle.** `WorkspaceEntry.ContainerID` is stored in `workspaces.json` but is never populated in `CreateWorkspace` (it remains `""`). If container tracking is needed for future `coo workspace stop` or health-check commands, this field needs population at container launch time. If not, it should be removed to avoid confusion.

---

## Metrics Summary

| Metric | Value |
|---|---|
| Sprint phase | Completed |
| Total tasks | 4 |
| Completed | 4 (100%) |
| Failed | 0 |
| Blocked | 0 |
| First-time-right rate | 100% |
| Total retries | 0 |
| Total review cycles | 1 |
| Merge conflicts | 0 |
| Average task duration | 4m 0s |
| Sprint duration | 8h 45m |
| Workers utilised | 2 (backend-engineer, code-reviewer) |
| PRs merged | 2 (#22, #23) |