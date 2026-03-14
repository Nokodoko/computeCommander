package jiraboard

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Renderer renders ticket descriptions using Go text/template files.
type Renderer struct {
	templateDir string
	templates   map[string]*template.Template
}

// NewRenderer creates a new description renderer that loads templates from
// the description-templates subdirectory within the partials directory.
func NewRenderer(templateDir string) (*Renderer, error) {
	descDir := filepath.Join(templateDir, "_partials", "description-templates")

	r := &Renderer{
		templateDir: descDir,
		templates:   make(map[string]*template.Template),
	}

	// Pre-load all .tmpl files.
	entries, err := os.ReadDir(descDir)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil // No templates directory — render will produce empty descriptions.
		}
		return nil, fmt.Errorf("read description templates dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		path := filepath.Join(descDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", entry.Name(), err)
		}

		tmpl, err := template.New(entry.Name()).Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", entry.Name(), err)
		}

		r.templates[entry.Name()] = tmpl
	}

	return r, nil
}

// Render renders a ticket description using the specified template name and context.
func (r *Renderer) Render(templateName string, ctx *DescriptionContext) (string, error) {
	tmpl, ok := r.templates[templateName]
	if !ok {
		// Template not found — return a basic description.
		return fmt.Sprintf("## %s\n\n%s", ctx.TrackName, ctx.Context), nil
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("render template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// RenderTickets fills in description fields for all expanded tickets.
func (r *Renderer) RenderTickets(tickets []ExpandedTicket, tmpl *BoardTemplate) error {
	for i := range tickets {
		t := &tickets[i]

		// Epics use epic-summary template.
		if t.IssueType == "Epic" {
			track := findTrackByID(tmpl.Tracks, t.TrackID)
			if track == nil {
				continue
			}
			ctx := &DescriptionContext{
				TrackName: track.Name,
				Phase:     t.Phase,
				Context:   track.Description,
			}
			desc, err := r.Render("epic-summary.md.tmpl", ctx)
			if err != nil {
				return fmt.Errorf("render epic %s: %w", t.DeterministicKey, err)
			}
			t.Description = desc
			continue
		}

		// Stories and tasks: build context from dimensions.
		ctx := buildDescriptionContext(t, tmpl)

		// Determine template name from the story/task template.
		tmplName := descriptionTemplateForTicket(t, tmpl)
		if tmplName == "" {
			tmplName = "integration-task.md.tmpl"
			if t.IssueType == "Story" {
				tmplName = "integration-story.md.tmpl"
			}
		}

		desc, err := r.Render(tmplName, ctx)
		if err != nil {
			return fmt.Errorf("render %s %s: %w", t.IssueType, t.DeterministicKey, err)
		}
		t.Description = desc
	}

	return nil
}

// buildDescriptionContext creates a DescriptionContext from an expanded ticket.
func buildDescriptionContext(t *ExpandedTicket, tmpl *BoardTemplate) *DescriptionContext {
	ctx := &DescriptionContext{
		TicketID:          t.DeterministicKey,
		TrackID:           t.TrackID,
		Phase:             t.Phase,
		Labels:            t.Labels,
		ParamDescriptions: make(map[string]string),
	}

	// Find track name.
	track := findTrackByID(tmpl.Tracks, t.TrackID)
	if track != nil {
		ctx.TrackName = track.Name
		ctx.Context = track.Description
	}

	// Populate integration details from dimensions.
	if db, ok := t.Dimensions["database"]; ok {
		ctx.Integration.ID = db.ID
		ctx.Integration.Label = db.Label
		ctx.Integration.Path = db.IntegrationPath()
		ctx.Integration.ConfSpec = db.ConfSpec()
		ctx.Integration.CheckDir = db.ID
		ctx.Integration.RequiredParams = db.RequiredParams()
		ctx.Integration.OptionalParams = db.OptionalParams()
		ctx.Integration.DBM = db.ID == "postgres" || db.ID == "mysql" || db.ID == "sqlserver"

		// Create placeholder param descriptions.
		for _, p := range ctx.Integration.RequiredParams {
			ctx.ParamDescriptions[p] = fmt.Sprintf("Required. See conf.yaml spec for details.")
		}
		for _, p := range ctx.Integration.OptionalParams {
			ctx.ParamDescriptions[p] = fmt.Sprintf("Optional. See conf.yaml spec for details.")
		}
	}

	// Cloud provider.
	if cp, ok := t.Dimensions["cloud_provider"]; ok {
		ctx.CloudProvider = cp
	}

	// OS.
	if osVal, ok := t.Dimensions["os"]; ok {
		ctx.OS = osVal
	}

	// App architecture.
	if aa, ok := t.Dimensions["app_architecture"]; ok {
		ctx.AppArchitecture = aa
	}

	// Environment.
	if env, ok := t.Dimensions["environment"]; ok {
		ctx.Environment = env.ID
	}

	return ctx
}

// descriptionTemplateForTicket looks up the description template name for a ticket.
func descriptionTemplateForTicket(t *ExpandedTicket, tmpl *BoardTemplate) string {
	// For stories, find the story template.
	for _, story := range tmpl.Stories {
		if story.Track == t.TrackID {
			if t.IssueType == "Story" {
				return story.DescriptionTemplate
			}
			// For tasks, check task templates.
			for _, task := range story.Tasks {
				// Match by checking if the ticket key contains the task ID pattern.
				return task.DescriptionTemplate
			}
		}
	}
	return ""
}

// findTrackByID finds a track by ID in the tracks slice.
func findTrackByID(tracks []Track, id string) *Track {
	for i := range tracks {
		if tracks[i].ID == id {
			return &tracks[i]
		}
	}
	return nil
}
