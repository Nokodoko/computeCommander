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
	templatePath  string
	tmpl          *template.Template
	recursiveTmpl *template.Template
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

// SubTaskData holds template variables for a sub-task within a recursive prompt.
type SubTaskData struct {
	Key                string
	Summary            string
	Description        string
	AcceptanceCriteria string
	Outcomes           []string
	Status             string
	IssueType          string
	Priority           string
	Assignee           string
}

// RecursivePromptData extends PromptData with sub-task information.
type RecursivePromptData struct {
	PromptData
	SubTasks    []SubTaskData
	HasSubTasks bool
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

	customLoaded := false
	if pg.templatePath != "" {
		data, err := os.ReadFile(pg.templatePath)
		if err == nil {
			t, err := template.New("prompt").Funcs(funcMap).Parse(string(data))
			if err == nil {
				pg.tmpl = t
				customLoaded = true
			}
		}
	}

	// Default template (used when no custom template is loaded).
	if !customLoaded {
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

	// Recursive template for parent tasks with sub-tasks (always initialized).
	const recursiveTmpl = `# {{ .Key }}: {{ .Summary }}

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
{{ if .HasSubTasks }}
## Sub-Tasks

{{ range .SubTasks }}
### {{ .Key }}: {{ .Summary }}

{{ .Description }}

**Acceptance Criteria:**
{{ .AcceptanceCriteria }}

**Outcomes:**
{{ range .Outcomes -}}
- {{ . }}
{{ end }}
**Status:** {{ .Status }} | **Priority:** {{ .Priority }}{{ if .Assignee }} | **Assignee:** {{ .Assignee }}{{ end }}

---
{{ end }}

## Orchestrator Instructions

Execute this parent task by processing each sub-task using the ` + "`/spec --review --loop`" + ` command.
The parent task ({{ .Key }}: {{ .Summary }}) defines the overall context and success criteria.

Process each sub-task in order:
{{ range $i, $st := .SubTasks }}
{{ add $i 1 }}. ` + "`/spec --review --loop`" + ` for **{{ $st.Key }}**: {{ $st.Summary }}
{{- end }}

For each sub-task:
1. Read the sub-task description and acceptance criteria above
2. Run ` + "`/spec --review --loop`" + ` scoped to that sub-task
3. Verify the sub-task outcomes are met before proceeding to the next
4. Ensure all work aligns with the parent task's acceptance criteria and outcomes
{{ end }}`
	pg.recursiveTmpl = template.Must(template.New("recursive").Funcs(template.FuncMap{
		"join": strings.Join,
		"add":  func(a, b int) int { return a + b },
	}).Parse(recursiveTmpl))
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

// GenerateRecursive creates a prompt from a parent JiraIssue and its sub-tasks.
// The prompt includes all sub-task material and orchestrator instructions to run
// /spec --review --loop per sub-task.
func (pg *PromptGenerator) GenerateRecursive(parent *JiraIssue, subTasks []JiraIssue, epicKey, epicSummary, projectKey string) (*PromptResult, error) {
	outcomes := extractOutcomes(parent.AcceptanceCriteria)

	var subTaskData []SubTaskData
	for _, st := range subTasks {
		subTaskData = append(subTaskData, SubTaskData{
			Key:                st.Key,
			Summary:            st.Summary,
			Description:        st.Description,
			AcceptanceCriteria: st.AcceptanceCriteria,
			Outcomes:           extractOutcomes(st.AcceptanceCriteria),
			Status:             st.Status,
			IssueType:          st.IssueType,
			Priority:           st.Priority,
			Assignee:           st.Assignee,
		})
	}

	data := RecursivePromptData{
		PromptData: PromptData{
			Key:                parent.Key,
			Summary:            parent.Summary,
			Description:        parent.Description,
			AcceptanceCriteria: parent.AcceptanceCriteria,
			Outcomes:           outcomes,
			EpicKey:            epicKey,
			EpicSummary:        epicSummary,
			ProjectKey:         projectKey,
			Priority:           parent.Priority,
			Labels:             parent.Labels,
			IssueType:          parent.IssueType,
			Status:             parent.Status,
			Assignee:           parent.Assignee,
		},
		SubTasks:    subTaskData,
		HasSubTasks: len(subTaskData) > 0,
	}

	var buf bytes.Buffer
	if err := pg.recursiveTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute recursive prompt template: %w", err)
	}

	prompt := buf.String()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))

	// Collect outcomes from parent + all sub-tasks.
	allOutcomes := make([]string, 0, len(outcomes))
	allOutcomes = append(allOutcomes, outcomes...)
	for _, st := range subTaskData {
		allOutcomes = append(allOutcomes, st.Outcomes...)
	}

	return &PromptResult{
		Prompt:     prompt,
		PromptHash: hash,
		Outcomes:   allOutcomes,
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
