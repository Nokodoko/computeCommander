package jira

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// PromptGenerator transforms Jira issues into machine-readable prompts
// using Go templates.
type PromptGenerator struct {
	templatePath string
	tmpl         *template.Template
}

// PromptData holds the template variables for prompt generation.
type PromptData struct {
	Key                string
	Summary            string
	Description        string
	AcceptanceCriteria string
	Outcomes           []string
	EpicKey            string
	EpicSummary        string
	ProjectKey         string
	Priority           string
	Labels             []string
	IssueType          string
	Status             string
	Assignee           string
}

// PromptResult holds the output of prompt generation.
type PromptResult struct {
	Prompt     string   `json:"prompt"`
	PromptHash string   `json:"promptHash"`
	Outcomes   []string `json:"outcomes"`
}

// NewPromptGenerator creates a prompt generator with the given template path.
// If templatePath is empty or the file doesn't exist, a default template is used.
func NewPromptGenerator(templatePath string) *PromptGenerator {
	pg := &PromptGenerator{templatePath: templatePath}
	pg.loadTemplate()
	return pg
}

// loadTemplate attempts to load the template from disk, falling back to a default.
func (pg *PromptGenerator) loadTemplate() {
	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	if pg.templatePath != "" {
		data, err := os.ReadFile(pg.templatePath)
		if err == nil {
			t, err := template.New("prompt").Funcs(funcMap).Parse(string(data))
			if err == nil {
				pg.tmpl = t
				return
			}
		}
	}

	// Default template.
	const defaultTmpl = `# {{ .Key }}: {{ .Summary }}

{{ .Description }}

## Acceptance Criteria

{{ .AcceptanceCriteria }}

## Outcomes

{{ range .Outcomes -}}
- {{ . }}
{{ end }}

## Constraints

- Parent epic: {{ .EpicKey }}{{ if .EpicSummary }} ({{ .EpicSummary }}){{ end }}
- Project: {{ .ProjectKey }}
- Priority: {{ .Priority }}
- Labels: {{ join .Labels ", " }}
`
	pg.tmpl = template.Must(template.New("prompt").Funcs(funcMap).Parse(defaultTmpl))
}

// Generate creates a prompt from a JiraIssue.
func (pg *PromptGenerator) Generate(issue *JiraIssue, epicKey, epicSummary, projectKey string) (*PromptResult, error) {
	outcomes := extractOutcomes(issue.AcceptanceCriteria)

	data := PromptData{
		Key:                issue.Key,
		Summary:            issue.Summary,
		Description:        issue.Description,
		AcceptanceCriteria: issue.AcceptanceCriteria,
		Outcomes:           outcomes,
		EpicKey:            epicKey,
		EpicSummary:        epicSummary,
		ProjectKey:         projectKey,
		Priority:           issue.Priority,
		Labels:             issue.Labels,
		IssueType:          issue.IssueType,
		Status:             issue.Status,
		Assignee:           issue.Assignee,
	}

	var buf bytes.Buffer
	if err := pg.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute prompt template: %w", err)
	}

	prompt := buf.String()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))

	return &PromptResult{
		Prompt:     prompt,
		PromptHash: hash,
		Outcomes:   outcomes,
	}, nil
}

// extractOutcomes parses bullet-point outcomes from acceptance criteria text.
// It looks for lines starting with "- " or "* " and treats them as outcomes.
func extractOutcomes(acceptanceCriteria string) []string {
	if acceptanceCriteria == "" {
		return []string{}
	}

	var outcomes []string
	for _, line := range strings.Split(acceptanceCriteria, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			outcomes = append(outcomes, strings.TrimPrefix(line, "- "))
		} else if strings.HasPrefix(line, "* ") {
			outcomes = append(outcomes, strings.TrimPrefix(line, "* "))
		}
	}

	if len(outcomes) == 0 {
		// If no bullet points, use the whole criteria as a single outcome.
		outcomes = append(outcomes, strings.TrimSpace(acceptanceCriteria))
	}

	return outcomes
}
