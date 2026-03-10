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

## 1. Overview

**Concept:** Implement the `coo workspace` command suite for coo-cli.

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


**Description:** Implement the `coo workspace` command suite for coo-cli.

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


---

## 2. Goals

1. **Replace all `make workspace *` targets** with `coo workspace` subcommands (create, list, exec, resume, delete) that are functionally equivalent and pass parity with the Makefile reference implementation.
2. **Support both runtime modes transparently** — k8s mode via COOWorkspace CRs and local Docker mode via direct container execution — with automatic detection requiring zero user configuration in the common case.
3. **Enable zero-k8s usage** — any developer with Docker and a Claude OAuth token can run `coo workspace create --repo owner/repo` and get a fully functional Claude Code environment without Kubernetes.
4. **Port handoff context injection to Go** — replace `build-handoff-context.sh` with a native Go implementation that fetches CRD state and injects a structured CLAUDE.md into the workspace pod.
5. **Provide consistent workspace lifecycle management** — create, list, resume, exec, and delete work identically in both runtime modes, backed by `~/.coo/workspaces.json` (local) or COOWorkspace CRs (k8s).

---

## 3. Non-Goals

1. **GUI or web interface** — this is a CLI-only deliverable; no dashboard or browser UI.
2. **Operator controller changes** — this implementation does not modify `code-orchestrator-operator`; it only consumes existing CRDs and APIs.
3. **Multi-cluster workspace management** — workspaces are scoped to a single kubeconfig context; cross-cluster federation is out of scope.
4. **Workspace sharing between users** — no multi-user collaboration features; workspaces are single-owner.
5. **CI/CD pipeline integration** — no non-interactive/headless mode; all `exec`/`resume` commands require an interactive TTY.

---

## 4. User Stories

1. As a **developer without Kubernetes access**, I want to run `coo workspace create --repo owner/repo` so that I get a Claude Code environment without needing a cluster.
2. As a **platform engineer**, I want to run `coo workspace create --repo owner/repo` so that a COOWorkspace CR is created and I'm exec'd into the pod once it reaches Ready.
3. As a **developer resuming work**, I want to run `coo workspace resume ws-1234567890` so that Claude restores my previous session automatically.
4. As a **developer doing handoff**, I want to run `coo workspace create --concept my-feature` so that the workspace is pre-loaded with full planning context (PRD, tasks, sprint state) in CLAUDE.md.
5. As a **developer managing workspaces**, I want to run `coo workspace list` so that I can see all active workspaces with their mode, phase, and age.
6. As a **developer finishing work**, I want to run `coo workspace delete ws-1234567890` so that resources (CR or container + volume) are cleaned up.
7. As a **developer switching machines**, I want token resolution to fall back to `~/.claude/credentials.json` so that I don't need to re-export env vars.
8. As a **platform engineer on a cluster**, I want `coo` to auto-detect k8s mode when the operator CRD is present so that I never need to specify `--local` or `--context`.

---

## 5. Acceptance Criteria

**workspace create (local mode)**
- Given Docker is available and `CLAUDE_CODE_OAUTH_TOKEN` is set, when I run `coo workspace create --repo owner/repo`, then a named volume is created at `~/.coo/volumes/<ws-name>/`, the repo is cloned into it, and a Docker container starts exec'd with Claude Code.
- Given a workspace is running, when I exit Claude, then the CLI prints `Resume this session: coo workspace resume <ws-name>`.

**workspace create (k8s mode)**
- Given kubectl access to a cluster with the COOWorkspace CRD, when I run `coo workspace create --repo owner/repo`, then a COOWorkspace CR is created in `coo-system`, the CLI waits up to 120s for `status.phase == Ready`, and I am exec'd into `-c workspace` with Claude Code.

**workspace create (handoff mode)**
- Given a valid `--concept` name, when the workspace reaches Ready, then the existing `/workspace/CLAUDE.md` is backed up, a rendered handoff CLAUDE.md is prepended with the override header, and the original is appended as historical reference.

**workspace list**
- Given workspaces exist, when I run `coo workspace list`, then a table is displayed with columns: name, mode, phase, pod/container, TTL/created.

**workspace resume**
- Given a workspace name, when I run `coo workspace resume <name>`, then the last session ID is discovered from `.jsonl` files and Claude is started with `--resume <id>`.

**workspace delete**
- Given a workspace name, when I run `coo workspace delete <name>`, then the COOWorkspace CR is deleted (k8s) or the container and volume entry are removed (local), with confirmation output.

**runtime detection**
- Given no flags and no reachable k8s API, when any workspace command runs, then local Docker mode is selected automatically.
- Given `--local` flag, when any workspace command runs, then k8s is never contacted.

---

## 6. Functional Requirements

- **FR-001** Runtime detector (`internal/runtime/detect.go`) MUST resolve mode in order: `--local` flag → `--context`/`--kubeconfig` flag → k8s API probe + CRD check → local fallback.
- **FR-002** Container runtime detection MUST try `docker`, `podman`, `nerdctl` in that order and use the first available binary.
- **FR-003** Local create MUST clone the repo into `~/.coo/volumes/<ws-name>/` before starting the container; subsequent exec/resume MUST skip re-cloning.
- **FR-004** Local create MUST write `/tmp/.claude.json` bootstrap inside the container (onboarding complete, workspace trusted, `skipDangerousModePermissionPrompt: true`).
- **FR-005** All container exec commands MUST use `bash -c 'git config --global --add safe.directory "*" && cd /workspace && claude --dangerously-skip-permissions'`.
- **FR-006** Token resolution MUST follow: `--token` flag → `CLAUDE_CODE_OAUTH_TOKEN` env → `~/.claude/credentials.json` → `~/.config/claude/credentials.json`. Error if none found.
- **FR-007** GitHub token resolution MUST follow: `--github-token` flag → `GITHUB_TOKEN` env → `GH_TOKEN` env → `gh auth token` output → anonymous (public repos only).
- **FR-008** `ANTHROPIC_API_KEY` MUST NOT be set in the container environment alongside `CLAUDE_CODE_OAUTH_TOKEN`.
- **FR-009** Workspace state MUST be persisted to `~/.coo/workspaces.json` with fields: name, containerID, repo, concept, createdAt, volumePath.
- **FR-010** K8s create MUST check for existing non-Terminated workspaces and prompt user to resume before creating a new one.
- **FR-011** K8s create MUST wait up to 120s for `status.phase == Ready` with a visible progress indicator, then exec into `-c workspace`.
- **FR-012** Handoff context injection MUST use the k8s exec API (not `kubectl cp`) to write CLAUDE.md into the pod.
- **FR-013** Handoff CLAUDE.md MUST include: override header, project metadata, original requirement, planning artifacts table, sprint/feature/task summary with status icons, worker roster, and phase-specific action guidance.
- **FR-014** Resume MUST discover the last session ID by globbing `*/*.jsonl` under the Claude projects directory and passing it as `--resume <id>` to Claude.
- **FR-015** `coo workspace list` MUST display k8s and local workspaces in a unified table.

---

## 7. Non-Functional Requirements

### Performance
- Runtime detection (k8s probe) MUST complete within 5 seconds; failure to connect MUST fall back immediately without blocking the user.
- COOWorkspace Ready wait MUST poll at 2-second intervals and display progress; timeout at 120s with a clear error message.
- Workspace list MUST render within 3 seconds for up to 50 workspaces.

### Security
- OAuth tokens MUST NOT be logged, printed, or written to disk outside of `~/.coo/workspaces.json` (which stores no tokens).
- `ANTHROPIC_API_KEY` MUST be explicitly excluded from container environment to prevent auth conflicts.
- GitHub token sourced from `gh auth token` subprocess MUST be captured via stdout only; stderr discarded.
- Volume directories (`~/.coo/volumes/`) MUST be created with mode `0700`.

### Scalability
- The local workspace tracker (`~/.coo/workspaces.json`) MUST handle up to 100 workspace entries without performance degradation.
- K8s mode relies on operator scaling; the CLI imposes no additional cluster-side constraints.

### Reliability
- All errors from k8s API, Docker SDK, and exec commands MUST be surfaced to stderr with actionable messages; no silent failures.
- On interrupted `workspace create` (Ctrl-C before Ready), any partially created CR or container MUST be reported; no automatic cleanup to avoid data loss.
- `~/.coo/workspaces.json` writes MUST be atomic (write to temp file, rename) to prevent corruption on interrupt.

---

## 8. Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework (already used in scaffold) |
| `k8s.io/client-go` | Kubernetes API client for COOWorkspace CR lifecycle |
| `k8s.io/apimachinery` | Unstructured types, GVK, REST mapping |
| `github.com/docker/docker/client` | Docker SDK for local mode container management |
| Docker / Podman / nerdctl binary | Container runtime for local mode (runtime dep, not Go dep) |
| `ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest` | Worker container image pulled at runtime |
| `kubectl` / kubeconfig | K8s cluster access credentials |
| `gh` CLI (optional) | GitHub token fallback via `gh auth token` |
| `CLAUDE_CODE_OAUTH_TOKEN` | Claude authentication for worker containers |

---

## 9. Out of Scope

- Modifying the `code-orchestrator-operator` controller or CRD schemas.
- Implementing any `coo` commands other than the `workspace` subcommand group.
- Non-interactive / headless workspace execution (no `--no-tty` mode).
- Windows support — Linux and macOS only for this iteration.
- Workspace sharing, RBAC, or multi-user access controls.
- Automatic workspace cleanup / TTL enforcement on the local side (TTL is operator-managed in k8s mode).
- A `coo workspace logs` command or streaming log tailing.

---

## 10. Success Metrics

- **Functional parity**: all five Makefile workspace targets (`workspace`, `workspace-list`, `workspace-exec`, `workspace-resume`, `workspace-delete`) are reproducible with equivalent `coo workspace` commands with identical end-state behaviour.
- **Zero-k8s path works end-to-end**: `coo workspace create --repo owner/repo` completes successfully on a machine with Docker and `CLAUDE_CODE_OAUTH_TOKEN` and no kubeconfig.
- **K8s path works end-to-end**: `coo workspace create --repo owner/repo` creates a CR, waits for Ready, and exec's the user into Claude Code on a cluster running the operator.
- **Handoff injection verified**: `coo workspace create --concept <name>` results in a `/workspace/CLAUDE.md` containing the override header, task list, and original CLAUDE.md as historical reference.
- **Resume works**: `coo workspace resume <name>` starts Claude with a valid `--resume <id>` argument (session ID sourced from `.jsonl` glob) in both local and k8s modes.
- **No token leakage**: code review confirms `ANTHROPIC_API_KEY` is never set and OAuth tokens never appear in logs or JSON state files.

---

## Appendix: Clarification Q&A

### Clarification Questions & Answers