// The two sides of the files the changelog check reads, rebuilt from the
// full-context diff of base..head (analyzer.BuildDiffFile emits one hunk
// with every line of both sides). The configuration is always the BASE
// side: a pull request cannot switch off the check it is subject to.
package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// diffSide is one file of the diff split into its two sides.
type diffSide struct {
	found    bool
	unusable bool
	old      string
	new      string
}

// fileSides finds name in the diff. A file in the notices (binary, too
// large) or with a windowed diff cannot be rebuilt and is unusable.
func fileSides(name string, diff *types.Diff, notices []analyzer.DiffNotice) diffSide {
	want := path.Clean(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	for _, n := range notices {
		if path.Clean(n.Path) == want {
			return diffSide{found: true, unusable: true}
		}
	}
	for _, f := range diff.Files {
		if path.Clean(f.Path) != want {
			continue
		}
		if len(f.Hunks) != 1 || f.Hunks[0].OldStart > 1 || f.Hunks[0].NewStart > 1 {
			return diffSide{found: true, unusable: true}
		}
		var oldLines, newLines []string
		for _, l := range f.Hunks[0].Lines {
			switch {
			case strings.HasPrefix(l, " "):
				oldLines = append(oldLines, l[1:])
				newLines = append(newLines, l[1:])
			case strings.HasPrefix(l, "-"):
				oldLines = append(oldLines, l[1:])
			case strings.HasPrefix(l, "+"):
				newLines = append(newLines, l[1:])
			default:
				return diffSide{found: true, unusable: true}
			}
		}
		return diffSide{found: true, old: strings.Join(oldLines, "\n"), new: strings.Join(newLines, "\n")}
	}
	return diffSide{}
}

// baseConfig is the configuration of the base commit. When the pull
// request changed config.yml, its old side is the base; otherwise the file
// is identical on both sides and is read from the checkout.
func baseConfig(repoRoot string, diff *types.Diff, notices []analyzer.DiffNotice) (*config.Config, error) {
	side := fileSides(config.DefaultConfigPath, diff, notices)
	switch {
	case side.unusable:
		return nil, fmt.Errorf("%s mudou e o lado da base nao pode ser lido", config.DefaultConfigPath)
	case side.found:
		return config.Parse([]byte(side.old), config.DefaultConfigPath+" (base)")
	}
	return config.LoadPath(filepath.Join(repoRoot, config.DefaultConfigPath))
}

// changelogRequirement overlays the declared section on the embedded
// defaults; agent-log markers declared by the repository add to the
// defaults, never replace them.
func changelogRequirement(c *config.ChangelogCheckConfig) (changelog.Requirement, error) {
	req, err := changelog.DefaultRequirement()
	if err != nil {
		return changelog.Requirement{}, err
	}
	if s := strings.TrimSpace(c.File); s != "" {
		req.File = s
	}
	if s := strings.TrimSpace(c.Section); s != "" {
		req.Section = s
	}
	for _, o := range []struct {
		dst *int
		v   int
	}{{&req.MaxEntryLines, c.MaxEntryLines}, {&req.MaxLineLength, c.MaxLineLength}, {&req.MaxReleaseLines, c.MaxReleaseLines}, {&req.MinWords, c.MinWords}} {
		if o.v > 0 {
			*o.dst = o.v
		}
	}
	req.AgentLogMarkers = append(req.AgentLogMarkers, c.AgentLogMarkers...)
	return req, req.Validate()
}

// evaluateChangelog returns the verdict and whether the base requires one.
func evaluateChangelog(repoRoot string, diff *types.Diff, notices []analyzer.DiffNotice) (changelog.Verdict, bool, error) {
	cfg, err := baseConfig(repoRoot, diff, notices)
	if err != nil {
		return changelog.Verdict{}, false, err
	}
	if !cfg.ChangelogCheck.Required() {
		return changelog.Verdict{}, false, nil
	}
	req, err := changelogRequirement(cfg.ChangelogCheck)
	if err != nil {
		return changelog.Verdict{}, true, err
	}
	side := fileSides(req.File, diff, notices)
	change := changelog.Change{State: changelog.FileUntouched}
	switch {
	case side.unusable:
		change.State = changelog.FileUnreadable
	case side.found:
		change = changelog.Change{State: changelog.FileChanged, Old: side.old, New: side.new}
	}
	return req.Verify(change), true, nil
}
