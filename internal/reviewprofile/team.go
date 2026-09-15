// team.go implements the TEAM-DEFINED half of the reviewer-profile system:
// a versioned repository file (.aurumcode/profiles.yml) that names extra
// presets -- product_owner, a client-specific "release", anything the team
// needs -- alongside the code-owned built-ins.
//
// A team profile has exactly the same authority as a built-in, which is to
// say none over the five boundaries: it declares an emphasis, rule families
// and free-text instructions, and a clause that names severity, --fail-on,
// redaction, the cost cap or the deterministic security pass is refused by
// the same fail-closed scan Compile uses (including YAML merge keys and
// aliases). The team file cannot fetch, execute or download anything; it is
// parsed in-process exactly like a built-in Spec.
//
// The file is DATA. It is versioned with the repository, parsed with
// KnownFields(true) so an unrecognised key fails closed, and never reaches
// the model as an instruction. Instructions are the profile author's text,
// carried verbatim like a built-in's.
package reviewprofile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultTeamFile is where a team declares its profiles, relative to the
// repository root. A missing file is the zero-config case: an empty Team and
// a nil error, exactly like a missing config.
const DefaultTeamFile = ".aurumcode/profiles.yml"

// Named, errors.Is-able failures for the team file. Each names the offending
// profile or clause so a broken declaration is actionable.
var (
	// ErrMissingName names a profile entry with no name.
	ErrMissingName = errors.New("reviewprofile: missing-name")
	// ErrEmptyProfile names a profile entry missing an emphasis, families or
	// instructions, or a file that declares no profile at all.
	ErrEmptyProfile = errors.New("reviewprofile: empty-profile")
	// ErrDuplicateProfile names two profiles that resolve to the same name
	// (within the file, or colliding with a built-in).
	ErrDuplicateProfile = errors.New("reviewprofile: duplicate-profile")
)

// Team is the resolved set of team-defined profiles, in declaration order.
type Team struct {
	// Profiles are the declared profiles, in file order.
	Profiles []Profile
	// Declared names the team file that supplied the profiles, for output.
	Declared string

	byName map[string]*Profile
}

// EmptyTeam is the zero-config team: no file, no profiles, no error.
func EmptyTeam() *Team { return &Team{Declared: "team profiles: none (zero-config)"} }

// Names returns the team profile names in declaration order.
func (t *Team) Names() []string {
	if t == nil {
		return nil
	}
	out := make([]string, 0, len(t.Profiles))
	for _, p := range t.Profiles {
		out = append(out, p.Name)
	}
	return out
}

// Lookup returns a copy of the named team profile, case-insensitively.
func (t *Team) Lookup(name string) (Profile, bool) {
	if t == nil || t.byName == nil {
		return Profile{}, false
	}
	p, ok := t.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Profile{}, false
	}
	cp := *p
	cp.Effective.Families = append([]Family(nil), p.Effective.Families...)
	return cp, true
}

// teamDoc is the typed shape of the team file. profiles is a SEQUENCE so a
// team cannot smuggle a profile under an alias of the whole document.
type teamDoc struct {
	Profiles []Spec `yaml:"profiles"`
}

// LoadTeam parses a team profile file. Every forbidden clause is refused by
// the raw scan before decode, naming the clause; missing/empty/duplicate
// names are refused naming the problem; a colliding built-in name is refused
// because a team file may not shadow a code-owned preset.
func LoadTeam(data []byte) (*Team, error) {
	source := DefaultTeamFile
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reviewprofile: parse-error (%s): %w", source, err)
	}
	// The raw scan descends sequences, so every profile mapping inside the
	// `profiles:` list is checked -- merge keys and aliases included.
	if len(doc.Content) > 0 {
		if err := scanRefusedClauses(doc.Content[0], ""); err != nil {
			return nil, err
		}
	}
	var parsed teamDoc
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("reviewprofile: parse-error (%s): %w", source, err)
	}
	if len(parsed.Profiles) == 0 {
		return nil, fmt.Errorf("%w: %s declares no profiles", ErrEmptyProfile, source)
	}

	team := &Team{Declared: fmt.Sprintf("team profiles: %s", source), byName: map[string]*Profile{}}
	seen := map[string]bool{}
	for i, spec := range parsed.Profiles {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: %s profile #%d does not declare a name", ErrMissingName, source, i+1)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("%w: %s declares %q more than once", ErrDuplicateProfile, source, name)
		}
		if _, isBuiltin := builtins[key]; isBuiltin {
			return nil, fmt.Errorf("%w: %s declares %q, which is a built-in profile", ErrDuplicateProfile, source, name)
		}
		if strings.TrimSpace(spec.Emphasis) == "" || strings.TrimSpace(spec.Instructions) == "" || len(spec.Families) == 0 {
			return nil, fmt.Errorf("%w: %s profile %q must declare an emphasis, at least one family and instructions", ErrEmptyProfile, source, name)
		}
		p, err := compileSpec(spec)
		if err != nil {
			return nil, err
		}
		seen[key] = true
		cp := *p
		cp.Effective.Families = append([]Family(nil), p.Effective.Families...)
		team.Profiles = append(team.Profiles, cp)
		team.byName[key] = &cp
	}
	return team, nil
}

// LoadTeamFile reads root/DefaultTeamFile. A missing file is the zero-config
// case (EmptyTeam, nil); a file that exists but cannot be read or parsed is a
// loud error, never silently empty.
func LoadTeamFile(root string) (*Team, error) {
	path := root + string(os.PathSeparator) + DefaultTeamFile
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EmptyTeam(), nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return LoadTeam(data)
}
