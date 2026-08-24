package welcome

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// publishedActionTag is the major-version release tag the example workflows
// already converged on (AUR-440): `.github/workflows/examples/*.yml` all
// resolve `uses: Mpaape/AurumCode@v1` today.
const publishedActionTag = "v1"

// actionRefPattern matches "Mpaape/AurumCode@<ref>" the way it appears inside
// a `uses:` line of a GitHub Actions workflow snippet.
var actionRefPattern = regexp.MustCompile(`Mpaape/AurumCode@([A-Za-z0-9._/-]+)`)

// pinnedTagPattern matches a semantic-version-shaped release tag: v1, v1.0,
// v1.0.1. AUR-472: matching this SHAPE is no longer sufficient to accept a
// ref -- v2, v10, v1.99 all match it without a v2 tag ever having been
// published, the exact form-versus-value defect a 2026-08-14 adversarial
// review found here. This variable is kept declared, unused by
// SanitizeActionRef's own decision now, only because
// tests/e2e/AUR-465.sh's own MUT-001 mutation references it by name
// (`_ = pinnedTagPattern`); deleting it would turn that unrelated card's
// mutation proof into a build failure instead of the caught defect it
// expects.
var pinnedTagPattern = regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)

// fullSHAPattern matches a full 40-character commit SHA -- the immutable pin
// the example workflows already offer a stricter consumer policy as an
// alternative to the tag.
var fullSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// publishedTags is the declared, repository-local roster of tags AurumCode
// has actually released -- an exact set, not a shape (AUR-472). Consulting
// `git tag` would require the git binary, which the sealed acceptance image
// does not have and which this card's Non-goals forbid depending on;
// consulting the network would make a documentation generator
// non-deterministic. A value declared in this package is the
// "configuracao declarada" the card's Non-goals name as the alternative.
// Kept in sync by hand with the real release tags (AUR-440).
var publishedTags = []string{"v1", "v1.0.0", "v1.0.1"}

// TagSanitizeResult is SanitizeActionRefTags' outcome: the possibly
// rewritten content, whether anything changed, and one human-readable
// notice per rewrite explaining why it happened. AC-001 and AC-003 both
// require a rewrite to be announced, never silent.
type TagSanitizeResult struct {
	Content string
	Changed bool
	Notices []string
}

// SanitizeActionRef rewrites every Mpaape/AurumCode@<ref> in content so the
// ref is either a tag that actually exists in the published set or a full
// commit SHA. Anything else -- @main, @master, an unpublished v2, or any
// other ref an LLM might echo back from a consumer's own README's own Quick
// Start -- is not provably immutable, so it is normalized to the published
// major tag (AUR-465, AC-001; AUR-472 fixed what "actually exists" means).
// This is the back-compat two-value entry point AUR-465 already depends on;
// it always validates against publishedTags, the declared default. Callers
// that want an explicit tag list or the rewrite notices should call
// SanitizeActionRefTags directly.
func SanitizeActionRef(content string) (string, bool) {
	res := SanitizeActionRefTags(content, publishedTags)
	return res.Content, res.Changed
}

// PublishedTags returns the declared, repository-local set of tags this
// package treats as real. It comes from neither the network nor the git
// binary (AUR-472 Non-goals): it is a value declared in this source file.
func PublishedTags() []string {
	out := make([]string, len(publishedTags))
	copy(out, publishedTags)
	return out
}

// SanitizeActionRefTags is AUR-472's fix for the form-versus-value defect a
// 2026-08-14 adversarial review found in this file: the previous
// implementation accepted any ref shaped like a semver tag (v2, v10,
// v1.99) as though it were the real, published v1 -- the same
// validate-the-shape/conclude-the-value mistake that showed up twice the
// same day as strings.Contains gate checks (AUR-457, AUR-465). A ref now
// passes only when it is a full commit SHA (the strictest possible pin,
// always accepted regardless of tags) or an EXACT match against `tags`,
// the caller-supplied roster of tags that actually exist.
//
// An empty tags means "no list is available". Per the card's Non-goals,
// the safe behavior with no roster to check against is to rewrite every
// ref that is not a full SHA to the published major tag and say so --
// never to trust an unverifiable ref's shape, and never to fail the build
// over it.
func SanitizeActionRefTags(content string, tags []string) TagSanitizeResult {
	known := make(map[string]bool, len(tags))
	for _, t := range tags {
		known[t] = true
	}
	noList := len(tags) == 0

	changed := false
	var notices []string

	fixed := actionRefPattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := actionRefPattern.FindStringSubmatch(match)
		ref := sub[1]

		if fullSHAPattern.MatchString(ref) {
			return match
		}
		if !noList && known[ref] {
			return match
		}

		changed = true
		if noList {
			notices = append(notices, fmt.Sprintf(
				"no published-tag list was available to validate %q against; rewrote it to the major tag %q instead of trusting its shape -- an unverifiable ref is always rewritten, never accepted, when there is nothing to check it against",
				ref, publishedActionTag))
		} else {
			notices = append(notices, fmt.Sprintf(
				"%q has the shape of a release tag but no such tag exists in the published set %v; rewrote it to the major tag %q",
				ref, tags, publishedActionTag))
		}
		return "Mpaape/AurumCode@" + publishedActionTag
	})

	return TagSanitizeResult{Content: fixed, Changed: changed, Notices: notices}
}

// gettingStartedGuidePath is the one internal documentation link the welcome
// content is allowed to point to: the actual guide the card's Non-goals
// clause names, addressed the exact way the generated site links to it.
// Matching is EXACT EQUALITY against the normalized target
// (normalizeLinkTarget), never Contains/HasPrefix/HasSuffix: each of those
// lets a different disguised path escape sanitization -- Contains lets
// "evil/docs/getting-started.md/../../etc/passwd" and
// "notes/docs/getting-started.md-old.md" both survive untouched even though
// neither is a file the generator ever produced (AUR-465 adversarial
// review). AC-003 requires every internal link to point to a file the
// generator itself produced; only the literal guide path qualifies.
const gettingStartedGuidePath = "docs/getting-started.md"

// internalLinkPattern matches a markdown link: [label](target).
var internalLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

// normalizeLinkTarget strips the parts of a link target that do not change
// which file it names: a leading "./" and a trailing "#fragment" anchor.
// Nothing else is stripped -- in particular this performs no path cleaning
// (no ".." resolution), so a target that merely CONTAINS the guide path
// inside a longer, different path is never normalized into equalling it.
func normalizeLinkTarget(target string) string {
	target = strings.TrimPrefix(target, "./")
	if hash := strings.IndexByte(target, '#'); hash >= 0 {
		target = target[:hash]
	}
	return target
}

// SanitizeInternalLinks neutralizes any markdown link whose target is a
// relative, local path that is not the getting-started guide. AC-003
// requires every internal link on the generated index to point to a file
// the generator itself produced. Nothing in this package can confirm that an
// LLM-invented relative path (e.g. the prompt template's own example
// "Section 1"/"link/" placeholders, if echoed back literally) names a real
// sibling page the site scaffold is about to write, so an unrecognized
// relative link is turned into plain text instead of shipped as a dangling
// link. External URLs, mailto links, and in-page anchors are untouched: they
// are not the class of link AC-003 is about.
func SanitizeInternalLinks(content string) (string, bool) {
	changed := false

	fixed := internalLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := internalLinkPattern.FindStringSubmatch(match)
		label, target := groups[1], groups[2]

		if isExternalOrAnchor(target) || normalizeLinkTarget(target) == gettingStartedGuidePath {
			return match
		}

		changed = true
		return label
	})

	return fixed, changed
}

func isExternalOrAnchor(target string) bool {
	switch {
	case strings.HasPrefix(target, "http://"),
		strings.HasPrefix(target, "https://"),
		strings.HasPrefix(target, "mailto:"),
		strings.HasPrefix(target, "#"):
		return true
	default:
		return false
	}
}

// logoKeyPattern matches a top-level "logo:" key in a Jekyll _config.yml,
// with or without surrounding quotes.
var logoKeyPattern = regexp.MustCompile(`(?m)^logo:\s*"?([^"\n]+?)"?\s*$`)

// DeclaredAssetPath extracts the value of a top-level "logo:" key from a
// Jekyll _config.yml, if one is declared. It returns ("", false) when no
// logo key is present: an undeclared asset is compliant with AC-002 by
// definition (the rule is "every declared asset path exists, or it is not
// declared"); only a declared-but-missing asset is the defect this reports.
func DeclaredAssetPath(configYML string) (string, bool) {
	m := logoKeyPattern.FindStringSubmatch(configYML)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// AssetExists resolves a declared asset path (as _config.yml would write it,
// e.g. "/assets/images/logo.png") against siteRoot -- the directory the
// generated site is published from -- and reports whether the file is
// actually there. AC-002's rule is "every declared asset path exists, or it
// is not declared": a config that declares a logo the generator never wrote
// renders a broken image on the one page everyone reads first.
func AssetExists(declaredPath, siteRoot string) bool {
	rel := strings.TrimPrefix(declaredPath, "/")
	info, err := os.Stat(filepath.Join(siteRoot, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}
