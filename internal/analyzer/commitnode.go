package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// commitReader reads commits for the merge-base walk the way git itself
// sees them: a commit listed in the shallow file (the boundary of a
// shallow clone) has no parents, though its object still names them.
func (r *Repo) commitReader() (nodeReader, error) {
	shallow, err := r.shallowCommits()
	if err != nil {
		return nil, err
	}
	return func(sha string) (commitNode, error) {
		n, err := r.readCommitNode(sha)
		if err != nil {
			return commitNode{}, err
		}
		if shallow[sha] {
			n.parents = nil
		}
		return n, nil
	}, nil
}

// readCommitNode reads a commit's parents and committer time from one
// object read.
func (r *Repo) readCommitNode(sha string) (commitNode, error) {
	objType, content, err := r.readObject(sha)
	if err != nil {
		return commitNode{}, err
	}
	if objType != "commit" {
		return commitNode{}, fmt.Errorf("object %s is a %s, not a commit", sha, objType)
	}
	var n commitNode
	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			break // blank line separates headers from the commit message
		}
		switch {
		case strings.HasPrefix(line, "parent "):
			n.parents = append(n.parents, strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "parent "))))
		case strings.HasPrefix(line, "committer "):
			n.when = committerTime(line)
		}
	}
	return n, nil
}

// committerTime is the Unix time after the identity's closing ">" (names
// may hold spaces); an unreadable time is 0, which only orders the walk.
func committerTime(line string) int64 {
	rest := line[strings.LastIndex(line, ">")+1:]
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0
	}
	when, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return when
}

// shallowCommits returns the boundary commits of a shallow clone (none for
// a complete one), from the shallow file git keeps in the common git dir.
func (r *Repo) shallowCommits() (map[string]bool, error) {
	path := filepath.Join(r.gitDir, "shallow")
	if r.useGitBinary {
		out, err := r.git("rev-parse", "--git-path", "shallow")
		if err != nil {
			return nil, err
		}
		path = strings.TrimSpace(out)
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.workDir, path)
		}
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the shallow file: %w", err)
	}
	set := map[string]bool{}
	for _, line := range strings.Fields(string(raw)) {
		set[strings.ToLower(line)] = true
	}
	return set, nil
}
