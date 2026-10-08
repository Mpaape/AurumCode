// Package dependencies is the dependency check of a change: the model reads
// the diff of any manifest or lockfile and names the packages it adds,
// updates and removes; the code grounds that answer in the diff, asks a
// public advisory source (OSV) about every version on each side, and
// classifies each advisory as introduced by the change, pre-existing or
// fixed. Nothing here lists ecosystems, file names or packages: what is a
// manifest is the model's reading, what is vulnerable is the source's
// answer. A source that cannot answer makes the check inconclusive, never
// clean.
package dependencies

// Status is where an advisory stands relative to the change.
type Status string

const (
	// StatusIntroduced: the advisory affects the head version and not the
	// base one (or the package is new).
	StatusIntroduced Status = "introduced"
	// StatusPreexisting: the advisory affects both sides.
	StatusPreexisting Status = "preexisting"
	// StatusFixed: the advisory affected the base and no longer affects the
	// head (the change fixed it, or removed the package).
	StatusFixed Status = "fixed"
)

// Change is one package the change touches in one manifest. Base and Head
// are the declared versions on each side ("" when absent on that side);
// BaseRange/HeadRange hold a declared range when no exact version is
// resolved. Explanation is the model's reading of a range.
type Change struct {
	Manifest  string `json:"manifest"`
	Ecosystem string `json:"ecosystem"`
	// LicenseSystem is the deps.dev system name of the package, as the
	// model names it ("" when it cannot).
	LicenseSystem string `json:"license_system,omitempty"`
	Name          string `json:"name"`
	Base          string `json:"base_version,omitempty"`
	Head          string `json:"head_version,omitempty"`
	BaseRange     string `json:"base_range,omitempty"`
	HeadRange     string `json:"head_range,omitempty"`
	Explanation   string `json:"explanation,omitempty"`
	// Unresolved is the model's statement that a side declares neither a
	// version nor a readable range.
	Unresolved bool `json:"unresolved,omitempty"`
}

// Added reports a package absent from the base side.
func (c Change) Added() bool { return c.Base == "" && c.BaseRange == "" }

// Removed reports a package absent from the head side.
func (c Change) Removed() bool { return c.Head == "" && c.HeadRange == "" }

// NewOrUpdated reports a change whose head side must be vetted (license,
// malicious package, suspicion): an added package or a changed version.
func (c Change) NewOrUpdated() bool {
	return !c.Removed() && (c.Added() || c.Base != c.Head || c.BaseRange != c.HeadRange)
}

// Key identifies the package in its manifest.
func (c Change) Key() string { return c.Manifest + "\x00" + c.Ecosystem + "\x00" + c.Name }

// Vulnerability is one advisory as the source returned it. Severity is the
// source's own word normalized to critical/high/medium/low, or "unknown".
type Vulnerability struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Severity string   `json:"severity"`
	Fixed    []string `json:"fixed,omitempty"`
	Link     string   `json:"link"`
}

// Malicious reports an advisory of the malicious-packages database (MAL-),
// by the source's own identifier, never by a list in this program.
func (v Vulnerability) Malicious() bool {
	if hasMalPrefix(v.ID) {
		return true
	}
	for _, a := range v.Aliases {
		if hasMalPrefix(a) {
			return true
		}
	}
	return false
}

func hasMalPrefix(id string) bool { return len(id) > 4 && id[:4] == "MAL-" }

// Identifiers is the advisory id followed by its aliases (CVE, GHSA).
func (v Vulnerability) Identifiers() []string { return append([]string{v.ID}, v.Aliases...) }

// Finding is one advisory on one changed package.
type Finding struct {
	Change Change        `json:"change"`
	Vuln   Vulnerability `json:"vulnerability"`
	Status Status        `json:"status"`
	// ByRange is true when the head side was a range consulted as a whole.
	ByRange bool `json:"by_range,omitempty"`
}

// License is the registered license of a new or updated package.
type License struct {
	Change     Change         `json:"change"`
	Expression string         `json:"expression"`
	Verdict    LicenseVerdict `json:"verdict"`
	// ByModel is true when the model classified the license text.
	ByModel bool `json:"by_model,omitempty"`
	// Reason is set when the license could not be determined.
	Reason string `json:"reason,omitempty"`
}

// Suspicion is a grounded typosquat or malicious-package suspicion.
type Suspicion struct {
	Change   Change     `json:"change"`
	Manifest string     `json:"manifest"`
	Name     string     `json:"name"`
	Summary  string     `json:"summary"`
	Evidence []Evidence `json:"evidence"`
}

// Evidence is one registry metadata field the suspicion rests on, quoted.
type Evidence struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// Report is one dependency check. Reason is the inconclusive motive ("" when
// conclusive); a report with a Reason is never read as "no findings".
type Report struct {
	Manifests   []string    `json:"manifests"`
	Changes     []Change    `json:"changes"`
	Findings    []Finding   `json:"findings"`
	Licenses    []License   `json:"licenses,omitempty"`
	Suspicions  []Suspicion `json:"suspicions,omitempty"`
	Divergences []string    `json:"divergences,omitempty"`
	Unvetted    []string    `json:"unvetted,omitempty"`
	Discarded   []string    `json:"discarded,omitempty"`
	Reason      string      `json:"reason,omitempty"`
	Detail      string      `json:"detail,omitempty"`
}

// The inconclusive reasons of a dependency check.
const (
	ReasonNoModel          = "dependencies_no_model"
	ReasonModelFailed      = "dependencies_model_failed"
	ReasonSourceFailed     = "dependencies_source_unreachable"
	ReasonSourceStale      = "dependencies_source_stale"
	ReasonScannerMissing   = "dependencies_scanner_unavailable"
	ReasonUnresolved       = "dependencies_unresolved_version"
	ReasonLicenseUnknown   = "dependencies_license_unknown"
	ReasonMetadataFailed   = "dependencies_metadata_unreachable"
	ReasonCheckoutBlocked  = "dependencies_unverified_checkout"
	ReasonManifestsOmitted = "dependencies_manifests_omitted"
	ReasonNotRun           = "dependencies_not_run"
	ReasonExtractionGap    = "dependencies_extraction_gap"
	ReasonScannerFailed    = "dependencies_scanner_failed"
	ReasonUnvetted         = "dependencies_unvetted_package"
)

// fail records the first inconclusive reason; later ones keep the first.
func (r *Report) fail(reason, detail string) {
	if r.Reason == "" {
		r.Reason, r.Detail = reason, detail
	}
}

// Inconclusive reports a check that cannot be trusted.
func (r Report) Inconclusive() bool { return r.Reason != "" }
