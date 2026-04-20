# LinkedIn Pipeline Refresh — Remaining Tracks (2-6)

This file enumerates the engineering tracks scoped in the 2026-04-20
pipeline-refresh plan. Plan A (systemd unit PATH fix and Mon..Fri 06:00
cadence on `lewis`) and Track 1 (file-based style guide in
`~/linkedin_context_one_shot.md`) are complete. The tracks below remain
and each should be executed as an independent commit on its own worker
pass.

All tracks respect the canonical style constants in
`~/linkedin_context_one_shot.md`. Do not re-introduce in-code style
constants.

---

## Track 2 — split post and article (two distinct artifacts)

**Goal.** Every topic produces both:

1. `posts/NNN-slug-article.md` — long-form, 1500-2500 words.
2. `posts/NNN-slug-post.md` — short LinkedIn announcement,
   200-400 chars, links to / teases the article.

**Files to change.**
- `internal/linkedin/types.go`: extend `Post` with `ArticleContent
  string` and `AnnouncementContent string`, or split into `Article`
  and `Announcement` types sharing a `PostMeta`. Prefer the latter
  for clarity; update `api.go` DB schema accordingly (new column or
  new table).
- `internal/linkedin/content.go`: add `BuildArticlePrompt()` and
  `BuildAnnouncementPrompt()`. The announcement prompt receives the
  article body as input and asks for the tease/link.
- `internal/linkedin/generator.go`: `Generate()` runs two sequential
  `claude -p` calls — article first, then announcement. Both are
  persisted; failure of the announcement pass does not void the
  article.
- `internal/linkedin/api.go` (schema): add columns or new
  `linkedin_announcements` table. Migration must be backward
  compatible (existing rows keep `Content` populated with the
  article).
- `internal/linkedin/delivery.go`: email body includes both artifacts
  side-by-side — announcement on top (for immediate paste),
  article below (for review or inline publish).

**Driver change.** `~/.local/bin/linkedin-post-gen` (on lewis) must
rsync the `posts/*-article.md` files to
`~/Portfolio/linkedin/articles/` and the `*-post.md` files to a new
`~/Portfolio/linkedin/posts/` directory (create if missing).

**Output contract.** See section 6 of
`~/linkedin_context_one_shot.md`.

---

## Track 3 — `prompts/` directory producer

**Goal.** Make the daily prompt a reviewable, cacheable artifact.

**Files to change.**
- `internal/commands/linkedin.go`: add
  `linkedinPreparePromptCmd(app)` — new subcommand
  `cmdr linkedin prepare-prompt`. It runs steps 1-5 of `Generate()`
  (seed topics → select → scan → trends → build prompt) and writes
  the assembled prompt to
  `~/Portfolio/linkedin/prompts/YYYY-MM-DD.md`. Flag `--out <path>`
  for override.
- `internal/linkedin/generator.go`: extract the prepare-prompt path
  into a reusable `PreparePrompt(date time.Time) (path string, err
  error)` method so `Generate()` can call it internally too.
- `~/.local/bin/linkedin-post-gen` (driver on lewis): change the
  pipeline to:
  1. `cmdr linkedin prepare-prompt` (writes
     `~/Portfolio/linkedin/prompts/YYYY-MM-DD.md`)
  2. `cmdr linkedin generate --prompt-file
     ~/Portfolio/linkedin/prompts/YYYY-MM-DD.md`
  3. `(existing Gmail draft + dunst pipeline)`
- `linkedinGenerateCmd`: new `--prompt-file <path>` flag. When set,
  skip the prompt-assembly step and read the file instead.

**Benefits.** (a) The daily prompt is reviewable after the fact.
(b) A failed generate can be re-run with the same prompt (no topic
re-selection, no trend re-fetch). (c) The prompt is diffable over
time to track prompt drift.

---

## Track 4 — structured image pipeline (headline + architecture)

**Goal.** Replace ad-hoc Claude-draws-SVG with a template-driven
pipeline that enforces the dark-bg canonical theme at compile time.

**Files to create.**
- `internal/linkedin/templates/headline.svg.tmpl` — Go `text/template`.
  Hardcodes `bgGrad` stops `#0a0a0f` and `#111127`. No parameter can
  override the bg. Accepts `.Title`, `.Subtitle`, `.Nodes`, `.Edges`,
  `.Footer`, `.Series`.
- `internal/linkedin/templates/architecture.svg.tmpl` — same contract,
  wider viewBox `0 0 1400 1000`, supports more nodes/lanes.
- `internal/linkedin/nodegraph.go` — types:
  ```go
  type Node struct{ ID, Label, Lane, Color string; X, Y int }
  type Edge struct{ From, To, Color, Label string }
  type NodeGraph struct{ Nodes []Node; Edges []Edge }
  ```
- `internal/linkedin/imagegen.go` — the orchestration:
  1. `ExtractGraph(title, diagramDesc string) (*NodeGraph, error)` —
     calls `claude -p` with a prompt that constrains output to the
     canonical palette (teal/green/red/gold/blue/purple from
     `linkedin_context_one_shot.md` §8.4) and the node/edge JSON
     schema above.
  2. `RenderSVG(template string, g *NodeGraph) ([]byte, error)` —
     executes the Go template.
  3. `Rasterize(svg []byte) ([]byte, error)` — resvg-go preferred;
     fallback to `rsvg-convert` if the system binary is present.
  4. `Validate(svg []byte) error` — parses the SVG and asserts:
     - `bgGrad` stops are exactly `#0a0a0f` and `#111127`.
     - No fill or stroke uses `#ffffff` outside the 0.04-opacity
       grid pattern.
     - All node `rect` fills are `#1a1a2e`.
     - Every `marker-end` URL resolves to a declared marker.
     - Every `url(#xxx)` fill resolves to a declared gradient.
- `internal/linkedin/renderer.go` — extend with
  `RenderHeadline(*Post) (svg, png []byte, err error)` and
  `RenderArchitecture(*Post) (svg, png []byte, err error)`.

**Rasterizer dependency.**
Add `github.com/kanrichan/resvg-go` to `go.mod`. It is pure Go, no
CGO, no system libs. Only fall back to `rsvg-convert` if the user
explicitly sets `LINKEDIN_RASTERIZER=rsvg-convert` in the env.

**Claude prompt for graph extraction.** Living in
`internal/linkedin/content.go` alongside `assemblePrompt`. Include
the full palette list and an example node-graph JSON. The LLM must
return JSON only, no prose.

**Validation failure handling.** On any validation failure, the job
logs the first offending element, writes the bad SVG to
`/tmp/linkedin-invalid-<postid>-<kind>.svg` for debugging, and
returns an error from `Generate()`. The driver notifies via dunst
and does NOT send the email.

---

## Track 5 — `cmdr linkedin mark-published` (n0kos.com sidecar)

**Goal.** A manual post-publish command that updates the n0kos.com
articles index. NOT part of the 06:00 daily job.

**Files to change.**
- `internal/commands/linkedin.go`: new
  `linkedinMarkPublishedCmd(app)`. Usage:
  `cmdr linkedin mark-published <post-id> <linkedin-url>`.
- `internal/linkedin/api.go`: add
  `PostStore.MarkPublished(id int64, url string) error` — sets
  status to `StatusPosted`, stores the URL, bumps `PostedAt`.
- `~/Portfolio/servers/n0kos/articles.json` (new sidecar file):
  append an entry `{id, title, slug, summary, url, published_at}`.
  If the file does not exist, create it with `[]`.
- Optional deeper integration: if
  `~/Portfolio/servers/n0kos/templates/index.templ` exists, update
  it and run `templ generate`. Gate this behind
  `--rebuild-template`.

**Commit discipline.** The n0kos commit is STAGED but not pushed.
The human reviews and pushes manually. Add a printed hint:
`"Staged commit in servers/n0kos — review with 'git -C
~/Portfolio/servers/n0kos log -1' and push when ready."`

**Documentation.** Extend the help text on `linkedin` command.

---

## Track 6 — security review

**Goal.** Before declaring the pipeline done, run a focused security
review over the final state (with Tracks 1-5 merged).

**Delegate to.** `security-review` agent.

**Scope.**
- `~/.local/bin/linkedin-post-gen` on lewis — temp file creation,
  cleanup, path sanitization, secret handling (Gmail MCP OAuth token
  location, any API keys).
- `internal/linkedin/delivery.go` — email body construction
  (HTML-escaped? safe against injection if DIAGRAM text from Claude
  contains markup?).
- `internal/linkedin/imagegen.go` (from Track 4) — node labels are
  influenced by generated content; must be SVG-escaped before going
  into the template. Path traversal on the SVG/PNG output filename
  (the slug is derived from Claude output).
- `internal/linkedin/api.go` — SQL construction (should be
  parameterized already; verify).
- `internal/commands/linkedin.go` mark-published — the URL is
  user-supplied; verify it cannot inject into the n0kos
  `articles.json` in a way that breaks the renderer.

**Findings output.** One markdown report in the commit message of
the final track; any CRITICAL findings fixed before the review is
closed.

---

## Dependency order (for future worker dispatch)

```
Track 1 (done) ──┬── Track 2 (post/article split)
                 ├── Track 3 (prompts/ dir)
                 ├── Track 4 (image pipeline)
                 └── Track 5 (n0kos mark-published)
                       │
                       ▼
                   Track 6 (security review, all merged)
```

Tracks 2-5 touch different files and can parallelize. Track 6 is
gated on all of them.

---

## Non-goals captured here so they don't resurface

- No light-bg image option. Ever. See `linkedin_context_one_shot.md`
  §8.2 for why.
- No auto-push to n0kos.com. The human reviews the staged commit.
- No Gmail-send via Go SMTP — existing Gmail MCP via `claude -p` is
  the only send path.
- No `a.o`-produced prompts. Prompts are assembled in Go from the
  style guide file + topic + scan + trends. If a user re-raises
  `a.o`, revisit.

---

Last updated: 2026-04-20. Author: supervisor session following the
six-question audit + execution plan.
