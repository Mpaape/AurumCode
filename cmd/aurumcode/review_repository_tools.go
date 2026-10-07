// Repository tools of the deliberation: with deliberation active
// and a checkout proven to be the reviewed revision, the model may read
// files, search text, look a symbol up through the grammar provider and read
// the diff of another changed file. The tools read the reviewed commit's
// tree only (internal/review/tools.Revision) and share one byte ceiling,
// deliberation.max_read_bytes; hitting it makes the review partial, which
// the gate treats as any deliberation limit (inconclusive per policy).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/grammar"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
)

// repositoryOffers is the repository tools over the scan root, or none
// (with the reason on stderr) when the root is not a trusted checkout of
// the reviewed revision.
func (s *reviewState) repositoryOffers() []reviewtools.Offer {
	if s.scanRoot == "" || s.scanBlocked != "" {
		return nil
	}
	rev, err := s.reviewedRevision()
	if err != nil {
		fmt.Fprintf(s.stderr, "aurumcode review: deliberation: ferramentas do repositório não oferecidas: %s\n", s.redactText(err.Error()))
		return nil
	}
	return reviewtools.RepositoryOffers(rev, s.diff, grammar.Default(), s.redactText)
}

// reviewedRevision opens HEAD of the scan root, the commit the diff was
// computed against (--base: base..HEAD; --pr: the verified head).
func (s *reviewState) reviewedRevision() (*reviewtools.Revision, error) {
	if _, err := os.Stat(filepath.Join(s.scanRoot, ".git")); err != nil {
		return nil, fmt.Errorf("a raiz revisada não é a raiz de um checkout git")
	}
	repo, err := analyzer.OpenRepo(s.scanRoot)
	if err != nil {
		return nil, err
	}
	tracked, err := repo.TrackedFiles("HEAD")
	if err != nil {
		return nil, err
	}
	return reviewtools.NewRevision(reviewtools.RevisionOptions{
		Root:    s.scanRoot,
		Tracked: tracked,
		Ignored: s.cfg.IgnoresPath,
		Secret:  s.cfg.IsSecretPath,
		Budget:  reviewtools.NewByteBudget(s.cfg.Deliberation.EffectiveMaxReadBytes()),
	})
}

// redactText is the AUR-009 filter as a string function.
func (s *reviewState) redactText(text string) string {
	if s.filter == nil {
		return text
	}
	return s.filter.Redact(text)
}
