package browserproof

import (
	"fmt"
	"strings"
)

// The scanner below understands the well-formed markup a documentation build
// emits: balanced tags, quoted attributes, no scripting-time DOM mutation. It is
// the rendering side of the in-process test double only; the pinned external
// driver reports what a real browser resolved.

type tokenKind int

const (
	tokenText tokenKind = iota
	tokenStart
	tokenEnd
)

type htmlToken struct {
	kind    tokenKind
	name    string
	id      string
	classes []string
	href    string
	src     string
	text    string
	void    bool
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// hiddenElements never contribute rendered text.
var hiddenElements = map[string]bool{
	"head": true, "script": true, "style": true, "template": true, "noscript": true,
}

type selectorSpec struct {
	tag   string
	id    string
	class string
}

// parseSelector accepts the subset a documentation assertion needs: "tag",
// "#id", ".class" and the "tag#id" / "tag.class" combinations.
func parseSelector(selector string) (selectorSpec, error) {
	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		return selectorSpec{}, fmt.Errorf("browserproof: empty selector")
	}
	if strings.ContainsAny(trimmed, " \t>,+~[]:*()") {
		return selectorSpec{}, fmt.Errorf("browserproof: selector %q uses an unsupported construct", selector)
	}

	cut := strings.IndexAny(trimmed, "#.")
	if cut < 0 {
		return selectorSpec{tag: strings.ToLower(trimmed)}, nil
	}

	spec := selectorSpec{tag: strings.ToLower(trimmed[:cut])}
	rest := trimmed[cut:]
	if strings.Count(rest, "#")+strings.Count(rest, ".") > 1 {
		return selectorSpec{}, fmt.Errorf("browserproof: selector %q uses an unsupported construct", selector)
	}
	if len(rest) < 2 {
		return selectorSpec{}, fmt.Errorf("browserproof: selector %q is incomplete", selector)
	}

	if rest[0] == '#' {
		spec.id = rest[1:]
	} else {
		spec.class = rest[1:]
	}

	return spec, nil
}

func (s selectorSpec) matches(tok htmlToken) bool {
	if s.tag != "" && s.tag != tok.name {
		return false
	}
	if s.id != "" && s.id != tok.id {
		return false
	}
	if s.class != "" {
		for _, class := range tok.classes {
			if class == s.class {
				return true
			}
		}
		return false
	}

	return s.tag != "" || s.id != ""
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f'
}

func tokenizeHTML(src string) []htmlToken {
	tokens := make([]htmlToken, 0, 64)

	for i := 0; i < len(src); {
		if src[i] != '<' {
			next := strings.IndexByte(src[i:], '<')
			var run string
			if next < 0 {
				run, i = src[i:], len(src)
			} else {
				run, i = src[i:i+next], i+next
			}
			if strings.TrimSpace(run) != "" {
				tokens = append(tokens, htmlToken{kind: tokenText, text: run})
			}
			continue
		}

		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}

		if i+1 < len(src) && (src[i+1] == '!' || src[i+1] == '?') {
			end := strings.IndexByte(src[i:], '>')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}

		if i+1 < len(src) && src[i+1] == '/' {
			end := strings.IndexByte(src[i:], '>')
			if end < 0 {
				break
			}
			tokens = append(tokens, htmlToken{
				kind: tokenEnd,
				name: strings.ToLower(strings.TrimSpace(src[i+2 : i+end])),
			})
			i += end + 1
			continue
		}

		stop := i + 1
		quote := byte(0)
		for ; stop < len(src); stop++ {
			c := src[stop]
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				continue
			}
			if c == '>' {
				break
			}
		}
		if stop >= len(src) {
			break
		}

		tok := parseStartTag(src[i+1 : stop])
		i = stop + 1
		if tok.name != "" {
			tokens = append(tokens, tok)
		}
	}

	return tokens
}

func parseStartTag(raw string) htmlToken {
	tok := htmlToken{kind: tokenStart}

	raw = strings.TrimSpace(raw)
	if strings.HasSuffix(raw, "/") {
		tok.void = true
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	}

	cut := strings.IndexFunc(raw, func(r rune) bool {
		return r < 128 && isHTMLSpace(byte(r))
	})
	if cut < 0 {
		tok.name = strings.ToLower(raw)
		tok.void = tok.void || voidElements[tok.name]
		return tok
	}

	tok.name = strings.ToLower(raw[:cut])
	tok.void = tok.void || voidElements[tok.name]

	eachAttribute(raw[cut:], func(key, value string) {
		switch key {
		case "id":
			tok.id = value
		case "class":
			tok.classes = strings.Fields(value)
		case "href":
			tok.href = value
		case "src":
			tok.src = value
		}
	})

	return tok
}

func eachAttribute(raw string, visit func(key, value string)) {
	i := 0
	for i < len(raw) {
		for i < len(raw) && isHTMLSpace(raw[i]) {
			i++
		}
		start := i
		for i < len(raw) && !isHTMLSpace(raw[i]) && raw[i] != '=' {
			i++
		}
		key := strings.ToLower(raw[start:i])
		if key == "" {
			return
		}

		for i < len(raw) && isHTMLSpace(raw[i]) {
			i++
		}
		value := ""
		if i < len(raw) && raw[i] == '=' {
			i++
			for i < len(raw) && isHTMLSpace(raw[i]) {
				i++
			}
			if i < len(raw) && (raw[i] == '"' || raw[i] == '\'') {
				quote := raw[i]
				i++
				valueStart := i
				for i < len(raw) && raw[i] != quote {
					i++
				}
				value = raw[valueStart:i]
				if i < len(raw) {
					i++
				}
			} else {
				valueStart := i
				for i < len(raw) && !isHTMLSpace(raw[i]) {
					i++
				}
				value = raw[valueStart:i]
			}
		}

		visit(key, value)
	}
}

func extractLinks(tokens []htmlToken) []string {
	links := make([]string, 0, 8)
	for _, tok := range tokens {
		if tok.kind == tokenStart && tok.name == "a" && tok.href != "" {
			links = append(links, tok.href)
		}
	}

	return links
}

func visibleText(tokens []htmlToken) string {
	var text strings.Builder
	hidden := 0

	for _, tok := range tokens {
		switch tok.kind {
		case tokenStart:
			if hiddenElements[tok.name] && !tok.void {
				hidden++
			}
		case tokenEnd:
			if hiddenElements[tok.name] && hidden > 0 {
				hidden--
			}
		case tokenText:
			if hidden == 0 {
				text.WriteString(tok.text)
				text.WriteByte(' ')
			}
		}
	}

	return collapseSpaces(text.String())
}

// selectorText returns the rendered text of the first element matching the
// selector, mirroring what a browser exposes as textContent.
func selectorText(tokens []htmlToken, spec selectorSpec) (string, bool) {
	var text strings.Builder

	depth, captureDepth, hidden := 0, 0, 0
	capturing, found := false, false

	for _, tok := range tokens {
		switch tok.kind {
		case tokenStart:
			if !found && spec.matches(tok) {
				found = true
				if !tok.void {
					capturing = true
					captureDepth = depth
				}
			}
			if hiddenElements[tok.name] && !tok.void {
				hidden++
			}
			if !tok.void {
				depth++
			}
		case tokenEnd:
			if hiddenElements[tok.name] && hidden > 0 {
				hidden--
			}
			if depth > 0 {
				depth--
			}
			if capturing && depth <= captureDepth {
				capturing = false
			}
		case tokenText:
			if capturing && hidden == 0 {
				text.WriteString(tok.text)
				text.WriteByte(' ')
			}
		}
	}

	return collapseSpaces(text.String()), found
}

// hasExecutableScript reports whether the page needs a scripting engine to reach
// its final rendered state.
func hasExecutableScript(tokens []htmlToken) bool {
	inScript := false

	for _, tok := range tokens {
		switch tok.kind {
		case tokenStart:
			if tok.name == "script" {
				if tok.src != "" {
					return true
				}
				inScript = true
			}
		case tokenEnd:
			if tok.name == "script" {
				inScript = false
			}
		case tokenText:
			if inScript && strings.TrimSpace(tok.text) != "" {
				return true
			}
		}
	}

	return false
}

var entityReplacer = strings.NewReplacer(
	"&nbsp;", " ",
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&#39;", "'",
	"&apos;", "'",
)

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(entityReplacer.Replace(s)), " ")
}
