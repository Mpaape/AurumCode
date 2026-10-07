package main

import "github.com/Mpaape/AurumCode/internal/config"

// codebaseExclude is what the codebase context never reads (AUR-470): the
// policy's ignore globs and the secret-file catalog.
func codebaseExclude(cfg *config.Config) func(string) bool {
	return func(path string) bool { return cfg.IgnoresPath(path) || cfg.IsSecretPath(path) }
}

// includedFiles is files without the ones the policy excludes, for the
// verified --pr scan set.
func includedFiles(files []string, cfg *config.Config) []string {
	exclude := codebaseExclude(cfg)
	out := make([]string, 0, len(files))
	for _, f := range files {
		if !exclude(f) {
			out = append(out, f)
		}
	}
	return out
}
