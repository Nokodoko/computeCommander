# TrustGraph Visualization Pane

Terminal-native graph visualization for TrustGraph knowledge graph data within computeCommander.

## Why

computeCommander integrates with OpenBrain for memory entries but has no visibility into the underlying knowledge graph. TrustGraph stores RDF triples (subject-predicate-object relationships) that form the structural backbone of agent knowledge -- entities, their relationships, and trust-scored connections. Without visualization, users cannot:

1. **See what agents know.** Graph structure is invisible. A user has no way to understand which entities exist, how they relate, or whether knowledge is sparse or dense for a given topic.
2. **Debug graph-rag queries.** When `graph-rag` returns unexpected results, users cannot inspect the subgraph it traversed. The TG pane provides that introspection.
3. **Monitor graph growth.** As agents write knowledge, the graph grows. Users need a live view of entity counts, edge density, and recent additions.

## Design Principles

1. **Terminal-native.** No browser, no JavaScript. All rendering uses Unicode box-drawing, lipgloss styling, and bubbletea components.
2. **Read-only.** The TG pane only reads from TrustGraph. It never writes triples or mutates graph state.
3. **Graceful degradation.** When TrustGraph is unavailable, the pane shows "disconnected" status -- same pattern as OpenBrain pane.
4. **Lazy loading.** Graph data is fetched on-demand (when pane is focused or on refresh tick), not continuously streamed.

## Data Model

TrustGraph uses RDF triples. The wire format from the REST gateway:

```
Term:   { t: "i"|"l"|"b", i?: IRI, v?: value, dt?: datatype, d?: blankID }
Triple: { s: Term, p: Term, o: Term }
```

For visualization, we derive:

```
Node:     unique IRI or blank-node ID extracted from Subject/Object positions
Edge:     predicate IRI connecting two nodes
NodeMeta: { id, label (short IRI), degree (edge count), lastSeen }
```

## Architecture

### Component Hierarchy

```
Config (TrustGraphConfig)
  |
  v
TrustGraph Client (internal/trustgraph/)
  |   - Shared Go module (bootstrap: vendored copy, then extracted to shared pkg)
  |   - HTTP client with health probe
  |
  v
TrustGraphPane (internal/tui/trustgraph_pane.go)
  |   - Bubbletea sub-view
  |   - Manages view modes, scrolling, search
  |
  v
GraphRenderer (internal/tui/graph_renderer.go)
      - ASCII/Unicode graph layout engine
      - Converts nodes+edges to terminal-renderable grid
```

### View Modes

The TG pane supports three view modes, cycled with `m`:

#### 1. Summary View (default)

Compact overview shown when pane is not focused or screen space is limited:

```
 TG  connected  47 nodes  132 edges
 ──────────────────────────────────
 Top Entities (by degree):
  computeCommander    18 edges
  bubbletea           12 edges
  zellij               9 edges
  TrustGraph           7 edges
  OpenBrain            5 edges

 Recent (last 5 triples):
  computeCommander --uses--> bubbletea
  TrustGraph --provides--> graph-rag
  zellij --renders--> dashboard
 ──────────────────────────────────
 Status: connected | 47 nodes | 2s ago
```

#### 2. Graph View

ASCII adjacency visualization for a selected entity (ego-graph):

```
 TG  Graph: computeCommander
 ──────────────────────────────────────────
                  [zellij]
                    ^
                    | renders
                    |
  [bubbletea] <--uses-- [computeCommander] --integrates--> [TrustGraph]
                    |
                    | contains
                    v
                [dashboard]
                    |
                    | displays
                    v
               [OpenBrain]
 ──────────────────────────────────────────
 j/k:nav  Enter:expand  h:back  /:search
```

The graph renderer uses a force-directed or tree layout adapted for terminal grid:
- Center node is the selected entity
- First-hop neighbors arranged in cardinal directions
- Edge labels on connection lines
- Unicode box-drawing for connections: arrows with `-->`, `<--`, `^`, `v`

#### 3. Triples View

Raw triple listing with search and filter:

```
 TG  Triples (132 total, showing 1-20)
 ──────────────────────────────────────────
 Subject              Predicate        Object
 ──────────────────────────────────────────
 computeCommander     uses             bubbletea
 computeCommander     uses             lipgloss
 computeCommander     integrates       TrustGraph
 computeCommander     integrates       OpenBrain
 computeCommander     renders          dashboard
 bubbletea            provides         tea.Model
 zellij               renders          dashboard
 ...
 ──────────────────────────────────────────
 /:search  j/k:scroll  Enter:inspect  m:mode
```

### Data Flow

```
TrustGraph REST Gateway (localhost:8088)
        |
        | POST /api/v1/triples  (SPO pattern query)
        | POST /api/v1/graph-embeddings (vector search)
        |
        v
TrustGraphClient (internal/trustgraph/client.go)
        |
        | TriplesQuery(), GraphEmbeddingsQuery()
        |
        v
TrustGraphPane.Refresh()
        |
        | Builds node/edge maps from triples
        |
        v
GraphRenderer.Render(nodes, edges, focusNode, width, height)
        |
        | Produces []string lines for terminal
        |
        v
RenderPane() in dashboard.View()
```

### Refresh Strategy

- **Prompt-execution-triggered:** Refresh automatically when the agent executes a prompt (via the existing `signalRefreshMsg` / fsnotify-on-DB-write mechanism). This keeps the TG pane in sync with graph mutations caused by agent activity without manual intervention.
- **On-focus:** When user navigates to TG pane, trigger an immediate refresh.
- **Lazy:** In summary mode, fetch only aggregate stats (count query with limit=0 + top entities). In graph/triples mode, fetch the active query.
- **Cache:** Keep last-fetched triples in memory. Only re-query if >5s since last fetch or user explicitly refreshes (`r` key).
- **No manual trigger required.** The pane refreshes as a side-effect of prompt execution and dashboard tick, not via a dedicated "refresh now" button.

## Integration Points

### 1. Config (`internal/config/config.go`)

Add `TrustGraphConfig` to the `Config` struct:

```go
type TrustGraphConfig struct {
    Enabled     bool   `yaml:"enabled"`
    GatewayURL  string `yaml:"gateway_url"`      // default: http://localhost:8088
    Token       string `yaml:"token"`             // API key for gateway auth (supports ${TG_TOKEN})
    MaxNodes    int    `yaml:"max_nodes"`          // max nodes to display (default: 100)
    MaxTriples  int    `yaml:"max_triples"`        // max triples per query (default: 200)
    RefreshSecs int    `yaml:"refresh_secs"`        // override refresh interval (default: 5)
}
```

### 2. PaneID (`internal/tui/pane.go`)

Add new pane constant:

```go
const (
    ...
    PaneTrustGraph  PaneID = 9  // after PaneLazyGit (8)
)
```

Assign focus key `8` (currently unassigned per code comments).

Update `AllPanes()`, `paneOrder`, and keyboard handler.

### 3. Dashboard (`internal/tui/dashboard.go`)

Add `trustGraph *TrustGraphPane` field to Dashboard struct.

Wire into:
- `NewDashboard()`: construct with config
- `Refresh()`: call `trustGraph.Refresh()`
- `View()`: render in bottom row (replace or add alongside existing panes)
- `handleFocusedPaneKey()`: add j/k/Enter/h/m/r/slash handlers
- `updatePaneSizes()`: call `trustGraph.SetSize()`

### 4. Zellij Layout (`internal/zellij/layout.go`)

Add a `TG` pane to the bottom row in `GenerateLayout()`:

```
pane name="TG" size="20%%" {
    command "%s"
    args "trustgraph" "--pane"
}
```

This requires a new `cmdr trustgraph` subcommand that runs the pane in standalone mode (same pattern as `cmdr feed --pane`, `cmdr status --pane`).

### 5. Gateway (`internal/gateway/`)

Add `TrustGraphProxy` that forwards requests to the TrustGraph REST gateway. This allows the cmdr gateway to be the single entry point for local development:

```
GET  /api/v1/trustgraph/stats     -> aggregate node/edge counts
POST /api/v1/trustgraph/triples   -> proxy to TG /api/v1/triples
POST /api/v1/trustgraph/search    -> proxy to TG /api/v1/graph-embeddings
```

**Authentication:** The local dev gateway uses API key authentication. The TrustGraph client sends a `Bearer` token in the `Authorization` header. The token is configured via `TrustGraphConfig.Token` (supports `${TG_TOKEN}` env var expansion). The gateway validates this token against the `GATEWAY_SECRET` environment variable on the TrustGraph side. No OAuth or complex auth flows -- a single shared secret is sufficient for local development.

### 6. TrustGraph Client

**Decision: Shared Go module.**

The TrustGraph client will be extracted from `openbrain/mcp/internal/trustgraph/` into a standalone, importable Go module (e.g., `github.com/noko/trustgraph-go` or a `pkg/trustgraph` directory within openbrain that is *not* under `internal/`). Both computeCommander and openbrain will import from this shared module.

**Migration plan:**
1. **Phase 1 (MVP):** Copy `types.go` and `client.go` into `internal/trustgraph/` within computeCommander as a temporary bootstrap. This unblocks TG pane development immediately.
2. **Post-MVP:** Extract the client into a shared module (`github.com/noko/trustgraph-go`), publish it, and replace both computeCommander's `internal/trustgraph/` and openbrain's `internal/trustgraph/` with imports from the shared module.

This avoids the Go `internal` import restriction while establishing a single source of truth for the TrustGraph wire protocol types.

## Graph Rendering Engine

### Terminal Constraints

- No pixel-level control. Rendering is character-grid based.
- Width varies (typically 20-80 chars for a bottom-row pane).
- Height varies (typically 8-25 rows).
- Colors via ANSI/lipgloss (256-color or truecolor depending on terminal).

### Layout Algorithm

For the Graph View ego-graph, use a **radial tree layout**:

1. Place the focus node at the center of the available grid.
2. Sort neighbors by edge count (highest-degree neighbors get cardinal positions: N, E, S, W).
3. Remaining neighbors fill diagonal positions (NE, SE, SW, NW).
4. If >8 first-hop neighbors, collapse low-degree neighbors into a `+N more` indicator.
5. Draw edges using Unicode box-drawing characters:
   - Horizontal: `---`
   - Vertical: `|`
   - Arrows: `-->`, `<--`, `^`, `v`
   - Labels inline on edges: `--uses-->`

### Node Rendering

```
[entity-name]     -- normal node (lipgloss.Color "#5DADE2")
[*focus-node*]    -- selected/focus node (bold, lipgloss.Color "#50fa7b")
[+5 more]         -- collapsed overflow (dim)
```

Node labels are truncated to 16 chars with `..` suffix if needed.

### Color Scheme

| Element | Color | Hex |
|---------|-------|-----|
| Focus node | Green bold | #50fa7b |
| IRI nodes | Blue | #5DADE2 |
| Literal nodes | Yellow | #FFB347 |
| Predicates/edges | Dim white | #888888 |
| Status connected | Green | #50fa7b |
| Status disconnected | Dim | #555555 |
| Status error | Red | #FF5555 |

## User Interaction

### Keybindings (when TG pane is focused)

| Key | Action |
|-----|--------|
| `j` / `down` | Scroll down / next node |
| `k` / `up` | Scroll up / previous node |
| `Enter` | Expand selected node (switch to Graph View centered on it) |
| `h` / `left` | Go back to previous focus node (breadcrumb navigation) |
| `m` | Cycle view mode: Summary -> Graph -> Triples -> Summary |
| `r` | Force refresh |
| `/` | Open search filter (type entity name to filter) |
| `Esc` | Close search / return to Summary |
| `Tab` | Navigate away from TG pane (dashboard-level) |

### Search

When `/` is pressed, a search input appears at the bottom of the pane. As the user types, the node list or triple list is filtered to matching entries. Enter selects the top match and switches to Graph View. Esc cancels search.

Search queries the TrustGraph API:
1. First, try `TriplesQuery` with the search term as a subject IRI pattern.
2. If no results, fall back to `Embeddings` + `GraphEmbeddingsQuery` for semantic search.

## Performance

### Constraints

- TrustGraph gateway may be on localhost or remote. Assume 50-200ms latency per request.
- Graphs may have 10-10,000+ nodes. The pane must not attempt to render all nodes.

### Mitigations

1. **Pagination.** All triple queries use `Limit` (default 200). UI paginates with j/k scrolling.
2. **Ego-graph scope.** Graph View only fetches 1-hop neighborhood of the focus node (typically 5-30 triples).
3. **Aggregate caching.** Summary View caches node/edge counts. Only refreshes every `RefreshSecs` (default 5).
4. **Background fetch.** Refresh runs in a goroutine; the pane renders stale data until the fetch completes, then swaps atomically (same `sync.Mutex` pattern as OpenBrain pane).
5. **Graceful timeout.** HTTP client timeout of 5s. On timeout, pane shows last-known data with "stale" indicator.

## Implementation Phases

### Phase 1: Foundation (MVP)

- Add `TrustGraphConfig` to config
- Bootstrap TrustGraph client (`types.go`, `client.go`) into `internal/trustgraph/` (vendored copy, to be extracted to shared module post-MVP)
- Create `TrustGraphPane` with Summary View only (full-screen overlay, key `8`)
- Add `PaneTrustGraph` to pane system
- Wire into Dashboard (constructor, refresh, view, keybinds)
- Auto-refresh on prompt execution via `signalRefreshMsg` / fsnotify
- API key auth via `TrustGraphConfig.Token` (supports `${TG_TOKEN}`)
- Status: connected/disconnected/error display

Deliverables: TG pane visible in dashboard as full-screen overlay, shows node/edge counts and top entities, auto-refreshes on agent activity.

### Phase 2: Triples View

- Implement Triples View with scrollable table
- Add j/k scrolling, pagination
- Add search (`/`) with IRI pattern matching
- Add `cmdr trustgraph --pane` subcommand for zellij standalone mode

Deliverables: Users can browse and search triples.

### Phase 3: Graph View

- Implement `GraphRenderer` with radial tree layout
- Implement ego-graph expansion (Enter on node)
- Implement breadcrumb navigation (h to go back)
- Node selection with j/k in graph mode

Deliverables: Visual graph exploration in terminal.

### Phase 4: Gateway Proxy + Polish

- Add `TrustGraphProxy` to gateway
- Add semantic search fallback (embeddings-based)
- Add `m` mode cycling
- Performance tuning (caching, lazy loading)
- Edge case handling (empty graph, single node, disconnected components)

Deliverables: Full feature set with production polish.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/trustgraph/types.go` | TrustGraph types (bootstrap copy; extract to shared module post-MVP) |
| `internal/trustgraph/client.go` | TrustGraph HTTP client (bootstrap copy; extract to shared module post-MVP) |
| `internal/tui/trustgraph_pane.go` | TrustGraphPane bubbletea sub-view |
| `internal/tui/graph_renderer.go` | ASCII graph layout engine |
| `internal/tui/pane.go` | Updated: add PaneTrustGraph |
| `internal/tui/dashboard.go` | Updated: wire TG pane |
| `internal/config/config.go` | Updated: add TrustGraphConfig |
| `internal/zellij/layout.go` | Updated: add TG pane to KDL |
| `internal/gateway/trustgraph.go` | TrustGraph gateway proxy |
| `internal/commands/trustgraph.go` | `cmdr trustgraph` subcommand |

## Decisions

1. **Refresh trigger:** Auto-refresh on prompt execution. The TG pane refreshes via the existing `signalRefreshMsg` / fsnotify-on-DB-write mechanism whenever an agent executes a prompt. No manual refresh button needed.

2. **Client strategy:** Shared Go module. Bootstrap with a vendored copy in `internal/trustgraph/` for Phase 1, then extract to `github.com/noko/trustgraph-go` post-MVP so both computeCommander and openbrain import from a single source.

3. **Gateway auth:** Local dev gateway with API key authentication. The TrustGraph client sends `Bearer ${TG_TOKEN}` via the `Authorization` header. Simple shared-secret model for local development.

4. **Bottom row layout:** Full-screen overlay accessed via key `8`, since graph visualization benefits from maximum screen space. Same pattern as the Jira pane (key `9`).

5. **Graph-rag integration:** Deferred to post-Phase 4. The TG pane is read-only for now.
