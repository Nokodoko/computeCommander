// Package cmdr implements the cmdr-bridge hook handler for the Go-TypeScript bridge.
//
// It handles Pi and Claude events by forwarding them to the cmdr CLI for
// session lifecycle management and activity tracking. This is the Go-native
// equivalent of the cmdr-bridge.ts Pi extension — events arriving via
// hook-bridge dispatch to this handler instead of going through TypeScript.
package cmdr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/noko/computecommander/bridge"
)

// sessionState tracks the commander session across bridge invocations.
// Because hook-bridge is invoked as a fresh process per event, state is
// persisted to a JSON file in /tmp so session_shutdown can find the
// session_id that session_start registered.
type sessionState struct {
	mu        sync.Mutex
	sessionID string
	project   string
	lastBeat  time.Time
}

var state sessionState

const stateFile = "/tmp/cmdr-bridge-state.json"

type persistedState struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
}

func loadPersistedState() {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return
	}
	var ps persistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		return
	}
	state.sessionID = ps.SessionID
	state.project = ps.Project
}

func savePersistedState() {
	ps := persistedState{
		SessionID: state.sessionID,
		Project:   state.project,
	}
	data, _ := json.Marshal(ps)
	os.WriteFile(stateFile, data, 0644)
}

func clearPersistedState() {
	os.Remove(stateFile)
}

const heartbeatInterval = 60 * time.Second

// Handler is the HookHandler for the cmdr-bridge binding.
// It dispatches events to the appropriate cmdr CLI subcommands.
func Handler(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	// Restore state from previous invocation.
	loadPersistedState()

	switch req.Event {
	case "session_start", "SessionStart":
		return handleSessionStart(req)
	case "session_shutdown", "SessionEnd":
		return handleSessionEnd(req)
	case "agent_start", "SubagentStart":
		return handleAgentStart(req)
	case "agent_end", "SubagentStop":
		return handleAgentEnd(req)
	case "tool_execution_start", "tool_execution_end", "PostToolUse":
		return handleToolEvent(req)
	case "turn_end":
		return handleTurnEnd(req)
	default:
		return &bridge.BridgeResponse{
			Success: true,
			Context: fmt.Sprintf("cmdr-bridge: unhandled event %q (no-op)", req.Event),
		}, nil
	}
}

// handleSessionStart registers a new session with cmdr.
func handleSessionStart(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	project := extractProject(req.Payload)

	state.mu.Lock()
	// Deregister previous session if any.
	if state.sessionID != "" {
		cmdrExec("deregister", state.sessionID)
	}
	state.mu.Unlock()

	out, err := cmdrExec("register",
		"--name", "pi-session",
		"--runtime", "pi",
		"--capability", "builder",
		"--task", project,
		"--pid", strconv.Itoa(os.Getpid()),
		"--json",
	)
	if err != nil {
		return &bridge.BridgeResponse{
			Success: false,
			Error:   fmt.Sprintf("cmdr register failed: %v", err),
		}, nil
	}

	// Parse session_id from JSON output.
	var reg struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(out), &reg); err == nil && reg.SessionID != "" {
		state.mu.Lock()
		state.sessionID = reg.SessionID
		state.project = project
		state.mu.Unlock()
		savePersistedState()
	}

	return &bridge.BridgeResponse{
		Success: true,
	}, nil
}

// handleSessionEnd deregisters the session.
func handleSessionEnd(_ *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	state.mu.Lock()
	sid := state.sessionID
	state.sessionID = ""
	state.mu.Unlock()

	if sid != "" {
		cmdrExec("deregister", sid)
	}
	clearPersistedState()

	return &bridge.BridgeResponse{Success: true}, nil
}

// handleAgentStart registers a subagent with cmdr.
func handleAgentStart(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	project := extractProject(req.Payload)
	name := fmt.Sprintf("pi-agent-%d", time.Now().UnixMilli())

	out, err := cmdrExec("register",
		"--name", name,
		"--runtime", "pi",
		"--capability", "builder",
		"--task", project,
		"--pid", strconv.Itoa(os.Getpid()),
		"--json",
	)
	if err != nil {
		return &bridge.BridgeResponse{Success: false, Error: err.Error()}, nil
	}

	// Return the session_id as context so the TS side can track it.
	var reg struct {
		SessionID string `json:"session_id"`
	}
	json.Unmarshal([]byte(out), &reg)

	resp := &bridge.BridgeResponse{Success: true}
	if reg.SessionID != "" {
		raw, _ := json.Marshal(map[string]string{"agent_session_id": reg.SessionID})
		resp.Output = raw
	}
	return resp, nil
}

// handleAgentEnd is a no-op for now — the TS extension manages deregistration
// via its LIFO queue. The Go handler acknowledges the event.
func handleAgentEnd(_ *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	return &bridge.BridgeResponse{Success: true}, nil
}

// handleToolEvent emits a feed event for tool activity.
func handleToolEvent(req *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	toolName := "unknown"
	eventType := "tool_event"

	// Try to extract tool name from payload.
	var payload map[string]interface{}
	if err := json.Unmarshal(req.Payload, &payload); err == nil {
		if tn, ok := payload["toolName"].(string); ok {
			toolName = tn
		}
		if strings.HasSuffix(req.Event, "_start") || req.Event == "PostToolUse" {
			eventType = "tool_start"
		} else {
			eventType = "tool_end"
		}
	}

	state.mu.Lock()
	project := state.project
	state.mu.Unlock()

	cmdrExec("feed", "emit",
		"--agent", "pi-session",
		"--type", eventType,
		"--data", fmt.Sprintf("tool=%s", toolName),
		"--runtime", "pi",
		"--task", project,
	)

	return &bridge.BridgeResponse{Success: true}, nil
}

// handleTurnEnd sends a throttled heartbeat.
func handleTurnEnd(_ *bridge.BridgeRequest) (*bridge.BridgeResponse, error) {
	state.mu.Lock()
	sid := state.sessionID
	last := state.lastBeat
	state.mu.Unlock()

	if sid == "" {
		return &bridge.BridgeResponse{Success: true}, nil
	}

	if time.Since(last) < heartbeatInterval {
		return &bridge.BridgeResponse{Success: true}, nil
	}

	cmdrExec("heartbeat", sid, "--state", "working")

	state.mu.Lock()
	state.lastBeat = time.Now()
	state.mu.Unlock()

	return &bridge.BridgeResponse{Success: true}, nil
}

// extractProject tries to pull a project name from the payload's cwd field.
func extractProject(payload json.RawMessage) string {
	var p map[string]interface{}
	if err := json.Unmarshal(payload, &p); err == nil {
		if cwd, ok := p["cwd"].(string); ok && cwd != "" {
			return filepath.Base(cwd)
		}
		if project, ok := p["project"].(string); ok && project != "" {
			return project
		}
	}
	return "unknown"
}

// cmdrExec runs `cmdr <args>` and returns stdout.
func cmdrExec(args ...string) (string, error) {
	cmd := exec.Command("cmdr", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("cmdr %s: %w (%s)", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
