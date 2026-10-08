package dependencies

import (
	"errors"
	"strings"
)

// LicenseVerdict is a license expression judged against the denied list.
type LicenseVerdict string

const (
	// LicenseAllowed: the package may be used under a license not denied.
	LicenseAllowed LicenseVerdict = "allowed"
	// LicenseDenied: every way to use the package goes through a denied
	// license.
	LicenseDenied LicenseVerdict = "denied"
	// LicenseUnknown: the expression cannot be judged (unparseable, a
	// LicenseRef, NOASSERTION, a non-standard license).
	LicenseUnknown LicenseVerdict = "unknown"
)

// EvaluateLicense judges an SPDX expression by its structure, never by
// substring: OR is denied only when every branch is denied (one allowed
// branch is a choice the user can make), AND is denied when any operand is,
// WITH judges the license it modifies. Identifiers compare
// case-insensitively. Anything the expression cannot vouch for is unknown.
func EvaluateLicense(expression string, denied []string) LicenseVerdict {
	set := deniedSet(denied)
	node, err := parseSPDX(expression)
	if err != nil {
		return LicenseUnknown
	}
	return node.verdict(set)
}

func deniedSet(denied []string) map[string]bool {
	set := map[string]bool{}
	for _, d := range denied {
		set[strings.ToLower(strings.TrimSpace(d))] = true
	}
	return set
}

// spdxNode is a parsed expression: a leaf license id, or an operator over
// operands.
type spdxNode struct {
	op       string // "", "AND", "OR"
	id       string
	operands []spdxNode
}

func (n spdxNode) verdict(denied map[string]bool) LicenseVerdict {
	switch n.op {
	case "OR":
		out := LicenseDenied
		for _, o := range n.operands {
			switch o.verdict(denied) {
			case LicenseAllowed:
				return LicenseAllowed
			case LicenseUnknown:
				out = LicenseUnknown
			}
		}
		return out
	case "AND":
		out := LicenseAllowed
		for _, o := range n.operands {
			switch o.verdict(denied) {
			case LicenseDenied:
				return LicenseDenied
			case LicenseUnknown:
				out = LicenseUnknown
			}
		}
		return out
	}
	return leafVerdict(n.id, denied)
}

// leafVerdict: a denied id is denied; an id that names no license the
// expression can vouch for is unknown; any other id is allowed.
func leafVerdict(id string, denied map[string]bool) LicenseVerdict {
	lower := strings.ToLower(id)
	switch {
	case denied[lower]:
		return LicenseDenied
	case lower == "noassertion", lower == "none", lower == "non-standard", lower == "unknown",
		strings.HasPrefix(lower, "licenseref-"), strings.HasPrefix(lower, "documentref-"):
		return LicenseUnknown
	}
	return LicenseAllowed
}

var errSPDX = errors.New("dependencies: unparseable SPDX expression")

// parseSPDX parses an expression: OR binds looser than AND, parentheses
// group, WITH attaches an exception to the license before it.
func parseSPDX(expression string) (spdxNode, error) {
	tokens := tokenizeSPDX(expression)
	if len(tokens) == 0 {
		return spdxNode{}, errSPDX
	}
	p := &spdxParser{tokens: tokens}
	node, err := p.parseOr()
	if err != nil || p.pos != len(p.tokens) {
		return spdxNode{}, errSPDX
	}
	return node, nil
}

func tokenizeSPDX(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "(", " ( "), ")", " ) ")
	return strings.Fields(s)
}

type spdxParser struct {
	tokens []string
	pos    int
}

func (p *spdxParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *spdxParser) parseOr() (spdxNode, error) {
	return p.parseList("OR", p.parseAnd)
}

func (p *spdxParser) parseAnd() (spdxNode, error) {
	return p.parseList("AND", p.parseTerm)
}

// parseList parses operands joined by op (case-insensitive).
func (p *spdxParser) parseList(op string, next func() (spdxNode, error)) (spdxNode, error) {
	first, err := next()
	if err != nil {
		return spdxNode{}, err
	}
	node := spdxNode{op: op, operands: []spdxNode{first}}
	for strings.EqualFold(p.peek(), op) {
		p.pos++
		operand, err := next()
		if err != nil {
			return spdxNode{}, err
		}
		node.operands = append(node.operands, operand)
	}
	if len(node.operands) == 1 {
		return first, nil
	}
	return node, nil
}

func (p *spdxParser) parseTerm() (spdxNode, error) {
	tok := p.peek()
	switch {
	case tok == "(":
		p.pos++
		node, err := p.parseOr()
		if err != nil || p.peek() != ")" {
			return spdxNode{}, errSPDX
		}
		p.pos++
		return node, nil
	case tok == "", tok == ")", strings.EqualFold(tok, "AND"), strings.EqualFold(tok, "OR"), strings.EqualFold(tok, "WITH"):
		return spdxNode{}, errSPDX
	}
	p.pos++
	if strings.EqualFold(p.peek(), "WITH") {
		p.pos++
		if exc := p.peek(); exc == "" || exc == "(" || exc == ")" {
			return spdxNode{}, errSPDX
		}
		p.pos++
	}
	return spdxNode{id: tok}, nil
}
