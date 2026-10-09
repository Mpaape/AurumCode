// The --pr reviewer profiles: the analysts a repository selects in
// review.profiles (built-in, or its own in .aurumcode/profiles.yml) review
// the pull request too, each in its own model pass, exactly as --base does.
// The team file is read from the trusted base ref, like the configuration
// and the skills: a pull request never brings its own analysts.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

// resolveProfiles resolves review.profiles against the built-in profiles
// and the base ref's team file. No selection is the single-reviewer path,
// unchanged; a selection that cannot be resolved stops the run with the
// profile's named error, never a silent single pass.
func (p *prReview) resolveProfiles() (int, bool) {
	names, err := p.cfg.ReviewProfiles()
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	if len(names) == 0 {
		return 0, false
	}
	team, err := p.remoteTeam()
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	res, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: names}, team)
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	p.profileRes = res
	if !res.Applied {
		return 0, false
	}
	fmt.Fprintf(p.stderr, "aurumcode review: %s\n", res.Declared)
	sigs := make([]string, 0, len(res.Profiles))
	for _, profile := range res.Profiles {
		sigs = append(sigs, profile.Signature())
	}
	p.profileIdentity = strings.Join(sigs, "\x1f")
	return 0, false
}

// remoteTeam reads .aurumcode/profiles.yml at the trusted base ref. A
// missing file is the zero-config team (built-in profiles only).
func (p *prReview) remoteTeam() (*reviewprofile.Team, error) {
	data, found, err := p.client.GetRepositoryFile(p.ctx, p.owner, p.repoName, reviewprofile.DefaultTeamFile, p.contextRef)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", reviewprofile.DefaultTeamFile, err)
	}
	if !found {
		return reviewprofile.EmptyTeam(), nil
	}
	return reviewprofile.LoadTeam(data)
}

// profilesApplied reports a selection of at least one analyst profile.
func (p *prReview) profilesApplied() bool {
	return p.profileRes != nil && p.profileRes.Applied
}
