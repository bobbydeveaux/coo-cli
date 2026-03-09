# coo-cli

> CLI for [itsacoo](https://github.com/bobbydeveaux/code-orchestrator-operator) — the Kubernetes-native AI software development operator.

## What is this?

`coo` is the command-line interface for **itsacoo** (Code Orchestrator Operator). It replaces the `make workspace *` Makefile targets with a proper developer UX.

**You don't need itsacoo to use `coo workspace`.** If you just want a fully containerised Claude Code environment without installing anything into Kubernetes, `coo` will fall back to running the worker image locally via Docker.

## Commands

### Workspace

Interactive Claude Code workspaces running inside your k8s cluster — no Claude Code CLI on your local machine.

```bash
# Freestyle — clone a repo and start coding
coo workspace create --repo bobbydeveaux/my-project

# Handoff — pick up where AI workers left off, with full context injected
coo workspace create --concept my-concept

# List active workspaces
coo workspace list

# Exec into an existing workspace
coo workspace exec ws-1234567890

# Resume last Claude Code session
coo workspace resume ws-1234567890

# Delete a workspace
coo workspace delete ws-1234567890
```

#### Handoff Mode

When using `--concept`, `coo` reads the live state of all COO CRDs (concept, plan, sprints, tasks, features, workers) and injects a rich `CLAUDE.md` into the workspace pod before you drop in. The context tells Claude Code:

- What itsacoo is and how it works
- Which concept it's working on and its current phase
- Sprint progress, task status, open PRs
- Exactly what to do next (review planning PR, pick up open tasks, etc.)

## Installation

```bash
# From source
git clone https://github.com/bobbydeveaux/coo-cli
cd coo-cli
go install .
```

> Homebrew tap and pre-built binaries coming soon.

## Modes

### Local (no Kubernetes required)

Just Docker + a Claude Code OAuth token:

```bash
export CLAUDE_CODE_OAUTH_TOKEN=<your-token>
export GITHUB_TOKEN=<your-token>   # optional, for private repos

coo workspace create --repo bobbydeveaux/my-project
```

`coo` pulls the worker image, clones the repo inside the container, and drops you straight into Claude Code. Your workspace is persisted in `~/.coo/volumes/` so you can resume it later.

### K8s / itsacoo (full operator mode)

When `coo` detects a running itsacoo operator, it automatically uses `COOWorkspace` CRs instead of local Docker. The `--concept` handoff mode (injecting live CRD context) is only available in this mode.

```bash
coo workspace create --concept my-concept   # handoff with full context
coo workspace create --local --repo owner/repo  # force local even with k8s available
```

## Requirements

- Docker (for local mode) _or_ a cluster running the itsacoo operator (for k8s mode)
- `CLAUDE_CODE_OAUTH_TOKEN` — your Claude Code OAuth token
- `GITHUB_TOKEN` (optional) — for cloning private repos

## Status

🚧 **Early development** — currently implements the workspace commands. More commands (concepts, sprints, tasks) coming as the operator matures.

## Related

- [code-orchestrator-operator](https://github.com/bobbydeveaux/code-orchestrator-operator) — the K8s operator this CLI wraps
- COOWorkspace spec: [issue #575](https://github.com/bobbydeveaux/code-orchestrator-operator/issues/575)
- CLI tracking issue: [issue #576](https://github.com/bobbydeveaux/code-orchestrator-operator/issues/576)
