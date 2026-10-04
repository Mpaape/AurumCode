// The pull request read from GitHub: the client, the repository's config and
// context at the reviewed refs, the diff, the commits and the CI context.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// loadPullRequestConfig reads the explicit repository config without
// checking out pull-request code. GitHub Actions supplies the PR head SHA,
// so a config added by the change is available to that review; a direct local
// invocation falls back to the current working tree. A missing file is the
// zero-config contract and selects the stable English default.
func loadPullRequestConfig(ctx context.Context, client *githubclient.Client, owner, repo, headRef, baseRef string) (*config.Config, string, error) {
	var cfg *config.Config
	var err error
	if strings.TrimSpace(headRef) == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return nil, "", cwdErr
		}
		cfg, err = config.Load(cwd)
	} else if strings.TrimSpace(baseRef) == "" {
		data, found, fetchErr := client.GetRepositoryFile(ctx, owner, repo, config.DefaultConfigPath, headRef)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		if !found {
			cfg = &config.Config{}
		} else {
			cfg, err = config.Parse(data, config.DefaultConfigPath)
		}
	} else {
		// Gate-affecting settings come from the base branch. A PR may
		// propose a language change, but it cannot weaken its own review by
		// changing rules or ignored paths in the same diff.
		baseData, baseFound, fetchErr := client.GetRepositoryFile(ctx, owner, repo, config.DefaultConfigPath, baseRef)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		if baseFound {
			cfg, err = config.Parse(baseData, config.DefaultConfigPath)
		} else {
			cfg = &config.Config{}
		}
		if err != nil {
			return nil, "", err
		}
		headData, headFound, fetchErr := client.GetRepositoryFile(ctx, owner, repo, config.DefaultConfigPath, headRef)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		if headFound {
			headCfg, parseErr := config.Parse(headData, config.DefaultConfigPath)
			if parseErr != nil {
				return nil, "", parseErr
			}
			if strings.TrimSpace(headCfg.Review.Language) != "" {
				cfg.Review.Language = headCfg.Review.Language
			}
		}
	}
	if err != nil {
		return nil, "", err
	}
	language, err := cfg.ReviewLanguage()
	if err != nil {
		return nil, "", err
	}
	return cfg, language, nil
}

// loadPullRequestContext fetches the curated prompt, skills and documentation
// declared by review.context. The base ref is selected by the caller so a PR
// cannot add or alter review guidance for its own evaluation. The historical
// default prompt remains optional; explicitly listed files are required and
// fail with their path when missing.
func loadPullRequestContext(ctx context.Context, client *githubclient.Client, owner, repo string, cfg *config.Config, ref string) ([]config.ContextProvider, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	files := cfg.Review.ContextFiles()
	providers := make([]config.ContextProvider, 0, len(files))
	for _, file := range files {
		data, found, err := client.GetRepositoryFile(ctx, owner, repo, file.Path, ref)
		if err != nil {
			return nil, fmt.Errorf("reading review %s %q: %w", file.Kind, file.Path, err)
		}
		if !found {
			if file.Optional {
				continue
			}
			return nil, fmt.Errorf("review %s %q was configured but not found", file.Kind, file.Path)
		}
		providers = append(providers, config.NewTextContextProvider(file, string(data)))
	}
	return providers, nil
}

// parseOwnerRepo splits the --repo flag's "owner/repo" form. Anything else
// -- no slash, an empty owner or repo segment, or more than one slash -- is
// a usage error.
func parseOwnerRepo(repo string) (owner, name string, err error) {
	idx := strings.IndexByte(repo, '/')
	if idx <= 0 || idx == len(repo)-1 {
		return "", "", fmt.Errorf("expected the form owner/repo, got %q", repo)
	}
	owner, name = repo[:idx], repo[idx+1:]
	if strings.IndexByte(name, '/') != -1 {
		return "", "", fmt.Errorf("expected the form owner/repo, got %q", repo)
	}
	return owner, name, nil
}

// convertDiff mirrors internal/git/githubclient's package-local diff shape
// into pkg/types.Diff, field by field. The client's Diff/DiffFile/DiffHunk
// are deliberately local to its own package -- AUR-437's sealed acceptance
// materializes only that package's declared paths/read_paths, so it cannot
// import pkg/types -- and the two shapes mirror each other exactly (same
// field names, same meaning) precisely so this conversion, owned by this
// card, is lossless and trivial at the one boundary where a caller holds
// both packages.
func convertDiff(d *githubclient.Diff) *types.Diff {
	out := &types.Diff{Files: make([]types.DiffFile, len(d.Files))}
	for i, f := range d.Files {
		hunks := make([]types.DiffHunk, len(f.Hunks))
		for j, h := range f.Hunks {
			lines := make([]string, len(h.Lines))
			copy(lines, h.Lines)
			hunks[j] = types.DiffHunk{
				OldStart: h.OldStart,
				OldLines: h.OldLines,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
				Lines:    lines,
			}
		}
		out.Files[i] = types.DiffFile{Path: f.Path, Lang: f.Lang, Hunks: hunks}
	}
	return out
}

// readCIContext reads the optional, workflow-produced check summary. It is
// intentionally not a required user setting: a review still runs when CI has
// not reported anything yet. No client-side truncation is applied; the
// configured model/provider owns its context window and an explicit prompt
// budget, when used, remains the only review-content budget.
func readCIContext() string {
	path := strings.TrimSpace(os.Getenv("AURUMCODE_CI_CONTEXT_FILE"))
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

// codebaseContextPack resolves bounded codebase dependency context from the
// current checkout for the changed paths. It is an enhancement, never a gate:
// the caller degrades to empty context on any error.
func codebaseContextPack(changed []string) (*codebasectx.Pack, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return codebasectx.NewResolver().Resolve(cwd, changed)
}
