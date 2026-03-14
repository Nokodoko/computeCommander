package jiraboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Engine loads, merges, and validates board templates.
type Engine struct {
	templateDir string
}

// NewEngine creates a new template engine rooted at the given template directory.
// templateDir should point to the templates/jira-board/ directory.
func NewEngine(templateDir string) *Engine {
	return &Engine{templateDir: templateDir}
}

// LoadTemplate loads the main template file and auto-discovers and merges
// all partial files from the _partials/ subdirectory.
func (e *Engine) LoadTemplate(projectType string) (*BoardTemplate, error) {
	mainPath := filepath.Join(e.templateDir, projectType+".yaml")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return nil, fmt.Errorf("load template %s: %w", mainPath, err)
	}

	var tmpl BoardTemplate
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parse template %s: %w", mainPath, err)
	}

	// Ensure dimensions map is initialized.
	if tmpl.Dimensions == nil {
		tmpl.Dimensions = make(map[string][]DimensionValue)
	}

	// Auto-discover and merge partials.
	partialsDir := filepath.Join(e.templateDir, "_partials")
	if err := e.mergePartials(&tmpl, partialsDir); err != nil {
		return nil, fmt.Errorf("merge partials: %w", err)
	}

	// Validate the merged template.
	if errs := e.Validate(&tmpl); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, ve := range errs {
			msgs[i] = ve.Error()
		}
		return nil, fmt.Errorf("template validation failed:\n  %s", strings.Join(msgs, "\n  "))
	}

	return &tmpl, nil
}

// mergePartials discovers all *.yaml files in partialsDir and merges their
// dimensions into the template. Non-dimension partials (like log_integrations)
// are skipped for dimension merging.
func (e *Engine) mergePartials(tmpl *BoardTemplate, partialsDir string) error {
	entries, err := os.ReadDir(partialsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No partials directory — not an error.
		}
		return fmt.Errorf("read partials dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		partialPath := filepath.Join(partialsDir, entry.Name())
		data, err := os.ReadFile(partialPath)
		if err != nil {
			return fmt.Errorf("read partial %s: %w", entry.Name(), err)
		}

		// Parse partial as a generic map to handle both dimension and
		// non-dimension partials.
		var partial map[string]any
		if err := yaml.Unmarshal(data, &partial); err != nil {
			return fmt.Errorf("parse partial %s: %w", entry.Name(), err)
		}

		// Look for a "dimensions" key in the partial.
		dimRaw, ok := partial["dimensions"]
		if !ok {
			continue // Non-dimension partial (e.g., log_integrations).
		}

		dimMap, ok := dimRaw.(map[string]any)
		if !ok {
			continue
		}

		// Merge each dimension from the partial into the template.
		for dimName, valuesRaw := range dimMap {
			values, err := parseDimensionValues(valuesRaw)
			if err != nil {
				return fmt.Errorf("parse dimension %s in %s: %w", dimName, entry.Name(), err)
			}

			// Lenient merge: add dimension even if not declared in main template.
			if existing, exists := tmpl.Dimensions[dimName]; exists && len(existing) > 0 {
				// Dimension already has values (from another partial); append.
				tmpl.Dimensions[dimName] = append(existing, values...)
			} else {
				tmpl.Dimensions[dimName] = values
			}
		}
	}

	return nil
}

// parseDimensionValues converts raw YAML values into typed DimensionValue slices.
func parseDimensionValues(raw any) ([]DimensionValue, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", raw)
	}

	var result []DimensionValue
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected map, got %T", item)
		}

		dv := DimensionValue{
			Extra: make(map[string]any),
		}

		for k, v := range m {
			switch k {
			case "id":
				if s, ok := v.(string); ok {
					dv.ID = s
				}
			case "label":
				if s, ok := v.(string); ok {
					dv.Label = s
				}
			default:
				dv.Extra[k] = v
			}
		}

		if dv.ID == "" {
			return nil, fmt.Errorf("dimension value missing required 'id' field")
		}
		if dv.Label == "" {
			dv.Label = dv.ID // Default label to ID if not specified.
		}

		result = append(result, dv)
	}

	return result, nil
}

// Validate checks the template for structural correctness.
func (e *Engine) Validate(tmpl *BoardTemplate) []SchemaValidationError {
	var errs []SchemaValidationError

	// Meta validation.
	if tmpl.Meta.Version == "" {
		errs = append(errs, SchemaValidationError{Field: "meta.version", Message: "required"})
	}
	if tmpl.Meta.ProjectType == "" {
		errs = append(errs, SchemaValidationError{Field: "meta.project_type", Message: "required"})
	}
	if tmpl.Meta.Description == "" {
		errs = append(errs, SchemaValidationError{Field: "meta.description", Message: "required"})
	}

	// Tracks validation.
	if len(tmpl.Tracks) == 0 {
		errs = append(errs, SchemaValidationError{Field: "tracks", Message: "at least one track required"})
	}
	trackIDs := make(map[string]bool)
	for _, t := range tmpl.Tracks {
		if t.ID == "" {
			errs = append(errs, SchemaValidationError{Field: "tracks[].id", Message: "required"})
		}
		if t.Name == "" {
			errs = append(errs, SchemaValidationError{Field: "tracks[].name", Message: "required"})
		}
		if t.Phase == "" {
			errs = append(errs, SchemaValidationError{Field: "tracks[].phase", Message: "required for track " + t.ID})
		}
		trackIDs[t.ID] = true
	}

	// Validate track dependencies exist.
	for _, t := range tmpl.Tracks {
		for _, dep := range t.DependsOn {
			if !trackIDs[dep] {
				errs = append(errs, SchemaValidationError{
					Field:   "tracks[" + t.ID + "].depends_on",
					Message: "unknown dependency: " + dep,
				})
			}
		}
	}

	// Detect circular dependencies.
	if hasCycle(tmpl.Tracks) {
		errs = append(errs, SchemaValidationError{
			Field:   "tracks",
			Message: "circular dependency detected in track dependencies",
		})
	}

	// Stories validation.
	for _, s := range tmpl.Stories {
		if !trackIDs[s.Track] {
			errs = append(errs, SchemaValidationError{
				Field:   "stories[" + s.ID + "].track",
				Message: "unknown track: " + s.Track,
			})
		}
		// Validate expand_dimensions reference known dimensions or special names.
		for _, dim := range s.ExpandDimensions {
			if dim == "environment" || dim == "track_ref" {
				continue
			}
			if _, ok := tmpl.Dimensions[dim]; !ok {
				errs = append(errs, SchemaValidationError{
					Field:   "stories[" + s.ID + "].expand_dimensions",
					Message: "unknown dimension: " + dim,
				})
			}
		}
	}

	// Columns validation.
	if len(tmpl.Columns) == 0 {
		errs = append(errs, SchemaValidationError{Field: "columns", Message: "at least one column required"})
	}

	// Phases validation.
	if len(tmpl.Phases) == 0 {
		errs = append(errs, SchemaValidationError{Field: "phases", Message: "at least one phase required"})
	}
	phaseIDs := make(map[string]bool)
	for _, p := range tmpl.Phases {
		phaseIDs[p.ID] = true
	}
	for _, t := range tmpl.Tracks {
		if t.Phase != "" && !phaseIDs[t.Phase] {
			errs = append(errs, SchemaValidationError{
				Field:   "tracks[" + t.ID + "].phase",
				Message: "unknown phase: " + t.Phase,
			})
		}
	}

	return errs
}

// hasCycle detects circular dependencies in track dependency graph.
func hasCycle(tracks []Track) bool {
	adj := make(map[string][]string)
	for _, t := range tracks {
		adj[t.ID] = t.DependsOn
	}

	// DFS-based cycle detection.
	white := 0 // not visited
	gray := 1  // in current path
	black := 2 // fully processed

	color := make(map[string]int)
	for _, t := range tracks {
		color[t.ID] = white
	}

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		for _, dep := range adj[node] {
			if color[dep] == gray {
				return true // Cycle detected.
			}
			if color[dep] == white {
				if dfs(dep) {
					return true
				}
			}
		}
		color[node] = black
		return false
	}

	for _, t := range tracks {
		if color[t.ID] == white {
			if dfs(t.ID) {
				return true
			}
		}
	}
	return false
}

// ListTemplates returns all available template project types.
func (e *Engine) ListTemplates() ([]TemplateListEntry, error) {
	entries, err := os.ReadDir(e.templateDir)
	if err != nil {
		return nil, fmt.Errorf("read template dir: %w", err)
	}

	var result []TemplateListEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		if entry.Name() == "schema.yaml" {
			continue
		}

		path := filepath.Join(e.templateDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var tmpl struct {
			Meta TemplateMeta `yaml:"meta"`
		}
		if err := yaml.Unmarshal(data, &tmpl); err != nil {
			continue
		}

		projType := strings.TrimSuffix(entry.Name(), ".yaml")
		result = append(result, TemplateListEntry{
			ProjectType: projType,
			Description: tmpl.Meta.Description,
			Version:     tmpl.Meta.Version,
			Path:        path,
		})
	}

	return result, nil
}

// LoadIntake parses an intake YAML file.
func LoadIntake(path string) (*IntakeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read intake file: %w", err)
	}

	var intake IntakeFile
	if err := yaml.Unmarshal(data, &intake); err != nil {
		return nil, fmt.Errorf("parse intake file: %w", err)
	}

	if intake.Intake.ProjectName == "" {
		return nil, fmt.Errorf("intake file missing required field: intake.project_name")
	}

	return &intake, nil
}

// ValidateIntakeAgainstTemplate checks that intake dimension selections reference
// values that exist in the merged template dimensions.
func ValidateIntakeAgainstTemplate(intake *IntakeFile, tmpl *BoardTemplate) []string {
	var warnings []string

	check := func(selected []string, dimName string) {
		dimValues, ok := tmpl.Dimensions[dimName]
		if !ok {
			return
		}
		known := make(map[string]bool)
		for _, dv := range dimValues {
			known[dv.ID] = true
		}
		for _, sel := range selected {
			if !known[sel] {
				warnings = append(warnings, fmt.Sprintf("intake references unknown %s value: %s", dimName, sel))
			}
		}
	}

	check(intake.Intake.CloudProviders, "cloud_provider")
	check(intake.Intake.OperatingSystems, "os")
	check(intake.Intake.Databases, "database")
	check(intake.Intake.AppArchitectures, "app_architecture")
	check(intake.Intake.StorageSystems, "storage")

	return warnings
}
