package analysis

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The secret-detection catalog is data, not Go source. secrets.json is the
// JSON derivation of the public gitleaks default configuration at the
// version and digest pinned in the scanners lock (source.sha256), produced
// by scripts/analysis/update-secret-rules.sh with python's stdlib TOML
// reader so no TOML parser enters the module graph; the TOML itself is not
// committed. secrets.json.sha256 records the derivation's own digest, and
// secrets-local.json holds the AurumCode keyword-assignment rule the public
// base does not cover.
var (
	//go:embed rules/secrets.json
	embeddedSecretCatalog []byte
	//go:embed rules/secrets.json.sha256
	embeddedSecretCatalogDigest []byte
	//go:embed rules/secrets-local.json
	embeddedLocalSecretCatalog []byte
)

// Catalog schemas understood by the loader. A different schema is refused so
// a reshaped artifact can never be read with stale field semantics.
const (
	secretCatalogSchema      = "aurumcode-secret-rules-v1"
	localSecretCatalogSchema = "aurumcode-secret-rules-local-v1"
	sha256Prefix             = "sha256:"
)

// Allowlist regex targets and conditions, with the gitleaks meaning: the
// default target is the captured secret, "match" is the whole regex match
// and "line" is the full scanned line; the default condition is OR (any
// criterion allows), AND requires every declared criterion.
const (
	targetSecret = "secret"
	targetMatch  = "match"
	targetLine   = "line"
	conditionAND = "AND"
)

// contextOutsideStrings marks a local rule whose match is dropped when it
// starts inside an already open string or raw-string literal, so a quoted
// documentation example of an assignment is not mistaken for a real one.
const contextOutsideStrings = "code-outside-string-literals"

// secretCatalogFile is the on-disk shape shared by the derived base catalog
// and the local catalog.
type secretCatalogFile struct {
	Schema    string           `json:"schema"`
	Source    secretSource     `json:"source"`
	Allowlist allowlistSpec    `json:"allowlist"`
	Rules     []secretRuleSpec `json:"rules"`
}

type secretSource struct {
	RulebaseID string `json:"rulebase_id"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
}

type allowlistSpec struct {
	Condition   string   `json:"condition"`
	RegexTarget string   `json:"regexTarget"`
	Regexes     []string `json:"regexes"`
	LiteralSHA  []string `json:"literal_sha256"`
	Paths       []string `json:"paths"`
	Stopwords   []string `json:"stopwords"`
}

type secretRuleSpec struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Regex       string          `json:"regex"`
	Path        string          `json:"path"`
	SecretGroup int             `json:"secretGroup"`
	Entropy     float64         `json:"entropy"`
	Keywords    []string        `json:"keywords"`
	Allowlists  []allowlistSpec `json:"allowlists"`
	Context     string          `json:"context"`
}

// ruleOrigin tells which catalog a compiled rule came from; it decides the
// message a finding carries.
type ruleOrigin int

const (
	originBase ruleOrigin = iota
	originLocal
)

// skippedRule records a catalog rule the evaluator cannot apply to a diff
// line, with the reason. Skips are kept, never silently dropped, so a test
// can pin exactly which rules of the public base are not enforced.
type skippedRule struct {
	ID     string
	Reason string
}

// Skip reasons.
const (
	skipPathOnly   = "path-only rule: no content regex to evaluate on a diff line"
	skipBadRegexAt = "regex rejected by Go RE2: "
)

// secretCatalog is the compiled, immutable catalog the evaluator runs.
type secretCatalog struct {
	source  secretSource
	global  *allowlist
	rules   []*secretRule
	skipped []skippedRule
}

// loadEmbeddedSecretCatalog compiles the catalog compiled into the binary.
func loadEmbeddedSecretCatalog() (*secretCatalog, error) {
	return loadSecretCatalog(catalogArtifacts{
		derived:       embeddedSecretCatalog,
		derivedDigest: embeddedSecretCatalogDigest,
		local:         embeddedLocalSecretCatalog,
	})
}

// catalogArtifacts are the raw embedded files the loader verifies and
// compiles: the JSON derivation of the public base, its recorded digest and
// the local rules.
type catalogArtifacts struct {
	derived       []byte
	derivedDigest []byte
	local         []byte
}

// sourceDigestRe is the shape of a recorded sha256 digest.
var sourceDigestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func sha256Of(b []byte) string {
	sum := sha256.Sum256(b)
	return sha256Prefix + hex.EncodeToString(sum[:])
}

// loadSecretCatalog verifies the derived JSON's bytes against its recorded
// digest and that it names a well-formed source digest, then compiles
// the base rules followed by the local ones. Any mismatch, unknown schema or
// uncompilable allowlist is an error: the pass fails loudly instead of
// running with a catalog of unknown provenance.
func loadSecretCatalog(a catalogArtifacts) (*secretCatalog, error) {
	if got, want := sha256Of(a.derived), strings.TrimSpace(string(a.derivedDigest)); got != want {
		return nil, fmt.Errorf("secret catalog: artifact digest %s does not match the recorded %s", got, want)
	}
	var base secretCatalogFile
	if err := json.Unmarshal(a.derived, &base); err != nil {
		return nil, fmt.Errorf("secret catalog: %w", err)
	}
	if base.Schema != secretCatalogSchema {
		return nil, fmt.Errorf("secret catalog: schema %q, want %q", base.Schema, secretCatalogSchema)
	}
	if !sourceDigestRe.MatchString(base.Source.SHA256) {
		return nil, fmt.Errorf("secret catalog: malformed source digest %q", base.Source.SHA256)
	}
	var extra secretCatalogFile
	if err := json.Unmarshal(a.local, &extra); err != nil {
		return nil, fmt.Errorf("local secret catalog: %w", err)
	}
	if extra.Schema != localSecretCatalogSchema {
		return nil, fmt.Errorf("local secret catalog: schema %q, want %q", extra.Schema, localSecretCatalogSchema)
	}
	global, err := compileAllowlist(base.Allowlist)
	if err != nil {
		return nil, fmt.Errorf("secret catalog global allowlist: %w", err)
	}
	cat := &secretCatalog{source: base.Source, global: global}
	if err := cat.addRules(extra.Rules, originLocal); err != nil {
		return nil, err
	}
	if err := cat.addRules(base.Rules, originBase); err != nil {
		return nil, err
	}
	return cat, nil
}

// addRules compiles specs into the catalog, recording rules that cannot run
// on a diff line as skipped. A local rule that fails to compile is an error,
// since AurumCode owns it.
func (c *secretCatalog) addRules(specs []secretRuleSpec, origin ruleOrigin) error {
	for _, spec := range specs {
		if spec.Regex == "" {
			c.skipped = append(c.skipped, skippedRule{ID: spec.ID, Reason: skipPathOnly})
			continue
		}
		r, err := compileSecretRule(spec, origin)
		if err != nil {
			if origin == originLocal {
				return fmt.Errorf("local secret rule %s: %w", spec.ID, err)
			}
			c.skipped = append(c.skipped, skippedRule{ID: spec.ID, Reason: skipBadRegexAt + err.Error()})
			continue
		}
		c.rules = append(c.rules, r)
	}
	return nil
}

func compileSecretRule(spec secretRuleSpec, origin ruleOrigin) (*secretRule, error) {
	re, err := regexp.Compile(spec.Regex)
	if err != nil {
		return nil, err
	}
	r := &secretRule{
		id:          spec.ID,
		description: spec.Description,
		origin:      origin,
		re:          re,
		secretGroup: spec.SecretGroup,
		entropy:     spec.Entropy,
		outsideStr:  spec.Context == contextOutsideStrings,
	}
	for _, k := range spec.Keywords {
		r.keywords = append(r.keywords, strings.ToLower(k))
	}
	if spec.Path != "" {
		if r.path, err = regexp.Compile(spec.Path); err != nil {
			return nil, err
		}
	}
	for _, a := range spec.Allowlists {
		al, err := compileAllowlist(a)
		if err != nil {
			return nil, err
		}
		r.allowlists = append(r.allowlists, al)
	}
	return r, nil
}

// allowlist is a compiled gitleaks allowlist. literals holds the
// credential-shaped allowlist entries (a metacharacter-free literal the
// rule's own regex matches) as sha256 digests: the public repository never
// carries them in clear, and they allow a target only on exact equality,
// where the source regex allowed any target containing them.
type allowlist struct {
	matchAll  bool
	target    string
	regexes   []*regexp.Regexp
	literals  map[string]bool
	paths     []*regexp.Regexp
	stopwords []string
}

func compileAllowlist(spec allowlistSpec) (*allowlist, error) {
	a := &allowlist{matchAll: spec.Condition == conditionAND, target: spec.RegexTarget}
	if a.target == "" {
		a.target = targetSecret
	}
	var err error
	if a.regexes, err = compileAll(spec.Regexes); err != nil {
		return nil, err
	}
	if a.paths, err = compileAll(spec.Paths); err != nil {
		return nil, err
	}
	a.literals = make(map[string]bool, len(spec.LiteralSHA))
	for _, h := range spec.LiteralSHA {
		a.literals[sha256Prefix+h] = true
	}
	for _, s := range spec.Stopwords {
		a.stopwords = append(a.stopwords, strings.ToLower(s))
	}
	return a, nil
}

func compileAll(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}
