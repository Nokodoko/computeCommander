package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ClientOpts configures a Jira REST API client.
type ClientOpts struct {
	BaseURL  string // e.g., "https://mycompany.atlassian.net"
	AuthType string // "pat", "basic", "oauth2"
	Token    string // PAT or OAuth2 bearer token
	Username string // For basic auth
	Password string // For basic auth

	// RateLimiter controls request rate. If nil, a default limiter is created.
	RateLimiter *RateLimiter

	// HTTPClient overrides the default HTTP client (useful for testing).
	HTTPClient *http.Client
}

// Client is a Jira REST API v3 client with rate limiting.
type Client struct {
	baseURL    string
	authType   string
	token      string
	username   string
	password   string
	limiter    *RateLimiter
	httpClient *http.Client
}

// NewClient creates a new Jira REST API client.
func NewClient(opts ClientOpts) *Client {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	limiter := opts.RateLimiter
	if limiter == nil {
		limiter = NewRateLimiter(10, 20)
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		authType:   opts.AuthType,
		token:      opts.Token,
		username:   opts.Username,
		password:   opts.Password,
		limiter:    limiter,
		httpClient: httpClient,
	}
}

// do executes an HTTP request with rate limiting and auth headers.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("jira: rate limit wait: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("jira: create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	switch c.authType {
	case "pat", "oauth2":
		req.Header.Set("Authorization", "Bearer "+c.token)
	case "basic":
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira: request %s %s: %w", method, path, err)
	}

	// Adapt rate limiter from response headers.
	c.adaptRateFromResponse(resp)

	return resp, nil
}

// adaptRateFromResponse reads Jira rate limit headers and adjusts the limiter.
func (c *Client) adaptRateFromResponse(resp *http.Response) {
	remaining := -1
	var retryAfter time.Duration

	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			remaining = n
		}
	}

	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			retryAfter = time.Duration(secs) * time.Second
		}
	}

	if remaining >= 0 || retryAfter > 0 {
		c.limiter.AdaptFromHeaders(remaining, retryAfter)
	}
}

// decodeResponse reads and decodes a JSON response body. It returns an error
// for non-2xx status codes.
func decodeResponse(resp *http.Response, v any) error {
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira: authentication failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("jira: rate limit exceeded (HTTP 429)")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("jira: decode response: %w", err)
		}
	}

	return nil
}

// GetProject fetches a Jira project by key.
func (c *Client) GetProject(ctx context.Context, projectKey string) (*APIProject, error) {
	resp, err := c.do(ctx, http.MethodGet, "/rest/api/3/project/"+projectKey, nil)
	if err != nil {
		return nil, err
	}
	var project APIProject
	if err := decodeResponse(resp, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ListProjects fetches all accessible Jira projects.
func (c *Client) ListProjects(ctx context.Context) ([]APIProject, error) {
	resp, err := c.do(ctx, http.MethodGet, "/rest/api/3/project/search", nil)
	if err != nil {
		return nil, err
	}
	var result ProjectListResponse
	if err := decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Values, nil
}

// SearchIssues performs a JQL search using the /rest/api/3/search/jql endpoint (POST).
// The legacy GET /rest/api/3/search endpoint was removed by Atlassian (HTTP 410).
func (c *Client) SearchIssues(ctx context.Context, jql string, maxResults int, startAt int) (*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	if startAt < 0 {
		startAt = 0
	}

	fields := "summary,description,status,issuetype,priority,assignee,labels,project,epic,parent"

	params := "?jql=" + url.QueryEscape(jql) +
		"&maxResults=" + strconv.Itoa(maxResults) +
		"&startAt=" + strconv.Itoa(startAt) +
		"&fields=" + fields

	resp, err := c.do(ctx, http.MethodGet, "/rest/api/3/search/jql"+params, nil)
	if err != nil {
		return nil, err
	}
	var result SearchResult
	if err := decodeResponse(resp, &result); err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}
	return &result, nil
}

// GetIssue fetches a single issue by key.
func (c *Client) GetIssue(ctx context.Context, issueKey string) (*APIIssue, error) {
	resp, err := c.do(ctx, http.MethodGet, "/rest/api/3/issue/"+issueKey, nil)
	if err != nil {
		return nil, err
	}
	var issue APIIssue
	if err := decodeResponse(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// TransitionIssue transitions an issue to a new status.
func (c *Client) TransitionIssue(ctx context.Context, issueKey, transitionID string) error {
	body := fmt.Sprintf(`{"transition":{"id":"%s"}}`, transitionID)
	resp, err := c.do(ctx, http.MethodPost,
		"/rest/api/3/issue/"+issueKey+"/transitions",
		strings.NewReader(body))
	if err != nil {
		return err
	}
	return decodeResponse(resp, nil)
}

// GetTransitions lists available transitions for an issue.
func (c *Client) GetTransitions(ctx context.Context, issueKey string) ([]APITransition, error) {
	resp, err := c.do(ctx, http.MethodGet,
		"/rest/api/3/issue/"+issueKey+"/transitions", nil)
	if err != nil {
		return nil, err
	}
	var result TransitionsResponse
	if err := decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Transitions, nil
}

// AddComment adds a comment to an issue using Atlassian Document Format (ADF).
func (c *Client) AddComment(ctx context.Context, issueKey, commentText string) error {
	// Jira Cloud API v3 requires ADF for comment bodies.
	// Wrap the plain text in a code block to preserve formatting.
	payload := map[string]any{
		"body": map[string]any{
			"version": 1,
			"type":    "doc",
			"content": []map[string]any{
				{
					"type": "codeBlock",
					"attrs": map[string]string{
						"language": "markdown",
					},
					"content": []map[string]any{
						{
							"type": "text",
							"text": commentText,
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal comment: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost,
		"/rest/api/3/issue/"+issueKey+"/comment",
		strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	return decodeResponse(resp, nil)
}

// AddCommentWithID adds a comment and returns the comment ID from the response.
func (c *Client) AddCommentWithID(ctx context.Context, issueKey, commentText string) (string, error) {
	// Jira Cloud API v3 requires ADF for comment bodies.
	payload := map[string]any{
		"body": map[string]any{
			"version": 1,
			"type":    "doc",
			"content": []map[string]any{
				{
					"type": "codeBlock",
					"attrs": map[string]string{
						"language": "markdown",
					},
					"content": []map[string]any{
						{
							"type": "text",
							"text": commentText,
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal comment: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost,
		"/rest/api/3/issue/"+issueKey+"/comment",
		strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := decodeResponse(resp, &result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// DeleteComment removes a comment from an issue.
func (c *Client) DeleteComment(ctx context.Context, issueKey, commentID string) error {
	resp, err := c.do(ctx, http.MethodDelete,
		"/rest/api/3/issue/"+issueKey+"/comment/"+commentID, nil)
	if err != nil {
		return err
	}
	// DELETE returns 204 No Content on success; decodeResponse handles non-2xx.
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeResponse(resp, nil)
	}
	return nil
}

// GetComments lists comments on an issue.
func (c *Client) GetComments(ctx context.Context, issueKey string) ([]APIComment, error) {
	resp, err := c.do(ctx, http.MethodGet,
		"/rest/api/3/issue/"+issueKey+"/comment", nil)
	if err != nil {
		return nil, err
	}
	var result CommentsResponse
	if err := decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Comments, nil
}

// SearchIssuesExcludeSubTasks performs a JQL search that excludes sub-task issue types.
func (c *Client) SearchIssuesExcludeSubTasks(ctx context.Context, jql string, maxResults int, startAt int) (*SearchResult, error) {
	subTaskFilter := "issuetype NOT IN subTaskIssueTypes()"
	var finalJQL string
	if jql == "" {
		finalJQL = subTaskFilter + " ORDER BY Rank ASC"
	} else {
		// Extract ORDER BY clause if present so filter is injected before it.
		upper := strings.ToUpper(jql)
		if idx := strings.LastIndex(upper, "ORDER BY"); idx >= 0 {
			base := strings.TrimSpace(jql[:idx])
			orderBy := strings.TrimSpace(jql[idx:])
			finalJQL = base + " AND " + subTaskFilter + " " + orderBy
		} else {
			finalJQL = jql + " AND " + subTaskFilter
		}
	}
	return c.SearchIssues(ctx, finalJQL, maxResults, startAt)
}

// SearchIssuesPaginated fetches all issues matching a JQL query using pagination.
// If excludeSubTasks is true, sub-task issue types are filtered out.
// Returns partial results and error on mid-stream failure.
func (c *Client) SearchIssuesPaginated(ctx context.Context, jql string, excludeSubTasks bool) (*SearchResult, error) {
	const pageSize = 50
	combined := &SearchResult{}

	for startAt := 0; ; startAt += pageSize {
		var page *SearchResult
		var err error
		if excludeSubTasks {
			page, err = c.SearchIssuesExcludeSubTasks(ctx, jql, pageSize, startAt)
		} else {
			page, err = c.SearchIssues(ctx, jql, pageSize, startAt)
		}
		if err != nil {
			// Return partial results collected so far along with the error.
			return combined, fmt.Errorf("paginated search at offset %d: %w", startAt, err)
		}

		combined.Issues = append(combined.Issues, page.Issues...)
		combined.Total = page.Total
		combined.MaxResults = page.MaxResults

		if len(page.Issues) == 0 || startAt+len(page.Issues) >= page.Total {
			break
		}
	}

	return combined, nil
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string { return c.baseURL }
