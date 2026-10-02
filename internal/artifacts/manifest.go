// Package artifacts implements AUR-533: the analysis-data artifact (a copy
// of the public OSV database plus the scanner versions the product pins),
// the builder a scheduled workflow runs, and the runtime client that
// resolves the newest published artifact, verifies every digest and refuses
// data older than the policy's maximum age.
//
// Nothing in this package knows an ecosystem, a language or a package: the
// ecosystems come from the public source's own listing at build time and
// from the manifest at run time.
package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SchemaV1 is the manifest schema identifier.
const SchemaV1 = "aurum-analysis-data/v1"

// ManifestName is the manifest's file (and release asset) name.
const ManifestName = "manifest.json"

// TagPrefix prefixes every release tag that carries this artifact.
const TagPrefix = "analysis-data/"

// File describes one payload file of the artifact.
type File struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"`
	Ecosystem string `json:"ecosystem,omitempty"`
}

// Source records where a payload came from.
type Source struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Manifest is manifest.json.
type Manifest struct {
	Schema      string            `json:"schema"`
	GeneratedAt string            `json:"generated_at"` // RFC 3339, UTC
	Sources     []Source          `json:"sources"`
	Files       []File            `json:"files"`
	Scanners    map[string]string `json:"scanners"`
	SetDigest   string            `json:"set_digest"`
}

// ComputeSetDigest hashes the sorted "path NUL sha256 LF" lines of files, so
// the digest covers names as well as contents.
func ComputeSetDigest(files []File) string {
	lines := make([]string, 0, len(files))
	for _, f := range files {
		lines = append(lines, f.Path+"\x00"+f.SHA256+"\n")
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		_, _ = io.WriteString(h, l)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// GeneratedTime parses generated_at; it must be UTC ("Z").
func (m *Manifest) GeneratedTime() (time.Time, error) {
	if !strings.HasSuffix(m.GeneratedAt, "Z") {
		return time.Time{}, fmt.Errorf("generated_at %q is not UTC", m.GeneratedAt)
	}
	t, err := time.Parse(time.RFC3339, m.GeneratedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("generated_at: %w", err)
	}
	return t.UTC(), nil
}

// Tag is the release tag this manifest is published under.
func (m *Manifest) Tag() (string, error) {
	t, err := m.GeneratedTime()
	if err != nil {
		return "", err
	}
	return TagPrefix + t.Format("20060102T150405Z"), nil
}

// ErrDigestMismatch marks any digest, size or set-digest divergence.
var ErrDigestMismatch = errors.New("digest mismatch")

// ParseManifest decodes and structurally validates a manifest: schema,
// date, safe relative paths, well-formed digests and a set digest that
// matches the file list. It does not read any payload file.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if m.Schema != SchemaV1 {
		return nil, fmt.Errorf("manifest: unsupported schema %q", m.Schema)
	}
	if _, err := m.GeneratedTime(); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if !safeName(f.Path) {
			return nil, fmt.Errorf("manifest: unsafe file path %q", f.Path)
		}
		if seen[f.Path] {
			return nil, fmt.Errorf("manifest: duplicate file path %q", f.Path)
		}
		seen[f.Path] = true
		if !validDigest(f.SHA256) {
			return nil, fmt.Errorf("manifest: file %q has malformed sha256", f.Path)
		}
		if f.Size < 0 {
			return nil, fmt.Errorf("manifest: file %q has negative size", f.Path)
		}
	}
	if got := ComputeSetDigest(m.Files); got != m.SetDigest {
		return nil, fmt.Errorf("manifest: set_digest %q does not match its files (%s): %w", m.SetDigest, got, ErrDigestMismatch)
	}
	return &m, nil
}

func validDigest(d string) bool {
	h, ok := strings.CutPrefix(d, "sha256:")
	if !ok || len(h) != 64 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

// safeName accepts a single flat file name: no separators, no traversal.
func safeName(p string) bool {
	return p != "" && p != "." && p != ".." && path.Base(p) == p && !strings.ContainsAny(p, `\/`) && p != ManifestName
}

// HashFile returns the sha256 digest and size of a file.
func HashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}

// VerifyFile checks one payload file in dir against its manifest entry.
func VerifyFile(dir string, f File) error {
	d, n, err := HashFile(filepath.Join(dir, f.Path))
	if err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	if n != f.Size || d != f.SHA256 {
		return fmt.Errorf("%s: expected %s (%d bytes), got %s (%d bytes): %w", f.Path, f.SHA256, f.Size, d, n, ErrDigestMismatch)
	}
	return nil
}

// VerifyDir re-reads manifest.json in dir and verifies every listed file.
func VerifyDir(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, err
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, err
	}
	for _, f := range m.Files {
		if err := VerifyFile(dir, f); err != nil {
			return nil, err
		}
	}
	return m, nil
}
