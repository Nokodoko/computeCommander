// Package intent implements the intent-verify hook handler for the Go-TypeScript bridge.
//
// It runs the Go IntentVerifier from internal/darkfactory against incoming prompts,
// returning verification results (pass/fail, scores, blockers) as structured JSON.
// The Pi extension (go-bridge.ts or a dedicated thin wrapper) invokes this via
// hook-bridge and uses the response to gate prompts and fire notifications.
package intent

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/noko/computecommander/bridge"
	"github.com/noko/computecommander/internal/darkfactory"
)

// Handler is the HookHandler for intent-verify.
func Handler(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	switch req.Event {
	case "input", "UserPromptSubmit":
		return handleInput(req)
	case "agent_end", "Stop":
		return handleAgentEnd(req)
	default:
		return &bridge.BridgeResponse{
			Success: true,
			Context: fmt.Sprintf("intent-verify: no-op for event %q", req.Event),
		}, nil
	}
}

// inputPayload is the expected shape of the input event payload.
type inputPayload struct {
	Text    string `json:"text"`
	Prompt  string `json:"prompt"`
	Content string `json:"content"`
}

// intentResult is the structured output returned in BridgeResponse.Output.
type intentResult struct {
	Action      string                     `json:"action"`   // "continue", "blocked", "skip"
	Valid       bool                       `json:"valid"`
	Score       float64                    `json:"score"`
	Total       int                        `json:"total"`
	Passed      int                        `json:"passed"`
	Failed      int                        `json:"failed"`
	Outcomes    []darkfactory.OutcomeCheck  `json:"outcomes,omitempty"`
	Blockers    []string                   `json:"blockers,omitempty"`
	Summary     string                     `json:"summary"`
	Remediation string                     `json:"remediation,omitempty"`
}

func handleInput(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	// Extract prompt text from payload.
	var p inputPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return &bridge.BridgeResponse{
			Success: true,
			Context: "intent-verify: could not parse payload, passing through",
		}, nil
	}

	prompt := p.Text
	if prompt == "" {
		prompt = p.Prompt
	}
	if prompt == "" {
		prompt = p.Content
	}
	if prompt == "" {
		return &bridge.BridgeResponse{
			Success: true,
			Context: "intent-verify: empty prompt, passing through",
		}, nil
	}

	verifier := darkfactory.NewIntentVerifier("")
	result := verifier.Verify(prompt)

	// No parseable outcomes.
	if len(result.Outcomes) == 0 {
		hasHeading := darkfactory.HasOutputsHeading(prompt)

		if !hasHeading {
			// No heading at all → pass through silently, gate doesn't activate.
			out := intentResult{
				Action:  "skip",
				Valid:   true,
				Score:   0,
				Summary: "No # Outputs section — skipped",
			}
			raw, _ := json.Marshal(out)
			return &bridge.BridgeResponse{Success: true, Output: raw}, nil
		}

		// Heading present but no list items → fail with notification.
		summary := "# Outputs heading found but no list items — use bullet (- ) or numbered (1. ) format"
		notifyFail(summary, result.Blockers)

		out := intentResult{
			Action:   "blocked",
			Valid:    false,
			Score:    0,
			Blockers: result.Blockers,
			Summary:  summary,
		}
		raw, _ := json.Marshal(out)

		return &bridge.BridgeResponse{
			Success: true,
			Output:  raw,
			Context: fmt.Sprintf("━━━ INTENT GATE: BLOCKED ━━━\n%s\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━", summary),
		}, nil
	}

	passed := 0
	failed := 0
	for _, o := range result.Outcomes {
		if o.Aligned {
			passed++
		} else {
			failed++
		}
	}

	action := "continue"
	if !result.Valid {
		action = "blocked"
	}

	summary := fmt.Sprintf("%d/%d objectives verified (score: %.2f)", passed, len(result.Outcomes), result.Score)

	out := intentResult{
		Action:   action,
		Valid:    result.Valid,
		Score:    result.Score,
		Total:    len(result.Outcomes),
		Passed:   passed,
		Failed:   failed,
		Outcomes: result.Outcomes,
		Blockers: result.Blockers,
		Summary:  summary,
	}

	// Fire desktop notification via dunstify.
	if result.Valid {
		notifyPass(summary)
	} else {
		out.Remediation = buildRemediation(result, verifier)
		notifyFail(summary, result.Blockers)
	}

	raw, _ := json.Marshal(out)

	resp := &bridge.BridgeResponse{
		Success: true,
		Output:  raw,
	}

	// If blocked, return context with remediation that the agent can act on.
	if !result.Valid {
		var sb strings.Builder
		sb.WriteString("━━━ INTENT GATE: BLOCKED ━━━\n")
		sb.WriteString(summary)
		sb.WriteString("\n\n")

		// Show each failed outcome with reason.
		for _, o := range result.Outcomes {
			if !o.Aligned {
				sb.WriteString(fmt.Sprintf("  ✗ [%.2f] %s — %s\n", o.Score, o.Text, o.Reason))
			}
		}

		if len(result.Blockers) > 0 {
			sb.WriteString("\nBlockers:\n")
			for _, b := range result.Blockers {
				sb.WriteString(fmt.Sprintf("  • %s\n", b))
			}
		}

		sb.WriteString("\n")
		sb.WriteString(out.Remediation)
		sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		resp.Context = sb.String()
	}

	return resp, nil
}

func handleAgentEnd(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	// Re-verify objectives at agent completion.
	// For now, acknowledge — full post-run verification requires response context.
	return &bridge.BridgeResponse{
		Success: true,
		Context: "intent-verify: agent_end acknowledged",
	}, nil
}

// buildRemediation generates a corrected # Outputs section from the failed outcomes.
// It keeps passing outcomes as-is and rewrites failing ones to align with objectives.
func buildRemediation(result *darkfactory.VerifyResult, verifier *darkfactory.IntentVerifier) string {
	objectives := verifier.LoadObjectives()

	var sb strings.Builder
	sb.WriteString("Rewrite your # Outputs section. Replace failing items with objective-aligned outcomes:\n\n")
	sb.WriteString("# Outputs\n\n")

	for i, o := range result.Outcomes {
		if o.Aligned {
			// Keep passing outcomes.
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, o.Text))
		} else {
			// Suggest the closest matching objective, or flag for rewrite.
			suggestion := findClosestObjective(o.Text, objectives)
			if suggestion != "" {
				sb.WriteString(fmt.Sprintf("%d. %s  ← (was: %s)\n", i+1, suggestion, o.Text))
			} else {
				sb.WriteString(fmt.Sprintf("%d. [REWRITE NEEDED] %s  ← no matching objective\n", i+1, o.Text))
			}
		}
	}

	if len(objectives) > 0 {
		sb.WriteString("\nAvailable objectives to align with:\n")
		for _, obj := range objectives {
			sb.WriteString(fmt.Sprintf("  • %s\n", obj))
		}
	}

	return sb.String()
}

// findClosestObjective returns the best-matching objective for a given outcome text.
func findClosestObjective(outcome string, objectives []string) string {
	outLower := strings.ToLower(outcome)
	bestScore := 0.0
	bestObj := ""

	for _, obj := range objectives {
		score := darkfactory.KeywordOverlap(outLower, strings.ToLower(obj))
		if score > bestScore {
			bestScore = score
			bestObj = obj
		}
	}

	// Only suggest if there's some overlap.
	if bestScore >= 0.2 {
		return bestObj
	}
	return ""
}

// notifyPass fires a green dunstify notification.
func notifyPass(summary string) {
	cmd := exec.Command("dunstify",
		"-a", "pi-intent-eval",
		"-u", "low",
		"-t", "4000",
		"-i", "dialog-information",
		"-r", "99201",
		"-h", "string:fgcolor:#00FF00",
		"-h", "string:bgcolor:#1a1a2e",
		"-h", "string:hlcolor:#00CC00",
		"Intent Eval PASSED",
		summary,
	)
	cmd.Run() // Wait for completion so notification lands before process exits.
}

// notifyFail fires a red critical dunstify notification.
func notifyFail(summary string, blockers []string) {
	body := summary
	if len(blockers) > 0 {
		body = summary + "\n" + strings.Join(blockers, "\n")
	}
	cmd := exec.Command("dunstify",
		"-a", "pi-intent-eval",
		"-u", "critical",
		"-t", "8000",
		"-i", "dialog-error",
		"-r", "99201",
		"-h", "string:fgcolor:#FF4500",
		"-h", "string:bgcolor:#1a1a2e",
		"-h", "string:hlcolor:#FF0000",
		"Intent Eval FAILED",
		body,
	)
	cmd.Run() // Wait for completion so notification lands before process exits.
}
