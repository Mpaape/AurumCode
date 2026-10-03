package dtrack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// AUR-570: bom/token's processing:false only says the BOM was ingested.
// Dependency-Track's policy evaluation and metrics recalculation finish
// afterwards, so the first read of metrics/project/{id}/current can still
// be the previous revision's numbers (or zero violations before the
// policy ran). A gate that grades that read approves what the server's
// own policy rejects. SettledMetrics returns metrics only once they are
// provably post-upload (lastOccurrence later than the upload instant)
// and stable (two consecutive fresh readings of critical, high and
// policyViolationsTotal coincide).

// ErrMetricsUnsettled means the metrics never became fresh and stable
// before the timeout; the gate treats it as inconclusive, never approved.
var ErrMetricsUnsettled = errors.New("dtrack: project metrics did not settle before the configured timeout")

// Note tokens appended to Outcome.Notes.
const NoteRefreshUnavailable = "dtrack_refresh_unavailable"

// refreshMetrics asks the server to recalculate the project's metrics
// (GET /api/v1/metrics/project/{id}/refresh). The endpoint needs a
// permission the gate's key may lack (403), or may not exist on a
// differently versioned server; both are tolerated and recorded, since
// the reading loop below is the actual guarantee.
func (c *Client) refreshMetrics(ctx context.Context, project string) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/metrics/project/"+url.PathEscape(project)+"/refresh", nil)
	if err != nil {
		return fmt.Errorf("dtrack: refresh: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: refresh", ErrUnreachable)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StatusError{Op: "refresh", Status: resp.StatusCode}
	}
	return nil
}

func freshSince(m ProjectMetrics, uploadedAt time.Time) bool {
	return m.LastOccurrence != nil && *m.LastOccurrence > uploadedAt.UnixMilli()
}

// projectAnalysis is the one field read from GET /api/v1/project/{id}.
type projectAnalysis struct {
	LastVulnerabilityAnalysis *int64 `json:"lastVulnerabilityAnalysis"`
}

// analysedSince reports whether the server's own analysis of the project
// finished after the upload. It is the second freshness proof: measured
// on Dependency-Track 5.1.1, metrics.lastOccurrence only moves when the
// recalculated metrics DIFFER from the stored ones, so re-uploading an
// unchanged SBOM leaves it frozen even though the server did process it.
// Any failure to read it (403, 404, malformed) means "not proven".
func (c *Client) analysedSince(ctx context.Context, project string, uploadedAt time.Time) bool {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/project/"+url.PathEscape(project), nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return false
	}
	var out projectAnalysis
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return false
	}
	return out.LastVulnerabilityAnalysis != nil && *out.LastVulnerabilityAnalysis > uploadedAt.UnixMilli()
}

func sameReading(a, b ProjectMetrics) bool {
	eq := func(x, y *int) bool { return x != nil && y != nil && *x == *y }
	return eq(a.Critical, b.Critical) && eq(a.High, b.High) && eq(a.PolicyViolationsTotal, b.PolicyViolationsTotal)
}

// A reading counts as fresh when metrics.lastOccurrence is later than the
// upload, or (unchanged metrics never move it) the project's own
// lastVulnerabilityAnalysis is. Either way two consecutive readings must
// coincide, so a policy evaluation landing between reads resets the count.
//
// SettledMetrics refreshes (when permitted) and rereads the project's
// metrics every interval until two consecutive fresh readings
// coincide, or timeout elapses (ErrMetricsUnsettled). A fresh reading
// that lacks a required count is returned at once so EvaluateMetrics can
// refuse it as incomplete. A 404 (no metrics computed yet) keeps waiting.
// A refresh refused by the server is appended to notes, never fatal.
func (c *Client) SettledMetrics(ctx context.Context, project string, uploadedAt time.Time, interval, timeout time.Duration, notes *[]string) (ProjectMetrics, error) {
	if err := c.refreshMetrics(ctx, project); err != nil {
		var se *StatusError
		if !errors.As(err, &se) {
			return ProjectMetrics{}, err
		}
		*notes = append(*notes, NoteRefreshUnavailable)
	}
	deadline := c.now().Add(timeout)
	var prev *ProjectMetrics
	for {
		m, err := c.ProjectMetrics(ctx, project)
		var se *StatusError
		switch {
		case errors.As(err, &se) && se.Status == http.StatusNotFound:
			prev = nil
		case err != nil:
			return ProjectMetrics{}, err
		case !freshSince(m, uploadedAt) && !c.analysedSince(ctx, project, uploadedAt):
			prev = nil
		case m.Critical == nil || m.High == nil || m.PolicyViolationsTotal == nil:
			return m, nil
		case prev != nil && sameReading(*prev, m):
			return m, nil
		default:
			cur := m
			prev = &cur
		}
		if !c.now().Before(deadline) {
			return ProjectMetrics{}, ErrMetricsUnsettled
		}
		c.sleep(interval)
	}
}
