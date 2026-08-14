package githubclient

// The diff model below is package-local on purpose. The historical client
// (commit c12d7ab) imported these shapes from pkg/types, but card AUR-437's
// sealed acceptance materializes only the card's declared paths/read_paths
// (internal/git/githubclient, tests/fixtures/scm/github, go.mod, go.sum), so
// the restored package must compile without reaching outside its own
// directory. The field names, meanings and json/yaml tags mirror
// pkg/types.Diff{,File,Hunk} exactly, so a caller that owns both packages
// (AUR-438's CLI wiring) can convert field-by-field without loss.

// DiffFile represents a single file in a diff
type DiffFile struct {
	Path  string     `json:"path" yaml:"path"`
	Lang  string     `json:"lang" yaml:"lang"`
	Hunks []DiffHunk `json:"hunks" yaml:"hunks"`
}

// DiffHunk represents a single hunk within a file diff
type DiffHunk struct {
	OldStart int      `json:"old_start" yaml:"old_start"`
	OldLines int      `json:"old_lines" yaml:"old_lines"`
	NewStart int      `json:"new_start" yaml:"new_start"`
	NewLines int      `json:"new_lines" yaml:"new_lines"`
	Lines    []string `json:"lines" yaml:"lines"`
}

// Diff represents the complete parsed diff
type Diff struct {
	Files []DiffFile `json:"files" yaml:"files"`
}
