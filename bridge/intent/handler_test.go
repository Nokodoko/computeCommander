package intent

import (
	"encoding/json"
	"testing"

	"github.com/noko/computecommander/bridge"
)

func TestHandlerInputWithOutcomes(t *testing.T) {
	prompt := "# Task\nDo something\n\n# Outcomes\n\n- Tests pass for all new code\n- API responds with 200\n- No hardcoded secrets\n"
	payload, _ := json.Marshal(map[string]string{"text": prompt})

	req := &bridge.BridgeRequest{
		Hook:    "intent-verify",
		Event:   "input",
		Payload: payload,
	}

	resp, err := Handler(req)
	if err != nil {
		t.Fatalf("Handler error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.Error)
	}

	var result intentResult
	if err := json.Unmarshal(resp.Output, &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("expected 3 outcomes, got %d", result.Total)
	}
	if result.Action != "continue" && result.Action != "blocked" {
		t.Errorf("expected action continue or blocked, got %q", result.Action)
	}
}

func TestHandlerInputNoOutcomes(t *testing.T) {
	prompt := "Just do the thing"
	payload, _ := json.Marshal(map[string]string{"text": prompt})

	req := &bridge.BridgeRequest{
		Hook:    "intent-verify",
		Event:   "input",
		Payload: payload,
	}

	resp, err := Handler(req)
	if err != nil {
		t.Fatalf("Handler error: %v", err)
	}

	var result intentResult
	if err := json.Unmarshal(resp.Output, &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if result.Action != "skip" {
		t.Errorf("expected action=skip for no outcomes, got %q", result.Action)
	}
}

func TestHandlerUnknownEvent(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{})
	req := &bridge.BridgeRequest{
		Hook:    "intent-verify",
		Event:   "some_other_event",
		Payload: payload,
	}

	resp, err := Handler(req)
	if err != nil {
		t.Fatalf("Handler error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success for unknown event")
	}
}
