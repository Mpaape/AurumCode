package sbom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// cycloneDXFormat is the exact "bomFormat" value every valid CycloneDX
// document carries (OWASP CycloneDX 1.6, "bomFormat" required field). It is
// never configurable: this card's Outcome names CycloneDX specifically, so
// a GeneratorConfig that asked for a different format is already refused
// at config-validation time (GeneratorConfig.Validate), before Validate
// here is ever reached.
const cycloneDXFormat = "CycloneDX"

// BOM is the minimal subset of a CycloneDX document this package reads:
// just enough to decide AC-002 (format and spec version), never a full
// CycloneDX schema validator -- that is explicitly out of scope (the card's
// Outcome asks only that "formato e versao da especificacao" be checked).
type BOM struct {
	BOMFormat   string `json:"bomFormat"`
	SpecVersion string `json:"specVersion"`
}

// formatAndVersionOK is AC-002's one decision, kept in its own small,
// single-return-statement function so a skeptical mutation
// (AC-002-MUT-001, tests/acceptance/AUR-549.sh) can replace the single line
// below with "return true" and prove AC-002's own tests go red when the
// check is bypassed.
//
// Card v3: spec_version in config is a MINIMUM ("1.6+"), never an exact
// match -- the digest-pinned Trivy (0.73.0) emits CycloneDX 1.7 with no
// flag to request an older spec version (Trivy CHANGELOG 0.71.0, PR
// #10715), so exact matching rejected every real SBOM this generator ever
// produced. A got spec version is accepted when it shares the configured
// MAJOR and its minor is at or above the configured one -- see
// docs/specs/AUR-549.md for why a different major is still refused
// outright (othermajor, tests/acceptance/AUR-549.sh).
func formatAndVersionOK(bom BOM, wantSpecVersion string) bool {
	return bom.BOMFormat == cycloneDXFormat && specVersionAtLeast(bom.SpecVersion, wantSpecVersion)
}

// specVersionAtLeast reports whether got is a CycloneDX spec version at
// least as new as want, with the SAME major component: both strings must
// parse strictly as "major.minor" (exactly one dot, both sides pure,
// non-negative decimal digits -- no sign, no leading/trailing
// whitespace, no third component); anything else is not a valid
// comparison and reports false rather than guessing.
func specVersionAtLeast(got, want string) bool {
	gotMajor, gotMinor, ok := parseMajorMinor(got)
	if !ok {
		return false
	}
	wantMajor, wantMinor, ok := parseMajorMinor(want)
	if !ok {
		return false
	}
	return gotMajor == wantMajor && gotMinor >= wantMinor
}

// parseMajorMinor parses a strict "major.minor" version string.
func parseMajorMinor(v string) (major, minor int, ok bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 2 || !isDigitsOnly(parts[0]) || !isDigitsOnly(parts[1]) {
		return 0, 0, false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// isDigitsOnly reports whether s is one or more ASCII decimal digits and
// nothing else -- rejecting what strconv.Atoi alone would still accept (a
// leading "+", internal whitespace, a leading zero like "01") and the
// empty string. A leading zero is rejected unless s is exactly "0": a
// strict version component never has one.
func isDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	if len(s) > 1 && s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Validate reads path and fails closed on every way a "SBOM" can be
// unusable: unreadable, empty (AC-002-MUT-001's sibling case -- a trivy
// that silently wrote nothing must never be read as success), not JSON at
// all, or valid JSON that is not a CycloneDX document at the configured
// spec version. A nil return is the only signal GenerateAndValidate treats
// as "this file may be published" -- see pipeline.go.
func Validate(path, wantSpecVersion string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("sbom: reading %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("sbom: %s is empty", path)
	}
	var bom BOM
	if err := json.Unmarshal(data, &bom); err != nil {
		return fmt.Errorf("sbom: %s is not valid JSON: %w", path, err)
	}
	if !formatAndVersionOK(bom, wantSpecVersion) {
		return fmt.Errorf(
			"sbom: %s: bomFormat=%q specVersion=%q, want bomFormat=%q specVersion>=%q (mesma major)",
			path, bom.BOMFormat, bom.SpecVersion, cycloneDXFormat, wantSpecVersion,
		)
	}
	return nil
}
