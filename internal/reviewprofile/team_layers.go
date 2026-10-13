package reviewprofile

import "strings"

// LayerTeams puts a higher layer's analysts over a lower layer's (AUR-613:
// the engineering standards layer over the reviewed repository): every
// profile of upper is available to the repository's review.profiles
// selection, and a profile of lower with the same name (case-insensitively)
// is dropped, so a lower layer cannot rewrite the analyst a higher one
// maintains. shadowed names every dropped profile of lower, in file order,
// for the caller to warn about.
//
// Without an upper team (nil or no profiles) lower is returned unchanged,
// the same pointer, and nothing is shadowed.
func LayerTeams(upper, lower *Team) (layered *Team, shadowed []string) {
	if upper == nil || len(upper.Profiles) == 0 {
		return lower, nil
	}
	layered = &Team{Declared: upper.Declared, byName: map[string]*Profile{}}
	for _, p := range upper.Profiles {
		layered.add(p)
	}
	if lower == nil {
		return layered, nil
	}
	for _, p := range lower.Profiles {
		if _, taken := layered.byName[strings.ToLower(p.Name)]; taken {
			shadowed = append(shadowed, p.Name)
			continue
		}
		layered.add(p)
	}
	return layered, shadowed
}

// add appends a copy of p to the team and indexes it by name.
func (t *Team) add(p Profile) {
	cp := p
	cp.Effective.Families = append([]Family(nil), p.Effective.Families...)
	t.Profiles = append(t.Profiles, cp)
	t.byName[strings.ToLower(cp.Name)] = &cp
}
