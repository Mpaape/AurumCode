// Package attestation is PRODUCT code. Its own path component is
// "attestation", which CONTAINS the substring "test" but is not equal to
// the component "tests" -- AUR-483 AC-003's canonical trap. It must always
// be documented.
package attestation

// Report describes one attestation record.
type Report struct {
	Subject string
	Verdict bool
}

// NewReport builds a Report for subject.
func NewReport(subject string) Report {
	return Report{Subject: subject, Verdict: true}
}
