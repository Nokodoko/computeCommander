# SSE Event Relay

Go SSE event relay service that subscribes to multiple upstream OpenBrain MCP server SSE streams, merges them into a unified stream, and re-publishes a single SSE endpoint. Runs on monty:8201, configurable via YAML, deployed as a systemd user service.

## Why

The TrustGraph visualizer (`tg-viz.html` served at `http://monty:3000/static/tg-viz.html`) and future consumers need live SSE events from OpenBrain's `/events/memories` endpoint. Two problems exist today:

- **Single-host blindspot.** The current `tg-viz.html` connects to `localhost:8200/events/memories` -- it only sees events from the OpenBrain MCP server running on the same machine. Lewis (workstation) and monty (server) each run independent MCP servers on port 8200. A visualizer on monty cannot see lewis's events and vice versa.
- **No reconnection resilience.** The browser's native `EventSource` reconnects on drop, but there is no server-side fan-in. If a consumer needs events from N upstreams, it must manage N connections with N retry loops. This relay centralizes that responsibility into a single service.
- **No source attribution.** When events from multiple hosts arrive in one stream, consumers cannot tell which MCP server produced an event. The relay injects `source_host` metadata into each event's data payload, enabling per-host filtering and debugging.

The relay has exactly three jobs: subscribe to upstream SSE streams, tag events with their origin, and re-publish a merged stream. This spec covers that surface.

## Design Principles

1. **Zero-dependency binary.** The relay uses only the Go standard library plus `gopkg.in/yaml.v3` for config parsing. No SSE framework, no web framework, no middleware stack.
2. **Reconnect with backoff.** Every upstream connection retries on failure using exponential backoff with jitter (initial 1s, max 30s, jitter factor 0.3). A dropped upstream never crashes the relay or affects other upstreams.
3. **Pass-through fidelity.** SSE events from upstreams are forwarded byte-for-byte in their `data:` field, with one exception: the relay injects `source_host` into the JSON data payload. Event `id:` and `event:` fields are preserved as-is.
4. **API key passthrough.** The relay forwards the `api_key` query parameter from downstream clients to each upstream connection. Upstream API keys may also be configured per-upstream in the YAML config file.
5. **Systemd-native.** The service runs as a systemd user unit on monty. It logs to stdout/stderr (journald captures), exits cleanly on SIGTERM/SIGINT, and supports `systemctl --user restart sse-relay`.
6. **Configuration over code.** Upstream URLs, listen address, and per-upstream labels are defined in a YAML config file. Adding a new upstream requires only a config change and service restart.

## On-Disk Format

```
~/Programs/sse-relay/
  go.mod                  # Go module definition
  go.sum                  # Dependency checksums (auto-generated)
  main.go                 # Entry point, signal handling, server startup
  config.go               # YAML config loading and validation
  config.yaml             # Runtime configuration (upstream list, listen addr)
  relay.go                # Core relay: upstream subscription, fan-in, broadcast
  relay_test.go           # Tests for relay logic
  health.go               # GET /health endpoint
  health_test.go          # Tests for health endpoint
  Makefile                # Build, install, deploy targets
  deploy/
    sse-relay.service     # systemd user unit file
  README.md               # Operator documentation
```

### config.yaml

YAML configuration read at startup. Path resolved via `--config` flag or `SSE_RELAY_CONFIG` env var, defaulting to `./config.yaml`.

```yaml
listen: ":8201"

upstreams:
  - url: "http://lewis:8200/events/memories"
    label: "lewis"
    api_key: "54c6862ad97a750e8743ab904e5abf14"

  - url: "http://localhost:8200/events/memories"
    label: "monty"
    api_key: "54c6862ad97a750e8743ab904e5abf14"

reconnect:
  initial_interval: "1s"
  max_interval: "30s"
  jitter_factor: 0.3
```

### sse-relay.service

Systemd user unit installed to `~/.config/systemd/user/sse-relay.service`.

```ini
[Unit]
Description=SSE Event Relay - merges upstream OpenBrain MCP SSE streams
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%h/Programs/sse-relay/sse-relay --config %h/Programs/sse-relay/config.yaml
Restart=on-failure
RestartSec=5s
Environment=GOMAXPROCS=2

[Install]
WantedBy=default.target
```

## Data Model

### Config

```typescript
interface Config {
  // Server
  listen: string;           // ":8201" -- net.Listen address

  // Upstreams
  upstreams: Upstream[];    // ordered list of SSE sources

  // Reconnect policy
  reconnect: ReconnectPolicy;
}

interface Upstream {
  url: string;              // "http://lewis:8200/events/memories"
  label: string;            // "lewis" -- injected as source_host
  api_key?: string;         // per-upstream API key, overrides client-provided key
}

interface ReconnectPolicy {
  initial_interval: string; // "1s" -- parsed as time.Duration
  max_interval: string;     // "30s" -- ceiling for backoff
  jitter_factor: number;    // 0.3 -- randomization factor (0.0-1.0)
}
```

### SSE Event (wire format)

```typescript
interface SSEEvent {
  // Standard SSE fields (from upstream, preserved)
  id?: string;              // SSE event ID, forwarded as-is
  event?: string;           // SSE event type, forwarded as-is
  data: string;             // JSON string -- relay parses, injects source_host, re-serializes

  // Injected by relay into JSON data payload
  // source_host: string;   // value from Upstream.label (e.g., "lewis", "monty")
}
```

### Upstream Connection State

```typescript
interface UpstreamState {
  label: string;            // "lewis"
  url: string;              // full upstream URL
  status: "connected" | "reconnecting" | "backoff";
  last_event_at?: string;   // ISO 8601 timestamp of last received event
  reconnect_count: number;  // total reconnects since relay start
  error?: string;           // last error message, cleared on successful reconnect
}
```

### Connection Lifecycle

```
disconnected ──> connecting ──> connected ──> disconnected
      ^               |              |              |
      |               v              v              |
      +──── backoff <─┘              └──────────────┘
              |
              v
         (retry with jitter)
```

### source_host Injection

When the relay receives an SSE event with a JSON `data:` payload, it:

1. Parses the `data:` field as JSON
2. Injects `"source_host": "<label>"` into the top-level object
3. Re-serializes to compact JSON
4. Forwards the modified `data:` line to all downstream clients

If the `data:` field is not valid JSON, the relay forwards it unmodified and appends a comment line: `: source_host=<label>` (SSE comment syntax).

## CLI

Binary name: `sse-relay` (self-describing, no abbreviation needed).

```
sse-relay                                Run the relay server
  --config <path>                        Config file path (default: ./config.yaml, env: SSE_RELAY_CONFIG)
  --listen <addr>                        Override listen address (default: from config)
  --version                              Print version and exit
```

No subcommands. The binary does one thing: run the relay. Version is embedded at build time via `-ldflags`.

## JSON Output Format

Not applicable -- the relay serves SSE streams and a health endpoint, not a JSON CLI. SSE wire format is defined in the Data Model section; the health endpoint returns structured JSON described below.

### Health Endpoint Response

```
GET /health
```

```json
{
  "status": "ok",
  "upstreams": [
    {
      "label": "lewis",
      "url": "http://lewis:8200/events/memories",
      "status": "connected",
      "last_event_at": "2026-04-08T14:23:01Z",
      "reconnect_count": 0
    },
    {
      "label": "monty",
      "url": "http://localhost:8200/events/memories",
      "status": "reconnecting",
      "last_event_at": "2026-04-08T14:22:45Z",
      "reconnect_count": 3,
      "error": "connection refused"
    }
  ]
}
```

Top-level `status` is `"ok"` when at least one upstream is connected, `"degraded"` when some are down, `"down"` when all are disconnected.

### SSE Endpoint

```
GET /events/memories[?api_key=<key>]
```

Returns `Content-Type: text/event-stream`. Each event:

```
event: memory
data: {"type":"semantic","content":"zellij uses KDL config format","captured_at":"2026-04-08T14:23:01Z","source_host":"lewis"}

event: memory
data: {"type":"episodic","content":"user deployed sse-relay to monty","captured_at":"2026-04-08T14:23:05Z","source_host":"monty"}
```

## Concurrency Model

Fan-In Channel Architecture

```
Upstream goroutines:  N (one per upstream in config)
Fan-in channel:       1 buffered chan (capacity 256)
Broadcaster:          1 goroutine reading fan-in, writing to all clients
Client registry:      sync.RWMutex-protected map[chan SSEEvent]struct{}
```

Implementation:

1. At startup, spawn one goroutine per upstream. Each goroutine owns its HTTP connection and reconnect loop.
2. Each upstream goroutine parses incoming SSE events and sends them to a shared `fan-in` channel (`chan SSEEvent`, buffered to 256).
3. A single broadcaster goroutine reads from the fan-in channel and writes each event to every registered client channel.
4. When a downstream client connects to `/events/memories`, the HTTP handler creates a per-client channel (`chan SSEEvent`, buffered to 16), registers it in the client map, and enters a write loop.
5. When the client disconnects (context canceled), the handler removes its channel from the map and closes it.

### Atomic Writes

Not applicable -- the relay is stateless with no on-disk writes during operation. Config is read once at startup.

### Conflict Resolution

**Slow client drop.** If a client channel is full (buffer 16), the broadcaster skips that client for the current event and logs a warning. This prevents one slow consumer from blocking all others. The client will miss events but remains connected.

### Graceful Shutdown

1. SIGTERM/SIGINT triggers `context.Cancel()`
2. HTTP server calls `Shutdown(ctx)` with 5s deadline
3. All upstream goroutines exit their reconnect loops
4. Broadcaster drains the fan-in channel and exits
5. Process exits 0

## Migration

Not applicable in the traditional sense -- this is a fresh build. However, the `tg-viz.html` consumer must be updated to point to the relay.

| Component | Current | Target |
|-----------|---------|--------|
| SSE source URL in tg-viz.html | `http://localhost:8200/events/memories` | `http://monty:8201/events/memories` |
| SSE connections per consumer | 1 (single upstream) | 1 (relay handles fan-in) |
| Source attribution | None | `source_host` field in every event |
| Reconnect responsibility | Browser EventSource | Relay (server-side) + browser EventSource (client-side) |

### tg-viz.html Update

The existing `tg-viz.html` at `.computecommander/scripts/tg-viz.html` currently uses polling to `http://localhost:8088/api/v1/flow/default/service/triples` (TrustGraph gateway). The SSE integration is a separate concern -- the user wants `tg-viz.html` updated to ALSO connect to the relay for live memory events. This is a downstream consumer update in computeCommander, not part of the sse-relay Go module itself.

The update in `tg-viz.html`:

```javascript
// Before (if/when SSE is added to tg-viz.html):
const SSE_URL = 'http://localhost:8200/events/memories?api_key=54c6862ad97a750e8743ab904e5abf14';

// After:
const SSE_URL = 'http://monty:8201/events/memories?api_key=54c6862ad97a750e8743ab904e5abf14';
```

## Integration

### tg-viz.html (TrustGraph Visualizer)

The visualizer connects to the relay's SSE endpoint via the browser's native `EventSource` API.

| tg-viz.html Action | Relay Endpoint |
|--------------------|----------------|
| Live memory feed | `GET /events/memories?api_key=<key>` |
| Check relay health | `GET /health` |

### computeCommander Gateway (future)

The computeCommander OpenBrain proxy (`internal/gateway/openbrain.go`) currently proxies SSE from a single MCP server (`handleStream` -> `MCPSseURL/events/memories`). In the future, it could point to the relay instead of directly to an MCP server:

```yaml
# computecommander config.yaml (future update)
openbrain:
  mcp_sse_url: "http://monty:8201"   # was: http://localhost:8200
```

### Deployment on monty

```bash
# Build and deploy
ssh monty "cd ~/Programs/sse-relay && go build -ldflags '-X main.version=$(git describe --tags --always)' -o sse-relay ."

# Install systemd unit
ssh monty "cp ~/Programs/sse-relay/deploy/sse-relay.service ~/.config/systemd/user/"
ssh monty "systemctl --user daemon-reload"
ssh monty "systemctl --user enable --now sse-relay"

# Verify
ssh monty "curl -s http://localhost:8201/health | jq ."
```

### Hooks Integration

Not applicable -- the relay is a standalone service with no computeCommander hook integration. It runs independently as a systemd user service.

## What It Does NOT Do

Explicitly out of scope:

- **Event persistence.** The relay is stateless. It does not store events in a database, file, or buffer beyond the in-memory fan-in channel. Consumers that disconnect miss events during the gap.
- **Authentication/authorization.** The relay passes through `api_key` query parameters to upstreams but does not validate them itself. It trusts the network (monty's LAN).
- **Event filtering.** The relay forwards all events from all upstreams. Per-source or per-type filtering is the consumer's responsibility.
- **TLS termination.** The relay listens on plain HTTP. TLS is handled by a reverse proxy (nginx, caddy) if needed in the future.
- **Event deduplication.** If the same event arrives from multiple upstreams (unlikely with distinct MCP servers), both copies are forwarded.
- **Upstream discovery.** Upstreams are statically configured in YAML. There is no mDNS, consul, or dynamic registration.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Project default language, matches computeCommander ecosystem |
| Language | Go | Mandatory per project rules -- no Python, no Node |
| Dependencies | `gopkg.in/yaml.v3` only | Single dependency for config parsing; SSE is stdlib `net/http` |
| Storage | None (stateless) | Relay holds no persistent state |
| Testing | `go test` + `net/http/httptest` | Standard Go testing with test SSE servers |
| Formatting | `gofmt` | Go standard formatter |
| Distribution | `go build` -> SCP to monty | Same deploy pattern as n0kos.com (build local or on monty, systemd restart) |

## Project Infrastructure

### Directory Structure

```
sse-relay/
  main.go                               # Entry point, flag parsing, signal handling
  config.go                             # Config struct, YAML loading, validation
  config_test.go                        # Config parsing tests
  relay.go                              # Relay struct: upstream mgmt, fan-in, broadcast
  relay_test.go                         # Relay integration tests with mock SSE servers
  health.go                             # GET /health handler
  health_test.go                        # Health endpoint tests
  config.yaml                           # Default config (committed, monty-specific)
  Makefile                              # build, test, deploy, install targets
  go.mod                                # module github.com/noko/sse-relay
  go.sum                                # dependency checksums
  deploy/
    sse-relay.service                   # systemd user unit
  README.md                             # Operator docs
```

### Version Management

Version is embedded at build time via ldflags:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always)" -o sse-relay .
```

Tags follow semver: `v0.1.0`, `v0.2.0`, etc.

### CHANGELOG.md

Not included for v0.1.0 (initial release). Add after first iteration of production feedback.

### CI Workflow

Not applicable -- this is a personal infrastructure project deployed via SCP. No GitHub Actions or CI pipeline.

### Scripts (Makefile)

```makefile
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.version=$(VERSION)" -o sse-relay .

test:
	go test -v -race ./...

deploy: build
	scp sse-relay monty:~/Programs/sse-relay/sse-relay
	ssh monty "systemctl --user restart sse-relay"

install-service:
	scp deploy/sse-relay.service monty:~/.config/systemd/user/
	ssh monty "systemctl --user daemon-reload && systemctl --user enable sse-relay"

clean:
	rm -f sse-relay
```

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| Core relay (main, relay, config) | 3 | ~350 |
| HTTP handlers (health, SSE endpoint) | 2 | ~120 |
| Tests | 3 | ~250 |
| Config + deployment | 3 | ~50 |
| Documentation | 1 | ~40 |
| **Total** | **12** | **~810** |

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | unix-coder | Initialize Go module and write `go.mod` | -- | `go.mod` | -- | `cd ~/Programs/sse-relay && go mod tidy` |
| T2 | unix-coder | Implement config loading (`config.go`, `config_test.go`) | `go.mod` | `config.go`, `config_test.go` | T1 | `cd ~/Programs/sse-relay && go test -run TestConfig -v ./...` |
| T3 | unix-coder | Implement core relay (`relay.go`, `relay_test.go`) -- upstream subscriptions, fan-in channel, broadcaster, SSE event parsing, source_host injection | `config.go` | `relay.go`, `relay_test.go` | T2 | `cd ~/Programs/sse-relay && go test -run TestRelay -v -race ./...` |
| T4 | unix-coder | Implement health endpoint (`health.go`, `health_test.go`) | `relay.go`, `config.go` | `health.go`, `health_test.go` | T3 | `cd ~/Programs/sse-relay && go test -run TestHealth -v ./...` |
| T5 | unix-coder | Implement main entry point (`main.go`) -- flag parsing, signal handling, server wiring | `config.go`, `relay.go`, `health.go` | `main.go` | T3, T4 | `cd ~/Programs/sse-relay && go build -o sse-relay . && ./sse-relay --version` |
| T6 | unix-coder | Write config.yaml, Makefile, deploy/sse-relay.service, README.md | -- | `config.yaml`, `Makefile`, `deploy/sse-relay.service`, `README.md` | T1 | `cd ~/Programs/sse-relay && make build` |
| T7 | unix-coder | Update tg-viz.html SSE URL to point to monty:8201 relay | `.computecommander/scripts/tg-viz.html` | `.computecommander/scripts/tg-viz.html` | -- | `rg -q 'monty:8201' .computecommander/scripts/tg-viz.html` |
| T8 | code-review | Review all sse-relay source files for correctness, error handling, goroutine leaks | `~/Programs/sse-relay/*.go` | -- | T5 | `cd ~/Programs/sse-relay && go vet ./...` |
| T9 | security-review | Review API key handling, no credential logging, safe header forwarding | `~/Programs/sse-relay/*.go`, `config.yaml` | -- | T5 | `cd ~/Programs/sse-relay && rg -c 'api_key\|APIKey\|Authorization' *.go` |
| T10 | unix-coder | Deploy to monty: build, SCP, install systemd unit, start service | `Makefile`, `deploy/sse-relay.service` | -- | T8, T9 | `ssh monty 'curl -sf http://localhost:8201/health \| jq -e .status'` |

## 16. Dependency Graph

```
Phase 1 (parallel): [T1, T7]
  T1: Initialize Go module
  T7: Update tg-viz.html SSE URL

Phase 2 (after T1): [T2]
  T2: Config loading implementation

Phase 3 (after T2): [T3]
  T3: Core relay implementation

Phase 4 (parallel, after T3): [T4, T5, T6]
  T4: Health endpoint
  T5: Main entry point (also depends on T4)
  T6: Config, Makefile, systemd, README

Phase 5 (after T5): [T5]
  T5: Main entry point (blocked by T3 + T4)

Phase 6 (parallel, after T5): [T8, T9]
  T8: Code review
  T9: Security review

Final: [T10] -- deploy to monty
```

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `~/Programs/sse-relay/go.mod` | ~8 | No |
| `~/Programs/sse-relay/main.go` | ~80 | No |
| `~/Programs/sse-relay/config.go` | ~90 | No |
| `~/Programs/sse-relay/config_test.go` | ~70 | No |
| `~/Programs/sse-relay/relay.go` | ~200 | No |
| `~/Programs/sse-relay/relay_test.go` | ~150 | No |
| `~/Programs/sse-relay/health.go` | ~60 | No |
| `~/Programs/sse-relay/health_test.go` | ~50 | No |
| `~/Programs/sse-relay/config.yaml` | ~18 | No |
| `~/Programs/sse-relay/Makefile` | ~20 | No |
| `~/Programs/sse-relay/deploy/sse-relay.service` | ~14 | No |
| `~/Programs/sse-relay/README.md` | ~40 | No |

Files modified:

| File Path | Change |
|-----------|--------|
| `.computecommander/scripts/tg-viz.html` | SSE URL changed from `localhost:8200` to `monty:8201` |

Files deleted: None

## 18. Verification Plan

**Per-task checks:** (from Task Manifest Verify Command column)

- T1: `cd ~/Programs/sse-relay && go mod tidy`
- T2: `cd ~/Programs/sse-relay && go test -run TestConfig -v ./...`
- T3: `cd ~/Programs/sse-relay && go test -run TestRelay -v -race ./...`
- T4: `cd ~/Programs/sse-relay && go test -run TestHealth -v ./...`
- T5: `cd ~/Programs/sse-relay && go build -o sse-relay . && ./sse-relay --version`
- T6: `cd ~/Programs/sse-relay && make build`
- T7: `rg -q 'monty:8201' .computecommander/scripts/tg-viz.html`
- T8: `cd ~/Programs/sse-relay && go vet ./...`
- T9: `cd ~/Programs/sse-relay && rg -c 'api_key|APIKey|Authorization' *.go`
- T10: `ssh monty 'curl -sf http://localhost:8201/health | jq -e .status'`

**Integration check:**

```bash
# Start relay on monty, then verify end-to-end SSE flow
ssh monty 'systemctl --user status sse-relay | rg -q Active:.running'
ssh monty 'curl -sf http://localhost:8201/health | jq -e ".status"'
# Open SSE connection for 5s and verify at least headers arrive
ssh monty 'timeout 5 curl -sN http://localhost:8201/events/memories 2>&1 | head -1 | rg -q "event-stream" || true'
```

**Rollback:**

```bash
# If deployment fails, stop the service and revert tg-viz.html
ssh monty 'systemctl --user stop sse-relay'
git checkout .computecommander/scripts/tg-viz.html
```

## 19. Success Criteria (Machine-Verifiable)

- [ ] `cd ~/Programs/sse-relay && go build -o sse-relay .` exits 0
- [ ] `cd ~/Programs/sse-relay && go test -v -race ./...` exits 0
- [ ] `cd ~/Programs/sse-relay && go vet ./...` exits 0
- [ ] `cd ~/Programs/sse-relay && ./sse-relay --version` outputs a version string
- [ ] `test -f ~/Programs/sse-relay/config.yaml` exits 0
- [ ] `test -f ~/Programs/sse-relay/deploy/sse-relay.service` exits 0
- [ ] `rg -q 'monty:8201' .computecommander/scripts/tg-viz.html` exits 0
- [ ] `ssh monty 'systemctl --user is-active sse-relay'` outputs "active"
- [ ] `ssh monty 'curl -sf http://localhost:8201/health | jq -e .status'` outputs "ok" or "degraded"
- [ ] `cd ~/Programs/sse-relay && rg -q 'source_host' relay.go` exits 0 -- source tagging is implemented
- [ ] `cd ~/Programs/sse-relay && rg -q 'exponential\|backoff\|jitter' relay.go` exits 0 -- reconnect with backoff is implemented

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Go module init, config, relay, health, main, deployment files | `unix-coder` | All implementation work -- Go service from scratch |
| tg-viz.html URL update | `unix-coder` | Single-line file edit in computeCommander |
| Code quality review | `code-review` | Goroutine leak analysis, error handling patterns, SSE edge cases |
| Security review | `security-review` | API key handling, header injection, log sanitization |
| Deployment to monty | `unix-coder` | Build + SCP + systemd, same pattern as n0kos.com |

## Execution Order

```
Phase 1: Scaffolding
  +-- T1: Init Go module (agent: unix-coder)
  +-- T7: Update tg-viz.html SSE URL (agent: unix-coder)  [parallel]

Phase 2: Config [blocked by Phase 1]
  +-- T2: Config loading (agent: unix-coder)

Phase 3: Core [blocked by Phase 2]
  +-- T3: Relay implementation (agent: unix-coder)

Phase 4: Endpoints + Infra [blocked by Phase 3]
  +-- T4: Health endpoint (agent: unix-coder)
  +-- T6: Config, Makefile, systemd, README (agent: unix-coder)  [parallel]

Phase 5: Entry Point [blocked by Phase 4]
  +-- T5: main.go (agent: unix-coder)

Phase 6: Review [blocked by Phase 5]
  +-- T8: Code review (agent: code-review)
  +-- T9: Security review (agent: security-review)  [parallel]

Phase 7: Deploy [blocked by Phase 6]
  +-- T10: Deploy to monty (agent: unix-coder)
```

Recommended directive: `/pai` -- linear plan-then-implement pipeline. The service is small (~810 LOC) with sequential dependencies; parallel fan-out adds coordination overhead without meaningful speedup.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| All upstreams unreachable | `/health` returns `{"status": "down"}` | Relay keeps retrying with backoff; check upstream MCP servers on lewis/monty |
| Upstream returns non-SSE response (HTML error page) | Log line: `unexpected content-type` | Relay closes connection and retries with backoff |
| Fan-in channel full (256 buffer) | Should not happen under normal load; log warning if send blocks >1s | Increase buffer or add upstream throttling |
| Client channel full (slow consumer) | Log warning: `dropping event for slow client` | Client misses events but stays connected; browser EventSource will show gaps |
| Config file missing or invalid YAML | Startup fails with descriptive error, exits 1 | Fix config.yaml and restart |
| API key rejected by upstream | Upstream returns 401/403 | Relay logs the status code and retries (upstream may recover); fix key in config.yaml |
| Relay process OOM on monty | systemd SIGKILL, `journalctl --user -u sse-relay` shows OOM | Set `MemoryMax=128M` in service unit; investigate client leak |
| Binary not found after deploy | `systemctl --user status sse-relay` shows exec failure | Re-run `make deploy` |

## SSE Protocol Reference

### SSE Wire Format (RFC 8895 / W3C EventSource)

The relay must parse and produce standards-compliant SSE streams:

```
event: <event-type>\n
id: <event-id>\n
data: <payload>\n
\n
```

Rules:
- Each field is `<field>:<space><value>\n`
- Events are delimited by a blank line (`\n\n`)
- Lines starting with `:` are comments (ignored by EventSource, used for keepalive)
- The `data:` field may span multiple lines; each line starts with `data:`
- The relay should send a comment keepalive (`: keepalive\n\n`) every 30s to prevent proxy/LB timeouts

### Reconnect Backoff Algorithm

```
interval = min(initial_interval * 2^attempt, max_interval)
jitter = interval * jitter_factor * random(0, 1)
sleep = interval + jitter
```

Example with default config (initial=1s, max=30s, jitter=0.3):

| Attempt | Base Interval | With Jitter (range) |
|---------|--------------|---------------------|
| 0 | 1s | 1.0s - 1.3s |
| 1 | 2s | 2.0s - 2.6s |
| 2 | 4s | 4.0s - 5.2s |
| 3 | 8s | 8.0s - 10.4s |
| 4 | 16s | 16.0s - 20.8s |
| 5+ | 30s | 30.0s - 39.0s |

On successful reconnect, attempt counter resets to 0.
