// Package bridge provides the Go-TypeScript translation layer for computeCommander.
//
// It loads a hook manifest (JSON) that maps Go hook implementations to Pi agent
// events and Claude Code events, then dispatches incoming requests to the
// appropriate Go handler. The CLI cmd/hook-bridge consumes this package.
package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ---------------------------------------------------------------------------
// Manifest types
// ---------------------------------------------------------------------------

// HookManifest is the top-level manifest that lists all hook bindings.
// bridge:export
type HookManifest struct {
	Version int           `json:"version"`
	Hooks   []HookBinding `json:"hooks"`
}

// HookBinding maps a named Go hook to the events it handles across runtimes.
// bridge:export
type HookBinding struct {
	Name         string   `json:"name"`
	GoPackage    string   `json:"goPackage"`
	PiEvents     []string `json:"piEvents"`
	ClaudeEvents []string `json:"claudeEvents"`
	Matcher      string   `json:"matcher,omitempty"`
	InputSchema  string   `json:"inputSchema,omitempty"`
	OutputSchema string   `json:"outputSchema,omitempty"`
}

// ---------------------------------------------------------------------------
// Request / Response protocol  (stdin/stdout JSON)
// ---------------------------------------------------------------------------

// BridgeRequest is the JSON envelope sent to hook-bridge on stdin.
// bridge:export
type BridgeRequest struct {
	Hook      string          `json:"hook"`
	Event     string          `json:"event"`
	Payload   json.RawMessage `json:"payload"`
	SessionID string          `json:"sessionId,omitempty"`
}

// BridgeResponse is the JSON envelope written to stdout by hook-bridge.
// bridge:export
type BridgeResponse struct {
	Success bool            `json:"success"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
	Context string          `json:"context,omitempty"`
}

// ---------------------------------------------------------------------------
// Handler interface and Registry
// ---------------------------------------------------------------------------

// HookHandler is the function signature that Go hook implementations must satisfy.
type HookHandler func(req *BridgeRequest) (*BridgeResponse, error)

// Registry maps hook names to their Go handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]HookHandler
}

// NewRegistry returns an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]HookHandler),
	}
}

// Register adds a handler for the given hook name.
// It returns an error if a handler is already registered for that name.
func (r *Registry) Register(name string, h HookHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("handler already registered for hook %q", name)
	}
	r.handlers[name] = h
	return nil
}

// Get returns the handler for the given hook name, or nil if none is registered.
func (r *Registry) Get(name string) HookHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[name]
}

// Names returns all registered hook names in no particular order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.handlers))
	for n := range r.handlers {
		names = append(names, n)
	}
	return names
}

// ---------------------------------------------------------------------------
// Manifest loading
// ---------------------------------------------------------------------------

// LoadManifest reads and validates a manifest JSON file.
func LoadManifest(path string) (*HookManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m HookManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if m.Version < 1 {
		return nil, fmt.Errorf("unsupported manifest version: %d", m.Version)
	}

	for i, h := range m.Hooks {
		if h.Name == "" {
			return nil, fmt.Errorf("hook at index %d has empty name", i)
		}
	}

	return &m, nil
}

// ---------------------------------------------------------------------------
// Binding lookup
// ---------------------------------------------------------------------------

// FindBinding returns the first HookBinding whose Name matches hookName.
func FindBinding(manifest *HookManifest, hookName string) (*HookBinding, error) {
	for i := range manifest.Hooks {
		if manifest.Hooks[i].Name == hookName {
			return &manifest.Hooks[i], nil
		}
	}
	return nil, fmt.Errorf("no binding found for hook %q", hookName)
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

// Dispatch looks up the handler for the binding's hook name and calls it.
func Dispatch(registry *Registry, binding *HookBinding, req *BridgeRequest) (*BridgeResponse, error) {
	handler := registry.Get(binding.Name)
	if handler == nil {
		return &BridgeResponse{
			Success: false,
			Error:   fmt.Sprintf("no handler registered for hook %q", binding.Name),
		}, nil
	}

	resp, err := handler(req)
	if err != nil {
		return &BridgeResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return resp, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidateManifest checks that every hook binding in the manifest has a
// corresponding handler registered in the registry. Returns a list of
// unregistered hook names (empty slice means all bindings are satisfied).
func ValidateManifest(manifest *HookManifest, registry *Registry) []string {
	var missing []string
	for _, h := range manifest.Hooks {
		if registry.Get(h.Name) == nil {
			missing = append(missing, h.Name)
		}
	}
	return missing
}
