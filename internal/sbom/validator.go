package sbom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
func formatAndVersionOK(bom BOM, wantSpecVersion string) bool {
	return bom.BOMFormat == cycloneDXFormat && bom.SpecVersion == wantSpecVersion
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
			"sbom: %s: bomFormat=%q specVersion=%q, want bomFormat=%q specVersion=%q",
			path, bom.BOMFormat, bom.SpecVersion, cycloneDXFormat, wantSpecVersion,
		)
	}
	return nil
}
