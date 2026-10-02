package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Reason codes of an unusable Outcome. Callers turn any of them into the
// policy's gate.inconclusive result with Outcome.Message as the reason.
const (
	ReasonUnavailable    = "analysis_data_unavailable"
	ReasonStale          = "analysis_data_stale"
	ReasonDigestMismatch = "analysis_data_digest_mismatch"
	ReasonInvalid        = "analysis_data_invalid"
)

// DefaultAPIBase is the GitHub REST API root.
const DefaultAPIBase = "https://api.github.com"

// maxClockSkew tolerates a runner clock slightly behind the publisher's.
const maxClockSkew = 10 * time.Minute

// Options configures Resolve. APIBase is injectable so tests point it at a
// local fake server; no real network is ever needed to exercise this code.
type Options struct {
	APIBase    string // DefaultAPIBase when empty
	Repository string // owner/name
	Token      string // optional; sent only to APIBase, never to asset hosts
	CacheDir   string // where verified artifacts live; temp dir when empty
	MaxAgeDays int
	Now        func() time.Time
	HTTP       *http.Client
	// Files restricts the download to these manifest paths (nil = all).
	// The manifest and its set digest are always verified in full.
	Files []string
	// Kinds restricts the download to manifest files of these kinds (nil =
	// all). Combined with Files, a file must match both when both are set.
	Kinds []string
}

// Outcome is Resolve's verdict. Usable is true only when a verified artifact
// within the maximum age is on disk in Dir.
type Outcome struct {
	Usable      bool
	Reason      string
	Detail      string
	Dir         string
	Tag         string
	Digest      string // manifest set digest
	GeneratedAt time.Time
	MaxAgeDays  int
	Manifest    *Manifest
}

// Message is the inconclusive reason text.
func (o Outcome) Message() string {
	if o.Usable {
		return ""
	}
	return o.Reason + ": " + o.Detail
}

// Audit is what AUR-521's audit record must register for this run.
func (o Outcome) Audit() map[string]string {
	if !o.Usable {
		return map[string]string{"analysis_data": "unusable", "reason": o.Message()}
	}
	return map[string]string{
		"analysis_data":              "used",
		"analysis_data_digest":       o.Digest,
		"analysis_data_generated_at": o.GeneratedAt.UTC().Format(time.RFC3339),
		"analysis_data_tag":          o.Tag,
	}
}

type release struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	CreatedAt  string `json:"created_at"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Resolve finds the newest published artifact, verifies it and applies the
// age limit. It never returns an error: every failure is an unusable Outcome,
// so a caller cannot treat "could not check" as "approved".
func Resolve(ctx context.Context, o Options) Outcome {
	out := Outcome{MaxAgeDays: o.MaxAgeDays}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	if o.MaxAgeDays <= 0 {
		out.Reason, out.Detail = ReasonInvalid, fmt.Sprintf("max_age_days %d must be positive", o.MaxAgeDays)
		return out
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Minute}
	}
	cache := o.CacheDir
	if cache == "" {
		d, err := os.MkdirTemp("", "aurum-analysis-data-")
		if err != nil {
			out.Reason, out.Detail = ReasonUnavailable, err.Error()
			return out
		}
		cache = d
	}

	rel, relErr := newestRelease(ctx, hc, o)
	if relErr != nil {
		// Offline fallback: a previously verified copy is still subject to
		// the same age limit below. No copy, no data: inconclusive.
		if dir, m := newestCached(cache); m != nil {
			return judge(out, dir, m, "", o.MaxAgeDays, now(), nil)
		}
		out.Reason, out.Detail = ReasonUnavailable, relErr.Error()
		return out
	}
	dir := filepath.Join(cache, strings.ReplaceAll(rel.TagName, "/", "_"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		out.Reason, out.Detail = ReasonUnavailable, err.Error()
		return out
	}
	assets := map[string]string{}
	for _, a := range rel.Assets {
		assets[a.Name] = a.URL
	}
	mu, ok := assets[ManifestName]
	if !ok {
		out.Reason, out.Detail = ReasonInvalid, "release "+rel.TagName+" has no "+ManifestName
		return out
	}
	raw, err := getBytes(ctx, hc, mu, "")
	if err != nil {
		out.Reason, out.Detail = ReasonUnavailable, err.Error()
		return out
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return fail(out, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), raw, 0o644); err != nil {
		out.Reason, out.Detail = ReasonUnavailable, err.Error()
		return out
	}
	fetch := func(f File) error {
		if err := VerifyFile(dir, f); err == nil {
			return nil
		}
		u, ok := assets[f.Path]
		if !ok {
			return fmt.Errorf("release %s lacks asset %s", rel.TagName, f.Path)
		}
		if err := downloadTo(ctx, hc, u, filepath.Join(dir, f.Path)); err != nil {
			return err
		}
		return VerifyFile(dir, f)
	}
	return judge(out, dir, m, rel.TagName, o.MaxAgeDays, now(), selected(m, o.Files, o.Kinds, fetch))
}

func selected(m *Manifest, only, kinds []string, fetch func(File) error) func() error {
	return func() error {
		want := map[string]bool{}
		for _, p := range only {
			want[p] = true
		}
		for _, f := range m.Files {
			if len(want) > 0 && !want[f.Path] {
				continue
			}
			if len(kinds) > 0 && !slices.Contains(kinds, f.Kind) {
				continue
			}
			if err := fetch(f); err != nil {
				return err
			}
		}
		return nil
	}
}

// judge applies the age limit BEFORE any payload download, then fetches and
// verifies the payload. fetchAll nil means "verify what is in dir".
func judge(out Outcome, dir string, m *Manifest, tag string, maxAgeDays int, now time.Time, fetchAll func() error) Outcome {
	gen, err := m.GeneratedTime()
	if err != nil {
		return fail(out, err)
	}
	out.Manifest, out.Digest, out.GeneratedAt, out.Dir, out.Tag = m, m.SetDigest, gen, dir, tag
	if out.Tag == "" {
		out.Tag, _ = m.Tag()
	}
	age := now.Sub(gen)
	if age < -maxClockSkew {
		out.Reason, out.Detail = ReasonInvalid, fmt.Sprintf("generated_at %s is in the future", m.GeneratedAt)
		return out
	}
	maxAge := time.Duration(maxAgeDays) * 24 * time.Hour
	if age > maxAge { // AUR-533 AC-003: the age limit
		out.Reason = ReasonStale
		out.Detail = fmt.Sprintf("artifact %s generated %s is %.1f days old, above max_age_days=%d", out.Tag, m.GeneratedAt, age.Hours()/24, maxAgeDays)
		return out
	}
	if fetchAll != nil {
		if err := fetchAll(); err != nil {
			return fail(out, err)
		}
	} else {
		for _, f := range m.Files {
			if err := VerifyFile(dir, f); err != nil {
				return fail(out, err)
			}
		}
	}
	out.Usable = true
	return out
}

func fail(out Outcome, err error) Outcome {
	out.Usable = false
	if errors.Is(err, ErrDigestMismatch) {
		out.Reason = ReasonDigestMismatch
	} else {
		out.Reason = ReasonInvalid
	}
	out.Detail = err.Error()
	return out
}

func newestRelease(ctx context.Context, hc *http.Client, o Options) (*release, error) {
	api := strings.TrimRight(o.APIBase, "/")
	if api == "" {
		api = DefaultAPIBase
	}
	raw, err := getBytes(ctx, hc, api+"/repos/"+o.Repository+"/releases?per_page=100", o.Token)
	if err != nil {
		return nil, err
	}
	var rels []release
	if err := json.Unmarshal(raw, &rels); err != nil {
		return nil, fmt.Errorf("release listing: %w", err)
	}
	var best *release
	var bestT time.Time
	for i := range rels {
		r := &rels[i]
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, TagPrefix) {
			continue
		}
		t, err := time.Parse(time.RFC3339, r.CreatedAt)
		if err != nil {
			continue
		}
		if best == nil || t.After(bestT) {
			best, bestT = r, t
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no published %s* release in %s", TagPrefix, o.Repository)
	}
	return best, nil
}

func newestCached(cache string) (string, *Manifest) {
	entries, _ := os.ReadDir(cache)
	var bestDir string
	var best *Manifest
	var bestT time.Time
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(cache, e.Name(), ManifestName))
		if err != nil {
			continue
		}
		m, err := ParseManifest(raw)
		if err != nil {
			continue
		}
		t, err := m.GeneratedTime()
		if err != nil {
			continue
		}
		if best == nil || t.After(bestT) {
			best, bestT, bestDir = m, t, filepath.Join(cache, e.Name())
		}
	}
	return bestDir, best
}

func getBytes(ctx context.Context, hc *http.Client, u, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func downloadTo(ctx context.Context, hc *http.Client, u, dst string) error {
	return download(ctx, hc, u, dst)
}
