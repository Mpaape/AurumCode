package gitleaks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// verifyShallowRange accepts a shallow clone only when it holds the whole
// range: no commit of `git rev-list <head> --not <base>` is a shallow
// boundary (listed in the shallow file). A boundary inside the range means
// git log would stop there and gitleaks would scan part of the pull request
// as if it were all of it.
func verifyShallowRange(ctx context.Context, run scanner.Command, root string, r scanner.Range) error {
	boundary, err := shallowBoundary(ctx, run, root)
	if err != nil {
		return err
	}
	stdout, stderr, err := run(ctx, root, gitBinary, "rev-list", r.Head, "--not", r.Base)
	if err != nil {
		return fmt.Errorf("%w: git rev-list %s --not %s: %v%s", ErrBadRange, r.Head, r.Base, err, quotedStderr(stderr))
	}
	for _, sha := range strings.Fields(stdout) {
		if boundary[strings.ToLower(sha)] {
			return fmt.Errorf("%w: the shallow clone cuts the pull request range at commit %s: fetch more history", ErrBadRange, sha)
		}
	}
	return nil
}

// shallowBoundary reads the shallow file git keeps in the common git dir
// (a linked worktree included). A shallow repository without one cannot be
// checked, so it is refused rather than read as complete.
func shallowBoundary(ctx context.Context, run scanner.Command, root string) (map[string]bool, error) {
	stdout, stderr, err := run(ctx, root, gitBinary, "rev-parse", "--git-path", "shallow")
	if err != nil {
		return nil, fmt.Errorf("%w: git rev-parse --git-path shallow: %v%s", ErrBadRange, err, quotedStderr(stderr))
	}
	path := strings.TrimSpace(stdout)
	if path == "" {
		return nil, fmt.Errorf("%w: shallow repository without a shallow file path", ErrBadRange)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: shallow repository, shallow file unreadable: %v", ErrBadRange, err)
	}
	set := map[string]bool{}
	for _, sha := range strings.Fields(string(raw)) {
		set[strings.ToLower(sha)] = true
	}
	return set, nil
}
