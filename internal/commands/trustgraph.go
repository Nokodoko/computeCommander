package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/trustgraph"
	"github.com/spf13/cobra"
)

// ─── TrustGraph pane command ─────────────────────────────────────────────────

// TrustGraphCmd creates the "tg" subcommand for TrustGraph dashboard integration.
// In --pane mode it runs a long-lived loop that polls the TrustGraph gateway and
// renders a compact ANSI status view suitable for a zellij dashboard pane.
func TrustGraphCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tg",
		Short:   "TrustGraph knowledge graph status",
		Long:    "Display TrustGraph gateway status, node/edge counts, and top entities. In --pane mode, streams live updates with ANSI styling for the zellij dashboard.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if paneMode {
				return runTGPane(cmd.Context(), app)
			}

			return printTGSummary(app, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// ─── One-shot summary ────────────────────────────────────────────────────────

func printTGSummary(app *App, jsonOut bool) error {
	cfg := app.Config.TrustGraph
	if !cfg.Enabled {
		fmt.Println("TrustGraph is disabled. Enable in config: trustgraph.enabled: true")
		return nil
	}

	client := trustgraph.New(cfg.GatewayURL, cfg.Token, cfg.FlowID)
	defer client.Close()

	if !client.Available() {
		fmt.Printf("TrustGraph gateway unreachable: %s\n", cfg.GatewayURL)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := cfg.MaxTriples
	if limit <= 0 {
		limit = 200
	}

	resp, err := client.TriplesQuery(ctx, trustgraph.TriplesQueryRequest{Limit: limit})
	if err != nil {
		return fmt.Errorf("triples query: %w", err)
	}

	nodes, topEntities := deriveTGStats(resp.Response, cfg.MaxNodes)

	if jsonOut {
		fmt.Printf(`{"status":"connected","nodes":%d,"edges":%d,"top_entities":%d}%s`,
			len(nodes), len(resp.Response), len(topEntities), "\n")
		return nil
	}

	fmt.Printf("TrustGraph: \033[32mconnected\033[0m  %d nodes  %d edges\n", len(nodes), len(resp.Response))
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("Top Entities (by degree):")
	for i, e := range topEntities {
		if i >= 15 {
			fmt.Printf("  ... and %d more\n", len(topEntities)-15)
			break
		}
		fmt.Printf("  %-24s %3d edges\n", e.id, e.degree)
	}

	return nil
}

// ─── Long-lived pane mode ────────────────────────────────────────────────────

func runTGPane(ctx context.Context, app *App) error {
	cfg := app.Config.TrustGraph

	refreshInterval := time.Duration(cfg.RefreshSecs) * time.Second
	if refreshInterval < 2*time.Second {
		refreshInterval = 5 * time.Second
	}

	// ANSI helpers.
	const (
		bold    = "\033[1m"
		dim     = "\033[2m"
		green   = "\033[32m"
		red     = "\033[31m"
		cyan    = "\033[36m"
		yellow  = "\033[33m"
		white   = "\033[37m"
		reset   = "\033[0m"
		clearSc = "\033[2J\033[H" // clear screen + cursor home
	)

	var client *trustgraph.Client

	for {
		select {
		case <-ctx.Done():
			if client != nil {
				client.Close()
			}
			return nil
		default:
		}

		var buf strings.Builder

		buf.WriteString(clearSc)

		if !cfg.Enabled {
			buf.WriteString(dim + " TG  disabled" + reset + "\n")
			buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n\n")
			buf.WriteString(dim + "  Enable in config:" + reset + "\n")
			buf.WriteString(dim + "  trustgraph.enabled: true" + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(refreshInterval):
			}
			continue
		}

		// Lazily create client.
		if client == nil {
			client = trustgraph.New(cfg.GatewayURL, cfg.Token, cfg.FlowID)
		}

		if !client.Available() {
			buf.WriteString(bold + cyan + " TG" + reset + "  " + dim + "disconnected" + reset + "\n")
			buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n\n")
			buf.WriteString(dim + "  Gateway: " + cfg.GatewayURL + reset + "\n")
			buf.WriteString(dim + "  Waiting for connection..." + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			select {
			case <-ctx.Done():
				client.Close()
				return nil
			case <-time.After(refreshInterval):
			}
			continue
		}

		// Query triples.
		queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
		limit := cfg.MaxTriples
		if limit <= 0 {
			limit = 200
		}
		resp, err := client.TriplesQuery(queryCtx, trustgraph.TriplesQueryRequest{Limit: limit})
		queryCancel()

		if err != nil {
			buf.WriteString(bold + cyan + " TG" + reset + "  " + red + "error" + reset + "\n")
			buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n\n")
			errMsg := err.Error()
			if len(errMsg) > 60 {
				errMsg = errMsg[:58] + ".."
			}
			buf.WriteString(red + "  " + errMsg + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			select {
			case <-ctx.Done():
				client.Close()
				return nil
			case <-time.After(refreshInterval):
			}
			continue
		}

		// Derive stats.
		nodes, topEntities := deriveTGStats(resp.Response, cfg.MaxNodes)
		nodeCount := len(nodes)
		edgeCount := len(resp.Response)

		// Render header.
		buf.WriteString(bold + cyan + " TG" + reset + "  " + green + "connected" + reset)
		buf.WriteString(dim + fmt.Sprintf("  %d nodes  %d edges", nodeCount, edgeCount) + reset + "\n")
		buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n")

		// Top entities.
		buf.WriteString(bold + white + " Top Entities:" + reset + "\n")

		if len(topEntities) == 0 {
			buf.WriteString(dim + "  No entities found." + reset + "\n")
		} else {
			maxShow := 12
			if len(topEntities) < maxShow {
				maxShow = len(topEntities)
			}

			// Find max name length for alignment.
			maxNameLen := 0
			for _, e := range topEntities[:maxShow] {
				if len(e.id) > maxNameLen {
					maxNameLen = len(e.id)
				}
			}
			if maxNameLen > 22 {
				maxNameLen = 22
			}

			for _, e := range topEntities[:maxShow] {
				name := e.id
				if len(name) > 22 {
					name = name[:20] + ".."
				}
				bar := strings.Repeat("|", min(e.degree, 20))
				buf.WriteString(fmt.Sprintf("  "+cyan+"%-*s"+reset+"  "+dim+"%3d"+reset+" "+yellow+"%s"+reset+"\n",
					maxNameLen, name, e.degree, bar))
			}

			if len(topEntities) > maxShow {
				buf.WriteString(dim + fmt.Sprintf("  ... +%d more", len(topEntities)-maxShow) + reset + "\n")
			}
		}

		// Footer with timestamp.
		buf.WriteString("\n" + dim + "  " + time.Now().Format("15:04:05") + "  refresh " + fmt.Sprintf("%ds", int(refreshInterval.Seconds())) + reset + "\n")

		fmt.Fprint(os.Stdout, buf.String())

		select {
		case <-ctx.Done():
			client.Close()
			return nil
		case <-time.After(refreshInterval):
		}
	}
}

// ─── Shared helpers ──────────────────────────────────────────────────────────

// tgNodeInfo holds aggregated entity info for the CLI view.
type tgNodeInfo struct {
	id     string // short display label
	fullID string // full IRI
	degree int    // total edge count
}

// deriveTGStats computes node/edge stats and top entities from triples.
func deriveTGStats(triples []trustgraph.Triple, maxNodes int) (map[string]*tgNodeInfo, []tgNodeInfo) {
	nodes := make(map[string]*tgNodeInfo)
	for _, triple := range triples {
		sLabel := triple.Subject.ShortLabel(32)
		sID := triple.Subject.DisplayValue()
		if n, ok := nodes[sID]; ok {
			n.degree++
		} else {
			nodes[sID] = &tgNodeInfo{id: sLabel, fullID: sID, degree: 1}
		}

		if triple.Object.IsEntity() {
			oLabel := triple.Object.ShortLabel(32)
			oID := triple.Object.DisplayValue()
			if n, ok := nodes[oID]; ok {
				n.degree++
			} else {
				nodes[oID] = &tgNodeInfo{id: oLabel, fullID: oID, degree: 1}
			}
		}
	}

	topEntities := make([]tgNodeInfo, 0, len(nodes))
	for _, n := range nodes {
		topEntities = append(topEntities, *n)
	}
	sort.Slice(topEntities, func(i, j int) bool {
		return topEntities[i].degree > topEntities[j].degree
	})

	if maxNodes <= 0 {
		maxNodes = 100
	}
	if len(topEntities) > maxNodes {
		topEntities = topEntities[:maxNodes]
	}

	return nodes, topEntities
}
