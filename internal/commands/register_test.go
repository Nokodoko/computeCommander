package commands

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// seedSession inserts a test session into the DB, handling FK constraints properly.
func seedSession(t *testing.T, app *App, sessionID, agentName, capability, taskID, state, runtime string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	err := app.DB.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, capability, worktree_path, branch_name,
			task_id, state, pid, parent_agent, depth, run_id,
			started_at, last_activity, escalation_level,
			transcript_path, runtime, heartbeat_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		sessionID, agentName, capability, "", "",
		taskID, state, 0, "", 0, nil,
		now, now, 0,
		"", runtime, now,
	)
	if err != nil {
		t.Fatalf("seed session %s: %v", sessionID, err)
	}
}

func TestRegisterCmd(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	sessionID := generateSessionID("pi")
	seedSession(t, app, sessionID, "test-agent", "builder", "TEST-001", "booting", "pi")

	// Verify session exists.
	var state string
	row := app.DB.QueryRow(ctx, "SELECT state FROM sessions WHERE id = $1", sessionID)
	if err := row.Scan(&state); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if state != "booting" {
		t.Errorf("expected state=booting, got %q", state)
	}
}

func TestDeregisterCmd(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	sessionID := generateSessionID("pi")
	seedSession(t, app, sessionID, "test-deregister", "builder", "TEST-002", "working", "pi")

	// Deregister.
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	err := app.DB.Exec(ctx,
		"UPDATE sessions SET state = $1, last_activity = $2 WHERE id = $3",
		"completed", now, sessionID,
	)
	if err != nil {
		t.Fatalf("deregister: %v", err)
	}

	var state string
	row := app.DB.QueryRow(ctx, "SELECT state FROM sessions WHERE id = $1", sessionID)
	if err := row.Scan(&state); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if state != "completed" {
		t.Errorf("expected state=completed, got %q", state)
	}
}

func TestHeartbeatCmd(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	sessionID := generateSessionID("gemini")
	seedSession(t, app, sessionID, "test-heartbeat", "scout", "TEST-003", "working", "gemini")

	// Heartbeat with state update.
	newTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	err := app.DB.Exec(ctx,
		"UPDATE sessions SET heartbeat_at = $1, last_activity = $2, state = $3 WHERE id = $4",
		newTime, newTime, "working", sessionID,
	)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	var heartbeatAt string
	row := app.DB.QueryRow(ctx, "SELECT heartbeat_at FROM sessions WHERE id = $1", sessionID)
	if err := row.Scan(&heartbeatAt); err != nil {
		t.Fatalf("query heartbeat: %v", err)
	}
	if heartbeatAt == "" {
		t.Error("expected non-empty heartbeat_at")
	}
}

// newTestRootCmd creates a minimal root command with --json flag and required groups.
func newTestRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "cmdr",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", true, "JSON output")
	root.AddGroup(
		&cobra.Group{ID: "CORE", Title: "Core Commands:"},
		&cobra.Group{ID: "OBSERVABILITY", Title: "Observability:"},
	)
	return root
}

func TestRegisterCmdViaCommand(t *testing.T) {
	app := testApp(t)

	cmd := RegisterCmd(app)
	rootCmd := newTestRootCmd(app)
	rootCmd.AddCommand(cmd)

	rootCmd.SetArgs([]string{"register",
		"--name", "pi-coder",
		"--runtime", "pi",
		"--capability", "builder",
		"--task", "TEST-CMD",
		"--json",
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("register command: %v", err)
	}
}

func TestDeregisterCmdViaCommand(t *testing.T) {
	app := testApp(t)

	sessionID := "pi-test1234"
	seedSession(t, app, sessionID, "test", "builder", "TEST", "working", "pi")

	cmd := DeregisterCmd(app)
	rootCmd := newTestRootCmd(app)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{"deregister", sessionID, "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("deregister command: %v", err)
	}
}

func TestHeartbeatCmdViaCommand(t *testing.T) {
	app := testApp(t)

	sessionID := "gemini-test5678"
	seedSession(t, app, sessionID, "test", "scout", "TEST", "working", "gemini")

	cmd := HeartbeatCmd(app)
	rootCmd := newTestRootCmd(app)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{"heartbeat", sessionID, "--state", "working", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("heartbeat command: %v", err)
	}
}

func TestGenerateSessionID(t *testing.T) {
	id := generateSessionID("pi")
	if id == "" {
		t.Error("expected non-empty session ID")
	}
	if len(id) < 4 {
		t.Errorf("session ID too short: %q", id)
	}
	if id[:3] != "pi-" {
		t.Errorf("expected pi- prefix, got %q", id[:3])
	}
}

func TestEmitAgentEvent(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Seed a session so the FK on events.session_id is satisfied.
	sessionID := "test-emit-sess"
	seedSession(t, app, sessionID, "test-agent", "builder", "T1", "working", "pi")

	emitAgentEvent(app, "test-agent", sessionID, "agent.registered", "runtime=pi capability=builder")

	var eventType, agentName string
	row := app.DB.QueryRow(ctx, "SELECT event_type, agent_name FROM events WHERE session_id = $1", sessionID)
	if err := row.Scan(&eventType, &agentName); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if eventType != "agent.registered" {
		t.Errorf("expected event_type=agent.registered, got %q", eventType)
	}
	if agentName != "test-agent" {
		t.Errorf("expected agent_name=test-agent, got %q", agentName)
	}
}

func TestRegisterJSONOutput(t *testing.T) {
	result := map[string]any{
		"success":    true,
		"command":    "register",
		"session_id": "pi-a3f8c1e2",
		"agent_name": "unix-coder",
		"runtime":    "pi",
		"state":      "booting",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["success"] != true {
		t.Errorf("expected success=true, got %v", parsed["success"])
	}
	if parsed["command"] != "register" {
		t.Errorf("expected command=register, got %v", parsed["command"])
	}
}
