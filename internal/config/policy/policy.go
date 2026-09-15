// Package policy makes the ISO/IEC 25010 review weights a declaration the
// team owns, not a constant baked into the binary.
//
// The declaration lives in a versioned file at the repository root
// (.aurumcode/iso25010-weights.yml). It names the eight ISO/IEC 25010
// characteristics, gives each a weight that must sum to 1.0, and may add
// the team's own convention: a per-characteristic severity limit and which
// characteristics are blocking versus informative.
//
// # EXPLICIT CONFIGURATION HAS AUTHORITY; CONTEXT TEXT DOES NOT
//
// This file is EXPLICIT USER CONFIGURATION: the team wrote it and the
// repository versions it, so it has authority over the review's severity
// policy. That is categorically different from context material -- a
// repository prompt, a skill, an MCP result, a RAG chunk -- which is
// UNTRUSTED DATA that can only ever inform the reviewer and can never
// change a rule, a severity, a gate, secret redaction, or a cost ceiling.
// Nothing in this package reads, parses, or accepts directives from any
// such untrusted text: Policy is decoded from the policy file alone.
//
// # THE ONE THING EXPLICIT CONFIGURATION STILL CANNOT DO
//
// Authority over severity does not extend to switching off the
// deterministic security pass or secret redaction. Those are code-owned
// safety boundaries, not tunable weights. A policy that tries to disable
// either -- security_pass: false, redaction: off, security.redact_secrets:
// false, or their spellings -- is refused at load time with a named error
// that quotes the refused clause. The Policy type has no field through
// which the security pass or redaction could be disabled even if a caller
// wanted that, so the refusal is structural as well as validated.
//
// # ZERO-CONFIG IS A BYTE-FOR-BYTE NO-OP
//
// A missing file is not an error and not an empty policy: Load returns
// (nil, nil), Evaluate returns (nil, nil), and Render emits exactly
// json.Marshal(result). A repository that has not declared weights gets
// today's output, byte for byte.
package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
	"gopkg.in/yaml.v3"
)

// DefaultPath is where Load looks, relative to the repository root.
const DefaultPath = ".aurumcode/iso25010-weights.yml"

// weightTolerance absorbs the rounding a hand-written decimal file will
// carry; weights must still sum to 1.0 to this precision. It is tiny on
// purpose: a file that sums to anything meaningfully other than 1.0 is an
// error, never silently normalized.
const weightTolerance = 1e-9

// Characteristic is one of the eight ISO/IEC 25010 product quality
// characteristics. The vocabulary is the standard's; a file cannot invent
// a ninth.
type Characteristic string

// The eight ISO/IEC 25010 characteristics.
const (
	Functionality   Characteristic = "functionality"
	Reliability     Characteristic = "reliability"
	Usability       Characteristic = "usability"
	Efficiency      Characteristic = "efficiency"
	Maintainability Characteristic = "maintainability"
	Portability     Characteristic = "portability"
	Security        Characteristic = "security"
	Compatibility   Characteristic = "compatibility"
)

// AllCharacteristics returns the canonical, stable order the review
// reports in. Every output of this package iterates this order so two runs
// over the same configuration cannot disagree.
func AllCharacteristics() []Characteristic {
	return []Characteristic{
		Functionality, Reliability, Usability, Efficiency,
		Maintainability, Portability, Security, Compatibility,
	}
}

var knownCharacteristics = func() map[Characteristic]bool {
	m := make(map[Characteristic]bool, 8)
	for _, c := range AllCharacteristics() {
		m[c] = true
	}
	return m
}()

// Severity is a finding severity the team can use as a limit.
type Severity string

// The three severities the engine already emits.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

func validSeverity(s Severity) bool {
	switch s {
	case SeverityError, SeverityWarning, SeverityInfo:
		return true
	}
	return false
}

// Named, errors.Is-able policy failures. Every one carries a stable code
// so a caller can distinguish "the team mis-declared the weights" from
// "the team tried to switch off a safety boundary" without string
// matching, while the wrapped message stays actionable.
var (
	ErrUnknownCharacteristic = errors.New("policy: unknown-characteristic")
	ErrInvalidWeight         = errors.New("policy: invalid-weight")
	ErrNonFiniteWeight       = errors.New("policy: non-finite-weight")
	ErrNegativeWeight        = errors.New("policy: negative-weight")
	ErrWeightsSum            = errors.New("policy: weights-sum")
	ErrInvalidSeverity       = errors.New("policy: invalid-severity")
	ErrConventionOverlap     = errors.New("policy: blocking-informative-overlap")
	ErrRefusedClause         = errors.New("policy: refused-clause")
)

// Convention is the team's declared severity policy. It is explicit
// configuration, so unlike untrusted context text it is allowed to set
// per-characteristic limits and to say what blocks.
type Convention struct {
	// SeverityLimit maps a characteristic to the highest severity the team
	// tolerates for it. An absent entry means "no declared limit".
	SeverityLimit map[Characteristic]Severity `json:"severity_limit,omitempty"`
	// Blocking lists characteristics whose findings block the change.
	Blocking []Characteristic `json:"blocking,omitempty"`
	// Informative lists characteristics whose findings only inform.
	Informative []Characteristic `json:"informative,omitempty"`
}

// Policy is a validated weights-and-convention declaration. It holds no
// field that can disable the security pass or redaction: those clauses are
// refused before a Policy ever exists.
type Policy struct {
	Source     string                     `json:"source"`
	Weights    map[Characteristic]float64 `json:"weights"`
	Convention Convention                 `json:"convention,omitempty"`
}

// CharacteristicScore is one row of the policy-aware report.
type CharacteristicScore struct {
	Characteristic Characteristic `json:"characteristic"`
	Score          int            `json:"score"`
	Weight         float64        `json:"weight"`
	Contribution   float64        `json:"contribution"`
	SeverityLimit  Severity       `json:"severity_limit,omitempty"`
	Blocking       bool           `json:"blocking"`
	Informative    bool           `json:"informative"`
}

// Evaluation is the policy-weighted view of a review's ISO scores.
type Evaluation struct {
	Enabled           bool                  `json:"enabled"`
	Source            string                `json:"source,omitempty"`
	PerCharacteristic []CharacteristicScore `json:"per_characteristic"`
	Aggregate         int                   `json:"aggregate"`
	AggregateExact    float64               `json:"aggregate_exact"`
}

// Load reads root/.aurumcode/iso25010-weights.yml. The zero-config
// contract: a missing file returns (nil, nil). Every caller treats nil
// exactly like today's behavior.
func Load(root string) (*Policy, error) {
	return LoadPath(filepath.Join(root, DefaultPath))
}

// LoadPath reads one explicit policy path. A missing path is zero-config;
// a path that exists but is invalid is a loud, named error.
func LoadPath(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("policy: read %s: %w", path, err)
	}
	return Parse(data, path)
}

// Parse decodes and validates policy bytes. The source is included in
// errors so a bad file stays actionable without echoing its contents.
func Parse(data []byte, source string) (*Policy, error) {
	if source == "" {
		source = DefaultPath
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("policy: parse-error: %s: %w", source, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("policy: invalid-root: %s must be a mapping", source)
	}
	root := doc.Content[0]

	// Safety boundaries first: a policy that tries to disable the
	// deterministic security pass or secret redaction is refused before
	// any weight is even considered, naming the clause it tried.
	if err := scanRefusedClauses(root, ""); err != nil {
		return nil, err
	}

	weightsNode := mappingValue(root, "weights")
	if weightsNode == nil || weightsNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: %s: ISO/IEC 25010 weights must sum to 1.0, got 0.000000", ErrWeightsSum, source)
	}

	weights := make(map[Characteristic]float64, 8)
	var sum float64
	for i := 0; i+1 < len(weightsNode.Content); i += 2 {
		name := Characteristic(strings.ToLower(strings.TrimSpace(weightsNode.Content[i].Value)))
		if !knownCharacteristics[name] {
			return nil, fmt.Errorf("%w: %q is not an ISO/IEC 25010 characteristic", ErrUnknownCharacteristic, string(name))
		}
		var value float64
		if err := weightsNode.Content[i+1].Decode(&value); err != nil {
			return nil, fmt.Errorf("%w: %q: %v", ErrInvalidWeight, string(name), err)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("%w: %q = %v is not a finite ISO/IEC 25010 weight", ErrNonFiniteWeight, string(name), value)
		}
		if value < 0 {
			return nil, fmt.Errorf("%w: %q = %v is not a valid ISO/IEC 25010 weight", ErrNegativeWeight, string(name), value)
		}
		weights[name] = value
		sum += value
	}
	// The inverted comparison is deliberate: a NaN sum makes both
	// `sum-1.0 < -tol` and `sum-1.0 > tol` false, so the direct `>` form
	// would let a NaN declaration through. `!(abs <= tol)` is false only
	// when the sum is genuinely finite and within tolerance.
	if !(math.Abs(sum-1.0) <= weightTolerance) {
		return nil, fmt.Errorf("%w: %s: ISO/IEC 25010 weights must sum to 1.0, got %.6f", ErrWeightsSum, source, sum)
	}

	conv, err := parseConvention(root)
	if err != nil {
		return nil, err
	}
	return &Policy{Source: source, Weights: weights, Convention: conv}, nil
}

func parseConvention(root *yaml.Node) (Convention, error) {
	conv := Convention{SeverityLimit: map[Characteristic]Severity{}}
	node := mappingValue(root, "convention")
	if node == nil {
		return conv, nil
	}
	if node.Kind != yaml.MappingNode {
		return conv, fmt.Errorf("policy: invalid-convention: convention must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := strings.ToLower(strings.TrimSpace(node.Content[i].Value))
		value := node.Content[i+1]
		switch key {
		case "severity_limit", "severity_limits":
			if value.Kind != yaml.MappingNode {
				return conv, fmt.Errorf("policy: invalid-convention: %s must be a mapping of characteristic to severity", key)
			}
			for j := 0; j+1 < len(value.Content); j += 2 {
				name := Characteristic(strings.ToLower(strings.TrimSpace(value.Content[j].Value)))
				if !knownCharacteristics[name] {
					return conv, fmt.Errorf("%w: %q is not an ISO/IEC 25010 characteristic", ErrUnknownCharacteristic, string(name))
				}
				var raw string
				if err := value.Content[j+1].Decode(&raw); err != nil {
					return conv, fmt.Errorf("policy: invalid-convention: %s: %q: %v", key, string(name), err)
				}
				sev := Severity(strings.ToLower(strings.TrimSpace(raw)))
				if !validSeverity(sev) {
					return conv, fmt.Errorf("%w: %q (want error, warning or info)", ErrInvalidSeverity, raw)
				}
				conv.SeverityLimit[name] = sev
			}
		case "blocking":
			list, err := characteristicList(value)
			if err != nil {
				return conv, err
			}
			conv.Blocking = list
		case "informative":
			list, err := characteristicList(value)
			if err != nil {
				return conv, err
			}
			conv.Informative = list
		default:
			return conv, fmt.Errorf("policy: unknown-convention-field: %q", key)
		}
	}
	if overlap := firstOverlap(conv.Blocking, conv.Informative); overlap != "" {
		return conv, fmt.Errorf("%w: %q", ErrConventionOverlap, string(overlap))
	}
	return conv, nil
}

func characteristicList(node *yaml.Node) ([]Characteristic, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("policy: invalid-convention: expected a sequence of characteristics")
	}
	list := make([]Characteristic, 0, len(node.Content))
	for _, item := range node.Content {
		name := Characteristic(strings.ToLower(strings.TrimSpace(item.Value)))
		if !knownCharacteristics[name] {
			return nil, fmt.Errorf("%w: %q is not an ISO/IEC 25010 characteristic", ErrUnknownCharacteristic, string(name))
		}
		list = append(list, name)
	}
	return list, nil
}

func firstOverlap(a, b []Characteristic) Characteristic {
	in := make(map[Characteristic]bool, len(a))
	for _, c := range a {
		in[c] = true
	}
	for _, c := range b {
		if in[c] {
			return c
		}
	}
	return ""
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if strings.EqualFold(strings.TrimSpace(m.Content[i].Value), key) {
			return m.Content[i+1]
		}
	}
	return nil
}

// scanRefusedClauses walks the document mapping and refuses any clause
// whose value would switch off the deterministic security pass or secret
// redaction. It runs on the raw YAML so it sees every spelling a hand
// written file might use, not only the fields the Policy struct knows.
func scanRefusedClauses(m *yaml.Node, prefix string) error {
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := strings.ToLower(strings.TrimSpace(m.Content[i].Value))
		value := resolveAlias(m.Content[i+1])
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		securityPass := key == "security_pass" || key == "securitypass" ||
			key == "security_pass_enabled" || key == "disable_security_pass" ||
			(prefix == "security" && (key == "pass" || key == "pass_enabled"))
		redaction := key == "redaction" || key == "redact_secrets" ||
			key == "secret_redaction" || key == "redaction_enabled" ||
			key == "disable_redaction"
		securityOff := key == "security" && isDisabling(value)

		if isDisabling(value) {
			switch {
			case securityPass:
				return fmt.Errorf("%w: %q: explicit policy cannot disable the deterministic security pass", ErrRefusedClause, full)
			case redaction:
				return fmt.Errorf("%w: %q: explicit policy cannot disable secret redaction", ErrRefusedClause, full)
			case securityOff:
				return fmt.Errorf("%w: %q: explicit policy cannot disable the deterministic security pass", ErrRefusedClause, full)
			}
		}
		if value.Kind == yaml.MappingNode {
			if err := scanRefusedClauses(value, full); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveAlias dereferences a YAML alias node to the anchor it points at.
// A hand-written file can spell a disabling clause as `redaction: *off`
// with `off: &off false` elsewhere; an alias is an AliasNode, not a
// ScalarNode, so without this the disabling value would slip past both
// the scalar spelling checks and isDisabling.
func resolveAlias(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		return n.Alias
	}
	return n
}

func isDisabling(n *yaml.Node) bool {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.ScalarNode {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(n.Value)) {
	case "false", "off", "disabled", "disable", "no", "none", "0", "null", "":
		return true
	}
	return false
}

// Evaluate applies a policy to a review's ISO scores. A nil policy (the
// zero-config case) returns (nil, nil): there is nothing to weigh and the
// caller keeps today's output. Reports are coherent with the file by
// construction -- each contribution is score*weight and the aggregate is
// the rounded sum of those contributions, over the canonical order.
func Evaluate(p *Policy, scores types.ISOScores) (*Evaluation, error) {
	if p == nil {
		return nil, nil
	}
	ev := &Evaluation{Enabled: true, Source: p.Source}
	var exact float64
	for _, c := range AllCharacteristics() {
		weight := p.Weights[c]
		score := scoreFor(c, scores)
		contribution := float64(score) * weight
		exact += contribution
		ev.PerCharacteristic = append(ev.PerCharacteristic, CharacteristicScore{
			Characteristic: c,
			Score:          score,
			Weight:         weight,
			Contribution:   contribution,
			SeverityLimit:  p.Convention.SeverityLimit[c],
			Blocking:       containsCharacteristic(p.Convention.Blocking, c),
			Informative:    containsCharacteristic(p.Convention.Informative, c),
		})
	}
	ev.AggregateExact = exact
	ev.Aggregate = int(math.Round(exact))
	return ev, nil
}

// Render serializes a review together with its policy evaluation. When the
// evaluation is nil or disabled -- no file, or a file that produced no
// policy -- the bytes are exactly json.Marshal(result), so the
// zero-configuration output is byte-identical to today's.
func Render(result *types.ReviewResult, ev *Evaluation) ([]byte, error) {
	if ev == nil || !ev.Enabled {
		return json.Marshal(result)
	}
	return json.Marshal(struct {
		*types.ReviewResult
		ISOPolicy *Evaluation `json:"iso_policy"`
	}{result, ev})
}

func containsCharacteristic(list []Characteristic, c Characteristic) bool {
	for _, item := range list {
		if item == c {
			return true
		}
	}
	return false
}

func scoreFor(c Characteristic, s types.ISOScores) int {
	switch c {
	case Functionality:
		return s.Functionality
	case Reliability:
		return s.Reliability
	case Usability:
		return s.Usability
	case Efficiency:
		return s.Efficiency
	case Maintainability:
		return s.Maintainability
	case Portability:
		return s.Portability
	case Security:
		return s.Security
	case Compatibility:
		return s.Compatibility
	}
	return 0
}
