package welcome

import (
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
// v1.0.1. Any of those is the published contract, not a moving target.
var pinnedTagPattern = regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)

// fullSHAPattern matches a full 40-character commit SHA -- the immutable pin
// the example workflows already offer a stricter consumer policy as an
// alternative to the tag.
var fullSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// SanitizeActionRef rewrites every Mpaape/AurumCode@<ref> in content so the
// ref is either the published major tag or a full commit SHA. Anything else
// -- @main, @master, or any other branch name an LLM might echo back from a
// consumer's own README's own Quick Start -- is a mutable ref: a later
// upstream push would change what executes in a copied workflow without the
// consumer ever editing anything (AUR-465, AC-001). It is normalized to the
// published tag. Returns the possibly-rewritten content and whether a
// rewrite happened.
func SanitizeActionRef(content string) (string, bool) {
	changed := false

	fixed := actionRefPattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := actionRefPattern.FindStringSubmatch(match)
		ref := sub[1]

		if pinnedTagPattern.MatchString(ref) || fullSHAPattern.MatchString(ref) {
			return match
		}

		changed = true
		return "Mpaape/AurumCode@" + publishedActionTag
	})

	return fixed, changed
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
