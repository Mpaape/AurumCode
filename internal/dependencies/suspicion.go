package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
// A package the registry cannot place (no system, or only a range) is
// declared; a registry
// that fails is inconclusive. A suspicion is kept only when grounded in the
// metadata the model was given.
func vetRegistry(ctx context.Context, model Completer, reg Registry, report *Report) {
	var batch []vetted
	for _, c := range report.Changes {
		if !c.NewOrUpdated() {
			continue
		}
		if c.Head == "" {
			report.Unvetted = append(report.Unvetted, fmt.Sprintf("%s faixa %s (%s, %s)", c.Name, c.HeadRange, c.Ecosystem, c.Manifest))
			continue
		}
		meta, err := reg.Metadata(ctx, c)
		if errors.Is(err, ErrNoSystem) {
			report.Unvetted = append(report.Unvetted, fmt.Sprintf("%s %s (%s, %s)", c.Name, c.Head, c.Ecosystem, c.Manifest))
			continue
		}
		if err != nil {
			report.fail(ReasonMetadataFailed, err.Error())
			return
		}
		batch = append(batch, vetted{Manifest: c.Manifest, Ecosystem: c.Ecosystem, Name: c.Name, Version: c.Head, Metadata: meta})
	}
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
