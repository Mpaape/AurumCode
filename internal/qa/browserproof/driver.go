package browserproof

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DriverPathEnv names the pinned browser driver executable.
const DriverPathEnv = "AURUM_BROWSERPROOF_DRIVER"

// ExternalDriverKind is the driver kind a production lock pins: a real headless
// browser reached through an external executable, identified by its digest.
const ExternalDriverKind = "external-headless-v1"

// driverProtocol is the request/response contract spoken over the driver's
// stdin and stdout. It carries observations only: no HTML, no screenshots.
const driverProtocol = "browserproof-driver-v1"

const maxDriverResponseBytes = 1 << 20

// driverWaitDelay bounds how long a cancelled driver may keep the harness
// waiting on its output before the process group is torn down.
const driverWaitDelay = time.Second

// DriverFingerprint identifies the driver that produced an observation.
type DriverFingerprint struct {
	Kind               string
	Digest             string
	ExecutesJavaScript bool
}

// PageObservation is everything a driver may report about one navigation.
type PageObservation struct {
	Status             int
	Bytes              int64
	SelectorFound      bool
	Text               string
	BodyTextLength     int
	Links              []string
	RequiresJavaScript bool
	StableAfter        time.Duration
}

// Driver is the headless browser seam. Implementations report observations;
// they never decide a verdict.
type Driver interface {
	Fingerprint(ctx context.Context) (DriverFingerprint, error)
	Navigate(ctx context.Context, url, selector string) (PageObservation, error)
}

// resolveDriverPath resolves the pinned driver to exactly one absolute path.
//
// It exists because a name without a path separator is found in two different
// places by the two standard-library calls this package needs: os.Stat reads it
// relative to the working directory, exec.Command resolves it through PATH.
// Hashing one file and running the other produces a lock that covers a binary
// nobody executed, so the driver is resolved once here and that single path is
// what gets hashed and what gets launched.
func resolveDriverPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: no driver path configured", ErrDriverAbsent)
	}

	// A lookup this refuses — missing, not executable, or reachable only
	// through a relative PATH entry — is reported as an absent driver, never
	// resolved a second way.
	found, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrDriverAbsent, path)
	}

	absolute, err := filepath.Abs(found)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrDriverAbsent, path)
	}

	return absolute, nil
}

// DriverDigestOf fingerprints a driver executable for the lock. It resolves the
// path the same way the run will, so the digest names the file that runs.
func DriverDigestOf(path string) (string, error) {
	resolved, err := resolveDriverPath(path)
	if err != nil {
		return "", err
	}

	return digestOfResolvedDriver(resolved)
}

// digestOfResolvedDriver hashes one already-resolved absolute path.
func digestOfResolvedDriver(resolved string) (string, error) {
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrDriverAbsent, resolved)
		}
		return "", fmt.Errorf("browserproof: stat driver: %w", err)
	}
	// Anything that is not a plain file is refused rather than read: hashing a
	// directory, a device or a pipe cannot identify a browser, and reading the
	// last two never returns.
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s is not an executable", ErrDriverAbsent, resolved)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("browserproof: open driver: %w", err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("browserproof: read driver: %w", err)
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type driverRequestV1 struct {
	Protocol string `json:"protocol"`
	URL      string `json:"url"`
	Selector string `json:"selector,omitempty"`
	MaxBytes int64  `json:"max_bytes"`
}

type driverResponseV1 struct {
	Status         int      `json:"status"`
	Bytes          int64    `json:"bytes"`
	SelectorFound  bool     `json:"selector_found"`
	Text           string   `json:"text"`
	BodyTextLength int      `json:"body_text_length"`
	Links          []string `json:"links,omitempty"`
	RequiresJS     bool     `json:"requires_javascript,omitempty"`
	StableAfterMs  int64    `json:"stable_after_ms,omitempty"`
}

// ExternalDriver drives a headless browser through a pinned executable. An
// absent or unreadable executable is reported as ErrDriverAbsent, so a missing
// browser can never be mistaken for a passing assertion.
type ExternalDriver struct {
	path string

	// The identity Fingerprint resolved and reported is the identity a lock was
	// checked against, so Navigate reuses that exact path — a relative name
	// would otherwise follow the working directory to a different file — and
	// refuses to launch it if its bytes no longer hash to the pinned digest.
	mu           sync.Mutex
	pinnedPath   string
	pinnedDigest string
}

// pinnedIdentity returns the path and digest Fingerprint recorded, if any.
func (d *ExternalDriver) pinnedIdentity() (string, string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.pinnedPath, d.pinnedDigest
}

func NewExternalDriver(path string) *ExternalDriver {
	return &ExternalDriver{path: path}
}

// NewExternalDriverFromEnv reads the pinned driver path from DriverPathEnv.
func NewExternalDriverFromEnv() *ExternalDriver {
	return NewExternalDriver(os.Getenv(DriverPathEnv))
}

func (d *ExternalDriver) Fingerprint(context.Context) (DriverFingerprint, error) {
	resolved, err := resolveDriverPath(d.path)
	if err != nil {
		return DriverFingerprint{}, err
	}

	digest, err := digestOfResolvedDriver(resolved)
	if err != nil {
		return DriverFingerprint{}, err
	}

	d.mu.Lock()
	d.pinnedPath, d.pinnedDigest = resolved, digest
	d.mu.Unlock()

	return DriverFingerprint{
		Kind:               ExternalDriverKind,
		Digest:             digest,
		ExecutesJavaScript: true,
	}, nil
}

func (d *ExternalDriver) Navigate(ctx context.Context, url, selector string) (PageObservation, error) {
	payload, err := json.Marshal(driverRequestV1{
		Protocol: driverProtocol,
		URL:      url,
		Selector: selector,
		MaxBytes: MaxPageBytes,
	})
	if err != nil {
		return PageObservation{}, fmt.Errorf("browserproof: encode driver request: %w", err)
	}

	// The path is resolved once — here, or already by Fingerprint — and it is
	// both hashed and executed below, so the digest a lock accepted is the
	// digest of the process that runs.
	resolved, pinned := d.pinnedIdentity()
	if resolved == "" {
		var err error
		if resolved, err = resolveDriverPath(d.path); err != nil {
			return PageObservation{}, err
		}
	}

	digest, err := digestOfResolvedDriver(resolved)
	if err != nil {
		return PageObservation{}, err
	}

	if pinned != "" && digest != pinned {
		// The executable changed between the fingerprint a lock was checked
		// against and this navigation. Refusing is the only answer that keeps
		// the verdict tied to the browser the lock covered.
		return PageObservation{}, fmt.Errorf(
			"browserproof: the driver changed after it was fingerprinted, refusing to launch it")
	}

	stdout := &cappedBuffer{limit: maxDriverResponseBytes}

	cmd := exec.CommandContext(ctx, resolved)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = io.Discard
	// The driver inherits no environment: the secret canary, tokens and proxy
	// settings of the runner must not reach the browser process.
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/nonexistent"}
	// A browser that leaves a child holding the output pipe would keep this
	// call running long past the navigation deadline the caller declared, so
	// the wait after cancellation is bounded too.
	cmd.WaitDelay = driverWaitDelay

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	if ctxErr := ctx.Err(); ctxErr != nil {
		return PageObservation{}, ctxErr
	}
	if runErr != nil {
		// Driver stderr is deliberately dropped instead of being embedded in the
		// verdict, which must never carry runner output.
		return PageObservation{}, fmt.Errorf("browserproof: driver %s failed", d.path)
	}

	decoder := json.NewDecoder(bytes.NewReader(stdout.buf.Bytes()))
	decoder.DisallowUnknownFields()

	var response driverResponseV1
	if err := decoder.Decode(&response); err != nil {
		return PageObservation{}, fmt.Errorf("browserproof: driver response: %w", err)
	}

	stable := elapsed
	if response.StableAfterMs > 0 {
		stable = time.Duration(response.StableAfterMs) * time.Millisecond
	}

	return PageObservation{
		Status:             response.Status,
		Bytes:              response.Bytes,
		SelectorFound:      response.SelectorFound,
		Text:               response.Text,
		BodyTextLength:     response.BodyTextLength,
		Links:              response.Links,
		RequiresJavaScript: response.RequiresJS,
		StableAfter:        stable,
	}, nil
}

// cappedBuffer refuses a driver that tries to flood the harness.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.limit {
		return 0, errors.New("browserproof: driver response exceeds the response limit")
	}

	return c.buf.Write(p)
}
