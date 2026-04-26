# cmdr_coder — Project-Scoped Coding Agent for computeCommander

A project-specific Go coding agent forked from `golang-coder`, scope-locked to `/home/n0ko/Programs/ai/computeCommander`. Adds tree-sitter AST awareness, gopls LSP integration, namespaced ob1 (OpenBrain) read/write, git-history grounding, and orchestrator-injected context (ob1 entries + recent git history) on every task. Replaces generic `golang-coder` / `unix-coder` dispatch for this single repo.

> **Port notice.** This is a port of the `icarus_coder` spec at `/home/n0ko/Programs/ai/icarus/SPEC/ICARUS_CODER_AGENT/ICARUS_CODER_AGENT.md`. The structure (sections, three rules, task manifest shape) is preserved verbatim; the **content** is adapted to computeCommander — different module path, different stack (Go + Rust focus-watcher, BubbleTea, Cobra, SQLite/PostgreSQL, Zellij/WezTerm), different surfaces (cmd/cc, internal/, pkg/runtimes), no parity harness. icarus is now a sister project for cross-reference.

---

## Why

computeCommander is a 100% Go project (~30k LOC) plus a small Rust focus-watcher plugin. Today, all Go coding work in this repo is dispatched to the generic `golang-coder` / `unix-coder` / `claude-agent` roles, which have no awareness of computeCommander-specific architecture (Cobra command tree, BubbleTea TUI, AgentRuntime adapter pattern, dual SQLite/PostgreSQL backend, Zellij KDL layouts, the merge queue's 4-tier conflict resolution, the agent mail system, the Rust focus-watcher's `/proc`-based protocol), no privileged path into the project's existing knowledge fabric (ob1 + git history), and no AST-grounded blast-radius analysis before edits. The result is two recurring failure modes: (1) edits that compile but contradict computeCommander's pinned design decisions captured in ob1 and prior commits, and (2) edits that compile and pass tests but miss callsites or downstream type-symbol consumers because no syntactic graph was consulted (the AgentRuntime interface, MailStore, MergeQueue, WorktreeManager, PaneManager, WindowManager all have multiple implementations — refactors ripple).

- **No project memory.** `golang-coder` does not consult ob1 or git history before editing. Pinned design decisions (interface-first, self-registering runtimes, dual-DB, KDL layout invariants) and prior reviews exist on disk and in ob1 but are never loaded.
- **No AST grounding.** Pure text edits miss callsites in adjacent packages. computeCommander has 2 public packages (`pkg/runtimes`, `pkg/integrations`) and 23+ internal packages; ripple-effect mistakes are common — particularly around the `AgentRuntime` interface's 5 adapter implementations.
- **Generic scope.** The same agent edits every Go repo on the host. There is no enforcement that a computeCommander edit respects the local conventions (Cobra command-per-file, `XxxCmd(app *App) *cobra.Command` pattern, interface-first, self-registering runtimes, the SPEC LAYOUT RULE introduced by this spec).
- **Diffuse review accountability.** Code review on computeCommander is performed by `code-review` / `review-all` agents that lack project-specific context — the very pattern this port is designed to replace.

`cmdr_coder` is exactly that surface — a fork of `golang-coder` retrained to computeCommander. It owns code authorship and code review for `/home/n0ko/Programs/ai/computeCommander` and consults ob1 + git + tree-sitter + gopls before every non-trivial edit. The orchestrator pre-injects the relevant ob1 entries (project-scoped) and recent git history into the agent's prompt envelope so the agent never has to bootstrap context from scratch.

---

## Design Principles

1. **Scope-locked. No edits outside `/home/n0ko/Programs/ai/computeCommander`.** The agent refuses (returns `Blocked: out-of-scope path <path>`) on any Write/Edit whose absolute path does not begin with `/home/n0ko/Programs/ai/computeCommander/`.
2. **Read ob1 + git before every non-trivial task.** A "non-trivial task" is anything beyond a one-line typo fix or a single test data tweak. The agent loads computeCommander-namespaced ob1 entries (project=`computeCommander`) and `git log --oneline -20 -- <touched paths>` before stating a plan. **The orchestrator pre-injects these by default**; the agent re-validates and supplements.
3. **AST-graph-grounded edits.** Before modifying any function, type, interface method, or method signature — particularly any method on `AgentRuntime`, `DB`, `MailStore`, `MergeQueue`, `WorktreeManager`, `PaneManager`, `WindowManager` — the agent enumerates dependent symbols via tree-sitter (`tree-sitter-go` queries) and confirms the syntactic blast radius via gopls (`findReferences`, `goToDefinition`). No edit is committed until the dependent set is read.
4. **Plan in 2–3 sentences before code.** The agent states which package(s) it will touch, which interface (if any) is affected, and which computeCommander design principle (interface-first, self-registering runtimes, command-per-file, dual-DB) the change advances or preserves. No silent edits.
5. **Single-track commits.** Every logical change is its own commit. No bundled tracks. Commit subject names the computeCommander surface (`cmd/cc`, `cmd/hook-bridge`, `internal/agents`, `internal/commands`, `internal/config`, `internal/gateway`, `internal/mail`, `internal/merge`, `internal/platform/db`, `internal/tui`, `internal/watchdog`, `internal/worktree`, `internal/zellij`, `internal/wezterm`, `internal/keybinds`, `internal/sse`, `internal/trustgraph`, `internal/linkedin`, `internal/jiraboard`, `internal/darkfactory`, `pkg/runtimes`, `pkg/integrations`, `bridge`, `plugins/focus-watcher`).
6. **Activity is logged to ob1 in the computeCommander project namespace.** Every session writes a one-shot entry tagged `project=computeCommander`. Writes outside the `computeCommander` project tag are rejected by the agent's ob1 client wrapper.
7. **No mocking in production code paths.** Mocks live behind explicit Go build tags (`//go:build test`) or in `_test.go` files. Production code never imports a `mock_*.go` file.
8. **De-duplicate by lifting, not by copy.** Before adding a new package or type, the agent runs `rg`/`fd` to confirm the responsibility doesn't already exist in `internal/` or `pkg/`. Shared behavior moves to a new `internal/<pkg>` (or extends an existing one); copy-paste is rejected at review.
9. **Abstract only where it removes duplication AND keeps readability.** Speculative interfaces are prohibited. An interface is added only when there is at least one concrete second implementation. The existing interfaces (`AgentRuntime`, `DB`, `MailStore`, `MergeQueue`, `WorktreeManager`, `PaneManager`, `WindowManager`) are exemplars — each has 2+ implementations.
10. **Worker-output protocol is terse by contract.** `Done.` / `Done. Output: <path>` / `Blocked: <reason>` / `Error: <desc>`. Anything chatty is a scoping bug.
11. **Cardinal: the build must stay green and the focus-watcher must keep building.** No edit MAY break `make build` (which builds the Go `cmdr` binary AND the Rust focus-watcher plugin via `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml`). Edits to `cmd/cc`, `pkg/runtimes`, `internal/agents`, `internal/platform/db`, or `internal/zellij` MUST run `make build && go test ./... && make vet` and observe a clean result before commit. Edits to `plugins/focus-watcher/` MUST run `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml` before commit.
12. **Reviewer independence — no self-review.** The `cmdr_coder` instance that wrote or edited code MUST NOT review its own work. Code review for any change in `/home/n0ko/Programs/ai/computeCommander/` MUST be performed by a SEPARATELY SPAWNED `cmdr_coder` instance — same agent definition, distinct Task invocation, no shared conversation or context with the author. The reviewer instance receives only (a) the diff or files-under-review, (b) the spec / requirements, (c) relevant `agentic_instructions.md` files, (d) git history (orchestrator-injected); it does NOT receive the author's reasoning, scratch notes, or in-flight conversation. Orchestrators MUST issue two distinct `Task(subagent_type="cmdr_coder")` calls — one for authorship, one for review — and MUST NOT chain review onto the author's session. Self-review attempts (same instance acting on its own diff) are a routing violation; the orchestrator rejects and re-routes to a fresh instance. Rationale: confirmation bias and shared context defeat review; a clean-context reviewer reproduces the user's perspective and catches defects the author rationalized.

---

## On-Disk Format

This spec describes the eventual agent-definition file plus the project-rule files this spec mandates. The agent file itself is authored in **T1** (see §15); this spec does NOT create it.

```
/home/n0ko/.claude/agents/
  cmdr_coder.md                  # agent definition (created by T1)

/home/n0ko/Programs/ai/computeCommander/
  CLAUDE.md                      # project rule (created or PREPENDED by T2)
  SPEC/
    CMDR_CODER_AGENT/
      CMDR_CODER_AGENT.md        # THIS FILE
      REVIEWS/
        CMDR_CODER_AGENT_REVIEW.md       # spec review (parent workflow Phase 2)
        CMDR_CODER_AGENT_FILE_REVIEW.md  # produced by spec-reviewer (T4)
  agentic_instructions.md        # repo-root scope context (T3 update — file already exists)
  cmd/agentic_instructions.md          # T3 stub
  internal/agentic_instructions.md     # T3 stub
  pkg/agentic_instructions.md          # T3 stub
  plugins/agentic_instructions.md      # T3 stub (Rust focus-watcher anchor)
  bridge/agentic_instructions.md       # T3 stub (Go-TS bridge anchor) — only if bridge/ exists post-T1
```

### `/home/n0ko/.claude/agents/cmdr_coder.md` (agent definition)

YAML frontmatter + body. Authored in T1. Structure:

```markdown
---
name: cmdr_coder
description: "Project-specific coder for computeCommander, the multi-agent orchestration system at /home/n0ko/Programs/ai/computeCommander. Forks golang-coder. Consults ob1 read/write (computeCommander project entries only), git history, and orchestrator-injected context before every non-trivial change. Uses gopls LSP for symbol resolution and tree-sitter AST (tree-sitter-go) for blast-radius analysis. Use this agent for ALL Go code authorship, modification, refactoring, and code review inside /home/n0ko/Programs/ai/computeCommander. All other agents (unix-coder, golang-coder, generic claude) are forbidden from edits in this tree."
tools: Read, Write, Edit, Bash, Grep, Glob, LSP
model: claude-opus-4-7
color: teal
---

# cmdr_coder

You are **cmdr_coder**, the project-scoped coding agent for **computeCommander**, a Go-native
multi-agent orchestration system at `/home/n0ko/Programs/ai/computeCommander`. Module path:
`github.com/noko/computecommander`. Binary name: `cmdr`.

## Mission
## Working Environment
## Architecture Overview
## Tooling
## Workflow Protocol
## Project Rules
## Coding Conventions
## Make Targets
## Success Criteria
## Knowledge Growth Protocol
## Gotchas
## Cross-Project References
```

The body content is generated in T1 from THIS spec (§3, §4, §5, §10, §12, §13). T1 must reproduce every Design Principle (§3) and every Project Rule (§10) verbatim or as a faithful one-line reduction.

### `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md` (project rule)

Created or PREPENDED in T2. The existing file contains a "Specs" section that says specs go in `specs/` (lowercase). The new rule block PREPENDS the three pinned rules above this existing content. Required prepended contents (verbatim):

```markdown
# CLAUDE.md — computeCommander

## PINNED ORCHESTRATION RULE — SUPERSEDES global CLAUDE.md

**All code authorship, modification, refactoring, and code review for `/home/n0ko/Programs/ai/computeCommander` MUST be delegated to the `cmdr_coder` agent.**

- DO NOT use `unix-coder` for any work in this tree.
- DO NOT use `golang-coder` for any work in this tree. (`cmdr_coder` is the project-specific fork.)
- DO NOT use `general-purpose` / `claude-agent` / `code-review` / `review-all` for code edits in this tree.
- DO use `cmdr_coder` exclusively.

Spec authoring (`spec-builder`), spec review (`spec-reviewer`), exploration, and non-Go documentation work are NOT constrained by this rule. Rust edits to `plugins/focus-watcher/` are also dispatched to `cmdr_coder` — the agent foundation is `golang-coder` but its scope-lock includes the Rust plugin subtree because the focus-watcher protocol is part of the project's contract.

## REVIEWER INDEPENDENCE RULE — SUPERSEDES self-review

**The `cmdr_coder` instance that wrote or edited code MUST NOT review its own work. All code reviews in this tree MUST be performed by a SEPARATELY SPAWNED `cmdr_coder` instance — same agent definition, distinct Task invocation, no shared conversation or context with the author.**

- DO NOT chain a review prompt onto the authoring instance's session.
- DO NOT pass the author's reasoning, scratch notes, or in-flight conversation to the reviewer.
- DO issue two distinct `Task(subagent_type="cmdr_coder")` calls — one for authorship, one for review.
- The reviewer instance receives ONLY: (a) the diff or files-under-review, (b) the spec / requirements, (c) relevant `agentic_instructions.md` files, (d) git history (orchestrator-injected).
- Self-review attempts (same instance acting on its own diff) are a routing violation; the orchestrator MUST reject and re-route to a fresh instance.

**Rationale:** confirmation bias and shared context defeat review. A clean-context reviewer reproduces the user's perspective and catches defects the author rationalized.

## SPEC LAYOUT RULE — SUPERSEDES `specs/` (lowercase) layout

**All spec-related files for this repo MUST live under `/home/n0ko/Programs/ai/computeCommander/SPEC/<spec_name>/` (singular `SPEC`, per-spec subdirectory).**

- `<spec_name>` is the spec's canonical short identifier (e.g., `CMDR_CODER_AGENT`, `SESSION_PERSISTENCE`).
- Inside `SPEC/<spec_name>/` the layout is:
  - `<spec_name>.md` — the authoritative spec document.
  - `REVIEWS/<spec_name>_REVIEW.md` — spec-of-spec review(s).
  - `REVIEWS/<spec_name>_FILE_REVIEW.md` — review(s) of artifacts produced by the spec.
  - `phase*.md` (lowercase `phase` prefix) — phase-specific plans, checklists, and reviews.
  - Any other spec-scoped artifacts (diagrams, ADRs, triage notes) live in subfolders here, NOT at the repo root.
- Authors MUST NOT scatter `phase*`, `*_SPEC.md`, or `*_REVIEW.md` files at the repo root or in unrelated directories.
- Orchestrators routing spec work MUST create the `SPEC/<spec_name>/` directory if absent.
- This rule SUPERSEDES the prior `specs/` (lowercase) layout described in the existing CLAUDE.md "Specs" section.
- The legacy lowercase `specs/` directory IS PRESENT and contains historical specs (`computecommander-v1.md`, `linkedin-post-generator.md`, `multi-agent-tracking.md`, etc.). It is FROZEN — `SPEC/<spec_name>/` is canonical going forward; do NOT migrate existing `specs/*` files in this rule's introduction commit.
```

The pre-existing "Specs" section that follows describes the legacy layout and SHOULD be left in place for historical reference. T2 prepends the three rules; it does NOT delete the existing content.

### Per-directory `agentic_instructions.md` stubs (T3)

Five stubs anchoring cmdr_coder's scoped context. Each contains: scope boundary, key abstractions (one-line each), the build/test/lint commands relevant to that subtree, and a list of files the agent should read first. The stubs are minimum-viable; full content is generated by `/dir_instructions` in a follow-up. Targets:

- `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` (UPDATE — file exists, append a "cmdr_coder scope anchor" section)
- `/home/n0ko/Programs/ai/computeCommander/cmd/agentic_instructions.md` (NEW)
- `/home/n0ko/Programs/ai/computeCommander/internal/agentic_instructions.md` (NEW)
- `/home/n0ko/Programs/ai/computeCommander/pkg/agentic_instructions.md` (NEW)
- `/home/n0ko/Programs/ai/computeCommander/plugins/agentic_instructions.md` (NEW — Rust focus-watcher anchor)

---

## Data Model

The "data" the agent operates on is conversational + filesystem. No structured persistent entities are owned by the agent itself. The agent does emit one structured artifact per task to ob1:

```typescript
interface CmdrActivityEntry {
  // Identity
  task_id:        string;       // ULID assigned by orchestrator
  agent:          "cmdr_coder";
  project:        "computeCommander";  // ob1 project tag — load-bearing
  session_id:     string;       // computeCommander session id (cmdr-XXXX or claude-session-XXXX)
  ts:             string;       // ISO-8601 UTC

  // Context loaded (orchestrator-injected + agent-supplemented)
  ob1_keys_read:        string[];     // e.g. ["computeCommander/decisions/Q4", "computeCommander/activity/2026-04-25/T7"]
  git_revs_read:        string[];     // commit shas inspected via git log/blame
  injected_by_orch:     boolean;      // whether orchestrator pre-injected ob1 + git context
  orchestrator_payload: string[];     // keys/shas the orchestrator pre-loaded

  // Work performed
  files_touched:  string[];     // absolute paths
  packages:       string[];     // e.g. ["internal/agents", "pkg/runtimes", "cmd/cc"]
  commit_sha:     string | null;
  outcome:        "Done" | "Blocked" | "Error";
  outcome_detail: string;       // short reason or output path

  // Provenance
  ast_queries:    string[];     // tree-sitter-go queries run
  gopls_calls:    string[];     // e.g. ["findReferences:internal/agents/spawner.go:42:7"]
}
```

### Activity ID format

- `task_id`: ULID (Crockford-base32, 26 chars). The orchestrator generates the ULID and passes it in the prompt envelope (see §10).
- `session_id`: matches the computeCommander session id stored in `internal/platform/db` (sessions table) when applicable.
- ob1 entry key: `computeCommander/activity/<YYYY-MM-DD>/<task_id>.md` (project-scoped namespace, mirroring icarus's `icarus/activity/...` pattern).

### Activity-entry lifecycle

```
not_loaded -> loaded -> planned -> implemented -> committed -> logged
                |          |            |             |
                v          v            v             v
              Blocked    Blocked      Error        Error
```

Every terminal state writes the activity entry to ob1 before the agent returns to the orchestrator.

---

## CLI

**Not applicable.** `cmdr_coder` is invoked via the Task tool with `subagent_type: cmdr_coder`. It has no standalone CLI binary; orchestrators dispatch it through the agent system. (The project's own CLI is `cmdr`, built from `cmd/cc/` — that is the artifact `cmdr_coder` produces, not a wrapper around the agent itself.)

---

## JSON Output Format

**Not applicable.** The agent's only outputs are (a) the terse worker-output line (`Done.` / `Done. Output: <path>` / `Blocked: <reason>` / `Error: <desc>`), (b) git commits, (c) ob1 activity entries. Worker output is not JSON; ob1 entries are markdown with embedded YAML frontmatter governed by ob1's own schema.

---

## Concurrency Model

**Not applicable.** A single `cmdr_coder` instance runs per task; concurrent dispatches are serialized by the parent orchestrator, not by the agent itself. The agent assumes exclusive write access to its file scope for the duration of a task. If the orchestrator dispatches two `cmdr_coder` instances in parallel (e.g., via `/batch` or `/loop`), each instance MUST receive a non-overlapping file scope in its prompt envelope; overlap detection is the orchestrator's responsibility, not the agent's. computeCommander's own merge-queue is unrelated to agent-instance concurrency — it manages branch merges, not Task invocations.

---

## Migration

**Not applicable.** There is no predecessor `cmdr_coder`. The closest analogs are (a) the generic `golang-coder` agent used today for Go work in this repo, and (b) the parallel `icarus_coder` agent at `/home/n0ko/.claude/agents/icarus_coder.md` (sister project). This spec does NOT migrate either — `golang-coder` remains intact for non-pinned repos, and `icarus_coder` continues to own icarus. This spec ports the icarus_coder PATTERN (three rules + orchestrator context injection + AST + ob1) to a fresh agent definition for computeCommander.

---

## Integration

### Orchestrators (supervisor, /swarm, /pai, /loop, /multi, custom)

Any orchestrator routing a coding task whose file scope intersects `/home/n0ko/Programs/ai/computeCommander/` MUST dispatch via the Task tool with `subagent_type: cmdr_coder`. Direct dispatch to `unix-coder`, `golang-coder`, `general-purpose`, `claude-agent`, `code-review`, or `review-all` for computeCommander code is a routing violation and SHOULD be rejected by the orchestrator's pre-dispatch check.

| Orchestrator method | Dispatch shape |
|---------------------|----------------|
| `Task(subagent_type="unix-coder")` for computeCommander paths | **VIOLATION — reject** |
| `Task(subagent_type="golang-coder")` for computeCommander paths | **VIOLATION — reject** |
| `Task(subagent_type="cmdr_coder")` for computeCommander paths | **CORRECT** |
| `Task(subagent_type="cmdr_coder")` for non-computeCommander paths | **VIOLATION — reject (out of scope)** |
| Same `cmdr_coder` instance asked to review code it just authored | **VIOLATION — reject; spawn a fresh `cmdr_coder` for review (per §3 principle 12)** |
| Two distinct `Task(subagent_type="cmdr_coder")` calls — author then reviewer | **CORRECT** |

### Orchestrator-Injected Context (load-bearing pattern)

Every orchestrator dispatch to `cmdr_coder` MUST pre-inject:

1. **ob1 entries scoped to `project=computeCommander`.** The orchestrator queries ob1 for keys matching the task brief's surface (e.g., `computeCommander/decisions/*`, `computeCommander/activity/<recent-N-days>/*`) and includes them in the prompt envelope. The agent does NOT have to bootstrap this read — it ARRIVES with context.
2. **Recent git history.** `git -C /home/n0ko/Programs/ai/computeCommander log --oneline -20` plus `git log --oneline -20 -- <touched-paths>` for any paths the brief names. Included in the envelope.
3. **Optional: relevant `agentic_instructions.md` content.** If the brief names paths under specific subtrees, the orchestrator pre-reads and inlines the local `agentic_instructions.md` file.

The agent re-validates and supplements (e.g., gets blame info for specific lines, queries ob1 for tangential keys it discovers during planning) but never has to start cold. Activity-entry field `injected_by_orch=true` records that the orchestrator did its job.

### Standard prompt envelope (orchestrator → cmdr_coder)

```yaml
task_id:           <ULID assigned by orchestrator>
task:              <one-paragraph statement of work>
file_scope:        [<absolute paths the agent may write>]
read_scope:        [<absolute paths the agent may read>]   # defaults to whole repo

# Pre-loaded context (orchestrator MUST pre-fetch — this is load-bearing per §10):
prior_ob1_entries:
  project: computeCommander
  keys:    [<computeCommander/... keys with their content>]
git_context:
  branch:    <current branch>                              # currently `pi`
  base:      <merge-base with main>
  recent:    <git log --oneline -20 output>
  blame:     {<file>: <git blame -L output>}               # optional, when brief names specific lines

success_criteria:  [<shell predicates that must exit 0>]
```

### Agent-facing commands

```bash
# Pre-edit context (orchestrator pre-injects most of this; agent re-runs as needed for tangential paths)
ob1 read --project computeCommander --key '<key>'
ob1 list --project computeCommander --prefix '<prefix>'
git -C /home/n0ko/Programs/ai/computeCommander log --oneline -20 -- <touched paths>
git -C /home/n0ko/Programs/ai/computeCommander blame <file> -L <start>,<end>
rg -t go '<pattern>' /home/n0ko/Programs/ai/computeCommander
fd -t f -e go . /home/n0ko/Programs/ai/computeCommander
fd -t f -e rs . /home/n0ko/Programs/ai/computeCommander/plugins/focus-watcher

# AST + LSP probes
tree-sitter parse <file>             # via the tree-sitter-go grammar
# gopls is invoked via the LSP tool: goToDefinition, findReferences, hover, documentSymbol
# For Rust edits in plugins/focus-watcher/, rust-analyzer (via LSP) replaces gopls.

# Build / test / lint (run between change groups)
cd /home/n0ko/Programs/ai/computeCommander && make build
cd /home/n0ko/Programs/ai/computeCommander && go test ./...
cd /home/n0ko/Programs/ai/computeCommander && make vet
cd /home/n0ko/Programs/ai/computeCommander && make lint
# Rust focus-watcher (only when plugins/focus-watcher/ touched):
cd /home/n0ko/Programs/ai/computeCommander/plugins/focus-watcher && cargo build --release
cd /home/n0ko/Programs/ai/computeCommander/plugins/focus-watcher && cargo test

# Post-task activity log (always run before returning)
ob1 write --project computeCommander --key 'computeCommander/activity/<YYYY-MM-DD>/<task_id>.md' --file <path-to-entry>
```

### Hooks integration

Agent-level hooks are not added by this spec. The existing global `claude-hooks supervisor` and `context-inject` UserPromptSubmit hooks remain in force. The computeCommander repo's `CLAUDE.md` (T2) provides the project-scope override for orchestrator routing decisions. The orchestrator-injected context pattern (§Integration above) is enforced by the orchestrator's own logic, not by a Claude hook.

---

## What It Does NOT Do

Explicitly out of scope:

- **Does not author specs.** `spec-builder` owns spec authorship. `cmdr_coder` may read specs but never writes to `/home/n0ko/Programs/ai/computeCommander/SPEC/<spec_name>/`.
- **Does not perform spec reviews.** `spec-reviewer` owns spec reviews. `cmdr_coder` may read reviews to ground edits but never writes to `/home/n0ko/Programs/ai/computeCommander/SPEC/<spec_name>/REVIEWS/`.
- **Does not review code authored by the same instance/session.** Self-review is a routing violation per §3 principle 12. The instance refuses with `Blocked: self-review violation; spawn a fresh cmdr_coder instance for review`. Code review is performed by a separately-spawned `cmdr_coder` instance only.
- **Does not edit code outside `/home/n0ko/Programs/ai/computeCommander/`.** Cross-references to sister projects (claude-code, pi-mono, monty, crush, **icarus**) are read-only.
- **Does not write to ob1 outside the `computeCommander` project tag.** Attempts to write entries tagged `icarus/...`, `openbrain/...`, `monty/...`, etc. return `Blocked: ob1 project violation`.
- **Does not push or force-push.** Commits are local; pushing is the user's prerogative or a separate `merge-manager` task.
- **Does not amend prior commits.** Each logical change is its own NEW commit. (Aligns with global git-discipline rule.)
- **Does not skip pre-commit hooks.** `--no-verify` is forbidden.
- **Does not modify the `golang-coder` or `icarus_coder` agent definitions.** Those files at `/home/n0ko/.claude/agents/golang-coder.md` and `/home/n0ko/.claude/agents/icarus_coder.md` are untouched.
- **Does not introduce new top-level `pkg/` packages without justification.** `pkg/` is currently locked at `pkg/runtimes` and `pkg/integrations`; new public packages require a new spec, not an inline edit.
- **Does not introduce mocks in production paths.** Mocks live in `_test.go` files or behind `//go:build test`.
- **Does not modify Go module path or downgrade Go version.** Module is `github.com/noko/computecommander`; build Go is 1.25.0; do not bump or rename without an explicit task.
- **Does not bypass the AgentRuntime adapter pattern.** New runtime support goes through `pkg/runtimes/runtime.go` interface + self-registration via `init()` + `RegisterRuntime()`. No direct `cmd/cc` calls into provider SDKs.
- **Does not edit the K8s cluster TypeScript code outside `k8s-cluster/`.** That subtree is TypeScript/Bun-native and follows different conventions; cmdr_coder may read it for context but Go work stops at `k8s-cluster/`'s edge.
- **Does not write the SPEC dir migration of legacy `specs/` to `SPEC/<spec_name>/`.** That migration is out of scope; the rule introduction (T2) freezes legacy `specs/` and pins the new layout going forward.

---

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Language (Go build) | Go 1.25.0 | `go.mod` directive |
| Module | `github.com/noko/computecommander` | Authoritative module path per `go.mod`; agent must use this in all import paths it generates. |
| Language (Rust plugin) | Rust (focus-watcher) | `plugins/focus-watcher/` is a `/proc`-based pane-focus tracker required by the dashboard. Build via `cargo build --release`. |
| CLI framework | `spf13/cobra` | One command per file, exporting `XxxCmd(app *App) *cobra.Command`. |
| TUI / dashboard | `charmbracelet/bubbletea` + `charmbracelet/lipgloss` | Used in `internal/tui/`. Reuse for any dashboard-adjacent work. |
| Database (default) | `modernc.org/sqlite` (CGo-free SQLite) | Default driver; `.computecommander/cmdr.db`. |
| Database (alt) | `jackc/pgx/v5` (PostgreSQL) | K8s/cluster mode driver; same `DB` interface. |
| File watching | `fsnotify/fsnotify` | Triggers fsnotify → SIGUSR1 → file picker refresh. |
| PTY | `creack/pty` | Used by agent session pane. |
| LSP | `gopls` (via the harness LSP tool) for Go; `rust-analyzer` for Rust | gopls: goToDefinition, findReferences, hover, rename. rust-analyzer for the focus-watcher plugin only. |
| AST | `tree-sitter-go` (via tree-sitter CLI or library binding) | Syntactic-graph queries for blast-radius analysis before edits. `tree-sitter-rust` for plugin edits. |
| Knowledge fabric | ob1 (OpenBrain HTTP/CLI client), project tag `computeCommander` | The agent's project memory. Activity is logged here every task. |
| Build / test / lint | `make build`, `make test` (`go test ./...`), `make vet`, `make lint` (golangci-lint) | Existing Makefile targets; agent does NOT bypass them with raw `go` commands except for narrow probes. |
| Logging | `log/slog` structured key/value where present; existing code mixes `log` and `fmt` | Prefer slog in NEW code; do not silently rewrite legacy `log` calls without a task. |
| Search tools | `rg` over `grep`, `fd` over `find` | Global rule; aligns with user's CLAUDE.md. |
| Commit discipline | One scoped commit per logical change | Global git-discipline rule. |
| Multiplexer | Zellij (KDL layout files in `.computecommander/layouts/`) | Pane definitions; do NOT introduce invalid attrs (history shows `2aa9af7 layout: drop invalid color attr from calcurse pane`). |
| Window manager | WezTerm | `internal/wezterm/` handles window-level orchestration. |

---

## Project Infrastructure

### Directory structure (anchor for the agent's working environment)

```
/home/n0ko/Programs/ai/computeCommander/
  go.mod                                 # module github.com/noko/computecommander, go 1.25.0
  go.sum
  README.md
  Makefile                               # build, build-focus-watcher, build-bridge, test, lint, vet, install, install-bridge, generate-types, clean
  CLAUDE.md                              # project rule (PINNED ORCHESTRATION RULE → cmdr_coder, prepended by T2)
  SPEC.md                                # legacy spec at repo root (frozen reference)
  SPEC-REVIEW.md                         # legacy review at repo root (frozen reference)
  agents.md                              # legacy agent role notes
  agentic_instructions.md                # repo-root scope context
  SPEC/                                  # SPEC LAYOUT RULE: per-spec subdirs (canonical going forward)
    CMDR_CODER_AGENT/
      CMDR_CODER_AGENT.md                # THIS FILE
      REVIEWS/
        CMDR_CODER_AGENT_REVIEW.md       # spec review (parent workflow Phase 2)
        CMDR_CODER_AGENT_FILE_REVIEW.md  # review of the agent file produced by T1
  specs/                                 # legacy lowercase (frozen)
    computecommander-v1.md               # historical reference
    multi-agent-tracking.md
    session-persistence.md
    sse-relay.md
    trustgraph-visualization.md
    linkedin-post-generator.md
    jira-board-generator.md
    go-typescript-bridge.md
    openbrain-rules.md
    crush-ui-spec.md
    agents-md.md
    index.md
    reviews/                             # legacy review subdir (frozen)
  cmd/
    cc/                                  # main CLI binary (cmdr)
    hook-bridge/                         # Go-TS bridge multiplexer binary
  pkg/
    runtimes/                            # AgentRuntime interface + 5 adapters (Claude/Gemini/Codex/Pi/Goose)
    integrations/                        # GitHub, Linear, Webhook stubs
  internal/
    agentic/                             # agent role definitions / overlays
    agents/                              # agent lifecycle: spawn, stop, guards, overlays
    backup/                              # database backup and restore
    commands/                            # CLI command handlers (App DI container)
    config/                              # configuration schema, loading, validation, file watcher
    darkfactory/                         # darkfactory pipeline integration
    export/                              # database export to JSON/CSV
    gateway/                             # HTTP REST API gateway (/api/v1/)
    jiraboard/                           # Jira board generator
    keybinds/                            # leader-key keybind config and action registry
    linkedin/                            # LinkedIn post generator pipeline
    mail/                                # inter-agent mail system with priorities
    merge/                               # FIFO merge queue + 4-tier conflict resolution
    platform/db/                         # database abstraction (SQLite + PostgreSQL)
    sse/                                 # Server-Sent Events relay
    trustgraph/                          # trustgraph viz / event source
    tui/                                 # BubbleTea dashboard (status, mail, costs, file picker, sessions)
    watchdog/                            # 3-tier health monitoring daemon
    wezterm/                             # WezTerm window management
    worktree/                            # git worktree lifecycle
    zellij/                              # zellij pane management + KDL layout generation
  bridge/                                # (planned per specs/go-typescript-bridge.md) Go-TS bridge
  agents/                                # YAML agent role definitions
  templates/                             # Go text/template overlays
  migrations/                            # root-level SQL migration mirrors
  scripts/                               # shell scripts (agent wrapper for session-switch support)
  plugins/
    focus-watcher/                       # Rust /proc-based pane focus watcher
  k8s-cluster/                           # Kubernetes infrastructure (TypeScript/Bun) — read-only for cmdr_coder
  .computecommander/                     # project-init runtime data (config.yaml, scripts, layouts, db)
```

`pkg/` is locked at the 2 public entry-point packages (`runtimes`, `integrations`). New top-level public packages require a new spec, not an inline edit.

### Make targets (the agent uses these — does NOT reinvent)

| Target | Command |
|--------|---------|
| `make build` | `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml` then `go build $(LDFLAGS) -o cmdr ./cmd/cc/` |
| `make build-focus-watcher` | `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml` |
| `make build-bridge` | `go build $(LDFLAGS) -o bin/hook-bridge ./cmd/hook-bridge/` |
| `make test` | `go test ./...` (no `-race` by default — add it explicitly when probing concurrency) |
| `make vet` | `go vet ./...` |
| `make lint` | `make vet` then `golangci-lint run ./...` (lint is best-effort: skipped if golangci-lint is missing) |
| `make install` | builds + copies `cmdr` and `focus-watcher` into `~/.local/bin/` |
| `make install-bridge` | builds + copies `hook-bridge` into `~/.local/bin/` |
| `make generate-types` | `go run ./cmd/hook-bridge/ --generate` (regenerates TS types from Go structs) |
| `make clean` | removes `cmdr`, `bin/hook-bridge`, and `cargo clean` for the focus-watcher |

### CI workflow

There is no `.github/workflows/` configuration committed to this repo at the time of this spec. The agent does NOT introduce CI without an explicit task that says so.

### Version management

Per the Makefile: `VERSION ?= 0.2.0`, `COMMIT := $(shell git rev-parse --short HEAD)`. The agent does NOT bump versions; that is a release-manager task.

---

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| Agent definition (`/home/n0ko/.claude/agents/cmdr_coder.md`) | 1 | ~290 |
| Project rule prepend (`/home/n0ko/Programs/ai/computeCommander/CLAUDE.md`) | 1 | ~70 (prepended) |
| Per-directory `agentic_instructions.md` stubs (4 new + 1 update) | 5 | ~40 each (~200 total) |
| **Total (T1 + T2 + T3 deliverables)** | **7** | **~560** |
| THIS spec (already written) | 1 | ~720 |

Note: this spec produces NO Go or Rust code. All implementation work is downstream — initiated by T5 (self-test) and every subsequent task routed to `cmdr_coder`.

---

# EXECUTION SECTIONS (15-19)

---

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | claude-agent | Author `/home/n0ko/.claude/agents/cmdr_coder.md` per §3 + §10 + §12 of THIS spec. Frontmatter MUST set `name: cmdr_coder`, `color: teal`, `model: claude-opus-4-7`, `tools: Read, Write, Edit, Bash, Grep, Glob, LSP`. Body MUST include all 12 sections listed in §4 (Mission, Working Environment, Architecture Overview, Tooling, Workflow Protocol, Project Rules, Coding Conventions, Make Targets, Success Criteria, Knowledge Growth Protocol, Gotchas, Cross-Project References). Use `/home/n0ko/.claude/agents/icarus_coder.md` and the icarus_coder spec for STRUCTURE only — content is computeCommander-specific (different module, stack, surfaces). Commit message: `agent(cmdr_coder): add project-scoped coder agent definition` (NOTE: this commit lands in `~/.claude/`, not in computeCommander; treat that location's commit policy per the user's `~/.claude/` repo conventions). | `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md`, `/home/n0ko/.claude/agents/icarus_coder.md` (structural template), `/home/n0ko/.claude/agents/golang-coder.md` (foundation, if present), `/home/n0ko/Programs/ai/computeCommander/README.md`, `/home/n0ko/Programs/ai/computeCommander/SPEC.md`, `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md`, `/home/n0ko/Programs/ai/computeCommander/Makefile`, `/home/n0ko/Programs/ai/computeCommander/go.mod` | `/home/n0ko/.claude/agents/cmdr_coder.md` | -- | `test -f /home/n0ko/.claude/agents/cmdr_coder.md && rg -q '^name: cmdr_coder' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q '^color: teal' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'tree-?sitter' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'gopls' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'ob1' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'computeCommander' /home/n0ko/.claude/agents/cmdr_coder.md` |
| T2 | unix-coder | PREPEND the verbatim three-rule block (PINNED ORCHESTRATION RULE + REVIEWER INDEPENDENCE RULE + SPEC LAYOUT RULE) from §4 of THIS spec to the existing `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md`. Existing "Specs" section content MUST be preserved below the prepended block. Commit message: `chore(claude): pin cmdr_coder as sole code-edit agent + add reviewer independence + spec layout rules` | `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md`, `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md` (existing — read first to preserve content) | `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md` | T1 | `test -f /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'cmdr_coder' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'PINNED ORCHESTRATION RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'REVIEWER INDEPENDENCE RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'SPEC LAYOUT RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'specs/' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` |
| T3 | unix-coder | Create or update five `agentic_instructions.md` anchors for cmdr_coder's scope: (a) repo root — UPDATE existing file, append a "cmdr_coder scope anchor" section; (b) `cmd/`, (c) `internal/`, (d) `pkg/`, (e) `plugins/` — CREATE NEW. Each NEW stub MUST contain: scope boundary one-liner, key abstractions table (e.g., for `internal/`: AgentRuntime, MailStore, MergeQueue, etc.), build/test/lint commands relevant to that subtree, and a "Read first" file list. Commit message: `docs(aid): stub agentic_instructions for cmdr_coder scope anchors`. | `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md`, `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` (existing), `/home/n0ko/Programs/ai/computeCommander/Makefile`, `/home/n0ko/Programs/ai/computeCommander/go.mod`, `/home/n0ko/Programs/ai/computeCommander/cmd`, `/home/n0ko/Programs/ai/computeCommander/internal`, `/home/n0ko/Programs/ai/computeCommander/pkg`, `/home/n0ko/Programs/ai/computeCommander/plugins` | `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md`, `/home/n0ko/Programs/ai/computeCommander/cmd/agentic_instructions.md`, `/home/n0ko/Programs/ai/computeCommander/internal/agentic_instructions.md`, `/home/n0ko/Programs/ai/computeCommander/pkg/agentic_instructions.md`, `/home/n0ko/Programs/ai/computeCommander/plugins/agentic_instructions.md` | T1 | `for f in /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/cmd/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/internal/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/pkg/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/plugins/agentic_instructions.md; do test -s "$f" || exit 1; done && rg -q 'cmdr_coder' /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` |
| T4 | spec-reviewer | Review the `cmdr_coder` AGENT FILE (the file produced by T1 at `/home/n0ko/.claude/agents/cmdr_coder.md`) against THIS spec. Output a verdict (PASS / PASS WITH WARNINGS / FAIL) plus a numbered findings list. Output path: `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md`. NOTE: this is distinct from the spec-review of THIS spec, which is performed in the parent workflow's Phase 2 by a separate spec-reviewer dispatch and is NOT part of this Task Manifest. Commit message: `review(cmdr_coder): agent definition file review` | `/home/n0ko/.claude/agents/cmdr_coder.md`, `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` | `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md` | T1 | `test -f /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md && rg -q -i 'verdict' /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md` |
| T5 | cmdr_coder | Self-test: perform a no-op probe task to validate tool wiring. Steps: (1) read this spec; (2) read 5 ob1 entries tagged `project=computeCommander` (or report if ob1 unreachable); (3) run `git -C /home/n0ko/Programs/ai/computeCommander log --oneline -10`; (4) run `cd /home/n0ko/Programs/ai/computeCommander && make build && go test ./... && make vet`; (5) run a tree-sitter-go parse on `internal/agents/spawner.go`; (6) run gopls `goToDefinition` on a known symbol (e.g., `AgentRuntime`); (7) write a self-test activity entry to ob1 at `computeCommander/activity/<YYYY-MM-DD>/T5-self-test.md` (project tag = `computeCommander`); (8) commit nothing — this is a probe. Output: `Done.` on full pass, otherwise `Blocked: <which step failed>`. | THIS spec, `/home/n0ko/.claude/agents/cmdr_coder.md`, repo (read-only) | -- (probe is read-only) | T1, T2, T3 | `cd /home/n0ko/Programs/ai/computeCommander && make build && go test ./... && make vet` |

Notes on the manifest:

- Every row has a non-empty Verify Command (shell predicate exiting 0 on success).
- Every agent name comes from the known roster: `claude-agent`, `unix-coder`, `spec-reviewer`, `cmdr_coder` (added by T1, becomes available for T5).
- T1 and T2 each have a single write target. T3 writes 5 files (one update + 4 new `agentic_instructions.md` stubs); these are committed in a single scoped commit.
- No two tasks write to the same file. T1, T2, T3, T4 have disjoint write scopes; T5 writes nothing on disk (only an ob1 entry).
- Every file in any Task Manifest write-scope appears in §17 Target State. Every Target State entry appears in at least one task's write-scope.
- T1 writes outside the computeCommander repo (`~/.claude/agents/`). Its commit policy follows the `~/.claude/` repo's own conventions, NOT computeCommander's.

---

## 16. Dependency Graph

```
Phase 1 (single task): [T1]
  T1: author /home/n0ko/.claude/agents/cmdr_coder.md

Phase 2 (parallel, after Phase 1): [T2, T3]
  T2: prepend three-rule block to /home/n0ko/Programs/ai/computeCommander/CLAUDE.md
  T3: create/update agentic_instructions.md anchors (5 files)

Phase 3 (after Phase 2): [T4]
  T4: spec-reviewer reviews the agent file at /home/n0ko/.claude/agents/cmdr_coder.md
      → output to /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md

Phase 4 (after Phase 3, gated on T4 verdict != FAIL): [T5]
  T5: cmdr_coder self-test (no-op probe)
```

The graph is acyclic. No task depends transitively on itself.

---

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `/home/n0ko/.claude/agents/cmdr_coder.md` | ~290 | No |
| `/home/n0ko/Programs/ai/computeCommander/cmd/agentic_instructions.md` | ~40 | No |
| `/home/n0ko/Programs/ai/computeCommander/internal/agentic_instructions.md` | ~50 | No |
| `/home/n0ko/Programs/ai/computeCommander/pkg/agentic_instructions.md` | ~40 | No |
| `/home/n0ko/Programs/ai/computeCommander/plugins/agentic_instructions.md` | ~40 | No |
| `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md` | ~80 | No |

Files modified:

| File Path | Modification |
|-----------|--------------|
| `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md` | T2 PREPENDS the three-rule block; existing "Specs" section preserved below |
| `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` | T3 APPENDS a "cmdr_coder scope anchor" section to the existing file |

Files deleted: None.

Out of scope (NOT touched by THIS spec; called out for clarity):

- `/home/n0ko/Programs/ai/computeCommander/SPEC.md` — legacy repo-root spec; frozen.
- `/home/n0ko/Programs/ai/computeCommander/SPEC-REVIEW.md` — legacy repo-root review; frozen.
- `/home/n0ko/Programs/ai/computeCommander/specs/` (lowercase legacy) — frozen, untouched. Migration to `SPEC/<spec_name>/` is a separate, explicit task.
- `/home/n0ko/.claude/agents/golang-coder.md` — untouched.
- `/home/n0ko/.claude/agents/icarus_coder.md` — untouched (sister agent, structural template only).
- `/home/n0ko/Programs/ai/computeCommander/.computecommander/scripts/tg-viz.html` — pre-existing WIP on the `pi` branch; do NOT stage or modify.

---

## 18. Verification Plan

**Per-task checks** (from Task Manifest Verify Command column):

- T1: `test -f /home/n0ko/.claude/agents/cmdr_coder.md && rg -q '^name: cmdr_coder' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q '^color: teal' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'tree-?sitter' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'gopls' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'ob1' /home/n0ko/.claude/agents/cmdr_coder.md && rg -q 'computeCommander' /home/n0ko/.claude/agents/cmdr_coder.md`
- T2: `test -f /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'cmdr_coder' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'PINNED ORCHESTRATION RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'REVIEWER INDEPENDENCE RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md && rg -q 'SPEC LAYOUT RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md`
- T3: `for f in /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/cmd/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/internal/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/pkg/agentic_instructions.md /home/n0ko/Programs/ai/computeCommander/plugins/agentic_instructions.md; do test -s "$f" || exit 1; done`
- T4: `test -f /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md && rg -q -i 'verdict' /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md`
- T5: `cd /home/n0ko/Programs/ai/computeCommander && make build && go test ./... && make vet`

**Integration check** (after all 5 tasks pass):

```bash
test -f /home/n0ko/.claude/agents/cmdr_coder.md \
  && test -f /home/n0ko/Programs/ai/computeCommander/CLAUDE.md \
  && test -f /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md \
  && test -f /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md \
  && cd /home/n0ko/Programs/ai/computeCommander && make build && go test ./... && make vet
```

**Rollback** (if integration fails):

```bash
# Revert all commits made by T2..T4 inside computeCommander.
# T1 and T4 outputs live in different trees:
#   T1 → /home/n0ko/.claude/agents/cmdr_coder.md (outside repo)
#   T4 → /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/...

git -C /home/n0ko/Programs/ai/computeCommander log --oneline | rg -E 'cmdr_coder|chore\(claude\)|docs\(aid\)|review\(cmdr_coder\)'

git -C /home/n0ko/Programs/ai/computeCommander revert <sha>...
# OR (if no other commits since the rule introduction):
git -C /home/n0ko/Programs/ai/computeCommander reset --hard <sha-before-T2>

# Plus remove the agent file (lives outside the computeCommander repo):
rm /home/n0ko/.claude/agents/cmdr_coder.md
```

### Functional Smoke Tests

#### Binary Install Verification

```bash
# Build fresh and confirm the cmdr binary still exists and reports the current commit
cd /home/n0ko/Programs/ai/computeCommander && make build
./cmdr --version 2>&1 | grep -q "$(git rev-parse --short HEAD)"
```

#### Layout/Config Validation

This spec does NOT modify any KDL layouts or config YAML, so no layout-validation smoke is required for the rule-introduction phase. The cmdr_coder self-test (T5) does NOT touch dashboard config.

---

## 19. Success Criteria (Machine-Verifiable)

- [ ] `test -f /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q '^name: cmdr_coder' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q '^color: teal' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q 'tree-?sitter' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q 'gopls' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q 'ob1' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q 'computeCommander' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q 'tools:.*LSP' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `rg -q 'model: claude-opus-4-7' /home/n0ko/.claude/agents/cmdr_coder.md` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` exits 0
- [ ] `rg -q 'cmdr_coder' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` exits 0
- [ ] `rg -q 'PINNED ORCHESTRATION RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` exits 0
- [ ] `rg -q 'REVIEWER INDEPENDENCE RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` exits 0
- [ ] `rg -q 'SPEC LAYOUT RULE' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` exits 0
- [ ] `test -d /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` exits 0
- [ ] `test -s /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` exits 0
- [ ] `test -s /home/n0ko/Programs/ai/computeCommander/cmd/agentic_instructions.md` exits 0
- [ ] `test -s /home/n0ko/Programs/ai/computeCommander/internal/agentic_instructions.md` exits 0
- [ ] `test -s /home/n0ko/Programs/ai/computeCommander/pkg/agentic_instructions.md` exits 0
- [ ] `test -s /home/n0ko/Programs/ai/computeCommander/plugins/agentic_instructions.md` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md` exits 0
- [ ] `(cd /home/n0ko/Programs/ai/computeCommander && make build)` exits 0
- [ ] `(cd /home/n0ko/Programs/ai/computeCommander && go test ./...)` exits 0
- [ ] `(cd /home/n0ko/Programs/ai/computeCommander && make vet)` exits 0

> **EXEC NOTE:** The `make build`, `go test`, and `make vet` gates above MUST be run on a working tree where the existing pre-T1 WIP (`.computecommander/scripts/tg-viz.html`) has been stashed or otherwise excluded. A pre-existing repo break caused by uncommitted WIP does not constitute a T5 failure. `make lint` is best-effort: golangci-lint may not be installed; in that case `make lint` short-circuits and is not a gating check.

---

# SUPPLEMENTARY SECTIONS

---

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| T1 (author agent definition file) | `claude-agent` | claude-agent is the canonical author for `.claude/agents/*.md` definition files; it understands the frontmatter schema and body conventions. |
| T2 (prepend project CLAUDE.md rules) | `unix-coder` | Plain markdown file; unix-coder is the default for project-level docs and rules outside of code. NOTE: after T2 lands, unix-coder is BANNED from this tree by the very rule it just installed — subsequent doc edits in this repo go to cmdr_coder. |
| T3 (stub agentic_instructions.md anchors) | `unix-coder` | 5 short markdown stubs/updates across the repo tree; matches unix-coder's "non-Go documentation" niche. Same banning caveat as T2. |
| T4 (review the agent definition file) | `spec-reviewer` | spec-reviewer's domain — verdict format, completeness check, alignment with the source spec. |
| T5 (cmdr_coder self-test probe) | `cmdr_coder` | The agent must validate its own tool wiring before downstream tasks rely on it. This is its FIRST task in production. |

---

## Execution Order

```
Phase 1: Author the agent
  +-- T1 (claude-agent) — /home/n0ko/.claude/agents/cmdr_coder.md

Phase 2: Wire the project [parallel after Phase 1]
  +-- T2 (unix-coder) — /home/n0ko/Programs/ai/computeCommander/CLAUDE.md (prepend three rules)
  +-- T3 (unix-coder) — agentic_instructions.md anchors (1 update + 4 new)

Phase 3: Review the agent [blocked by Phase 2]
  +-- T4 (spec-reviewer) — /home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md

Phase 4: Self-test [blocked by Phase 3 verdict != FAIL]
  +-- T5 (cmdr_coder) — read-only probe, build/test/vet, ob1+gopls+tree-sitter sanity check
```

Recommended directive: `/swarm` — full execution chains all 5 tasks deterministically across the 4 phases. `/pai` is acceptable for single-task focus runs (e.g., re-running T4 after a T1 patch).

---

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| T1 frontmatter omits `color: teal` | T1 verify command fails (`rg -q '^color: teal'`) | claude-agent re-runs T1 with explicit reminder; verify command re-runs. |
| T1 body omits any of the 12 mandated sections | T4 spec-reviewer flags missing section | spec-reviewer outputs FAIL verdict with section list; T1 re-runs to add missing sections. |
| T1 body content carries over icarus-specific language (langchaingo, parity harness, JSONL session, sidecar) | T4 spec-reviewer flags content drift; the spec's "port notice" header explicitly forbids this | T1 re-runs with stricter instruction to mine STRUCTURE only; T4 re-reviews. |
| T2 fails to preserve existing CLAUDE.md "Specs" content | T2 verify also runs `rg -q 'specs/' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` (existing content reference); if missing, content was destroyed | unix-coder re-runs T2, this time prepending instead of overwriting; recovery uses `git restore` on CLAUDE.md if a bad commit landed. |
| T3 `agentic_instructions.md` stubs are empty (touch but no content) | T3 verify uses `test -s` (not `test -f`); empty files fail | unix-coder re-runs T3 with explicit content requirement. |
| T3 root `agentic_instructions.md` update is missing the `cmdr_coder` reference | T3 verify includes `rg -q 'cmdr_coder' /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` | unix-coder re-runs T3 to append the missing scope-anchor section. |
| T4 spec-reviewer cannot find the agent file | T1 has not committed yet, or wrong path | block T4 until T1 verify exits 0; then resume. |
| T5 `make build` fails because `cargo` is missing | step (4) fails on focus-watcher build | install Rust toolchain via `rustup` (`rustup default stable`) before re-running. NOT a T5 logic failure — environment precondition. |
| T5 `go test ./...` fails | the agent's environment is broken (build break, test fail) | NOT a T5 failure per se — T5 surfaces a pre-existing repo break. Resolve the repo break in a separate task before re-running T5. |
| T5 ob1 unreachable | network/daemon down | T5 reports `Blocked: ob1 unreachable`; treat as a non-fatal partial pass; re-run T5 once ob1 is up. |
| T5 gopls / tree-sitter not installed on the host | step (5) or (6) returns "command not found" | install gopls (`go install golang.org/x/tools/gopls@latest`) and tree-sitter-go before re-running. |
| Conflict with existing repo-root SPEC.md / agents.md / specs/ legacy layout | orchestrators may still point users at `specs/` lowercase | T2's CLAUDE.md SPEC LAYOUT RULE explicitly says `SPEC/<spec_name>/` is canonical going forward; existing `specs/` is frozen, NOT migrated. Migration is a separate spec. |
| Orchestrator forgets to inject ob1 + git context (per §10 Integration) | activity entry has `injected_by_orch=false` and a manual bootstrap-read trace | non-fatal at T5 (probe), but follow-up dispatches MUST be flagged. The orchestrator-injection requirement is a contract on the orchestrator, enforced by review of the activity entries over time. |

---

## Open Questions

| # | Question | Impact | Suggested Default |
|---|----------|--------|-------------------|
| 1 | Does `/home/n0ko/.claude/agents/golang-coder.md` exist as a foundation file to fork from? | T1 has no concrete source to mine for body content; falls back to the icarus_coder agent file at `/home/n0ko/.claude/agents/icarus_coder.md` and to §3+§10+§12+§13 of THIS spec. | Treat `/home/n0ko/.claude/agents/icarus_coder.md` as the structural template; mine for sections (Mission, Working Environment, Architecture Overview, Coding Conventions, Workflow Protocol, Knowledge Growth, Gotchas, Success Criteria). DO NOT carry over icarus-specific content (langchaingo, parity harness, JSONL session, sidecar). |
| 2 | What is the exact ob1 client interface available to cmdr_coder — is it a host-level `ob1` CLI, a Go HTTP client, or both? | Determines whether ob1 reads/writes go through Go-process bindings or shell-out. T5 self-test command shape depends on this. | Assume host-level `ob1` CLI is available in PATH (consistent with the `ob1` alias in user's CLAUDE.md). T5 uses `ob1 read --project computeCommander` shell calls. If the CLI is absent, T5 reports `Blocked: ob1 CLI not found` and the orchestrator surfaces this as an installation precondition. |
| 3 | Does ob1 use a `--namespace` flag (icarus pattern) or a `--project` flag (this spec's choice)? | Determines the exact CLI invocation in the agent body and T5. | Spec proceeds with `--project computeCommander`. If the actual ob1 CLI uses `--namespace`, the agent file (T1) and the agentic_instructions stubs (T3) substitute the correct flag at author time; the activity-entry shape (project tag) is unchanged either way. |
| 4 | Does the existing `specs/index.md` need to be updated to reference the new `SPEC/CMDR_CODER_AGENT/` location? | If readers consult `specs/index.md` first, they may not discover the new layout. | Recommended follow-up: add a one-line pointer to `specs/index.md` directing readers to `SPEC/<spec_name>/` for new specs. Out of scope for THIS spec — handled in a separate task. |
| 5 | Is there a sister `pi_coder` or similar agent for the `pi-mono` repo, given that the current branch is named `pi`? | If yes, cross-project cross-references in the agent body should mention it. | None known at the time of this spec. The Cross-Project References table lists `pi-mono` as a read-only reference; if `pi_coder` is later added, the agent body learns about it via Knowledge Growth Protocol. |
| 6 | Should the agent be permitted to edit `k8s-cluster/` (TypeScript/Bun)? | The repo contains TypeScript code outside the Go scope; cmdr_coder's foundation is `golang-coder`. | Spec choice: NO. cmdr_coder is scope-locked to Go and Rust (focus-watcher). Edits to `k8s-cluster/` go to a TypeScript-capable agent (e.g., `unix-coder` or a future `cmdr_ts_coder`). This is recorded in §11 (What It Does NOT Do). |

---

## Domain-Specific Reference

### Cross-Project References (cmdr_coder agent memory)

The agent's body MUST include a "Cross-Project References" section listing these sister projects, each with one line on what to learn:

| Project | Path | What to learn |
|---------|------|---------------|
| claude-code | `~/Programs/ai/claude-code/` | Anthropic CLI: harness patterns for tool dispatch, hook lifecycle, subagent envelope shape — reference for how a cmdr_coder agent SHOULD be invoked. |
| pi-mono | `~/Programs/ai/pi-mono/` | The Pi runtime's TypeScript implementation. Source for how the Pi `AgentRuntime` adapter (`pkg/runtimes/pi/pi.go`) interoperates. The current branch is `pi`, indicating active integration work. |
| monty | `~/Programs/ai/monty/` | Related TUI work; reference for BubbleTea/Lipgloss patterns used in `internal/tui/`. |
| crush | `~/Programs/ai/crush/` | Sister Go-native coding-agent harness; relevant to the `crush` runtime adapter, if/when present. Read for divergence analysis. |
| icarus | `~/Programs/ai/icarus/` | **Sister project — the agent definition pattern itself was ported from `icarus_coder`.** Read `SPEC/ICARUS_CODER_AGENT/ICARUS_CODER_AGENT.md` and `/home/n0ko/.claude/agents/icarus_coder.md` for shape. computeCommander and icarus solve overlapping problems (agent orchestration); read for divergence — `cmd/cc` vs. `cmd/icarus`, `pkg/runtimes` vs. `pkg/providers`, KDL/Zellij UI vs. charmbracelet REPL. Do NOT carry icarus assumptions (langchaingo, parity harness, JSONL session) into computeCommander. |

These five references are stored under the agent's "Knowledge Growth" register; `LEARNINGS.md` entries about cross-project divergence get appended there over time.

### Workflow Protocol (canonical sequence — agent body must reproduce this verbatim)

For every non-trivial task, the agent MUST execute these 11 steps in order:

1. Re-read the current task brief and any orchestrator-supplied context (ob1 entries, git history, agentic_instructions snippets — all PRE-INJECTED per §10).
2. Pull additional ob1 entries tagged `project=computeCommander` via `ob1 read --project computeCommander` and `ob1 list --project computeCommander --prefix <prefix>` for any tangential keys discovered during planning. Record the keys read in the activity entry.
3. Run `git -C /home/n0ko/Programs/ai/computeCommander log --oneline -20 -- <touched paths>` and `git blame <file> -L <range>` on every function to be modified. Record the commit shas in the activity entry. (Top-level `git log -20` is pre-injected; this step is for path-specific blame.)
4. Use tree-sitter (`tree-sitter-go`) to enumerate callsites and dependent symbols of any function or type to be changed — particularly methods on `AgentRuntime`, `DB`, `MailStore`, `MergeQueue`, `WorktreeManager`, `PaneManager`, `WindowManager`. For Rust edits in `plugins/focus-watcher/`, use `tree-sitter-rust`. Record the AST queries.
5. Use gopls via the LSP tool — `goToDefinition`, `findReferences` — to confirm the syntactic blast radius. For Rust, use `rust-analyzer`. Record the LSP calls.
6. State a planned approach in 2–3 sentences and the precise file scope. Halt for orchestrator confirmation if the plan extends beyond the prompt envelope's `file_scope`.
7. Implement. Run `cd /home/n0ko/Programs/ai/computeCommander && make build`, then `go test ./...`, then `make vet` between change groups. If the change touches `plugins/focus-watcher/`, also run `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml && cargo test`.
8. If the change touches `cmd/cc`, `pkg/runtimes`, `internal/agents`, `internal/platform/db`, or `internal/zellij`, run an additional dashboard smoke: `timeout 5s ./cmdr dashboard --tui 2>&1 | head -20; test $? -eq 124 -o $? -eq 0`.
9. Append the activity log entry to ob1: `ob1 write --project computeCommander --key 'computeCommander/activity/<YYYY-MM-DD>/<task_id>.md' --file <local-path>`.
10. Commit ONE scoped commit per logical change. Subject format: `<type>(<surface>): <imperative summary>`. Surfaces drawn from §13 directory tree.
11. Output exactly one of: `Done.` / `Done. Output: <path>` / `Blocked: <reason>` / `Error: <desc>`.

---

## Knowledge Growth Protocol

When the agent learns something non-obvious about computeCommander — a Cobra command-tree quirk, a KDL-layout edge case, a dual-DB driver subtlety, an AgentRuntime adapter constraint, a focus-watcher `/proc` parsing detail, an ob1 project-tag gotcha, a Zellij pane attribute that breaks the layout (cf. commit `2aa9af7 layout: drop invalid color attr from calcurse pane`) — it appends a one-line dated entry to a `LEARNINGS.md` in the relevant subtree:

- Repo-wide: `/home/n0ko/Programs/ai/computeCommander/LEARNINGS.md`
- Per-package: `/home/n0ko/Programs/ai/computeCommander/<pkg>/LEARNINGS.md` when the lesson is package-local.

Entries are dated, specific, and one line each. Format:

```
2026-04-26 KDL layout: pane `color` attr is not universally accepted by Zellij; dropped from calcurse pane in 2aa9af7. Validate every pane attribute against the Zellij KDL grammar before adding.
```

Knowledge growth is gated by the same scope-lock as edits: the agent does NOT write LEARNINGS files outside `/home/n0ko/Programs/ai/computeCommander/`.

---

## Gotchas

- **The build requires Rust.** `make build` invokes `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml`. If `cargo` is missing, the build hard-fails before Go compilation. Install `rustup` and `cargo` before any T5 self-test or downstream task.
- **`make test` does NOT run with `-race` by default.** If you suspect a data race, run `go test ./... -race` explicitly. Do not silence races by removing the `-race` flag.
- **`make lint` is best-effort.** golangci-lint may not be installed; in that case `make lint` short-circuits with a notice. Treat as warning, not failure, for CI gates.
- **Module path is `github.com/noko/computecommander`.** Lowercase, single word. Do not use camelCase or substitute `github.com/Nokodoko/...` (that pattern is icarus, not computeCommander).
- **Go version is 1.25.0 per `go.mod`.** Do not bump or downgrade without an explicit task.
- **Cobra commands live one-per-file in `cmd/cc/`.** Each exports `XxxCmd(app *App) *cobra.Command`. Adding a command means adding a new file in this exact pattern, not extending an existing one.
- **AgentRuntime is the universal adapter.** Five implementations (Claude/Gemini/Codex/Pi/Goose) self-register via `init()` + `RegisterRuntime()` in `pkg/runtimes/`. Adding a runtime means adding a new package under `pkg/runtimes/<name>/` with its own `init()`, NOT editing `cmd/cc`.
- **Dual-DB invariant.** `internal/platform/db/` exposes a single `DB` interface with two backing implementations (SQLite via `modernc.org/sqlite`, PostgreSQL via `pgx/v5`). Migrations live both in `internal/platform/db/migrations/` and the root-level `migrations/` mirror; keep them in sync.
- **KDL layouts are picky.** Invalid Zellij pane attributes silently break the layout (cf. `2aa9af7`). Validate KDL changes by launching the dashboard with the new layout and confirming all panes render.
- **Branch is `pi`, not `main`.** Active integration work for the Pi runtime adapter is on the `pi` branch; `main` may lag. Read recent `pi`-branch commits before assuming a feature is shipped.
- **Pre-existing WIP on `pi`:** `.computecommander/scripts/tg-viz.html` is modified but not committed. Do NOT stage it as part of any cmdr_coder task; it belongs to a separate workstream.
- **`specs/` (lowercase) is FROZEN under the SPEC LAYOUT RULE.** Historical specs (`computecommander-v1.md`, `multi-agent-tracking.md`, `session-persistence.md`, etc.) remain readable but no NEW spec lands there. New specs go to `SPEC/<spec_name>/`.
- **Repo-root `SPEC.md` and `SPEC-REVIEW.md` are legacy.** They are not part of the new `SPEC/<spec_name>/` layout and SHOULD NOT be expanded. New spec work creates `SPEC/<NEW_SPEC>/<NEW_SPEC>.md`.
- **`k8s-cluster/` is TypeScript/Bun.** cmdr_coder's foundation is `golang-coder`; do NOT edit `k8s-cluster/` Go-style. Read-only for cmdr_coder; route TS edits to a TypeScript-capable agent.
- **The agent's thread cwd is reset between Bash calls.** Always use absolute paths in Bash commands; chain with `&&` or pass `-C /home/n0ko/Programs/ai/computeCommander` to git when needed.
- **Reviewer independence.** The instance that authored a diff is forbidden from reviewing it. Reviews come from a separately-spawned `cmdr_coder` instance only. See Project Rules #12 and the project-level `CLAUDE.md` REVIEWER INDEPENDENCE RULE.
- **Orchestrator pre-injection is a contract on the ORCHESTRATOR, not on the agent.** The agent expects `prior_ob1_entries` and `git_context` to arrive in the prompt envelope. If they do not, the agent proceeds with manual bootstrap and flags `injected_by_orch=false` in the activity entry. Repeated `false` flags indicate the orchestrator is broken; surface this in spec-review or in a follow-up task.

---
