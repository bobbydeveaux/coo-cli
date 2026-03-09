# coo-cli — Claude Code Instructions

## What This Is

`coo` is the official CLI for **itsacoo** (Code Orchestrator Operator) — the Kubernetes-native AI software development system. It provides a human-friendly interface to everything that currently lives in `make` targets inside `code-orchestrator-operator`.

This repo is **Go**, using [cobra](https://github.com/spf13/cobra) for commands and [client-go](https://github.com/kubernetes/client-go) for Kubernetes API access.

## Purpose

Replace all `make workspace *` Makefile targets (and eventually other operator Makefile targets) with a proper CLI:

```
# Instead of:
make workspace REPO=owner/repo
make workspace CONCEPT=test-page
make workspace-list
make workspace-exec NAME=ws-1234567890
make workspace-resume NAME=ws-1234567890
make workspace-delete NAME=ws-1234567890

# You get:
coo workspace create --repo owner/repo
coo workspace create --concept test-page
coo workspace list
coo workspace exec ws-1234567890
coo workspace resume ws-1234567890
coo workspace delete ws-1234567890
```

## Technical Spec — What the Makefile Does Today

The reference implementation is in `code-orchestrator-operator/Makefile` (targets: `workspace`, `workspace-list`, `workspace-exec`, `workspace-resume`, `workspace-delete`) and `code-orchestrator-operator/scripts/build-handoff-context.sh`.

### `coo workspace create`

**Flags:**
- `--repo owner/repo` — Freestyle mode: clone this repo into the workspace pod
- `--concept <name>` — Handoff mode: derive repo from COOProject, inject CLAUDE.md context
- `--model` — AI model (default: `claude-sonnet-4-5`)
- `--ttl` — Workspace TTL (default: `4h`)
- `--image` — Worker image override

**Flow:**
1. List existing COOWorkspaces in `coo-system` namespace with non-Terminated phase
2. If any exist → prompt user to resume one (or press Enter to create new)
   - If resuming: get pod name from `status.podName`, find last session ID from `/tmp/.claude/projects/*/*.jsonl` inside the pod, exec in with `--resume <id>`
3. Generate workspace name: `ws-<unix-timestamp>`
4. In **handoff mode**: auto-detect repo from `COOConcept → spec.affectedProjects[0] → COOProject → spec.github.owner/repo`
5. Create `COOWorkspace` CR in `coo-system`:
   ```yaml
   apiVersion: coo.itsacoo.com/v1alpha1
   kind: COOWorkspace
   spec:
     mode: freestyle | handoff
     repo: "owner/repo"
     conceptRef: "<concept-name>"
     model: "claude-sonnet-4-5"
     ttl: "4h"
     image: "ghcr.io/bobbydeveaux/code-orchestrator-operator/coo-worker-claude:0.6.1"
     imagePullPolicy: IfNotPresent
   ```
6. Wait for `status.phase == Ready` (timeout: 120s), showing a progress indicator
7. In **handoff mode**: run context injection (see below) after pod is ready
8. `kubectl exec -it <pod> -c workspace -- bash -c 'cd /workspace && claude --dangerously-skip-permissions; ...'`
9. On exit, print: `Resume this session: coo workspace resume <ws-name>`

### Handoff Context Injection (replaces `build-handoff-context.sh`)

This is the Go equivalent of `scripts/build-handoff-context.sh`. Use the k8s client directly (no shelling out to kubectl):

1. Fetch `COOConcept` from `coo-system` namespace via k8s API
2. Fetch `COOPlan` from `coo-<concept>` namespace
3. Fetch all `COOSprints`, `COOFeatures`, `COOTasks`, `COOWorkers` from concept namespace
4. Render CLAUDE.md using a Go template (see `internal/handoff/template.go`)
5. Prepend to existing `/workspace/CLAUDE.md` in the pod (use exec API, not kubectl cp):
   - Move existing CLAUDE.md to CLAUDE.md.original
   - Write rendered context as new CLAUDE.md
   - Append separator + "Historical Reference Only" warning + original content
   - Clean up temp file

**The rendered CLAUDE.md must include:**
- Override header: `⚠️ COO HANDOFF WORKSPACE — READ THIS SECTION FIRST`
- What itsacoo is (system explanation)
- Project: repo, concept name, phase, complexity tier
- Original requirement (`spec.rawConcept`)
- Planning artifacts table (PRD, HLD, LLD, epic, tasks — paths from `COOPlan.spec.artifacts`)
- Planning PR link (`status.planningPRURL`)
- Sprint summary (name, type, phase)
- Feature summary
- Task list with ✅/⏳ status
- Worker roster
- Phase-specific "What You Should Do" section:
  - `Planned` phase → "Review planning PR, check issues, approve to trigger Sprint 1"
  - `Executing` phase → "Pick up open tasks, review worker PRs"

### `coo workspace list`

List all COOWorkspaces in `coo-system`, showing: name, mode, phase, pod name, TTL expiry.

### `coo workspace exec <name>`

Get pod from `status.podName`, exec into `-c workspace` container in `/workspace` with Claude Code.

### `coo workspace resume <name>`

Same as exec but auto-discovers last session ID from `/tmp/.claude/projects/*/*.jsonl` inside the pod via exec API.

### `coo workspace delete <name>`

Delete the COOWorkspace CR. The controller handles pod + ConfigMap cleanup via finalizer.

## Kubernetes Configuration

- **Default namespace**: `coo-system`
- **Kubeconfig**: use standard `~/.kube/config` + `KUBECONFIG` env, with `--kubeconfig` flag override
- **Context**: use current context by default, `--context` flag override
- **Container name**: always `-c workspace` when execing into workspace pods

## CRD Types

The CRD types live in `code-orchestrator-operator/api/v1alpha1/`. For now, use `unstructured.Unstructured` or copy the relevant type structs rather than importing the operator module directly (avoid tight coupling). Define minimal Go structs for the fields we need.

Key types needed:
- `COOWorkspace` (spec: mode, repo, conceptRef, model, ttl, image; status: phase, podName, contextConfigMap)
- `COOConcept` (spec: rawConcept, affectedProjects; status: phase, complexityAssessment.tier)
- `COOPlan` (spec: artifacts; status: planningPRURL, planningPRNumber, epicCount, featureCount, issueCount, roam)
- `COOSprint` (status: phase, iteration)
- `COOFeature` (status: phase)
- `COOTask` (spec: worker, priority; status: phase, prNumber)
- `COOWorker` (spec: agentType; status: phase)

## Project Layout

```
coo-cli/
├── cmd/
│   └── root.go           # cobra root command, global flags (--kubeconfig, --context, --namespace)
├── internal/
│   ├── k8s/
│   │   └── client.go     # kubernetes client setup
│   ├── workspace/
│   │   ├── create.go     # workspace create logic
│   │   ├── list.go       # workspace list logic
│   │   ├── exec.go       # exec/resume into pod
│   │   └── delete.go     # workspace delete logic
│   └── handoff/
│       ├── context.go    # CRD data fetching
│       └── template.go   # CLAUDE.md rendering
├── main.go
├── go.mod
├── CLAUDE.md             # this file
└── README.md
```

## Dependencies

- `github.com/spf13/cobra` — CLI framework
- `k8s.io/client-go` — Kubernetes client
- `k8s.io/apimachinery` — K8s types
- `sigs.k8s.io/controller-runtime/pkg/client` — Higher-level client (optional, for unstructured)

## Style

- Go 1.22+
- `gofmt` + `go vet` clean
- Short, focused files — one concern per file
- Errors bubbled up, not swallowed
- User-facing output via `fmt.Printf` / `fmt.Fprintln(os.Stderr, ...)` for errors
- No external logging frameworks

## Reference

- Operator repo: https://github.com/bobbydeveaux/code-orchestrator-operator
- Makefile targets: `workspace`, `workspace-list`, `workspace-exec`, `workspace-resume`, `workspace-delete`
- Handoff script: `scripts/build-handoff-context.sh`
- COOWorkspace spec issue: https://github.com/bobbydeveaux/code-orchestrator-operator/issues/575
- CLI tracking issue: https://github.com/bobbydeveaux/code-orchestrator-operator/issues/576
