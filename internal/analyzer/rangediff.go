package analyzer

import (
	"errors"
	"fmt"
)

// ErrRangeDiffUnavailable is returned by RangeDiff when no git binary is
// available; RangeDiffFromObjects reads the same range from the object
// database instead.
var ErrRangeDiffUnavailable = errors.New("a pull request range diff needs a git binary")

// RangeDiff returns the unified diff text of baseRef...headRef: the changes
// of headRef since its merge base with baseRef, the same range and the same
// windowed hunks a forge renders for a pull request. It is read-only and
// never runs repository-configured helpers: external diff drivers,
// textconv filters and the fsmonitor hook are switched off.
func (r *Repo) RangeDiff(baseRef, headRef string) (string, error) {
	if !r.useGitBinary {
		return "", ErrRangeDiffUnavailable
	}
	baseSHA, err := r.ResolveRef(baseRef)
	if err != nil {
		return "", fmt.Errorf("resolving base ref %q: %w", baseRef, err)
	}
	headSHA, err := r.ResolveRef(headRef)
	if err != nil {
		return "", fmt.Errorf("resolving head ref %q: %w", headRef, err)
	}
	return r.git("-c", "core.fsmonitor=false", "-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", baseSHA+"..."+headSHA)
}
