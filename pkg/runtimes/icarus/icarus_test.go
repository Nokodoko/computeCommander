package icarus_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noko/computecommander/pkg/runtimes"
	"github.com/noko/computecommander/pkg/runtimes/icarus"
)

func TestIDConstantsAgree(t *testing.T) {
	if icarus.RuntimeIcarus != string(runtimes.RuntimeIcarus) {
		t.Errorf("icarus.RuntimeIcarus = %q, want %q",
			icarus.RuntimeIcarus, runtimes.RuntimeIcarus)
	}
}

func TestRegistration(t *testing.T) {
	rt, err := runtimes.GetRuntime("icarus")
	if err != nil {
		t.Fatalf("GetRuntime(\"icarus\"): %v", err)
	}
	if rt.ID() != runtimes.RuntimeIcarus {
		t.Errorf("ID() = %q, want %q", rt.ID(), runtimes.RuntimeIcarus)
	}
}

func TestNew(t *testing.T) {
	r := icarus.New()
	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.ID() != runtimes.RuntimeIcarus {
		t.Errorf("ID() = %q, want %q", r.ID(), runtimes.RuntimeIcarus)
	}
}

func TestInstructionPath(t *testing.T) {
	r := icarus.New()
	if got := r.InstructionPath(); got != ".claude/CLAUDE.md" {
		t.Errorf("InstructionPath() = %q, want %q", got, ".claude/CLAUDE.md")
	}
}

func TestBuildSpawnCommand(t *testing.T) {
	r := icarus.New()

	// Ensure the parent-process env doesn't leak into per-test results.
	t.Setenv("ICARUS_EFFORT", "")

	tt := []struct {
		name string
		opts runtimes.SpawnOpts
		want string
	}{
		{
			name: "minimal opts",
			opts: runtimes.SpawnOpts{},
			want: "icarus run",
		},
		{
			name: "model only",
			opts: runtimes.SpawnOpts{Model: "claude-sonnet-4-7"},
			want: "icarus run --model claude-sonnet-4-7",
		},
		{
			name: "model with prompt file",
			opts: runtimes.SpawnOpts{
				Model:      "claude-sonnet-4-7",
				PromptFile: "/tmp/prompt.md",
			},
			want: "icarus run --model claude-sonnet-4-7 < /tmp/prompt.md",
		},
		{
			name: "model with system prompt",
			opts: runtimes.SpawnOpts{
				Model:        "gpt-5-codex",
				SystemPrompt: "You are a coder",
			},
			want: "icarus run --model gpt-5-codex 'You are a coder'",
		},
		{
			name: "effort pass-through via env",
			opts: runtimes.SpawnOpts{
				Model: "claude-sonnet-4-7",
				Env:   map[string]string{"ICARUS_EFFORT": "high"},
			},
			want: "icarus run --model claude-sonnet-4-7 --effort high",
		},
		{
			name: "all effort levels: xhigh",
			opts: runtimes.SpawnOpts{
				Env: map[string]string{"ICARUS_EFFORT": "xhigh"},
			},
			want: "icarus run --effort xhigh",
		},
		{
			name: "invalid effort is dropped",
			opts: runtimes.SpawnOpts{
				Env: map[string]string{"ICARUS_EFFORT": "ludicrous"},
			},
			want: "icarus run",
		},
		{
			name: "append prompt is shell-quoted",
			opts: runtimes.SpawnOpts{
				AppendPrompt: "extra context",
			},
			want: "icarus run --append-system-prompt 'extra context'",
		},
		{
			name: "prompt file wins over system prompt",
			opts: runtimes.SpawnOpts{
				PromptFile:   "/tmp/p.md",
				SystemPrompt: "ignored",
			},
			want: "icarus run < /tmp/p.md",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := r.BuildSpawnCommand(tc.opts)
			if got != tc.want {
				t.Errorf("BuildSpawnCommand() =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

func TestBuildSpawnCommandEffortFromParentEnv(t *testing.T) {
	r := icarus.New()
	t.Setenv("ICARUS_EFFORT", "medium")
	got := r.BuildSpawnCommand(runtimes.SpawnOpts{Model: "m"})
	want := "icarus run --model m --effort medium"
	if got != want {
		t.Errorf("BuildSpawnCommand() =\n  %q\nwant\n  %q", got, want)
	}
}

func TestBuildSpawnCommandPerSpawnEnvOverridesParent(t *testing.T) {
	r := icarus.New()
	t.Setenv("ICARUS_EFFORT", "low")
	got := r.BuildSpawnCommand(runtimes.SpawnOpts{
		Env: map[string]string{"ICARUS_EFFORT": "xhigh"},
	})
	want := "icarus run --effort xhigh"
	if got != want {
		t.Errorf("BuildSpawnCommand() =\n  %q\nwant\n  %q", got, want)
	}
}

func TestBuildPrintCommand(t *testing.T) {
	r := icarus.New()
	got := r.BuildPrintCommand("fix the bug", "claude-sonnet-4-7")
	expected := []string{"icarus", "run", "--model", "claude-sonnet-4-7", "fix the bug"}
	if len(got) != len(expected) {
		t.Fatalf("BuildPrintCommand() returned %d args, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestBuildPrintCommandNoModel(t *testing.T) {
	r := icarus.New()
	got := r.BuildPrintCommand("hello", "")
	expected := []string{"icarus", "run", "hello"}
	if len(got) != len(expected) {
		t.Fatalf("BuildPrintCommand() returned %d args, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestDeployConfigInstructionFile(t *testing.T) {
	tmpDir := t.TempDir()
	r := icarus.New()

	overlay := &runtimes.OverlayContent{Content: "# Test Instructions\nDo the thing."}
	if err := r.DeployConfig(context.Background(), tmpDir, overlay, nil); err != nil {
		t.Fatalf("DeployConfig: %v", err)
	}

	instrPath := filepath.Join(tmpDir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(instrPath)
	if err != nil {
		t.Fatalf("read instruction file: %v", err)
	}
	if string(data) != overlay.Content {
		t.Errorf("instruction content = %q, want %q", string(data), overlay.Content)
	}

	// With nil hooks, no .icarus dir should be created.
	if _, err := os.Stat(filepath.Join(tmpDir, ".icarus")); !os.IsNotExist(err) {
		t.Errorf(".icarus dir should not exist when hooks are nil; got err=%v", err)
	}
}

func TestDeployConfigWithHooks(t *testing.T) {
	tmpDir := t.TempDir()
	r := icarus.New()

	overlay := &runtimes.OverlayContent{Content: "# Coder"}
	hooks := &runtimes.HooksDef{
		AgentName: "test-agent",
		Rules: &runtimes.HookRules{
			AllowedTools: []string{"Read", "Write"},
			DeniedTools:  []string{"WebFetch"},
		},
	}

	if err := r.DeployConfig(context.Background(), tmpDir, overlay, hooks); err != nil {
		t.Fatalf("DeployConfig: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".icarus", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read icarus settings: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse icarus settings: %v", err)
	}
	ob1, ok := parsed["ob1"].(map[string]any)
	if !ok {
		t.Fatalf("ob1 block missing from settings.json: %s", data)
	}
	if got, _ := ob1["api_key_env"].(string); got != "OB_API_KEY" {
		t.Errorf("ob1.api_key_env = %q, want %q", got, "OB_API_KEY")
	}
}

func TestDetectReady(t *testing.T) {
	r := icarus.New()

	tt := []struct {
		name    string
		content string
		phase   string
	}{
		{"ready with prompt", "some output\n> ", "ready"},
		{"ready with box", "some output\n\u256d\u2500", "ready"},
		{"loading", "Loading icarus...", "loading"},
		{"dialog allow", "Do you want to Allow this?", "dialog"},
		{"dialog trust", "Trust this project?", "dialog"},
		{"dialog approve", "Approve this action?", "dialog"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			state := r.DetectReady(tc.content)
			if state.Phase != tc.phase {
				t.Errorf("DetectReady() phase = %q, want %q", state.Phase, tc.phase)
			}
		})
	}
}

func TestParseTranscript(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "session.jsonl")

	lines := []string{
		`{"model":"claude-sonnet-4-7","usage":{"input_tokens":100,"output_tokens":50}}`,
		`{"usage":{"input_tokens":25,"output_tokens":10}}`,
		``,
		`not-json should be skipped`,
		`{"model":"claude-sonnet-4-7","usage":{"input_tokens":7,"output_tokens":3}}`,
	}
	if err := os.WriteFile(transcriptPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	r := icarus.New()
	got, err := r.ParseTranscript(transcriptPath)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}

	if got.Model != "claude-sonnet-4-7" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-sonnet-4-7")
	}
	if got.InputTokens != 132 {
		t.Errorf("InputTokens = %d, want %d", got.InputTokens, 132)
	}
	if got.OutputTokens != 63 {
		t.Errorf("OutputTokens = %d, want %d", got.OutputTokens, 63)
	}
}

func TestParseTranscriptMissingFile(t *testing.T) {
	r := icarus.New()
	_, err := r.ParseTranscript("/nonexistent/path/session.jsonl")
	if err == nil {
		t.Fatal("expected error for missing transcript file")
	}
}

func TestBuildEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("OB_API_KEY", "ob-test")
	t.Setenv("ICARUS_HOME", "/tmp/icarus-home")
	// Explicitly clear keys we expect NOT to leak.
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	r := icarus.New()
	env := r.BuildEnv("claude-sonnet-4-7")

	if env["ANTHROPIC_API_KEY"] != "sk-ant-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", env["ANTHROPIC_API_KEY"], "sk-ant-test")
	}
	if env["OB_API_KEY"] != "ob-test" {
		t.Errorf("OB_API_KEY = %q, want %q", env["OB_API_KEY"], "ob-test")
	}
	if env["ICARUS_HOME"] != "/tmp/icarus-home" {
		t.Errorf("ICARUS_HOME = %q, want %q", env["ICARUS_HOME"], "/tmp/icarus-home")
	}
	if env["ICARUS_MODEL"] != "claude-sonnet-4-7" {
		t.Errorf("ICARUS_MODEL = %q, want %q", env["ICARUS_MODEL"], "claude-sonnet-4-7")
	}
	if _, ok := env["OPENAI_API_KEY"]; ok {
		t.Errorf("OPENAI_API_KEY should not be present when env is empty")
	}
	if _, ok := env["GOOGLE_API_KEY"]; ok {
		t.Errorf("GOOGLE_API_KEY should not be present when env is empty")
	}
}

func TestBuildEnvNoModel(t *testing.T) {
	r := icarus.New()
	env := r.BuildEnv("")
	if _, ok := env["ICARUS_MODEL"]; ok {
		t.Error("ICARUS_MODEL should not be set when model is empty")
	}
}

func TestRequiresBeaconVerification(t *testing.T) {
	r := icarus.New()
	if r.RequiresBeaconVerification() {
		t.Error("Icarus should NOT require beacon verification (uses TypedBus events)")
	}
}

func TestConnect(t *testing.T) {
	r := icarus.New()
	if conn := r.Connect(nil); conn != nil {
		t.Error("Icarus Connect() should return nil (RPC not implemented)")
	}
}

func TestAllRuntimeIDsContainsIcarus(t *testing.T) {
	ids := runtimes.AllRuntimeIDs()
	for _, id := range ids {
		if id == runtimes.RuntimeIcarus {
			return
		}
	}
	t.Errorf("AllRuntimeIDs() does not contain RuntimeIcarus; got %v", ids)
}
