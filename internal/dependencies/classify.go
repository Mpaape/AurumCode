package dependencies

import (
	"context"
	"fmt"
)

// cachedSource asks each distinct question once per check.
type cachedSource struct {
	src  Source
	seen map[Query][]Vulnerability
}

func (c *cachedSource) Query(ctx context.Context, q Query) ([]Vulnerability, error) {
	if c.seen == nil {
		c.seen = map[Query][]Vulnerability{}
	}
	if v, ok := c.seen[q]; ok {
		return v, nil
	}
	v, err := c.src.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	c.seen[q] = v
	return v, nil
}

// side queries one side of a change: its exact version, or the whole
// package when only a range is declared (every advisory of the package is
// then a candidate, never fewer). An absent side has no advisories.
func side(ctx context.Context, src Source, c Change, version, rng string) (vulns []Vulnerability, byRange bool, err error) {
	switch {
	case version != "":
		vulns, err = src.Query(ctx, Query{Ecosystem: c.Ecosystem, Name: c.Name, Version: version})
	case rng != "":
		vulns, err = src.Query(ctx, Query{Ecosystem: c.Ecosystem, Name: c.Name})
		byRange = true
	}
	return mergeAliases(vulns), byRange, err
}

// mergeAliases folds the records of one advisory published under several
// databases (GO-, GHSA-, PYSEC-, CVE-) that alias each other into the first
// one: identifiers and fixed versions are joined, and the highest known
// severity is kept, so one advisory is one finding.
func mergeAliases(vulns []Vulnerability) []Vulnerability {
	var out []Vulnerability
	for _, v := range vulns {
		merged := false
		for i := range out {
			if !containsAdvisory([]Vulnerability{out[i]}, v) {
				continue
			}
			for _, id := range v.Identifiers() {
				if id != out[i].ID {
					out[i].Aliases = appendUnique(out[i].Aliases, id)
				}
			}
			for _, f := range v.Fixed {
				out[i].Fixed = appendUnique(out[i].Fixed, f)
			}
			out[i].Severity = higherSeverity(out[i].Severity, v.Severity)
			merged = true
			break
		}
		if !merged {
			v.Aliases = append([]string(nil), v.Aliases...)
			v.Fixed = append([]string(nil), v.Fixed...)
			out = append(out, v)
		}
	}
	return out
}

// severityRank orders the normalized severities.
var severityRank = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

// higherSeverity keeps the higher of two severities of one advisory; a
// severity one database gives wins over another database's silence.
func higherSeverity(a, b string) string {
	switch {
	case a == SeverityUnknown:
		return b
	case b == SeverityUnknown:
		return a
	case severityRank[b] > severityRank[a]:
		return b
	}
	return a
}

// CanonicalID is the one identifier of an advisory that does not depend on
// which database answered first: the smallest of its identifiers.
func (v Vulnerability) CanonicalID() string {
	best := v.ID
	for _, id := range v.Aliases {
		if id < best {
			best = id
		}
	}
	return best
}

// classify asks the source about both sides of every change: an advisory on
// the head side only is introduced, on both sides pre-existing, on the base
// side only fixed. A failed question makes the report inconclusive and
// drops every finding, so a failure is never read as "clean".
func classify(ctx context.Context, src Source, changes []Change, report *Report) {
	cache := &cachedSource{src: src}
	var findings []Finding
	for _, c := range changes {
		if c.Unresolved {
			report.fail(ReasonUnresolved, fmt.Sprintf("%s em %s sem versao ou faixa resolvivel", c.Name, c.Manifest))
			continue
		}
		base, _, err := side(ctx, cache, c, c.Base, c.BaseRange)
		if err != nil {
			report.fail(sourceReason(err), err.Error())
			continue
		}
		head, byRange, err := side(ctx, cache, c, c.Head, c.HeadRange)
		if err != nil {
			report.fail(sourceReason(err), err.Error())
			continue
		}
		findings = append(findings, compare(c, base, head, byRange)...)
	}
	if report.Inconclusive() {
		return
	}
	report.Findings = findings
}

// compare classifies by advisory identity (id or any alias), so the same
// advisory under its GHSA and CVE names is one advisory.
func compare(c Change, base, head []Vulnerability, byRange bool) []Finding {
	var out []Finding
	for _, v := range head {
		status := StatusIntroduced
		if containsAdvisory(base, v) {
			status = StatusPreexisting
		}
		out = append(out, Finding{Change: c, Vuln: v, Status: status, ByRange: byRange})
	}
	for _, v := range base {
		if !containsAdvisory(head, v) {
			out = append(out, Finding{Change: c, Vuln: v, Status: StatusFixed})
		}
	}
	return out
}

func containsAdvisory(list []Vulnerability, v Vulnerability) bool {
	ids := map[string]bool{}
	for _, id := range v.Identifiers() {
		ids[id] = true
	}
	for _, x := range list {
		for _, id := range x.Identifiers() {
			if ids[id] {
				return true
			}
		}
	}
	return false
}

func sourceReason(err error) string {
	if isStale(err) {
		return ReasonSourceStale
	}
	return ReasonSourceFailed
}
