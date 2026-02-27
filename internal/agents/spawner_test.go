package agents

import (
	"context"
	"fmt"
	"testing"

	"github.com/noko/computecommander/pkg/runtimes"
)

func TestNewSpawner(t *testing.T) {
	s := NewSpawner(SpawnerOpts{
		MaxDepth:      3,
		MaxConcurrent: 5,
	})
	if s == nil {
		t.Fatal("NewSpawner returned nil")
	}
	if s.maxDepth != 3 {
		t.Errorf("maxDepth = %d, want 3", s.maxDepth)
	}
	if s.maxConcurrent != 5 {
		t.Errorf("maxConcurrent = %d, want 5", s.maxConcurrent)
	}
}

func TestNewSpawner_Defaults(t *testing.T) {
	s := NewSpawner(SpawnerOpts{})
	if s.maxDepth != 2 {
		t.Errorf("default maxDepth = %d, want 2", s.maxDepth)
	}
	if s.maxConcurrent != 10 {
		t.Errorf("default maxConcurrent = %d, want 10", s.maxConcurrent)
	}
	if s.generateID == nil {
		t.Error("generateID should not be nil")
	}
}

func TestValidateSpawnRequest(t *testing.T) {
	s := NewSpawner(SpawnerOpts{MaxDepth: 2})

	tests := []struct {
		name    string
		req     SpawnRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: SpawnRequest{
				TaskID:     "task-1",
				Capability: CapBuilder,
				Name:       "builder-1",
				Runtime:    runtimes.RuntimeClaude,
				Depth:      1,
			},
			wantErr: false,
		},
		{
			name: "missing task ID",
			req: SpawnRequest{
				Capability: CapBuilder,
				Name:       "builder-1",
				Runtime:    runtimes.RuntimeClaude,
			},
			wantErr: true,
		},
		{
			name: "invalid capability",
			req: SpawnRequest{
				TaskID:     "task-1",
				Capability: Capability("hacker"),
				Name:       "evil-1",
				Runtime:    runtimes.RuntimeClaude,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			req: SpawnRequest{
				TaskID:     "task-1",
				Capability: CapScout,
				Runtime:    runtimes.RuntimeGemini,
			},
			wantErr: true,
		},
		{
			name: "missing runtime",
			req: SpawnRequest{
				TaskID:     "task-1",
				Capability: CapScout,
				Name:       "scout-1",
			},
			wantErr: true,
		},
		{
			name: "depth exceeds max",
			req: SpawnRequest{
				TaskID:     "task-1",
				Capability: CapBuilder,
				Name:       "builder-1",
				Runtime:    runtimes.RuntimeClaude,
				Depth:      5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.validateSpawnRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSpawnRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpawn_MissingRuntime(t *testing.T) {
	s := NewSpawner(SpawnerOpts{
		GetRuntime: func(id string) (runtimes.AgentRuntime, error) {
			return nil, fmt.Errorf("unknown runtime: %q", id)
		},
	})

	_, err := s.Spawn(context.Background(), SpawnRequest{
		TaskID:     "task-1",
		Capability: CapBuilder,
		Name:       "builder-1",
		Runtime:    runtimes.RuntimeID("nonexistent"),
		Depth:      0,
	})
	if err == nil {
		t.Error("expected error for unknown runtime")
	}
}

func TestDefaultIDGenerator(t *testing.T) {
	id1 := defaultIDGenerator()
	id2 := defaultIDGenerator()
	if id1 == "" {
		t.Error("generated ID should not be empty")
	}
	if id1 == id2 {
		t.Error("consecutive IDs should be different")
	}
}
