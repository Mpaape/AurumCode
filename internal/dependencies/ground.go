package dependencies

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// sides is one manifest's diff split by side: removed lines, added lines,
// and every line (context included).
type sides struct {
	removed, added, all string
}

func diffSides(diff *types.Diff, path string) sides {
	var rm, add, all strings.Builder
	for _, f := range diff.Files {
		if f.Path != path {
			continue
		}
		for _, h := range f.Hunks {
			for _, line := range h.Lines {
				all.WriteString(line)
				all.WriteByte('\n')
				switch {
				case strings.HasPrefix(line, "+"):
					add.WriteString(line[1:])
					add.WriteByte('\n')
				case strings.HasPrefix(line, "-"):
					rm.WriteString(line[1:])
					rm.WriteByte('\n')
				}
			}
		}
	}
	return sides{removed: rm.String(), added: add.String(), all: all.String()}
}

// ground keeps only the changes the diff shows: the manifest is one the
// model was given, the name appears in that file's diff, and every version
// or range the model states appears on the matching side's changed lines.
// The model never adds a dependency the change does not hold (AC-008).
func ground(diff *types.Diff, manifests []string, changes []Change) (kept []Change, discarded []string) {
	allowed := map[string]bool{}
	for _, m := range manifests {
		allowed[m] = true
	}
	seen := map[string]bool{}
	for _, c := range changes {
		c = trimChange(c)
		if why := groundingFailure(diff, allowed, c); why != "" {
			discarded = append(discarded, fmt.Sprintf("%s %s (%s): %s", c.Manifest, c.Name, c.Ecosystem, why))
			continue
		}
		if seen[c.Key()] {
			continue
		}
		seen[c.Key()] = true
		kept = append(kept, c)
	}
	return kept, discarded
}

func groundingFailure(diff *types.Diff, allowed map[string]bool, c Change) string {
	switch {
	case !allowed[c.Manifest]:
		return "arquivo fora dos manifestos alterados"
	case c.Name == "" || c.Ecosystem == "":
		return "sem nome ou ecossistema"
	case c.Added() && c.Removed() && !c.Unresolved:
		return "sem versao em nenhum dos lados"
	}
	s := diffSides(diff, c.Manifest)
	if !strings.Contains(s.all, c.Name) {
		return "nome ausente do diff"
	}
	for _, want := range []struct{ value, text, side string }{
		{c.Head, s.added, "versao nova"},
		{c.HeadRange, s.added, "faixa nova"},
		{c.Base, s.removed, "versao anterior"},
		{c.BaseRange, s.removed, "faixa anterior"},
	} {
		if want.value != "" && !strings.Contains(want.text, want.value) {
			return want.side + " " + want.value + " ausente das linhas alteradas"
		}
	}
	return ""
}

func trimChange(c Change) Change {
	for _, f := range []*string{&c.Manifest, &c.Ecosystem, &c.LicenseSystem, &c.Name, &c.Base, &c.Head, &c.BaseRange, &c.HeadRange} {
		*f = strings.TrimSpace(*f)
	}
	return c
}
