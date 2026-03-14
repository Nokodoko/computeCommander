// Package jiraboard implements a template engine for generating Jira boards
// from YAML meta-templates with dimension-based matrix expansion.
package jiraboard

// TemplateMeta holds metadata about a board template.
type TemplateMeta struct {
	Version          string `yaml:"version"`
	ProjectType      string `yaml:"project_type"`
	Description      string `yaml:"description"`
	DefaultProjectKey string `yaml:"default_project_key"`
	DefaultBoardType string `yaml:"default_board_type"`
}

// DimensionValue represents a single value within a dimension (e.g., one database type).
// Fields beyond ID and Label are dimension-specific and stored in Extra.
type DimensionValue struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`

	// Dimension-specific fields (integration_path, required_params, etc.)
	// Populated from YAML via custom unmarshalling.
	Extra map[string]any `yaml:"-"`
}

// IntegrationPath returns the integration_path field if present.
func (d DimensionValue) IntegrationPath() string {
	if v, ok := d.Extra["integration_path"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ConfSpec returns the conf_spec field if present.
func (d DimensionValue) ConfSpec() string {
	if v, ok := d.Extra["conf_spec"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RequiredParams returns the required_params field if present.
func (d DimensionValue) RequiredParams() []string {
	if v, ok := d.Extra["required_params"]; ok {
		if arr, ok := v.([]any); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

// OptionalParams returns the optional_params field if present.
func (d DimensionValue) OptionalParams() []string {
	if v, ok := d.Extra["optional_params"]; ok {
		if arr, ok := v.([]any); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

// AgentInstall returns the agent_install field if present.
func (d DimensionValue) AgentInstall() string {
	if v, ok := d.Extra["agent_install"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Track represents a work track that becomes a Jira epic.
type Track struct {
	ID          string     `yaml:"id"`
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Phase       string     `yaml:"phase"`
	DependsOn   []string   `yaml:"depends_on"`
	AppliesWhen AppliesWhen `yaml:"applies_when"`
}

// AppliesWhen defines conditional inclusion logic for tracks and tasks.
type AppliesWhen struct {
	Any       bool   `yaml:"any,omitempty"`
	Dimension string `yaml:"dimension,omitempty"`
	HasAny    bool   `yaml:"has_any,omitempty"`
	NotValue  string `yaml:"not_value,omitempty"`
}

// StoryTemplate defines a story within a track, with dimension expansion.
type StoryTemplate struct {
	Track              string            `yaml:"track"`
	ID                 string            `yaml:"id"`
	Name               string            `yaml:"name"`
	ExpandDimensions   []string          `yaml:"expand_dimensions"`
	ExcludeWhen        []map[string]string `yaml:"exclude_when"`
	DescriptionTemplate string           `yaml:"description_template"`
	Tasks              []TaskTemplate    `yaml:"tasks"`
}

// TaskTemplate defines a task within a story, with optional dimension expansion.
type TaskTemplate struct {
	ID                 string            `yaml:"id"`
	Name               string            `yaml:"name"`
	ExpandDimensions   []string          `yaml:"expand_dimensions"`
	AppliesWhen        map[string][]string `yaml:"applies_when"`
	DescriptionTemplate string           `yaml:"description_template"`
	Labels             []string          `yaml:"labels"`
	Priority           string            `yaml:"priority"`
}

// ExcludeRule maps dimension names to values that should be excluded.
type ExcludeRule = map[string]string

// Column represents a Jira board column.
type Column struct {
	Name           string  `yaml:"name"`
	StatusCategory string  `yaml:"status_category"`
	WIPLimit       *int    `yaml:"wip_limit"`
}

// Phase represents a workflow phase / swimlane.
type Phase struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Order int    `yaml:"order"`
}

// BoardTemplate is the top-level template structure loaded from YAML.
type BoardTemplate struct {
	Meta       TemplateMeta                `yaml:"meta"`
	Dimensions map[string][]DimensionValue `yaml:"dimensions"`
	Tracks     []Track                     `yaml:"tracks"`
	Stories    []StoryTemplate             `yaml:"stories"`
	Columns    []Column                    `yaml:"columns"`
	Phases     []Phase                     `yaml:"phases"`
}

// IntakeFile defines the client-specific pruning parameters.
type IntakeFile struct {
	Intake IntakeConfig `yaml:"intake"`
}

// IntakeConfig holds the intake configuration for dimension pruning.
type IntakeConfig struct {
	ProjectName      string            `yaml:"project_name"`
	ProjectKey       string            `yaml:"project_key"`
	Environments     []string          `yaml:"environments"`
	CloudProviders   []string          `yaml:"cloud_providers"`
	OperatingSystems []string          `yaml:"operating_systems"`
	Databases        []string          `yaml:"databases"`
	AppArchitectures []string          `yaml:"app_architectures"`
	StorageSystems   []string          `yaml:"storage_systems"`
	IncludePhases    []string          `yaml:"include_phases"`
	ExcludeTracks    []string          `yaml:"exclude_tracks"`
	ExcludeLabels    []string          `yaml:"exclude_labels"`
	CustomFields     map[string]string `yaml:"custom_fields"`
}

// ExpandedTicket represents a fully expanded ticket ready for Jira API publishing.
type ExpandedTicket struct {
	// Identity
	DeterministicKey string `json:"deterministic_key" yaml:"deterministic_key"`
	IssueType        string `json:"issue_type" yaml:"issue_type"` // "Epic", "Story", "Task"

	// Content
	Summary     string   `json:"summary" yaml:"summary"`
	Description string   `json:"description" yaml:"description"`
	Labels      []string `json:"labels" yaml:"labels"`
	Priority    string   `json:"priority" yaml:"priority"`

	// Hierarchy
	ParentKey string `json:"parent_key,omitempty" yaml:"parent_key,omitempty"`
	TrackID   string `json:"track_id" yaml:"track_id"`
	Phase     string `json:"phase" yaml:"phase"`

	// Dimensions used for this ticket
	Dimensions map[string]DimensionValue `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`

	// Dependencies
	DependsOn []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Blocks    []string `json:"blocks,omitempty" yaml:"blocks,omitempty"`
}

// PublishResult holds the outcome of publishing tickets to Jira.
type PublishResult struct {
	ProjectKey     string         `json:"project_key"`
	BoardName      string         `json:"board_name"`
	TicketsCreated int            `json:"tickets_created"`
	TicketsUpdated int            `json:"tickets_updated"`
	TicketsSkipped int            `json:"tickets_skipped"`
	Epics          int            `json:"epics"`
	Stories        int            `json:"stories"`
	Tasks          int            `json:"tasks"`
	Tracks         []TrackSummary `json:"tracks"`
	Errors         []string       `json:"errors,omitempty"`
}

// TrackSummary summarises a track in the publish result.
type TrackSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Phase       string `json:"phase"`
	TicketCount int    `json:"ticket_count"`
}

// PreviewResult holds the result of a template preview (no Jira API calls).
type PreviewResult struct {
	TotalTickets       int                `json:"total_tickets"`
	ByType             map[string]int     `json:"by_type"`
	ByPhase            map[string]int     `json:"by_phase"`
	DimensionsSelected map[string][]string `json:"dimensions_selected"`
}

// TemplateListEntry represents an available template for listing.
type TemplateListEntry struct {
	ProjectType string `json:"project_type"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Path        string `json:"path"`
}

// LogIntegration represents a log management integration from partials.
type LogIntegration struct {
	ID           string   `yaml:"id"`
	Label        string   `yaml:"label"`
	ConfigPath   string   `yaml:"config_path"`
	OSConstraint []string `yaml:"os_constraint"`
}

// LogIntegrationsPartial holds the log_integrations partial data.
type LogIntegrationsPartial struct {
	LogIntegrations []LogIntegration `yaml:"log_integrations"`
}

// DescriptionContext holds all data passed to description templates for rendering.
type DescriptionContext struct {
	// Ticket identity
	TicketID  string
	TrackID   string
	TrackName string
	Phase     string

	// Integration details (from dimension value)
	Integration struct {
		ID             string
		Label          string
		Path           string
		ConfSpec       string
		CheckDir       string
		RequiredParams []string
		OptionalParams []string
		DBM            bool
	}

	// Environment
	Environment string

	// Dimension values
	CloudProvider   DimensionValue
	OS              DimensionValue
	AppArchitecture DimensionValue

	// Context paragraph
	Context string

	// Labels
	Labels []string

	// Parameter descriptions (placeholder map)
	ParamDescriptions map[string]string
}

// SchemaValidationError represents a schema validation failure.
type SchemaValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e SchemaValidationError) Error() string {
	return e.Field + ": " + e.Message
}
