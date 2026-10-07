package dependencies

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Query is one question to the advisory source: a package at a version, or
// the whole package when Version is empty (a declared range).
type Query struct {
	Ecosystem string
	Name      string
	Version   string
}

// Source answers which advisories affect a package version. An error is
// never "no advisories".
type Source interface {
	Query(ctx context.Context, q Query) ([]Vulnerability, error)
}

// ErrStale marks an answer older than the configured limit.
var ErrStale = errors.New("dependencies: advisory source answer is older than the limit")

// maxOSVBody bounds one answer; a larger one is not trustworthy.
const maxOSVBody = 16 << 20

// maxOSVPages bounds the paging of one query.
const maxOSVPages = 20

// OSV is the OSV API client (POST /v1/query). MaxAge, when positive, bounds
// how old an answer may be by its Date and Age headers; an answer without a
// Date header is then stale too.
type OSV struct {
	BaseURL string
	Client  *http.Client
	MaxAge  time.Duration
	Now     func() time.Time
}

type osvQuery struct {
	Package   osvPackage `json:"package"`
	Version   string     `json:"version,omitempty"`
	PageToken string     `json:"page_token,omitempty"`
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvAnswer struct {
	Vulns         []osvVuln `json:"vulns"`
	NextPageToken string    `json:"next_page_token"`
}

type osvVuln struct {
	ID               string          `json:"id"`
	Aliases          []string        `json:"aliases"`
	Summary          string          `json:"summary"`
	DatabaseSpecific osvSpecific     `json:"database_specific"`
	Affected         []osvAffected   `json:"affected"`
	References       []osvReference  `json:"references"`
	Severity         []osvSeverity   `json:"severity"`
	Withdrawn        string          `json:"withdrawn"`
	Raw              json.RawMessage `json:"-"`
}

type osvSpecific struct {
	Severity string `json:"severity"`
}

type osvAffected struct {
	Package           osvPackage  `json:"package"`
	Ranges            []osvRange  `json:"ranges"`
	DatabaseSpecific  osvSpecific `json:"database_specific"`
	EcosystemSpecific osvSpecific `json:"ecosystem_specific"`
}

type osvRange struct {
	Events []map[string]string `json:"events"`
}

type osvReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

// Query asks the source about q, following its pages.
func (o OSV) Query(ctx context.Context, q Query) ([]Vulnerability, error) {
	var out []Vulnerability
	token := ""
	for page := 0; page < maxOSVPages; page++ {
		answer, err := o.page(ctx, osvQuery{Package: osvPackage{Name: q.Name, Ecosystem: q.Ecosystem}, Version: q.Version, PageToken: token})
		if err != nil {
			return nil, err
		}
		for _, v := range answer.Vulns {
			if strings.TrimSpace(v.Withdrawn) != "" {
				continue
			}
			out = append(out, o.convert(v, q))
		}
		if token = answer.NextPageToken; token == "" {
			return out, nil
		}
	}
	return nil, fmt.Errorf("dependencies: OSV answer for %s/%s exceeds %d pages", q.Ecosystem, q.Name, maxOSVPages)
}

func (o OSV) page(ctx context.Context, body osvQuery) (osvAnswer, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return osvAnswer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/v1/query", bytes.NewReader(payload))
	if err != nil {
		return osvAnswer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client().Do(req)
	if err != nil {
		return osvAnswer{}, fmt.Errorf("dependencies: OSV unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return osvAnswer{}, fmt.Errorf("dependencies: OSV answered HTTP %d", resp.StatusCode)
	}
	if err := o.fresh(resp.Header); err != nil {
		return osvAnswer{}, err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOSVBody+1))
	if err != nil {
		return osvAnswer{}, fmt.Errorf("dependencies: reading OSV answer: %w", err)
	}
	if len(data) > maxOSVBody {
		return osvAnswer{}, fmt.Errorf("dependencies: OSV answer exceeds %d bytes", maxOSVBody)
	}
	var answer osvAnswer
	if err := json.Unmarshal(data, &answer); err != nil {
		return osvAnswer{}, fmt.Errorf("dependencies: OSV answer is not JSON: %w", err)
	}
	return answer, nil
}

// fresh refuses an answer older than MaxAge: Date plus Age, the way an HTTP
// cache in between states it.
func (o OSV) fresh(h http.Header) error {
	if o.MaxAge <= 0 {
		return nil
	}
	date, err := http.ParseTime(h.Get("Date"))
	if err != nil {
		return fmt.Errorf("%w: no readable Date header", ErrStale)
	}
	age := time.Duration(0)
	if raw := strings.TrimSpace(h.Get("Age")); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs < 0 {
			return fmt.Errorf("%w: unreadable Age header", ErrStale)
		}
		age = time.Duration(secs) * time.Second
	}
	if o.now().Sub(date)+age > o.MaxAge {
		return fmt.Errorf("%w: answer dated %s", ErrStale, date.UTC().Format(time.RFC3339))
	}
	return nil
}

func (o OSV) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (o OSV) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// convert keeps what the report needs: identifiers, the source's severity,
// the fixed versions of the queried package and the advisory link.
func (o OSV) convert(v osvVuln, q Query) Vulnerability {
	out := Vulnerability{ID: v.ID, Aliases: v.Aliases, Summary: v.Summary, Severity: NormalizeSeverity(v.DatabaseSpecific.Severity)}
	for _, a := range v.Affected {
		if !strings.EqualFold(a.Package.Name, q.Name) {
			continue
		}
		if out.Severity == SeverityUnknown {
			out.Severity = NormalizeSeverity(firstNonEmpty(a.DatabaseSpecific.Severity, a.EcosystemSpecific.Severity))
		}
		for _, r := range a.Ranges {
			for _, ev := range r.Events {
				if fixed := ev["fixed"]; fixed != "" {
					out.Fixed = appendUnique(out.Fixed, fixed)
				}
			}
		}
	}
	out.Link = advisoryLink(v.References, strings.TrimRight(o.BaseURL, "/")+"/v1/vulns/"+v.ID)
	return out
}

// advisoryLink prefers the advisory's own ADVISORY reference, then any web
// reference, then the source's record.
func advisoryLink(refs []osvReference, fallback string) string {
	for _, want := range []string{"ADVISORY", "WEB"} {
		for _, r := range refs {
			if strings.EqualFold(r.Type, want) && strings.HasPrefix(r.URL, "https://") {
				return r.URL
			}
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
