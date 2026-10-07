// Package profiles is the declarative catalog of LLM provider profiles: what
// each known OpenAI-compatible Chat Completions endpoint expects on the wire
// (base URL, authentication, path, query, reply-cap field, structured
// output). The catalog is data (profiles.yml, embedded), extensible by an
// operator file; no provider is described in code.
package profiles

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
)

//go:embed profiles.yml
var embeddedCatalog []byte

// Profile is one catalog entry, as written in the yml.
type Profile struct {
	Description      string            `yaml:"description"`
	BaseURL          string            `yaml:"base_url"`
	KeyEnv           []string          `yaml:"key_env"`
	Auth             string            `yaml:"auth"`
	AuthHeader       string            `yaml:"auth_header"`
	Path             string            `yaml:"path"`
	Query            map[string]string `yaml:"query"`
	TokenField       string            `yaml:"token_field"`
	StructuredOutput string            `yaml:"structured_output"`
}

// Catalog maps a profile name to its profile.
type Catalog map[string]Profile

type catalogFile struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

// Embedded returns the catalog shipped with the binary.
func Embedded() (Catalog, error) {
	return parse(embeddedCatalog, "embedded profiles.yml")
}

// Merge returns c with every profile of the operator file content added or,
// by name, replaced whole. origin names the file in errors.
func (c Catalog) Merge(content []byte, origin string) (Catalog, error) {
	extra, err := parse(content, origin)
	if err != nil {
		return nil, err
	}
	merged := Catalog{}
	for name, p := range c {
		merged[name] = p
	}
	for name, p := range extra {
		merged[name] = p
	}
	return merged, nil
}

// Names lists the profile names, sorted.
func (c Catalog) Names() []string {
	names := make([]string, 0, len(c))
	for name := range c {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Lookup returns the named profile, or an error that lists the valid ones.
func (c Catalog) Lookup(name string) (Profile, error) {
	p, ok := c[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown LLM provider profile %q; valid profiles: %s", name, strings.Join(c.Names(), ", "))
	}
	return p, nil
}

// parse decodes and validates a catalog file: every profile must name only
// values the engine knows, so a typo fails at load, never on the wire.
func parse(content []byte, origin string) (Catalog, error) {
	var file catalogFile
	if err := yaml.Unmarshal(content, &file); err != nil {
		return nil, fmt.Errorf("reading %s: %w", origin, err)
	}
	for name, p := range file.Profiles {
		if err := p.validate(); err != nil {
			return nil, fmt.Errorf("%s: profile %q: %w", origin, name, err)
		}
	}
	return Catalog(file.Profiles), nil
}

func (p Profile) validate() error {
	switch litellm.AuthStyle(p.Auth) {
	case litellm.AuthBearer, litellm.AuthNone:
	case litellm.AuthHeaderKey:
		if p.AuthHeader == "" {
			return fmt.Errorf("auth %q needs auth_header", p.Auth)
		}
	default:
		return fmt.Errorf("auth %q is not one of bearer, header, none", p.Auth)
	}
	switch litellm.TokenField(p.TokenField) {
	case litellm.TokenFieldMaxTokens, litellm.TokenFieldMaxCompletionTokens:
	default:
		return fmt.Errorf("token_field %q is not one of max_tokens, max_completion_tokens", p.TokenField)
	}
	switch litellm.StructuredOutput(p.StructuredOutput) {
	case litellm.StructuredJSONSchema, litellm.StructuredJSONObject, litellm.StructuredNone:
	default:
		return fmt.Errorf("structured_output %q is not one of json_schema, json_object, none", p.StructuredOutput)
	}
	if !strings.HasPrefix(p.Path, "/") {
		return fmt.Errorf("path %q must start with /", p.Path)
	}
	if len(p.KeyEnv) == 0 && litellm.AuthStyle(p.Auth) != litellm.AuthNone {
		return fmt.Errorf("auth %q needs key_env", p.Auth)
	}
	return nil
}
