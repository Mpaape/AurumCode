package config

import (
	"fmt"
	"sort"
	"strings"
)

// The authority a central policy has over each section of the
// configuration, declared once. ApplyCentralPolicy walks these tables; a
// reflexive test fails when Config, ReviewConfig or QualityGatesConfig grows
// a field none of them classifies, so a new section can never be born
// silently controlled by the repository under review.

// centralPolicyProvider names the warnings a policy produces.
const centralPolicyProvider = "politica central"

// governedSection is one top-level key of config.yml and how a policy and a
// repository resolve it. apply writes the effective value and returns the
// warnings that name what of the repository's was ignored.
type governedSection struct {
	key   string
	apply func(effective, repo, central *Config) []ProviderWarning
}

// governedSections is every top-level section, in the order its warnings
// are emitted.
var governedSections = []governedSection{
	{key: "rules", apply: governRules},
	{key: "ignore", apply: governIgnore},
	{key: "review", apply: governReview},
	{key: "gate", apply: governGate},
	{key: "exceptions", apply: governExceptions},
	{key: "quality_gates", apply: governQualityGates},
	{key: "analysis_data", apply: governAnalysisData},
	{key: "deliberation", apply: mergeDeliberation},
	{key: "batches", apply: mergeBatches},
	{key: "dependencies", apply: governDependencies},
}

// reviewPolicyFields are the review keys a policy decides when it sets
// them; the repository's value stays when the policy leaves one empty.
var reviewPolicyFields = []struct {
	key   string
	field func(*ReviewConfig) *string
}{
	{key: "language", field: func(r *ReviewConfig) *string { return &r.Language }},
	{key: "publication", field: func(r *ReviewConfig) *string { return &r.Publication }},
}

// reviewRepositoryFields are the review keys that stay the repository's
// own, additive choice under any policy: context, presentation, memory.
var reviewRepositoryFields = []string{"inline_comments", "context", "memory", "changelog", "version", "profiles"}

// qualityGateSections are the quality_gates keys. Scanners (and sast, the
// semgrep alias) resolve engine by engine; every other subsection is
// decided by a policy that declares it and left to the repository
// otherwise.
var qualityGateSections = []string{"sast", "scanners", "ssor_dtrack", "supply_chain"}

func ignoredWarning(reason string) ProviderWarning {
	return ProviderWarning{Provider: centralPolicyProvider, Reason: reason}
}

func governRules(effective, repo, central *Config) []ProviderWarning {
	ruleIDs := make([]string, 0, len(repo.Rules))
	for id := range repo.Rules {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	var warnings []ProviderWarning
	for _, id := range ruleIDs {
		warnings = append(warnings, ignoredWarning(fmt.Sprintf("override da regra %q no config do repositório foi ignorado: a política central decide sozinha", id)))
	}
	effective.Rules = central.Rules
	return warnings
}

func governIgnore(effective, repo, central *Config) []ProviderWarning {
	var warnings []ProviderWarning
	for _, pattern := range repo.Ignore {
		warnings = append(warnings, ignoredWarning(fmt.Sprintf("padrão de ignore %q do repositório não foi aplicado: a política central decide sozinha", pattern)))
	}
	effective.Ignore = central.Ignore
	return warnings
}

func governReview(effective, _, central *Config) []ProviderWarning {
	for _, f := range reviewPolicyFields {
		if value := *f.field(&central.Review); strings.TrimSpace(value) != "" {
			*f.field(&effective.Review) = value
		}
	}
	return nil
}

// governGate: under a policy only the policy's gate applies, declared or
// not; the repository's own opt-in has authority only without a policy.
func governGate(effective, repo, central *Config) []ProviderWarning {
	var warnings []ProviderWarning
	if repo.Gate.Declared() {
		warnings = append(warnings, ignoredWarning("gate do config do repositório foi ignorado: a política central decide sozinha"))
	}
	effective.Gate = central.Gate
	return warnings
}

// governExceptions: a repository cannot declare its own exception for a
// policy-governed finding; each one dropped is named by its identifying
// triple.
func governExceptions(effective, repo, central *Config) []ProviderWarning {
	var warnings []ProviderWarning
	for _, exc := range repo.Exceptions {
		warnings = append(warnings, ignoredWarning(fmt.Sprintf(
			"exceção do repositório para a regra %q no caminho %q (repositório %q) foi ignorada: a política central decide sozinha",
			exc.Rule, exc.Path, exc.Repo,
		)))
	}
	effective.Exceptions = central.Exceptions
	return warnings
}

// governQualityGates resolves each quality_gates subsection independently:
// a policy that mentions one of them must not silently turn off another it
// never addressed.
func governQualityGates(effective, repo, central *Config) []ProviderWarning {
	var warnings []ProviderWarning
	effective.QualityGates, warnings = mergeScanners(central.QualityGates, repo.QualityGates)
	if central.QualityGates.SsorDtrack != nil {
		if repo.QualityGates.SsorDtrack != nil {
			warnings = append(warnings, ignoredWarning("quality_gates.ssor_dtrack do config do repositório foi ignorado: a política central decide sozinha"))
		}
		effective.QualityGates.SsorDtrack = central.QualityGates.SsorDtrack
	}
	if central.QualityGates.SupplyChain != nil {
		if repo.QualityGates.SupplyChain != nil {
			warnings = append(warnings, ignoredWarning("quality_gates.supply_chain do config do repositório foi ignorado: a política central decide sozinha"))
		}
		effective.QualityGates.SupplyChain = central.QualityGates.SupplyChain
	}
	return warnings
}

// governAnalysisData: a policy that declares analysis_data decides alone
// (the repository's max_age_days cannot loosen it); a policy silent on it
// leaves the repository's.
func governAnalysisData(effective, repo, central *Config) []ProviderWarning {
	if central.AnalysisData == nil {
		return nil
	}
	effective.AnalysisData = central.AnalysisData
	if repo.AnalysisData == nil {
		return nil
	}
	return []ProviderWarning{ignoredWarning("analysis_data do config do repositório foi ignorado: a política central decide sozinha")}
}
