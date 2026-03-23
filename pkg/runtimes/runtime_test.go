package runtimes_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noko/computecommander/pkg/runtimes"

	// Blank imports trigger init() registration for each runtime.
	_ "github.com/noko/computecommander/pkg/runtimes/claude"
	_ "github.com/noko/computecommander/pkg/runtimes/codex"
	_ "github.com/noko/computecommander/pkg/runtimes/gemini"
	_ "github.com/noko/computecommander/pkg/runtimes/goose"
	_ "github.com/noko/computecommander/pkg/runtimes/pi"
)

func TestGetRuntimeClaude(t *testing.T) {
	rt, err := runtimes.GetRuntime("claude")
	if err != nil {
		t.Fatalf("GetRuntime(\"claude\"): %v", err)
	}
	if rt.ID() != runtimes.RuntimeClaude {
		t.Errorf("ID() = %q, want %q", rt.ID(), runtimes.RuntimeClaude)
	}
}

func TestGetRuntimeAll(t *testing.T) {
	ids := []string{"claude", "gemini", "codex", "pi", "goose"}
	for _, id := range ids {
		rt, err := runtimes.GetRuntime(id)
		if err != nil {
			t.Errorf("GetRuntime(%q): %v", id, err)
			continue
		}
		if string(rt.ID()) != id {
			t.Errorf("ID() = %q, want %q", rt.ID(), id)
		}
	}
}

func TestGetRuntimeUnknown(t *testing.T) {
	_, err := runtimes.GetRuntime("nonexistent")
	if err == nil {
		t.Fatal("GetRuntime(\"nonexistent\") should return error")
	}
	if !strings.Contains(err.Error(), "unknown runtime") {
		t.Errorf("error = %q, want it to contain \"unknown runtime\"", err.Error())
	}
}

func TestClaudeInstructionPath(t *testing.T) {
	rt, _ := runtimes.GetRuntime("claude")
	if got := rt.InstructionPath(); got != ".claude/CLAUDE.md" {
		t.Errorf("InstructionPath() = %q, want %q", got, ".claude/CLAUDE.md")
	}
}

func TestClaudeBuildSpawnCommand(t *testing.T) {
	rt, _ := runtimes.GetRuntime("claude")

	tt := []struct {
		name string
		opts runtimes.SpawnOpts
		want string
	}{
		{
			name: "bypass mode with model and prompt file",
			opts: runtimes.SpawnOpts{
				Model:          "claude-sonnet-4",
				PermissionMode: "bypass",
				PromptFile:     "/tmp/prompt.md",
			},
			want: "claude --dangerously-skip-permissions -p --model claude-sonnet-4 < /tmp/prompt.md",
		},
		{
			name: "ask mode with system prompt",
			opts: runtimes.SpawnOpts{
				Model:          "claude-opus-4",
				PermissionMode: "ask",
				SystemPrompt:   "You are a coder",
			},
			want: "claude -p --model claude-opus-4 'You are a coder'",
		},
		{
			name: "minimal opts",
			opts: runtimes.SpawnOpts{},
			want: "claude -p",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := rt.BuildSpawnCommand(tc.opts)
			if got != tc.want {
				t.Errorf("BuildSpawnCommand() =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

func TestClaudeBuildPrintCommand(t *testing.T) {
	rt, _ := runtimes.GetRuntime("claude")
	got := rt.BuildPrintCommand("fix the bug", "claude-sonnet-4")
	expected := []string{"claude", "-p", "--model", "claude-sonnet-4", "fix the bug"}
	if len(got) != len(expected) {
		t.Fatalf("BuildPrintCommand() returned %d args, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestClaudeDeployConfig(t *testing.T) {
	tmpDir := t.TempDir()

	rt, _ := runtimes.GetRuntime("claude")

	overlay := &runtimes.OverlayContent{Content: "# Test Instructions\nDo the thing."}
	hooks := &runtimes.HooksDef{
		AgentName: "test-agent",
		Rules: &runtimes.HookRules{
			AllowedTools: []string{"Read", "Write"},
			DeniedTools:  []string{"Bash"},
		},
	}

	err := rt.DeployConfig(context.Background(), tmpDir, overlay, hooks)
	if err != nil {
		t.Fatalf("DeployConfig: %v", err)
	}

	// Verify instruction file.
	instrPath := filepath.Join(tmpDir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(instrPath)
	if err != nil {
		t.Fatalf("read instruction file: %v", err)
	}
	if string(data) != overlay.Content {
		t.Errorf("instruction content = %q, want %q", string(data), overlay.Content)
	}

	// Verify settings file exists.
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.local.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
}

func TestClaudeDetectReady(t *testing.T) {
	rt, _ := runtimes.GetRuntime("claude")

	tt := []struct {
		name    string
		content string
		phase   string
	}{
		{"ready with prompt", "some output\n> ", "ready"},
		{"ready with box", "some output\n\u256d\u2500", "ready"},
		{"loading", "Loading Claude...", "loading"},
		{"dialog allow", "Do you want to Allow this?", "dialog"},
		{"dialog trust", "Trust this project?", "dialog"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			state := rt.DetectReady(tc.content)
			if state.Phase != tc.phase {
				t.Errorf("DetectReady() phase = %q, want %q", state.Phase, tc.phase)
			}
		})
	}
}

func TestClaudeRequiresBeacon(t *testing.T) {
	rt, _ := runtimes.GetRuntime("claude")
	if !rt.RequiresBeaconVerification() {
		t.Error("Claude should require beacon verification")
	}
}

func TestClaudeConnect(t *testing.T) {
	rt, _ := runtimes.GetRuntime("claude")
	if conn := rt.Connect(nil); conn != nil {
		t.Error("Claude Connect() should return nil")
	}
}

func TestGeminiInstructionPath(t *testing.T) {
	rt, _ := runtimes.GetRuntime("gemini")
	if got := rt.InstructionPath(); got != ".gemini/GEMINI.md" {
		t.Errorf("InstructionPath() = %q, want %q", got, ".gemini/GEMINI.md")
	}
}

func TestCodexInstructionPath(t *testing.T) {
	rt, _ := runtimes.GetRuntime("codex")
	if got := rt.InstructionPath(); got != "AGENTS.md" {
		t.Errorf("InstructionPath() = %q, want %q", got, "AGENTS.md")
	}
}

func TestPiInstructionPath(t *testing.T) {
	rt, _ := runtimes.GetRuntime("pi")
	if got := rt.InstructionPath(); got != ".claude/CLAUDE.md" {
		t.Errorf("InstructionPath() = %q, want %q", got, ".claude/CLAUDE.md")
	}
}

func TestPiBuildSpawnCommand(t *testing.T) {
	rt, _ := runtimes.GetRuntime("pi")

	tt := []struct {
		name string
		opts runtimes.SpawnOpts
		want string
	}{
		{
			name: "bypass mode with model and prompt file",
			opts: runtimes.SpawnOpts{
				Model:          "gemini-2.5-pro",
				PermissionMode: "bypass",
				PromptFile:     "/tmp/prompt.md",
			},
			want: "pi --no-extensions -p --model gemini-2.5-pro < /tmp/prompt.md",
		},
		{
			name: "ask mode with system prompt",
			opts: runtimes.SpawnOpts{
				Model:          "claude-sonnet-4",
				PermissionMode: "ask",
				SystemPrompt:   "You are a coder",
			},
			want: "pi -p --model claude-sonnet-4 'You are a coder'",
		},
		{
			name: "minimal opts",
			opts: runtimes.SpawnOpts{},
			want: "pi -p",
		},
		{
			name: "with append prompt",
			opts: runtimes.SpawnOpts{
				Model:        "gemini-2.5-pro",
				AppendPrompt: "extra context",
			},
			want: "pi -p --model gemini-2.5-pro --append-system-prompt 'extra context'",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := rt.BuildSpawnCommand(tc.opts)
			if got != tc.want {
				t.Errorf("BuildSpawnCommand() =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

func TestPiBuildPrintCommand(t *testing.T) {
	rt, _ := runtimes.GetRuntime("pi")
	got := rt.BuildPrintCommand("fix the bug", "gemini-2.5-pro")
	expected := []string{"pi", "-p", "--model", "gemini-2.5-pro", "fix the bug"}
	if len(got) != len(expected) {
		t.Fatalf("BuildPrintCommand() returned %d args, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestPiDeployConfig(t *testing.T) {
	tmpDir := t.TempDir()

	rt, _ := runtimes.GetRuntime("pi")

	overlay := &runtimes.OverlayContent{Content: "# Test Instructions\nDo the thing."}
	hooks := &runtimes.HooksDef{
		AgentName: "test-agent",
		Rules: &runtimes.HookRules{
			AllowedTools: []string{"Read", "Write"},
			DeniedTools:  []string{"Bash"},
		},
	}

	err := rt.DeployConfig(context.Background(), tmpDir, overlay, hooks)
	if err != nil {
		t.Fatalf("DeployConfig: %v", err)
	}

	// Verify instruction file (shared .claude/CLAUDE.md path).
	instrPath := filepath.Join(tmpDir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(instrPath)
	if err != nil {
		t.Fatalf("read instruction file: %v", err)
	}
	if string(data) != overlay.Content {
		t.Errorf("instruction content = %q, want %q", string(data), overlay.Content)
	}

	// Verify Pi settings directory was created.
	piSettingsPath := filepath.Join(tmpDir, ".pi", "agent", "settings.json")
	if _, err := os.Stat(piSettingsPath); err != nil {
		t.Fatalf("pi settings file missing: %v", err)
	}
}

func TestPiDetectReady(t *testing.T) {
	rt, _ := runtimes.GetRuntime("pi")

	tt := []struct {
		name    string
		content string
		phase   string
	}{
		{"ready with prompt", "some output\n> ", "ready"},
		{"ready with box", "some output\n\u256d\u2500", "ready"},
		{"loading", "Loading Pi...", "loading"},
		{"dialog allow", "Do you want to Allow this?", "dialog"},
		{"dialog trust", "Trust this extension?", "dialog"},
		{"dialog approve", "Approve this action?", "dialog"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			state := rt.DetectReady(tc.content)
			if state.Phase != tc.phase {
				t.Errorf("DetectReady() phase = %q, want %q", state.Phase, tc.phase)
			}
		})
	}
}

func TestPiRequiresBeacon(t *testing.T) {
	rt, _ := runtimes.GetRuntime("pi")
	if rt.RequiresBeaconVerification() {
		t.Error("Pi should not require beacon verification (uses JSON-RPC)")
	}
}

func TestPiConnect(t *testing.T) {
	rt, _ := runtimes.GetRuntime("pi")
	if conn := rt.Connect(nil); conn != nil {
		t.Error("Pi Connect() should return nil (RPC not yet implemented)")
	}
}

func TestGooseInstructionPath(t *testing.T) {
	rt, _ := runtimes.GetRuntime("goose")
	if got := rt.InstructionPath(); got != ".goose/instructions.md" {
		t.Errorf("InstructionPath() = %q, want %q", got, ".goose/instructions.md")
	}
}

func TestAllRuntimeIDs(t *testing.T) {
	ids := runtimes.AllRuntimeIDs()
	if len(ids) != 5 {
		t.Fatalf("AllRuntimeIDs() returned %d, want 5", len(ids))
	}
	expected := map[runtimes.RuntimeID]bool{
		runtimes.RuntimeClaude: true,
		runtimes.RuntimeGemini: true,
		runtimes.RuntimeCodex:  true,
		runtimes.RuntimePi:     true,
		runtimes.RuntimeGoose:  true,
	}
	for _, id := range ids {
		if !expected[id] {
			t.Errorf("unexpected ID %q in AllRuntimeIDs()", id)
		}
	}
}

func TestCodexHeadlessReady(t *testing.T) {
	rt, _ := runtimes.GetRuntime("codex")
	state := rt.DetectReady("anything")
	if state.Phase != "ready" {
		t.Errorf("Codex DetectReady() phase = %q, want \"ready\"", state.Phase)
	}
}

func TestStubRuntimesDoNotRequireBeacon(t *testing.T) {
	for _, id := range []string{"codex", "pi"} {
		rt, _ := runtimes.GetRuntime(id)
		if rt.RequiresBeaconVerification() {
			t.Errorf("%s should not require beacon verification", id)
		}
	}
}

func TestRegisterRuntime(t *testing.T) {
	// Verify re-registration overwrites.
	rt, _ := runtimes.GetRuntime("claude")
	runtimes.RegisterRuntime(rt)
	rt2, err := runtimes.GetRuntime("claude")
	if err != nil {
		t.Fatalf("GetRuntime after re-register: %v", err)
	}
	if rt2.ID() != runtimes.RuntimeClaude {
		t.Errorf("ID after re-register = %q, want %q", rt2.ID(), runtimes.RuntimeClaude)
	}
}
