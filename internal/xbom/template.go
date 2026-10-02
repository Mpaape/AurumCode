package xbom

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholderRE = regexp.MustCompile(`\{([^{}]+)\}`)

type placeholder struct {
	groups  []string
	filters []string
	asInt   bool
}

var filters = map[string]func(string) string{
	"upper":  strings.ToUpper,
	"lower":  strings.ToLower,
	"trim":   strings.TrimSpace,
	"nodash": func(s string) string { return strings.NewReplacer("-", "", "_", "").Replace(s) },
	"dot":    func(s string) string { return strings.NewReplacer("_", ".", "-", ".").Replace(s) },
}

func parsePlaceholder(s string) (placeholder, error) {
	parts := strings.Split(s, "|")
	expr := parts[0]
	var p placeholder
	if strings.HasSuffix(expr, ":int") {
		p.asInt = true
		expr = strings.TrimSuffix(expr, ":int")
	}
	p.groups = strings.Split(expr, "?")
	for _, g := range p.groups {
		if g == "" {
			return p, fmt.Errorf("placeholder {%s}: empty group name", s)
		}
	}
	for _, f := range parts[1:] {
		if _, ok := filters[f]; !ok {
			return p, fmt.Errorf("placeholder {%s}: unknown filter %q", s, f)
		}
		p.filters = append(p.filters, f)
	}
	return p, nil
}

func checkPlaceholders(s string, groups map[string]bool) error {
	for _, m := range placeholderRE.FindAllStringSubmatch(s, -1) {
		p, err := parsePlaceholder(m[1])
		if err != nil {
			return err
		}
		for _, g := range p.groups {
			if !groups[g] {
				return fmt.Errorf("placeholder {%s} references %q, which is not a named group of the pattern", m[1], g)
			}
		}
	}
	return nil
}

func (p placeholder) eval(g map[string]string) string {
	v := ""
	for _, name := range p.groups {
		if g[name] != "" {
			v = g[name]
			break
		}
	}
	for _, f := range p.filters {
		v = filters[f](v)
	}
	return v
}

func renderString(t string, g map[string]string) string {
	return placeholderRE.ReplaceAllStringFunc(t, func(m string) string {
		p, err := parsePlaceholder(m[1 : len(m)-1])
		if err != nil {
			return ""
		}
		return p.eval(g)
	})
}

// renderValue renders a crypto template value; empty results are dropped
// (nil), a whole-value {group:int} becomes an integer.
func renderValue(v any, g map[string]string) any {
	switch x := v.(type) {
	case string:
		if m := placeholderRE.FindStringSubmatch(x); m != nil && m[0] == x {
			if p, err := parsePlaceholder(m[1]); err == nil && p.asInt {
				n, err := strconv.Atoi(p.eval(g))
				if err != nil {
					return nil
				}
				return n
			}
		}
		s := renderString(x, g)
		if s == "" {
			return nil
		}
		return s
	case []any:
		out := make([]any, 0, len(x))
		for _, i := range x {
			if r := renderValue(i, g); r != nil {
				out = append(out, r)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, i := range x {
			if r := renderValue(i, g); r != nil {
				out[k] = r
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return v
}

func collapseName(n string) string {
	n = strings.TrimSpace(n)
	for strings.Contains(n, "--") {
		n = strings.ReplaceAll(n, "--", "-")
	}
	return strings.Trim(n, "-")
}
