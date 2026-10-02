package sbom

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
)

// GeneratorConfig is an alias for config.SBOMGeneratorConfig. AUR-549 v2
// moved the single definition of quality_gates.ssor_dtrack.sbom_generator
// into internal/config (config.QualityGatesConfig, qualitygates.go) so it
// is read from the SAME .aurumcode/config.yml every other section already
// uses, with the SAME central-policy precedence (ApplyCentralPolicy) as
// Gate/Exceptions -- never a second, separate configuration file. The
// alias keeps this package's own API (and cmd/aurumcode's wiring) reading
// naturally as "the SBOM generator's config" without this package having
// to redeclare the struct.
type GeneratorConfig = config.SBOMGeneratorConfig

// ResolveOutputPath joins root and outputFile, refusing anything that
// would resolve outside root: an absolute path, a "../" escape, OR a
// symlink anywhere in the resolved parent chain that points outside root
// -- a repository could otherwise commit a symlinked directory and have a
// plain-looking relative output_file redirect the write anywhere on disk.
// output_file is repository- or policy-authored text, but it is still an
// externally supplied filename and is treated exactly like every other
// one this engine resolves -- untrusted as a path even when its source is
// trusted as configuration (CR-TRUST-001).
//
// Only the EXISTING part of the path is resolved through symlinks
// (resolveExistingSymlinks): a first SBOM run's output directory may not
// exist yet (GenerateAndValidate creates it with MkdirAll), so requiring
// the whole path to already exist (plain filepath.EvalSymlinks) would
// reject a valid, brand-new configuration.
func ResolveOutputPath(root, outputFile string) (string, error) {
	outputFile = strings.TrimSpace(outputFile)
	if outputFile == "" {
		return "", fmt.Errorf("sbom: output_file must not be empty")
	}
	clean := filepath.Clean(filepath.FromSlash(outputFile))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("sbom: output_file %q must be a relative path", outputFile)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("sbom: %w", err)
	}
	rootResolved, err := resolveExistingSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("sbom: resolving repository root %s: %w", rootAbs, err)
	}

	fullAbs := filepath.Join(rootAbs, clean)
	parentAbs := filepath.Dir(fullAbs)
	parentResolved, err := resolveExistingSymlinks(parentAbs)
	if err != nil {
		return "", fmt.Errorf("sbom: resolving %s: %w", parentAbs, err)
	}
	if parentResolved != rootResolved && !strings.HasPrefix(parentResolved, rootResolved+string(filepath.Separator)) {
		return "", fmt.Errorf("sbom: output_file %q escapes the repository root", outputFile)
	}

	// The final path is built from the RESOLVED parent plus the original
	// (unresolved) base name: resolving symlinks only decides whether the
	// destination escapes root, it never silently redirects the actual
	// write target to a different base name than the one configured.
	return filepath.Join(parentResolved, filepath.Base(fullAbs)), nil
}

// resolveExistingSymlinks resolves symlinks for the LONGEST EXISTING
// prefix of path (filepath.EvalSymlinks itself requires every component
// to exist, which a not-yet-created output directory often does not),
// then re-joins whatever trailing components do not exist yet, unresolved
// -- there is nothing to resolve in a path segment that is not a symlink
// because it does not exist at all.
func resolveExistingSymlinks(path string) (string, error) {
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		// Reached the filesystem root without finding an existing
		// component; nothing further to resolve.
		return path, nil
	}
	resolvedParent, err := resolveExistingSymlinks(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}
