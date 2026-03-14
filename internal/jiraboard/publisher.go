package jiraboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/noko/computecommander/pkg/integrations/jira"
)

// Publisher handles creating/updating Jira issues from expanded tickets.
type Publisher struct {
	client *jira.Client
}

// NewPublisher creates a new Jira issue publisher.
func NewPublisher(client *jira.Client) *Publisher {
	return &Publisher{client: client}
}

// createIssuePayload builds the Jira REST API v3 payload for creating an issue.
func createIssuePayload(projectKey string, ticket *ExpandedTicket, parentIssueKey string) map[string]any {
	fields := map[string]any{
		"project":  map[string]string{"key": projectKey},
		"summary":  ticket.Summary,
		"issuetype": map[string]string{"name": ticket.IssueType},
		"labels":   ticket.Labels,
	}

	if ticket.Priority != "" {
		fields["priority"] = map[string]string{"name": ticket.Priority}
	}

	if ticket.Description != "" {
		// Jira Cloud API v3 requires ADF for descriptions.
		fields["description"] = toADF(ticket.Description)
	}

	if parentIssueKey != "" {
		fields["parent"] = map[string]string{"key": parentIssueKey}
	}

	return map[string]any{"fields": fields}
}

// toADF converts a markdown string to Atlassian Document Format.
// Uses a simple paragraph-based conversion.
func toADF(text string) map[string]any {
	paragraphs := strings.Split(text, "\n\n")
	var content []map[string]any

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Detect headings.
		if strings.HasPrefix(p, "## ") {
			content = append(content, map[string]any{
				"type": "heading",
				"attrs": map[string]any{
					"level": 2,
				},
				"content": []map[string]any{
					{"type": "text", "text": strings.TrimPrefix(p, "## ")},
				},
			})
			continue
		}

		// Detect checkbox lists.
		if strings.Contains(p, "- [ ]") || strings.Contains(p, "- [x]") {
			lines := strings.Split(p, "\n")
			var items []map[string]any
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "- [ ] ") {
					items = append(items, map[string]any{
						"type": "taskItem",
						"attrs": map[string]any{"state": "TODO"},
						"content": []map[string]any{
							{"type": "text", "text": strings.TrimPrefix(line, "- [ ] ")},
						},
					})
				} else if strings.HasPrefix(line, "- [x] ") {
					items = append(items, map[string]any{
						"type": "taskItem",
						"attrs": map[string]any{"state": "DONE"},
						"content": []map[string]any{
							{"type": "text", "text": strings.TrimPrefix(line, "- [x] ")},
						},
					})
				}
			}
			if len(items) > 0 {
				content = append(content, map[string]any{
					"type":    "taskList",
					"attrs":   map[string]any{"localId": ""},
					"content": items,
				})
			}
			continue
		}

		// Detect bullet lists.
		if strings.HasPrefix(p, "- ") {
			lines := strings.Split(p, "\n")
			var items []map[string]any
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "- ") {
					items = append(items, map[string]any{
						"type": "listItem",
						"content": []map[string]any{
							{
								"type": "paragraph",
								"content": []map[string]any{
									{"type": "text", "text": strings.TrimPrefix(line, "- ")},
								},
							},
						},
					})
				}
			}
			if len(items) > 0 {
				content = append(content, map[string]any{
					"type":    "bulletList",
					"content": items,
				})
			}
			continue
		}

		// Default: paragraph.
		content = append(content, map[string]any{
			"type": "paragraph",
			"content": []map[string]any{
				{"type": "text", "text": p},
			},
		})
	}

	if len(content) == 0 {
		content = []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		}
	}

	return map[string]any{
		"version": 1,
		"type":    "doc",
		"content": content,
	}
}

// Publish creates or updates Jira issues for all expanded tickets.
// Tickets are published in dependency order: epics first, then stories, then tasks.
// Returns a PublishResult with counts and any errors.
func (p *Publisher) Publish(ctx context.Context, projectKey string, tickets []ExpandedTicket) (*PublishResult, error) {
	result := &PublishResult{
		ProjectKey: projectKey,
	}

	// Build a map from deterministic key to Jira issue key for parent resolution.
	keyMap := make(map[string]string)

	// Search for existing issues with cmdr-key labels.
	existingKeys, err := p.findExistingKeys(ctx, projectKey)
	if err != nil {
		// Non-fatal: proceed with creation only.
		result.Errors = append(result.Errors, fmt.Sprintf("warning: could not search existing issues: %v", err))
	}

	// Merge existing keys.
	for k, v := range existingKeys {
		keyMap[k] = v
	}

	// Group tickets by type for ordered publishing.
	var epics, stories, tasks []ExpandedTicket
	for _, t := range tickets {
		switch t.IssueType {
		case "Epic":
			epics = append(epics, t)
		case "Story":
			stories = append(stories, t)
		case "Task":
			tasks = append(tasks, t)
		}
	}

	// Publish epics first.
	for _, ticket := range epics {
		issueKey, created, err := p.publishTicket(ctx, projectKey, &ticket, keyMap)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("epic %s: %v", ticket.DeterministicKey, err))
			continue
		}
		keyMap[ticket.DeterministicKey] = issueKey
		if created {
			result.TicketsCreated++
		} else {
			result.TicketsUpdated++
		}
		result.Epics++
	}

	// Publish stories.
	for _, ticket := range stories {
		issueKey, created, err := p.publishTicket(ctx, projectKey, &ticket, keyMap)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("story %s: %v", ticket.DeterministicKey, err))
			continue
		}
		keyMap[ticket.DeterministicKey] = issueKey
		if created {
			result.TicketsCreated++
		} else {
			result.TicketsUpdated++
		}
		result.Stories++
	}

	// Publish tasks.
	for _, ticket := range tasks {
		issueKey, created, err := p.publishTicket(ctx, projectKey, &ticket, keyMap)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("task %s: %v", ticket.DeterministicKey, err))
			continue
		}
		keyMap[ticket.DeterministicKey] = issueKey
		if created {
			result.TicketsCreated++
		} else {
			result.TicketsUpdated++
		}
		result.Tasks++
	}

	// Build track summaries.
	trackCounts := make(map[string]int)
	trackNames := make(map[string]string)
	trackPhases := make(map[string]string)
	for _, t := range tickets {
		trackCounts[t.TrackID]++
		if t.IssueType == "Epic" {
			trackNames[t.TrackID] = t.Summary
			trackPhases[t.TrackID] = t.Phase
		}
	}
	for id, count := range trackCounts {
		result.Tracks = append(result.Tracks, TrackSummary{
			ID:          id,
			Name:        trackNames[id],
			Phase:       trackPhases[id],
			TicketCount: count,
		})
	}

	return result, nil
}

// publishTicket creates or updates a single Jira issue.
// Returns the Jira issue key, whether it was created (vs updated), and any error.
func (p *Publisher) publishTicket(ctx context.Context, projectKey string, ticket *ExpandedTicket, keyMap map[string]string) (string, bool, error) {
	cmdrKeyLabel := "cmdr-key:" + ticket.DeterministicKey

	// Check if issue already exists.
	if existingKey, ok := keyMap[ticket.DeterministicKey]; ok {
		// Update existing issue.
		if err := p.updateIssue(ctx, existingKey, ticket); err != nil {
			return "", false, fmt.Errorf("update issue %s: %w", existingKey, err)
		}
		return existingKey, false, nil
	}

	// Resolve parent Jira key.
	var parentJiraKey string
	if ticket.ParentKey != "" {
		if jiraKey, ok := keyMap[ticket.ParentKey]; ok {
			parentJiraKey = jiraKey
		}
	}

	// Ensure the cmdr-key label is in the labels list.
	hasKey := false
	for _, l := range ticket.Labels {
		if l == cmdrKeyLabel {
			hasKey = true
			break
		}
	}
	if !hasKey {
		ticket.Labels = append(ticket.Labels, cmdrKeyLabel)
	}

	// Create new issue.
	payload := createIssuePayload(projectKey, ticket, parentJiraKey)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("marshal payload: %w", err)
	}

	issueKey, err := p.createIssue(ctx, data)
	if err != nil {
		return "", false, err
	}

	return issueKey, true, nil
}

// createIssue sends a POST request to create a Jira issue.
func (p *Publisher) createIssue(ctx context.Context, payload []byte) (string, error) {
	resp, err := p.doRequest(ctx, http.MethodPost, "/rest/api/3/issue", payload)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create issue failed with status %d", resp.StatusCode)
	}

	var result struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create response: %w", err)
	}

	return result.Key, nil
}

// updateIssue sends a PUT request to update an existing Jira issue.
func (p *Publisher) updateIssue(ctx context.Context, issueKey string, ticket *ExpandedTicket) error {
	fields := map[string]any{
		"summary": ticket.Summary,
		"labels":  ticket.Labels,
	}

	if ticket.Priority != "" {
		fields["priority"] = map[string]string{"name": ticket.Priority}
	}

	if ticket.Description != "" {
		fields["description"] = toADF(ticket.Description)
	}

	payload, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		return fmt.Errorf("marshal update payload: %w", err)
	}

	resp, err := p.doRequest(ctx, http.MethodPut, "/rest/api/3/issue/"+issueKey, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("update issue %s failed with status %d", issueKey, resp.StatusCode)
	}

	return nil
}

// findExistingKeys searches for issues in the project with cmdr-key labels.
func (p *Publisher) findExistingKeys(ctx context.Context, projectKey string) (map[string]string, error) {
	result := make(map[string]string)

	jql := fmt.Sprintf("project = %s AND labels IN (\"cmdr-key:*\") ORDER BY created ASC", projectKey)
	searchResult, err := p.client.SearchIssuesPaginated(ctx, jql, false)
	if err != nil {
		return result, err
	}

	for _, issue := range searchResult.Issues {
		for _, label := range issue.Fields.Labels {
			if strings.HasPrefix(label, "cmdr-key:") {
				detKey := strings.TrimPrefix(label, "cmdr-key:")
				result[detKey] = issue.Key
			}
		}
	}

	return result, nil
}

// doRequest is a helper that performs an HTTP request using the Jira client's
// underlying connection (auth, rate limiting).
func (p *Publisher) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	// Use the client's BaseURL to construct the full URL.
	url := p.client.BaseURL() + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("jiraboard: create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return http.DefaultClient.Do(req)
}
