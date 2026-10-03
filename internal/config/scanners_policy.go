package config

import "github.com/Mpaape/AurumCode/internal/scanner"

// mergeScanners resolves the scanners of a central policy and a repository
// engine by engine. A policy entry that is required wins: the repository's
// entry for the same engine (including enabled: false) is dropped with a
// named warning. A policy entry that is not required yields to the
// repository's entry for the same engine, and that is a named warning too.
// An engine only one side declares is kept as declared.
func mergeScanners(central, repo QualityGatesConfig) (QualityGatesConfig, []ProviderWarning) {
	effective := repo
	effective.Sast, effective.Scanners = nil, nil
	effective.policyEngines = map[string]bool{}
	var warnings []ProviderWarning

	repoWins := map[string]bool{}
	for _, r := range repo.AllScanners() {
		p, declared := central.Scanner(r.Engine)
		if declared && p.Required {
			warnings = append(warnings, ProviderWarning{
				Provider: "politica central",
				Reason:   r.Label() + " do config do repositório foi ignorado: a política central decide sozinha",
			})
			continue
		}
		if declared {
			warnings = append(warnings, ProviderWarning{
				Provider: "politica central",
				Reason:   "quality_gates.scanners[" + p.Name() + "] da política central não é obrigatória (required: false): vale a entrada do repositório",
			})
		}
		repoWins[r.Name()] = true
	}
	if central.Sast != nil && !repoWins[central.Sast.AsScanner().Name()] {
		effective.Sast = central.Sast
		effective.policyEngines[central.Sast.AsScanner().Name()] = true
	}
	for _, p := range central.Scanners {
		if !repoWins[p.Name()] {
			effective.Scanners = append(effective.Scanners, p)
			effective.policyEngines[p.Name()] = true
		}
	}
	if repo.Sast != nil && repoWins[repo.Sast.AsScanner().Name()] {
		effective.Sast = repo.Sast
	}
	for _, r := range repo.Scanners {
		if repoWins[r.Name()] {
			effective.Scanners = append(effective.Scanners, r)
		}
	}
	return effective, warnings
}

// knownScannerSource reports whether a gate.sources/gate.triage entry names
// a registered engine or category.
func knownScannerSource(source string) bool { return scanner.KnownSource(source) }

// scannerSourceNames is the scanner part of gate.sources' vocabulary as
// messages spell it: every category, then every engine without one.
func scannerSourceNames() []string {
	out := scanner.Categories()
	for _, name := range scanner.Names() {
		if e, _ := scanner.Lookup(name); scanner.Normalize(e.Category) == "" {
			out = append(out, name)
		}
	}
	return out
}
