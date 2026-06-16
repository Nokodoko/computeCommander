package agentui

// Standardized degraded-marker reasons. The strings below are LOAD-BEARING:
// the sessionbanner consumer side (phase3.md) is allowed to pattern-match on
// them, and changing them is a breaking change.
const (
	ReasonUnavailable = "unavailable"
	ReasonNoData      = "no data"
	ReasonNotARepo    = "not a repo"
)

// Labels for the three renderers. Used by the per-renderer degraded paths.
const (
	LabelAgents = "agents"
	LabelEvals  = "evals"
	LabelGit    = "git"
)
