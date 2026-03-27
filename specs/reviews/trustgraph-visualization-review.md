# Spec Review Report

| Field | Value |
|-------|-------|
| Spec | `specs/trustgraph-visualization.md` |
| Reviewed | 2026-03-26T08:55:00Z |
| Iteration | 1 of 3 |
| Verdict | **PASS** (0 critical, 3 warnings, 2 info) |

## Summary

The spec is thorough, well-structured, and directly implementable. The three user decisions (auto-refresh on prompt execution, shared Go module, local dev gateway with API key auth) have been cleanly incorporated. The phased approach is practical, the data model is correct (matches the existing openbrain TrustGraph client wire format), and integration points are precisely mapped to existing code patterns (OpenBrain pane as reference implementation). No critical issues. Three warnings around minor gaps that should be addressed before Phase 2+ but do not block Phase 1.

## Dimension Results

| Dimension | Status | Findings | Critical |
|-----------|--------|----------|----------|
| completeness | WARN | 2 | 0 |
| clarity | PASS | 1 | 0 |
| correctness | PASS | 1 | 0 |
| consistency | PASS | 1 | 0 |
| sdlc | PASS | 0 | 0 |
| actionability | PASS | 0 | 0 |

## Findings

---

### Dimension 1: Completeness

### COMP-001 [WARNING] -- PaneID numbering mismatch with overlay pattern

**Section:** Integration Points > 2. PaneID
**Issue:** The spec says `PaneTrustGraph PaneID = 9` (after PaneLazyGit=8) with focus key `8`. However, PaneJira already uses PaneID=6 in the code but focus key `9`, and PaneLazyGit uses PaneID=8 with focus key `0`. The Decisions section correctly says "full-screen overlay accessed via key 8", which is the unassigned key. But the PaneID `9` in the code example could confuse implementers. The actual iota value matters less than the FocusKey assignment.
**Suggestion:** Clarify that PaneTrustGraph gets the next iota value (9) and FocusKey "8". The focus key is what matters for user interaction. This is already implicitly correct but making it explicit avoids implementation errors.

### COMP-002 [WARNING] -- Missing test strategy for Phase 1

**Section:** Implementation Phases > Phase 1
**Issue:** Phase 1 deliverables do not mention unit tests. The existing client has a comprehensive test file (`client_test.go` with 7 test functions). The TrustGraphPane should also have basic tests (View rendering, Refresh error handling, SetSize). The OpenBrain pane has no tests, so there is no test pattern to follow in-tree.
**Suggestion:** Add to Phase 1: "Unit tests for TrustGraphPane: View() renders correctly in each status state, Refresh() handles connection errors gracefully, SetSize() propagates dimensions." Bootstrap from the existing client_test.go patterns.

### COMP-003 [INFO] -- No mention of config defaults in DefaultConfig()

**Section:** Integration Points > 1. Config
**Issue:** The spec shows the TrustGraphConfig struct but does not specify what defaults should be set in `DefaultConfig()`. Should TrustGraph be enabled by default? The OpenBrain config defaults to `Enabled: true`. For consistency, TrustGraph should probably default to `Enabled: false` since it requires a running TrustGraph gateway, unlike OpenBrain which is a local MCP server.
**Suggestion:** Add default values: `Enabled: false, GatewayURL: "http://localhost:8088", MaxNodes: 100, MaxTriples: 200, RefreshSecs: 5`.

---

### Dimension 2: Clarity

### CLAR-001 [WARNING] -- Dashboard.View() integration for full-screen overlay

**Section:** Integration Points > 3. Dashboard
**Issue:** The spec says "render in bottom row (replace or add alongside existing panes)" but the Decisions section says "full-screen overlay accessed via key 8, same pattern as Jira pane." These are contradictory. The Jira pane uses `viewJira()` which renders a full-screen overlay replacing the entire grid. The TG pane should follow the same pattern with a `viewTrustGraph()` method.
**Suggestion:** Update section 3 to read: "View(): render as full-screen overlay when focused (same pattern as Jira pane's viewJira()), not in the bottom row grid." This eliminates the bottom-row layout concern entirely.

---

### Dimension 3: Correctness

### CORR-001 [INFO] -- Refresh mechanism clarification

**Section:** Refresh Strategy
**Issue:** The spec says "auto-refresh on prompt execution via signalRefreshMsg / fsnotify-on-DB-write." The fsnotify mechanism watches the SQLite DB file, and the signalRefreshMsg triggers `Dashboard.Refresh()` which calls all pane Refresh() methods. This means the TG pane will refresh whenever ANY DB write occurs (agent state change, event log, etc.), not specifically when a prompt executes. This is actually better (more responsive) but the wording could be more precise.
**Suggestion:** Minor wording update: "Refreshes are triggered by the dashboard's existing signalRefreshMsg mechanism, which fires on any database write (agent state changes, event logs, etc.). This naturally covers prompt execution events."

---

### Dimension 4: Consistency

### CONS-001 [PASS] -- Pattern alignment with OpenBrain pane

**Observation:** The spec correctly follows the OpenBrain pane pattern: `sync.Mutex` for concurrent access, `SetSize(w, h int)`, `Refresh() error`, `View() string`, status enum (connected/disconnected/error), graceful degradation on unavailability. The Jira overlay pattern is also correctly referenced for full-screen rendering.

---

## Recommendation

**Proceed to Phase 1 implementation.** All warnings are non-blocking for Phase 1. Address COMP-002 (tests) during implementation and CLAR-001 (full-screen overlay vs bottom row) in the implementation itself (use the Jira overlay pattern). COMP-001 and CORR-001 are informational clarifications that the implementer should be aware of.
