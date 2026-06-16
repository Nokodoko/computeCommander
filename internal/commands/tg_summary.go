package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/trustgraph"
)

// TGSummaryCmd returns the "tg-summary" subcommand: a single-shot, fixed-shape,
// optionally-ANSI-colored top-N TrustGraph summary suitable for embedding in a
// small ASCII frame next to the existing OB1 status frame.
//
// Unlike tg-list, this command emits exactly --lines lines and exits. It is
// intended to be invoked as a subprocess by the claude/pi sessionbanner to
// populate an adjacent "tg" frame. The data path matches tg-list exactly so
// the embedded summary never regresses to stale/hardcoded counts:
//
//   - gateway URL via cfg.ResolveGatewayURL() (the lewis dynamic-gateway fix:
//     --tg-url / TG_URL / TRUSTGRAPH_URL / config.gateway_url / monty default)
//   - per-query deadline via cfg.TGQueryTimeout() (the lewis config-driven
//     timeout fix, default 30s — NOT a hardcoded 5s)
//   - live TriplesQuery against POST /api/v1/flow/{flow}/service/triples
//
// Node/edge counts in the header are computed from the LIVE result set on every
// invocation; nothing is hardcoded. This is the contractual fix that keeps the
// lewis tg pane dynamic where the mac reference froze the counts.
func TGSummaryCmd(app *App) *cobra.Command {
	var (
		lines   int
		width   int
		noColor bool
	)
	cmd := &cobra.Command{
		Use:   "tg-summary",
		Short: "Emit a fixed-shape, embeddable top-N TrustGraph summary",
		Long: `Single-shot top-N TrustGraph summary, sized to embed in a ~5-8 line
ASCII frame. Reuses the same live data path as tg-list (POST /api/v1/flow/{flow}/
service/triples, config-driven timeout) but renders a fixed line count
(default 5):

  ── connected · 41 nodes · 89 edges
  top: alice(d=12) bob(d=9) carol(d=7)
  edge: alice --owns--> repo:cmdr
  edge: bob --merged--> branch:codeviewer
  updated 14:32:07

Edge budget = --lines - 3 (header + top-nodes + trailer). With --lines < 3,
only header + trailer are emitted. On TG client error or empty result, a
diagnostic hint line + padded body + "updated HH:MM:SS" trailer are emitted;
the embedded frame size is preserved. Honours NO_COLOR per https://no-color.org.

Counts are LIVE on every invocation (computed from the query result), never
hardcoded. Gateway URL resolution and the per-query timeout are inherited from
the TrustGraph config exactly as tg-list resolves them.`,
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTGSummary(cmd.Context(), app, lines, width, noColor)
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 5, "total output lines including header and trailer")
	cmd.Flags().IntVar(&width, "width", 60, "inner width hint, used for label truncation")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "suppress all ANSI colour codes (also honours $NO_COLOR)")
	return cmd
}

// runTGSummary fetches once, formats to exactly `lines` lines, and exits.
// On any failure it still emits exactly `lines` lines of padded output so the
// embedded frame size does not shift.
//
// The gateway URL and per-query deadline are resolved from the TrustGraph
// config via the SAME helpers tg-list uses (ResolveGatewayURL, TGQueryTimeout)
// so the embedded summary tracks the live graph and never regresses to the
// stale hardcoded counts of the mac reference.
func runTGSummary(ctx context.Context, app *App, lines, width int, noColor bool) error {
	cfg := app.Config.TrustGraph

	// NO_COLOR env var (https://no-color.org): any non-empty value disables.
	if !noColor {
		if v := os.Getenv("NO_COLOR"); v != "" {
			noColor = true
		}
	}

	// Palette identical to tg_list.go; emptied when noColor is set.
	pal := newPalette(noColor)

	now := time.Now().Format("15:04:05")

	// Resolve the gateway through the same path tg-list uses. ResolveGatewayURL
	// always returns at least the configured default, so an empty value here
	// means TrustGraph is genuinely unconfigured.
	gatewayURL := cfg.ResolveGatewayURL()
	if gatewayURL == "" {
		emitDisconnected(lines, pal, "trustgraph gateway not configured", now)
		return nil
	}

	client := trustgraph.New(gatewayURL, cfg.Token, cfg.FlowID)
	defer client.Close()

	if !client.Available() {
		emitDisconnected(lines, pal, "gateway unreachable: "+gatewayURL, now)
		return nil
	}

	// Triple-query limit (matches tg-list + the HTML viewer default). This
	// bounds the fetched sample; the header counts are derived from whatever
	// the gateway returns, so they grow with the live graph up to this ceiling.
	queryLimit := cfg.MaxTriples
	if queryLimit <= 0 {
		queryLimit = 200
	}

	// Config-driven per-query deadline (the lewis timeout fix), NOT a hardcoded
	// 5s. Mirrors tg-list exactly.
	queryCtx, queryCancel := context.WithTimeout(ctx, cfg.TGQueryTimeout())
	resp, err := client.TriplesQuery(queryCtx, trustgraph.TriplesQueryRequest{Limit: queryLimit})
	queryCancel()

	if err != nil {
		emitDisconnected(lines, pal, err.Error(), now)
		return nil
	}

	triples := resp.Response
	if len(triples) == 0 {
		emitDisconnected(lines, pal, "no triples returned", now)
		return nil
	}

	// Compute aggregate counts + sorted top-nodes from the LIVE result set.
	// If the gateway returned exactly queryLimit triples the underlying graph
	// may be larger and our counts are a lower bound — flag with a "+" suffix.
	nodeCount, edgeCount, capped, topNodes := summarizeTriples(triples, queryLimit)
	suffix := ""
	if capped {
		suffix = "+"
	}

	// Render to exactly `lines` lines.
	out := make([]string, 0, lines)

	// Line 1: substantive detail. The embedding frame's title already conveys
	// "tg · connected · <host>" so we do NOT re-emit a "TG · …" banner here;
	// we keep only the diagnostic detail (node/edge counts) that the title
	// cannot convey. The leading "──" anchors the line as a detail row.
	header := pal.dim + "── " + pal.reset +
		pal.green + "connected" + pal.reset +
		pal.dim + fmt.Sprintf(" · %d%s nodes · %d%s edges", nodeCount, suffix, edgeCount, suffix) + pal.reset
	out = append(out, truncateVisible(header, width))

	if lines >= 3 {
		// Line 2: top-nodes — bold labels, yellow degrees.
		out = append(out, renderTopNodesLine(topNodes, width, pal))

		// Edge budget = lines - 3 (header + top-nodes + trailer).
		edgeBudget := lines - 3
		if edgeBudget > 0 {
			n := edgeBudget
			if n > len(triples) {
				n = len(triples)
			}
			for i := 0; i < n; i++ {
				out = append(out, renderEdgeLine(triples[i], width, pal))
			}
			// Pad to budget if fewer triples than the edge budget.
			for i := n; i < edgeBudget; i++ {
				out = append(out, "")
			}
		}
	}

	// Trailer (always last).
	trailer := pal.dim + "updated " + now + pal.reset
	out = appendOrReplaceTrailer(out, lines, trailer)

	for _, ln := range out {
		fmt.Fprintln(os.Stdout, ln)
	}
	return nil
}

// summarizeTriples computes aggregate node/edge counts and a degree-sorted
// top-nodes slice from a triples query result. It is the pure compute kernel
// of runTGSummary; isolated so unit tests can pin the counts-from-live-result
// contract without spinning up a TG gateway.
//
// Inputs:
//   - triples: the full result slice from TriplesQuery (no truncation here).
//   - countLimit: the ceiling the caller passed to the query. When the result
//     length equals this ceiling we assume the gateway may have capped it, so
//     the header should render a "+" suffix on both counts.
//
// Returns:
//   - nodeCount: number of distinct nodes (subjects + entity-typed objects).
//   - edgeCount: number of triples in the input slice.
//   - capped: true iff len(triples) == countLimit AND countLimit > 0.
//   - topNodes: descending-degree, label-ascending tie-break; safe to render.
func summarizeTriples(triples []trustgraph.Triple, countLimit int) (nodeCount, edgeCount int, capped bool, topNodes []summaryNode) {
	nodes := make(map[string]*summaryNode)
	touchNode := func(t trustgraph.Term) {
		id := t.DisplayValue()
		if id == "" {
			return
		}
		if n, ok := nodes[id]; ok {
			n.degree++
			return
		}
		nodes[id] = &summaryNode{
			label:  t.ShortLabel(28),
			degree: 1,
		}
	}
	for _, tr := range triples {
		touchNode(tr.Subject)
		if tr.Object.IsEntity() {
			touchNode(tr.Object)
		}
	}

	topNodes = make([]summaryNode, 0, len(nodes))
	for _, n := range nodes {
		topNodes = append(topNodes, *n)
	}
	sort.Slice(topNodes, func(i, j int) bool {
		if topNodes[i].degree != topNodes[j].degree {
			return topNodes[i].degree > topNodes[j].degree
		}
		return topNodes[i].label < topNodes[j].label
	})

	return len(nodes), len(triples), countLimit > 0 && len(triples) == countLimit, topNodes
}

// summaryNode carries the per-node display label and aggregated degree used
// by the top-N summary line. (Named distinctly from the function-local
// nodeInfo in tg_list.go to avoid any future package-level collision.)
type summaryNode struct {
	label  string
	degree int
}

// renderTopNodesLine builds the "top: alice(d=12) bob(d=9) ..." line.
// It iterates topNodes greedily, packing as many "label(d=N)" tokens as fit
// within --width visible chars (ANSI codes excluded from the width budget).
func renderTopNodesLine(topNodes []summaryNode, width int, pal palette) string {
	prefix := "top:"
	prefixVis := visibleLen(prefix)
	var sb strings.Builder
	sb.WriteString(prefix)
	visUsed := prefixVis

	for _, n := range topNodes {
		// Each token renders as: " <bold>label<reset>(d=<yellow>N<reset>)".
		// Visible length of the token is len(" label(d=N)").
		token := " " + n.label + "(d=" + fmt.Sprintf("%d", n.degree) + ")"
		tokenVis := visibleLen(token)
		if visUsed+tokenVis > width {
			break
		}
		sb.WriteString(" ")
		sb.WriteString(pal.bold)
		sb.WriteString(n.label)
		sb.WriteString(pal.reset)
		sb.WriteString("(d=")
		sb.WriteString(pal.yellow)
		sb.WriteString(fmt.Sprintf("%d", n.degree))
		sb.WriteString(pal.reset)
		sb.WriteString(")")
		visUsed += tokenVis
	}
	return sb.String()
}

// renderEdgeLine builds an "edge: <subject> --<predicate>--> <object>" line.
// The whole line is truncated to `width` visible chars (ANSI codes excluded).
func renderEdgeLine(tr trustgraph.Triple, width int, pal palette) string {
	prefix := "edge: "
	prefixVis := visibleLen(prefix)
	avail := width - prefixVis
	if avail < 12 {
		// Width too small to render anything sensible; fall back to short labels.
		s := tr.Subject.ShortLabel(4)
		p := tr.Predicate.ShortLabel(4)
		o := tr.Object.ShortLabel(4)
		return truncateVisible(prefix+s+" --"+p+"--> "+o, width)
	}

	// Allocate roughly 1/3 each, predicate slightly tighter ("--p-->" eats 6).
	thirdS := (avail - 6) / 3
	thirdP := (avail - 6) / 3
	thirdO := (avail - 6) - thirdS - thirdP
	if thirdS < 4 {
		thirdS = 4
	}
	if thirdP < 4 {
		thirdP = 4
	}
	if thirdO < 4 {
		thirdO = 4
	}
	s := tr.Subject.ShortLabel(thirdS)
	p := tr.Predicate.ShortLabel(thirdP)
	o := tr.Object.ShortLabel(thirdO)

	line := prefix + pal.bold + s + pal.reset +
		pal.dim + " --" + pal.reset + p +
		pal.dim + "--> " + pal.reset + pal.bold + o + pal.reset
	return truncateVisibleANSI(line, width)
}

// emitDisconnected writes a disconnected/error frame padded to `lines` lines.
//
// The embedding frame's title (rendered by the harness) already shows
// "tg · disconnected · <host>"; emitting a second "TG · disconnected" banner
// inside the body duplicates that signal. We emit only the diagnostic hint
// (which the frame title cannot convey) on line 1, pad to lines-1, and place
// the "updated HH:MM:SS" trailer last.
//
//	line 1: red diagnostic hint (e.g. "gateway unreachable: http://...")
//	...    : empty padding
//	line N: dim "updated HH:MM:SS" trailer
func emitDisconnected(lines int, pal palette, hint, now string) {
	out := make([]string, 0, lines)
	// Truncate hint defensively to a reasonable inner width (60 chars visible).
	hintLine := pal.red + truncateVisible(hint, 60) + pal.reset
	out = append(out, hintLine)

	// Pad with empty lines up to lines-1; trailer fills the last slot.
	for i := len(out); i < lines-1; i++ {
		out = append(out, "")
	}

	trailer := pal.dim + "updated " + now + pal.reset
	out = appendOrReplaceTrailer(out, lines, trailer)

	for _, ln := range out {
		fmt.Fprintln(os.Stdout, ln)
	}
}

// appendOrReplaceTrailer normalises `out` to exactly `lines` entries, with
// `trailer` as the last line. Truncates any overflow; pads with empties if
// short. With lines <= 0 returns an empty slice.
func appendOrReplaceTrailer(out []string, lines int, trailer string) []string {
	if lines <= 0 {
		return nil
	}
	if len(out) > lines-1 {
		out = out[:lines-1]
	}
	for len(out) < lines-1 {
		out = append(out, "")
	}
	return append(out, trailer)
}

// ── ANSI palette + visible-width helpers ─────────────────────────────────────

type palette struct {
	bold, dim, reset         string
	red, green, yellow, cyan string
}

func newPalette(noColor bool) palette {
	if noColor {
		return palette{}
	}
	return palette{
		bold:   "\033[1m",
		dim:    "\033[2m",
		reset:  "\033[0m",
		red:    "\033[31m",
		green:  "\033[32m",
		yellow: "\033[33m",
		cyan:   "\033[36m",
	}
}

// visibleLen returns the count of "visible" runes in s, skipping ANSI CSI
// escape sequences (\033[…m). Good enough for our palette which only uses
// SGR sequences.
func visibleLen(s string) int {
	n := 0
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if c >= 0x40 && c <= 0x7E {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		c := s[i]
		if c < 0x80 || c >= 0xC0 {
			n++
		}
		i++
	}
	return n
}

// truncateVisible truncates s to at most `max` visible chars. Ignores ANSI
// codes in width accounting but preserves them in the prefix that fits.
// Use truncateVisibleANSI when s is known to contain SGR codes that must be
// balanced with a trailing reset.
func truncateVisible(s string, max int) string {
	if max <= 0 || visibleLen(s) <= max {
		return s
	}
	return truncateVisibleANSI(s, max)
}

// truncateVisibleANSI truncates s to at most `max` visible chars, skipping
// ANSI CSI sequences in the width accounting and appending a trailing reset
// if any SGR was encountered before the cut point. Cuts on a rune boundary.
func truncateVisibleANSI(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var sb strings.Builder
	visible := 0
	sawSGR := false
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if c >= 0x40 && c <= 0x7E {
					j++
					break
				}
				j++
			}
			sb.WriteString(s[i:j])
			sawSGR = true
			i = j
			continue
		}
		if visible >= max {
			break
		}
		c := s[i]
		runeLen := 1
		switch {
		case c < 0x80:
			runeLen = 1
		case c >= 0xF0:
			runeLen = 4
		case c >= 0xE0:
			runeLen = 3
		case c >= 0xC0:
			runeLen = 2
		}
		if i+runeLen > len(s) {
			runeLen = len(s) - i
		}
		sb.WriteString(s[i : i+runeLen])
		visible++
		i += runeLen
	}
	if sawSGR {
		sb.WriteString("\033[0m")
	}
	return sb.String()
}
