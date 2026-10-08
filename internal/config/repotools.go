package config

import (
	_ "embed"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// DefaultDeliberationMaxReadBytes is the default ceiling of bytes the
// repository tools return to the model over one review (AUR-526).
const DefaultDeliberationMaxReadBytes = 256 * 1024

//go:embed secret_paths.yml
var secretPathsYAML []byte

type secretPathCatalog struct {
	SecretPaths []string `yaml:"secret_paths"`
}

var (
	secretPathsOnce sync.Once
	secretPaths     []string
	secretPathsErr  error
)

// defaultSecretPaths is the embedded catalog of secret-file globs.
func defaultSecretPaths() ([]string, error) {
	secretPathsOnce.Do(func() {
		var c secretPathCatalog
		secretPathsErr = yaml.Unmarshal(secretPathsYAML, &c)
		secretPaths = c.SecretPaths
	})
	return secretPaths, secretPathsErr
}

// DefaultDeliberationMaxCacheBytes is the default ceiling of file bytes the
// repository tools keep in memory over one review.
const DefaultDeliberationMaxCacheBytes = 64 << 20

// EffectiveMaxCacheBytes is max_cache_bytes, defaulted.
func (d *DeliberationConfig) EffectiveMaxCacheBytes() int {
	if d != nil && d.MaxCacheBytes != 0 {
		return d.MaxCacheBytes
	}
	return DefaultDeliberationMaxCacheBytes
}

// EffectiveMaxReadBytes is max_read_bytes, defaulted.
func (d *DeliberationConfig) EffectiveMaxReadBytes() int {
	if d != nil && d.MaxReadBytes != 0 {
		return d.MaxReadBytes
	}
	return DefaultDeliberationMaxReadBytes
}

// IsSecretPath reports a path the repository tools must never read: one
// matching the embedded catalog or deliberation.secret_paths, ignoring
// case. An unreadable catalog fails closed: every path counts as secret.
func (c *Config) IsSecretPath(path string) bool {
	patterns, err := defaultSecretPaths()
	if err != nil {
		return true
	}
	if c != nil && c.Deliberation != nil {
		patterns = append(append([]string(nil), patterns...), c.Deliberation.SecretPaths...)
	}
	// Case-insensitive: .ENV and Credentials are the same secret file on a
	// case-insensitive filesystem and to anyone reading them.
	lowered := make([]string, len(patterns))
	for i, p := range patterns {
		lowered[i] = strings.ToLower(p)
	}
	return matchesAny(lowered, strings.ToLower(path))
}

// IgnoresPath reports a path the policy's ignore globs exclude from the
// review; the repository tools refuse it as well.
func (c *Config) IgnoresPath(path string) bool {
	return c != nil && matchesAny(c.Ignore, path)
}
