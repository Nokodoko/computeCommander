package keybinds

import "fmt"

// ActionHandler is a function that executes a keybind action.
type ActionHandler func() error

// Registry maps action names to their handlers.
// This is a data-driven, plugin-ready design: new commands and keybinds
// can register at runtime without modifying switch statements.
type Registry struct {
	handlers    map[string]ActionHandler
	descriptions map[string]string
}

// NewRegistry creates an empty action registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers:    make(map[string]ActionHandler),
		descriptions: make(map[string]string),
	}
}

// Register adds an action handler to the registry.
func (r *Registry) Register(action string, description string, handler ActionHandler) {
	r.handlers[action] = handler
	r.descriptions[action] = description
}

// Execute runs the handler for the given action name.
// Returns an error if the action is not registered.
func (r *Registry) Execute(action string) error {
	handler, ok := r.handlers[action]
	if !ok {
		return fmt.Errorf("unknown action: %q", action)
	}
	return handler()
}

// HasAction returns true if the action is registered.
func (r *Registry) HasAction(action string) bool {
	_, ok := r.handlers[action]
	return ok
}

// Actions returns all registered action names.
func (r *Registry) Actions() []string {
	actions := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		actions = append(actions, name)
	}
	return actions
}

// Description returns the description for a registered action.
func (r *Registry) Description(action string) string {
	return r.descriptions[action]
}
