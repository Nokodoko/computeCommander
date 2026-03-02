package config

import (
	"fmt"
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// ConfigChangeHandler is called when the config file changes and is successfully reloaded.
type ConfigChangeHandler func(newConfig *Config)

// Watcher monitors a config file for changes and reloads it automatically.
type Watcher struct {
	path      string
	current   *Config
	mu        sync.RWMutex
	handlers  []ConfigChangeHandler
	watcher   *fsnotify.Watcher
	done      chan struct{}
}

// NewWatcher creates a config file watcher for the given path.
// It loads the initial config and starts watching for changes.
func NewWatcher(path string) (*Watcher, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load initial config: %w", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		path:    path,
		current: cfg,
		watcher: fsw,
		done:    make(chan struct{}),
	}

	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, fmt.Errorf("watch config file: %w", err)
	}

	go w.watch()

	return w, nil
}

// OnChange registers a handler to be called when the config changes.
func (w *Watcher) OnChange(handler ConfigChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, handler)
}

// Config returns the current config (thread-safe).
func (w *Watcher) Config() *Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.current
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}

// watch runs the fsnotify event loop.
func (w *Watcher) watch() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.reload()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("config watcher error: %v", err)
		case <-w.done:
			return
		}
	}
}

// reload attempts to reload the config file.
func (w *Watcher) reload() {
	newCfg, err := LoadConfig(w.path)
	if err != nil {
		log.Printf("config reload failed (keeping current config): %v", err)
		return
	}

	if err := newCfg.Validate(); err != nil {
		log.Printf("config reload validation failed (keeping current config): %v", err)
		return
	}

	w.mu.Lock()
	w.current = newCfg
	handlers := make([]ConfigChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)
	w.mu.Unlock()

	// Notify handlers.
	for _, h := range handlers {
		h(newCfg)
	}
}
