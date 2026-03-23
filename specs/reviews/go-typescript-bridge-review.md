# Go-TypeScript Bridge Layer -- Architecture Review

**Date:** 2026-03-23
**Reviewer:** code-review (automated)
**Scope:** bridge/, bridge/types/, cmd/hook-bridge/, ~/.pi/agent/extensions/go-bridge.ts

## Summary

The bridge layer implements a clean Go-TypeScript translation layer that connects Go-built hooks to Pi's TypeScript extension system via a stdin/stdout JSON protocol. The architecture is sound, with proper separation of concerns across four components.

## Architecture Assessment

### Strengths

1. **Clean protocol design**: BridgeRequest/BridgeResponse as JSON envelopes on stdin/stdout is simple, debuggable, and process-isolated. No shared memory or IPC complexity.

2. **Thread-safe registry**: The Registry uses sync.RWMutex correctly, allowing concurrent reads during dispatch while serializing writes during registration.

3. **Manifest-driven binding**: Hook-to-event mappings are declarative (JSON manifest), not hardcoded. Adding new hooks requires only a manifest entry and a handler registration.

4. **Type generation from source**: Using go/ast to parse structs with bridge:export markers means the TypeScript types stay synchronized with Go types automatically. The generated.d.ts is correct and complete.

5. **Graceful degradation**: The Pi extension handles missing hook-bridge binary and missing manifest without crashing, logging warnings instead.

6. **Error containment**: Dispatch wraps handler errors into BridgeResponse.Error rather than propagating them, preventing a single bad handler from crashing the bridge.

### Issues Found

#### Medium Severity

1. **No stdin size limit in CLI** (cmd/hook-bridge/main.go:123): `io.ReadAll(os.Stdin)` has no size limit. A malformed or adversarial input could exhaust memory. Recommend `io.LimitReader(os.Stdin, maxRequestSize)` with a 1-10MB cap.

2. **No timeout on dispatch** (bridge/bridge.go): The Dispatch function has no context/timeout. If a handler blocks indefinitely, the bridge process hangs. Consider adding `context.Context` to HookHandler and Dispatch signatures.

3. **Pi extension hardcodes binary name** (go-bridge.ts): Uses `which hook-bridge` to find the binary. If installed to a non-PATH location, it silently fails. Consider checking `~/.local/bin/hook-bridge` as a fallback.

#### Low Severity

4. **No manifest file watch** (go-bridge.ts): The manifest is loaded once on extension init. If the manifest is updated, the extension must be reloaded. Acceptable for now but worth noting for future enhancement.

5. **Generated .d.ts not in go:generate directive** (bridge/types/generator.go): The package doc comment references `go:generate` but no actual `//go:generate` directive exists. Consider adding one or relying solely on `make generate-types`.

6. **ValidateManifest reports missing handlers but hooks list is also empty** (cmd/hook-bridge/main.go): `registerBuiltinHooks` is a no-op placeholder. Running `--validate` against the manifest will always fail until handlers are registered. This is expected but should be documented.

## Type Safety

- Go types are correctly annotated with `json` struct tags
- The `bridge:export` marker detection correctly filters standalone directives vs prose mentions (fixed during implementation)
- JSON marshaling uses `json.RawMessage` for payload passthrough, avoiding unnecessary deserialization
- TypeScript type mapping covers all Go primitives, slices, pointers, maps, and json.RawMessage

## Test Coverage

- **bridge/**: 16 tests covering manifest loading (valid, bad version, empty name, missing file, bad JSON), binding lookup (found, not found), registry (register, duplicate, get unregistered, names), dispatch (with handler, no handler, handler error), and validation (all present, some missing).
- **bridge/types/**: 10 tests covering struct parsing (exported, non-exported, pointer, map, json:"-"), TypeScript generation (output format), integration (file-to-file), error cases (no exported structs, bad file).
- Total: 26 tests, all passing, go vet clean.

## Recommendations

1. Add `context.Context` to HookHandler before implementing real handlers
2. Add `io.LimitReader` to the CLI stdin read
3. Add a `//go:generate` directive or document that `make generate-types` is the canonical way to regenerate
4. Consider a `--dry-run` flag for dispatch that validates the request without executing

## Verdict

**PASS** -- The bridge architecture is well-designed for its purpose. The medium-severity items should be addressed before production use but do not block the initial integration.
