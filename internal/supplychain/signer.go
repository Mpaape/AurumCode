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
func (s Signer) SignBlob(ctx context.Context, opts Options, path string) error {
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
	args = append(args, path)
	if err := s.run(ctx, args); err != nil {
		return fmt.Errorf("supplychain: signing sbom %s: %w", path, err)
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
	args = append(args, path)
	if err := s.run(ctx, args); err != nil {
		return fmt.Errorf("supplychain: verifying sbom signature %s: %w", path, err)
	}
	return nil
}

// SignImage signs image (already validated by ValidateArtifactRef --
// called here again so this method is safe even when invoked directly,
// bypassing cmd/aurumcode's own config-time validation) with
// `cosign sign`.
func (s Signer) SignImage(ctx context.Context, opts Options, image string) error {
	if err := ValidateArtifactRef(image); err != nil {
		return fmt.Errorf("supplychain: signing image: %w", err)
	}
	args := []string{"sign", "--yes"}
	if opts.KeyPath != "" {
		args = append(args, "--key", opts.KeyPath)
	}
	if opts.SkipTransparencyLog {
		args = append(args, "--tlog-upload=false")
	}
	args = append(args, image)
	if err := s.run(ctx, args); err != nil {
		return fmt.Errorf("supplychain: signing image %s: %w", image, err)
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
