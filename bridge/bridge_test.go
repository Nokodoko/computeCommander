package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Manifest loading
// ---------------------------------------------------------------------------

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	manifest := HookManifest{
		Version: 1,
		Hooks: []HookBinding{
			{
				Name:         "cmdr-bridge",
				GoPackage:    "github.com/noko/computecommander/hooks/cmdr",
				PiEvents:     []string{"session_start", "agent_start"},
				ClaudeEvents: []string{"SubagentStart", "SessionStart"},
			},
		},
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if len(got.Hooks) != 1 {
		t.Fatalf("hooks count = %d, want 1", len(got.Hooks))
	}
	if got.Hooks[0].Name != "cmdr-bridge" {
		t.Errorf("hook name = %q, want %q", got.Hooks[0].Name, "cmdr-bridge")
	}
}

func TestLoadManifest_BadVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	data := []byte(`{"version": 0, "hooks": []}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for version 0")
	}
}

func TestLoadManifest_EmptyHookName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	data := []byte(`{"version": 1, "hooks": [{"name": "", "goPackage": "x"}]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for empty hook name")
	}
}

func TestLoadManifest_MissingFile(t *testing.T) {
	_, err := LoadManifest("/nonexistent/path/manifest.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadManifest_BadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	if err := os.WriteFile(path, []byte("{bad json}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

// ---------------------------------------------------------------------------
// FindBinding
// ---------------------------------------------------------------------------

func TestFindBinding_Found(t *testing.T) {
	m := &HookManifest{
		Version: 1,
		Hooks: []HookBinding{
			{Name: "alpha"},
			{Name: "beta"},
		},
	}

	b, err := FindBinding(m, "beta")
	if err != nil {
		t.Fatalf("FindBinding: %v", err)
	}
	if b.Name != "beta" {
		t.Errorf("binding name = %q, want %q", b.Name, "beta")
	}
}

func TestFindBinding_NotFound(t *testing.T) {
	m := &HookManifest{
		Version: 1,
		Hooks:   []HookBinding{{Name: "alpha"}},
	}

	_, err := FindBinding(m, "gamma")
	if err == nil {
		t.Fatal("expected error for missing binding")
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()

	called := false
	handler := func(req *BridgeRequest) (*BridgeResponse, error) {
		called = true
		return &BridgeResponse{Success: true}, nil
	}

	if err := r.Register("test-hook", handler); err != nil {
		t.Fatalf("Register: %v", err)
	}

	h := r.Get("test-hook")
	if h == nil {
		t.Fatal("Get returned nil for registered hook")
	}

	resp, err := h(&BridgeRequest{Hook: "test-hook"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	noop := func(req *BridgeRequest) (*BridgeResponse, error) {
		return &BridgeResponse{Success: true}, nil
	}

	if err := r.Register("dup", noop); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register("dup", noop); err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestRegistry_GetUnregistered(t *testing.T) {
	r := NewRegistry()
	if h := r.Get("nope"); h != nil {
		t.Error("expected nil for unregistered hook")
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	noop := func(req *BridgeRequest) (*BridgeResponse, error) {
		return &BridgeResponse{Success: true}, nil
	}

	_ = r.Register("a", noop)
	_ = r.Register("b", noop)

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("names count = %d, want 2", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["a"] || !nameSet["b"] {
		t.Errorf("names = %v, want [a b]", names)
	}
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func TestDispatch_WithHandler(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("echo", func(req *BridgeRequest) (*BridgeResponse, error) {
		return &BridgeResponse{
			Success: true,
			Output:  req.Payload,
		}, nil
	})

	binding := &HookBinding{Name: "echo"}
	req := &BridgeRequest{
		Hook:    "echo",
		Event:   "test",
		Payload: json.RawMessage(`{"key":"value"}`),
	}

	resp, err := Dispatch(r, binding, req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !resp.Success {
		t.Errorf("success = false, want true")
	}
	if string(resp.Output) != `{"key":"value"}` {
		t.Errorf("output = %s, want %s", resp.Output, `{"key":"value"}`)
	}
}

func TestDispatch_NoHandler(t *testing.T) {
	r := NewRegistry()
	binding := &HookBinding{Name: "missing"}
	req := &BridgeRequest{Hook: "missing"}

	resp, err := Dispatch(r, binding, req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for missing handler")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestDispatch_HandlerError(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("fail", func(req *BridgeRequest) (*BridgeResponse, error) {
		return nil, fmt.Errorf("intentional failure")
	})

	binding := &HookBinding{Name: "fail"}
	req := &BridgeRequest{Hook: "fail"}

	resp, err := Dispatch(r, binding, req)
	if err != nil {
		t.Fatalf("Dispatch should not return error: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for handler error")
	}
	if resp.Error != "intentional failure" {
		t.Errorf("error = %q, want %q", resp.Error, "intentional failure")
	}
}

// ---------------------------------------------------------------------------
// ValidateManifest
// ---------------------------------------------------------------------------

func TestValidateManifest_AllPresent(t *testing.T) {
	r := NewRegistry()
	noop := func(req *BridgeRequest) (*BridgeResponse, error) {
		return &BridgeResponse{Success: true}, nil
	}
	_ = r.Register("a", noop)
	_ = r.Register("b", noop)

	m := &HookManifest{
		Version: 1,
		Hooks: []HookBinding{
			{Name: "a"},
			{Name: "b"},
		},
	}

	missing := ValidateManifest(m, r)
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestValidateManifest_SomeMissing(t *testing.T) {
	r := NewRegistry()
	noop := func(req *BridgeRequest) (*BridgeResponse, error) {
		return &BridgeResponse{Success: true}, nil
	}
	_ = r.Register("a", noop)

	m := &HookManifest{
		Version: 1,
		Hooks: []HookBinding{
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		},
	}

	missing := ValidateManifest(m, r)
	if len(missing) != 2 {
		t.Fatalf("missing count = %d, want 2", len(missing))
	}
}
