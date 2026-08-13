package analyzer

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// This file guards the memory bound of the line differ.
//
// Before the fix, myersDiff kept, for every edit distance d, a snapshot of
// the whole furthest-reaching-x vector -- width 2*(N+M)+1 -- and every one
// of those snapshots stayed alive until the function returned. That is
// O(D*(N+M)) simultaneously-live memory, not garbage the collector could
// reclaim, so neither GC nor GOMEMLIMIT could help. Measured cost of a
// single added file: 4.18 MB at 500 lines, 16.58 at 1000, 65.89 at 2000,
// 262.90 at 4000, 1050.31 at 8000 -- quadrupling at every doubling. One
// real 12674-line added file in this repository's own history needed
// ~2.6 GB, and a ~22k-line file exhausted a 31 GB machine outright.
//
// WHY TotalAlloc AND NOT testing.AllocsPerRun
//
// AllocsPerRun counts allocations, not bytes, and re-runs the function
// several times -- on the pre-fix code that meant multiplying a gigabyte by
// the run count. The cumulative TotalAlloc delta is the right instrument
// here: the old trace was never freed before the return, so cumulative
// allocation equalled the simultaneously-live set. For the fixed code the
// metric is merely conservative -- it also counts transient garbage -- so a
// passing assertion is a strictly stronger claim than "the live set is
// small". It is deterministic and does not depend on GC timing.

// measureDiffAlloc reports the bytes diffLines cumulatively allocates for
// one call.
func measureDiffAlloc(tb testing.TB, oldLines, newLines []string) uint64 {
	tb.Helper()

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	out := diffLines(oldLines, newLines)

	runtime.ReadMemStats(&after)
	runtime.KeepAlive(out)
	return after.TotalAlloc - before.TotalAlloc
}

func mib(b uint64) string { return fmt.Sprintf("%.2f MiB", float64(b)/(1<<20)) }

// genAdded builds the "whole file is new" shape, which is what the
// measurements quoted above used.
func genAdded(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line %d of a newly added file", i)
	}
	return out
}

// genAlternating builds the *general* edit shape: both sides the same
// length, every other line different, so there is no long common prefix or
// suffix and the edit distance stays proportional to the file size. A
// one-sided fast path (pure insert / pure delete) would leave this case on
// the quadratic path while making genAdded look fixed, so this is the
// load-bearing measurement, not genAdded.
func genAlternating(n int) (oldLines, newLines []string) {
	oldLines = make([]string, n)
	newLines = make([]string, n)
	for i := 0; i < n; i++ {
		oldLines[i] = fmt.Sprintf("line %d common text", i)
		if i%2 == 0 {
			newLines[i] = oldLines[i]
		} else {
			newLines[i] = fmt.Sprintf("line %d changed text", i)
		}
	}
	return oldLines, newLines
}

// allocCeiling is the per-call budget every shape must stay under. The
// bounded trace can hold at most maxTraceCells ints (32 MiB) before
// diffLines abandons the minimal-script search; add the rendered output and
// the input slices and 64 MiB is a comfortable ceiling that still fails
// loudly on any return of the quadratic shape, which cost 1-3 GB here.
const allocCeiling = 64 << 20

// The two sizes below bracket what the input cap admits: 8000 lines is the
// largest size the pre-fix measurements covered, and 10000 lines is exactly
// maxDiffLines, i.e. the worst case the cap still lets through.

func TestDiffLines_AllocBound_AddedFile8k(t *testing.T) {
	got := measureDiffAlloc(t, nil, genAdded(8000))
	t.Logf("added 8000 lines: %s", mib(got))
	if got > allocCeiling {
		t.Errorf("diffLines allocated %s for an 8000-line added file, over the %s budget: the quadratic trace is back",
			mib(got), mib(allocCeiling))
	}
}

func TestDiffLines_AllocBound_GeneralEdit8k(t *testing.T) {
	oldLines, newLines := genAlternating(8000)
	got := measureDiffAlloc(t, oldLines, newLines)
	t.Logf("8000 vs 8000 lines, every other line changed: %s", mib(got))
	if got > allocCeiling {
		t.Errorf("diffLines allocated %s for an 8000-line two-sided edit, over the %s budget: the quadratic trace is back",
			mib(got), mib(allocCeiling))
	}
}

func TestDiffLines_AllocBound_AtInputCap(t *testing.T) {
	oldLines, newLines := genAlternating(maxDiffLines)
	got := measureDiffAlloc(t, oldLines, newLines)
	t.Logf("%d vs %d lines (the input cap), every other line changed: %s", maxDiffLines, maxDiffLines, mib(got))
	if got > allocCeiling {
		t.Errorf("diffLines allocated %s at the input cap, over the %s budget", mib(got), mib(allocCeiling))
	}
}

// TestDiffLines_CheapEditInLargeFile pins the property the bounded trace
// exists to preserve: capping the *trace* rather than pre-rejecting on
// worst-case size means a one-line change inside a large file is still
// diffed exactly and still costs almost nothing, because its edit distance
// is 2 regardless of how long the file is.
func TestDiffLines_CheapEditInLargeFile(t *testing.T) {
	oldLines := genAdded(maxDiffLines)
	newLines := append([]string{}, oldLines...)
	newLines[5000] = "the one line that actually changed"

	got := measureDiffAlloc(t, oldLines, newLines)
	t.Logf("one changed line in a %d-line file: %s", maxDiffLines, mib(got))
	if got > 16<<20 {
		t.Errorf("a single-line edit in a large file allocated %s; it should be nearly free", mib(got))
	}

	rendered := diffLines(oldLines, newLines)
	if len(rendered) != maxDiffLines+1 {
		t.Fatalf("expected %d rendered lines, got %d", maxDiffLines+1, len(rendered))
	}
	joined := strings.Join(rendered, "\n")
	if !strings.Contains(joined, "+the one line that actually changed") {
		t.Error("the changed line was not rendered as an insertion")
	}
}
