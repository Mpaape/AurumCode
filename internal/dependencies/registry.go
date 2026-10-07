package dependencies

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Metadata is a package's live registry metadata, flattened to named
// fields ("published_at", "links.SOURCE_REPO", ...): the only facts a
// suspicion may rest on.
type Metadata map[string]string

// Registry answers live facts about a package version. An error is never
// "nothing suspicious".
type Registry interface {
	Metadata(ctx context.Context, c Change) (Metadata, error)
	// Licenses is the version's registered license expressions.
	Licenses(ctx context.Context, c Change) ([]string, error)
}

// ErrNoSystem marks a package the model could not place in a registry
// system: the registry cannot be asked about it.
var ErrNoSystem = fmt.Errorf("dependencies: no registry system for the package")

// maxRegistryBody bounds one registry answer.
const maxRegistryBody = 4 << 20

// DepsDev is the deps.dev v3 API: the package (its versions and their
// publication dates) and the version (links, licenses, deprecation).
type DepsDev struct {
	BaseURL string
	Client  *http.Client
}

type depsDevPackage struct {
	Versions []struct {
		VersionKey  struct{ Version string } `json:"versionKey"`
		PublishedAt string                    `json:"publishedAt"`
	} `json:"versions"`
}

type depsDevVersion struct {
	PublishedAt  string   `json:"publishedAt"`
	IsDeprecated bool     `json:"isDeprecated"`
	Licenses     []string `json:"licenses"`
	Links        []struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"links"`
}

// Metadata flattens the package and version answers.
func (d DepsDev) Metadata(ctx context.Context, c Change) (Metadata, error) {
	pkgURL, err := d.packageURL(c)
	if err != nil {
		return nil, err
	}
	var pkg depsDevPackage
	if err := d.get(ctx, pkgURL, &pkg); err != nil {
		return nil, err
	}
	ver, err := d.version(ctx, pkgURL, c)
	if err != nil {
		return nil, err
	}
	meta := Metadata{
		"version":        c.Head,
		"published_at":   ver.PublishedAt,
		"versions_count": strconv.Itoa(len(pkg.Versions)),
		"is_deprecated":  strconv.FormatBool(ver.IsDeprecated),
		"licenses":       strings.Join(ver.Licenses, " AND "),
	}
	var dates []string
	for _, v := range pkg.Versions {
		if v.PublishedAt != "" {
			dates = append(dates, v.PublishedAt)
		}
	}
	sort.Strings(dates)
	if len(dates) > 0 {
		meta["first_published_at"] = dates[0]
	}
	meta["links.SOURCE_REPO"] = ""
	for _, l := range ver.Links {
		meta["links."+l.Label] = l.URL
	}
	return meta, nil
}

// Licenses is the version's license list as deps.dev holds it (SPDX
// expressions, or "non-standard").
func (d DepsDev) Licenses(ctx context.Context, c Change) ([]string, error) {
	pkgURL, err := d.packageURL(c)
	if err != nil {
		return nil, err
	}
	ver, err := d.version(ctx, pkgURL, c)
	if err != nil {
		return nil, err
	}
	return ver.Licenses, nil
}

func (d DepsDev) version(ctx context.Context, pkgURL string, c Change) (depsDevVersion, error) {
	var ver depsDevVersion
	err := d.get(ctx, pkgURL+"/versions/"+url.PathEscape(c.Head), &ver)
	return ver, err
}

func (d DepsDev) packageURL(c Change) (string, error) {
	if strings.TrimSpace(c.LicenseSystem) == "" {
		return "", ErrNoSystem
	}
	return strings.TrimRight(d.BaseURL, "/") + "/v3/systems/" + url.PathEscape(strings.ToLower(c.LicenseSystem)) + "/packages/" + url.PathEscape(c.Name), nil
}

func (d DepsDev) get(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dependencies: registry unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dependencies: registry answered HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryBody+1))
	if err != nil || len(data) > maxRegistryBody {
		return fmt.Errorf("dependencies: registry answer unreadable or above %d bytes", maxRegistryBody)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("dependencies: registry answer is not JSON: %w", err)
	}
	return nil
}
