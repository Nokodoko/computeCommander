package cmdr

import (
	"encoding/json"
	"testing"

	"github.com/noko/computecommander/bridge"
)

func TestHandlerSessionStartEvent(t *testing.T) {
	// Without cmdr on PATH in CI this will fail gracefully.
	// The test verifies the handler doesn't panic and returns a response.
	payload, _ := json.Marshal(map[string]string{"cwd": "/tmp/test-project"})
	req := &bridge.BridgeRequest{
		Hook:    "cmdr-bridge",
		Event:   "session_start",
		Payload: payload,
	}

	resp, err := Handler(req)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	// If cmdr isn't installed, success=false is expected.
	// If it is installed, success=true.
	_ = resp
}

func TestHandlerUnknownEvent(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{})
	req := &bridge.BridgeRequest{
		Hook:    "cmdr-bridge",
		Event:   "unknown_event",
		Payload: payload,
	}

	resp, err := Handler(req)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true for unhandled events")
	}
	if resp.Context == "" {
		t.Error("expected context message for unhandled events")
	}
}

func TestExtractProject(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]string
		want    string
	}{
		{"from cwd", map[string]string{"cwd": "/home/user/Projects/myapp"}, "myapp"},
		{"from project", map[string]string{"project": "test-proj"}, "test-proj"},
		{"empty", map[string]string{}, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			got := extractProject(raw)
			if got != tc.want {
				t.Errorf("extractProject(%s) = %q, want %q", string(raw), got, tc.want)
			}
		})
	}
}
