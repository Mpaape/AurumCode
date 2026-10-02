// AUR-521: the single, canonical finding identity. SARIF's
// partialFingerprints (this card) is its first caller; AUR-494's own finding
// deduplication, once that card lands, must import this function instead of
// defining a second identity for "the same finding".
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strconv"
	"strings"
)

// FindingIdentity is what makes two findings "the same finding": the rule
// that fired, the path and line it fired at, and the normalized source
// context AT that line. It deliberately excludes the model's free-text
// message -- a model can reword the same finding's explanation across runs,
// and the identity must not move just because the wording did.
type FindingIdentity struct {
	RuleID  string
	Path    string
	Line    int
	Context string
}

// FindingFingerprint is the single canonical identity function over
// FindingIdentity. It must never be reimplemented a second time anywhere in
// this codebase (see the package comment above): AUR-494 calls this
// function rather than defining its own notion of "same finding".
//
// It is sha256(ruleID + "\x00" + normalizedPath + "\x00" + line + "\x00" +
// normalizedContext), hex-encoded, and reads nothing else -- no clock, no
// random source, no process or environment state. The same finding (same
// rule, path, line and code context) always produces the same fingerprint,
// this run or any later one (AC-002). A fingerprint that moved for the SAME
// finding across two runs would silently defeat every consumer that
// deduplicates or tracks a finding by it (SARIF's own suppression carry-over
// included) -- see MUT-001.
func FindingFingerprint(id FindingIdentity) string {
	normPath := normalizeFindingPath(id.Path)
	normContext := normalizeFindingContext(id.Context)
	// MUT-001 anchor: the fingerprint must never depend on wall-clock time, a
	// random source, or any other per-call value -- only on the finding's
	// own identity joined below. Folding a per-call nonce into `nonce` here
	// is exactly MUT-001, and TestAUR521FindingFingerprintStableAcrossRuns
	// (and the AC-002/MUT-001 acceptance selector) must go red when it does.
	nonce := ""
	payload := strings.Join([]string{
		strings.TrimSpace(id.RuleID),
		normPath,
		strconv.Itoa(id.Line),
		normContext,
		nonce,
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// normalizeFindingPath cleans separators and "." segments so the same file
// named with a leading "./" or backslash separators still identifies the
// same finding.
func normalizeFindingPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

// normalizeFindingContext collapses whitespace so the same code line
// reindented by an unrelated formatting change still identifies the same
// finding, while an actual content change still produces a different
// fingerprint (AC-002's own stability-vs-change test).
func normalizeFindingContext(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
