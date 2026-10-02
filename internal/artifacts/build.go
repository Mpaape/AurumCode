package artifacts

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultOSVBase is the public, official OSV bucket. Its ecosystems.txt
// lists every ecosystem the project currently publishes.
const DefaultOSVBase = "https://osv-vulnerabilities.storage.googleapis.com"

// BuildOptions configures one artifact build.
type BuildOptions struct {
	OSVBase      string // public source base URL; DefaultOSVBase when empty
	ScannersFile string // path of the scanner pin file (scanners.yml); optional
	OutDir       string // directory to write; created if missing
	Now          func() time.Time
	HTTP         *http.Client
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Build downloads every ecosystem the source lists, validates each payload
// is a readable zip, copies the scanner pins and writes manifest.json. The
// ecosystem list is read from the source on every build; nothing here names
// one. A single failing ecosystem fails the whole build: no partial artifact.
func Build(ctx context.Context, o BuildOptions) (*Manifest, error) {
	base := strings.TrimRight(o.OSVBase, "/")
	if base == "" {
		base = DefaultOSVBase
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Minute}
	}
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return nil, err
	}

	listing, err := fetchBytes(ctx, hc, base+"/ecosystems.txt")
	if err != nil {
		return nil, fmt.Errorf("listing ecosystems: %w", err)
	}
	var ecosystems []string
	sc := bufio.NewScanner(strings.NewReader(string(listing)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ecosystems = append(ecosystems, line)
	}
	if len(ecosystems) == 0 {
		return nil, fmt.Errorf("source %s lists no ecosystems; refusing to publish an empty artifact", base)
	}
	sort.Strings(ecosystems)

	m := &Manifest{
		Schema:      SchemaV1,
		GeneratedAt: now().UTC().Format(time.RFC3339),
		Sources:     []Source{{Name: "osv", URL: base}},
		Scanners:    map[string]string{},
	}
	used := map[string]bool{}
	for _, eco := range ecosystems {
		name := "osv-" + unsafeChars.ReplaceAllString(eco, "_") + ".zip"
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("osv-%s-%d.zip", unsafeChars.ReplaceAllString(eco, "_"), i)
		}
		used[name] = true
		dst := filepath.Join(o.OutDir, name)
		if err := download(ctx, hc, base+"/"+url.PathEscape(eco)+"/all.zip", dst); err != nil {
			return nil, fmt.Errorf("ecosystem %q: %w", eco, err)
		}
		zr, err := zip.OpenReader(dst)
		if err != nil {
			return nil, fmt.Errorf("ecosystem %q: payload is not a readable zip: %w", eco, err)
		}
		zr.Close()
		d, n, err := HashFile(dst)
		if err != nil {
			return nil, err
		}
		m.Files = append(m.Files, File{Path: name, SHA256: d, Size: n, Kind: "osv", Ecosystem: eco})
	}

	if o.ScannersFile != "" {
		raw, err := os.ReadFile(o.ScannersFile)
		if err != nil {
			return nil, fmt.Errorf("scanner pins: %w", err)
		}
		flat := map[string]any{}
		if err := yaml.Unmarshal(raw, &flat); err != nil {
			return nil, fmt.Errorf("scanner pins: %w", err)
		}
		for k, v := range flat {
			if s, ok := v.(string); ok {
				m.Scanners[k] = s
			}
		}
		dst := filepath.Join(o.OutDir, "scanners.yml")
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return nil, err
		}
		d, n, err := HashFile(dst)
		if err != nil {
			return nil, err
		}
		m.Files = append(m.Files, File{Path: "scanners.yml", SHA256: d, Size: n, Kind: "scanners"})
	}

	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	m.SetDigest = ComputeSetDigest(m.Files)
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(o.OutDir, ManifestName), append(out, '\n'), 0o644); err != nil {
		return nil, err
	}
	return m, nil
}

func fetchBytes(ctx context.Context, hc *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func download(ctx context.Context, hc *http.Client, u, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
