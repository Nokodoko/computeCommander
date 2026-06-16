package agentui

// PadOrTruncate normalises lines to exactly n entries. Trims overflow from
// the end, pads short input with empty strings. n <= 0 returns nil.
func PadOrTruncate(lines []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if len(lines) == n {
		// Allocate a fresh slice so callers cannot mutate the caller's input
		// by mistake when they assume PadOrTruncate returned a copy.
		out := make([]string, n)
		copy(out, lines)
		return out
	}
	out := make([]string, n)
	if len(lines) > n {
		copy(out, lines[:n])
		return out
	}
	copy(out, lines)
	return out
}

// ClampWidth applies Truncate to every line so each is <= w visible cols.
// w <= 0 collapses every line to the empty string.
func ClampWidth(lines []string, w int) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = Truncate(ln, w)
	}
	return out
}

// DegradedMarker returns a single-line marker padded to n total lines.
// Used as the universal failure-mode payload when a renderer cannot
// produce its normal output. label is the short tag (e.g. "agents",
// "evals", "git"); the marker line is "<label>: <reason>" where reason
// defaults to "unavailable".
func DegradedMarker(label string, n int) []string {
	return DegradedMarkerWithReason(label, "unavailable", n)
}

// DegradedMarkerWithReason builds a degraded marker with an explicit reason
// (e.g. "no data" for evals, "not a repo" for git). The first line is
// "<label>: <reason>", followed by empty padding to n.
func DegradedMarkerWithReason(label, reason string, n int) []string {
	if n <= 0 {
		return nil
	}
	out := make([]string, n)
	out[0] = label + ": " + reason
	return out
}
