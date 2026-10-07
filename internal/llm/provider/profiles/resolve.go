package profiles

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
)

// Environment variables the selection reads. They are operator settings:
// the provider, its URL and its key never come from the repository under
// review, which could otherwise point the key and the diff at its own host.
const (
	// EnvProvider names the profile to use.
	EnvProvider = "LLM_PROVIDER"
	// EnvBaseURL overrides the profile's base URL.
	EnvBaseURL = "LLM_BASE_URL"
	// EnvProvidersFile points to an operator catalog merged over the embedded one.
	EnvProvidersFile = "LLM_PROVIDERS_FILE"
)

// Endpoint is a resolved profile: the dialect plus the URL and key to use.
type Endpoint struct {
	Profile string
	Dialect litellm.Dialect
	BaseURL string
	APIKey  string
}

// placeholder is {VAR} or {VAR:-default} inside a base URL or query value.
var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// FromEnv resolves the profile named by LLM_PROVIDER. The boolean is false
// when LLM_PROVIDER is unset: the caller keeps its previous behaviour. Any
// set-but-unusable configuration is an error, never a silent fallback.
func FromEnv(getenv func(string) string) (Endpoint, bool, error) {
	name := strings.TrimSpace(getenv(EnvProvider))
	if name == "" {
		return Endpoint{}, false, nil
	}
	catalog, err := Embedded()
	if err != nil {
		return Endpoint{}, true, err
	}
	if path := getenv(EnvProvidersFile); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return Endpoint{}, true, fmt.Errorf("reading %s=%s: %w", EnvProvidersFile, path, err)
		}
		if catalog, err = catalog.Merge(content, path); err != nil {
			return Endpoint{}, true, err
		}
	}
	endpoint, err := catalog.Resolve(name, getenv)
	return endpoint, true, err
}

// Resolve turns the named profile into an endpoint, reading the URL
// placeholders and the key from getenv.
func (c Catalog) Resolve(name string, getenv func(string) string) (Endpoint, error) {
	p, err := c.Lookup(name)
	if err != nil {
		return Endpoint{}, err
	}
	baseURL := getenv(EnvBaseURL)
	if baseURL == "" {
		if p.BaseURL == "" {
			return Endpoint{}, fmt.Errorf("LLM provider %q has no default URL: set %s", name, EnvBaseURL)
		}
		if baseURL, err = expand(p.BaseURL, getenv); err != nil {
			return Endpoint{}, fmt.Errorf("LLM provider %q base_url: %w", name, err)
		}
	}
	query := map[string]string{}
	for k, v := range p.Query {
		if query[k], err = expand(v, getenv); err != nil {
			return Endpoint{}, fmt.Errorf("LLM provider %q query %s: %w", name, k, err)
		}
	}
	key := firstSet(p.KeyEnv, getenv)
	auth := litellm.AuthStyle(p.Auth)
	if key == "" && auth != litellm.AuthNone {
		return Endpoint{}, fmt.Errorf("LLM provider %q needs an API key: set one of %s", name, strings.Join(p.KeyEnv, ", "))
	}
	return Endpoint{
		Profile: name,
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  key,
		Dialect: litellm.Dialect{
			Name:             name,
			Auth:             auth,
			AuthHeader:       p.AuthHeader,
			Path:             p.Path,
			Query:            query,
			TokenField:       litellm.TokenField(p.TokenField),
			StructuredOutput: litellm.StructuredOutput(p.StructuredOutput),
		},
	}, nil
}

// expand replaces every placeholder; an unset variable without a default
// is an error, so no request ever leaves with a literal "{VAR}" in its URL.
func expand(template string, getenv func(string) string) (string, error) {
	var missing []string
	out := placeholder.ReplaceAllStringFunc(template, func(m string) string {
		parts := placeholder.FindStringSubmatch(m)
		if v := getenv(parts[1]); v != "" {
			return v
		}
		if strings.Contains(m, ":-") {
			return parts[2]
		}
		missing = append(missing, parts[1])
		return m
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("set %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func firstSet(names []string, getenv func(string) string) string {
	for _, n := range names {
		if v := getenv(n); v != "" {
			return v
		}
	}
	return ""
}
