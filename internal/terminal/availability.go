package terminal

import (
	"fmt"
	"github.com/alexdancer/skillglass/internal/collection"
)

// Both command detail and browser detail present the same snapshot explanation.
func availabilityLines(value collection.AvailabilityExplanation) []string {
	if value.Evidence == nil {
		return nil
	}
	lines := []string{fmt.Sprintf("Evidence: %s | %s | validated %s", value.Evidence.Kind, value.Evidence.Rule, value.Evidence.AgentVersion)}
	if value.NativeNameEvidence != nil {
		lines = append(lines, "Native name evidence: "+string(value.NativeNameEvidence.Kind)+" | "+value.NativeNameEvidence.Rule+" | "+value.NativeNameEvidence.InputPath+" | "+value.NativeNameEvidence.Reason)
	}
	if value.NativeName != "" {
		lines = append(lines, "Native name: "+value.NativeName)
	}
	if value.Evidence.InputPath != "" {
		lines = append(lines, "Controlling input: "+value.Evidence.InputPath)
	}
	if value.Evidence.Setting != "" {
		lines = append(lines, "Setting: "+value.Evidence.Setting)
	}
	for _, fact := range value.References {
		if fact.NativeName != "" {
			lines = append(lines, "Reference native name: "+fact.NativeName)
		}
		lines = append(lines, fmt.Sprintf("Reference %s: %s (%s)", fact.Reference, fact.Participation, fact.Evidence.Kind), "Discovery rule: "+fact.Evidence.Rule+" | "+fact.Evidence.Reason, "Discovery input: "+fact.Evidence.InputPath, fmt.Sprintf("Permission: %s (%s) | %s", fact.Permission.State, fact.Permission.Evidence.Kind, fact.Permission.Evidence.Rule+" | "+fact.Permission.Evidence.Reason))
		if fact.Permission.Evidence.InputPath != "" {
			lines = append(lines, "Permission input: "+fact.Permission.Evidence.InputPath)
		}
		if fact.Permission.Evidence.Setting != "" {
			lines = append(lines, "Permission setting: "+fact.Permission.Evidence.Setting)
		}
		if fact.Evidence.Setting != "" {
			lines = append(lines, "Discovery setting: "+fact.Evidence.Setting)
		}
		lines = append(lines, "Provenance: "+fact.Provenance.Kind+" | "+fact.Provenance.Evidence.Reason)
		if fact.Provenance.PluginID != "" {
			lines = append(lines, "Plugin: "+fact.Provenance.PluginID+" | version "+fact.Provenance.Version, "Plugin manifest: "+fact.Provenance.Evidence.InputPath)
		}
		if setting := fact.Provenance.Setting; setting != nil {
			lines = append(lines, "Plugin setting: "+setting.InputPath+" | "+setting.Setting)
		}
		if activation := fact.Provenance.Activation; activation != nil {
			lines = append(lines, "Registration activation: "+string(activation.Kind)+" | "+activation.Rule+" | "+activation.Reason)
		}
		for _, record := range fact.Provenance.Records {
			lines = append(lines, "Plugin observation: "+record.Kind+" | "+record.InputPath+" | "+record.Path)
			if record.RegistrationInput != "" {
				lines = append(lines, "Marketplace registration: "+record.RegistrationInput+" | source "+record.DeclaredSource+" | lastUpdated "+string(record.LastUpdated))
			}
			if record.Scope != "" {
				lines = append(lines, "Installation: "+record.Scope+" | "+record.Applicability+" | project "+string(record.ProjectPath)+" | version "+string(record.Version))
			}
		}
	}
	// Layers are the same context-wide calculation on each eligible reference.
	for _, fact := range value.References {
		if len(fact.Permission.Layers) > 0 {
			for _, layer := range fact.Permission.Layers {
				lines = append(lines, fmt.Sprintf("Config layer: %s [%s, %s] | %s", layer.Path, layer.Scope, layer.State, layer.Reason))
				for _, choice := range layer.Choices {
					lines = append(lines, fmt.Sprintf("  Selector: %s | enabled=%t", choice.Path, choice.Enabled))
				}
			}
			break
		}
	}
	if value.Precedence != nil {
		lines = append(lines, "Precedence: "+string(value.Precedence.State)+" ("+string(value.Precedence.Evidence.Kind)+") | "+value.Precedence.Evidence.Rule+" | "+value.Precedence.Evidence.Reason)
		if value.Precedence.Winner != "" {
			label := "Winning source: "
			if value.Precedence.Evidence.Rule == "native-index-first-name" {
				label = "Winning index source: "
			}
			lines = append(lines, label+string(value.Precedence.Winner))
		}
		for _, id := range value.Precedence.RelatedSources {
			lines = append(lines, "Related independent source: "+string(id))
		}
	}
	if value.Invocation != nil {
		lines = append(lines, "Invocation: "+value.Invocation.State+" ("+string(value.Invocation.Evidence.Kind)+") | "+value.Invocation.Evidence.Reason)
	}
	if value.Session != nil {
		lines = append(lines, "Session: "+value.Session.State+" | "+value.Session.Evidence.Reason)
	}
	if value.Reload != nil {
		lines = append(lines, "Reload: "+value.Reload.State+" | "+value.Reload.Evidence.Reason)
	}
	for _, capability := range value.Capabilities {
		lines = append(lines, fmt.Sprintf("Capability: %s | %s | %s | %s", capability.Operation, capability.Scope, capability.State, capability.Evidence.Reason))
	}
	return lines
}
func scopeAgentLines(scope collection.Scope) []string {
	if scope.Agent == nil {
		return nil
	}
	lines := []string{"Agent: " + scope.Agent.Application, "Validated rules: " + scope.Agent.ValidatedVersion, "Version evidence: " + scope.Agent.VersionEvidence}
	for _, gap := range scope.Agent.Gaps {
		lines = append(lines, "Unverified: "+gap)
	}
	return lines
}
func optionFilters(snapshot *collection.Snapshot, options commonOptions, search string) (collection.Filters, error) {
	filters, err := snapshot.NewFilters(search, options.sourceRoot, options.context, options.problem)
	if err != nil {
		return filters, err
	}
	filters.Availability, err = collection.ParseAvailabilityState(options.availability)
	return filters, err
}

func availabilitySummaryLines(value collection.AvailabilityExplanation) []string {
	if value.Evidence == nil {
		return nil
	}
	lines := []string{"Availability: " + string(value.State) + " (" + string(value.Evidence.Kind) + ")"}
	for _, fact := range value.References {
		if value.ReferenceID != "" {
			if fact.Reference != value.ReferenceID {
				continue
			}
		} else if fact.Participation == collection.ParticipationInspection {
			continue
		}
		lines = append(lines, "Configured permission: "+string(fact.Permission.State)+" ("+string(fact.Permission.Evidence.Kind)+")")
		break
	}
	lines = append(lines, "Session: unknown | Reload: unknown")
	return lines
}
