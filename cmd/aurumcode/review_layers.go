// A central configuration layer at the command seam. internal/config
// declares the layers (config.Layers, config.LayerOrder) and loads a layer's
// directory; this file resolves where the engineering standards layer is
// (--padroes, AURUMCODE_STANDARDS), loads it through that one loader and
// hands each part to the mechanism that already exists for it: skills to the
// catalog, analysts to the team, identity to the audit record. The central
// policy keeps its own config loader (config.LoadCentralPolicy) because its
// config.yml is decoded with authority; everything else here is written for
// any central layer.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

// layerLocalSource is the audit source of a layer given on the command line
// rather than checked out by the reusable workflow.
const layerLocalSource = "local"

// centralLayer is one central layer a review applies, loaded.
type centralLayer struct {
	*config.LayerDir
	// source and ref name where the workflow checked the layer out from.
	source, ref string
	// team is the layer's analysts (.aurumcode/profiles.yml) when the
	// layer contributes them; the zero-config team otherwise.
	team *reviewprofile.Team
}

// resolveStandardsDir picks the standards directory: --padroes/--standards,
// else AURUMCODE_STANDARDS, else "" (no layer). An explicitly empty value is
// a usage error, never a silent fallback -- the rule resolvePolicyDir
// applies to --politica/--policy.
func (f *reviewFlags) resolveStandardsDir(stderr io.Writer) (dir string, exit int, ok bool) {
	dir = strings.TrimSpace(f.padroes)
	if dir == "" {
		dir = strings.TrimSpace(f.standards)
	}
	given := f.given["padroes"] || f.given["standards"]
	if given && dir == "" {
		fmt.Fprintln(stderr, "aurumcode review: --padroes/--standards: directory must not be empty")
		return "", 2, false
	}
	if !given {
		dir = strings.TrimSpace(os.Getenv("AURUMCODE_STANDARDS"))
	}
	return dir, 0, true
}

// resolveCentralLayers loads the repository's config at cwd and the two
// central layers over it, in the order the review's resolve steps take
// (loadConfig, then loadStandards): the policy validated outside the tree,
// loaded and folded over the config; the effective review language; then
// the engineering standards layer. The agent server's rules listing reaches
// the same skill layers the review session resolves through it; the
// session keeps the steps apart only to print each warning in place.
func resolveCentralLayers(cwd, policyDir, standardsDir string, env reviewEnv) (skillLayers, error) {
	cfg, err := config.Load(cwd)
	if err != nil {
		return skillLayers{}, err
	}
	l := skillLayers{cwd: cwd, policyDir: policyDir, cfg: cfg}
	if policyDir != "" {
		if err := config.ValidatePolicyOutsideReviewedTree(policyDir, cwd); err != nil {
			return skillLayers{}, err
		}
		if l.centralCfg, err = config.LoadCentralPolicy(policyDir); err != nil {
			return skillLayers{}, err
		}
		l.cfg, _ = config.ApplyCentralPolicy(cfg, l.centralCfg)
	}
	if standardsDir == "" {
		return l, nil
	}
	if l.language, err = l.cfg.ReviewLanguage(); err != nil {
		return skillLayers{}, err
	}
	if l.standards, err = loadCentralLayer(config.Layers[config.LayerStandards], standardsDir, cwd, l.language, env); err != nil {
		return skillLayers{}, err
	}
	return l, nil
}

// loadCentralLayer validates and loads layer at dir for the tree at
// reviewedRoot: outside that tree, with .aurumcode/, a config.yml within
// what the layer may declare and, when the layer contributes analysts, a
// well-formed profiles.yml. dir == "" is no layer (nil, nil). Every failure
// is an error in the layer's language, before any model call.
func loadCentralLayer(layer config.Layer, dir, reviewedRoot, language string, env reviewEnv) (*centralLayer, error) {
	if dir == "" {
		return nil, nil
	}
	if err := config.ValidateLayerOutsideReviewedTree(layer, dir, reviewedRoot, language); err != nil {
		return nil, err
	}
	ld, err := config.LoadLayerDir(layer, dir, language)
	if err != nil {
		return nil, err
	}
	team := reviewprofile.EmptyTeam()
	if layer.Analysts {
		if team, err = reviewprofile.LoadTeamFile(dir); err != nil {
			return nil, layerLoadError(layer, language, err)
		}
	}
	source := strings.TrimSpace(env.standardsRepository)
	if source == "" {
		source = layerLocalSource
	}
	return &centralLayer{LayerDir: ld, source: source, ref: strings.TrimSpace(env.standardsRef), team: team}, nil
}

// layerLoadError names the layer in front of a loading error, in the
// layer's language.
func layerLoadError(layer config.Layer, language string, err error) error {
	language = layer.ErrorLang(language)
	return errors.New(i18n.Format(language, "layer.load_failed", layer.Prefix(language), err.Error()))
}

// loadStandards is the resolve step both review sources share: it loads
// the declared engineering standards layer for the tree at reviewedRoot,
// after the review language is known. A broken layer stops the run (exit 1).
func (s *reviewState) loadStandards(reviewedRoot string) (int, bool) {
	layer, err := loadCentralLayer(config.Layers[config.LayerStandards], s.standardsDir, reviewedRoot, s.reviewLanguage, s.env())
	if err != nil {
		fmt.Fprintf(s.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	s.standards = layer
	return 0, false
}

// skillSource is the layer's skill directories, labelled with the layer's
// name ("<layer>/.aurumcode/skills/<name>", see layerOfDoc); nil without a
// layer.
func (l *centralLayer) skillSource() skills.Source {
	if l == nil {
		return nil
	}
	return localSkillSource(l.Dir, string(l.Layer.Name))
}

// teamOver puts the layer's analysts over the repository's team and warns,
// in language, about every repository analyst the layer's replaced, naming
// both layers. Without a layer, repo is returned unchanged.
func (l *centralLayer) teamOver(repo *reviewprofile.Team, language string, stderr io.Writer) *reviewprofile.Team {
	if l == nil {
		return repo
	}
	team, shadowed := reviewprofile.LayerTeams(l.team, repo)
	lower, upper := config.Layers[config.LayerRepository].Of(language), l.Layer.In(language)
	for _, name := range shadowed {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", i18n.Format(language, "layer.profile_shadowed", name, lower, upper))
	}
	return team
}

// audit is the audit record's entry for the layer, nil without a layer.
func (l *centralLayer) audit() *render.AuditStandards {
	if l == nil {
		return nil
	}
	return &render.AuditStandards{Source: l.source, Ref: l.ref, Digest: l.Digest}
}

// activeDir is the layer's directory, "" without a layer.
func (l *centralLayer) activeDir() string {
	if l == nil {
		return ""
	}
	return l.Dir
}
