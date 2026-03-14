# Jira Board Generator — Datadog Onboarding Template System

Machine-readable YAML template engine for generating Jira boards that onboard corporate clients into Datadog. Produces agent-ready tickets with descriptions rich enough for one-shot `/spec --review --loop` execution. Go CLI integration via `cmdr jira-board`, YAML template storage, six-sigma parallelizable work tracks.

## 1. Why

Manual Jira board creation for Datadog client onboarding is slow, inconsistent, and produces tickets that lack the context agents need for one-shot execution:

- **No permutation handling.** Each client has a unique stack (cloud provider, OS, databases, app frameworks, storage systems). Manually creating tickets for every combination means missed integrations, duplicated effort, and inconsistent coverage. A single client onboarding with 15 integrations across 3 environments generates ~200 tickets — doing this by hand takes hours and guarantees drift.
- **Tickets lack agent context.** Current Jira tickets contain one-line summaries like "Set up PostgreSQL monitoring." An agent receiving this cannot generate a spec without extensive exploration. Tickets must contain: conf.yaml reference paths, required parameters, environment-specific configuration, acceptance criteria, and links to the `integrations-core`/`integrations-extras` repos.
- **No pipeline integration.** The `/sr` pipeline (`/spec --review --loop`) needs tickets with structured descriptions that map directly to spec sections. Current tickets require human interpretation before an agent can act on them.
- **Inconsistent work decomposition.** Without a template, epics/stories/tasks are structured differently per engagement. Six-sigma principles demand parallelizable, predictable, isolated work tracks — the template enforces this structure.

The `/jira-board` command generates ~50-200 tickets from a single YAML template, with every ticket containing enough context for autonomous agent execution.

## 2. Design Principles

1. **Template-driven, not code-driven.** All board structure lives in YAML templates. Adding a new integration or workflow phase means editing YAML, not Go code. The engine is generic; the templates are specific.
2. **Permutation-aware.** Templates define a matrix of dimensions (cloud provider, OS, database, app architecture, storage). The engine expands the matrix into concrete tickets, pruning irrelevant combinations. A PostgreSQL ticket for AWS Linux differs from one for Azure Windows — the template captures both.
3. **Agent-ready descriptions.** Every generated ticket description contains: (a) a one-paragraph context statement, (b) the conf.yaml reference path in `integrations-core` or `integrations-extras`, (c) required and optional parameters with types, (d) environment-specific notes, (e) acceptance criteria as a checkbox list, and (f) a `## Spec Hints` section with file paths and data model references for the `/spec` builder.
4. **Six-sigma work tracks.** Tickets are organized into parallelizable tracks (epics) with explicit dependencies. No ticket in track A depends on a ticket in track B unless the dependency is declared in the template. This enables `/loop` fan-out execution.
5. **Intake-pruned.** The `/org-generator` pipeline passes an intake file that prunes the template to only the integrations, environments, and phases relevant to the specific client. The full template covers all integrations cataloged in `_partials/`; a typical client uses 10-20.
6. **Idempotent generation.** Running `/jira-board` twice with the same intake produces the same board. Existing tickets are matched by a deterministic key (`{project}-{epic}-{integration}-{environment}`) and updated rather than duplicated.
7. **Hierarchical: Epic > Story > Task.** Epics represent work tracks (e.g., "Database Monitoring", "APM Instrumentation"). Stories represent integrations within a track (e.g., "PostgreSQL Monitoring"). Tasks represent environment-specific work items (e.g., "PostgreSQL conf.yaml for prod-us-east-1").
8. **Partials are the single source of truth for dimension data.** The `org-generator.yaml` template file is a slim orchestration file that defines tracks, stories, phases, and columns. It does NOT inline dimension catalog data. All dimension values (database definitions, cloud provider details, OS variants, etc.) live exclusively in `_partials/*.yaml` files. During `LoadTemplate()`, the engine auto-discovers all `*.yaml` files in the `_partials/` directory and merges their `dimensions:` maps into the loaded template. This eliminates data duplication and makes partials independently maintainable. *(Resolves former Open Question #1.)*

## 3. On-Disk Format

```
computeCommander/
  templates/
    jira-board/
      schema.yaml                    # JSON Schema for template validation
      org-generator.yaml             # Slim orchestration template (no inline dimensions)
      _partials/
        integrations-db.yaml         # Database integration dimension catalog
        integrations-apm.yaml        # APM integration dimension catalog
        integrations-infra.yaml      # Infrastructure integration dimension catalog
        integrations-log.yaml        # Log management dimension catalog
        integrations-cloud.yaml      # Cloud provider dimension catalog
        description-templates/
          integration-task.md.tmpl   # Go text/template for task descriptions
          integration-story.md.tmpl  # Go text/template for story descriptions
          epic-summary.md.tmpl       # Go text/template for epic descriptions
  internal/
    commands/
      jira_board.go                  # CLI command handler
    jiraboard/
      engine.go                      # Template engine: load, merge partials, validate
      expander.go                    # Matrix expansion: dimensions -> tickets
      renderer.go                    # Description renderer using text/template
      publisher.go                   # Jira API publisher: create/update issues
      types.go                       # Data types for templates and tickets
      engine_test.go                 # Engine unit tests
      expander_test.go               # Expander unit tests
  .claude/
    commands/
      jira-board.md                  # Skill definition (updated)
```

### org-generator.yaml

The primary template file. Defines the board structure (tracks, stories, phases, columns) but does NOT contain dimension catalog data. Dimension data is loaded from `_partials/*.yaml` files and merged by the engine at load time.

The `dimensions:` section in this file declares which dimension keys are used by the template. The actual dimension values (with `id`, `label`, `integration_path`, `required_params`, etc.) are defined exclusively in the partial files.

```yaml
# Datadog Client Onboarding — Jira Board Template
# Version: 1.0.0
# Project Type: org-generator
#
# NOTE: This is a slim orchestration file.
# Dimension catalog data lives in _partials/*.yaml and is merged at load time.
# Do NOT add dimension values here — edit the corresponding partial file instead.

meta:
  version: "1.0.0"
  project_type: "org-generator"
  description: "Datadog onboarding board for corporate clients"
  default_project_key: "DD"
  default_board_type: "kanban"

# Dimension keys used by this template.
# Actual values are loaded from _partials/*.yaml during LoadTemplate().
# This section declares the expected dimension names for schema validation.
dimensions:
  cloud_provider: []   # Populated from integrations-cloud.yaml
  os: []               # Populated from integrations-infra.yaml
  database: []         # Populated from integrations-db.yaml
  app_architecture: [] # Populated from integrations-apm.yaml
  storage: []          # Populated from integrations-cloud.yaml

# Work tracks — epics that organize the board
tracks:
  - id: "agent-deploy"
    name: "Agent Deployment"
    description: "Install and configure the Datadog Agent across all target hosts"
    phase: "foundation"
    depends_on: []
    applies_when:
      any: true

  - id: "infra-monitoring"
    name: "Infrastructure Monitoring"
    description: "Configure system-level checks: CPU, memory, disk, network, process"
    phase: "foundation"
    depends_on: ["agent-deploy"]
    applies_when:
      any: true

  - id: "db-monitoring"
    name: "Database Monitoring"
    description: "Configure database integrations with DBM where supported"
    phase: "integrations"
    depends_on: ["agent-deploy"]
    applies_when:
      dimension: "database"
      has_any: true

  - id: "apm-instrumentation"
    name: "APM Instrumentation"
    description: "Instrument application code with Datadog tracing libraries"
    phase: "integrations"
    depends_on: ["agent-deploy"]
    applies_when:
      dimension: "app_architecture"
      not_value: "serverless"

  - id: "log-management"
    name: "Log Management"
    description: "Configure log collection, parsing pipelines, and log-to-metric rules"
    phase: "integrations"
    depends_on: ["agent-deploy"]
    applies_when:
      any: true

  - id: "cloud-integrations"
    name: "Cloud Provider Integrations"
    description: "Configure cloud provider API integrations (AWS/Azure/GCP)"
    phase: "integrations"
    depends_on: []
    applies_when:
      dimension: "cloud_provider"
      not_value: "on-prem"

  - id: "monitors-dashboards"
    name: "Monitors & Dashboards"
    description: "Create monitors, dashboards, and SLOs for all configured integrations"
    phase: "observability"
    depends_on: ["infra-monitoring", "db-monitoring", "apm-instrumentation", "log-management"]
    applies_when:
      any: true

  - id: "validation"
    name: "Validation & Handoff"
    description: "Validate all integrations report data, run smoke tests, handoff to client"
    phase: "finalization"
    depends_on: ["monitors-dashboards", "cloud-integrations"]
    applies_when:
      any: true

# Story templates within each track
stories:
  - track: "agent-deploy"
    id: "agent-install-{os}-{cloud_provider}"
    name: "Install Datadog Agent on {os.label} ({cloud_provider.label})"
    expand_dimensions: ["os", "cloud_provider"]
    exclude_when:
      - os: "macos"
        cloud_provider: "on-prem"
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "agent-install-{os}-{cloud_provider}-{environment}"
        name: "Agent install: {os.label} / {cloud_provider.label} / {environment}"
        expand_dimensions: ["environment"]
        description_template: "integration-task.md.tmpl"
        labels: ["agent-deploy", "{os.id}", "{cloud_provider.id}"]
        priority: "High"

  - track: "db-monitoring"
    id: "db-{database}-setup"
    name: "Configure {database.label} Monitoring"
    expand_dimensions: ["database"]
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "db-{database}-conf-{environment}"
        name: "{database.label} conf.yaml for {environment}"
        expand_dimensions: ["environment"]
        description_template: "integration-task.md.tmpl"
        labels: ["database", "{database.id}", "conf-yaml"]
        priority: "High"
      - id: "db-{database}-dbm-{environment}"
        name: "{database.label} DBM setup for {environment}"
        expand_dimensions: ["environment"]
        applies_when:
          database: ["postgres", "mysql", "sqlserver"]
        description_template: "integration-task.md.tmpl"
        labels: ["database", "{database.id}", "dbm"]
        priority: "Medium"

  - track: "infra-monitoring"
    id: "infra-system-checks"
    name: "System-Level Checks (CPU, Memory, Disk, Network)"
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "infra-process-check-{os}"
        name: "Process check conf.yaml for {os.label}"
        expand_dimensions: ["os"]
        description_template: "integration-task.md.tmpl"
        labels: ["infrastructure", "process-check", "{os.id}"]
        priority: "Medium"
      - id: "infra-network-check-{os}"
        name: "Network check conf.yaml for {os.label}"
        expand_dimensions: ["os"]
        description_template: "integration-task.md.tmpl"
        labels: ["infrastructure", "network-check", "{os.id}"]
        priority: "Medium"

  - track: "apm-instrumentation"
    id: "apm-{app_architecture}-setup"
    name: "APM for {app_architecture.label} Applications"
    expand_dimensions: ["app_architecture"]
    exclude_when:
      - app_architecture: "serverless"
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "apm-{app_architecture}-tracing-{environment}"
        name: "Tracing library setup: {app_architecture.label} / {environment}"
        expand_dimensions: ["environment"]
        description_template: "integration-task.md.tmpl"
        labels: ["apm", "{app_architecture.id}"]
        priority: "High"

  - track: "log-management"
    id: "log-collection-{os}"
    name: "Log Collection for {os.label}"
    expand_dimensions: ["os"]
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "log-agent-config-{os}-{environment}"
        name: "Log agent config: {os.label} / {environment}"
        expand_dimensions: ["environment"]
        description_template: "integration-task.md.tmpl"
        labels: ["logs", "{os.id}"]
        priority: "Medium"

  - track: "cloud-integrations"
    id: "cloud-{cloud_provider}-api"
    name: "{cloud_provider.label} API Integration"
    expand_dimensions: ["cloud_provider"]
    exclude_when:
      - cloud_provider: "on-prem"
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "cloud-{cloud_provider}-iam"
        name: "{cloud_provider.label} IAM/Service Account Setup"
        description_template: "integration-task.md.tmpl"
        labels: ["cloud", "{cloud_provider.id}", "iam"]
        priority: "High"
      - id: "cloud-{cloud_provider}-metrics"
        name: "{cloud_provider.label} Metrics Collection"
        description_template: "integration-task.md.tmpl"
        labels: ["cloud", "{cloud_provider.id}", "metrics"]
        priority: "High"

  - track: "monitors-dashboards"
    id: "monitors-{track_ref}"
    name: "Monitors for {track_ref.name}"
    expand_dimensions: ["track_ref"]
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "monitor-create-{track_ref}"
        name: "Create monitors for {track_ref.name}"
        description_template: "integration-task.md.tmpl"
        labels: ["monitors", "{track_ref.id}"]
        priority: "Medium"
      - id: "dashboard-create-{track_ref}"
        name: "Create dashboard for {track_ref.name}"
        description_template: "integration-task.md.tmpl"
        labels: ["dashboards", "{track_ref.id}"]
        priority: "Low"

  - track: "validation"
    id: "validation-smoke-tests"
    name: "Smoke Tests & Data Validation"
    description_template: "integration-story.md.tmpl"
    tasks:
      - id: "validation-data-flowing"
        name: "Verify all integrations report data to Datadog"
        description_template: "integration-task.md.tmpl"
        labels: ["validation", "smoke-test"]
        priority: "Highest"
      - id: "validation-handoff"
        name: "Client handoff: documentation and access review"
        description_template: "integration-task.md.tmpl"
        labels: ["validation", "handoff"]
        priority: "Highest"

# Workflow columns (Jira board columns)
columns:
  - name: "Backlog"
    status_category: "To Do"
    wip_limit: null
  - name: "Spec Ready"
    status_category: "To Do"
    wip_limit: null
  - name: "In Progress"
    status_category: "In Progress"
    wip_limit: 8
  - name: "Review"
    status_category: "In Progress"
    wip_limit: 5
  - name: "Validation"
    status_category: "In Progress"
    wip_limit: 3
  - name: "Done"
    status_category: "Done"
    wip_limit: null

# Phases map to board swimlanes
phases:
  - id: "foundation"
    label: "Foundation"
    order: 1
  - id: "integrations"
    label: "Integrations"
    order: 2
  - id: "observability"
    label: "Observability"
    order: 3
  - id: "finalization"
    label: "Finalization"
    order: 4
```

### _partials/integrations-db.yaml

Partial file containing the database dimension catalog. Merged into the main template's `dimensions.database` during `LoadTemplate()`.

```yaml
# Database Integration Dimension Catalog
# Merged into org-generator.yaml dimensions.database during LoadTemplate()

dimensions:
  database:
    - id: "postgres"
      label: "PostgreSQL"
      integration_path: "integrations-core/postgres"
      conf_spec: "assets/configuration/spec.yaml"
      required_params: ["host", "port", "username", "password", "dbname"]
      optional_params: ["ssl", "reported_hostname", "dbm", "collect_activity_metrics"]
    - id: "mysql"
      label: "MySQL"
      integration_path: "integrations-core/mysql"
      conf_spec: "assets/configuration/spec.yaml"
      required_params: ["host", "port", "username", "password"]
      optional_params: ["ssl", "dbm", "reported_hostname"]
    - id: "sqlserver"
      label: "SQL Server"
      integration_path: "integrations-core/sqlserver"
      conf_spec: "assets/configuration/spec.yaml"
      required_params: ["host", "username", "password"]
      optional_params: ["database", "adoprovider"]
    - id: "mongodb"
      label: "MongoDB"
      integration_path: "integrations-core/mongo"
      conf_spec: "assets/configuration/spec.yaml"
      required_params: ["hosts", "username", "password"]
      optional_params: ["database", "replica_check", "ssl"]
    - id: "redis"
      label: "Redis"
      integration_path: "integrations-core/redisdb"
      conf_spec: "assets/configuration/spec.yaml"
      required_params: ["host", "port"]
      optional_params: ["password", "slowlog-max-len"]
    - id: "elasticsearch"
      label: "Elasticsearch"
      integration_path: "integrations-core/elastic"
      conf_spec: "assets/configuration/spec.yaml"
      required_params: ["url"]
      optional_params: ["username", "password", "ssl_verify", "cluster_stats"]
```

### _partials/integrations-cloud.yaml

Partial file containing cloud provider and storage dimension catalogs.

```yaml
# Cloud Provider + Storage Dimension Catalog
# Merged into org-generator.yaml dimensions.cloud_provider and dimensions.storage

dimensions:
  cloud_provider:
    - id: "aws"
      label: "AWS"
      services: ["ec2", "rds", "ecs", "eks", "lambda", "s3", "cloudwatch"]
    - id: "azure"
      label: "Azure"
      services: ["vm", "sql-db", "aks", "functions", "blob", "monitor"]
    - id: "gcp"
      label: "GCP"
      services: ["gce", "cloud-sql", "gke", "cloud-functions", "gcs", "stackdriver"]
    - id: "on-prem"
      label: "On-Premises"
      services: []

  storage:
    - id: "s3"
      label: "AWS S3"
      integration_path: "integrations-core/amazon_s3"
    - id: "blob"
      label: "Azure Blob"
      integration_path: "integrations-extras/azure_blob_storage"
    - id: "gcs"
      label: "Google Cloud Storage"
      integration_path: "integrations-core/google_cloud_platform"
    - id: "nfs"
      label: "NFS/Local"
      integration_path: null
```

### _partials/integrations-infra.yaml

Partial file containing OS dimension catalog and infrastructure check references.

```yaml
# OS + Infrastructure Dimension Catalog
# Merged into org-generator.yaml dimensions.os

dimensions:
  os:
    - id: "linux"
      label: "Linux"
      variants: ["ubuntu-22.04", "rhel-9", "amazon-linux-2023"]
      agent_install: "DD_API_KEY=<key> DD_SITE=<site> bash -c \"$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)\""
    - id: "windows"
      label: "Windows"
      variants: ["server-2022", "server-2019"]
      agent_install: "Start-Process -Wait msiexec -ArgumentList '/qn /i datadog-agent-7-latest.amd64.msi APIKEY=<key> SITE=<site>'"
    - id: "macos"
      label: "macOS"
      variants: ["14-sonoma"]
      agent_install: "DD_API_KEY=<key> DD_SITE=<site> bash -c \"$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)\""
```

### _partials/integrations-apm.yaml

Partial file containing app architecture dimension catalog.

```yaml
# App Architecture Dimension Catalog
# Merged into org-generator.yaml dimensions.app_architecture

dimensions:
  app_architecture:
    - id: "containerized"
      label: "Containerized (Docker/K8s)"
      agent_type: "datadog-agent (DaemonSet)"
      autodiscovery: true
    - id: "vm-based"
      label: "VM-Based"
      agent_type: "datadog-agent (host)"
      autodiscovery: false
    - id: "serverless"
      label: "Serverless"
      agent_type: "datadog-lambda-extension"
      autodiscovery: false
    - id: "hybrid"
      label: "Hybrid"
      agent_type: "mixed"
      autodiscovery: true
```

### _partials/integrations-log.yaml

Partial file containing log management integration references. This partial does not define a new dimension — it provides supplementary metadata used by log management story/task templates.

```yaml
# Log Management Integration Catalog
# Supplementary metadata for log-management track templates

log_integrations:
  - id: "journald"
    label: "systemd Journal"
    config_path: "/etc/datadog-agent/conf.d/journald.d/conf.yaml"
    os_constraint: ["linux"]
  - id: "windows-event-log"
    label: "Windows Event Log"
    config_path: "C:\\ProgramData\\Datadog\\conf.d\\win32_event_log.d\\conf.yaml"
    os_constraint: ["windows"]
  - id: "file-tailing"
    label: "File Tailing"
    config_path: "/etc/datadog-agent/conf.d/<integration>.d/conf.yaml"
    os_constraint: ["linux", "macos"]
```

### integration-task.md.tmpl

Go `text/template` file that renders agent-ready task descriptions. Uses the expanded dimension context to produce rich, actionable descriptions.

```
## Context

{{ .Context }}

## Integration Reference

- **Integration**: {{ .Integration.Label }}
- **Repository**: `~/Programs/{{ .Integration.Path }}`
- **Conf Spec**: `{{ .Integration.Path }}/{{ .Integration.ConfSpec }}`
- **Environment**: {{ .Environment }}
- **Cloud Provider**: {{ .CloudProvider.Label }}
- **OS**: {{ .OS.Label }}
- **Architecture**: {{ .AppArchitecture.Label }}

## Required Parameters

{{ range .Integration.RequiredParams -}}
- `{{ . }}`: {{ index $.ParamDescriptions . }}
{{ end }}

## Optional Parameters

{{ range .Integration.OptionalParams -}}
- `{{ . }}`: {{ index $.ParamDescriptions . }}
{{ end }}

## Acceptance Criteria

- [ ] conf.yaml written to `/etc/datadog-agent/conf.d/{{ .Integration.ID }}.d/conf.yaml`
- [ ] `datadog-agent check {{ .Integration.ID }}` exits 0
- [ ] Metrics visible in Datadog UI within 5 minutes
- [ ] No errors in `/var/log/datadog/agent.log` related to {{ .Integration.Label }}
{{ if .Integration.DBM -}}
- [ ] DBM query samples visible in Datadog UI
{{ end }}

## Spec Hints

Use these references when generating a spec via `/spec`:

- **conf.yaml spec**: Read `~/Programs/{{ .Integration.Path }}/{{ .Integration.ConfSpec }}` for all parameter definitions
- **Check source**: `~/Programs/{{ .Integration.Path }}/datadog_checks/{{ .Integration.CheckDir }}/`
- **Example conf**: `~/Programs/{{ .Integration.Path }}/datadog_checks/{{ .Integration.CheckDir }}/data/conf.yaml.example`
- **Agent install command**: `{{ .OS.AgentInstall }}`

## Labels

{{ range .Labels -}}
`{{ . }}` {{ end }}
```

### schema.yaml

JSON Schema (in YAML syntax) for validating template files before expansion.

```yaml
type: object
required: [meta, dimensions, tracks, stories, columns, phases]
properties:
  meta:
    type: object
    required: [version, project_type, description]
    properties:
      version: { type: string, pattern: "^\\d+\\.\\d+\\.\\d+$" }
      project_type: { type: string }
      description: { type: string }
      default_project_key: { type: string, pattern: "^[A-Z]{2,10}$" }
      default_board_type: { type: string, enum: ["kanban", "scrum"] }
  dimensions:
    type: object
    additionalProperties:
      type: array
      items:
        type: object
        required: [id, label]
  tracks:
    type: array
    items:
      type: object
      required: [id, name, description, phase]
      properties:
        id: { type: string }
        name: { type: string }
        description: { type: string }
        phase: { type: string }
        depends_on: { type: array, items: { type: string } }
        applies_when: { type: object }
  stories:
    type: array
    items:
      type: object
      required: [track, id, name]
  columns:
    type: array
    items:
      type: object
      required: [name, status_category]
  phases:
    type: array
    items:
      type: object
      required: [id, label, order]
```

## 4. Data Model

### BoardTemplate

```typescript
interface BoardTemplate {
  // Metadata
  meta: TemplateMeta;

  // Permutation matrix — populated by merging partials into declared keys
  dimensions: Record<string, DimensionValue[]>;

  // Work track definitions (become epics)
  tracks: Track[];

  // Story templates within tracks
  stories: StoryTemplate[];

  // Board column configuration
  columns: Column[];

  // Phase/swimlane definitions
  phases: Phase[];
}

interface TemplateMeta {
  version: string;           // "1.0.0"
  project_type: string;      // "org-generator"
  description: string;       // Human-readable
  default_project_key: string; // "DD"
  default_board_type: string;  // "kanban" | "scrum"
}

interface DimensionValue {
  id: string;                // "postgres", "aws", "linux"
  label: string;             // "PostgreSQL", "AWS", "Linux"
  [key: string]: any;        // Dimension-specific fields (integration_path, required_params, etc.)
}
```

### Track (becomes Jira Epic)

```typescript
interface Track {
  id: string;                // "db-monitoring"
  name: string;              // "Database Monitoring"
  description: string;       // Rich description for the epic
  phase: string;             // "integrations" — maps to swimlane
  depends_on: string[];      // ["agent-deploy"] — track-level dependencies
  applies_when: AppliesWhen; // Conditional inclusion based on intake
}

interface AppliesWhen {
  any?: boolean;             // Always applies
  dimension?: string;        // Dimension name to check
  has_any?: boolean;         // True if dimension has any selected values
  not_value?: string;        // Exclude specific dimension value
}
```

### StoryTemplate (becomes Jira Story)

```typescript
interface StoryTemplate {
  track: string;             // Parent track ID
  id: string;                // Template ID with {dimension} placeholders
  name: string;              // Template name with {dimension.label} placeholders
  expand_dimensions: string[]; // Dimensions to expand over
  exclude_when?: ExcludeRule[]; // Combinations to skip
  description_template: string; // Path to .md.tmpl file
  tasks: TaskTemplate[];     // Child task templates
}

interface TaskTemplate {
  id: string;                // Template ID with {dimension} placeholders
  name: string;              // Template name with {dimension.label} placeholders
  expand_dimensions?: string[]; // Additional dimensions to expand
  applies_when?: Record<string, string[]>; // Conditional inclusion
  description_template: string; // Path to .md.tmpl file
  labels: string[];          // Labels with {dimension.id} placeholders
  priority: string;          // "Highest" | "High" | "Medium" | "Low" | "Lowest"
}

interface ExcludeRule {
  [dimension: string]: string; // e.g., { os: "macos", cloud_provider: "on-prem" }
}
```

### ExpandedTicket (ready for Jira API)

```typescript
interface ExpandedTicket {
  // Identity
  deterministic_key: string; // "{project}-{track}-{integration}-{environment}"
  issue_type: "Epic" | "Story" | "Task";

  // Content
  summary: string;           // Expanded from template (no placeholders)
  description: string;       // Rendered from .md.tmpl with full context
  labels: string[];          // Expanded labels
  priority: string;          // Jira priority name

  // Hierarchy
  parent_key?: string;       // Parent ticket deterministic key
  track_id: string;          // Source track ID
  phase: string;             // "foundation" | "integrations" | "observability" | "finalization"

  // Dimensions used
  dimensions: Record<string, DimensionValue>; // The specific dimension values for this ticket

  // Dependencies
  depends_on: string[];      // Deterministic keys of dependency tickets
  blocks: string[];          // Deterministic keys of tickets this blocks
}
```

### IntakeFile

```typescript
interface IntakeFile {
  intake: {
    project_name: string;          // "Acme Corp Datadog Onboarding"
    project_key?: string;          // "ACME" — override default_project_key
    environments: string[];        // ["prod-us-east-1", "staging-us-east-1", "dev"]

    // Dimension selections (prune the matrix)
    cloud_providers: string[];     // ["aws"] — subset of dimensions.cloud_provider
    operating_systems: string[];   // ["linux"] — subset of dimensions.os
    databases: string[];           // ["postgres", "redis"] — subset of dimensions.database
    app_architectures: string[];   // ["containerized"] — subset of dimensions.app_architecture
    storage_systems: string[];     // ["s3"] — subset of dimensions.storage

    // Phase/track pruning
    include_phases?: string[];     // ["foundation", "integrations"] — skip observability/finalization
    exclude_tracks?: string[];     // ["apm-instrumentation"] — skip specific tracks
    exclude_labels?: string[];     // ["optional"] — prune tasks with these labels

    // Custom fields for template interpolation
    custom_fields?: Record<string, string>;
  };
}
```

### The `environment` Pseudo-Dimension

The `environments` list from the `IntakeFile` is treated as an implicit dimension during expansion. It is NOT defined in the template's `dimensions:` map or in any partial file. The expander injects intake environments as the `environment` dimension when resolving task templates that include `"environment"` in their `expand_dimensions` list.

This means: when a task template declares `expand_dimensions: ["environment"]`, the expander iterates over the intake's `environments` array and produces one ticket per environment. The `{environment}` placeholder in task IDs and names is resolved to the raw environment string (e.g., `"prod-us-east-1"`), not to a `DimensionValue` with `id`/`label` structure.

Implementation: during `Expand()`, if `expand_dimensions` contains `"environment"` and no matching key exists in `template.dimensions`, the expander falls back to `intake.environments` as the value source. Each environment string is used directly — no `.id`/`.label` access is available for environment (use `{environment}` not `{environment.label}`).

### ID Generation

- **Deterministic keys**: `{project_key}-{track_id}-{story_id_expanded}-{task_id_expanded}` (e.g., `DD-db-monitoring-db-postgres-setup-db-postgres-conf-prod-us-east-1`)
- **Collision handling**: Keys are deterministic by construction — same input always produces the same key. If a Jira issue with the matching key exists (stored in a custom field or label), update rather than create.
- **Rationale**: Deterministic keys enable idempotent board generation. Running `/jira-board` twice with the same intake must not create duplicate tickets.

### Ticket Lifecycle

```
Backlog ──> Spec Ready ──> In Progress ──> Review ──> Validation ──> Done
   ^                           |               |
   +---------------------------+               |
   (blocked/rework)            +---------------+
                               (review failed)
```

### Priority Scale

| Value | Label | Use |
|-------|-------|-----|
| Highest | Highest | Validation smoke tests, client handoff |
| High | High | Agent deployment, conf.yaml creation, IAM setup |
| Medium | Medium | DBM setup, process/network checks, monitors |
| Low | Low | Dashboard creation, optional integrations |
| Lowest | Lowest | Stretch goals, nice-to-have enhancements |

## 5. CLI

Binary name: `cmdr` (existing ComputeCommander binary).

### jira-board

```
cmdr jira-board generate                   Generate board from template
  --project-type <type>   (required)       Template to use (default: org-generator)
  --intake <file>         (required)       YAML intake file for dimension pruning
  --project-key <KEY>                      Override Jira project key from template
  --dry-run                                Preview tickets as YAML without Jira API calls
  --output <file>                          Write expanded tickets to file (implies --dry-run)
  --instance <name>                        Jira instance name from cmdr config

cmdr jira-board validate <template>        Validate a template file against schema
  --strict                                 Treat warnings as errors

cmdr jira-board list                       List available project types

cmdr jira-board preview                    Preview ticket count and structure
  --project-type <type>   (required)
  --intake <file>         (required)

cmdr jira-board delete <project-key>       Delete all board tickets (destructive)
  --confirm                                Skip confirmation prompt
```

## 6. JSON Output Format

Success (generate):

```json
{
  "success": true,
  "command": "jira-board generate",
  "project_key": "ACME",
  "board_name": "Acme Corp Datadog Onboarding",
  "tickets_created": 47,
  "tickets_updated": 3,
  "tickets_skipped": 0,
  "epics": 8,
  "stories": 15,
  "tasks": 27,
  "tracks": [
    {
      "id": "agent-deploy",
      "name": "Agent Deployment",
      "phase": "foundation",
      "ticket_count": 4
    }
  ]
}
```

Error:

```json
{
  "success": false,
  "command": "jira-board generate",
  "error": "intake file missing required field: intake.project_name",
  "intake_path": "./intake.yaml"
}
```

Preview:

```json
{
  "success": true,
  "command": "jira-board preview",
  "total_tickets": 47,
  "by_type": { "Epic": 8, "Story": 15, "Task": 24 },
  "by_phase": { "foundation": 12, "integrations": 25, "observability": 6, "finalization": 4 },
  "dimensions_selected": {
    "cloud_provider": ["aws"],
    "os": ["linux"],
    "database": ["postgres", "redis"],
    "app_architecture": ["containerized"],
    "storage": ["s3"]
  }
}
```

## 7. Concurrency Model

Rate-Limited Sequential Publishing

```
Rate limit:   10 requests/second (Jira Cloud default)
Retry:        Exponential backoff on 429 (uses existing RateLimiter)
Timeout:      30 seconds per API call
```

Implementation:

1. Template loading and expansion happen in-memory (no concurrency concerns)
2. Jira API publishing uses the existing `jira.Client` with its built-in `RateLimiter`
3. Tickets are published in dependency order: epics first, then stories, then tasks
4. Each publish call returns the Jira issue key, which is used as the parent for child issues
5. If a publish fails mid-stream, already-created tickets remain in Jira (idempotent re-run will update them)

### Atomic Writes

Not applicable at the Jira API level. Board generation is not transactional — partial boards are acceptable because re-running is idempotent. Local file writes (expanded ticket YAML for `--output`) use temp file + rename.

### Conflict Resolution

Deterministic key matching. Before creating a ticket, search for existing issues with a matching label `cmdr-key:{deterministic_key}`. If found, update the existing issue instead of creating a new one.

## 8. Migration

Not applicable — this is a new feature with no predecessor system to migrate from. The existing `/jira-board` skill definition (`~/.claude/commands/jira-board.md`) will be updated to reference the new template system, but it defines the Claude Code skill interface, not a data migration.

## 9. Integration

### /org-generator Pipeline

`/org-generator` invokes `/jira-board` as a hook after Terraform infrastructure is generated. The org-generator writes an intake YAML file and sets `$INTAKE_FILE` env var.

| org-generator Step | jira-board Action |
|--------------------|-------------------|
| Generate Terraform | -- |
| Write intake YAML | -- |
| Invoke `/jira-board` | Load intake, expand template, publish to Jira |
| -- | Return board URL + ticket summary |

### /sr Pipeline (Spec-Review-Execute)

Each generated ticket is designed for consumption by `/sr` (`/spec --review --loop`). The ticket description's `## Spec Hints` section provides:

| Spec Hint | Purpose |
|-----------|---------|
| `conf.yaml spec` path | Grounds the spec in actual parameter definitions |
| `Check source` path | Points to the integration's Python check code |
| `Example conf` path | Provides a working conf.yaml example |
| `Agent install command` | OS-specific install command for the task |

### Agent-Facing Commands

```bash
# Generate board from intake
cmdr jira-board generate --project-type org-generator --intake ./intake.yaml --json

# Preview before generating
cmdr jira-board preview --project-type org-generator --intake ./intake.yaml --json

# Validate template
cmdr jira-board validate templates/jira-board/org-generator.yaml --json
```

### Hooks Integration

```json
{
  "hooks": {
    "PostOrgGenerate": [
      {
        "command": "cmdr jira-board generate --project-type org-generator --intake $INTAKE_FILE --json",
        "description": "Generate Jira board after org-generator completes"
      }
    ]
  }
}
```

## 10. What It Does NOT Do

Explicitly out of scope:

- **Board administration.** Does not manage Jira board settings (column order, swimlane config, board filters) beyond initial creation. Board layout customization is done in the Jira UI.
- **Sprint management.** Does not create or manage sprints. The template produces a kanban board by default; scrum sprint planning is a human activity.
- **Ticket state transitions.** Does not move tickets through workflow states. Agents transition tickets via the existing `cmdr jira` command after completing work.
- **User assignment.** Does not assign tickets to specific Jira users. Assignment happens during sprint planning or agent dispatch.
- **Integration code generation.** Does not generate conf.yaml files or Python check code. It generates tickets that describe the work; the `/sr` pipeline generates the specs and code.
- **Datadog API calls.** Does not interact with the Datadog API. It creates Jira tickets that describe Datadog configuration work.

## 11. Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Existing cmdr runtime (confirmed in `go.mod`: `go 1.25.0`) |
| Language | Go | Matches existing codebase |
| Template Engine | Go `text/template` | Already used in `templates/agent-overlay.tmpl`; handles `{{template}}` actions for description rendering |
| Template Format | YAML | Human-readable, supports complex nested structures |
| Schema Validation | `gopkg.in/yaml.v3` + custom validation | No external schema library needed for YAML validation |
| Jira API | `pkg/integrations/jira` | Existing client with rate limiting |
| Testing | Go `testing` | Standard library |
| CLI Framework | Cobra | Existing CLI framework |

## 12. Project Infrastructure

### Directory Structure

```
computeCommander/
  templates/
    jira-board/
      schema.yaml                            # Template schema definition
      org-generator.yaml                     # Slim orchestration template
      _partials/
        integrations-db.yaml                 # Database dimension catalog
        integrations-apm.yaml                # APM dimension catalog
        integrations-infra.yaml              # Infrastructure/OS dimension catalog
        integrations-log.yaml                # Log management catalog
        integrations-cloud.yaml              # Cloud provider + storage dimension catalog
        description-templates/
          integration-task.md.tmpl           # Task description template
          integration-story.md.tmpl          # Story description template
          epic-summary.md.tmpl               # Epic description template
  internal/
    jiraboard/
      engine.go                              # Template engine core (load + merge partials)
      expander.go                            # Dimension matrix expansion
      renderer.go                            # Description rendering
      publisher.go                           # Jira API publishing
      types.go                               # All data types
      engine_test.go                         # Engine tests
      expander_test.go                       # Expander tests
    commands/
      jira_board.go                          # Cobra command handler
```

### Version Management

Template version is tracked in `meta.version` within each template YAML file. The engine validates that the template version is compatible with the current engine version.

### CHANGELOG.md

Follows Keep a Changelog format. Template changes and engine changes are logged separately under `### Jira Board` sub-heading.

### CI Workflow

```yaml
name: CI
on: [push, pull_request]
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go test ./internal/jiraboard/...
      - run: go vet ./internal/jiraboard/...
```

### Scripts

```json
{
  "scripts": {
    "test-jiraboard": "go test ./internal/jiraboard/...",
    "validate-templates": "go run ./cmd/cc jira-board validate templates/jira-board/*.yaml"
  }
}
```

## 13. Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| Template engine (`internal/jiraboard/`) | 5 | ~920 |
| CLI command (`internal/commands/jira_board.go`) | 1 | ~200 |
| YAML templates (`templates/jira-board/org-generator.yaml`) | 1 | ~250 |
| Partial templates (`templates/jira-board/_partials/`) | 5 | ~400 |
| Description templates (`.md.tmpl`) | 3 | ~150 |
| Schema (`schema.yaml`) | 1 | ~80 |
| Tests | 2 | ~400 |
| Skill update (`jira-board.md`) | 1 | ~50 |
| **Total** | **19** | **~2,450** |

## 14. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | unix-coder | Create jiraboard types and data model | pkg/integrations/jira/types.go | internal/jiraboard/types.go | -- | `go build ./internal/jiraboard/...` |
| T2 | unix-coder | Implement template engine (load YAML, auto-discover and merge `_partials/*.yaml`, validate against schema) | templates/jira-board/schema.yaml, internal/jiraboard/types.go | internal/jiraboard/engine.go | T1, T9 | `go build ./internal/jiraboard/...` |
| T3 | unix-coder | Implement dimension matrix expander (with `environment` pseudo-dimension fallback to intake) | internal/jiraboard/types.go | internal/jiraboard/expander.go | T1 | `go build ./internal/jiraboard/...` |
| T4 | unix-coder | Implement description renderer using Go `text/template` | templates/jira-board/_partials/description-templates/, internal/jiraboard/types.go | internal/jiraboard/renderer.go | T1, T8 | `go build ./internal/jiraboard/...` |
| T5 | unix-coder | Implement Jira API publisher (create/update with idempotent deterministic keys) | pkg/integrations/jira/client.go, internal/jiraboard/types.go | internal/jiraboard/publisher.go | T1 | `go build ./internal/jiraboard/...` |
| T6 | unix-coder | Create slim org-generator.yaml template (tracks, stories, phases, columns — NO inline dimension data) | -- | templates/jira-board/org-generator.yaml | -- | `test -f templates/jira-board/org-generator.yaml && grep -q 'dimensions:' templates/jira-board/org-generator.yaml && ! grep -q 'integration_path' templates/jira-board/org-generator.yaml` |
| T7 | unix-coder | Create partial templates (integrations-db, apm, infra, log, cloud) with full dimension catalogs. **Prerequisite**: `~/Programs/integrations-core/` and `~/Programs/integrations-extras/` must be locally available for reference. If unavailable, use the dimension data from this spec's On-Disk Format section as the source of truth. | ~/Programs/integrations-core/, ~/Programs/integrations-extras/ | templates/jira-board/_partials/integrations-db.yaml, templates/jira-board/_partials/integrations-apm.yaml, templates/jira-board/_partials/integrations-infra.yaml, templates/jira-board/_partials/integrations-log.yaml, templates/jira-board/_partials/integrations-cloud.yaml | T6 | `ls templates/jira-board/_partials/*.yaml | wc -l | grep -q 5` |
| T8 | unix-coder | Create description .md.tmpl files (task, story, epic) | -- | templates/jira-board/_partials/description-templates/integration-task.md.tmpl, templates/jira-board/_partials/description-templates/integration-story.md.tmpl, templates/jira-board/_partials/description-templates/epic-summary.md.tmpl | -- | `ls templates/jira-board/_partials/description-templates/*.tmpl | wc -l | grep -q 3` |
| T9 | unix-coder | Create schema.yaml for template validation | -- | templates/jira-board/schema.yaml | -- | `test -f templates/jira-board/schema.yaml` |
| T10 | unix-coder | Implement CLI command handler (cmdr jira-board subcommands) | internal/jiraboard/engine.go, internal/jiraboard/expander.go, internal/jiraboard/renderer.go, internal/jiraboard/publisher.go, internal/commands/agentic_instructions.md | internal/commands/jira_board.go | T2, T3, T4, T5 | `go build ./cmd/cc/...` |
| T11 | unix-coder | Write engine and expander tests (including partial merge and environment pseudo-dimension tests) | internal/jiraboard/engine.go, internal/jiraboard/expander.go, internal/jiraboard/types.go | internal/jiraboard/engine_test.go, internal/jiraboard/expander_test.go | T2, T3 | `go test ./internal/jiraboard/... -v` |
| T12 | unix-coder | Update /jira-board skill definition to reference new template system | ~/.claude/commands/jira-board.md | ~/.claude/commands/jira-board.md | T10 | `grep -q 'cmdr jira-board' ~/.claude/commands/jira-board.md` |
| T13 | code-review | Review engine, expander, publisher, and CLI for correctness | internal/jiraboard/*.go, internal/commands/jira_board.go, templates/jira-board/ | -- | T10, T11 | `go vet ./internal/jiraboard/... && go vet ./internal/commands/...` |

## 15. Dependency Graph

```
Phase 1 (parallel): [T1, T6, T8, T9]
  T1: Create jiraboard types and data model
  T6: Create slim org-generator.yaml template
  T8: Create description .md.tmpl files
  T9: Create schema.yaml

Phase 2 (parallel, after T1): [T2, T3, T4, T5]
  T2: Implement template engine (also needs T9)
  T3: Implement dimension matrix expander
  T4: Implement description renderer (also needs T8)
  T5: Implement Jira API publisher

Phase 2b (after T6): [T7]
  T7: Create partial templates

Phase 3 (parallel, after Phase 2): [T10, T11]
  T10: Implement CLI command handler (needs T2, T3, T4, T5)
  T11: Write engine and expander tests (needs T2, T3)

Phase 4 (after T10): [T12]
  T12: Update /jira-board skill definition

Final: [T13] -- code review (after T10, T11)
```

## 16. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `internal/jiraboard/types.go` | ~120 | No |
| `internal/jiraboard/engine.go` | ~250 | No |
| `internal/jiraboard/expander.go` | ~250 | No |
| `internal/jiraboard/renderer.go` | ~150 | No |
| `internal/jiraboard/publisher.go` | ~200 | No |
| `internal/jiraboard/engine_test.go` | ~200 | No |
| `internal/jiraboard/expander_test.go` | ~200 | No |
| `templates/jira-board/org-generator.yaml` | ~250 | No |
| `templates/jira-board/schema.yaml` | ~80 | No |
| `templates/jira-board/_partials/integrations-db.yaml` | ~80 | No |
| `templates/jira-board/_partials/integrations-apm.yaml` | ~60 | No |
| `templates/jira-board/_partials/integrations-infra.yaml` | ~80 | No |
| `templates/jira-board/_partials/integrations-log.yaml` | ~40 | No |
| `templates/jira-board/_partials/integrations-cloud.yaml` | ~80 | No |
| `templates/jira-board/_partials/description-templates/integration-task.md.tmpl` | ~50 | No |
| `templates/jira-board/_partials/description-templates/integration-story.md.tmpl` | ~30 | No |
| `templates/jira-board/_partials/description-templates/epic-summary.md.tmpl` | ~20 | No |
| `internal/commands/jira_board.go` | ~200 | No |

Files modified:

| File Path | Change |
|-----------|--------|
| `~/.claude/commands/jira-board.md` | Updated to reference `cmdr jira-board` CLI and new template paths |

Files deleted: None

## 17. Verification Plan

**Per-task checks:**
- T1: `go build ./internal/jiraboard/...`
- T2: `go build ./internal/jiraboard/...`
- T3: `go build ./internal/jiraboard/...`
- T4: `go build ./internal/jiraboard/...`
- T5: `go build ./internal/jiraboard/...`
- T6: `test -f templates/jira-board/org-generator.yaml && grep -q 'dimensions:' templates/jira-board/org-generator.yaml && ! grep -q 'integration_path' templates/jira-board/org-generator.yaml`
- T7: `ls templates/jira-board/_partials/*.yaml | wc -l | grep -q 5`
- T8: `ls templates/jira-board/_partials/description-templates/*.tmpl | wc -l | grep -q 3`
- T9: `test -f templates/jira-board/schema.yaml`
- T10: `go build ./cmd/cc/...`
- T11: `go test ./internal/jiraboard/... -v`
- T12: `grep -q 'cmdr jira-board' ~/.claude/commands/jira-board.md`
- T13: `go vet ./internal/jiraboard/... && go vet ./internal/commands/...`

**Integration check:** `go build ./cmd/cc/... && go test ./internal/jiraboard/... && go vet ./...`

**Rollback:** `git stash` or create a backup branch before execution: `git checkout -b jira-board-backup`

#### Binary Install Verification

```bash
go build -o cmdr ./cmd/cc
./cmdr jira-board --help | grep -q "generate"
./cmdr jira-board list --json 2>&1 | grep -q "org-generator"
```

## 18. Success Criteria (Machine-Verifiable)

- [ ] `go build ./cmd/cc/...` exits 0
- [ ] `go test ./internal/jiraboard/...` exits 0
- [ ] `go vet ./internal/jiraboard/...` exits 0
- [ ] `go vet ./internal/commands/...` exits 0
- [ ] `test -f templates/jira-board/org-generator.yaml` exits 0
- [ ] `test -f templates/jira-board/schema.yaml` exits 0
- [ ] `ls templates/jira-board/_partials/*.yaml | wc -l | grep -q 5` exits 0
- [ ] `ls templates/jira-board/_partials/description-templates/*.tmpl | wc -l | grep -q 3` exits 0
- [ ] `test -f internal/jiraboard/types.go` exits 0
- [ ] `test -f internal/jiraboard/engine.go` exits 0
- [ ] `test -f internal/jiraboard/expander.go` exits 0
- [ ] `test -f internal/jiraboard/renderer.go` exits 0
- [ ] `test -f internal/jiraboard/publisher.go` exits 0
- [ ] `test -f internal/commands/jira_board.go` exits 0
- [ ] `grep -q 'org-generator' templates/jira-board/org-generator.yaml` exits 0 -- template contains project type
- [ ] `! grep -q 'integration_path' templates/jira-board/org-generator.yaml` exits 0 -- orchestration file does not inline dimension catalog data
- [ ] `grep -q 'integration_path' templates/jira-board/_partials/integrations-db.yaml` exits 0 -- dimension data lives in partials
- [ ] `grep -q 'Spec Hints' templates/jira-board/_partials/description-templates/integration-task.md.tmpl` exits 0 -- task descriptions include spec hints for /sr pipeline

### Functional Smoke Criteria

- [ ] `go build -o cmdr ./cmd/cc && ./cmdr jira-board --help | grep -q "generate"` -- jira-board subcommand registered
- [ ] `go build -o cmdr ./cmd/cc && ./cmdr jira-board list 2>&1 | grep -q "org-generator"` -- template listing works
- [ ] `go build -o cmdr ./cmd/cc && ./cmdr jira-board validate templates/jira-board/org-generator.yaml 2>&1; test $? -eq 0` -- template validation passes

### Negation Checks

- [ ] `! grep -rn '{[a-z_]*}' templates/jira-board/org-generator.yaml | grep -v 'expand_dimensions' | grep -v 'description_template' | grep -v '#'` -- no unresolved `{placeholder}` strings in orchestration file outside of template fields
- [ ] `go test ./internal/jiraboard/... -run TestNoUnresolvedPlaceholders` exits 0 -- rendered descriptions contain no `{placeholder}` artifacts

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Data model and types (T1) | `unix-coder` | Pure Go struct definitions |
| Template engine core (T2, T3, T4, T5) | `unix-coder` | Implementation work -- YAML parsing, partial merging, expansion logic, rendering, publishing |
| YAML templates (T6, T7, T8, T9) | `unix-coder` | File creation -- slim orchestration YAML + partial catalogs from integrations-core/extras |
| CLI command (T10) | `unix-coder` | Cobra command wiring -- follows existing patterns in `internal/commands/` |
| Tests (T11) | `unix-coder` | Unit test creation for engine (including partial merge) and expander (including environment pseudo-dimension) |
| Skill update (T12) | `unix-coder` | Markdown file update |
| Code review (T13) | `code-review` | Architecture, correctness, and DRY review |

## Execution Order

```
Phase 1: Foundation
  +-- T1: types.go (agent: unix-coder)
  +-- T6: org-generator.yaml (agent: unix-coder)     [parallel]
  +-- T8: description templates (agent: unix-coder)   [parallel]
  +-- T9: schema.yaml (agent: unix-coder)             [parallel]

Phase 2: Engine Implementation [blocked by T1]
  +-- T2: engine.go (agent: unix-coder)
  +-- T3: expander.go (agent: unix-coder)              [parallel]
  +-- T4: renderer.go (agent: unix-coder)              [parallel]
  +-- T5: publisher.go (agent: unix-coder)             [parallel]

Phase 2b: Template Partials [blocked by T6]
  +-- T7: partial templates (agent: unix-coder)

Phase 3: CLI + Testing [blocked by Phase 2]
  +-- T10: jira_board.go (agent: unix-coder)
  +-- T11: tests (agent: unix-coder)                   [parallel, needs T2+T3 only]

Phase 4: Finalization [blocked by T10]
  +-- T12: skill update (agent: unix-coder)

Phase 5: Review [blocked by T10, T11]
  +-- T13: code review (agent: code-review)
```

Recommended directive: `/loop` -- Phase 1 and Phase 2 tasks are independent and benefit from parallel fan-out.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| YAML template parse error | `yaml.Unmarshal` returns error | Validate template before expansion; report line number in error message |
| Invalid dimension reference in story template | Expander encounters unknown dimension ID not in merged dimensions and not `environment` | Pre-validate all dimension references during template load; halt with error listing unknown refs |
| Partial file not found | `os.ReadFile` error during `_partials/` auto-discovery | Engine logs warning per missing partial; continues with remaining partials. Validate that all declared dimension keys have non-empty values after merge. |
| Partial defines dimension key not declared in main template | Partial contains `dimensions.foo` but `foo` not in org-generator.yaml `dimensions:` | Engine adds the key (lenient merge). Log info-level message about undeclared dimension. |
| Jira API auth failure | HTTP 401/403 from `jira.Client.do()` | Existing `decodeResponse` handles auth errors; bubble up to CLI with "check JIRA_EMAIL and JIRA_API_TOKEN" |
| Jira rate limit exceeded | HTTP 429 | Existing `RateLimiter.AdaptFromHeaders()` handles backoff automatically |
| Circular track dependencies | Dependency graph cycle detection | Validate dependency graph at template load time; reject templates with cycles |
| Template variable in description not resolved | Rendered description contains `{placeholder}` | Post-render check for unresolved `{...}` patterns; halt with error listing unresolved vars |
| Intake file references dimension value not in template | e.g., intake says `databases: ["oracle"]` but no partial defines `oracle` | Validate intake dimension values against merged dimensions during load; warn on unknown values |
| `environment` dimension used without intake environments | Task template has `expand_dimensions: ["environment"]` but intake `environments` is empty | Expander returns error: "task template requires environment expansion but intake.environments is empty" |
| External repo unavailable during T7 | `~/Programs/integrations-core/` not cloned locally | Fall back to dimension data in this spec's On-Disk Format section as source of truth |

## Open Questions

| # | Question | Impact | Suggested Default |
|---|----------|--------|-------------------|
| 2 | Should the engine support custom Jira fields (e.g., Story Points, Sprint) beyond the standard fields? | Affects publisher complexity | No custom fields in v1 -- use labels for all metadata. Custom fields can be added in v2. |
| 3 | Where should the `environments` list come from -- intake file only, or also from template defaults? | Affects intake schema design | Intake file only -- environments are always client-specific. Template has no default environments. |
| 4 | Should the description template have access to the full `integrations-core` conf.yaml spec, or just the parameter list from the template? | Affects renderer complexity and description quality | Parameter list from template only. The description includes a file path reference; the agent reads the full spec at execution time. |

## Datadog Integration Reference

### integrations-core Repository Structure

Each integration in `~/Programs/integrations-core/` follows this structure:

```
<integration>/
  assets/
    configuration/
      spec.yaml              # Authoritative parameter definitions
    dashboards/               # JSON dashboard definitions
    logs/                     # Log pipeline definitions
    monitors/                 # Monitor definitions
    service_checks.json       # Service check definitions
  datadog_checks/
    <integration>/
      __init__.py
      <integration>.py        # Check implementation
      data/
        conf.yaml.example     # Example configuration
  CHANGELOG.md
  README.md
```

### conf.yaml spec.yaml Format

The `spec.yaml` file in each integration defines all configurable parameters:

```yaml
name: <IntegrationName>
files:
- name: <integration>.yaml
  options:
  - template: init_config
    options:
    - name: <param_name>
      description: |
        <Multi-line description>
      value:
        type: <string|integer|boolean|array|object>
        example: <default_value>
  - template: instances
    options:
    - name: <param_name>
      required: true|false
      description: |
        <Multi-line description>
      value:
        type: <string|integer|boolean>
        display_default: <value>
```

### Key Integrations for Onboarding

| Integration | Repo | Typical Use |
|------------|------|-------------|
| postgres | integrations-core | Database monitoring + DBM |
| mysql | integrations-core | Database monitoring + DBM |
| sqlserver | integrations-core | Database monitoring + DBM |
| mongo | integrations-core | Database monitoring |
| redisdb | integrations-core | Cache monitoring |
| elastic | integrations-core | Search monitoring |
| nginx | integrations-core | Web server monitoring |
| apache | integrations-core | Web server monitoring |
| docker | integrations-core | Container monitoring |
| kubernetes | integrations-core | Orchestration monitoring |
| amazon_eks | integrations-core | AWS EKS monitoring |
| process | integrations-core | Process monitoring |
| network | integrations-core | Network monitoring |
| disk | integrations-core | Disk monitoring |

### integrations-extras Repository

Contains community and partner integrations at `~/Programs/integrations-extras/`. Same directory structure as `integrations-core` but for non-core integrations. Used for specialized monitoring needs (e.g., `aerospike_enterprise`, `auth0`, `backstage`).
