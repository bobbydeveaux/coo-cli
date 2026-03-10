# ROAM Analysis: coo-cli-workspace

**Feature Count:** 9
**Created:** 2026-03-09T22:31:35Z

## Risks

1. **Docker SDK TTY Handling** (High): The Docker SDK's `ContainerExecAttach` does not reliably handle raw TTY resize signals across all environments. The plan mitigates this with `os/exec docker exec -it`, but this introduces a hard runtime dependency on the Docker binary being in PATH — Podman and nerdctl users may face subtle compatibility issues with CLI flag differences (e.g., nerdctl's `exec` flag parity is incomplete).

2. **SPDY Exec API Stability for Handoff Injection** (High): Injecting CLAUDE.md via `client-go/tools/remotecommand` SPDY exec (`cat > /workspace/CLAUDE.md`) will silently fail if the workspace container lacks `bash` or `cat`, or if the CLAUDE.md content contains shell metacharacters that corrupt the piped stdin stream. Large handoff contexts (many tasks/sprints) could also exceed pipe buffer limits.

3. **COOWorkspace CRD Schema Drift** (Medium): The plan uses `unstructured.Unstructured` with hand-written field accessors to avoid importing the operator module directly. If the operator team changes field paths in `status.phase`, `status.podName`, or handoff CRD schemas without a coordinated update to `coo-cli`, the CLI will silently read empty/nil values and produce misleading behaviour rather than a typed compilation error.

4. **Podman/nerdctl Compatibility Gap** (Medium): The container runtime detection probes for `docker`, `podman`, `nerdctl` via `exec.LookPath`, but the `docker run` subprocess command constructed in `local.go` uses Docker-specific flags. Podman is largely compatible, but `nerdctl` has known divergences in volume mount syntax, `--rm` behaviour with `-it`, and image pull progress output. A user on nerdctl may get cryptic errors.

5. **Session ID Discovery Fragility** (Medium): Resume logic globs `*/*.jsonl` under the Claude projects directory and picks the most recently modified file to extract a session ID. If Claude Code changes its project directory structure, session file naming convention, or JSONL schema between versions baked into the worker image, resume will silently select the wrong session or return no results — with no fallback.

6. **Worker Image Availability and Latency** (Low): `coo workspace create` in local mode pulls `ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest` on first use. If GHCR is unavailable, rate-limited, or the image tag is force-pushed with a breaking change, all local `workspace create` calls will fail with a potentially opaque Docker pull error.

7. **Atomic Write Race on Multi-Shell Use** (Low): `~/.coo/workspaces.json` is written atomically via temp-file + `os.Rename`, but if two `coo workspace create` commands are invoked concurrently (e.g., split terminal), both processes will read the same stale state, each write their entry, and the last writer wins — silently dropping the first entry.

---

## Obstacles

- **No existing coo-cli skeleton to validate against**: The planning documents reference a scaffold (`cmd/workspace.go` stubs, existing `go.mod`) but the current repo state is unknown. If the module path, Go version, or existing cobra wiring differs from what the LLD assumes, the implementation will require rework before any feature code can be written.

- **COOWorkspace CRD not importable as a Go module**: ADR-001 explicitly avoids importing the operator module due to coupling risk. This means all field access must be validated manually against the live CRD schema, with no compile-time safety net. The operator repo's CRD schema has no published JSON Schema or OpenAPI spec referenced in the planning docs, making it difficult to verify accessor correctness without a running cluster.

- **Interactive TTY testing is not automatable in standard CI**: The exec/resume commands require an interactive TTY. The LLD proposes E2E tests gated behind `//go:build e2e` with Docker-in-Docker, but TTY simulation in CI (e.g., `script`, `expect`, `pty`) is fragile and platform-specific. This creates a gap where the most critical user-facing paths (entering and exiting a Claude session) may go untested in automated pipelines.

- **SPDY/WebSocket deprecation in newer Kubernetes versions**: Kubernetes 1.29+ is deprecating SPDY in favour of WebSocket-based exec. The `client-go remotecommand` package supports both, but the correct transport must be negotiated at runtime. If the target cluster runs a newer API server that has removed SPDY support, the handoff injection via exec will fail without an explicit WebSocket fallback.

---

## Assumptions

1. **The COOWorkspace CRD `status.phase` values are stable and match `Pending | Ready | Terminated`** — the 120s readiness poll and the "resume existing workspace" prompt both depend on this exact enumeration. *Validation approach:* Confirm against the operator's CRD validation schema or integration-test against a live cluster before shipping the k8s runtime.

2. **The worker image entrypoint supports being overridden with `bash -c '...'`** — all exec/resume flows assume the container can be started or exec'd into with a custom bash command. If the image uses a non-bash entrypoint or a restricted `ENTRYPOINT ["claude"]` that ignores CMD arguments, the exec flow will break silently. *Validation approach:* Run `docker run --rm <image> bash -c 'echo ok'` against the actual image to verify entrypoint override behaviour.

3. **`~/.claude/credentials.json` contains an `oauthToken` field readable without decryption** — the token fallback chain reads this file directly. If Claude Code stores credentials encrypted at rest or changes the JSON key name, the fallback will silently return an empty token and the command will fail at container start. *Validation approach:* Inspect the credentials file format on a machine with Claude Code installed before implementing the file-read fallback.

4. **The operator's `COOConcept → affectedProjects[0] → COOProject` chain is always populated for handoff mode** — the handoff context fetcher assumes `spec.affectedProjects[0]` exists and resolves to a valid COOProject with `spec.github.owner` and `spec.github.repo`. A concept with no affected projects or a dangling project reference will produce a nil-dereference or empty repo string. *Validation approach:* Add explicit nil/empty checks with actionable error messages and test against a concept in each lifecycle phase.

5. **Podman CLI is Docker-compatible enough for the `docker run`/`docker exec` subprocess calls** — the plan treats Podman as a drop-in replacement invoked via the same CLI flags. Podman 4.x has near-full compatibility but diverges on rootless networking defaults and some `--format` outputs used in container ID extraction. *Validation approach:* Run the local runtime smoke test explicitly against Podman before marking Podman support as complete.

---

## Mitigations

### Risk 1: Docker SDK TTY Handling
- Extract all interactive exec calls into a single `runInteractive(binary, args []string) error` helper in `local.go` that constructs the subprocess with inherited `os.Stdin/Stdout/Stderr` and raw terminal mode. This isolates TTY handling to one testable function.
- Add an explicit compatibility matrix in the README documenting which operations use the SDK vs subprocess, with tested versions for Docker, Podman, and nerdctl.
- For nerdctl: test `nerdctl exec -it` flag parity before the first release; add a `// TODO(nerdctl): verify flag compatibility` comment at the subprocess construction site.

### Risk 2: SPDY Exec API Stability for Handoff Injection
- Write the CLAUDE.md content to a base64-encoded string and inject via `bash -c 'echo <b64> | base64 -d > /workspace/CLAUDE.md'` to eliminate all shell metacharacter concerns.
- Add a size guard: if the rendered CLAUDE.md exceeds 512KB, log a warning and truncate the task list with a `... and N more` suffix before injection.
- Add a post-injection verification step: exec `wc -c /workspace/CLAUDE.md` in the pod and compare against the expected byte count; fail loudly if they differ.
- Track the `client-go` WebSocket exec transport (`remotecommand.NewWebSocketExecutor`) as the fallback path; implement it behind a `--exec-transport=websocket` flag for users on newer clusters.

### Risk 3: COOWorkspace CRD Schema Drift
- Define a `internal/k8s/fields.go` file with named constants for all field paths (e.g., `const FieldStatusPhase = "status.phase"`). All accessor calls in `k8s.go` and `context.go` reference these constants, making schema changes a one-line fix.
- Add a `coo version --check-crd` diagnostic subcommand (future) that fetches the live CRD OpenAPI schema and validates that all expected fields exist.
- Pin the operator version in the LLD and add a compatibility note in CLAUDE.md: "Tested against operator vX.Y.Z; re-validate field accessors on operator upgrades."

### Risk 4: Podman/nerdctl Compatibility Gap
- In the short term, gate nerdctl support behind an explicit `--runtime nerdctl` flag with a `[experimental]` label rather than auto-detecting it. Auto-detect only `docker` and `podman`.
- For Podman, test specifically: `podman run -it --rm`, `podman exec -it`, `podman stop`, `podman rm`, and `podman ps --format json`. Document any flag differences as code comments.
- Add a `detectRuntime()` function that returns not just the binary path but also a `RuntimeFlavour` enum (`Docker | Podman | Nerdctl`) used to switch flag construction where divergence exists.

### Risk 5: Session ID Discovery Fragility
- Add a `findLastSessionID(projectsDir string) (string, error)` function that globs `*/*.jsonl`, sorts by `ModTime` descending, reads the last line of the most recent file, and extracts the `sessionId` field from the JSONL record using a minimal struct unmarshal — rather than using the filename as the session ID.
- Add a clear error message when no `.jsonl` files are found: `"No previous Claude session found in <dir>. Use 'coo workspace exec <name>' to start a new session."` with a non-zero exit code.
- Document the Claude Code projects directory structure assumption in a comment so it can be quickly updated if the format changes.

### Risk 6: Worker Image Availability and Latency
- Before pulling, check if the image already exists locally via `docker image inspect` (SDK: `ImageInspectWithRaw`). Skip pull and log `"Using cached worker image"` if present.
- Surface pull errors with the full image reference and a suggestion: `"Ensure you have access to ghcr.io and are logged in: docker login ghcr.io"`.
- Add an `--image` flag override (already in the spec) and document that users can pre-pull or mirror the image: `docker pull <image> && docker tag <image> my-registry/coo-worker:latest`.

### Risk 7: Atomic Write Race on Multi-Shell Use
- Add an advisory file lock (`~/.coo/workspaces.lock`) using `flock(2)` via `syscall.Flock` around all read-modify-write operations on `workspaces.json`. Use a short timeout (500ms) before failing with `"workspace state is locked by another coo process"`.
- Document the single-user, single-process assumption explicitly in the package docstring so future contributors understand the concurrency model.

---

## Appendix: Plan Documents

### PRD
# Product Requirements Document: Implement the `coo workspace` command suite for coo-cli.

coo-cli is a Go CLI (cobra) for itsacoo — the Kubernetes-native AI development operator.
The full technical spec is in CLAUDE.md at the repo root. Read it carefully before starting.

## What to implement

### 1. Runtime detection (internal/runtime/detect.go)
Auto-detect whether to use k8s mode or local Docker mode:
- --local flag → force local Docker mode
- Try k8s API → if reachable and COOWorkspace CRD exists → k8s mode
- Otherwise → local Docker mode
- Detect container runtime: docker, podman, nerdctl (in that order)

### 2. Local Docker runtime (internal/runtime/local.go)
Implement coo workspace create/list/exec/resume/delete using Docker:
- Pull worker image: ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest
- Clone repo into ~/.coo/volumes/<ws-name>/ on host, mount as /workspace
- Bootstrap /tmp/.claude.json (onboarding complete, workspace trusted, skipDangerousModePermissionPrompt)
- Run: docker run -it with CLAUDE_CODE_OAUTH_TOKEN + GITHUB_TOKEN
- Token resolution: --token flag → CLAUDE_CODE_OAUTH_TOKEN env → ~/.claude/credentials.json
- GitHub token: --github-token flag → GITHUB_TOKEN → GH_TOKEN → `gh auth token`
- Track workspaces in ~/.coo/workspaces.json (name, containerID, repo, createdAt, volumePath)
- Resume: find last session ID from ~/.coo/volumes/<name>/.claude/projects/*/*.jsonl
- Exec back into a stopped container by starting it again

### 3. K8s runtime (internal/runtime/k8s.go)
Implement coo workspace create/list/exec/resume/delete using COOWorkspace CRs:
- Create COOWorkspace CR in coo-system namespace
- Wait for status.phase == Ready
- Exec into pod: kubectl exec -it <pod> -c workspace
- For handoff mode: fetch CRD context and inject CLAUDE.md (see handoff package)
- Resume: find last session ID from /tmp/.claude/projects/*/*.jsonl inside pod

### 4. Handoff context (internal/handoff/context.go + template.go)
Port build-handoff-context.sh to Go:
- Use k8s client to fetch COOConcept, COOPlan, COOSprints, COOTasks, COOFeatures, COOWorkers
- Render CLAUDE.md with Go template
- Inject via pod exec API (not kubectl cp) — write directly to /workspace/CLAUDE.md
- Prepend to existing CLAUDE.md with STOP/override header
- Append original CLAUDE.md as historical reference

### 5. Wire up cobra commands (cmd/workspace.go)
Replace the stubs with real implementations delegating to the runtime.

## Key constraints
- CLAUDE_CODE_OAUTH_TOKEN only — never set ANTHROPIC_API_KEY alongside it (causes auth conflict)
- Container must always exec with: bash -c 'cd /workspace && claude --dangerously-skip-permissions'
- Always use -c workspace when execing into k8s pods
- git config --global --add safe.directory '*' must run before claude starts
- The worker image already has Claude Code installed; do not reinstall it

## Reference implementation
The Makefile targets in code-orchestrator-operator are the ground truth for what each
command should do. See CLAUDE.md for the complete spec.


**Created:** 2026-03-09T22:24:41Z
**Status:** Draft

### HLD
# High-Level Design: coo-cli

**Created:** 2026-03-09T22:26:19Z
**Status:** Draft

*(See full HLD document above)*

### LLD
The LLD has been written to `docs/concepts/coo-cli-workspace/LLD.md`. Here's a summary of what it covers:

**Key design decisions captured:**

- **File structure** — 11 new files under `internal/` (runtime, k8s, workspace, handoff packages), with `cmd/workspace.go` as the only modified existing file
- **Component designs** — detailed flows for both k8s and local runtimes, including the clone-into-volume strategy, token resolution priority chains, and SPDY exec-based CLAUDE.md injection
- **Function signatures** — complete signatures for all exported and key unexported functions across every package
- **State management** — k8s mode is stateless (API server is source of truth); local mode uses atomic JSON file writes to `~/.coo/workspaces.json`
- **Error handling** — specific user-facing error messages for each failure mode, `SilenceUsage: true` on cobra root, exit code propagation from container exec
- **Test plan** — unit tests for token resolution/state logic, integration tests with fake k8s client and real Docker (CI DinD), E2E tests gated behind `//go:build e2e`
- **Migration** — purely additive to the existing skeleton; subcommands can be shipped one at a time