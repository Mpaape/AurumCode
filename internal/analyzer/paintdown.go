package analyzer

import (
	"container/heap"
	"fmt"
)

// This file is git's own merge-base walk ("paint down to common", see
// commit-reach.c): instead of collecting every ancestor of both tips, it
// walks both at once, newest committer date first, painting each commit
// with the side it was reached from, and stops as soon as nothing left to
// visit is painted by one side only. A pull request three commits ahead of
// a base a few commits ahead reads a few dozen commits, not the whole
// history, and a shallow clone works whenever the common ancestor is in it.

// Paint flags: reached from the first tip, from the others, from a common
// ancestor (an ancestor of a common ancestor can no longer be a best one),
// and already recorded as a candidate.
const (
	paintOne uint8 = 1 << iota
	paintTwo
	paintStale
	paintResult
)

// commitNode is what the walk needs of a commit: its parents (none for a
// root or a shallow boundary) and its committer time.
type commitNode struct {
	parents []string
	when    int64
}

// nodeReader reads one commit for the walk.
type nodeReader func(sha string) (commitNode, error)

// paintWalk caches each commit read once across the walks of one merge
// base computation; flags are per walk.
type paintWalk struct {
	read  nodeReader
	nodes map[string]commitNode
}

func newPaintWalk(read nodeReader) *paintWalk {
	return &paintWalk{read: read, nodes: map[string]commitNode{}}
}

func (w *paintWalk) node(sha string) (commitNode, error) {
	if n, ok := w.nodes[sha]; ok {
		return n, nil
	}
	n, err := w.read(sha)
	if err != nil {
		return commitNode{}, fmt.Errorf("reading commit %s: %w", sha, err)
	}
	w.nodes[sha] = n
	return n, nil
}

// mergeBase returns the single best common ancestor of a and b: none is an
// error, several (criss-cross history) is errAmbiguousMergeBase.
func (w *paintWalk) mergeBase(a, b string) (string, error) {
	candidates, _, err := w.paintDown(a, []string{b})
	if err != nil {
		return "", err
	}
	if len(candidates) > 1 {
		if candidates, err = w.removeRedundant(candidates); err != nil {
			return "", err
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("%s and %s: %w", a, b, errNoCommonAncestor)
	case 1:
		return candidates[0], nil
	}
	return "", &ambiguousMergeBase{a: a, b: b, candidates: candidates}
}

// ambiguousMergeBase is errAmbiguousMergeBase carrying the best common
// ancestors, newest first (the walk's order), so a caller that must pick
// one picks the one git would.
type ambiguousMergeBase struct {
	a, b       string
	candidates []string
}

func (e *ambiguousMergeBase) Error() string {
	return fmt.Sprintf("merge base of %s and %s: %v", e.a, e.b, errAmbiguousMergeBase)
}

func (e *ambiguousMergeBase) Is(target error) bool { return target == errAmbiguousMergeBase }

// paintDown paints one with paintOne and twos with paintTwo and returns the
// commits reached from both sides that are not ancestors of another such
// commit found by the walk, plus the flags it painted.
func (w *paintWalk) paintDown(one string, twos []string) ([]string, map[string]uint8, error) {
	flags := map[string]uint8{}
	q := &paintQueue{}
	push := func(sha string, f uint8) error {
		n, err := w.node(sha)
		if err != nil {
			return err
		}
		flags[sha] |= f
		q.push(sha, n.when)
		return nil
	}
	if err := push(one, paintOne); err != nil {
		return nil, nil, err
	}
	for _, two := range twos {
		if err := push(two, paintTwo); err != nil {
			return nil, nil, err
		}
	}
	var results []string
	for q.hasNonStale(flags) {
		sha := q.pop()
		f := flags[sha] & (paintOne | paintTwo | paintStale)
		if f == paintOne|paintTwo {
			if flags[sha]&paintResult == 0 {
				flags[sha] |= paintResult
				results = append(results, sha)
			}
			f |= paintStale
		}
		for _, p := range w.nodes[sha].parents {
			if flags[p]&f == f {
				continue
			}
			if err := push(p, f); err != nil {
				return nil, nil, err
			}
		}
	}
	var best []string
	for _, sha := range results {
		if flags[sha]&paintStale == 0 {
			best = append(best, sha)
		}
	}
	return best, flags, nil
}

// removeRedundant drops every candidate that is an ancestor of another, by
// painting each candidate against the rest (git's remove_redundant): the
// walk stops, as above, once nothing one-sided is left to visit.
func (w *paintWalk) removeRedundant(candidates []string) ([]string, error) {
	redundant := make([]bool, len(candidates))
	for i, c := range candidates {
		if redundant[i] {
			continue
		}
		var others []string
		var at []int
		for j, o := range candidates {
			if j != i && !redundant[j] {
				others = append(others, o)
				at = append(at, j)
			}
		}
		_, flags, err := w.paintDown(c, others)
		if err != nil {
			return nil, err
		}
		if flags[c]&paintTwo != 0 {
			redundant[i] = true
		}
		for k, o := range others {
			if flags[o]&paintOne != 0 {
				redundant[at[k]] = true
			}
		}
	}
	var kept []string
	for i, c := range candidates {
		if !redundant[i] {
			kept = append(kept, c)
		}
	}
	return kept, nil
}

// paintQueue orders the walk by committer time, newest first; equal times
// leave in the order they were queued.
type paintQueue struct {
	items []paintItem
	seq   int
}

type paintItem struct {
	sha  string
	when int64
	seq  int
}

func (q *paintQueue) Len() int { return len(q.items) }
func (q *paintQueue) Less(i, j int) bool {
	if q.items[i].when != q.items[j].when {
		return q.items[i].when > q.items[j].when
	}
	return q.items[i].seq < q.items[j].seq
}
func (q *paintQueue) Swap(i, j int) { q.items[i], q.items[j] = q.items[j], q.items[i] }
func (q *paintQueue) Push(x any)    { q.items = append(q.items, x.(paintItem)) }
func (q *paintQueue) Pop() any {
	last := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return last
}

func (q *paintQueue) push(sha string, when int64) {
	q.seq++
	heap.Push(q, paintItem{sha: sha, when: when, seq: q.seq})
}

func (q *paintQueue) pop() string { return heap.Pop(q).(paintItem).sha }

// hasNonStale reports whether anything left to visit can still change the
// answer: the walk ends when every queued commit is stale.
func (q *paintQueue) hasNonStale(flags map[string]uint8) bool {
	for _, it := range q.items {
		if flags[it.sha]&paintStale == 0 {
			return true
		}
	}
	return false
}
