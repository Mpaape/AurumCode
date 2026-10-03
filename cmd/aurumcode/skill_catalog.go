package main

import (
	"fmt"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// resolveSkillCatalog is the single place the review (--base and --pr) turns
// the skill directories into the catalog sent to the model: the central
// policy's skills layered over the repository's, selected by language and
// path from the changed files.
//
// A policy that cannot be loaded is an error (the policy is the
// organization's, so it fails closed before any model call). A repository
// whose skills cannot be loaded is only declared: the review continues
// without them and the warning reaches the model and the review text.
// A nil source means that layer is absent.
func resolveSkillCatalog(policy, repo skills.Source) (*skills.Catalog, error) {
	var policySet, repoSet *skills.Set
	var warnings []string
	if policy != nil {
		set, err := skills.LoadSource(policy)
		if err != nil {
			return nil, fmt.Errorf("central policy: %w", err)
		}
		policySet = set
	}
	if repo != nil {
		set, err := skills.LoadSource(repo)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("repository skills unavailable, review continues without them: %v", err))
		} else {
			repoSet = set
		}
	}
	return skills.NewCatalog(policySet, repoSet, nil, warnings), nil
}

// localSkillSource reads root/.aurumcode/skills from disk, shown with a
// repository-relative label.
func localSkillSource(root, label string) skills.Source {
	return skills.DirSource{
		Dir:    filepath.Join(root, filepath.FromSlash(skills.DefaultDirName)),
		Prefix: filepath.ToSlash(filepath.Join(label, skills.DefaultDirName)),
	}
}

// skillSelectionNotices returns the declared warnings of the selection for
// the changed paths, redacted, for the review text.
func skillSelectionNotices(catalog *skills.Catalog, changed []string, filter *redaction.Filter) []string {
	if catalog == nil {
		return nil
	}
	warnings := catalog.Select(changed).Warnings
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		if filter != nil {
			w = filter.Redact(w)
		}
		out = append(out, w)
	}
	return out
}
