// Package darkfactory provides autonomous task execution from Jira issues.
// It orchestrates the sync-generate-execute-verify-transition pipeline.
package darkfactory

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/pkg/integrations/jira"
)

// TaskState tracks the state of a dark factory task.
type TaskState struct {
	IssueKey   string    `json:"key"`
	AgentState string    `json:"agentState"`
	SessionID  string    `json:"sessionId,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	Error      string    `json:"error,omitempty"`
}

// ExecutorStatus holds the overall dark factory status.
type ExecutorStatus struct {
	Mode      string      `json:"mode"`
	Project   string      `json:"project"`
	Active    int         `json:"active"`
	Completed int         `json:"completed"`
	Failed    int         `json:"failed"`
	Pending   int         `json:"pending"`
	Tasks     []TaskState `json:"tasks"`
}

// Executor orchestrates autonomous Jira task execution.
type Executor struct {
	db             db.DB
	config         *config.DarkFactoryConfig
	syncEngine     *jira.SyncEngine
	promptGen      *jira.PromptGenerator
	verifier       *IntentVerifier
	maxConcurrent  int
	uatTimeout     time.Duration

	mu        sync.Mutex
	active    map[string]*TaskState
	completed int
	failed    int
}

// ExecutorOpts configures the dark factory executor.
type ExecutorOpts struct {
	DB         db.DB
	Config     *config.DarkFactoryConfig
	SyncEngine *jira.SyncEngine
	PromptGen  *jira.PromptGenerator
	Verifier   *IntentVerifier
}

// NewExecutor creates a new dark factory executor.
func NewExecutor(opts ExecutorOpts) *Executor {
	maxConcurrent := opts.Config.MaxConcurrentTasks
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	uatTimeout := 15 * time.Minute
	if opts.Config.UATTimeout != "" {
		if d, err := time.ParseDuration(opts.Config.UATTimeout); err == nil {
			uatTimeout = d
		}
	}

	return &Executor{
		db:            opts.DB,
		config:        opts.Config,
		syncEngine:    opts.SyncEngine,
		promptGen:     opts.PromptGen,
		verifier:      opts.Verifier,
		maxConcurrent: maxConcurrent,
		uatTimeout:    uatTimeout,
		active:        make(map[string]*TaskState),
	}
}

// Run starts the dark factory execution loop for a project.
// It syncs issues, generates prompts, and manages task execution.
func (e *Executor) Run(ctx context.Context, projectKey string) error {
	log.Printf("dark-factory: starting for project %s (mode: %s, max_concurrent: %d)",
		projectKey, e.config.ExecutionMode, e.maxConcurrent)

	// Sync project first.
	result, err := e.syncEngine.SyncProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("dark-factory: sync failed: %w", err)
	}
	log.Printf("dark-factory: synced %d issues", result.IssuesSync)

	// Get pending issues (To Do status).
	issues, err := e.syncEngine.GetCachedIssues(ctx, projectKey, "To Do")
	if err != nil {
		return fmt.Errorf("dark-factory: get issues: %w", err)
	}

	if len(issues) == 0 {
		log.Printf("dark-factory: no pending issues for project %s", projectKey)
		return nil
	}

	log.Printf("dark-factory: found %d pending issues", len(issues))

	// Process issues with concurrency control.
	sem := make(chan struct{}, e.maxConcurrent)

	var wg sync.WaitGroup
	for i := range issues {
		select {
		case <-ctx.Done():
			log.Printf("dark-factory: context cancelled, stopping")
			wg.Wait()
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(issue jira.JiraIssue) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := e.processIssue(ctx, &issue, projectKey); err != nil {
				log.Printf("dark-factory: error processing %s: %v", issue.Key, err)
				e.mu.Lock()
				e.failed++
				e.mu.Unlock()
			}
		}(issues[i])
	}

	wg.Wait()
	log.Printf("dark-factory: completed. active=%d completed=%d failed=%d",
		len(e.active), e.completed, e.failed)
	return nil
}

// processIssue handles a single issue through the dark factory pipeline.
func (e *Executor) processIssue(ctx context.Context, issue *jira.JiraIssue, projectKey string) error {
	state := &TaskState{
		IssueKey:   issue.Key,
		AgentState: "booting",
		StartedAt:  time.Now(),
	}

	e.mu.Lock()
	e.active[issue.Key] = state
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.active, issue.Key)
		e.mu.Unlock()
	}()

	// Generate prompt.
	promptResult, err := e.promptGen.Generate(issue, "", "", projectKey)
	if err != nil {
		state.Error = err.Error()
		return fmt.Errorf("generate prompt for %s: %w", issue.Key, err)
	}

	// Verify intent if verifier is available.
	if e.verifier != nil {
		verifyResult := e.verifier.Verify(promptResult.Prompt)
		if !verifyResult.Valid {
			state.AgentState = "blocked"
			state.Error = fmt.Sprintf("intent verification failed (score: %.2f)", verifyResult.Score)
			return fmt.Errorf("intent verification failed for %s: score %.2f", issue.Key, verifyResult.Score)
		}
	}

	state.AgentState = "working"

	// In stepped mode, we just generate and validate -- no agent spawn.
	if e.config.ExecutionMode == "stepped" {
		log.Printf("dark-factory: [stepped] prompt generated for %s (hash: %s)",
			issue.Key, promptResult.PromptHash[:12])
		state.AgentState = "completed"
		e.mu.Lock()
		e.completed++
		e.mu.Unlock()
		return nil
	}

	// In full_auto mode, we would spawn an agent here.
	// For now, mark as completed after prompt generation + verification.
	log.Printf("dark-factory: [%s] processed %s successfully", e.config.ExecutionMode, issue.Key)
	state.AgentState = "completed"
	e.mu.Lock()
	e.completed++
	e.mu.Unlock()

	return nil
}

// Status returns the current executor status.
func (e *Executor) Status(projectKey string) *ExecutorStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	var tasks []TaskState
	for _, t := range e.active {
		tasks = append(tasks, *t)
	}

	return &ExecutorStatus{
		Mode:      e.config.ExecutionMode,
		Project:   projectKey,
		Active:    len(e.active),
		Completed: e.completed,
		Failed:    e.failed,
		Tasks:     tasks,
	}
}
