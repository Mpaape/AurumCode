package cpp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// doxyfileTempPattern prefixes the private directory that holds one run's
	// Doxyfile. os.MkdirTemp appends random characters and creates it 0700.
	doxyfileTempPattern = "aurumcode-doxygen-"

	// doxyfileName is the configuration file name inside that private directory.
	doxyfileName = "Doxyfile"
)

// validateDoxyfileValue refuses any path that cannot be safely embedded in a
// doxygen configuration file.
//
// The doxygen config format is line oriented: a line break ends the current
// directive and the next line is parsed as a fresh one. Interpolating a path
// that contains a line break therefore lets whoever controls that path append
// directives of their own, and INPUT_FILTER makes doxygen run a shell command
// for every input file. Quoting the value is necessary but not sufficient,
// because a quoted value still ends at the line break.
//
// Refusing beats escaping here. A directory whose name carries a line break, a
// NUL or a raw control character is not a legitimate input for this pipeline, so
// a wrong refusal costs one clear error message while a wrong accept costs
// arbitrary command execution under the doxygen process.
//
// The returned error names the field and the byte offset but never echoes the
// value, so a hostile path cannot smuggle itself into logs.
func validateDoxyfileValue(field, value string) error {
	if value == "" {
		return fmt.Errorf("invalid %s for Doxyfile: value is empty", field)
	}

	for offset, r := range value {
		switch {
		case r == '\n' || r == '\r':
			return fmt.Errorf("invalid %s for Doxyfile: line break at byte offset %d would inject a doxygen directive", field, offset)
		case r < 0x20 || r == 0x7f:
			return fmt.Errorf("invalid %s for Doxyfile: control character U+%04X at byte offset %d", field, r, offset)
		case r == '"':
			return fmt.Errorf("invalid %s for Doxyfile: double quote at byte offset %d would terminate the quoted value", field, offset)
		}
	}

	// A trailing backslash would escape the closing quote this writer appends,
	// letting the value run on and swallow the directive on the next line.
	// Interior backslashes stay legal so native Windows paths still work.
	if strings.HasSuffix(value, `\`) {
		return fmt.Errorf("invalid %s for Doxyfile: value ends with a backslash, which would escape the closing quote", field)
	}

	// Doxygen expands $(NAME) inside configuration values; a path is never
	// meant to reach into this process's environment.
	if strings.Contains(value, "$(") {
		return fmt.Errorf("invalid %s for Doxyfile: value contains the $( environment expansion sequence", field)
	}

	return nil
}

// newDoxyfile creates a private directory owned by this extraction and writes
// the Doxyfile into it. The returned cleanup removes that directory and its
// contents.
//
// The per-run directory replaces a fixed os.TempDir()/Doxyfile path. That fixed
// path was shared by every concurrent extraction, so one run's deferred remove
// deleted another run's configuration, and any local user could pre-create it as
// a symlink to redirect the write.
func (c *CPPExtractor) newDoxyfile(sourceDir, outputDir string) (string, func(), error) {
	if err := validateDoxyfileValue("source directory", sourceDir); err != nil {
		return "", nil, err
	}
	if err := validateDoxyfileValue("output directory", outputDir); err != nil {
		return "", nil, err
	}

	dir, err := os.MkdirTemp("", doxyfileTempPattern)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary directory for Doxyfile: %w", err)
	}

	path := filepath.Join(dir, doxyfileName)
	if err := c.createDoxyfile(path, sourceDir, outputDir); err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}

	return path, func() { os.RemoveAll(dir) }, nil
}

// createDoxyfile writes the doxygen configuration for a single extraction.
//
// Both interpolated values are validated again here, so the file cannot be built
// through any other call path without the check, and both are written quoted so
// a path containing spaces stays a single value.
//
// The file is created with O_CREATE|O_EXCL|O_NOFOLLOW: creation fails if
// anything already exists at the path, a symlink included, so the write can
// never be redirected onto a file this process does not own. os.WriteFile, which
// this replaces, follows a pre-existing symlink and truncates its target.
func (c *CPPExtractor) createDoxyfile(path, sourceDir, outputDir string) error {
	if err := validateDoxyfileValue("source directory", sourceDir); err != nil {
		return err
	}
	if err := validateDoxyfileValue("output directory", outputDir); err != nil {
		return err
	}

	config := fmt.Sprintf(`PROJECT_NAME = "Documentation"
INPUT = "%s"
OUTPUT_DIRECTORY = "%s"
RECURSIVE = YES
GENERATE_HTML = NO
GENERATE_LATEX = NO
GENERATE_XML = YES
EXTRACT_ALL = YES
`, sourceDir, outputDir)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|openNoFollow, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create Doxyfile: %w", err)
	}

	if _, err := file.WriteString(config); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("failed to write Doxyfile: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("failed to close Doxyfile: %w", err)
	}

	return nil
}
