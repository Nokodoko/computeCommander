package agentui

// EvalTypeHex returns the Dracula-palette hex string for an eval type
// name as used by the existing internal/commands/evals.go. Returns the
// empty string for unknown types (renderer falls back to no per-type
// color).
//
// This palette mirrors evalTypeANSI in internal/commands/evals.go verbatim
// so the embeddable subcommand output matches the long-lived --pane mode
// rendering byte-for-byte (modulo width-clamping).
var evalTypeHex = map[string]string{
	"unit_test":            "#8be9fd", // cyan
	"integration":          "#bd93f9", // purple
	"lint":                 "#f1fa8c", // yellow
	"build":                "#50fa7b", // green
	"custom":               "#66d9ef", // blue-cyan
	"semantic_check":       "#50fa7b", // green
	"structural_check":     "#50fa7b", // green
	"contains_pattern":     "#50fa7b", // green
	"count_check":          "#50fa7b", // green
	"ast_check":            "#50fa7b", // green
	"negation_check":       "#50fa7b", // green
	"test_execution":       "#8be9fd", // cyan
	"type_check":           "#66d9ef", // blue-cyan
	"diff_validation":      "#f1fa8c", // yellow
	"output_pattern_match": "#bd93f9", // purple
}

// EvalTypeHex returns the hex color string for an eval type. Empty when
// unknown.
func EvalTypeHex(evalType string) string {
	return evalTypeHex[evalType]
}
