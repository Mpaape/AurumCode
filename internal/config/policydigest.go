package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// PolicyDigest is AC-001's policy-change-detection digest: sha256 over the
// central policy's own config.yml bytes, then every skill file its
// review.context.skills names (in that declared order -- deterministic
// because it is the policy author's own order, never re-sorted into a
// different one), hex-encoded. A nil c (no policy declared this run)
// returns "" -- an empty digest means no policy was active, never "a
// policy whose content happens to hash to the empty string".
//
// A skill file this function cannot read contributes only its own
// (trimmed) path to the digest and no error: LoadCentralPolicy has already
// refused to start the run at all when a non-optional skill was missing, so
// this is defense in depth, never the enforcement point -- and it must
// never panic or abort a review just to compute an audit-trail digest.
func (c *Config) PolicyDigest(policyDir string) string {
	if c == nil {
		return ""
	}
	h := sha256.New()
	configPath := filepath.Join(policyDir, DefaultConfigPath)
	if data, err := os.ReadFile(configPath); err == nil {
		h.Write(data)
	}
	for _, skill := range c.Review.Context.Skills {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			continue
		}
		h.Write([]byte("\x00" + skill + "\x00"))
		if data, err := os.ReadFile(filepath.Join(policyDir, filepath.FromSlash(skill))); err == nil {
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
