# Crush UI Refactor

Refactor computeCommander's TUI dashboard to match Charm's Crush design system. Replace all hardcoded hex colors with charmtone semantic tokens, swap hand-rolled widgets for Bubbles v2 components, add gradient rendering, animated spinners, responsive breakpoints, and a branded wordmark header. Go 1.25, BubbleTea/Lipgloss/Bubbles v2, charmtone, ultraviolet, go-colorful.

Replaces the current 28/100 scored TUI (garish RGB primaries, hand-rolled everything, zero animation) with a Crush-grade terminal interface scoring >= 70/100 on the five-axis Charm UX rubric.

## 1. Why

The computeCommander TUI is functionally complete but visually primitive. A comprehensive Charm UX review scored it 28/100 across five axes:

- **Garish color palette (4/20 Color).** Every color is a raw RGB primary -- `#00FF00`, `#FF0000`, `#00FFFF`, `#FFFFFF`. These are 1990s IRC colors, not a curated design system. The evals pane independently uses Dracula palette colors (`#50fa7b`, `#bd93f9`) that clash with both the TUI theme and commands styles.
- **Duplicated color definitions (4/20 Color).** `internal/tui/theme.go` and `internal/commands/styles.go` define identical hex values independently. When one changes, the other drifts. There is no shared abstraction.
- **No visual hierarchy (6/30 Hierarchy).** No landing page, no wordmark, bracket-prefix pane titles (`[1] File Picker`), flat status bar (`"Agents: 3 | Mail: 2 | Merge: 1 | Cost: $0.42"`), no three-tier text system. Agent names, timestamps, and data values all render at the same visual weight.
- **Zero animation (5/15 Interaction).** No spinners, no pulsing indicators, no cycling effects. Agent "working" state is a static green string. Loading states are static text. Focus transitions are binary color swaps with no interpolation.
- **Every widget hand-rolled (5/15 Components).** CommandPalette, AgentTable, EventsPane, EvalsPane, JiraPane all implement their own cursor navigation, filtering, and rendering from scratch instead of using Bubbles v2 `list`, `table`, `viewport`, `textinput`.

The dashboard has ~9 interactive panes and ~20 styled files. This spec covers exactly that surface -- the entire TUI visual layer -- without touching business logic, data models, or CLI command semantics.

## 2. Design Principles

1. **Charmtone everywhere.** Every color in the codebase comes from `charmtone` semantic tokens or a HCL gradient between two charmtone endpoints. Zero hardcoded hex strings. One shared palette package imported by both `internal/tui` and `internal/commands`.
2. **Bubbles v2 components over hand-rolled widgets.** If Bubbles v2 has a component that does what a hand-rolled widget does, replace it. `table.Model` for AgentTable, `list.Model` for CommandPalette/EvalsPane/JiraPane, `viewport.Model` for EventsPane, `textinput.Model` for palette input, `spinner.Model` for loading states.
3. **Three-tier text hierarchy.** `FgBase` (charmtone.Ash) for primary data (agent names, branch names), `FgMuted` (charmtone.Squid) for secondary data (descriptions, types), `FgSubtle` (charmtone.Oyster) for tertiary data (timestamps, placeholders, empty states). Never render all text at the same weight.
4. **Gradient accents via go-colorful HCL blending.** Focused pane title fills use `charmtone.Dolly` to `charmtone.Charple` gradients. Wordmark uses the same gradient. Queue depth indicators use gradient triangles. No hard color edges on accent elements.
5. **Section-header pane titles, not bracket prefixes.** Pane titles render as `bold_title fill_chars` where focused panes get `● title ╱╱╱╱` gradient fill and blurred panes get `  title ───` muted fill. No `[1] Title` bracket syntax.
6. **Border vocabulary matches pane type.** PTY panes (agent session, lazygit) get thick left-border accent with no box border. Data panes get thin left-border. Agent session (center main) is borderless -- it is the primary content area.
7. **Responsive layout with breakpoints.** Wide (>=160 cols): full grid. Normal (>=120 cols): standard grid. Compact (>=80 cols): collapsed sidebar, tab-switcher for right panel. Minimal (<80 cols): single-pane with toggle overlay.
8. **Animation for liveness.** Working agents get hex spinners. Loading states get ellipsis cycling. Merge queue gets animated progress. The UI must visually communicate that things are happening.
9. **No business logic changes.** This refactor touches only rendering, styling, and component wiring. Data models, CLI semantics, database queries, and hook integrations remain untouched.

## 3. On-Disk Format

```
internal/
  ui/
    palette/
      palette.go          # Shared charmtone color constants + gradient helpers
      palette_test.go     # Palette rendering tests
  tui/
    theme.go              # MODIFIED: Theme struct uses palette.go, no hardcoded hex
    pane.go               # MODIFIED: Section-header titles, border vocabulary
    render.go             # MODIFIED: Status bar, help bar, truncation, table rendering
    dashboard.go          # MODIFIED: Responsive breakpoints, spinner commands, header bar
    agent_table.go        # MODIFIED: Replaced with bubbles/table, status icons, spinner
    command_palette.go    # MODIFIED: Replaced with bubbles/list + textinput, gradient title
    events_pane.go        # MODIFIED: Replaced with bubbles/viewport, status icons
    evals_pane.go         # MODIFIED: Replaced with bubbles/list, kill Dracula palette
    jira_pane.go          # MODIFIED: Replaced with bubbles/list grouped items
    merge_view.go         # MODIFIED: Gradient triangle indicators, status icons
    mail_summary.go       # MODIFIED: Status icons, charmtone colors
    floating.go           # MODIFIED: Consistent border vocab, gradient title
    cost_tracker.go       # MODIFIED: Charmtone colors
    header.go             # NEW: ASCII wordmark with gradient diagonal fill
  commands/
    styles.go             # MODIFIED: Imports from ui/palette, kills all hardcoded hex
    status_bar.go         # MODIFIED: Structured layout, charmtone colors
    prompt_line.go        # MODIFIED: Charmtone colors, gradient accent
```

### internal/ui/palette/palette.go

Single source of truth for all colors. Both `internal/tui` and `internal/commands` import from here.

```go
package palette

import (
    "strings"

    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/x/exp/charmtone"
    "github.com/lucasb-eyer/go-colorful"
)

// Semantic color tokens -- the ONLY place colors are defined.
var (
    // Accent
    Primary   = charmtone.Charple  // #AD58B4 -- focus borders, gradient endpoints
    Secondary = charmtone.Dolly    // #FFE156 -- cursor, gradient start
    Tertiary  = charmtone.Bok      // #FF6F61 -- prompt indicators

    // Backgrounds
    BgBase    = charmtone.Pepper   // #1A1A1A -- main background
    BgSubtle  = charmtone.Charcoal // #2B2B2B -- panels, code blocks
    BgOverlay = charmtone.Iron     // #3B3B3B -- dialogs, overlays

    // Foregrounds
    FgBase    = charmtone.Ash      // #DDDBD7 -- primary text
    FgMuted   = charmtone.Squid    // #6C6C6C -- secondary text
    FgSubtle  = charmtone.Oyster   // #9A9A9A -- hints, placeholders

    // Semantic states
    Error   = charmtone.Sriracha   // #ED567A -- errors
    Warning = charmtone.Zest       // #FFBA52 -- warnings
    Info    = charmtone.Malibu     // #7DC1FF -- information
    Success = charmtone.Julep      // #6CCB5F -- success, completed
    InProg  = charmtone.Guac       // #4A9E3F -- in-progress, working
)

// ForegroundGrad returns a slice of individually-colored characters
// blended along the HCL color space from colorA to colorB.
func ForegroundGrad(text string, colorA, colorB lipgloss.Color) []string {
    runes := []rune(text)
    if len(runes) == 0 {
        return nil
    }
    c1, _ := colorful.Hex(string(colorA))
    c2, _ := colorful.Hex(string(colorB))
    result := make([]string, len(runes))
    for i, r := range runes {
        t := float64(i) / float64(max(len(runes)-1, 1))
        blended := c1.BlendHcl(c2, t)
        hex := blended.Hex()
        result[i] = lipgloss.NewStyle().
            Foreground(lipgloss.Color(hex)).
            Render(string(r))
    }
    return result
}

// GradientFill returns a string of `char` repeated `width` times with
// HCL gradient coloring from colorA to colorB.
func GradientFill(char string, width int, colorA, colorB lipgloss.Color) string {
    fill := strings.Repeat(char, width)
    chars := ForegroundGrad(fill, colorA, colorB)
    return strings.Join(chars, "")
}
```

### internal/tui/header.go

Compact ASCII wordmark rendered as a persistent header bar above the dashboard grid.

```go
package tui

import (
    "strings"

    "github.com/charmbracelet/lipgloss"
    "github.com/noko/computecommander/internal/ui/palette"
)

// renderWordmark produces a gradient "CMDR" wordmark with diagonal fill flanking.
func renderWordmark(width int) string {
    word := "CMDR"
    chars := palette.ForegroundGrad(word, palette.Secondary, palette.Primary)
    wordRendered := strings.Join(chars, "")
    wordWidth := lipgloss.Width(wordRendered)
    remaining := width - wordWidth
    if remaining < 2 {
        return wordRendered
    }
    leftW := remaining / 2
    rightW := remaining - leftW
    leftFill := palette.GradientFill("╱", leftW, palette.Primary, palette.BgOverlay)
    rightFill := palette.GradientFill("╱", rightW, palette.BgOverlay, palette.Primary)
    return leftFill + wordRendered + rightFill
}

// renderHeader renders the full header bar: wordmark + project context.
func renderHeader(projectName string, width int) string {
    if width < 20 {
        return ""
    }
    wordmark := renderWordmark(width)
    if projectName != "" {
        // Overlay project name right-aligned in muted text
        ctx := lipgloss.NewStyle().Foreground(palette.FgMuted).Render(" " + projectName + " ")
        ctxW := lipgloss.Width(ctx)
        if ctxW < width-10 {
            // Replace rightmost characters of wordmark
            wmRunes := []rune(wordmark)
            if len(wmRunes) > ctxW {
                wordmark = string(wmRunes[:len(wmRunes)-ctxW]) + ctx
            }
        }
    }
    return wordmark
}
```

## 4. Data Model

No data model changes -- this is a pure visual refactor. All data models (AgentSession, MailMessage, MergeQueueEntry, EvalResult, JiraIssue) remain unchanged. The refactor only changes how existing data is rendered. The following visual mapping tables define the transformation.

### Color Migration Map

This table is the authoritative mapping from current hex values to charmtone tokens. Every occurrence of the left-column hex must be replaced with the right-column token.

| Current Usage | Current Hex | Charmtone Token | Charmtone Hex | Files Affected |
|---------------|-------------|-----------------|---------------|----------------|
| green (working, merged, staged) | `#00FF00` | `palette.Success` (charmtone.Julep) | `#6CCB5F` | theme.go, styles.go |
| yellow (stalled, pending, unstaged) | `#FFFF00` | `palette.Warning` (charmtone.Zest) | `#FFBA52` | theme.go, styles.go |
| red (zombie, failed, error, untracked) | `#FF0000` | `palette.Error` (charmtone.Sriracha) | `#ED567A` | theme.go, styles.go |
| cyan (booting, merging, info, focused) | `#00FFFF` | `palette.Info` (charmtone.Malibu) | `#7DC1FF` | theme.go, styles.go |
| white (text, headers) | `#FFFFFF` | `palette.FgBase` (charmtone.Ash) | `#DDDBD7` | theme.go, styles.go |
| gray (muted, completed, help) | `#808080` | `palette.FgMuted` (charmtone.Squid) | `#6C6C6C` | theme.go, styles.go |
| magenta (conflict, branch) | `#FF00FF` | `palette.Primary` (charmtone.Charple) | `#AD58B4` | theme.go, styles.go |
| dimGray (unfocused borders) | `#555555` | `palette.BgOverlay` (charmtone.Iron) | `#3B3B3B` | theme.go |
| blue (files, accent) | `#5588FF` | `palette.Info` (charmtone.Malibu) | `#7DC1FF` | theme.go, styles.go |
| statusbar bg | `#333333` | `palette.BgSubtle` (charmtone.Charcoal) | `#2B2B2B` | theme.go, styles.go |
| cursor bg | `#333355` | `palette.BgOverlay` (charmtone.Iron) | `#3B3B3B` | theme.go |
| floating border | `#7C3AED` | `palette.Primary` (charmtone.Charple) | `#AD58B4` | floating.go |
| Dracula green | `#50fa7b` | `palette.Success` (charmtone.Julep) | `#6CCB5F` | evals_pane.go, styles.go |
| Dracula red | `#ff5555` | `palette.Error` (charmtone.Sriracha) | `#ED567A` | evals_pane.go, styles.go |
| Dracula purple | `#bd93f9` | `palette.Primary` (charmtone.Charple) | `#AD58B4` | evals_pane.go, styles.go |
| Dracula cyan | `#8be9fd` | `palette.Info` (charmtone.Malibu) | `#7DC1FF` | evals_pane.go, styles.go |
| Dracula yellow | `#f1fa8c` | `palette.Warning` (charmtone.Zest) | `#FFBA52` | evals_pane.go, styles.go |
| Dracula teal | `#66d9ef` | `palette.Info` (charmtone.Malibu) | `#7DC1FF` | evals_pane.go, styles.go |
| Dracula comment | `#6272a4` | `palette.FgMuted` (charmtone.Squid) | `#6C6C6C` | evals_pane.go, styles.go |
| gold (completed) | `#FFD700` | `palette.Secondary` (charmtone.Dolly) | `#FFE156` | theme.go |
| agent palette coral | `#FF6B6B` | `palette.Error` | `#ED567A` | theme.go |
| agent palette teal | `#4ECDC4` | `palette.Info` | `#7DC1FF` | theme.go |
| agent palette amber | `#FFB347` | `palette.Warning` | `#FFBA52` | theme.go |
| agent palette violet | `#9B59B6` | `palette.Primary` | `#AD58B4` | theme.go |

### Component Replacement Map

| Current Component | Current File | Replacement | Bubbles v2 Import | Key Benefits |
|-------------------|-------------|-------------|-------------------|--------------|
| `AgentTable` (manual rows + cursor) | `agent_table.go` | `bubbles/table.Model` | `github.com/charmbracelet/bubbles/v2/table` | Built-in sort, scroll indicators, column resize, help keys |
| `CommandPalette` (manual list + filter) | `command_palette.go` | `bubbles/list.Model` + `bubbles/textinput.Model` | `github.com/charmbracelet/bubbles/v2/list`, `github.com/charmbracelet/bubbles/v2/textinput` | Fuzzy filtering, pagination, category delegates, help |
| `EventsPane` (manual scroll) | `events_pane.go` | `bubbles/viewport.Model` | `github.com/charmbracelet/bubbles/v2/viewport` | Scroll indicators, page up/down, mouse scroll, performance |
| `EvalsPane` (manual cursor list) | `evals_pane.go` | `bubbles/list.Model` with status delegate | `github.com/charmbracelet/bubbles/v2/list` | Filtering, status rendering, pagination |
| `JiraPane` (manual tree + cursor) | `jira_pane.go` | `bubbles/list.Model` with grouped items | `github.com/charmbracelet/bubbles/v2/list` | Tree-like groups, filtering, keyboard nav |
| `renderTable` (manual concat) | `render.go` | `bubbles/table.Model` or styled custom | `github.com/charmbracelet/bubbles/v2/table` | Alternating rows, column alignment, header styling |
| Palette text input (`query` string) | `command_palette.go` | `bubbles/textinput.Model` | `github.com/charmbracelet/bubbles/v2/textinput` | Cursor, placeholder, blink, validation |
| Loading states (static text) | multiple | `bubbles/spinner.Model` | `github.com/charmbracelet/bubbles/v2/spinner` | Hex spinner animation, gradient coloring |

### Status Icon Map

| State | Icon | Color Token | Rendered |
|-------|------|-------------|----------|
| working | `●` | `palette.InProg` | `● working` |
| booting | `⋯` | `palette.Info` | `⋯ booting` |
| stalled | `◇` | `palette.Warning` | `◇ stalled` |
| zombie | `×` | `palette.Error` | `× zombie` |
| completed | `✓` | `palette.FgMuted` | `✓ done` |
| pending (merge) | `○` | `palette.Warning` | `○ pending` |
| merging | `→` | `palette.Info` | `→ merging` |
| merged | `✓` | `palette.Success` | `✓ merged` |
| conflict | `×` | `palette.Primary` | `× conflict` |
| failed | `×` | `palette.Error` | `× failed` |

## 5. CLI

Not applicable -- this refactor does not add, remove, or modify any CLI commands. The `cmdr dashboard` command continues to launch the TUI. The `cmdr status --pane` command continues to render styled ANSI output. Only the visual appearance of these outputs changes.

## 6. JSON Output Format

Not applicable -- this refactor does not modify any JSON output. All `--json` flag behavior remains unchanged.

## 7. Concurrency Model

Not applicable -- the TUI runs in a single-threaded BubbleTea event loop. Spinner and animation `tea.Cmd` messages are dispatched through the existing BubbleTea message passing system. No new concurrency concerns are introduced.

## 8. Migration

This is an in-place refactor, not a data migration. The migration is purely visual.

| Component | Current State | Target State |
|-----------|--------------|--------------|
| Color definitions | Hardcoded hex in `theme.go` + `styles.go` (duplicated) | Single `internal/ui/palette/palette.go` with charmtone tokens |
| Pane titles | `[1] File Picker` bracket prefix | `● File Picker ╱╱╱╱` section-header with gradient fill |
| Pane borders | `RoundedBorder()` everywhere | Thick left-accent for PTY, thin left for data, borderless for agent session |
| Agent table | Hand-rolled `AgentTable` struct | `bubbles/table.Model` with custom cell renderer |
| Command palette | Hand-rolled list + string query | `bubbles/list.Model` + `bubbles/textinput.Model` |
| Events pane | Hand-rolled scroll | `bubbles/viewport.Model` |
| Evals pane | Hand-rolled cursor + Dracula colors | `bubbles/list.Model` + charmtone colors |
| Jira pane | Hand-rolled tree + cursor | `bubbles/list.Model` with grouped items |
| Status bar | Flat concatenated string | Structured layout: left project+branch, center agent count+spinner, right keybind hints |
| Help bar | Plain gray text | `key action │ key action` format, bold keys, muted actions |
| Truncation | `".."` suffix | `"…"` (Unicode ellipsis) |
| Loading states | Static text (`"No events"`) | `spinner.Model` hex animation |
| Status labels | Plain colored text | Icon prefix + abbreviated label |
| Header | None | ASCII wordmark `CMDR` with gradient diagonal fill |
| Layout | Fixed 70/30 + 15/65/20 percentages | Responsive breakpoints at 160/120/80 cols |
| Floating panes | `DoubleBorder()` + `#7C3AED` | `RoundedBorder()` + `charmtone.Charple` + gradient title |

No data migration script is needed. The refactor is additive -- old rendering code is replaced in-place.

## 9. Integration

### Zellij Dashboard

The `--pane` mode commands (`cmdr status --pane`, `cmdr evals --pane`, etc.) render styled ANSI output consumed by zellij panes. These commands import from `internal/commands/styles.go` which will be updated to use the shared palette. The rendered output changes visually but the zellij integration mechanism (pane command strings in KDL layout) is unchanged.

| Current Method | Change |
|----------------|--------|
| `printAgentsPane()` in `status.go` | Colors come from `palette` package instead of local `paneState*` vars |
| `stateStyle()` in `status.go` | Returns icon + charmtone color instead of just ANSI color |
| `renderEvalsPane()` in evals commands | Colors from `palette` instead of Dracula hex |

### Claude Code Hooks

The `cmdr-bridge.sh` hook writes to `local.db`. The TUI reads from `local.db` and renders. This refactor changes only the rendering step. Hook integration is unaffected.

### Agent-Facing Commands

```bash
# Agent wrapper script -- unchanged
cmdr status --pane             # renders with new charmtone colors
cmdr evals --pane              # renders with new charmtone colors (kills Dracula)
```

## 10. What It Does NOT Do

- **Business logic.** No changes to agent spawning, mail delivery, merge queue processing, eval execution, or any data flow.
- **Data models.** No schema changes, no new tables, no field additions.
- **CLI commands.** No new commands, no removed commands, no changed flags.
- **Zellij KDL layout.** `GenerateLayout()` in `internal/zellij/layout.go` is untouched. Pane geometry is managed by zellij, not the TUI.
- **Hook scripts.** `cmdr-bridge.sh` and other hooks are unchanged.
- **Database queries.** All SQL queries remain identical.
- **External API calls.** Jira, GitHub, Linear integrations are unchanged.
- **Test infrastructure.** Existing tests continue to pass. New tests added only for the palette package and component rendering.

## 11. Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Existing project runtime |
| TUI Framework | `github.com/charmbracelet/bubbletea/v2` | Upgrade from v1 for Bubbles v2 compat |
| Styling | `github.com/charmbracelet/lipgloss/v2` | Upgrade from v1 for charmtone integration |
| Components | `github.com/charmbracelet/bubbles/v2` | Pre-built table, list, viewport, textinput, spinner |
| Semantic Colors | `github.com/charmbracelet/x/exp/charmtone` | Crush design system palette |
| Layout Engine | `github.com/charmbracelet/ultraviolet` (optional) | Screen-space responsive layout. May not be available as a Go module -- fall back to manual breakpoints if `go get` fails. |
| Gradients | `github.com/lucasb-eyer/go-colorful` | HCL color blending (already a transitive dep) |
| Markdown | `github.com/charmbracelet/glamour/v2` | Markdown rendering in help/overlay panes |
| Testing | `go test ./...` | Existing test framework |
| Build | `make build` | Existing build system |

## 12. Project Infrastructure

### Directory Structure

```
internal/
  ui/
    palette/
      palette.go              # NEW: Shared charmtone palette + gradient helpers
      palette_test.go         # NEW: Color rendering tests
  tui/
    theme.go                  # MODIFIED: Uses palette, no hardcoded hex
    pane.go                   # MODIFIED: Section headers, border vocab
    render.go                 # MODIFIED: Status bar, help bar, truncation
    dashboard.go              # MODIFIED: Breakpoints, spinners, header
    agent_table.go            # MODIFIED: bubbles/table replacement
    command_palette.go        # MODIFIED: bubbles/list + textinput
    events_pane.go            # MODIFIED: bubbles/viewport
    evals_pane.go             # MODIFIED: bubbles/list, kill Dracula
    jira_pane.go              # MODIFIED: bubbles/list grouped
    merge_view.go             # MODIFIED: Gradient triangles, icons
    mail_summary.go           # MODIFIED: Icons, charmtone
    floating.go               # MODIFIED: Consistent borders, gradient title
    cost_tracker.go           # MODIFIED: Charmtone colors
    header.go                 # NEW: Wordmark + header bar
    git_status.go             # MODIFIED: Charmtone colors
    openbrain_pane.go         # MODIFIED: Charmtone colors
  commands/
    styles.go                 # MODIFIED: Imports palette, kills all hex
    status_bar.go             # MODIFIED: Structured layout
    prompt_line.go            # MODIFIED: Charmtone colors
```

### Version Management

No version bump required for a visual-only refactor. The binary version (`cmdr --version`) is unchanged.

### CI Workflow

Existing CI runs `go test ./...` and `go vet ./...`. New palette tests are picked up automatically. No CI config changes needed.

### Scripts

```json
{
  "scripts": {
    "build": "make build",
    "test": "go test ./...",
    "vet": "go vet ./...",
    "lint-hex": "grep -rn '#[0-9A-Fa-f]\\{6\\}' internal/tui/ internal/commands/styles.go | grep -v palette.go | grep -v _test.go"
  }
}
```

The `lint-hex` script verifies no hardcoded hex values remain outside the palette package.

## 13. Estimated Size

| Area | Files | LOC (changed/new) |
|------|-------|--------------------|
| Palette package (new) | 2 | ~120 |
| Theme rewrite | 1 | ~150 |
| Pane rendering (titles, borders) | 1 | ~80 |
| Render helpers (status bar, help bar, truncation) | 1 | ~60 |
| Header/wordmark (new) | 1 | ~50 |
| Dashboard (breakpoints, spinners) | 1 | ~100 |
| Agent table (bubbles/table) | 1 | ~200 |
| Command palette (bubbles/list + textinput) | 1 | ~180 |
| Events pane (bubbles/viewport) | 1 | ~80 |
| Evals pane (bubbles/list) | 1 | ~120 |
| Jira pane (bubbles/list) | 1 | ~120 |
| Merge view (gradient triangles) | 1 | ~60 |
| Minor files (mail, floating, cost, git, openbrain) | 5 | ~100 |
| Commands styles.go rewrite | 1 | ~60 |
| Status bar + prompt line | 2 | ~50 |
| Tests | 2 | ~80 |
| **Total** | **~25** | **~1,610** |

## 14. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T0 | unix-coder | Pre-flight dependency verification: `go get` all new dependencies (bubbletea/v2, lipgloss/v2, bubbles/v2, charmtone, glamour/v2, go-colorful), verify they resolve. Run `go doc github.com/charmbracelet/x/exp/charmtone` to confirm token names. If ultraviolet fails to resolve, note it as unavailable for T9 fallback. | go.mod | go.mod, go.sum | -- | `go mod tidy && go vet ./...` |
| T1 | unix-coder | Create shared palette package with charmtone tokens and gradient helpers. After `go get`, run `go doc github.com/charmbracelet/x/exp/charmtone` to verify token names. If any token name differs from this spec, use the hex values from the comments (e.g., `#AD58B4` for Primary) as `lipgloss.Color()` constants instead. | internal/tui/theme.go, internal/commands/styles.go | internal/ui/palette/palette.go, internal/ui/palette/palette_test.go | T0 | `go test ./internal/ui/palette/...` |
| T2 | unix-coder | Rewrite theme.go to import palette, eliminate all hardcoded hex, add three-tier foreground styles. Also migrate `internal/tui/palette.go` (existing `AgentColorStyle`, `PaletteStyles`, `CompletedGoldStyle`) -- move color definitions to `internal/ui/palette/palette.go` and update all callers, then delete the old file or re-export from the new package. | internal/tui/theme.go, internal/tui/palette.go, internal/ui/palette/palette.go | internal/tui/theme.go, internal/tui/palette.go | T1 | `go vet ./internal/tui/...` |
| T3 | unix-coder | Rewrite commands/styles.go to import palette, eliminate all hardcoded hex including Dracula colors | internal/commands/styles.go, internal/ui/palette/palette.go | internal/commands/styles.go | T1 | `go vet ./internal/commands/...` |
| T4 | unix-coder | Add status icons to agent states and merge states (icon + label pattern) | internal/tui/agent_table.go, internal/tui/merge_view.go, internal/commands/status.go | internal/tui/agent_table.go, internal/tui/merge_view.go, internal/commands/status.go | T2 | `go vet ./internal/tui/... && go vet ./internal/commands/...` |
| T5 | unix-coder | Replace `..` truncation with Unicode ellipsis `…` in render.go | internal/tui/render.go | internal/tui/render.go | T2 | `go test ./internal/tui/... -run TestTruncate` |
| T6 | unix-coder | Rewrite RenderPane: section-header titles with gradient fill, border vocabulary (PTY thick-left, data thin-left, agent session borderless) | internal/tui/pane.go, internal/ui/palette/palette.go | internal/tui/pane.go | T2 | `go vet ./internal/tui/...` |
| T7 | unix-coder | Rewrite status bar: structured layout with lipgloss.Place, gradient project name, animated spinner placeholder | internal/tui/render.go, internal/ui/palette/palette.go | internal/tui/render.go | T5 | `go vet ./internal/tui/...` |
| T8 | unix-coder | Rewrite help bar: key+action format with bold keys, muted actions, thin `│` dividers | internal/tui/render.go | internal/tui/render.go | T7 | `go vet ./internal/tui/...` |
| T9 | unix-coder | Add responsive breakpoints to dashboard: wide/normal/compact/minimal layout modes. Use `ultraviolet` if available (check T0 result); otherwise implement manual `calculateLayout()` with if/else breakpoints at 160/120/80 cols. | internal/tui/dashboard.go | internal/tui/dashboard.go | T6 | `go vet ./internal/tui/...` |
| T10 | unix-coder | Add hex spinner for working agent state and loading states | internal/tui/dashboard.go, internal/tui/agent_table.go | internal/tui/dashboard.go, internal/tui/agent_table.go | T4, T9 | `go vet ./internal/tui/...` |
| T11 | unix-coder | Create header.go: ASCII wordmark with gradient diagonal fill, persistent header bar | internal/ui/palette/palette.go | internal/tui/header.go | T2 | `go vet ./internal/tui/...` |
| T12 | unix-coder | Integrate header bar into dashboard View() | internal/tui/dashboard.go, internal/tui/header.go | internal/tui/dashboard.go | T9, T10, T11 | `go vet ./internal/tui/...` |
| T13 | unix-coder | Replace AgentTable with bubbles/table.Model, custom cell renderer with status icons and charmtone colors | internal/tui/agent_table.go | internal/tui/agent_table.go | T10 | `go vet ./internal/tui/...` |
| T14 | unix-coder | Replace CommandPalette with bubbles/list.Model + textinput.Model, gradient title bar, category sections, `▌` selection indicator | internal/tui/command_palette.go | internal/tui/command_palette.go | T2, T6 | `go vet ./internal/tui/...` |
| T15 | unix-coder | Replace EventsPane with bubbles/viewport.Model, status icons | internal/tui/events_pane.go | internal/tui/events_pane.go | T2 | `go vet ./internal/tui/...` |
| T16 | unix-coder | Replace EvalsPane with bubbles/list.Model, kill Dracula palette, use charmtone | internal/tui/evals_pane.go | internal/tui/evals_pane.go | T2, T3 | `go vet ./internal/tui/...` |
| T17 | unix-coder | Replace JiraPane with bubbles/list.Model using grouped items | internal/tui/jira_pane.go | internal/tui/jira_pane.go | T2 | `go vet ./internal/tui/...` |
| T18 | unix-coder | Add gradient triangle queue indicators to merge_view.go | internal/tui/merge_view.go, internal/ui/palette/palette.go | internal/tui/merge_view.go | T4 | `go vet ./internal/tui/...` |
| T19 | unix-coder | Update remaining minor files: mail_summary.go, floating.go, cost_tracker.go, git_status.go, openbrain_pane.go with charmtone colors and consistent borders | internal/tui/mail_summary.go, internal/tui/floating.go, internal/tui/cost_tracker.go, internal/tui/git_status.go, internal/tui/openbrain_pane.go | internal/tui/mail_summary.go, internal/tui/floating.go, internal/tui/cost_tracker.go, internal/tui/git_status.go, internal/tui/openbrain_pane.go | T2, T6 | `go vet ./internal/tui/...` |
| T20 | unix-coder | Update commands/status_bar.go and prompt_line.go to use palette | internal/commands/status_bar.go, internal/commands/prompt_line.go, internal/ui/palette/palette.go | internal/commands/status_bar.go, internal/commands/prompt_line.go | T3 | `go vet ./internal/commands/...` |
| T21 | unix-coder | Update go.mod: add bubbles/v2, lipgloss/v2, bubbletea/v2, charmtone, glamour/v2, go-colorful deps. Only add ultraviolet if T0 confirmed it resolves. | go.mod, go.sum | go.mod, go.sum | T0 | `go mod tidy && go vet ./...` |
| T22 | code-review | Review all changes for: no remaining hardcoded hex, consistent palette usage, Bubbles v2 component wiring, gradient rendering correctness | all modified files | -- | T0-T21 | `grep -rn '#[0-9A-Fa-f]\{6\}' internal/tui/ internal/commands/styles.go \| grep -v palette.go \| grep -v _test.go \| wc -l` outputs `0` |
| T23 | unix-coder | Final integration: run full test suite, build binary, verify dashboard launches | all files | -- | T22 | `make build && go test ./... && timeout 5s ./cmdr dashboard --tui 2>&1; test $? -eq 124 -o $? -eq 0` |

## 15. Dependency Graph

```
Phase 1 (parallel): [T0, T21]
  T0: Pre-flight dependency verification
  T21: Update go.mod with new dependencies

Phase 2 (parallel, after Phase 1): [T1]
  T1: Create shared palette package

Phase 3 (parallel, after Phase 2): [T2, T3]
  T2: Rewrite theme.go with palette
  T3: Rewrite commands/styles.go with palette

Phase 4 (parallel, after Phase 3): [T4, T5, T6, T11]
  T4: Add status icons to agent/merge states
  T5: Fix truncation to Unicode ellipsis
  T6: Rewrite RenderPane with section headers + border vocab
  T11: Create header.go wordmark

Phase 5 (parallel, after Phase 4): [T7, T9, T14, T15, T16, T17, T18, T19, T20]
  T7: Rewrite status bar structured layout (depends on T5)
  T9: Add responsive breakpoints to dashboard
  T14: Replace CommandPalette with bubbles/list
  T15: Replace EventsPane with bubbles/viewport
  T16: Replace EvalsPane with bubbles/list
  T17: Replace JiraPane with bubbles/list
  T18: Add gradient triangle queue indicators
  T19: Update minor files (mail, floating, cost, git, openbrain)
  T20: Update commands status_bar.go + prompt_line.go

Phase 6 (parallel, after Phase 5): [T8, T10, T12, T13]
  T8: Rewrite help bar (depends on T7)
  T10: Add hex spinners for working state
  T12: Integrate header bar into dashboard
  T13: Replace AgentTable with bubbles/table

Phase 7 (after Phase 6): [T22]
  T22: Code review -- no remaining hex, consistent usage

Final (after Phase 7): [T23]
  T23: Integration test -- build, test, dashboard launches
```

## 16. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `internal/ui/palette/palette.go` | ~80 | No |
| `internal/ui/palette/palette_test.go` | ~40 | No |
| `internal/tui/header.go` | ~50 | No |

Files modified:

- `internal/tui/theme.go`
- `internal/tui/palette.go` (migrate color definitions to `internal/ui/palette/palette.go`, then delete or re-export)
- `internal/tui/pane.go`
- `internal/tui/render.go`
- `internal/tui/dashboard.go`
- `internal/tui/agent_table.go`
- `internal/tui/command_palette.go`
- `internal/tui/events_pane.go`
- `internal/tui/evals_pane.go`
- `internal/tui/jira_pane.go`
- `internal/tui/merge_view.go`
- `internal/tui/mail_summary.go`
- `internal/tui/floating.go`
- `internal/tui/cost_tracker.go`
- `internal/tui/git_status.go`
- `internal/tui/openbrain_pane.go`
- `internal/commands/status.go`
- `internal/commands/styles.go`
- `internal/commands/status_bar.go`
- `internal/commands/prompt_line.go`
- `go.mod`
- `go.sum`

Files deleted: None

## 17. Verification Plan

**Per-task checks:**
- T0: `go mod tidy && go vet ./...`
- T1: `go test ./internal/ui/palette/...`
- T2: `go vet ./internal/tui/...`
- T3: `go vet ./internal/commands/...`
- T4: `go vet ./internal/tui/... && go vet ./internal/commands/...`
- T5: `go test ./internal/tui/... -run TestTruncate`
- T6-T19: `go vet ./internal/tui/...`
- T20: `go vet ./internal/commands/...`
- T21: `go mod tidy && go vet ./...`
- T22: `grep -rn '#[0-9A-Fa-f]\{6\}' internal/tui/ internal/commands/styles.go | grep -v palette.go | grep -v _test.go | wc -l` outputs `0`
- T23: `make build && go test ./...`

**Integration check:**
```bash
make build && go test ./... && timeout 5s ./cmdr dashboard --tui 2>&1; test $? -eq 124 -o $? -eq 0
```

**Rollback:** `git stash` or `git checkout -b crush-ui-rollback && git reset --hard HEAD~N` where N is the number of commits in this feature branch.

### Functional Smoke Tests

#### TUI Smoke Tests

**Dashboard launches without crash:**

```bash
timeout 5s cmdr dashboard --tui 2>&1 | tail -5
test $? -eq 124 -o $? -eq 0
```

**Status pane renders with new icons:**

```bash
timeout 3s cmdr status --pane 2>&1 | grep -qE '●|⋯|✓|×|◇'
```

#### Binary Install Verification

```bash
make build
INSTALLED=$(cmdr --version 2>&1)
BUILT=$(./cmdr --version 2>&1)
test "$INSTALLED" = "$BUILT" || { echo "STALE INSTALL: installed=$INSTALLED built=$BUILT"; exit 1; }
```

#### Hex Color Audit

```bash
# Zero hardcoded hex outside palette package and test files
COUNT=$(grep -rn '#[0-9A-Fa-f]\{6\}' internal/tui/ internal/commands/styles.go | grep -v palette.go | grep -v _test.go | wc -l)
test "$COUNT" -eq 0 || { echo "REMAINING HEX: $COUNT occurrences"; exit 1; }
```

## 18. Success Criteria (Machine-Verifiable)

- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `make build` exits 0
- [ ] `go test ./internal/ui/palette/...` exits 0 -- palette package tests pass
- [ ] `test -f internal/ui/palette/palette.go` -- shared palette package exists
- [ ] `test -f internal/tui/header.go` -- wordmark header file exists
- [ ] `grep -rn '#[0-9A-Fa-f]\{6\}' internal/tui/ internal/commands/styles.go | grep -v palette.go | grep -v _test.go | wc -l` outputs `0` -- zero hardcoded hex outside palette
- [ ] `grep -c 'charmtone' internal/ui/palette/palette.go` outputs >= 10 -- palette uses charmtone tokens
- [ ] `grep -c 'Dracula' internal/tui/evals_pane.go internal/commands/styles.go` outputs `0` -- Dracula palette eliminated
- [ ] `grep -qE 'bubbles.*v2.*table' internal/tui/agent_table.go` -- AgentTable uses bubbles/table
- [ ] `grep -qE 'bubbles.*v2.*list' internal/tui/command_palette.go` -- CommandPalette uses bubbles/list
- [ ] `grep -qE 'bubbles.*v2.*viewport' internal/tui/events_pane.go` -- EventsPane uses bubbles/viewport
- [ ] `grep -qE 'bubbles.*v2.*list' internal/tui/evals_pane.go` -- EvalsPane uses bubbles/list
- [ ] `grep -qE 'bubbles.*v2.*spinner' internal/tui/dashboard.go` -- Dashboard uses spinner for animation
- [ ] `grep -q '…' internal/tui/render.go` -- truncation uses Unicode ellipsis
- [ ] `grep -qv '\[.*\].*File Picker' internal/tui/pane.go || true` -- no bracket-prefix titles (grep for absence)
- [ ] `grep -c 'go-colorful' internal/ui/palette/palette.go` outputs >= 1 -- gradient helpers use go-colorful
- [ ] `timeout 5s ./cmdr dashboard --tui 2>&1; test $? -eq 124 -o $? -eq 0` -- dashboard launches without crash

### Functional Smoke Criteria

- [ ] `timeout 3s cmdr status --pane 2>&1 | grep -qE '●|⋯|✓|×|◇'` -- status pane renders with Crush icons
- [ ] `make build && INSTALLED=$(cmdr --version 2>&1) && BUILT=$(./cmdr --version 2>&1) && test "$INSTALLED" = "$BUILT"` -- installed binary matches build artifact

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Phase 1-6: All implementation tasks (T0-T21, T23) | `unix-coder` | Mechanical refactoring + Bubbles v2 component wiring is pure implementation work |
| Phase 7: Code review (T22) | `code-review` | Post-implementation review for palette consistency, hex leaks, component correctness |

## Execution Order

```
Phase 1: Pre-flight [no dependencies]
  +-- T0: Dependency verification (agent: unix-coder)
  +-- T21: go.mod dependency updates (agent: unix-coder)  [parallel]

Phase 2: Foundation [blocked by Phase 1]
  +-- T1: Shared palette package (agent: unix-coder)

Phase 3: Color Migration [blocked by Phase 2]
  +-- T2: theme.go rewrite (agent: unix-coder)
  +-- T3: styles.go rewrite (agent: unix-coder)  [parallel]

Phase 4: Quick Wins [blocked by Phase 3]
  +-- T4: Status icons (agent: unix-coder)
  +-- T5: Truncation fix (agent: unix-coder)
  +-- T6: Pane titles + borders (agent: unix-coder)
  +-- T11: Wordmark header (agent: unix-coder)  [parallel]

Phase 5: Component Replacement [blocked by Phase 4]
  +-- T7: Status bar (agent: unix-coder) [depends on T5]
  +-- T9: Responsive breakpoints (agent: unix-coder)
  +-- T14: CommandPalette -> bubbles/list (agent: unix-coder)
  +-- T15: EventsPane -> bubbles/viewport (agent: unix-coder)
  +-- T16: EvalsPane -> bubbles/list (agent: unix-coder)
  +-- T17: JiraPane -> bubbles/list (agent: unix-coder)
  +-- T18: Gradient triangle indicators (agent: unix-coder)
  +-- T19: Minor file updates (agent: unix-coder)
  +-- T20: Commands style updates (agent: unix-coder)  [parallel]

Phase 6: Integration [blocked by Phase 5]
  +-- T8: Help bar (agent: unix-coder) [depends on T7]
  +-- T10: Hex spinners (agent: unix-coder)
  +-- T12: Header bar integration (agent: unix-coder)
  +-- T13: AgentTable -> bubbles/table (agent: unix-coder)  [parallel]

Phase 7: Review [blocked by Phase 6]
  +-- T22: Code review (agent: code-review)

Final: Verification [blocked by Phase 7]
  +-- T23: Integration test (agent: unix-coder)
```

Recommended directive: `/swarm` -- Phase-gated parallel fan-out. The 8 phases have clear sequential dependencies with parallelism within each phase. `/swarm` leverages the parallel structure better than sequential `/pai`.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Bubbles v2 API incompatibility with existing BubbleTea v1 event loop | `go vet ./...` fails with type mismatch errors | Pin to bubbletea v2 (not v1). All BubbleTea code must be updated to v2 API simultaneously in T21. |
| charmtone import path changes (experimental package) | `go mod tidy` fails | Check `github.com/charmbracelet/x` for current path. Fall back to hardcoded hex matching charmtone values if package unavailable. |
| ultraviolet not yet released or API unstable | `go get` fails | Fall back to manual percentage-based layout with breakpoint `if/else` in `calculateLayout()`. Mark T9 as degraded. |
| Gradient rendering produces mojibake in non-truecolor terminals | Visual inspection in 256-color terminal | Add `lipgloss.HasDarkBackground()` / `colorprofile` detection. Fall back to solid colors when truecolor unavailable. |
| Spinner commands cause event loop performance degradation | Dashboard becomes sluggish with >10 agents | Throttle spinner tick rate to 150ms. Use a single shared spinner model instead of per-agent spinners. |
| Existing tests fail after Theme struct changes | `go test ./internal/tui/...` fails | Tests that construct `Theme` directly need updating to use `DefaultTheme()`. Fix in T2. |
| Hand-rolled widget removal breaks dashboard Update() key dispatch | Dashboard key events stop reaching panes | Each bubbles component must be wired into the dashboard `Update()` switch. Verify all existing keybinds still work via keybind coverage test. |
| Agent palette (12-color array) lost in theme rewrite | Agents render without color differentiation | Preserve `AgentColors` field in Theme, regenerate from charmtone gradient (Dolly through Charple in 12 HCL steps). |

## Open Questions

| # | Question | Impact | Suggested Default |
|---|----------|--------|-------------------|
| 1 | Should the BubbleTea/Lipgloss/Bubbles upgrade be v1->v2 or stay on v1? The spec assumes v2 but the current go.mod has bubbletea v1.3.10 and lipgloss v1.1.0. | Bubbles v2 components require bubbletea v2 and lipgloss v2. If staying on v1, component replacement tasks (T13-T17) must use bubbles v1 equivalents which have fewer features. | Upgrade to v2 -- the refactor is a natural upgrade point. Handle API changes in T21. |
| 2 | Is `charmtone` stable enough for production use? It lives under `charmbracelet/x/exp/`. | If API changes, palette.go must be updated. | Use it -- the experimental API is just color constants which are unlikely to change. Wrap in palette.go so only one file needs updating if it does. |
| 3 | Is `ultraviolet` available as a Go module? It may be unreleased or Rust-only. | T9 (responsive breakpoints) depends on it for layout calculation. | Fall back to manual breakpoint `if/else` in `calculateLayout()`. The responsive behavior is more important than the layout engine. |
| 4 | Should the wordmark header consume a terminal row permanently, reducing dashboard content height by 1? | Affects information density on small terminals. | Yes on wide/normal layouts (>=120 cols), no on compact/minimal. Header bar is 1 row -- negligible on modern terminals. |
| 5 | The agent palette uses 12 hardcoded hex colors for agent identification. Should these be replaced with a charmtone gradient or kept as-is? | Visual consistency vs agent distinguishability. | Replace with 12-step HCL gradient from Dolly to Charple. Agents need to be visually distinct but within the palette. |

## Charm API Reference

> **API v2 DISCLAIMER:** The Bubbles v2 code examples below are based on v1 API patterns and must be adapted to v2 signatures during implementation. After T0/T21 install dependencies, run `go doc` on each package to confirm constructor signatures, method names, and type interfaces. The charmtone token examples are from an experimental (`exp/`) package -- verify with `go doc github.com/charmbracelet/x/exp/charmtone` before using.

### charmtone Color Tokens

```go
import "github.com/charmbracelet/x/exp/charmtone"

// Each token is a lipgloss.Color (string hex value).
// Usage: lipgloss.NewStyle().Foreground(charmtone.Charple)
//
// Accent colors
charmtone.Charple   // #AD58B4 -- purple, primary accent
charmtone.Dolly     // #FFE156 -- yellow, secondary accent
charmtone.Bok       // #FF6F61 -- coral, tertiary accent
//
// Background colors
charmtone.Pepper    // #1A1A1A -- darkest, main bg
charmtone.Charcoal  // #2B2B2B -- subtle bg, panels
charmtone.Iron      // #3B3B3B -- overlay bg, dialogs
//
// Foreground colors
charmtone.Ash       // #DDDBD7 -- primary text
charmtone.Squid     // #6C6C6C -- muted text
charmtone.Oyster    // #9A9A9A -- subtle text, hints
//
// Semantic colors
charmtone.Sriracha  // #ED567A -- error, danger
charmtone.Zest      // #FFBA52 -- warning, caution
charmtone.Malibu    // #7DC1FF -- info, link
charmtone.Julep     // #6CCB5F -- success, complete
charmtone.Guac      // #4A9E3F -- in-progress, active
```

### go-colorful HCL Blending

```go
import "github.com/lucasb-eyer/go-colorful"

// Create colors from hex
c1, _ := colorful.Hex("#FFE156") // Dolly
c2, _ := colorful.Hex("#AD58B4") // Charple

// Blend in HCL space (0.0 = c1, 1.0 = c2)
mid := c1.BlendHcl(c2, 0.5)
hex := mid.Hex() // e.g., "#D87C85"

// Use as lipgloss color
style := lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
```

### Bubbles v2 Table

```go
import "github.com/charmbracelet/bubbles/v2/table"

columns := []table.Column{
    {Title: "Name", Width: 20},
    {Title: "State", Width: 10},
    {Title: "Duration", Width: 12},
}

rows := []table.Row{
    {"unix-coder-a1b2", "● working", "3m42s"},
    {"code-review-c3d4", "✓ done", "1m15s"},
}

t := table.New(
    table.WithColumns(columns),
    table.WithRows(rows),
    table.WithFocused(true),
    table.WithHeight(10),
)

// Style the table
s := table.DefaultStyles()
s.Header = s.Header.
    Foreground(lipgloss.Color(string(charmtone.Ash))).
    Bold(true).
    BorderBottom(true).
    BorderStyle(lipgloss.NormalBorder()).
    BorderForeground(lipgloss.Color(string(charmtone.Iron)))
s.Selected = s.Selected.
    Foreground(lipgloss.Color(string(charmtone.Ash))).
    Background(lipgloss.Color(string(charmtone.Iron))).
    Bold(true)
t.SetStyles(s)
```

### Bubbles v2 List with Custom Delegate

```go
import (
    "github.com/charmbracelet/bubbles/v2/list"
    "github.com/charmbracelet/bubbles/v2/key"
)

// Item interface
type paletteItem struct {
    name, desc, category string
    action func()
}
func (i paletteItem) Title() string       { return i.name }
func (i paletteItem) Description() string { return i.desc }
func (i paletteItem) FilterValue() string { return i.name + " " + i.desc }

// Custom delegate for rendering
type paletteDelegate struct{}
func (d paletteDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
    i := item.(paletteItem)
    selected := index == m.Index()
    if selected {
        fmt.Fprintf(w, "▌ %s", lipgloss.NewStyle().Bold(true).Render(i.name))
    } else {
        fmt.Fprintf(w, "  %s", i.name)
    }
}

items := []list.Item{
    paletteItem{name: "kill", desc: "Kill agent session", category: "Agent"},
    paletteItem{name: "sling", desc: "Spawn new agent", category: "Core"},
}
l := list.New(items, paletteDelegate{}, 40, 20)
l.Title = "Command Palette"
l.SetFilteringEnabled(true)
```

### Bubbles v2 Viewport

```go
import "github.com/charmbracelet/bubbles/v2/viewport"

vp := viewport.New(width, height)
vp.SetContent(eventLogContent)

// In Update():
vp, cmd = vp.Update(msg)

// In View():
return vp.View()
```

### Bubbles v2 Spinner

```go
import "github.com/charmbracelet/bubbles/v2/spinner"

s := spinner.New()
s.Spinner = spinner.Dot // or spinner.MiniDot, spinner.Pulse, etc.
s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(charmtone.Julep)))

// In Init():
return s.Tick

// In Update():
s, cmd = s.Update(msg)

// In View():
if isWorking {
    return s.View() + " working"
}
```

### Gradient Pane Title Pattern

```go
func renderPaneTitle(title string, focused bool, width int, theme *Theme) string {
    remaining := width - lipgloss.Width(title) - 3
    if remaining < 0 {
        remaining = 0
    }
    if focused {
        icon := lipgloss.NewStyle().Foreground(palette.Primary).Render("● ")
        titleRendered := lipgloss.NewStyle().Bold(true).Foreground(palette.FgBase).Render(title)
        gradFill := palette.GradientFill("╱", remaining, palette.Secondary, palette.Primary)
        return icon + titleRendered + " " + gradFill
    }
    fill := lipgloss.NewStyle().Foreground(palette.FgMuted).Render(strings.Repeat("─", remaining))
    titleRendered := lipgloss.NewStyle().Foreground(palette.FgMuted).Render(title)
    return "  " + titleRendered + " " + fill
}
```

### Gradient Triangle Queue Indicator

```go
func renderQueueDepth(count int, maxDisplay int) string {
    if count == 0 {
        return lipgloss.NewStyle().Foreground(palette.FgSubtle).Render("empty")
    }
    n := min(count, maxDisplay)
    triangles := strings.Repeat("▶", n)
    chars := palette.ForegroundGrad(triangles, palette.Error, palette.Secondary)
    pill := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(palette.BgOverlay).
        Padding(0, 1)
    return pill.Render(strings.Join(chars, "") + fmt.Sprintf(" %d", count))
}
```

### Border Vocabulary Pattern

```go
// PTY panes: thick left accent, no box border
ptyBorder := lipgloss.NewStyle().
    BorderLeft(true).
    BorderStyle(lipgloss.ThickBorder()).
    BorderForeground(palette.Primary)

// Data panes: thin left border
dataBorder := lipgloss.NewStyle().
    BorderLeft(true).
    BorderStyle(lipgloss.NormalBorder()).
    BorderForeground(palette.BgOverlay)

// Agent session (center main): borderless
mainBorder := lipgloss.NewStyle() // no border at all
```
