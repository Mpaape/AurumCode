// Package supplychain is AUR-551's Sigstore/Cosign signing: the SBOM
// (cosign sign-blob) and the artifact image (cosign sign), both shelling
// out to an external `cosign` binary -- never a network call or any
// cryptography this package performs itself, and never a key AurumCode
// manages (the card's own Non-goal). Verification (cosign verify-blob)
// runs only when a key is configured; production CI signs keyless
// through GitHub's own OIDC identity (no --key at all) and this package
// never verifies that path itself -- that is Sigstore's transparency log
// and the eventual consumer's job.
//
// Signer is the seam cmd/aurumcode's wiring (aur551.go) calls through, so
// the orchestration and its tests never depend on the real cosign binary
// being installed: tests and docs/specs/AUR-551.md's own real-proof run
// point Binary at a script or a pinned container instead of a system
// install. Every failure returned by this package names the artifact
// (the blob path or the image reference) that was being signed or
// verified -- a caller never has to re-derive which of several calls
// failed from a bare exec error (AC-002: "o parecer diz qual artefato
// ficou sem assinatura").
package supplychain

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Signer runs the cosign binary. Binary defaults to "cosign" (resolved
// through PATH by os/exec, exactly like internal/sbom.TrivyGenerator
// resolves "trivy").
type Signer struct {
	Binary string
}

func (s Signer) binary() string {
	if strings.TrimSpace(s.Binary) == "" {
		return "cosign"
	}
	return s.Binary
}

// Options controls how a single sign/verify call is shaped. KeyPath is
// the private key (signing) or public key (verifying) path; empty means
// keyless -- cosign's own ambient-OIDC flow (Fulcio/Rekor), used by
// production CI and never by this package's own tests, which always
// supply a local key so they run with no network. BundlePath, when set,
// is where cosign writes (signing) or reads (verifying) the Sigstore
// bundle carrying the signature, certificate and transparency-log proof.
// SkipTransparencyLog adds cosign's own offline switches
// (--tlog-upload=false --use-signing-config=false for signing,
// --insecure-ignore-tlog --insecure-ignore-sct for verifying) -- set only
// by tests and the documented real-proof run with an ephemeral local key
// that is never meant to reach the public transparency log; production
// keyless signing in CI leaves this false so the signature IS uploaded
// and becomes publicly verifiable.
type Options struct {
	KeyPath             string
	BundlePath          string
	SkipTransparencyLog bool
}

// ValidateArtifactRef refuses any image reference that is not pinned by a
// full sha256 digest -- a tag alone (":latest", ":v1", or nothing at all)
// is exactly the race/tamper window the card's own public contract
// refuses ("imagem a assinar vem por referencia com digest (@sha256:);
// tag sem digest e recusada"). Deliberately duplicated (never imported)
// from internal/config's own validateImageDigestRef: this package must
// refuse an unpinned image before it ever shells out to cosign, even when
// called directly rather than through cmd/aurumcode's config-validated
// path, and internal/config must never import this package in return.
func ValidateArtifactRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("image reference must not be empty")
	}
	// H3: a reference beginning with "-" could be read as a cosign flag
	// instead of a positional argument, depending on where it lands in
	// the built argv -- refused outright, never left to "--" alone to
	// neutralize (see signer.go's own run-argv comment for why both
	// defenses are kept).
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("image reference %q must not start with \"-\"", ref)
	}
	idx := strings.LastIndex(ref, "@sha256:")
	if idx < 0 {
		return fmt.Errorf("image reference %q must be pinned by digest (@sha256:<64 hex>), not a tag", ref)
	}
	digest := ref[idx+len("@sha256:"):]
	if len(digest) != 64 {
		return fmt.Errorf("image reference %q has a sha256 digest of the wrong length", ref)
	}
	for _, r := range digest {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return fmt.Errorf("image reference %q has a non-hex sha256 digest", ref)
		}
	}
	return nil
}

// SignBlob signs path (the SBOM file) with `cosign sign-blob`. Any
// failure is wrapped naming path, never a bare exec error -- AC-002's own
// "o parecer diz qual artefato ficou sem assinatura".
//
// When opts.BundlePath is set, this method removes any file already
// there BEFORE invoking cosign (so a stale bundle -- left over from a
// previous run, or one a PR happens to carry already committed -- can
// never be mistaken for this run's own output), and, after cosign exits
// 0, requires the bundle to exist with a non-zero size. cosign exiting 0
// without ever writing a bundle (a real failure mode, not merely
// hypothetical: a broken pipe to the signing backend, or a bug in a
// future cosign release) would otherwise read as "signed" from the exit
// code alone.
func (s Signer) SignBlob(ctx context.Context, opts Options, path string) error {
	if opts.BundlePath != "" {
		if err := removeStaleBundle(opts.BundlePath); err != nil {
			return fmt.Errorf("supplychain: signing sbom %s: %w", path, err)
		}
	}
	args := []string{"sign-blob", "--yes"}
	if opts.KeyPath != "" {
		args = append(args, "--key", opts.KeyPath)
	}
	if opts.BundlePath != "" {
		args = append(args, "--bundle", opts.BundlePath)
	}
	if opts.SkipTransparencyLog {
		args = append(args, "--tlog-upload=false", "--use-signing-config=false")
	}
	// H3: "--" stops cosign's own (pflag-based) flag parsing before the
	// positional blob path, confirmed against the pinned v3.1.3 binary's
	// own --help/behavior (docs/specs/AUR-551.md): a value placed after
	// "--" is never re-interpreted as a flag even when it starts with
	// "--" itself. ValidateArtifactRef's own leading-"-" refusal (image
	// refs) and this package's path check (SBOM paths, cmd/aurumcode's
	// own aur551.go) are the PRIMARY defense -- a single-dash value was
	// observed, in that same investigation, to still trigger cosign's
	// legacy shorthand-flag normalization even after "--", so "--" alone
	// is not relied upon here.
	args = append(args, "--", path)
	if err := s.run(ctx, args); err != nil {
		return fmt.Errorf("supplychain: signing sbom %s: %w", path, err)
	}
	if opts.BundlePath != "" {
		if err := requireNonEmptyFile(opts.BundlePath); err != nil {
			return fmt.Errorf("supplychain: signing sbom %s: cosign exited 0 but %w", path, err)
		}
	}
	return nil
}

// VerifyBlob verifies path's signature against opts.BundlePath. Used by
// tests and the documented real proof, never by production signing
// itself -- see the package doc for why keyless verification is out of
// scope here.
func (s Signer) VerifyBlob(ctx context.Context, opts Options, path string) error {
	args := []string{"verify-blob"}
	if opts.KeyPath != "" {
		args = append(args, "--key", opts.KeyPath)
	}
	if opts.BundlePath != "" {
		args = append(args, "--bundle", opts.BundlePath)
	}
	if opts.SkipTransparencyLog {
		args = append(args, "--insecure-ignore-tlog", "--insecure-ignore-sct")
	}
	args = append(args, "--", path)
	if err := s.run(ctx, args); err != nil {
		return fmt.Errorf("supplychain: verifying sbom signature %s: %w", path, err)
	}
	return nil
}

// SignImage signs image (already validated by ValidateArtifactRef --
// called here again so this method is safe even when invoked directly,
// bypassing cmd/aurumcode's own config-time validation) with
// `cosign sign`.
//
// An image has no local file of its own to check the way SignBlob checks
// a bundle next to the signed path, but `cosign sign` accepts the exact
// same `--bundle FILE` flag SignBlob uses ("write everything required to
// verify the image to FILE", confirmed on the pinned v3.1.3 binary's own
// --help). This method always asks for one -- opts.BundlePath when the
// caller names one, otherwise a private temp file cleaned up before
// returning -- purely to get the same "cosign exited 0 but wrote
// nothing" detection SignBlob has; the exit code remains the primary,
// unconditional signal (AC-002), and this check is additional, not a
// replacement for it.
func (s Signer) SignImage(ctx context.Context, opts Options, image string) error {
	if err := ValidateArtifactRef(image); err != nil {
		return fmt.Errorf("supplychain: signing image: %w", err)
	}
	bundlePath := opts.BundlePath
	if bundlePath == "" {
		tmp, err := os.CreateTemp("", "cosign-image-*.sigstore.json")
		if err != nil {
			return fmt.Errorf("supplychain: signing image %s: creating verification bundle: %w", image, err)
		}
		bundlePath = tmp.Name()
		_ = tmp.Close()
		defer os.Remove(bundlePath)
	}
	if err := removeStaleBundle(bundlePath); err != nil {
		return fmt.Errorf("supplychain: signing image %s: %w", image, err)
	}

	args := []string{"sign", "--yes", "--bundle", bundlePath}
	if opts.KeyPath != "" {
		args = append(args, "--key", opts.KeyPath)
	}
	if opts.SkipTransparencyLog {
		args = append(args, "--tlog-upload=false")
	}
	args = append(args, "--", image)
	if err := s.run(ctx, args); err != nil {
		return fmt.Errorf("supplychain: signing image %s: %w", image, err)
	}
	if err := requireNonEmptyFile(bundlePath); err != nil {
		return fmt.Errorf("supplychain: signing image %s: cosign exited 0 but %w", image, err)
	}
	return nil
}

// removeStaleBundle deletes any pre-existing file at path, so a leftover
// bundle from a previous run (or one a pull request happens to carry
// already committed) is never still sitting there, with a non-zero size,
// when requireNonEmptyFile checks this run's own output afterwards. A
// path that does not exist yet is not an error.
func removeStaleBundle(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale bundle %s: %w", path, err)
	}
	return nil
}

// requireNonEmptyFile is the other half of removeStaleBundle's own
// guarantee: since any old file at this path was already deleted before
// cosign ran, a missing or empty file here can only mean THIS cosign
// invocation itself wrote nothing, despite exiting 0.
func requireNonEmptyFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("wrote no signature bundle to %s: %w", path, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("wrote an empty signature bundle to %s", path)
	}
	return nil
}

// run executes the cosign binary with args, capturing combined
// stdout+stderr only to attach to the returned error (never printed
// verbatim elsewhere by this package -- the caller's own redaction
// writer decides how/where to surface it, exactly like
// internal/sbom.TrivyGenerator.run already does for trivy).
func (s Signer) run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, s.binary(), args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(combined.String()))
	}
	return nil
}
