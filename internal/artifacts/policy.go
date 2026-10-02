package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultMaxAgeDays: the workflow publishes daily; a week tolerates a
	// few failed runs and a long weekend without letting vulnerability data
	// rot for longer than that.
	DefaultMaxAgeDays = 7
	// MaxAgeDaysCeiling stops a typo from silently disabling the limit.
	MaxAgeDaysCeiling = 365
	// DefaultRepository publishes the artifact (public GitHub releases).
	DefaultRepository = "Mpaape/AurumCode"
	// ConfigPath mirrors internal/config's DefaultConfigPath, relative to a
	// repository or to a central policy directory.
	ConfigPath = ".aurumcode/config.yml"
)

// Policy is the effective `analysis_data` section.
type Policy struct {
	MaxAgeDays int
	Repository string
	// Warnings names repository declarations the central policy overrode.
	Warnings []string
}

type section struct {
	MaxAgeDays *int   `yaml:"max_age_days"`
	Repository string `yaml:"repository"`
}

type file struct {
	AnalysisData *section `yaml:"analysis_data"`
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

func readSection(p string) (*section, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return f.AnalysisData, nil
}

// LoadPolicy resolves the effective section. Like quality_gates, the section
// is governed independently: when the central policy (policyDir, possibly
// empty) declares `analysis_data` it decides alone and the repository's own
// declaration is dropped with a warning; otherwise the repository's applies;
// otherwise the defaults.
func LoadPolicy(repoRoot, policyDir string) (Policy, error) {
	pol := Policy{MaxAgeDays: DefaultMaxAgeDays, Repository: DefaultRepository}
	repoSec, err := readSection(filepath.Join(repoRoot, ConfigPath))
	if err != nil {
		return pol, err
	}
	chosen := repoSec
	if policyDir != "" {
		cs, err := readSection(filepath.Join(policyDir, ConfigPath))
		if err != nil {
			return pol, err
		}
		if cs != nil {
			if repoSec != nil {
				pol.Warnings = append(pol.Warnings, "analysis_data do config do repositório foi ignorado: a política central decide sozinha")
			}
			chosen = cs
		}
	}
	if chosen == nil {
		return pol, nil
	}
	if chosen.MaxAgeDays != nil {
		if *chosen.MaxAgeDays <= 0 || *chosen.MaxAgeDays > MaxAgeDaysCeiling {
			return pol, fmt.Errorf("analysis_data.max_age_days: %d out of range (1..%d)", *chosen.MaxAgeDays, MaxAgeDaysCeiling)
		}
		pol.MaxAgeDays = *chosen.MaxAgeDays
	}
	if chosen.Repository != "" {
		if !repoRe.MatchString(chosen.Repository) {
			return pol, fmt.Errorf("analysis_data.repository: %q is not owner/name", chosen.Repository)
		}
		pol.Repository = chosen.Repository
	}
	return pol, nil
}
