package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// vetted is one new or updated package with its registry metadata, as the
// model receives it.
type vetted struct {
	Manifest  string   `json:"manifest"`
	Ecosystem string   `json:"ecosystem"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Metadata  Metadata `json:"metadata"`
}

// vetRegistry asks the registry about every new or updated package and the
// model whether any of them looks like a typosquat or a malicious package.
// A package the registry cannot place (no system from the model, or only a
// range) is unvetted unless the same package was vetted from another file
// of the change (a range in a manifest, the exact version in its lockfile).
// An unvetted package is declared, and with required (a declared fail_on)
// it makes the check inconclusive: it never passes as vetted. A registry
// that fails is inconclusive. A suspicion is kept only when grounded in the
// metadata the model was given.
func vetRegistry(ctx context.Context, model Completer, reg Registry, required bool, report *Report) {
	var batch []vetted
	var pending []Change
	for _, c := range report.Changes {
		if !c.NewOrUpdated() {
			continue
		}
		if c.Head == "" {
			pending = append(pending, c)
			continue
		}
		meta, err := reg.Metadata(ctx, c)
		if errors.Is(err, ErrNoSystem) {
			pending = append(pending, c)
			continue
		}
		if err != nil {
			report.fail(ReasonMetadataFailed, err.Error())
			return
		}
		batch = append(batch, vetted{Manifest: c.Manifest, Ecosystem: c.Ecosystem, Name: c.Name, Version: c.Head, Metadata: meta})
	}
	settleUnvetted(batch, pending, required, report)
	if len(batch) == 0 {
		return
	}
	user, err := json.Marshal(map[string][]vetted{"packages": batch})
	if err != nil {
		report.fail(ReasonModelFailed, err.Error())
		return
	}
	var answer struct {
		Suspicions []Suspicion `json:"suspicions"`
	}
	if err := ask(ctx, model, "suspicion.md", string(user), &answer); err != nil {
		report.fail(ReasonModelFailed, err.Error())
		return
	}
	for _, s := range answer.Suspicions {
		kept, why := groundSuspicion(batch, report.Changes, s)
		if why != "" {
			report.Discarded = append(report.Discarded, fmt.Sprintf("suspeita sobre %s descartada: %s", s.Name, why))
			continue
		}
		report.Suspicions = append(report.Suspicions, kept)
	}
}

// groundSuspicion keeps a suspicion only when it names a package that was
// vetted and every evidence item quotes a field of that package's metadata
// with its exact value; at least one item is required.
func groundSuspicion(batch []vetted, changes []Change, s Suspicion) (Suspicion, string) {
	for _, v := range batch {
		if v.Name != s.Name || (s.Manifest != "" && v.Manifest != s.Manifest) {
			continue
		}
		if len(s.Evidence) == 0 {
			return s, "sem evidencia"
		}
		for _, e := range s.Evidence {
			value, ok := v.Metadata[e.Field]
			if !ok || value != e.Value {
				return s, fmt.Sprintf("evidencia %q=%q fora dos metadados consultados", e.Field, e.Value)
			}
		}
		for _, c := range changes {
			if c.Manifest == v.Manifest && c.Name == v.Name {
				s.Change = c
			}
		}
		return s, ""
	}
	return s, "pacote fora dos pacotes novos consultados"
}

// settleUnvetted declares every pending package not vetted from another
// file, and fails the check for it when vetting is required.
func settleUnvetted(batch []vetted, pending []Change, required bool, report *Report) {
	for _, c := range pending {
		if vettedElsewhere(batch, c) {
			continue
		}
		version := c.Head
		if version == "" {
			version = "faixa " + c.HeadRange
		}
		entry := fmt.Sprintf("%s %s (%s, %s)", c.Name, version, c.Ecosystem, c.Manifest)
		report.Unvetted = append(report.Unvetted, entry)
		if required {
			report.fail(ReasonUnvetted, "pacote sem análise de registro: "+entry)
		}
	}
}

func vettedElsewhere(batch []vetted, c Change) bool {
	for _, v := range batch {
		if v.Name == c.Name && strings.EqualFold(v.Ecosystem, c.Ecosystem) {
			return true
		}
	}
	return false
}
