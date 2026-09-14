package changelog

import (
	"fmt"
	"strconv"
	"strings"
)

// Bump names the semver component an input raises.
type Bump int

const (
	// BumpNone keeps the version unchanged.
	BumpNone Bump = iota
	// BumpPatch raises the patch component.
	BumpPatch
	// BumpMinor raises the minor component.
	BumpMinor
	// BumpMajor raises the major component.
	BumpMajor
)

// String returns the stable, lowercase name of the bump.
func (b Bump) String() string {
	switch b {
	case BumpPatch:
		return "patch"
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "major"
	default:
		return "none"
	}
}

// Version is a semantic version triple. It deliberately has no pre-release or
// build metadata: the engine computes release-line bumps only.
type Version struct {
	Major int
	Minor int
	Patch int
}

// String renders the version as "major.minor.patch".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// ParseVersion parses "major.minor.patch", with an optional leading "v". It is
// strict: exactly three non-negative integer components.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	raw = strings.TrimPrefix(raw, "v")
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("changelog: version %q is not major.minor.patch", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" {
			return Version{}, fmt.Errorf("changelog: version %q has an empty component", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("changelog: version %q has a non-numeric component", s)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

// NextVersion computes the next version from the current version and the
// reviewed commits.
//
// Exact table:
//
//	any breaking   -> major bump, except a 0.y.z line rises minor
//	else any feat   -> minor bump
//	else any fix    -> patch bump
//	else any perf   -> patch bump
//	else            -> no bump (BumpNone, version unchanged)
//
// The highest applicable rule wins. Breaking is read only from the parsed
// commit (a bang after the type/scope, or a BREAKING CHANGE: footer); prose in
// a subject such as "chore: release v9.0.0" is never a bump.
func NextVersion(current Version, commits []Commit) (Version, Bump) {
	classes, _ := classifyBounded(commits)

	var breaking, feat, fixLike bool
	for _, cl := range classes {
		if cl.Breaking {
			breaking = true
		}
		if !cl.Conventional {
			continue
		}
		switch cl.Kind {
		case KindFeat:
			feat = true
		case KindFix, KindPerf:
			fixLike = true
		}
	}

	switch {
	case breaking:
		if current.Major == 0 {
			return Version{Major: 0, Minor: current.Minor + 1, Patch: 0}, BumpMinor
		}
		return Version{Major: current.Major + 1, Minor: 0, Patch: 0}, BumpMajor
	case feat:
		return Version{Major: current.Major, Minor: current.Minor + 1, Patch: 0}, BumpMinor
	case fixLike:
		return Version{Major: current.Major, Minor: current.Minor, Patch: current.Patch + 1}, BumpPatch
	default:
		return current, BumpNone
	}
}
