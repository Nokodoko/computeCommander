# Article Architecture: "Your AI Has No Memory — Here's How I Fixed That"

## Narrative Flow

### Act 1: The Problem (Hook + Pain Point)
1. **Feed Teaser / Blurb** — "Your AI has no memory." Provocative opening that challenges the assumption that smarter models solve the continuity problem.
2. **The Amnesia Tax** — Quantified: 15-25% of session time re-establishing context. Longer context windows don't solve it because they're session-scoped. JSONL files are better than nothing but lack search, temporal awareness, and real-time access.

### Act 2: The Solution (Architecture Deep-Dive)
3. **OpenBrain: Memory as Infrastructure** — System overview: Go MCP server + PostgreSQL/pgvector + Python embedding sidecar + context router. Fleet-wide shared memory pool. Any MCP-compatible model connects.
4. **The Eight Types of Memory** — Deep-dive into each item type (contact, session, observation, project, task, event, reference, idea) with real-world examples showing why typed memory > flat storage.
5. **The Pipeline: From Capture to Recall** — Six-stage pipeline walkthrough (Capture → Sorter → Bouncer → Router → Linker → Compactor) with technical specifics on each stage.

### Act 3: The Differentiation (Why This, Not That)
6. **Why Not Just Use JSONL Files?** — Direct comparison on four axes: temporal awareness, semantic search, cross-session pattern matching, real-time agent access. Acknowledges JSONL value as fine-tuning training data.
7. **Immediate Recall: The Context Injection System** — The 500ms SessionStart hook that fetches 5 context layers in parallel (Identity, Project, Session, Knowledge, Hybrid).

### Act 4: The Compounding Effect
8. **Pattern Matching Over Extended Periods** — How individual observations accumulate into detectable patterns over weeks/months. Compressor snapshots, TrustGraph entity relationships.
9. **Any Model Can Reference ob1** — MCP tool table. Model agnosticism. Fleet connectivity. Memory survives model switches.

### Act 5: The Future
10. **What's Next** — JSONL export for SLM fine-tuning, TrustGraph deep integration, temporal decay and relevance scoring.
11. **The Real Differentiator** — Thesis restatement: continuity beats intelligence. Memory is infrastructure, not a feature.

## Key Technical Details Referenced

### From Codebase Research
- **Item Types (8)**: task, reference, idea, contact, event, project, observation, session (`src/openbrain/models.py` ItemType enum)
- **Item Sources (7)**: voice, text, email, screenshot, clipper, webhook, mcp (`models.py` ItemSource enum)
- **Pipeline Stages (6)**: Sorter → Bouncer → Router → Linker → Compactor → Nudger
- **MCP Tools**: ob.write, ob.read, ob.query, ob.cleanup, ob.status, ob.notify, ob.session.start, ob.session.end, ob2.search, ob2.gate, TrustGraph tools
- **Embedding**: all-MiniLM-L6-v2, 384 dimensions, local-only
- **Database**: PostgreSQL + pgvector, async via SQLAlchemy 2.x + asyncpg
- **Context Layers (5)**: Identity, Project, Session, Knowledge, Hybrid
- **Router Categories**: contact, project, observation, decision, session, reference, task, uncategorized
- **Compressor**: 6-hour sweep interval, 48-hour min age, groups by project scope
- **Transport tiers**: stdio, SSE-local, SSE-remote, Unix socket
- **Fleet roles**: n0ko-assistant, workhorse, security, site-server, primary-server

## Diagram Architecture

The SVG diagram shows:
1. **Agent Layer** (top) — Multiple agents on multiple hosts connecting via different transports
2. **MCP Server** (center) — Go binary exposing tools, backed by auth layer
3. **Pipeline** (middle) — Capture → Sort → Bounce → Route → Link → Compact flow
4. **Storage Layer** (bottom) — PostgreSQL/pgvector, TrustGraph, embedding sidecar
5. **Context Injection** (right) — SessionStart hook fetching 5 layers in parallel
6. **Item Types** (callout) — Visual legend of the 8 types with routing destinations

## Word Count Target
~2500 words (article body, excluding blurb and hashtags)
