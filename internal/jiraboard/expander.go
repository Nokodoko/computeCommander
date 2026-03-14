package jiraboard

import (
	"fmt"
	"strings"
)

// Expander takes a BoardTemplate and IntakeFile and produces expanded tickets.
type Expander struct {
	tmpl   *BoardTemplate
	intake *IntakeFile
}

// NewExpander creates a new dimension matrix expander.
func NewExpander(tmpl *BoardTemplate, intake *IntakeFile) *Expander {
	return &Expander{tmpl: tmpl, intake: intake}
}

// Expand produces all expanded tickets from the template, pruned by intake selections.
func (e *Expander) Expand() ([]ExpandedTicket, error) {
	projectKey := e.tmpl.Meta.DefaultProjectKey
	if e.intake != nil && e.intake.Intake.ProjectKey != "" {
		projectKey = e.intake.Intake.ProjectKey
	}

	// Build pruned dimension maps based on intake selections.
	prunedDims := e.pruneDimensions()

	var tickets []ExpandedTicket

	// Phase 1: Generate epic tickets from tracks.
	trackPhase := make(map[string]string)
	for _, track := range e.tmpl.Tracks {
		trackPhase[track.ID] = track.Phase

		if !e.trackApplies(track, prunedDims) {
			continue
		}

		if e.isTrackExcluded(track.ID) {
			continue
		}

		if e.isPhaseExcluded(track.Phase) {
			continue
		}

		epicKey := fmt.Sprintf("%s-%s", projectKey, track.ID)
		tickets = append(tickets, ExpandedTicket{
			DeterministicKey: epicKey,
			IssueType:        "Epic",
			Summary:          track.Name,
			Description:      track.Description,
			Labels:           []string{"cmdr-key:" + epicKey, track.Phase},
			Priority:         "High",
			TrackID:          track.ID,
			Phase:            track.Phase,
			DependsOn:        prefixKeys(projectKey, track.DependsOn),
		})
	}

	// Phase 2: Generate story and task tickets from story templates.
	for _, story := range e.tmpl.Stories {
		track := e.findTrack(story.Track)
		if track == nil {
			continue
		}

		if !e.trackApplies(*track, prunedDims) {
			continue
		}

		if e.isTrackExcluded(track.ID) {
			continue
		}

		if e.isPhaseExcluded(track.Phase) {
			continue
		}

		epicKey := fmt.Sprintf("%s-%s", projectKey, track.ID)

		// Expand story dimensions.
		storyCombinations := e.expandDimensions(story.ExpandDimensions, prunedDims)

		for _, storyComb := range storyCombinations {
			// Check exclude rules.
			if e.isExcluded(storyComb, story.ExcludeWhen) {
				continue
			}

			storyID := e.resolvePlaceholders(story.ID, storyComb)
			storyName := e.resolvePlaceholders(story.Name, storyComb)
			storyKey := fmt.Sprintf("%s-%s-%s", projectKey, track.ID, storyID)

			storyLabels := []string{"cmdr-key:" + storyKey}

			tickets = append(tickets, ExpandedTicket{
				DeterministicKey: storyKey,
				IssueType:        "Story",
				Summary:          storyName,
				Description:      "", // Filled by renderer later.
				Labels:           storyLabels,
				Priority:         "Medium",
				ParentKey:        epicKey,
				TrackID:          track.ID,
				Phase:            track.Phase,
				Dimensions:       storyComb,
			})

			// Expand tasks within the story.
			for _, task := range story.Tasks {
				// Check task-level applies_when.
				if !e.taskApplies(task, storyComb) {
					continue
				}

				taskCombinations := e.expandDimensions(task.ExpandDimensions, prunedDims)

				for _, taskComb := range taskCombinations {
					// Merge story and task dimensions.
					merged := mergeDimensions(storyComb, taskComb)

					taskID := e.resolvePlaceholders(task.ID, merged)
					taskName := e.resolvePlaceholders(task.Name, merged)
					taskKey := fmt.Sprintf("%s-%s-%s-%s", projectKey, track.ID, storyID, taskID)

					// Resolve label placeholders.
					taskLabels := []string{"cmdr-key:" + taskKey}
					for _, lbl := range task.Labels {
						resolved := e.resolvePlaceholders(lbl, merged)
						if !e.isLabelExcluded(resolved) {
							taskLabels = append(taskLabels, resolved)
						}
					}

					// Check label exclusion.
					if e.hasExcludedLabel(taskLabels) {
						continue
					}

					tickets = append(tickets, ExpandedTicket{
						DeterministicKey: taskKey,
						IssueType:        "Task",
						Summary:          taskName,
						Description:      "", // Filled by renderer later.
						Labels:           taskLabels,
						Priority:         task.Priority,
						ParentKey:        storyKey,
						TrackID:          track.ID,
						Phase:            track.Phase,
						Dimensions:       merged,
					})
				}
			}
		}
	}

	return tickets, nil
}

// pruneDimensions returns a map of dimension values filtered by intake selections.
func (e *Expander) pruneDimensions() map[string][]DimensionValue {
	result := make(map[string][]DimensionValue)

	// Copy all template dimensions.
	for k, v := range e.tmpl.Dimensions {
		result[k] = v
	}

	if e.intake == nil {
		return result
	}

	// Prune each dimension based on intake selections.
	prune := func(dimName string, selected []string) {
		if len(selected) == 0 {
			return // No selection = no pruning for this dimension.
		}
		selectedSet := make(map[string]bool)
		for _, s := range selected {
			selectedSet[s] = true
		}
		var filtered []DimensionValue
		for _, dv := range result[dimName] {
			if selectedSet[dv.ID] {
				filtered = append(filtered, dv)
			}
		}
		result[dimName] = filtered
	}

	prune("cloud_provider", e.intake.Intake.CloudProviders)
	prune("os", e.intake.Intake.OperatingSystems)
	prune("database", e.intake.Intake.Databases)
	prune("app_architecture", e.intake.Intake.AppArchitectures)
	prune("storage", e.intake.Intake.StorageSystems)

	return result
}

// trackApplies checks whether a track should be included based on its applies_when
// condition and the pruned dimensions.
func (e *Expander) trackApplies(track Track, prunedDims map[string][]DimensionValue) bool {
	aw := track.AppliesWhen

	if aw.Any {
		return true
	}

	if aw.Dimension != "" {
		dimValues := prunedDims[aw.Dimension]

		if aw.HasAny {
			return len(dimValues) > 0
		}

		if aw.NotValue != "" {
			// Track applies if at least one dimension value is NOT the excluded value.
			for _, dv := range dimValues {
				if dv.ID != aw.NotValue {
					return true
				}
			}
			return false
		}
	}

	return true
}

// taskApplies checks task-level applies_when conditions against the story's dimensions.
func (e *Expander) taskApplies(task TaskTemplate, dims map[string]DimensionValue) bool {
	if len(task.AppliesWhen) == 0 {
		return true
	}

	for dimName, allowedValues := range task.AppliesWhen {
		dv, ok := dims[dimName]
		if !ok {
			return false
		}
		found := false
		for _, v := range allowedValues {
			if dv.ID == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// expandDimensions produces all combinations of the given dimension names.
// Handles the "environment" pseudo-dimension by using intake environments.
// Handles the "track_ref" pseudo-dimension by using active tracks.
func (e *Expander) expandDimensions(dimNames []string, prunedDims map[string][]DimensionValue) []map[string]DimensionValue {
	if len(dimNames) == 0 {
		// No dimensions to expand — return a single empty combination.
		return []map[string]DimensionValue{{}}
	}

	// Resolve dimension values for each dimension name.
	dimValueSets := make([][]DimensionValue, len(dimNames))
	for i, name := range dimNames {
		switch name {
		case "environment":
			// Pseudo-dimension: use intake environments.
			if e.intake == nil || len(e.intake.Intake.Environments) == 0 {
				// No environments — skip expansion.
				dimValueSets[i] = []DimensionValue{{ID: "default", Label: "default"}}
			} else {
				envs := make([]DimensionValue, len(e.intake.Intake.Environments))
				for j, env := range e.intake.Intake.Environments {
					envs[j] = DimensionValue{ID: env, Label: env}
				}
				dimValueSets[i] = envs
			}
		case "track_ref":
			// Pseudo-dimension: use active tracks.
			var trackDVs []DimensionValue
			for _, track := range e.tmpl.Tracks {
				if !e.isTrackExcluded(track.ID) && !e.isPhaseExcluded(track.Phase) {
					trackDVs = append(trackDVs, DimensionValue{
						ID:    track.ID,
						Label: track.Name,
						Extra: map[string]any{"name": track.Name},
					})
				}
			}
			dimValueSets[i] = trackDVs
		default:
			if vals, ok := prunedDims[name]; ok {
				dimValueSets[i] = vals
			} else {
				// Unknown dimension — no values to expand.
				return nil
			}
		}
	}

	// Compute cartesian product.
	return cartesianProduct(dimNames, dimValueSets)
}

// cartesianProduct computes all combinations of dimension values.
func cartesianProduct(names []string, sets [][]DimensionValue) []map[string]DimensionValue {
	if len(names) == 0 {
		return []map[string]DimensionValue{{}}
	}

	var result []map[string]DimensionValue

	// Recursive: combinations of first set * combinations of remaining sets.
	rest := cartesianProduct(names[1:], sets[1:])

	for _, val := range sets[0] {
		for _, combo := range rest {
			merged := make(map[string]DimensionValue, len(combo)+1)
			for k, v := range combo {
				merged[k] = v
			}
			merged[names[0]] = val
			result = append(result, merged)
		}
	}

	return result
}

// resolvePlaceholders replaces {dimension} and {dimension.label} style placeholders
// in a template string with actual dimension values.
func (e *Expander) resolvePlaceholders(tmpl string, dims map[string]DimensionValue) string {
	result := tmpl

	for name, dv := range dims {
		result = strings.ReplaceAll(result, "{"+name+"}", dv.ID)
		result = strings.ReplaceAll(result, "{"+name+".id}", dv.ID)
		result = strings.ReplaceAll(result, "{"+name+".label}", dv.Label)
		result = strings.ReplaceAll(result, "{"+name+".name}", dv.Label) // For track_ref.name

		// Also resolve extra fields.
		if name, ok := dv.Extra["name"]; ok {
			if s, ok := name.(string); ok {
				result = strings.ReplaceAll(result, "{"+dv.ID+".name}", s)
			}
		}
	}

	return result
}

// isExcluded checks whether a dimension combination matches any exclude rule.
func (e *Expander) isExcluded(dims map[string]DimensionValue, rules []map[string]string) bool {
	for _, rule := range rules {
		match := true
		for dimName, excludeVal := range rule {
			dv, ok := dims[dimName]
			if !ok || dv.ID != excludeVal {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// isTrackExcluded checks if a track is in the intake's exclude list.
func (e *Expander) isTrackExcluded(trackID string) bool {
	if e.intake == nil {
		return false
	}
	for _, excluded := range e.intake.Intake.ExcludeTracks {
		if excluded == trackID {
			return true
		}
	}
	return false
}

// isPhaseExcluded checks if a phase is excluded by the intake's include_phases filter.
func (e *Expander) isPhaseExcluded(phase string) bool {
	if e.intake == nil || len(e.intake.Intake.IncludePhases) == 0 {
		return false
	}
	for _, included := range e.intake.Intake.IncludePhases {
		if included == phase {
			return false
		}
	}
	return true
}

// isLabelExcluded checks if a single label is in the intake's exclude list.
func (e *Expander) isLabelExcluded(label string) bool {
	if e.intake == nil {
		return false
	}
	for _, excluded := range e.intake.Intake.ExcludeLabels {
		if excluded == label {
			return true
		}
	}
	return false
}

// hasExcludedLabel checks if any label in the list is excluded by intake.
func (e *Expander) hasExcludedLabel(labels []string) bool {
	if e.intake == nil || len(e.intake.Intake.ExcludeLabels) == 0 {
		return false
	}
	for _, label := range labels {
		if e.isLabelExcluded(label) {
			return true
		}
	}
	return false
}

// findTrack returns the track with the given ID, or nil if not found.
func (e *Expander) findTrack(id string) *Track {
	for i := range e.tmpl.Tracks {
		if e.tmpl.Tracks[i].ID == id {
			return &e.tmpl.Tracks[i]
		}
	}
	return nil
}

// mergeDimensions merges two dimension maps, with b taking precedence.
func mergeDimensions(a, b map[string]DimensionValue) map[string]DimensionValue {
	result := make(map[string]DimensionValue, len(a)+len(b))
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

// prefixKeys prefixes each key with the project key.
func prefixKeys(projectKey string, keys []string) []string {
	result := make([]string, len(keys))
	for i, k := range keys {
		result[i] = projectKey + "-" + k
	}
	return result
}

// Preview generates a summary of what would be created without producing full tickets.
func (e *Expander) Preview() (*PreviewResult, error) {
	tickets, err := e.Expand()
	if err != nil {
		return nil, err
	}

	result := &PreviewResult{
		TotalTickets:       len(tickets),
		ByType:             make(map[string]int),
		ByPhase:            make(map[string]int),
		DimensionsSelected: make(map[string][]string),
	}

	for _, t := range tickets {
		result.ByType[t.IssueType]++
		result.ByPhase[t.Phase]++
	}

	// Populate selected dimensions from pruned values.
	prunedDims := e.pruneDimensions()
	for name, vals := range prunedDims {
		ids := make([]string, len(vals))
		for i, v := range vals {
			ids[i] = v.ID
		}
		result.DimensionsSelected[name] = ids
	}

	return result, nil
}
