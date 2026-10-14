package tools

// max_read_bytes and max_cache_bytes are optional (issue #179): 0 sets no
// ceiling, so no result is refused for its size; a positive value is an
// explicit ceiling that refuses the result that would cross it, with the
// same message and reason as always (AUR-526, AC-004).

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

func TestByteBudgetWithoutCeilingRefusesNothing(t *testing.T) {
	for _, max := range []int{0, -1} {
		b := NewByteBudget(max)
		for i := 0; i < 3; i++ {
			// 10 MiB a charge, far above the former 256 KiB default.
			if err := b.Charge(10 << 20); err != nil {
				t.Fatalf("NewByteBudget(%d): charge %d refused: %v", max, i+1, err)
			}
		}
		if b.Exhausted() || b.Limit() != "" {
			t.Fatalf("NewByteBudget(%d) exhausted (limit %q) with no ceiling", max, b.Limit())
		}
	}
}

func TestByteBudgetWithCeilingKeepsItsMessageAndReason(t *testing.T) {
	b := NewByteBudget(100)
	if err := b.Charge(60); err != nil {
		t.Fatalf("a charge within the ceiling was refused: %v", err)
	}
	err := b.Charge(41)
	if err == nil || err.Error() != "teto de max_read_bytes atingido (60 de 100 bytes já devolvidos); a revisão fica parcial" {
		t.Fatalf("err = %v, want the unchanged ceiling message", err)
	}
	if !b.Exhausted() || b.Limit() != LimitMaxReadBytes || b.Detail() != "60 de 100 bytes devolvidos pelas ferramentas do repositório" {
		t.Fatalf("exhausted=%v limit=%q detail=%q", b.Exhausted(), b.Limit(), b.Detail())
	}
	if err := b.Charge(1); err == nil || err.Error() != "teto de max_read_bytes atingido; a revisão fica parcial" {
		t.Fatalf("an exhausted budget returned %v", err)
	}
}

// A read_file result of about 280 KB, above the former 256 KiB default, is
// returned when nothing declares a ceiling and refused, as the partial
// review max_read_bytes, when a ceiling of 256 KiB is declared.
func TestReadFileResultIsOnlyRefusedForSizeUnderAnExplicitCeiling(t *testing.T) {
	big := strings.Repeat(strings.Repeat("a", 1400)+"\n", 210)
	root, tracked := revisionFixture(t, map[string]string{"big.txt": big})

	rev := openRevision(t, root, tracked, RevisionOptions{})
	res, err := run(t, NewReadFileTool(rev, nil), `{"path":"big.txt"}`)
	if err != nil || len(res.Content) <= 256*1024 {
		t.Fatalf("no ceiling: err=%v bytes=%d, want the whole window returned", err, len(res.Content))
	}
	if rev.Budget().Exhausted() {
		t.Fatal("a budget without a ceiling was exhausted")
	}

	capped := openRevision(t, root, tracked, RevisionOptions{Budget: NewByteBudget(256 * 1024)})
	offers := RepositoryOffers(capped, nil, grammar.Default(), nil)
	if _, err := run(t, offers[0].Tool, `{"path":"big.txt"}`); err == nil || !strings.Contains(err.Error(), LimitMaxReadBytes) {
		t.Fatalf("explicit 256 KiB ceiling: err = %v, want max_read_bytes", err)
	}
	if limit := PartialLimit(offers)(); limit == nil || limit.Limit != LimitMaxReadBytes {
		t.Fatalf("partial = %+v, want max_read_bytes", limit)
	}
}

// The file cache has no memory ceiling unless one is declared: with 1 GiB
// already cached (far above the former 64 MiB default) a read is still
// served; an explicit ceiling refuses it, naming max_cache_bytes.
func TestCacheCeilingExistsOnlyWhenDeclared(t *testing.T) {
	root, tracked := revisionFixture(t, javaFixture(t))

	rev := openRevision(t, root, tracked, RevisionOptions{})
	rev.cached = 1 << 30
	offers := RepositoryOffers(rev, nil, grammar.Default(), nil)
	if _, err := run(t, offers[0].Tool, `{"path":"src/shop/Contract.java"}`); err != nil {
		t.Fatalf("no ceiling: a read was refused for memory: %v", err)
	}
	if limit := PartialLimit(offers)(); limit != nil {
		t.Fatalf("no ceiling: partial = %+v, want none", limit)
	}

	capped := openRevision(t, root, tracked, RevisionOptions{MaxCacheBytes: 1 << 20})
	capped.cached = 1 << 30
	cappedOffers := RepositoryOffers(capped, nil, grammar.Default(), nil)
	if _, err := run(t, cappedOffers[0].Tool, `{"path":"src/shop/Contract.java"}`); err == nil || !strings.Contains(err.Error(), LimitMaxCacheBytes) {
		t.Fatalf("explicit ceiling: err = %v, want max_cache_bytes", err)
	}
	if limit := PartialLimit(cappedOffers)(); limit == nil || limit.Limit != LimitMaxCacheBytes {
		t.Fatalf("explicit ceiling: partial = %+v, want max_cache_bytes", limit)
	}
}

// Without a declared ceiling the cache keeps nothing: every read is verified
// against the reviewed tree again, and memory stays flat however many files a
// search walks. With a ceiling the verified bytes are kept, as before.
func TestFileCacheKeepsNothingWithoutADeclaredCeiling(t *testing.T) {
	root, tracked := revisionFixture(t, javaFixture(t))

	rev := openRevision(t, root, tracked, RevisionOptions{})
	offers := RepositoryOffers(rev, nil, grammar.Default(), nil)
	for i := 0; i < 2; i++ {
		if _, err := run(t, offers[0].Tool, `{"path":"src/shop/Contract.java"}`); err != nil {
			t.Fatal(err)
		}
	}
	if len(rev.cache) != 0 || rev.cached != 0 {
		t.Fatalf("no ceiling: %d files (%d bytes) kept in memory, want none", len(rev.cache), rev.cached)
	}

	capped := openRevision(t, root, tracked, RevisionOptions{MaxCacheBytes: 1 << 30})
	cappedOffers := RepositoryOffers(capped, nil, grammar.Default(), nil)
	if _, err := run(t, cappedOffers[0].Tool, `{"path":"src/shop/Contract.java"}`); err != nil {
		t.Fatal(err)
	}
	if len(capped.cache) != 1 {
		t.Fatalf("explicit ceiling: %d files kept, want the one read", len(capped.cache))
	}
}

// Issue #179: a file past the old 1 MiB per-file read bound is read whole
// when nothing declares a ceiling; an explicit MaxFileBytes still refuses.
func TestFilePastTheOldReadBoundIsReadWhole(t *testing.T) {
	big := strings.Repeat(strings.Repeat("b", 1023)+"\n", 2048)
	root, tracked := revisionFixture(t, map[string]string{"big.txt": big})

	data, err := openRevision(t, root, tracked, RevisionOptions{}).Read("big.txt")
	if err != nil || len(data) != len(big) {
		t.Fatalf("no ceiling: err=%v bytes=%d, want %d", err, len(data), len(big))
	}
	if _, err := openRevision(t, root, tracked, RevisionOptions{MaxFileBytes: 1 << 20}).Read("big.txt"); err == nil {
		t.Fatal("an explicit MaxFileBytes of 1 MiB accepted a 2 MiB file")
	}
}
