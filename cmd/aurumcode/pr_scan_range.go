package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

// prScanRange is the commit range a history engine scans on --pr: from
// AURUMCODE_BASE_SHA (the pull request's base) to the HEAD of the verified
// checkout, which resolveCheckout already proved equal to the head the API
// names. It is never GITHUB_SHA: on a pull_request event GitHub reserves
// that variable to the synthetic merge commit (refs/pull/N/merge), which a
// checkout of the pull request head does not contain, so the range would
// name a commit the engine cannot find. Without a verified checkout the
// head stays empty; every scanner is blocked then anyway.
func (p *prReview) prScanRange() scanner.Range {
	r := scanner.Range{Base: strings.TrimSpace(p.env().baseSHA)}
	if p.verifiedDir == "" {
		return r
	}
	repo, err := analyzer.OpenRepo(p.verifiedDir)
	if err != nil {
		return r
	}
	if head, err := repo.ResolveRef("HEAD"); err == nil {
		r.Head = strings.ToLower(strings.TrimSpace(head))
	}
	return r
}
