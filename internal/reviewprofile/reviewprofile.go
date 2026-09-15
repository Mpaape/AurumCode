// Package reviewprofile defines the built-in, versioned reviewer profiles a
// team can pick for a review: solid, seguranca and performance.
//
// A profile is a DETERMINISTIC PRESET, never a plugin. It declares exactly two
// things -- the review EMPHASIS and the ENABLED RULE FAMILIES -- and it is
// code-owned and resolved in-process. Nothing here reads a profile file, opens
// a socket, executes profile content, or lets a profile carry an instruction.
//
// # THE FIVE THINGS A PROFILE CAN NEVER TOUCH
//
// 1. finding severity,
// 2. the --fail-on gate,
// 3. secret redaction,
// 4. the cost cap,
// 5. the deterministic security pass.
//
// A Spec that names any of those clauses is refused at Compile time with a
// named error quoting the refused clause (see RefusedClauseError). The
// Profile's Effective preset exposes the boundary values a caller renders, and
// for every built-in they are fixed: security pass on, redaction on, severity
// and --fail-on untouched, cost cap unchanged.
//
// # SELECTION AND ZERO-CONFIG
//
// Selection carries the two places a profile may be named: the repository
// config and the review flag. The flag wins. An empty Selection is the
// zero-config case: Resolve returns Applied=false and the caller keeps
// today's behavior byte for byte. An unknown name is a named error returned
// before any model call.
package reviewprofile

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Family is a rule family a profile can enable. The names are the catalog's
// categories (internal/review/rules/*.yml), never new vocabulary.
type Family string

// The rule families the embedded catalog declares.
const (
	FamilyQuality     Family = "quality"
	FamilySecurity    Family = "security"
	FamilyPerformance Family = "performance"
)

// AllFamilies is the canonical, stable order outputs iterate.
func AllFamilies() []Family {
	return []Family{FamilyQuality, FamilySecurity, FamilyPerformance}
}

var knownFamilies = func() map[Family]bool {
	m := make(map[Family]bool, 3)
	for _, f := range AllFamilies() {
		m[f] = true
	}
	return m
}()

// DefaultVersion is the version a profile declares when its definition omits
// one. Every built-in declares its own explicitly.
const DefaultVersion = "1"

// Named, errors.Is-able failures.
var (
	// ErrUnknownProfile names a selection that is not a built-in profile.
	ErrUnknownProfile = errors.New("reviewprofile: unknown-profile")
	// ErrRefusedClause names a profile definition that tried to change a
	// boundary the profile system owns (severity, gate, redaction, cost,
	// security pass).
	ErrRefusedClause = errors.New("reviewprofile: refused-clause")
	// ErrUnknownFamily names an enabled family outside the catalog.
	ErrUnknownFamily = errors.New("reviewprofile: unknown-family")
)

// UnknownProfileError is the fail-closed error an unknown selection produces.
// It names the unknown value and every built-in the caller could have meant.
type UnknownProfileError struct {
	Name string
}

func (e *UnknownProfileError) Error() string {
	return fmt.Sprintf("%s: %q (built-ins: %s)", ErrUnknownProfile, e.Name, strings.Join(Names(), ", "))
}

// Unwrap makes errors.Is(err, ErrUnknownProfile) true.
func (e *UnknownProfileError) Unwrap() error { return ErrUnknownProfile }

// RefusedClauseError is returned when a profile Spec tries to change severity,
// --fail-on, redaction, the cost cap, or the security pass. It quotes the
// exact clause so the refusal is actionable.
type RefusedClauseError struct {
	Clause string
}

func (e *RefusedClauseError) Error() string {
	return fmt.Sprintf("%s: %q: a reviewer profile may set only review emphasis and enabled rule families; it can never change severity, relax --fail-on, change the cost cap, disable secret redaction, or disable the deterministic security pass", ErrRefusedClause, e.Clause)
}

// Unwrap makes errors.Is(err, ErrRefusedClause) true.
func (e *RefusedClauseError) Unwrap() error { return ErrRefusedClause }

// Effective is the resolved preset a caller renders. Its zero values are the
// "untouched" values; built-ins always pin the safety boundaries on.
type Effective struct {
	// Emphasis is the human-readable focus the review declares.
	Emphasis string
	// Families is the enabled rule families, canonical order, deduplicated.
	Families []Family
	// SecurityPassEnabled is the deterministic security pass. A profile can
	// never turn it off; every built-in leaves it true.
	SecurityPassEnabled bool
	// RedactionEnabled is secret redaction. A profile can never turn it off.
	RedactionEnabled bool
	// SeverityFloor and FailOnThreshold are "" when the profile does not
	// touch them; a profile that names either is refused before this exists.
	SeverityFloor   string
	FailOnThreshold string
	// CostCap is -1 when the profile does not touch the cost cap.
	CostCap int
}

func defaultEffective() Effective {
	return Effective{
		SecurityPassEnabled: true,
		RedactionEnabled:    true,
		CostCap:             -1,
	}
}

// Profile is a built-in, versioned reviewer preset plus its resolved effect.
type Profile struct {
	Name         string
	Version      string
	Instructions string
	Effective    Effective
}

// Emphasis is the declared review focus.
func (p Profile) Emphasis() string { return p.Effective.Emphasis }

// Families returns the enabled families in canonical order.
func (p Profile) Families() []Family { return append([]Family(nil), p.Effective.Families...) }

// Signature is a deterministic identity for the profile: same input, same
// signature. It covers the version, emphasis, enabled families and the
// instruction text, so AC-002 can compare two resolutions directly.
func (p Profile) Signature() string {
	fams := make([]string, 0, len(p.Effective.Families))
	for _, f := range p.Effective.Families {
		fams = append(fams, string(f))
	}
	return strings.Join([]string{
		"name=" + p.Name,
		"version=" + p.Version,
		"emphasis=" + p.Effective.Emphasis,
		"families=" + strings.Join(fams, ","),
		"instructions=" + p.Instructions,
	}, "|")
}

// Selection is where a profile may be named: repository config and the review
// flag. The flag wins; an empty Selection is zero-config.
type Selection struct {
	Config string
	Flag   string
}

// Name resolves the effective profile name: the flag if present, else the
// config value, trimmed. Empty means no profile selected.
func (s Selection) Name() string {
	if v := strings.TrimSpace(s.Flag); v != "" {
		return v
	}
	return strings.TrimSpace(s.Config)
}

// Result is a resolved selection. Applied is false for the zero-config case,
// where the caller keeps today's behavior.
type Result struct {
	Applied   bool
	Profile   Profile
	Declared  string
	Effective Effective
}

// Spec is the declarative definition of a profile. Built-ins are Specs
// compiled at init. Only Emphasis and Families are profile fields that carry
// authority; the pointer fields below exist so a hostile definition that names
// them is REFUSED and so a mutant that drops the guard provably changes
// Effective.
type Spec struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Emphasis     string   `yaml:"emphasis"`
	Families     []string `yaml:"families"`
	Instructions string   `yaml:"instructions"`

	// Forbidden clauses -- present only to be refused.
	Severity            *string `yaml:"severity"`
	FailOn              *string `yaml:"fail_on"`
	CostCap             *int    `yaml:"cost_cap"`
	RedactSecrets       *bool   `yaml:"redact_secrets"`
	SecurityPass        *bool   `yaml:"security_pass"`
	DisableSecurityPass *bool   `yaml:"disable_security_pass"`
}

// refuseForbidden refuses every clause a profile may never carry, naming the
// clause. This is the single load-bearing boundary: removing the security_pass
// branch is MUT-002 and must let the guard through (and the security pass off).
func refuseForbidden(s Spec) error {
	if s.Severity != nil {
		return &RefusedClauseError{Clause: "severity"}
	}
	if s.FailOn != nil {
		return &RefusedClauseError{Clause: "fail_on"}
	}
	if s.CostCap != nil {
		return &RefusedClauseError{Clause: "cost_cap"}
	}
	if s.RedactSecrets != nil && !*s.RedactSecrets {
		return &RefusedClauseError{Clause: "redact_secrets"}
	}
	if s.SecurityPass != nil && !*s.SecurityPass {
		return &RefusedClauseError{Clause: "security_pass"}
	}
	if s.DisableSecurityPass != nil && *s.DisableSecurityPass {
		return &RefusedClauseError{Clause: "disable_security_pass"}
	}
	return nil
}

// canonicalFamilies validates, deduplicates and canonically orders families.
func canonicalFamilies(list []string) ([]Family, error) {
	seen := map[Family]bool{}
	var out []Family
	for _, raw := range list {
		f := Family(strings.ToLower(strings.TrimSpace(raw)))
		if f == "" {
			continue
		}
		if !knownFamilies[f] {
			return nil, fmt.Errorf("%w: %q (known: %s)", ErrUnknownFamily, raw, familyList())
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return familyIndex(out[i]) < familyIndex(out[j])
	})
	return out, nil
}

func familyIndex(f Family) int {
	for i, item := range AllFamilies() {
		if item == f {
			return i
		}
	}
	return len(AllFamilies())
}

func familyList() string {
	items := make([]string, 0, len(AllFamilies()))
	for _, f := range AllFamilies() {
		items = append(items, string(f))
	}
	return strings.Join(items, ", ")
}

// compileSpec turns a guarded Spec into a Profile. It is only reachable after
// refuseForbidden; the forbidden branches below are the observable effect a
// mutant that drops the guard would produce.
func compileSpec(s Spec) (*Profile, error) {
	eff := defaultEffective()
	eff.Emphasis = strings.TrimSpace(s.Emphasis)
	fams, err := canonicalFamilies(s.Families)
	if err != nil {
		return nil, err
	}
	eff.Families = fams

	if s.SecurityPass != nil {
		eff.SecurityPassEnabled = *s.SecurityPass
	}
	if s.DisableSecurityPass != nil {
		eff.SecurityPassEnabled = !*s.DisableSecurityPass
	}
	if s.RedactSecrets != nil {
		eff.RedactionEnabled = *s.RedactSecrets
	}
	if s.Severity != nil {
		eff.SeverityFloor = *s.Severity
	}
	if s.FailOn != nil {
		eff.FailOnThreshold = *s.FailOn
	}
	if s.CostCap != nil {
		eff.CostCap = *s.CostCap
	}

	name := strings.TrimSpace(s.Name)
	version := strings.TrimSpace(s.Version)
	if version == "" {
		version = DefaultVersion
	}
	if name == "" {
		return nil, fmt.Errorf("reviewprofile: invalid-profile: a profile must declare a name")
	}
	return &Profile{Name: name, Version: version, Instructions: s.Instructions, Effective: eff}, nil
}

// Compile parses and validates a profile definition. A definition that names
// a forbidden clause is refused, naming it; an unknown family is a named
// error. No file is executed and nothing is fetched.
func Compile(data []byte) (*Profile, error) {
	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("reviewprofile: parse-error: %w", err)
	}
	if err := refuseForbidden(spec); err != nil {
		return nil, err
	}
	return compileSpec(spec)
}

// builtinSpecs are the code-owned built-ins. Each declares a version and only
// emphasis/families: a built-in that carried a forbidden clause would fail
// mustBuiltin at init, so the binary cannot start with a boundary-violating
// preset.
var builtinSpecs = []Spec{
	{
		Name:         "solid",
		Version:      "1",
		Emphasis:     "SOLID principles and maintainability",
		Families:     []string{string(FamilyQuality)},
		Instructions: "Priorize responsabilidade unica, coesao, acoplamento e substituicao de Liskov.",
	},
	{
		Name:         "seguranca",
		Version:      "1",
		Emphasis:     "deterministic security review",
		Families:     []string{string(FamilySecurity)},
		Instructions: "Priorize segredos embutidos, injecao, deserializacao insegura e superficie de ataque.",
	},
	{
		Name:         "performance",
		Version:      "1",
		Emphasis:     "performance and algorithmic cost",
		Families:     []string{string(FamilyPerformance)},
		Instructions: "Priorize complexidade, alocacoes, consultas N+1 e retencao de recursos.",
	},
}

var builtins = mustBuiltins()

func mustBuiltins() map[string]*Profile {
	m := make(map[string]*Profile, len(builtinSpecs))
	for _, spec := range builtinSpecs {
		if err := refuseForbidden(spec); err != nil {
			panic(fmt.Sprintf("reviewprofile: built-in %q violates the boundary: %v", spec.Name, err))
		}
		p, err := compileSpec(spec)
		if err != nil {
			panic(fmt.Sprintf("reviewprofile: built-in %q is invalid: %v", spec.Name, err))
		}
		m[strings.ToLower(p.Name)] = p
	}
	return m
}

// Names returns the built-in profile names in canonical order.
func Names() []string {
	out := make([]string, 0, len(builtinSpecs))
	for _, s := range builtinSpecs {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}

// Builtin returns a copy of the named built-in profile, or false.
func Builtin(name string) (Profile, bool) {
	p, ok := builtins[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Profile{}, false
	}
	cp := *p
	return cp, true
}

// Resolve turns a Selection into a Result. An empty selection is the
// zero-config case (Applied=false, nil error). A name that is not a built-in
// is a named UnknownProfileError returned BEFORE any model call. A successful
// resolution declares which profile entered in Result.Declared.
func Resolve(sel Selection) (*Result, error) {
	name := sel.Name()
	if name == "" {
		return &Result{Applied: false, Declared: "profile: none (zero-config)"}, nil
	}
	p, ok := Builtin(name)
	if !ok {
		return nil, &UnknownProfileError{Name: name}
	}
	return &Result{
		Applied:   true,
		Profile:   p,
		Effective: p.Effective,
		Declared:  fmt.Sprintf("profile: %s (v%s)", p.Name, p.Version),
	}, nil
}
