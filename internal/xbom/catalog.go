// Package xbom is AUR-552's generation of xBOMs beyond the SBOM (Build BOM
// and CBOM) in CycloneDX 1.6. Candidate collection is DATA-DRIVEN: the
// engine in this package knows no language, tool or algorithm; everything it
// looks for is declared in a catalog (catalog/<type>.yml, embedded as the
// default and overridable per repository or central policy). An LLM may
// classify and enrich candidates, but a component only reaches the BOM when
// the line it cites, re-read from disk, contains its evidence token.
package xbom

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed catalog/*.yml
var catalogFS embed.FS

//go:embed prompt/*.md
var promptFS embed.FS

// Catalog is one parsed and validated catalog file.
type Catalog struct {
	Version  int                 `yaml:"version"`
	Type     string              `yaml:"type"`
	FileSets map[string][]string `yaml:"file_sets"`
	Exclude  []string            `yaml:"exclude"`
	Entries  []Entry             `yaml:"entries"`

	// Source is where the catalog came from: "embedded", "repository" or
	// "policy". Recorded in metadata.properties.
	Source string `yaml:"-"`

	exclude []*regexp.Regexp
}

// Entry is one extractor: files to read, a line pattern with named groups,
// the group that is the evidence token and the component template.
type Entry struct {
	ID        string            `yaml:"id"`
	Files     []string          `yaml:"files"`
	Pattern   string            `yaml:"pattern"`
	Token     string            `yaml:"token"`
	Component ComponentTemplate `yaml:"component"`

	re    *regexp.Regexp
	globs []*regexp.Regexp
}

// ComponentTemplate is the CycloneDX component an entry produces. Strings may
// use {group}, {a?b} (first non-empty group), {group|filter...} and, as a
// whole value, {group:int}. See docs/specs/AUR-552.md.
type ComponentTemplate struct {
	Type        string            `yaml:"type"`
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Purl        string            `yaml:"purl"`
	Description string            `yaml:"description"`
	Properties  map[string]string `yaml:"properties"`
	Crypto      map[string]any    `yaml:"crypto"`
}

// cycloneDXComponentTypes is the CycloneDX 1.6 component type vocabulary
// (a spec enumeration, not a detection list).
var cycloneDXComponentTypes = map[string]bool{
	"application": true, "framework": true, "library": true, "container": true,
	"platform": true, "operating-system": true, "device": true, "device-driver": true,
	"firmware": true, "file": true, "machine-learning-model": true, "data": true,
	"cryptographic-asset": true,
}

// cryptoAssetTypes is the CycloneDX 1.6 cryptoProperties.assetType enumeration.
var cryptoAssetTypes = map[string]bool{
	"algorithm": true, "certificate": true, "protocol": true, "related-crypto-material": true,
}

// Types lists the catalog types embedded in the binary.
func Types() []string {
	ents, _ := catalogFS.ReadDir("catalog")
	var out []string
	for _, e := range ents {
		if n := e.Name(); strings.HasSuffix(n, ".yml") {
			out = append(out, strings.TrimSuffix(n, ".yml"))
		}
	}
	sort.Strings(out)
	return out
}

// ParseCatalog parses and validates catalog YAML. Unknown keys are refused.
func ParseCatalog(data []byte, wantType, source string) (*Catalog, error) {
	var c Catalog
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("catalog %s (%s): %w", wantType, source, err)
	}
	c.Source = source
	if err := c.validate(wantType); err != nil {
		return nil, fmt.Errorf("catalog %s (%s): %w", wantType, source, err)
	}
	return &c, nil
}

func (c *Catalog) validate(wantType string) error {
	if c.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", c.Version)
	}
	if c.Type != wantType {
		return fmt.Errorf("type %q does not match the requested type %q", c.Type, wantType)
	}
	if len(c.Entries) == 0 {
		return fmt.Errorf("entries must not be empty")
	}
	for _, g := range c.Exclude {
		re, err := compileGlob(g)
		if err != nil {
			return fmt.Errorf("exclude %q: %w", g, err)
		}
		c.exclude = append(c.exclude, re)
	}
	seen := map[string]bool{}
	for i := range c.Entries {
		e := &c.Entries[i]
		if strings.TrimSpace(e.ID) == "" {
			return fmt.Errorf("entries[%d]: id must not be empty", i)
		}
		if seen[e.ID] {
			return fmt.Errorf("entries[%d]: duplicate id %q", i, e.ID)
		}
		seen[e.ID] = true
		if err := e.validate(c.FileSets); err != nil {
			return fmt.Errorf("entry %q: %w", e.ID, err)
		}
	}
	return nil
}

func (e *Entry) validate(sets map[string][]string) error {
	patterns, err := expandFileSets(e.Files, sets)
	if err != nil {
		return err
	}
	if len(patterns) == 0 {
		return fmt.Errorf("files must not be empty")
	}
	for _, g := range patterns {
		re, err := compileGlob(g)
		if err != nil {
			return fmt.Errorf("files %q: %w", g, err)
		}
		e.globs = append(e.globs, re)
	}
	if strings.TrimSpace(e.Pattern) == "" {
		return fmt.Errorf("pattern must not be empty")
	}
	re, err := regexp.Compile(e.Pattern)
	if err != nil {
		return fmt.Errorf("pattern: %w", err)
	}
	e.re = re
	groups := map[string]bool{}
	for _, n := range re.SubexpNames() {
		if n != "" {
			groups[n] = true
		}
	}
	if !groups[e.Token] {
		return fmt.Errorf("token %q is not a named group of the pattern", e.Token)
	}
	t := e.Component
	if !cycloneDXComponentTypes[t.Type] {
		return fmt.Errorf("component.type %q is not a CycloneDX component type", t.Type)
	}
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("component.name must not be empty")
	}
	if t.Type == "cryptographic-asset" {
		at, _ := t.Crypto["assetType"].(string)
		if !cryptoAssetTypes[at] {
			return fmt.Errorf("component.crypto.assetType %q is not a CycloneDX cryptoProperties assetType", at)
		}
	} else if len(t.Crypto) > 0 {
		return fmt.Errorf("component.crypto is only valid for type cryptographic-asset")
	}
	// Every placeholder must reference a group the pattern defines.
	var check func(v any) error
	check = func(v any) error {
		switch x := v.(type) {
		case string:
			return checkPlaceholders(x, groups)
		case []any:
			for _, i := range x {
				if err := check(i); err != nil {
					return err
				}
			}
		case map[string]any:
			for _, i := range x {
				if err := check(i); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, s := range []string{t.Name, t.Version, t.Purl, t.Description} {
		if err := check(s); err != nil {
			return err
		}
	}
	for _, s := range t.Properties {
		if err := check(s); err != nil {
			return err
		}
	}
	return check(map[string]any(t.Crypto))
}

func expandFileSets(files []string, sets map[string][]string) ([]string, error) {
	var out []string
	for _, f := range files {
		if strings.HasPrefix(f, "@") {
			s, ok := sets[f[1:]]
			if !ok {
				return nil, fmt.Errorf("files: unknown file_set %q", f)
			}
			out = append(out, s...)
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// compileGlob translates a slash-separated glob (* ? ** ) into an anchored
// regular expression over repo-relative slash paths.
func compileGlob(g string) (*regexp.Regexp, error) {
	g = strings.TrimSpace(g)
	if g == "" || strings.HasPrefix(g, "/") || strings.Contains(g, "..") {
		return nil, fmt.Errorf("glob must be a non-empty relative pattern without '..'")
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(g); {
		switch {
		case strings.HasPrefix(g[i:], "**/"):
			b.WriteString(`(?:.*/)?`)
			i += 3
		case strings.HasPrefix(g[i:], "**"):
			b.WriteString(`.*`)
			i += 2
		case g[i] == '*':
			b.WriteString(`[^/]*`)
			i++
		case g[i] == '?':
			b.WriteString(`[^/]`)
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(g[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func (c *Catalog) excluded(rel string, isDir bool) bool {
	p := rel
	if isDir {
		p += "/"
	}
	for _, re := range c.exclude {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// CatalogResolution says where a catalog came from and what was ignored.
type CatalogResolution struct {
	Catalog  *Catalog
	Warnings []string
}

// overridePath is the repo-relative location of an override for a type.
func overridePath(root, typ, ext string) string {
	return filepath.Join(root, ".aurumcode", "xbom", typ+ext)
}

// readOverride reads an override file. It returns (nil, nil) when the file
// does not exist; a symlink or non-regular file is refused (fail closed).
func readOverride(p string) ([]byte, error) {
	fi, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: override must be a regular file, not a symlink or directory", p)
	}
	return os.ReadFile(p)
}

// LoadCatalog resolves the catalog for a type with per-section precedence,
// like quality_gates: the central policy's .aurumcode/xbom/<type>.yml wins
// outright over the repository's, which wins over the embedded default. An
// override that is invalid is an error, never a silent fallback.
func LoadCatalog(typ, repoRoot, policyDir string) (*CatalogResolution, error) {
	embedded, err := catalogFS.ReadFile("catalog/" + typ + ".yml")
	if err != nil {
		return nil, fmt.Errorf("unknown xbom type %q (available: %s)", typ, strings.Join(Types(), ", "))
	}
	var warnings []string
	var policyData, repoData []byte
	if strings.TrimSpace(policyDir) != "" {
		if policyData, err = readOverride(overridePath(policyDir, typ, ".yml")); err != nil {
			return nil, err
		}
	}
	if repoData, err = readOverride(overridePath(repoRoot, typ, ".yml")); err != nil {
		return nil, err
	}
	switch {
	case policyData != nil:
		if repoData != nil {
			warnings = append(warnings, fmt.Sprintf(".aurumcode/xbom/%s.yml do repositorio ignorado: a politica central decide sozinha", typ))
		}
		c, err := ParseCatalog(policyData, typ, "policy")
		return &CatalogResolution{c, warnings}, err
	case repoData != nil:
		c, err := ParseCatalog(repoData, typ, "repository")
		return &CatalogResolution{c, warnings}, err
	}
	c, err := ParseCatalog(embedded, typ, "embedded")
	return &CatalogResolution{c, warnings}, err
}

// LoadPrompt returns the LLM prompt for a type: the central policy's
// .aurumcode/xbom/<type>.md when present, otherwise the embedded one. The
// repository under review cannot override it (it would be prompt injection
// by the reviewed tree).
func LoadPrompt(typ, policyDir string) (string, error) {
	if strings.TrimSpace(policyDir) != "" {
		data, err := readOverride(overridePath(policyDir, typ, ".md"))
		if err != nil {
			return "", err
		}
		if data != nil {
			return string(data), nil
		}
	}
	data, err := promptFS.ReadFile(path.Join("prompt", typ+".md"))
	if err != nil {
		return "", fmt.Errorf("no prompt for xbom type %q", typ)
	}
	return string(data), nil
}
