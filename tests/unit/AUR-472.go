// Package unit holds AUR-472's Unit-layer proof: an action ref is validated
// against the tags that actually exist, never merely against the SHAPE of a
// semver tag.
//
// THE DEFECT (2026-08-14 adversarial review of AUR-465)
//
//	internal/documentation/welcome/sanitize.go used to define
//	pinnedTagPattern = ^v[0-9]+(\.[0-9]+){0,2}$ and treated ANY ref matching
//	that shape as already-compliant: v2, v10, v1.99 all passed untouched
//	even though only v1, v1.0.0 and v1.0.1 have ever been published. The
//	same validate-the-shape/conclude-the-value mistake showed up twice the
//	same day as strings.Contains gate checks (AUR-457, AUR-465's own
//	SanitizeInternalLinks review).
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office: tests/acceptance/AUR-472.sh stages a private copy of
// the module and bridges TestAUR472 into a real `go test`.
package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
)

// TestAUR472 is AUR-472's Unit-layer selector.
func TestAUR472(t *testing.T) {
	declared := []string{"v1", "v1.0.0", "v1.0.1"}

	// -- AC-001: a ref shaped like a semver tag but never published is
	// rewritten, and the rewrite is announced.
	shapedButUnpublished := []string{"v2", "v10", "v1.99"}
	for _, ref := range shapedButUnpublished {
		t.Run("AC-001/unpublished-shaped-ref/"+ref, func(t *testing.T) {
			in := "uses: Mpaape/AurumCode@" + ref
			res := welcome.SanitizeActionRefTags(in, declared)
			if !res.Changed {
				t.Fatalf("AUR-472/AC-001/behavior-missing: %q has semver SHAPE but no such tag was ever published, yet SanitizeActionRefTags reported no change (this is the exact 2026-08-14 defect)", ref)
			}
			if !strings.Contains(res.Content, "Mpaape/AurumCode@v1") || strings.Contains(res.Content, "@"+ref) {
				t.Fatalf("AUR-472/AC-001/behavior-missing: %q was not rewritten to the published major tag, got %q", ref, res.Content)
			}
			if len(res.Notices) == 0 {
				t.Fatalf("AUR-472/AC-001/behavior-missing: rewriting %q produced no notice; the rewrite must be announced, not silent", ref)
			}
			if !strings.Contains(res.Notices[0], ref) {
				t.Fatalf("AUR-472/AC-001/behavior-missing: notice %q does not name the rejected ref %q", res.Notices[0], ref)
			}
		})
	}

	// -- AC-002: a ref that IS a real, published tag passes intocada.
	for _, ref := range declared {
		t.Run("AC-002/real-tag-survives/"+ref, func(t *testing.T) {
			in := "uses: Mpaape/AurumCode@" + ref
			res := welcome.SanitizeActionRefTags(in, declared)
			if res.Changed {
				t.Fatalf("AUR-472/AC-002/behavior-missing: %q is a real published tag but was rewritten anyway: %q", ref, res.Content)
			}
			if len(res.Notices) != 0 {
				t.Fatalf("AUR-472/AC-002/behavior-missing: a real tag %q produced a notice, expected none: %v", ref, res.Notices)
			}
			if !strings.Contains(res.Content, "Mpaape/AurumCode@"+ref) {
				t.Fatalf("AUR-472/AC-002/behavior-missing: output lost the untouched ref %q: %q", ref, res.Content)
			}
		})
	}

	// -- AC-002: a full commit SHA remains the strictest possible pin,
	// accepted regardless of the declared tag list.
	sha := "11bd71901bbe5b1630ceea73d27597364c9af683"
	t.Run("AC-002/full-sha-survives", func(t *testing.T) {
		in := "uses: Mpaape/AurumCode@" + sha
		res := welcome.SanitizeActionRefTags(in, declared)
		if res.Changed {
			t.Fatalf("AUR-472/AC-002/behavior-missing: a full commit SHA must never be rewritten, got %q", res.Content)
		}
		resNoList := welcome.SanitizeActionRefTags(in, nil)
		if resNoList.Changed {
			t.Fatalf("AUR-472/AC-002/behavior-missing: a full commit SHA must survive even with no tag list available, got %q", resNoList.Content)
		}
	})

	// -- AC-003: with no tag list available, every non-SHA ref is rewritten
	// -- even one that would have been a real, published tag had a list
	// been available -- and the notice says validation happened without a
	// list.
	t.Run("AC-003/no-list-rewrites-everything", func(t *testing.T) {
		in := "uses: Mpaape/AurumCode@v1"
		res := welcome.SanitizeActionRefTags(in, nil)
		if !res.Changed {
			t.Fatalf("AUR-472/AC-003/behavior-missing: with no tag list, even a ref that would be a real tag must be rewritten (safe default), got unchanged %q", res.Content)
		}
		if len(res.Notices) == 0 {
			t.Fatalf("AUR-472/AC-003/behavior-missing: no-list rewrite produced no notice")
		}
		if !strings.Contains(res.Notices[0], "no published-tag list") {
			t.Fatalf("AUR-472/AC-003/behavior-missing: notice %q never says validation happened without a list", res.Notices[0])
		}
	})

	t.Run("AC-003/no-list-still-accepts-full-sha", func(t *testing.T) {
		in := "uses: Mpaape/AurumCode@" + sha
		res := welcome.SanitizeActionRefTags(in, nil)
		if res.Changed {
			t.Fatalf("AUR-472/non-goal/behavior-missing: a full SHA must be accepted even with no tag list (Non-goals: immutable pin never rejected), got %q", res.Content)
		}
	})

	// -- Back-compat: the two-value SanitizeActionRef entry point (still
	// depended on by AUR-465) must behave identically to
	// SanitizeActionRefTags(content, welcome.PublishedTags()).
	t.Run("back-compat/SanitizeActionRef-matches-declared-default", func(t *testing.T) {
		in := "uses: Mpaape/AurumCode@v2"
		out, changed := welcome.SanitizeActionRef(in)
		if !changed || strings.Contains(out, "@v2") {
			t.Fatalf("AUR-472/AC-001/behavior-missing: back-compat SanitizeActionRef did not reject the unpublished v2, got %q", out)
		}
		declaredDefault := welcome.PublishedTags()
		if len(declaredDefault) == 0 {
			t.Fatalf("AUR-472/AC-001/behavior-missing: PublishedTags() returned an empty declared default in normal operation")
		}
	})
}
