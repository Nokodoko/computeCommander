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

// TGListCmd returns the "tg-list" subcommand: a long-lived terminal renderer
// that periodically clears the screen and prints a list of TrustGraph nodes
// (top by degree) and edges (subject --predicate--> object) fetched from the
// gateway's POST /api/v1/flow/{flow}/service/triples endpoint.
//
// This replaces the Electron trustgraph-viewer overlay for low-resource
// environments — the same data is rendered as a plain text list inside the
// existing "TG Viz" zellij pane.
func TGListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tg-list",
		Short:   "List TrustGraph nodes and edges in a refreshing pane",
		Long: `Periodically poll the TrustGraph gateway and print a text list of
the top nodes (by degree) and edges (triples). Designed to run in a long-lived
zellij pane as a lightweight replacement for the Electron trustgraph-viewer
overlay. Refreshes roughly every 10 seconds; honours Ctrl+C / context cancel.`,
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTGList(cmd.Context(), app)
		},
	}
	return cmd
}

// runTGList is the long-running render loop for tg-list.
// It exits cleanly when ctx is cancelled (Ctrl+C / pane close).
func runTGList(ctx context.Context, app *App) error {
	cfg := app.Config.TrustGraph

	// Fixed 10s refresh per the user-requested cadence; ignore RefreshSecs
	// here since this is a heavyweight clear+reprint render, not the compact
	// "tg --pane" status view.
	refreshInterval := 10 * time.Second

	const (
		bold    = "\033[1m"
		dim     = "\033[2m"
		green   = "\033[32m"
		red     = "\033[31m"
		cyan    = "\033[36m"
		yellow  = "\033[33m"
		reset   = "\033[0m"
		clearSc = "\033[2J\033[H"
	)

	// Limits for what we render. Spec calls for top 30 nodes, top 50 edges.
	const maxNodes = 30
	const maxEdges = 50

	// Triple-query limit (matches the HTML viewer + tg --pane default).
	queryLimit := cfg.MaxTriples
	if queryLimit <= 0 {
		queryLimit = 200
	}

	// If TrustGraph is disabled we still keep the pane alive — print a
	// hint and idle on the refresh tick so closing the pane is clean.
	if !cfg.Enabled {
		for {
			fmt.Fprint(os.Stdout, clearSc)
			fmt.Fprintln(os.Stdout, dim+" TG  disabled"+reset)
			fmt.Fprintln(os.Stdout, dim+strings.Repeat("─", 40)+reset)
			fmt.Fprintln(os.Stdout, dim+"  Enable in config:"+reset)
			fmt.Fprintln(os.Stdout, dim+"  trustgraph.enabled: true"+reset)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(refreshInterval):
			}
		}
	}

	client := trustgraph.New(cfg.GatewayURL, cfg.Token, cfg.FlowID)
	defer client.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var buf strings.Builder
		buf.WriteString(clearSc)

		if !client.Available() {
			buf.WriteString(bold + cyan + " TG" + reset + "  " + red + "disconnected" + reset + "\n")
			buf.WriteString(dim + "  Gateway: " + cfg.GatewayURL + reset + "\n")
			buf.WriteString(dim + "  last update: " + time.Now().Format("15:04:05") + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(refreshInterval):
			}
			continue
		}

		queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := client.TriplesQuery(queryCtx, trustgraph.TriplesQueryRequest{Limit: queryLimit})
		queryCancel()

		if err != nil {
			errMsg := err.Error()
			if len(errMsg) > 60 {
				errMsg = errMsg[:58] + ".."
			}
			buf.WriteString(bold + cyan + " TG" + reset + "  " + red + "error" + reset + "\n")
			buf.WriteString(dim + "  " + errMsg + reset + "\n")
			buf.WriteString(dim + "  last update: " + time.Now().Format("15:04:05") + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(refreshInterval):
			}
			continue
		}

		triples := resp.Response

		// Aggregate node info for the top-by-degree view.
		// We track the short label, full ID, and a tiny "host" hint extracted
		// from the IRI authority (the segment between // and the first /),
		// which gives a cheap "who owns this entity" cue.
		type nodeInfo struct {
			label  string
			fullID string
			host   string
			degree int
		}
		nodes := make(map[string]*nodeInfo)
		touchNode := func(t trustgraph.Term) {
			id := t.DisplayValue()
			if id == "" {
				return
			}
			if n, ok := nodes[id]; ok {
				n.degree++
				return
			}
			nodes[id] = &nodeInfo{
				label:  t.ShortLabel(28),
				fullID: id,
				host:   extractIRIHost(t),
				degree: 1,
			}
		}
		for _, tr := range triples {
			touchNode(tr.Subject)
			if tr.Object.IsEntity() {
				touchNode(tr.Object)
			}
		}

		topNodes := make([]nodeInfo, 0, len(nodes))
		for _, n := range nodes {
			topNodes = append(topNodes, *n)
		}
		sort.Slice(topNodes, func(i, j int) bool {
			if topNodes[i].degree != topNodes[j].degree {
				return topNodes[i].degree > topNodes[j].degree
			}
			return topNodes[i].label < topNodes[j].label
		})
		if len(topNodes) > maxNodes {
			topNodes = topNodes[:maxNodes]
		}

		buf.WriteString(bold + cyan + " TG" + reset + "  " + green + "connected" + reset)
		buf.WriteString(dim + fmt.Sprintf("  %d nodes  %d edges", len(nodes), len(triples)) + reset + "\n")
		buf.WriteString(bold + " == TG Nodes (top " + fmt.Sprintf("%d", maxNodes) + " by degree) ==" + reset + "\n")
		if len(topNodes) == 0 {
			buf.WriteString(dim + "  (no nodes)" + reset + "\n")
		} else {
			for _, n := range topNodes {
				host := n.host
				if host == "" {
					host = "-"
				}
				buf.WriteString(fmt.Sprintf("  %s%-28s%s  %sdeg=%-3d%s  %shost=%s%s\n",
					bold, n.label, reset,
					yellow, n.degree, reset,
					dim, host, reset,
				))
			}
		}

		buf.WriteString(bold + " == TG Edges (top " + fmt.Sprintf("%d", maxEdges) + ") ==" + reset + "\n")
		edgeCount := len(triples)
		if edgeCount > maxEdges {
			edgeCount = maxEdges
		}
		for i := 0; i < edgeCount; i++ {
			tr := triples[i]
			s := tr.Subject.ShortLabel(24)
			p := tr.Predicate.ShortLabel(24)
			o := tr.Object.ShortLabel(24)
			buf.WriteString(fmt.Sprintf("  %s --%s--> %s\n", s, p, o))
		}
		if len(triples) > maxEdges {
			buf.WriteString(dim + fmt.Sprintf("  ... and %d more edges", len(triples)-maxEdges) + reset + "\n")
		}

		buf.WriteString("\n")
		buf.WriteString(dim + "  last update: " + time.Now().Format("15:04:05") + reset + "\n")

		fmt.Fprint(os.Stdout, buf.String())

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(refreshInterval):
		}
	}
}

// extractIRIHost returns the authority portion of an IRI Term for display.
// Returns "" for non-IRI terms or IRIs without a recognizable scheme.
func extractIRIHost(t trustgraph.Term) string {
	if !t.IsEntity() {
		return ""
	}
	iri := t.IRI
	// Strip scheme.
	for _, prefix := range []string{"http://", "https://", "tg://"} {
		if strings.HasPrefix(iri, prefix) {
			iri = iri[len(prefix):]
			break
		}
	}
	// Authority is everything up to the first '/', '#', or '?'.
	for i := 0; i < len(iri); i++ {
		c := iri[i]
		if c == '/' || c == '#' || c == '?' {
			return iri[:i]
		}
	}
	if iri == t.IRI {
		// No scheme stripped and no separator — likely a bare IRI segment.
		return ""
	}
	return iri
}
