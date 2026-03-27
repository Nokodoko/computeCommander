# LinkedIn Post Generator

Automated LinkedIn content pipeline that generates technically deep, visually compelling posts about AI systems engineering. Draws from the user's actual projects (computeCommander, openbrain, trustgraph, rayne, and the Claude Code hooks/agent system) to produce ByteByteGo-style technical content positioning the user as an AI authority. Runs inside Claude Code sessions on a Max account -- no standalone API keys or external SMTP required.

<!-- Spec Type: feature -->

---

## 1. Why

The user builds sophisticated AI systems daily but has no mechanism to translate that work into professional visibility. LinkedIn is the primary platform for B2B thought leadership. The gap:

- **Work exists** -- Multi-agent orchestration (computeCommander), AI-powered observability (rayne), MCP context routing (openbrain), graph-native knowledge (trustgraph), ~45 Claude Code hooks for intent engineering and delegation enforcement
- **Visibility does not** -- None of this work is being shared in a consumable format
- **Goal** -- Position user as a sought-after AI systems implementer and consultant

ByteByteGo sets the bar: technical architecture diagrams, data-driven insights, system design breakdowns. This generator must match that quality while being grounded in the user's real implementations.

---

## 2. Design Principles

1. **Content from code, not thin air.** Every post must reference real implementations from the user's projects. The generator reads code, specs, and architecture docs to extract insights.
2. **Approval gate before publish.** Nothing goes live without explicit user approval. Email + desktop notification for review.
3. **Go-native.** Implemented in Go as a new `internal/linkedin/` package within computeCommander, consistent with the project's Go-first architecture.
4. **Visual-first.** Posts must include or describe technical diagrams/graphics. The generator produces structured content that can be rendered visually.
5. **Cron-driven, human-gated.** Automated generation on schedule, but always requires manual approval before posting.
6. **Iterative improvement.** Feedback loop where post ratings inform future content direction.

---

## 3. Architecture

```
                    +------------------------------------------+
                    |        LINKEDIN POST GENERATOR            |
                    +------------------------------------------+

  +-------------+     +----------------+     +---------------+
  | PROJECT     |---->| CONTENT        |---->| POST          |
  | SCANNER     |     | GENERATOR      |     | RENDERER      |
  | (code/specs)|     | (Claude Code   |     | (HTML email + |
  +-------------+     |  session)      |     |  text format) |
        |             +----------------+     +-------+-------+
        v                    |                       |
  +-------------+     +----------------+             v
  | TREND       |     | FEEDBACK       |     +---------------+
  | ANALYZER    |     | STORE          |     | DELIVERY      |
  | (RSS feeds) |     | (SQLite)       |     | - Gmail MCP   |
  +-------------+     +----------------+     | - Dunst       |
                                             | - LinkedIn API|
                                             +---------------+
```

### Components

| Component | Responsibility |
|-----------|----------------|
| **Project Scanner** | Reads local project files (Go source, specs, CLAUDE.md, hooks) to extract technical insights, architecture patterns, and data points |
| **Content Generator** | Runs inside a Claude Code session (Max account) to transform raw technical insights into LinkedIn-ready content with ByteByteGo-style framing. No standalone API key needed. |
| **Post Renderer** | Formats the generated content into: (1) HTML email for review, (2) plain text for LinkedIn posting, (3) diagram descriptions for visual assets |
| **Trend Analyzer** | (Phase 1) Pulls trending AI topics from RSS feeds (TechCrunch, The Verge, Hacker News) to incorporate into posts |
| **Feedback Store** | SQLite table tracking post ratings, engagement metrics, and content preferences |
| **Delivery** | Sends review email via Gmail MCP tools (already wired up in Claude Code sessions), triggers dunst notification, and (future) posts to LinkedIn API |

---

## 4. Content Sources (User's Projects)

### Tier 1: Primary Content Mines

| Project | Location | Key AI Angles |
|---------|----------|---------------|
| **computeCommander** | `/home/n0ko/Programs/ai/computeCommander/` | Multi-agent orchestration CLI, TUI dashboard, 5 AI runtime adapters (Claude/Gemini/Codex/Pi/Goose), inter-agent mail system, FIFO merge queue with 4-tier conflict resolution, worktree isolation. Based on overstory fork (see note below). |
| **openbrain** | `/home/n0ko/Programs/ai/openbrain/` | Go MCP server + Python pipeline, PostgreSQL+pgvector, context router for automatic prompt classification and specialist delegation |
| **trustgraph** | Docker Compose stack on primary host (integration plan: `/home/n0ko/openbrain/PLAN-trustgraph-memory-integration.md`) | Graph-native knowledge backend (Cassandra+Qdrant+Pulsar), being integrated with openbrain as knowledge storage and retrieval backend. RDF knowledge graph for structured reasoning, GraphRAG, DocumentRAG, Context Cores. |
| **rayne** | `/home/n0ko/Portfolio/rayne/` | AI-powered root cause analysis at a leading tech company, Claude sidecar pattern, vector DB incident learning (Qdrant + Ollama), auto-generated Datadog notebooks, multi-account observability |
| **Claude Code Hooks** | `~/.claude/hooks/` | ~45 hooks for intent engineering (NLP predicate verification), context injection (automatic prompt classification + routing), enforce-supervisor (delegation protocol), bias detection, context routing |

### Note on overstory

computeCommander is a **fork** of jaymin West's [overstory](https://github.com/jaymin-west/overstory) project. Do NOT claim overstory as original work. References to overstory should drive traffic to jaymin's sites. Content should focus on computeCommander and the extensions built on top of the fork.

### Tier 2: Supporting Angles

- Intent engineering as a software practice (predicate types, bias detection, interrogation)
- Context routing as an architectural pattern (classify prompts -> route to specialists)
- Recall systems (memory files, warm-start markers, per-session state)
- Go-TypeScript bridge layer (cross-runtime interop for AI tools)
- Colony multi-team orchestration (WebSocket hive, queen/supervisor/worker hierarchy)

---

## 5. First Post: Rayne Deep Dive

The inaugural post focuses on rayne for the user's employer to publish. This is a special case with distinct requirements.

### Confidentiality Constraints

- **CONFIDENTIAL**: No MTTR numbers, no specific metric data, no production statistics.
- **Employer attribution**: Refer to the employer as "a leading tech company". Do not name the company.
- **Generic volume references**: Can mention large webhook volume generically (e.g., "high-volume alerting pipeline") but no specific numbers.
- **Architecture is shareable**: The technical patterns (sidecar, vector DB, notebook generation) can be described in full.

### Rayne Content Angles

1. **"What if your monitoring system could think?"** -- Datadog webhooks trigger Claude-powered root cause analysis automatically
2. **The sidecar pattern for AI** -- Claude agent runs as a Kubernetes sidecar alongside the Go API, receiving pre-fetched context from Datadog
3. **Incident memory** -- Qdrant vector DB stores past RCA results; Ollama generates embeddings for similarity search on new incidents
4. **Auto-generated notebooks** -- Every alert produces a Datadog notebook with hyperlinked resources (logs, metrics, hosts)
5. **Multi-account governance** -- Single API gateway managing multiple Datadog orgs

### Rayne Post Format (Employer-Ready)

```
[Visual: Architecture diagram of rayne system -- webhook -> prefetch -> analyze -> notebook]

Title: "We Built an AI That Learns From Every Incident"
or: "How We Automated Root Cause Analysis with Claude and Datadog"

Body Structure:
1. The Problem (alert fatigue, manual investigation, tribal knowledge)
2. The Architecture (sidecar pattern, pre-fetched context)
3. The Learning Loop (vector embeddings, similarity search)
4. The Output (auto-generated notebooks with actionable links)
5. Results / Impact (generic: "dramatically reduced investigation time", no specific MTTR numbers)

CTA: "Interested in implementing AI-powered observability? Reach out."
```

### Delivery Format
- Full post text in email body (cmonty614@gmail.com)
- Suggested diagram descriptions (for the user or a designer to create)
- LinkedIn-ready plain text version
- Employer-appropriate tone (professional, we-focused for team credit)

---

## 6. Recurring Post Topics (After Rayne)

### Topic Queue (Generated from Project Analysis)

| # | Topic | Source Project | Hook |
|---|-------|---------------|------|
| 1 | "I Built 45 Hooks That Think Before My AI Acts" -- Intent engineering pipeline | Claude hooks | Intent gate blocks ambiguous prompts before execution |
| 2 | "My AI Agents Have a Mail System" -- Inter-agent communication | computeCommander | SQLite mail with priorities, thread IDs, read receipts |
| 3 | "The 5-Runtime Problem: Making AI Tools Portable" -- Adapter pattern | computeCommander | Claude, Gemini, Codex, Pi, Goose behind one interface |
| 4 | "Context Routing: How I Classify Every Prompt in 50ms" -- Automatic context injection | openbrain / Claude hooks | Prompt -> classify -> inject context -> delegate to specialist |
| 5 | "AI Agents in Git Worktrees: Isolation Without Docker" -- Concurrent agent work | computeCommander | Each agent gets isolated branch, merge queue resolves conflicts |
| 6 | "Graph-Native Knowledge for AI Reasoning" -- Structured knowledge backends | trustgraph | Cassandra+Qdrant+Pulsar for RDF knowledge graph traversal |
| 7 | "Vector DBs for Incident Memory" -- Learning from failures | rayne | Qdrant + Ollama embeddings for similar incident detection |
| 8 | "The Bias Detector I Built for My AI Pipeline" -- Responsible AI | Claude hooks/intent | Predicate verification prevents harmful or biased outputs |
| 9 | "MCP Servers: Building the Context Layer for AI" -- MCP architecture | openbrain | Go MCP server + Python pipeline, PostgreSQL+pgvector |
| 10 | "Building a Queen: Multi-Team AI Orchestration" -- Colony architecture | colony | WebSocket hive, supervisor hierarchy, janitor for context pruning |

---

## 7. Workflow

```
  CRON (2x/week)
       |
       v
  [1] Scan Projects
       |  - Read recent git log, changed files
       |  - Parse specs, CLAUDE.md, agentic_instructions.md
       |  - Extract architecture patterns, data points
       v
  [2] Select Topic
       |  - Check topic queue (avoid repeats)
       |  - Cross-reference with trending AI themes (RSS feeds)
       |  - Weight by feedback scores from past posts
       v
  [3] Generate Content
       |  - Generate content within Claude Code session using extracted context + style guide
       |  - Produce: headline, body, diagram description, CTA
       |  - Apply ByteByteGo style template
       v
  [4] Render & Deliver
       |  - Format HTML email with post preview
       |  - Send via Gmail MCP tools (gmail_create_draft or direct send) to cmonty614@gmail.com
       |  - Trigger dunst notification: "LinkedIn post ready for review"
       |  - Store draft in SQLite with status="pending_review"
       v
  [5] User Review
       |  - User reads email, approves/rejects/edits
       |  - (Phase 1: manual copy-paste to LinkedIn)
       |  - (Phase 2: approve via CLI -> auto-post via LinkedIn API)
       v
  [6] Feedback Loop
       - After posting, user rates 1-5 on: relevance, engagement, quality
       - Ratings stored in SQLite, influence future topic selection
       - Monthly auto-analysis of what topics/styles performed best
```

---

## 8. Implementation Plan

### Phase 1: Core Generator (MVP)

**New package:** `internal/linkedin/`

```
internal/linkedin/
  generator.go        -- Main orchestrator: scan -> generate -> deliver
  scanner.go          -- Project file scanner (reads Go source, specs, hooks)
  content.go          -- Content generation (runs inside Claude Code session, no API key needed)
  renderer.go         -- HTML email template + plain text formatter
  delivery.go         -- Gmail MCP email delivery + dunst notification trigger
  feedback.go         -- SQLite feedback store (ratings, metrics)
  topics.go           -- Topic queue management, deduplication
  trends.go           -- RSS feed parser for trending topics (TechCrunch, The Verge, Hacker News)
  types.go            -- Post, Topic, Feedback, Config types
```

**New CLI command:** `cc linkedin`

```
cc linkedin generate          -- Run the generator now (outside cron)
cc linkedin preview           -- Show next scheduled post topic
cc linkedin approve <id>      -- Approve a pending post for publishing
cc linkedin reject <id>       -- Reject a pending post
cc linkedin feedback <id> <rating>  -- Rate a past post (1-5)
cc linkedin history           -- List generated posts with ratings
cc linkedin topics            -- Show topic queue
cc linkedin stats             -- Show engagement trends and monthly summary
```

**Cron configuration:** systemd timer triggers a headless Claude Code session via `claude -p`.

The core challenge: content generation and email delivery both require a Claude Code session (for LLM generation and Gmail MCP tools). A bare `cc linkedin generate` binary launched by systemd has no access to these capabilities. The solution is to have systemd launch Claude Code itself.

#### Phase 1 (Recommended): `claude -p` headless session

The systemd timer runs `claude -p` with a prompt that instructs the headless Claude Code session to execute the generation pipeline. The headless session has full access to Claude (Max account) and Gmail MCP tools.

```ini
# ~/.config/systemd/user/cc-linkedin.timer
[Unit]
Description=LinkedIn Post Generator (2x/week)

[Timer]
OnCalendar=Tue,Thu 08:00
Persistent=true

[Install]
WantedBy=timers.target
```

```ini
# ~/.config/systemd/user/cc-linkedin.service
[Unit]
Description=LinkedIn Post Generator

[Service]
Type=oneshot
WorkingDirectory=/home/n0ko/Programs/ai/computeCommander
ExecStart=/usr/local/bin/claude -p "Run cc linkedin generate. Scan projects for content, select a topic, generate a LinkedIn post, send the draft via Gmail MCP to cmonty614@gmail.com, and trigger a dunst notification."
# Headless Claude Code session -- full access to:
#   - Claude LLM (Max account, no API key needed)
#   - Gmail MCP tools (email delivery)
#   - Local filesystem (project scanning)
# The session runs non-interactively and exits when the task completes.
```

This approach is simple, requires no additional infrastructure, and works today. The `claude -p` invocation starts a headless session with the same capabilities as an interactive one -- including MCP tools for Gmail delivery and full LLM access for content generation. The Go binary (`cc linkedin generate`) handles scanning, topic selection, and content structuring; Claude handles the actual generation and email delivery within the session.

#### Phase 2 Enhancement: openbrain async proxy

As an alternative or upgrade path, the systemd timer can write a task to openbrain's capture pipeline instead of launching Claude directly. Openbrain classifies the task and routes it to a Claude Code session for generation.

```ini
# Phase 2 alternative -- systemd writes to openbrain capture pipeline
ExecStart=/usr/local/bin/ob write --type=task --priority=high "Generate LinkedIn post: scan projects, select topic, generate content, deliver via email"
```

Benefits of the openbrain proxy approach:
- **Decoupled scheduling from execution** -- openbrain queues the task and can retry on failure
- **Unified task pipeline** -- LinkedIn generation becomes just another openbrain-routed task
- **Observability** -- task status, timing, and errors tracked in openbrain's PostgreSQL store
- **Fleet-aware** -- openbrain can route to whichever host/session is available

This is recommended as a Phase 2 enhancement once the openbrain integration plan (see `/home/n0ko/openbrain/PLAN-trustgraph-memory-integration.md`) is further along.

### Phase 2: LinkedIn API + openbrain proxy

- LinkedIn API integration for direct posting (requires LinkedIn developer app + OAuth -- deferred, no credentials yet)
- openbrain async proxy for task routing (see Phase 2 Enhancement above)
- Auto-scheduling posts at optimal engagement times
- Engagement metric collection from LinkedIn API

### Phase 3: Visual Asset Generation

- Mermaid/D2 diagram generation from architecture descriptions
- Integration with image generation APIs for custom graphics
- Automated diagram-to-image pipeline for LinkedIn carousel posts

---

## 9. Style Guide (ByteByteGo-Inspired)

### Format Template

```
[Hook Line -- provocative question or surprising statement]

[1-2 sentence problem statement]

[Architecture diagram or visual description]

[3-5 numbered points explaining the system/approach]

[Result or impact statement]

[CTA -- question to audience or invitation to connect]

#AI #SystemDesign #DevOps [relevant hashtags]
```

### Style Rules

1. **Lead with a question or bold claim.** "What if your CI/CD pipeline could think?" not "We implemented AI in our pipeline."
2. **Show architecture, not just prose.** Every post needs a visual element -- even if it's ASCII art in the initial version.
3. **Concrete numbers over vague claims.** "45 hooks fire on every prompt" not "we have lots of automation."
4. **Technical depth, accessible language.** Explain the WHY before the HOW. Decision rationale > implementation details.
5. **Personal voice.** First person singular for personal posts, first person plural for employer posts.
6. **End with engagement.** Ask a question that professionals can answer from their own experience.
7. **2000 character sweet spot.** LinkedIn truncates at ~3000 chars. Aim for 1500-2000 for full visibility.

---

## 10. Email Template

```html
Subject: [LinkedIn Draft] {post_title} -- Ready for Review

<h2>LinkedIn Post Draft</h2>
<p><strong>Topic:</strong> {topic}</p>
<p><strong>Source Project:</strong> {project}</p>
<p><strong>Generated:</strong> {timestamp}</p>
<p><strong>Target:</strong> {personal|employer}</p>

<hr/>

<h3>Post Content</h3>
<div style="background: #f5f5f5; padding: 16px; border-radius: 8px; font-family: sans-serif;">
  {post_content_html}
</div>

<hr/>

<h3>Visual Asset Description</h3>
<p>{diagram_description}</p>
<p><em>Create this diagram using draw.io, Excalidraw, or similar tool.</em></p>

<hr/>

<h3>Plain Text (Copy-Paste to LinkedIn)</h3>
<pre style="background: #1a1a1a; color: #e0e0e0; padding: 16px; border-radius: 8px;">
{post_content_plain}
</pre>

<hr/>

<p>
  <strong>Actions:</strong><br/>
  - Approve: <code>cc linkedin approve {post_id}</code><br/>
  - Reject: <code>cc linkedin reject {post_id}</code><br/>
  - Edit: Reply to this email with changes
</p>
```

---

## 11. Feedback Mechanism

### SQLite Schema

```sql
CREATE TABLE linkedin_posts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    topic       TEXT NOT NULL,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    diagram_desc TEXT,
    source_project TEXT,
    target      TEXT DEFAULT 'personal',  -- 'personal' or 'employer'
    status      TEXT DEFAULT 'draft',     -- draft, pending_review, approved, posted, rejected
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    posted_at   DATETIME,
    feedback_rating INTEGER,              -- 1-5 scale
    feedback_notes TEXT,
    engagement_likes INTEGER DEFAULT 0,
    engagement_comments INTEGER DEFAULT 0,
    engagement_reposts INTEGER DEFAULT 0
);

CREATE TABLE linkedin_topics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    topic       TEXT NOT NULL,
    source_project TEXT,
    priority    INTEGER DEFAULT 5,
    used        BOOLEAN DEFAULT FALSE,
    used_at     DATETIME,
    avg_rating  REAL DEFAULT 0
);
```

### Rating Flow

1. After posting, user runs `cc linkedin feedback <id> <1-5>` with optional notes
2. Rating is stored and used to weight future topic selection
3. Topics from higher-rated source projects get priority
4. Monthly summary: `cc linkedin stats` shows engagement trends

---

## 12. Resolved Questions

All critical and important questions have been answered. Decisions are integrated into the spec above.

| # | Question | Resolution |
|---|----------|------------|
| 1 | **Gmail delivery** | Gmail MCP tools are already wired up in Claude Code sessions. Use `gmail_create_draft` for email delivery. No SMTP, App Passwords, or OAuth needed. |
| 2 | **Anthropic API** | No API keys needed. User is on Max account. The generator runs inside Claude Code sessions, not as a standalone API client. |
| 3 | **Rayne metrics** | CONFIDENTIAL. No MTTR numbers, no specific metric data. Attribute employer as "a leading tech company". Can mention large webhook volume generically. Architecture patterns are shareable. |
| 4 | **LinkedIn API** | No credentials available. Phase 1 is email-only with manual copy-paste posting. LinkedIn API exploration deferred to Phase 2. |
| 5 | **Trending topics** | TechCrunch, The Verge, Hacker News via RSS feeds. |
| 6 | **overstory** | This is a FORK from jaymin West's project. Do NOT claim as original work. Only reference to drive traffic to his sites. computeCommander is based on overstory -- focus content on computeCommander instead. |
| 7 | **Content focus projects** | computeCommander, openbrain, trustgraph, rayne, and the Claude Code hooks/agent system (~45 hooks, intent engineering, context routing, enforce-supervisor delegation). |

### Remaining Open (Nice to Have, non-blocking)

- **Visual asset workflow**: Phase 1 describes diagrams in text. Mermaid/D2 generation deferred to Phase 3.
- **Post scheduling optimization**: Deferred. Phase 1 uses fixed systemd timer (Tue/Thu 08:00).

---

## 13. Dependencies

| Dependency | Purpose | Phase |
|------------|---------|-------|
| Gmail MCP tools (already wired up) | Email delivery via `gmail_create_draft` | 1 |
| Claude Code session (Max account) | Content generation (no API key needed) | 1 |
| `modernc.org/sqlite` (already in go.mod) | Feedback store | 1 |
| `notify-send` (system) | Dunst desktop notification | 1 |
| RSS feed parser (stdlib `encoding/xml`) | Trending topics from TechCrunch, The Verge, HN | 1 |
| LinkedIn API | Direct posting (deferred, no credentials) | 2 |
| Mermaid CLI or D2 | Diagram rendering | 3 |

---

## 14. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Post generation rate | 2/week consistently | systemd timer logs |
| User approval rate | >80% of drafts approved | SQLite status tracking |
| Average feedback rating | >3.5/5 after 10 posts | SQLite feedback table |
| Content grounding | >90% of posts cite specific code/architecture | Manual review |
| LinkedIn engagement | Growing trend over 3 months | Manual tracking (Phase 1), API (Phase 2) |
| Consulting inquiries | At least 1 inbound within 6 months | User tracking |

---

## 15. Non-Goals

- **Not a social media manager.** This generates LinkedIn posts only. No Twitter, no blog.
- **Not fully autonomous.** Every post requires human approval. No auto-posting in Phase 1.
- **Not a design tool.** The generator describes visuals but does not create final graphics (Phase 1).
- **Not real-time.** Cron-driven batch generation, not reactive to events.
