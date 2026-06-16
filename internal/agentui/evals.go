package agentui

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EvalRecord is the renderable shape of an eval row, mirroring the schema
// queried by internal/commands/evals.go:runEvalsPane. The renderer is
// decoupled from the live DB so tests can pass a fixed slice.
type EvalRecord struct {
	ID          string
	ProjectName string
	AgentTask   string
	EvalType    string
	// Passed is nil when the eval has never run. true / false otherwise.
	Passed      *bool
	ErrorDetail string
	// LastRunAt and CreatedAt are RFC3339 strings (with a tolerant parser
	// for the space-separated SQLite datetime('now') variant). Empty when
	// the record is freshly inserted.
	LastRunAt string
	CreatedAt string
}

// EvalSource is the dependency contract for RenderEvals. Implementations
// fetch eval records from the DB; tests pass a static slice.
type EvalSource interface {
	ListEvals(ctx context.Context, projectFilter string) ([]EvalRecord, error)
}

// EvalsOpts is the contract surface of RenderEvals.
type EvalsOpts struct {
	Lines   int
	Width   int
	NoColor bool
	Now     time.Time
	// Project (optional) limits results to a single project name. Mirrors
	// internal/commands/evals.go --project flag.
	Project string
}

// RenderEvals fetches eval records via src and renders them as exactly
// opts.Lines lines, each <= opts.Width visible cols. On any fetch failure,
// returns the "evals: unavailable" degraded marker. Empty registry
// returns "evals: no data" padded to opts.Lines.
//
// Sort: COALESCE(last_run_at, created_at) DESC — matches the existing
// runEvalsPane sort. Caller's src is responsible for the SQL ORDER BY;
// RenderEvals does NOT re-sort beyond a defensive stable-sort fallback.
func RenderEvals(ctx context.Context, src EvalSource, opts EvalsOpts) []string {
	if opts.Lines <= 0 {
		return nil
	}
	if opts.Width <= 0 {
		return DegradedMarker(LabelEvals, opts.Lines)
	}
	if src == nil {
		return DegradedMarker(LabelEvals, opts.Lines)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	evals, err := src.ListEvals(ctx, opts.Project)
	if err != nil {
		return DegradedMarker(LabelEvals, opts.Lines)
	}
	if len(evals) == 0 {
		return DegradedMarkerWithReason(LabelEvals, ReasonNoData, opts.Lines)
	}

	pal := NewPalette(opts.NoColor)
	bs := NewBoxStyle(pal)

	// Aggregate pass/fail counts for the header.
	pass, fail := 0, 0
	for _, e := range evals {
		if e.Passed == nil {
			continue
		}
		if *e.Passed {
			pass++
		} else {
			fail++
		}
	}

	out := make([]string, 0, opts.Lines)

	// Line 1: header.
	header := pal.Bold + "Evals" + pal.Reset +
		pal.Dim + bs.Sep + fmt.Sprintf("%d registered", len(evals)) + pal.Reset
	if pass+fail > 0 {
		header += pal.Dim + bs.Sep + fmt.Sprintf("%d pass / %d fail", pass, fail) + pal.Reset
	}
	out = append(out, Truncate(header, opts.Width))

	// Reserve last line for trailer.
	rowBudget := max(opts.Lines-2, 0)

	// Emit row lines.
	shown := 0
	overflow := 0
	for i, e := range evals {
		if shown >= rowBudget {
			overflow = len(evals) - i
			break
		}
		row := renderEvalRow(e, opts.Width, now, pal)
		out = append(out, Truncate(row, opts.Width))
		shown++
	}

	// If there is overflow AND we still have an unfilled row slot, swap
	// the last visible row with an overflow indicator instead of padding.
	if overflow > 0 && shown > 0 {
		indicator := pal.Dim + fmt.Sprintf("... +%d more", overflow) + pal.Reset
		out[len(out)-1] = Truncate(indicator, opts.Width)
	}

	// Pad row area with empty lines if fewer evals than budget.
	for shown < rowBudget {
		out = append(out, "")
		shown++
	}

	if opts.Lines >= 2 {
		trailer := pal.Dim + "updated " + now.Format("15:04:05") + pal.Reset
		out = append(out, Truncate(trailer, opts.Width))
	}

	return PadOrTruncate(out, opts.Lines)
}

// renderEvalRow formats a single eval record row. Width is the per-row
// budget; the row format is "  STATUS  TYPE  ID  TASK  AGO".
func renderEvalRow(e EvalRecord, width int, now time.Time, pal Palette) string {
	// Status: PASS / FAIL / NEVER.
	var status string
	switch {
	case e.Passed == nil:
		status = pal.Dim + "----" + pal.Reset
	case *e.Passed:
		status = pal.Green + pal.Bold + "PASS" + pal.Reset
	default:
		status = pal.Red + pal.Bold + "FAIL" + pal.Reset
	}

	// Type colored with Dracula palette.
	typeColored := e.EvalType
	if pal.Reset != "" {
		if hex := EvalTypeHex(e.EvalType); hex != "" {
			if esc := pal.Hex24(hex); esc != "" {
				typeColored = esc + e.EvalType + pal.Reset
			}
		}
	}

	// Relative age — falls back to created_at when last_run_at is empty.
	ts := e.LastRunAt
	if ts == "" {
		ts = e.CreatedAt
	}
	ago := formatEvalAgoStr(ts, now)
	agoDim := pal.Dim + ago + pal.Reset

	// Columns: status(4) typ(12) id(14) task(remainder) ago(8)
	typWidth := 12
	idWidth := 14
	agoWidth := 8
	taskBudget := max(width-2-4-1-typWidth-1-idWidth-1-agoWidth-1, 4)

	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(padRightVisible(status, 4))
	sb.WriteString(" ")
	sb.WriteString(padRightVisible(truncASCIIColored(typeColored, e.EvalType, typWidth), typWidth))
	sb.WriteString(" ")
	sb.WriteString(padRightAscii(truncASCII(e.ID, idWidth), idWidth))
	sb.WriteString(" ")
	sb.WriteString(padRightAscii(truncASCII(e.AgentTask, taskBudget), taskBudget))
	sb.WriteString(" ")
	sb.WriteString(padRightVisible(agoDim, agoWidth))
	return sb.String()
}

// truncASCIIColored truncates the colored variant by referencing the plain
// equivalent's visible length. Used because we cannot directly truncate
// inside an SGR sequence without knowing where the visible text starts.
func truncASCIIColored(colored, plain string, w int) string {
	if VisibleLen(plain) <= w {
		return colored
	}
	// Truncate the plain string, return ungiven color (acceptable
	// degradation — width clamp takes priority over color).
	return truncASCII(plain, w)
}

// formatEvalAgoStr is the renderer-friendly variant of
// internal/commands/evals.go:formatEvalAgo.
func formatEvalAgoStr(ts string, now time.Time) string {
	if ts == "" {
		return "-"
	}
	cleaned := strings.Replace(ts, " ", "T", 1)
	if !strings.HasSuffix(cleaned, "Z") && !strings.Contains(cleaned, "+") {
		cleaned += "Z"
	}
	t, err := time.Parse(time.RFC3339, cleaned)
	if err != nil {
		return "-"
	}
	d := max(now.Sub(t), 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
