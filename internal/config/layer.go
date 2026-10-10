// The configuration layers of a review, declared once: the central security
// policy (AUR-518), the engineering standards layer (AUR-613) and the
// reviewed repository. Each layer is a directory that HOLDS a .aurumcode/
// (config.yml, skills/<name>/SKILL.md, profiles.yml, prompt and docs); the
// two central ones are checked out by the workflow, never fetched by this
// program and never inside the reviewed tree. What differs between them --
// precedence, what config.yml may declare, who may contribute analysts, how
// a malformed skill is treated -- is data on Layer, so the code that loads,
// orders and names layers is written once.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// LayerName identifies one configuration layer.
type LayerName string

const (
	// LayerPolicy is the central security policy: it takes authority over
	// the repository's sections (governance.go) and fails closed.
	LayerPolicy LayerName = "policy"
	// LayerStandards is the engineering standards layer: a second central
	// directory that only adds skills and analysts.
	LayerStandards LayerName = "standards"
	// LayerRepository is the reviewed repository's own .aurumcode/.
	LayerRepository LayerName = "repository"
)

// LayerOrder is the precedence of the layers, highest first: when two
// layers declare a skill or an analyst of the same name, the earlier layer
// wins and the later one is ignored with a warning.
var LayerOrder = []LayerName{LayerPolicy, LayerStandards, LayerRepository}

// SectionPolicy is what a layer's config.yml may declare.
type SectionPolicy int

const (
	// AnySection: every section. The policy declares them with authority
	// (ApplyCentralPolicy); the repository declares them under the policy.
	AnySection SectionPolicy = iota
	// NoSection: config.yml must be absent or empty. The layer contributes
	// files only, never gate, ignore, exceptions, rules, scanners or
	// publication; a section is refused, never silently ignored.
	NoSection
)

// Layer describes one configuration layer.
type Layer struct {
	Name LayerName
	// Central is a layer checked out by the workflow, outside the reviewed
	// tree: its errors fail the run before any model call, and a malformed
	// SKILL.md is an error, not a warning.
	Central bool
	// Analysts is whether the layer's .aurumcode/profiles.yml joins the
	// team the repository's review.profiles selects from.
	Analysts bool
	// Sections is what the layer's config.yml may declare.
	Sections SectionPolicy
	// ErrorLanguage pins the language of the layer's own errors; empty
	// follows the review's language. The policy's errors predate the
	// catalog (AUR-518) and are recorded by tutorials and tests in English,
	// so they stay English whatever the review language.
	ErrorLanguage string
}

// Layers declares every layer once.
var Layers = map[LayerName]Layer{
	LayerPolicy:     {Name: LayerPolicy, Central: true, Analysts: false, Sections: AnySection, ErrorLanguage: "en"},
	LayerStandards:  {Name: LayerStandards, Central: true, Analysts: true, Sections: NoSection},
	LayerRepository: {Name: LayerRepository, Central: false, Analysts: true, Sections: AnySection},
}

// Label is the layer's name for people, in language ("security policy",
// "engineering standards", "repository").
func (l Layer) Label(language string) string {
	return i18n.Text(language, "layer."+string(l.Name))
}

// Prefix is how the layer's own errors start ("central policy",
// "standards layer"), in the layer's error language.
func (l Layer) Prefix(language string) string {
	return i18n.Text(l.ErrorLang(language), "layer."+string(l.Name)+".prefix")
}

// ErrorLang is the language of the layer's own messages: ErrorLanguage
// when pinned, else the review's language.
func (l Layer) ErrorLang(language string) string {
	if l.ErrorLanguage != "" {
		return l.ErrorLanguage
	}
	return language
}

// noun is how the layer is called after "a" in a sentence ("policy",
// "standards layer"), in language.
func (l Layer) noun(language string) string {
	return i18n.Text(l.ErrorLang(language), "layer."+string(l.Name)+".noun")
}

// Precedes reports whether l wins over other on a name collision.
func (l Layer) Precedes(other Layer) bool {
	return layerRank(l.Name) < layerRank(other.Name)
}

// layerRank is the layer's position in LayerOrder; an unknown name ranks
// last.
func layerRank(name LayerName) int {
	for i, n := range LayerOrder {
		if n == name {
			return i
		}
	}
	return len(LayerOrder)
}

// ValidateLayerOutsideReviewedTree refuses a central layer's directory that
// resolves (after symlinks) to the reviewed tree itself or anywhere under
// it: the repository under review must never supply the policy or the
// standards it is judged against, because whoever controls the reviewed
// checkout (a pull request's own head, in the --pr path) could edit them.
// Either path missing is not this function's problem to report -- a
// missing dir surfaces as the layer loader's own "not found" error, and
// reviewedRoot always exists (the process's working directory) -- so a
// resolution failure falls back to the plain absolute path. The message is
// in language.
func ValidateLayerOutsideReviewedTree(layer Layer, dir, reviewedRoot, language string) error {
	dir = strings.TrimSpace(dir)
	reviewedRoot = strings.TrimSpace(reviewedRoot)
	if !insideReviewedTree(dir, reviewedRoot) {
		return nil
	}
	language = layer.ErrorLang(language)
	return errors.New(i18n.Format(language, "layer.inside_reviewed_tree", layer.Prefix(language), dir, reviewedRoot, layer.noun(language)))
}

// insideReviewedTree reports whether dir resolves (after symlinks) to the
// reviewed tree itself or anywhere under it. An empty dir or root is never
// inside: there is nothing declared to contain.
func insideReviewedTree(dir, reviewedRoot string) bool {
	if dir == "" || reviewedRoot == "" {
		return false
	}
	resolvedDir := resolvePathBestEffort(dir)
	resolvedRoot := resolvePathBestEffort(reviewedRoot)
	return resolvedDir == resolvedRoot || strings.HasPrefix(resolvedDir, resolvedRoot+string(filepath.Separator))
}

// resolvePathBestEffort returns dir's absolute, symlink-resolved form. A dir
// that does not exist yet (EvalSymlinks fails) or an unresolvable absolute
// path falls back to the plain absolute form -- there is nothing further to
// resolve, and the caller that actually needs the directory to exist still
// reports that failure on its own.
func resolvePathBestEffort(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// LayerDir is one declared layer on disk: its directory and the digest of
// what it contributes, for the audit record.
type LayerDir struct {
	Layer Layer
	// Dir is the directory that holds the layer's .aurumcode/.
	Dir string
	// Digest identifies the layer's contributing files (layerDigest).
	Digest string
}

// LoadLayerDir reads root/.aurumcode as layer. A declared layer must be
// there: a root without .aurumcode/ is an error naming it, never an empty
// layer. Its config.yml is checked against layer.Sections
// (validateLayerConfig). Messages are in language. The central policy's
// own config is decoded by LoadCentralPolicy; this is the file-level
// identity every central layer shares.
func LoadLayerDir(layer Layer, root, language string) (*LayerDir, error) {
	language = layer.ErrorLang(language)
	aurum := filepath.Join(root, filepath.Dir(DefaultConfigPath))
	if info, err := os.Stat(aurum); err != nil || !info.IsDir() {
		return nil, errors.New(i18n.Format(language, "layer.not_found", layer.Prefix(language), aurum))
	}
	if err := validateLayerConfig(layer, filepath.Join(root, DefaultConfigPath), language); err != nil {
		return nil, err
	}
	digest, err := layerDigest(root)
	if err != nil {
		return nil, errors.New(i18n.Format(language, "layer.load_failed", layer.Prefix(language), err.Error()))
	}
	return &LayerDir{Layer: layer, Dir: root, Digest: digest}, nil
}
