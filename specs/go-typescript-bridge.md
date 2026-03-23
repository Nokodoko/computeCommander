# Go-TypeScript Bridge Layer

Translation layer that makes Go-built assets (hooks, tools, CLIs) consumable by the Pi agent's TypeScript extension system. Bridges the gap between computeCommander's Go infrastructure and Pi's TypeScript-native extension API.

Replaces the current manual porting approach (rewriting each Claude hook as a TypeScript extension in `~/.pi/agent/extensions/`) with an automated bridge that wraps Go binaries and exposes them through Pi's `ExtensionAPI` interface.

<!-- Spec Type: feature -->

## 2. Why

The project is heavily invested in Go:
- ComputeCommander is 100% Go (~30k LOC)
- Claude hooks are Bash/Python but increasingly being rewritten in Go (see `openbrain/hooks/go/`)
- Go provides superior concurrency, type safety, and single-binary deployment
- The `bitfield/script` pipeline library and `go_context.md` patterns are the foundation for all new tooling

Meanwhile, Pi requires TypeScript extensions:
- Pi's extension API (`ExtensionAPI` from `@mariozechner/pi-coding-agent`) only accepts TypeScript
- Events (`session_start`, `tool_result`, `tool_call`, etc.) must be handled via `pi.on()` callbacks
- Commands are registered via `pi.registerCommand()` with TypeScript handlers
- Tools are registered via `pi.registerTool()` with TypeScript implementations

The current approach is unsustainable:
- 11 hand-ported TypeScript extensions exist at `~/.pi/agent/extensions/`
- The `bidirectional-sync.ts` extension auto-generates stubs, but stubs merely shell out to the original scripts -- no native TypeScript behavior
- Every new Go hook requires a manual TypeScript port
- Feature drift between Go and TypeScript implementations is inevitable
- No type safety across the bridge -- Go structs are not reflected in TypeScript types

## 3. Design Principles

1. **Go is the source of truth.** Business logic lives in Go. TypeScript is a thin adapter layer.
2. **Single binary, multiple consumers.** Go assets compile to a single binary that both Claude hooks and Pi extensions can invoke.
3. **Structured communication.** Go binaries emit JSON on stdout; TypeScript extensions parse it. No string scraping.
4. **Type generation.** Go struct definitions generate TypeScript interfaces automatically.
5. **Event mapping is declarative.** A manifest file maps Go hook event types to Pi extension events.
6. **Zero manual porting.** New Go hooks automatically become available to Pi via the bridge.
7. **Graceful degradation.** If the Go binary is not found, the Pi extension logs a warning and skips.

## 4. On-Disk Format

```
~/.claude/
  bridge/
    manifest.json              -- Declarative mapping: Go hooks -> Pi events
    types/
      generated.d.ts           -- TypeScript interfaces generated from Go structs
    bin/
      hook-bridge              -- Go binary: multiplexer for all bridged hooks

~/.pi/agent/extensions/
    go-bridge.ts               -- Single Pi extension that loads manifest and dispatches
```

### manifest.json

```json
{
  "version": 1,
  "hooks": [
    {
      "name": "cmdr-bridge",
      "goPackage": "github.com/noko/computecommander/hooks/cmdr-bridge",
      "piEvents": ["session_start", "tool_result", "agent_end"],
      "claudeEvents": ["SessionStart", "PostToolUse", "SubagentStop"],
      "inputSchema": "CmdrBridgeInput",
      "outputSchema": "CmdrBridgeOutput"
    },
    {
      "name": "intent-build-verify",
      "goPackage": "github.com/noko/computecommander/hooks/intent-build-verify",
      "piEvents": ["tool_result"],
      "claudeEvents": ["PostToolUse"],
      "matcher": "Bash",
      "inputSchema": "BuildVerifyInput",
      "outputSchema": "BuildVerifyOutput"
    }
  ]
}
```

### generated.d.ts

```typescript
// Auto-generated from Go structs. DO NOT EDIT.
// Source: go generate ./bridge/types/...

export interface CmdrBridgeInput {
  action: string;
  sessionId?: string;
  agentName?: string;
}

export interface CmdrBridgeOutput {
  success: boolean;
  message?: string;
}

export interface BuildVerifyInput {
  command: string;
  cwd: string;
  toolResult: string;
}

export interface BuildVerifyOutput {
  triggered: boolean;
  passed: boolean;
  results: BuildVerifyResult[];
}

export interface BuildVerifyResult {
  label: string;
  passed: boolean;
  evidence: string;
  exitCode?: number;
}
```

## 5. Data Model

### Go Side

```go
// HookManifest is the top-level bridge configuration.
type HookManifest struct {
    Version int            `json:"version"`
    Hooks   []HookBinding  `json:"hooks"`
}

// HookBinding maps a Go hook to Pi events.
type HookBinding struct {
    Name         string   `json:"name"`
    GoPackage    string   `json:"goPackage"`
    PiEvents     []string `json:"piEvents"`
    ClaudeEvents []string `json:"claudeEvents"`
    Matcher      string   `json:"matcher,omitempty"`
    InputSchema  string   `json:"inputSchema,omitempty"`
    OutputSchema string   `json:"outputSchema,omitempty"`
}

// BridgeRequest is the JSON envelope sent from TypeScript to Go.
type BridgeRequest struct {
    Hook      string          `json:"hook"`
    Event     string          `json:"event"`
    Payload   json.RawMessage `json:"payload"`
    SessionID string          `json:"sessionId,omitempty"`
}

// BridgeResponse is the JSON envelope returned from Go to TypeScript.
type BridgeResponse struct {
    Success bool            `json:"success"`
    Output  json.RawMessage `json:"output,omitempty"`
    Error   string          `json:"error,omitempty"`
    Context string          `json:"context,omitempty"` // Injected into agent context
}
```

### TypeScript Side

```typescript
interface BridgeRequest {
  hook: string;
  event: string;
  payload: unknown;
  sessionId?: string;
}

interface BridgeResponse {
  success: boolean;
  output?: unknown;
  error?: string;
  context?: string; // Injected into agent context
}
```

## 6. CLI

### hook-bridge binary

```
hook-bridge <hook-name> [flags]

  Dispatches a hook invocation to the appropriate Go handler.
  Reads BridgeRequest JSON from stdin, writes BridgeResponse JSON to stdout.

  --list          List all registered hook bindings
  --manifest      Print the manifest.json path
  --generate      Regenerate TypeScript type definitions
  --validate      Validate manifest against registered hooks
```

### Type generation

```
go generate ./bridge/types/...

  Reads Go struct definitions annotated with `// bridge:export` comments
  and generates TypeScript interfaces in bridge/types/generated.d.ts.
```

## 7. JSON Output Format

All communication between Go and TypeScript uses JSON:

```json
// Request (TypeScript -> Go via stdin)
{
  "hook": "cmdr-bridge",
  "event": "session_start",
  "payload": { "action": "session-start" },
  "sessionId": "abc123"
}

// Response (Go -> TypeScript via stdout)
{
  "success": true,
  "output": { "registered": true },
  "context": "<bridge-inject>Agent registered successfully</bridge-inject>"
}

// Error response
{
  "success": false,
  "error": "database connection failed: timeout after 5s"
}
```

## 8. Concurrency Model

- **Go side:** Each hook invocation is a fresh process (exec from TypeScript). No persistent goroutines or shared state beyond what the hook itself manages (e.g., database connections).
- **TypeScript side:** The `go-bridge.ts` extension registers event handlers that spawn the Go binary via `pi.exec()`. Events are handled sequentially within Pi's event loop. No concurrency management needed on the TypeScript side.
- **Type generation:** Runs at build time via `go generate`. No runtime code generation.

## 9. Migration

### Phase 1: Bridge Infrastructure (this spec)
- Build `hook-bridge` Go binary
- Create `manifest.json` format and loader
- Build `go-bridge.ts` Pi extension
- Build type generation pipeline

### Phase 2: Port Existing Hooks
- Port `cmdr-bridge.sh` to Go (via hook-bridge)
- Port `intent-build-verify.py` to Go
- Port `sync-agents-dict.sh` to Go
- Update manifest with bindings
- Remove corresponding hand-written TypeScript extensions

### Phase 3: Deprecate Manual Porting
- Remove `bidirectional-sync.ts` auto-stub generation
- All new hooks written in Go first, auto-available to Pi
- TypeScript extensions directory contains only `go-bridge.ts` + project-specific extensions

## 10. Integration

### With Claude Code
- Claude hooks continue to fire as normal (bash/python scripts in `~/.claude/hooks/`)
- Go hooks can be called directly by Claude's PostToolUse mechanism
- The `intent-build-verify.py` hook detects `go build` for the bridge binary and runs smoke tests

### With Pi Agent
- `go-bridge.ts` is the single extension that loads `manifest.json` on `session_start`
- For each hook binding, it registers the appropriate `pi.on()` handlers
- On each event, it spawns `hook-bridge <name>` with the event payload on stdin
- Response JSON is parsed; `context` field is injected via `pi.sendMessage()`

### With ComputeCommander
- The `hook-bridge` binary is built alongside `cmdr` in the Makefile
- `make install` copies both binaries to `~/.local/bin/`
- The Pi runtime adapter (`pkg/runtimes/pi/pi.go`) can reference the bridge

### With bidirectional-sync.ts
- Phase 1: Both coexist. `bidirectional-sync.ts` still generates stubs for non-bridged hooks.
- Phase 3: `bidirectional-sync.ts` is retired. `go-bridge.ts` handles all Go hooks.

## 11. What It Does NOT Do

- Does not replace Pi's native TypeScript extension system. Project-specific extensions remain TypeScript.
- Does not provide a general-purpose Go-to-TypeScript FFI. This is specifically for hook/event bridging.
- Does not handle Pi's command registration (`pi.registerCommand()`). Commands remain TypeScript.
- Does not handle Pi's tool registration (`pi.registerTool()`). Tools remain TypeScript.
- Does not run Go code inside the Pi process (no WASM compilation). Communication is via subprocess + JSON.
- Does not require changes to the Pi agent source code (pi-mono).

## 12. Tech Stack

| Component | Technology | Justification |
|-----------|-----------|---------------|
| Bridge binary | Go 1.25 | Matches computeCommander; single binary deployment |
| Type generation | `go generate` + custom generator | No external dependencies; reads Go AST |
| Pi extension | TypeScript (Pi ExtensionAPI) | Required by Pi's extension system |
| Manifest | JSON | Simple, schema-validatable, no external parser needed |
| IPC | stdin/stdout JSON | Zero dependencies, works everywhere, debuggable |
| Build | Makefile target | Matches existing `make build` / `make install` |

## 13. Project Infrastructure

- Go module: `github.com/noko/computecommander` (existing)
- New packages: `bridge/`, `bridge/types/`, `bridge/hooks/`
- Makefile targets: `build-bridge`, `install-bridge`, `generate-types`
- Test: `go test ./bridge/...` + Pi extension integration test via `pi --mode rpc`

## 14. Estimated Size

| Component | Lines |
|-----------|-------|
| `bridge/bridge.go` (manifest loader, dispatcher) | ~200 |
| `bridge/types/generator.go` (TypeScript type gen) | ~300 |
| `bridge/types/generated.d.ts` (output) | ~100 |
| `bridge/hooks/` (Go hook implementations) | ~500 (grows with hooks) |
| `cmd/hook-bridge/main.go` (CLI entry) | ~80 |
| `go-bridge.ts` (Pi extension) | ~150 |
| `manifest.json` | ~50 |
| Tests | ~400 |
| **Total** | **~1780** |

---

# EXECUTION SECTIONS (15-19)

---

## 15. Task Manifest

| ID | Agent | Description | File Scope (Read) | File Scope (Write) | Depends On | Verify Command |
|----|-------|-------------|-------------------|-------------------|------------|----------------|
| T1 | unix-coder | Create bridge package with manifest loader and dispatcher | `pkg/runtimes/runtime.go`, `pkg/runtimes/pi/pi.go` | `bridge/bridge.go`, `bridge/bridge_test.go` | -- | `go test ./bridge/...` |
| T2 | unix-coder | Create type generator that reads Go structs and emits TypeScript interfaces | `bridge/bridge.go` | `bridge/types/generator.go`, `bridge/types/generator_test.go`, `bridge/types/generated.d.ts` | T1 | `go test ./bridge/types/...` |
| T3 | unix-coder | Create hook-bridge CLI entry point | `bridge/bridge.go` | `cmd/hook-bridge/main.go` | T1 | `go build ./cmd/hook-bridge/ && ./hook-bridge --list` |
| T4 | unix-coder | Create go-bridge.ts Pi extension | `bridge/bridge.go`, manifest schema | `~/.pi/agent/extensions/go-bridge.ts` | T1 | `pi --no-extensions -e ~/.pi/agent/extensions/go-bridge.ts -p 'test'` |
| T5 | unix-coder | Add Makefile targets for bridge build and install | `Makefile` | `Makefile` | T3 | `make build-bridge` |
| T6 | unix-coder | Create initial manifest.json with cmdr-bridge binding | `bridge/bridge.go`, `~/.claude/hooks/cmdr-bridge.sh` | `~/.claude/bridge/manifest.json` | T1, T3 | `go run ./cmd/hook-bridge/ --validate` |
| T7 | code-review | Review bridge architecture and type safety | all bridge files | `specs/reviews/go-typescript-bridge-review.md` | T1, T2, T3, T4, T5, T6 | `test -f specs/reviews/go-typescript-bridge-review.md` |

## 16. Dependency Graph

```
Phase 1 (sequential): T1
Phase 2 (parallel):   T2, T3, T4 (all depend on T1)
Phase 3 (parallel):   T5, T6 (T5 depends on T3; T6 depends on T1, T3)
Phase 4 (gate):       T7 (review after T1, T2, T3, T4, T5, T6)
```

## 17. Target State

### Files Created
- `bridge/bridge.go` -- Manifest loader, request/response types, dispatcher
- `bridge/bridge_test.go` -- Unit tests for manifest loading and dispatch
- `bridge/types/generator.go` -- TypeScript type generator from Go structs
- `bridge/types/generator_test.go` -- Unit tests for type generation
- `bridge/types/generated.d.ts` -- Generated TypeScript interfaces
- `cmd/hook-bridge/main.go` -- CLI entry point for hook-bridge binary
- `~/.claude/bridge/manifest.json` -- Initial hook manifest
- `~/.pi/agent/extensions/go-bridge.ts` -- Pi bridge extension
- `specs/reviews/go-typescript-bridge-review.md` -- Architecture review output

### Files Modified
- `Makefile` -- Add `build-bridge`, `install-bridge`, `generate-types` targets

### Files Deleted
- None in Phase 1 (bidirectional-sync.ts retained for backward compatibility)

## 18. Verification Plan

### Per-Task Verification
- T1: `go test ./bridge/... -v` -- manifest loading, dispatch, error handling
- T2: `go test ./bridge/types/... -v` -- type generation roundtrip
- T3: `go build ./cmd/hook-bridge/ && ./hook-bridge --list` -- binary builds and runs
- T4: Extension loads without error in Pi
- T5: `make build-bridge` succeeds
- T6: `hook-bridge --validate` passes

### Integration Check
- End-to-end: Pi session starts -> go-bridge.ts loads manifest -> `session_start` event fires -> hook-bridge spawned -> response parsed -> no errors in Pi log

### Rollback
- Delete `bridge/`, `cmd/hook-bridge/`, `~/.claude/bridge/`
- Revert Makefile changes
- `~/.pi/agent/extensions/go-bridge.ts` can be deleted independently

## 19. Success Criteria

- [ ] `go test ./bridge/...` passes with 0 failures
- [ ] `go test ./bridge/types/...` passes with 0 failures
- [ ] `hook-bridge --list` shows at least one registered hook
- [ ] `hook-bridge --validate` exits 0
- [ ] `make build-bridge` produces `hook-bridge` binary
- [ ] `go-bridge.ts` loads in Pi without errors
- [ ] TypeScript type definitions are generated from Go structs
- [ ] End-to-end event dispatch works: Pi event -> Go hook -> JSON response
- [ ] No modifications to pi-mono source code required
- [ ] Manifest schema is documented and validated
