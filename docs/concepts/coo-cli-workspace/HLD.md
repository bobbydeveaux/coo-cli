# High-Level Design: coo-cli

**Created:** 2026-03-09T22:26:19Z
**Status:** Draft

## 1. Architecture Overview

`coo` is a single-binary Go CLI following a clean layered architecture:

```
┌─────────────────────────────────────────────────────┐
│                  cmd/ (cobra layer)                  │
│         root.go  │  workspace.go (subcommands)       │
└───────────────────────────┬─────────────────────────┘
                            │ delegates to
┌───────────────────────────▼─────────────────────────┐
│              internal/workspace/ (business logic)    │
│       create  │  list  │  exec  │  resume  │  delete │
└──────────┬────────────────────────┬──────────────────┘
           │                        │
┌──────────▼──────────┐  ┌──────────▼──────────────────┐
│ internal/runtime/   │  │ internal/handoff/            │
│  detect.go          │  │  context.go (CRD fetch)      │
│  k8s.go             │  │  template.go (CLAUDE.md)     │
│  local.go           │  └─────────────────────────────┘
└──────────┬──────────┘
           │ uses
┌──────────▼──────────────────────────────────────────┐
│ internal/k8s/client.go  │  Docker SDK / exec.LookPath │
└─────────────────────────────────────────────────────┘
```

**Pattern:** Single-process CLI monolith. No daemon, no server. All state is either in the Kubernetes API (k8s mode) or `~/.coo/workspaces.json` (local mode). Interactive TTY is always required for exec/resume commands.

**Runtime duality:** The `Runtime` interface abstracts k8s vs local, allowing `cmd/workspace.go` to be mode-agnostic. Mode is resolved once at command invocation by `detect.go` and passed down.

---

## 2. System Components

| Component | File(s) | Responsibility |
|---|---|---|
| **CLI layer** | `cmd/root.go`, `cmd/workspace.go` | Cobra command definitions, global flag binding, delegates to workspace package |
| **Workspace orchestrator** | `internal/workspace/*.go` | Per-subcommand business logic (create, list, exec, resume, delete); mode-agnostic |
| **Runtime detector** | `internal/runtime/detect.go` | Resolves k8s vs local mode; detects container binary (docker/podman/nerdctl) |
| **K8s runtime** | `internal/runtime/k8s.go` | COOWorkspace CR CRUD, pod readiness poll, pod exec via k8s API |
| **Local runtime** | `internal/runtime/local.go` | Docker container lifecycle, volume management, `~/.coo/workspaces.json` R/W |
| **K8s client** | `internal/k8s/client.go` | Kubeconfig loading, dynamic client construction, REST config |
| **Handoff context** | `internal/handoff/context.go` | Fetches COOConcept/COOPlan/sprints/tasks/features/workers via k8s dynamic client |
| **Handoff renderer** | `internal/handoff/template.go` | Go `text/template` rendering of CLAUDE.md; pod injection via exec API |

**Runtime interface (shared contract):**
```go
type Runtime interface {
    Create(ctx context.Context, opts CreateOptions) error
    List(ctx context.Context) ([]WorkspaceInfo, error)
    Exec(ctx context.Context, name string) error
    Resume(ctx context.Context, name string) error
    Delete(ctx context.Context, name string) error
}
```

---

## 3. Data Model

### Local state: `~/.coo/workspaces.json`
```json
{
  "workspaces": [
    {
      "name": "ws-1741564800",
      "containerID": "abc123def456",
      "repo": "owner/repo",
      "concept": "",
      "createdAt": "2026-03-09T22:00:00Z",
      "volumePath": "/home/user/.coo/volumes/ws-1741564800"
    }
  ]
}
```
Written atomically (temp file + rename). Mode `0600`. No tokens stored.

### K8s state: COOWorkspace CR (unstructured)
Key fields consumed from `status`:
- `phase`: `Pending | Ready | Terminated`
- `podName`: used for exec/resume
- `contextConfigMap`: for handoff (if applicable)

### In-memory: `WorkspaceInfo` (unified list view)
```go
type WorkspaceInfo struct {
    Name       string
    Mode       string   // "k8s" | "local"
    Phase      string
    PodOrContainer string
    Repo       string
    CreatedAt  time.Time
}
```

### Handoff context: fetched CRD graph
```
COOConcept → affectedProjects → COOProject (repo)
           → COOPlan (artifacts, PRURL)
           → COOSprints[] (phase, iteration)
           → COOFeatures[] (phase)
           → COOTasks[] (phase, prNumber, worker, priority)
           → COOWorkers[] (agentType, phase)
```
All fetched via `dynamic.Interface` using `unstructured.Unstructured`.

---

## 4. API Contracts

### Runtime interface — key method signatures

**Create:**
```go
type CreateOptions struct {
    Repo        string
    Concept     string
    Model       string
    TTL         string
    Image       string
    Token       string   // resolved Claude OAuth token
    GitHubToken string   // resolved GitHub token
    LocalFlag   bool
}
```

**List response (`WorkspaceInfo` slice)** — rendered as table to stdout:
```
NAME               MODE   PHASE    POD/CONTAINER        CREATED
ws-1741564800      local  running  abc123def456         2h ago
ws-1741564801      k8s    Ready    ws-1741564801-pod-x  45m ago
```

### K8s API calls (via dynamic client)
| Operation | GVR | Verb |
|---|---|---|
| Create workspace | `coo.itsacoo.com/v1alpha1/cooworkspaces` | create |
| Get/Watch workspace | same | get, watch |
| Delete workspace | same | delete |
| List workspaces | same | list |
| Fetch COOConcept | `coo.itsacoo.com/v1alpha1/cooconcepts` | get |
| Fetch COOPlan | `coo.itsacoo.com/v1alpha1/cooplans` | get |
| Fetch COOSprints/Features/Tasks/Workers | respective GVRs | list |
| Pod exec | `core/v1/pods/exec` subresource | create (SPDY) |

### Docker SDK calls (local mode)
- `ImagePull` — pull worker image
- `ContainerCreate` + `ContainerStart` — create workspace
- `ContainerExecCreate` + `ContainerExecAttach` — not used; exec via `os/exec docker exec -it` for TTY
- `ContainerStop` + `ContainerRemove` — delete workspace

> Note: Docker SDK exec attach does not support raw TTY well; `docker exec -it` subprocess is used for interactive sessions.

---

## 5. Technology Stack

### Backend
- **Go 1.22+** — single compiled binary, cross-platform (Linux/macOS)
- **github.com/spf13/cobra** — CLI framework, subcommand routing, flag binding
- **k8s.io/client-go** — Kubernetes REST client, dynamic client, pod exec (SPDY)
- **k8s.io/apimachinery** — `unstructured.Unstructured`, GVR/GVK types
- **github.com/docker/docker/client** — Docker SDK for container management (local mode)
- **text/template** — Go stdlib; handoff CLAUDE.md rendering

### Frontend
N/A — terminal stdout/stderr only. No TUI framework; plain `fmt` output with tabwriter for list tables.

### Infrastructure
- **Container runtime**: Docker, Podman, or nerdctl (detected at runtime via `exec.LookPath`)
- **Worker image**: `ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:latest`
- **Kubernetes**: operator cluster running `code-orchestrator-operator` (k8s mode only)

### Data Storage
- **`~/.coo/workspaces.json`** — local workspace registry (JSON, atomic writes)
- **`~/.coo/volumes/<ws-name>/`** — bind-mount volume for workspace filesystem persistence (mode `0700`)
- **Kubernetes etcd** (via API server) — COOWorkspace CR state in k8s mode
- **`~/.kube/config`** / `KUBECONFIG` env — kubeconfig, standard client-go loading

---

## 6. Integration Points

| System | Integration Method | Direction |
|---|---|---|
| Kubernetes API server | client-go dynamic client over HTTPS | outbound |
| Docker / Podman daemon | Docker SDK (Unix socket) + subprocess exec for TTY | outbound |
| GitHub (repo clone) | `git clone https://$GITHUB_TOKEN@github.com/...` inside container | container-outbound |
| `gh` CLI | `exec.Command("gh", "auth", "token")` stdout capture | outbound (optional) |
| Claude Code (worker) | Container entrypoint: `claude --dangerously-skip-permissions` | within container |
| `~/.claude/credentials.json` | Direct file read for OAuth token fallback | local filesystem |
| COOWorkspace CRD | create/get/list/delete via dynamic client | k8s API |
| COOConcept/COOPlan/etc. CRDs | get/list via dynamic client (handoff only) | k8s API |
| Pod exec (handoff inject) | SPDY exec subresource via client-go `remotecommand` | k8s API |

---

## 7. Security Architecture

**Token handling:**
- Claude OAuth token resolved at startup; passed to container via `-e CLAUDE_CODE_OAUTH_TOKEN=<token>` only. Never written to disk in `~/.coo/`.
- `ANTHROPIC_API_KEY` explicitly absent from all `docker run` env construction. Guard enforced in `local.go` env-building function.
- GitHub token passed via `-e GITHUB_TOKEN=<token>`; sourced from env or `gh auth token` subprocess (stdout only).

**Filesystem permissions:**
- `~/.coo/` created with mode `0700`
- `~/.coo/volumes/` and subdirectories: mode `0700`
- `~/.coo/workspaces.json`: mode `0600`; written via `os.CreateTemp` + `os.Rename` for atomicity

**Kubernetes RBAC:**
- CLI uses the operator's existing kubeconfig permissions; no additional RBAC setup required
- Namespace scoped to `coo-system` by default; overridable via `--namespace`

**Container security:**
- Worker image runs with existing image entrypoint; no `--privileged` required
- No host network; no extra capabilities added by CLI

**Credential precedence (safe defaults):**
- Token flags take highest precedence → env vars → credential files → subprocess → error/anonymous
- No prompting for tokens interactively (fail fast with actionable error message)

---

## 8. Deployment Architecture

`coo` is a **statically compiled Go binary** distributed as:
- GitHub Release artifact (`coo-linux-amd64`, `coo-darwin-arm64`, etc.)
- Optionally via Homebrew tap or `go install`

No server-side deployment. All runtime dependencies are:
- User's existing Docker daemon (local mode)
- User's existing kubeconfig (k8s mode)
- Worker image pulled on-demand at `workspace create`

**Build:**
```
GOOS=linux GOARCH=amd64 go build -o dist/coo-linux-amd64 ./main.go
```

**Distribution path:** GitHub Actions CI builds and attaches binaries to releases on tag push. No container image needed for the CLI itself.

---

## 9. Scalability Strategy

`coo` is a CLI tool; scalability concerns are minimal and bounded:

**Local mode:**
- `~/.coo/workspaces.json` is a flat JSON array; linear scan acceptable up to 100 entries (PRD NFR). File is read entirely into memory on each command; no streaming needed.
- Each workspace maps to one Docker container + one host volume directory; limited only by host disk/memory.

**K8s mode:**
- Workspace scaling is entirely handled by the operator; the CLI adds zero cluster-side load beyond standard API calls.
- List command performs a single `list` API call scoped to `coo-system`; no pagination needed at current scale.

**Runtime detection:**
- k8s probe uses a 5-second `context.WithTimeout`; non-blocking on failure. Single lightweight API version check (`/api` or CRD existence check).

**No concurrency required:** All workspace operations are inherently sequential (interactive TTY). No goroutine pools or worker queues needed.

---

## 10. Monitoring & Observability

`coo` is a CLI tool; traditional server-side monitoring does not apply. Observability strategy:

**User-facing feedback:**
- Progress indicator during `workspace create` k8s wait loop (spinner or dot-progress every 2s poll)
- Clear phase transitions printed: `Creating workspace... Waiting for Ready... Exec'ing into pod...`
- On exit from Claude session: `Resume this session: coo workspace resume <name>`

**Error surfacing:**
- All errors written to `os.Stderr` with context (e.g., `error waiting for workspace Ready: timeout after 120s`)
- k8s API errors include HTTP status codes; Docker SDK errors include daemon message
- Non-zero exit codes propagated from container exec for scripting compatibility

**Debug mode (future):** `--verbose` / `COO_DEBUG=1` flag could enable client-go request logging and Docker SDK debug output; not required for initial implementation but hooks should be left in place.

**No external telemetry** — no analytics, no crash reporting, no external calls beyond k8s API, Docker daemon, and GitHub.

---

## 11. Architectural Decisions (ADRs)

**ADR-001: Use `unstructured.Unstructured` instead of importing operator CRD types**
- *Decision:* Define minimal Go structs locally; use dynamic client with unstructured for all CRD access.
- *Rationale:* Avoids tight coupling to operator module versioning. CRD schema is stable enough to read via field accessors. Eliminates circular dependency risk.
- *Trade-off:* Less type safety; mitigated by defensive field access helpers.

**ADR-002: Use `os/exec docker exec -it` for interactive sessions instead of Docker SDK attach**
- *Decision:* Spawn `docker exec -it <id> bash -c '...'` as a child process for interactive workspace entry.
- *Rationale:* Docker SDK's `ContainerExecAttach` does not correctly handle raw TTY resize signals and terminal state in all environments. `os/exec` with inherited stdio is simpler and battle-tested.
- *Trade-off:* Requires Docker binary on PATH; Docker SDK still used for non-interactive operations (create, list, stop, remove).

**ADR-003: Atomic JSON writes for `~/.coo/workspaces.json`**
- *Decision:* Write to `<file>.tmp`, then `os.Rename` atomically.
- *Rationale:* Prevents state file corruption if the process is interrupted mid-write. Single-file JSON is sufficient at the targeted scale (≤100 entries).
- *Trade-off:* Not suitable if workspaces are managed from multiple concurrent `coo` processes; acceptable as single-user CLI.

**ADR-004: Handoff CLAUDE.md injection via k8s exec API, not `kubectl cp`**
- *Decision:* Use `client-go/tools/remotecommand` SPDY exec to stream file content into the pod via `bash -c 'cat > /workspace/CLAUDE.md'`.
- *Rationale:* `kubectl cp` requires the `kubectl` binary and creates a tar stream overhead. Exec API is available natively via client-go and avoids the binary dependency.
- *Trade-off:* Slightly more complex streaming setup; well-supported by client-go.

**ADR-005: Runtime mode resolved once per command invocation**
- *Decision:* `detect.Resolve(flags)` is called at the top of each `workspace` subcommand; the resolved `Runtime` is passed to the workspace package.
- *Rationale:* Keeps mode-selection logic in one place; workspace business logic remains mode-agnostic. Consistent with single-responsibility principle.
- *Trade-off:* k8s probe adds up to 5s latency on each command for ambiguous environments; acceptable given the probe has a hard timeout.

---

## Appendix: PRD Reference

*(See PRD document: "Implement the `coo workspace` command suite for coo-cli" — 2026-03-09T22:24:41Z)*