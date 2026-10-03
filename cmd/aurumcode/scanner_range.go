package main

import (
	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

// localScanRange resolves the --base ref and HEAD of the local repository to
// full commit ids: the reviewed range a history engine (gitleaks) scans. An
// end that does not resolve leaves the range empty, and an engine that needs
// it then reports an inconclusive scan instead of scanning something else.
func localScanRange(repoRoot, base string) scanner.Range {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		return scanner.Range{}
	}
	baseID, baseErr := repo.ResolveRef(base)
	headID, headErr := repo.ResolveRef("HEAD")
	if baseErr != nil || headErr != nil {
		return scanner.Range{}
	}
	return scanner.Range{Base: baseID, Head: headID}
}
