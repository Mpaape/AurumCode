package browserproof_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

// This file reproduces the attacks the lock and the artifact root are supposed
// to stop. Each case runs the attack, not the happy path: it puts a second
// binary where the harness could pick it up, or a link that points out of the
// artifact, and fails if the harness treats either as evidence.

// decoyMarker is written by the tests into files that stand in for content the
// harness must never read: a made-up token, never a real credential.
const decoyMarker = "AURUM-DECOY-OUTSIDE-THE-ARTIFACT-4b7c"

// markerDriverStub writes an executable that speaks browserproof-driver-v1,
// reports marker as the text it observed and, when sentinel is not empty,
// leaves that file behind so a test can tell whether it ran at all.
func markerDriverStub(t *testing.T, path, marker, sentinel string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the external driver protocol stub needs a POSIX shell")
	}

	script := "#!/bin/sh\ncat >/dev/null\n"
	if sentinel != "" {
		script += ": > '" + sentinel + "'\n"
	}
	script += fmt.Sprintf(
		"cat <<'BROWSERPROOF_EOF'\n"+
			`{"status":200,"bytes":%d,"selector_found":true,"text":%q,"body_text_length":%d}`+
			"\nBROWSERPROOF_EOF\n",
		len(marker), marker, len(marker))

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write driver stub: %v", err)
	}
}

func mustDigest(t *testing.T, path string) string {
	t.Helper()

	digest, err := browserproof.DriverDigestOf(path)
	if err != nil {
		t.Fatalf("digest driver: %v", err)
	}

	return digest
}

// chdir moves the process into dir for one test and puts it back afterwards, so
// a case can exercise the difference between the working directory and PATH.
func chdir(t *testing.T, dir string) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

// TestDriverLockCoversTheBinaryThatActuallyRuns is the PATH substitution: two
// executables answer to the same bare name, one in the working directory and one
// on PATH. The harness must fingerprint and run the same file, so the digest a
// lock is checked against is the digest of the browser that produced the
// observation.
func TestDriverLockCoversTheBinaryThatActuallyRuns(t *testing.T) {
	workDir := t.TempDir()
	pathDir := t.TempDir()

	const (
		fromWorkDir = "OBSERVED-BY-THE-WORKING-DIRECTORY-BINARY"
		fromPath    = "OBSERVED-BY-THE-PATH-BINARY"
	)
	markerDriverStub(t, filepath.Join(workDir, "drv"), fromWorkDir, "")
	markerDriverStub(t, filepath.Join(pathDir, "drv"), fromPath, "")

	observedBy := map[string]string{
		mustDigest(t, filepath.Join(workDir, "drv")): fromWorkDir,
		mustDigest(t, filepath.Join(pathDir, "drv")): fromPath,
	}
	if len(observedBy) != 2 {
		t.Fatal("the two stubs must have different digests for this case to mean anything")
	}

	chdir(t, workDir)
	t.Setenv("PATH", pathDir)

	driver := browserproof.NewExternalDriver("drv")

	fingerprint, err := driver.Fingerprint(context.Background())
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	observation, err := driver.Navigate(context.Background(), "http://127.0.0.1:1/index.html", symbolSelector)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}

	want, ok := observedBy[fingerprint.Digest]
	if !ok {
		t.Fatalf("the fingerprint digest %q belongs to neither stub", fingerprint.Digest)
	}
	if observation.Text != want {
		t.Fatalf("the lock covered the binary that reports %q and the binary that reports %q ran instead",
			want, observation.Text)
	}
}

// TestPathResolvedDriverOutsideTheLockIsRefusedAndNeverRuns pins the refusal: a
// bare driver name that PATH resolves to a binary the lock does not cover is
// refused as a driver mismatch, and that binary is never executed.
func TestPathResolvedDriverOutsideTheLockIsRefusedAndNeverRuns(t *testing.T) {
	workDir := t.TempDir()
	pathDir := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "the-unlocked-binary-ran")

	// Both stubs report exactly what the assertion wants, so only the lock
	// stands between the substituted binary and a verdict.
	markerDriverStub(t, filepath.Join(workDir, "drv"), symbolText, "")
	markerDriverStub(t, filepath.Join(pathDir, "drv"), symbolText, sentinel)

	// The lock pins the working-directory binary, which is what a run that
	// fingerprints a relative name would hash.
	lock := browserproof.DriverLock{
		Kind:   browserproof.ExternalDriverKind,
		Digest: mustDigest(t, filepath.Join(workDir, "drv")),
	}

	req := scriptedRequest(writeSite(t, map[string]string{
		"index.html": `<!doctype html><html><body><nav><a href="` + symbolRoute + `">Symbol</a></nav></body></html>`,
		"symbols/extractorpipeline.html": `<!doctype html><html><body><h1 id="symbol-name">` +
			symbolText + `</h1></body></html>`,
	}))
	req.DriverLock = lock

	chdir(t, workDir)
	t.Setenv("PATH", pathDir)

	result, err := browserproof.New(browserproof.NewExternalDriver("drv")).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("a driver outside the lock produced a verdict: outcome %q code %q", result.Outcome, result.Code)
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatal("a binary the lock never covered was executed")
	}
	if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeDriverMismatch {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s",
			result.Outcome, result.Code, browserproof.CodeDriverMismatch)
	}
}

// TestExternalDriverRefusesABinarySwappedAfterTheLockCheck closes the window
// between the fingerprint a lock is checked against and the process that runs:
// a driver whose bytes changed in between is refused instead of being launched.
func TestExternalDriverRefusesABinarySwappedAfterTheLockCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drv")
	sentinel := filepath.Join(t.TempDir(), "the-swapped-binary-ran")

	markerDriverStub(t, path, symbolText, "")

	driver := browserproof.NewExternalDriver(path)
	if _, err := driver.Fingerprint(context.Background()); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	markerDriverStub(t, path, "SWAPPED-AFTER-THE-LOCK-CHECK", sentinel)

	if _, err := driver.Navigate(context.Background(), "http://127.0.0.1:1/index.html", symbolSelector); err == nil {
		t.Fatal("a driver swapped after the lock check produced an observation")
	}
	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatal("a driver swapped after the lock check was executed")
	}
}

// TestSymlinkedRouteNeverBecomesEvidence is the artifact escape: a link inside
// the built site points at a file outside it, and the assertion asks for exactly
// what that file says. Serving it would let content the artifact does not
// contain prove a claim about the artifact.
func TestSymlinkedRouteNeverBecomesEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}

	outside := filepath.Join(t.TempDir(), "outside-the-artifact.html")
	if err := os.WriteFile(outside,
		[]byte(`<!doctype html><html><body><h1 id="symbol-name">`+decoyMarker+`</h1></body></html>`),
		0o600); err != nil {
		t.Fatalf("write decoy: %v", err)
	}

	site := writeSite(t, map[string]string{
		"index.html": `<!doctype html><html><body><nav><a href="/leak.html">leak</a></nav></body></html>`,
	})
	if err := os.Symlink(outside, filepath.Join(site, "leak.html")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	req := scriptedRequest(site)
	req.Assertions = []browserproof.RouteAssertion{{
		Route: "/leak.html", Selector: symbolSelector, ExpectedText: decoyMarker,
	}}

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("bytes from outside the artifact produced proof: outcome %q code %q", result.Outcome, result.Code)
	}

	// The assertion's own expected text is what the caller asked for, not
	// something the harness read, so it is dropped before the verdict is
	// searched for anything that could only have come from outside the root.
	observed := result
	observed.Routes = append([]browserproof.RouteObservationV1(nil), result.Routes...)
	for i := range observed.Routes {
		observed.Routes[i].ExpectedText = ""
	}

	encoded, marshalErr := json.Marshal(observed)
	if marshalErr != nil {
		t.Fatalf("encode verdict: %v", marshalErr)
	}
	if strings.Contains(string(encoded), decoyMarker) {
		t.Fatal("content from outside the artifact reached the verdict")
	}

	if len(result.Routes) != 1 || result.Routes[0].Status != http.StatusNotFound {
		t.Fatalf("the linked-out route was served with status %d, want %d",
			result.Routes[0].Status, http.StatusNotFound)
	}
	if result.Code != browserproof.CodeTargetAbsent {
		t.Fatalf("code = %q, want %s", result.Code, browserproof.CodeTargetAbsent)
	}
}

// shellDriverStub writes an executable driver whose body the case controls, so a
// stub can crash, stall or write on stderr on purpose.
func shellDriverStub(t *testing.T, path, body string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the external driver protocol stub needs a POSIX shell")
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write driver stub: %v", err)
	}
}

const stubObservation = `{"status":200,"bytes":17,"selector_found":true,` +
	`"text":"ExtractorPipeline","body_text_length":17}`

// TestDriverDigestRefusesAFileThatCannotBeExecuted keeps a lock from pinning
// something that could never have produced an observation: a readable file with
// no execute bit is an absent driver, not a browser with a digest.
func TestDriverDigestRefusesAFileThatCannotBeExecuted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute permission is expressed differently on windows")
	}

	path := filepath.Join(t.TempDir(), "drv")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	digest, err := browserproof.DriverDigestOf(path)
	if err == nil {
		t.Fatalf("a file that cannot be executed was fingerprinted as %q", digest)
	}
	if !errors.Is(err, browserproof.ErrDriverAbsent) {
		t.Fatalf("want ErrDriverAbsent, got %v", err)
	}
}

// TestDriverDigestRefusesAnEndlessFile pins the read that never comes back: a
// pipe carries the execute bit a lookup accepts, and hashing it would hang the
// run with no deadline to end it.
func TestDriverDigestRefusesAnEndlessFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes are created differently on windows")
	}

	path := filepath.Join(t.TempDir(), "drv")
	if err := syscall.Mkfifo(path, 0o755); err != nil {
		t.Skipf("the filesystem under test refuses a fifo: %v", err)
	}

	done := make(chan error, 1)
	go func() { _, err := browserproof.DriverDigestOf(path); done <- err }()

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, browserproof.ErrDriverAbsent) {
			t.Fatalf("want ErrDriverAbsent, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fingerprinting a pipe never returned")
	}
}

// TestDriverPathWithNoValueIsNamedAsSuch keeps the misconfiguration legible: an
// unset driver path is reported as a driver that was never configured, not as a
// lookup that happened to fail.
func TestDriverPathWithNoValueIsNamedAsSuch(t *testing.T) {
	for _, path := range []string{"", "   "} {
		_, err := browserproof.DriverDigestOf(path)
		if !errors.Is(err, browserproof.ErrDriverAbsent) {
			t.Fatalf("path %q: want ErrDriverAbsent, got %v", path, err)
		}
		if !strings.Contains(err.Error(), "no driver path configured") {
			t.Fatalf("path %q was refused as %q instead of an unconfigured driver", path, err)
		}
	}
}

// TestRelativeDriverSurvivesAChangeOfWorkingDirectory pins the resolution to one
// absolute file: a driver named relative to the working directory must stay the
// binary the fingerprint covered even if the harness moves afterwards.
func TestRelativeDriverSurvivesAChangeOfWorkingDirectory(t *testing.T) {
	const (
		fromFirst  = "OBSERVED-BY-THE-FINGERPRINTED-BINARY"
		fromSecond = "OBSERVED-BY-THE-BINARY-NEXT-DOOR"
	)

	first := t.TempDir()
	second := t.TempDir()
	markerDriverStub(t, filepath.Join(first, "drv"), fromFirst, "")
	markerDriverStub(t, filepath.Join(second, "drv"), fromSecond, "")

	chdir(t, first)

	driver := browserproof.NewExternalDriver("./drv")
	if _, err := driver.Fingerprint(context.Background()); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	chdir(t, second)

	observation, err := driver.Navigate(context.Background(), "http://127.0.0.1:1/index.html", symbolSelector)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if observation.Text != fromFirst {
		t.Fatalf("the fingerprinted binary reports %q and the observation came from %q",
			fromFirst, observation.Text)
	}
}

// TestDriverThatExitsNonZeroProducesNoObservation refuses a browser that printed
// something and then failed: a partial state a crashed process left behind is
// not an observation.
func TestDriverThatExitsNonZeroProducesNoObservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drv")
	shellDriverStub(t, path, "printf '%s' '"+stubObservation+"'\nexit 1")

	if _, err := browserproof.NewExternalDriver(path).Navigate(
		context.Background(), "http://127.0.0.1:1/index.html", symbolSelector); err == nil {
		t.Fatal("a driver that exited non-zero produced an observation")
	}
}

// TestDriverStderrNeverReachesTheObservation keeps runner output out of the
// protocol: whatever the browser writes on stderr is dropped, and the
// observation is decoded from stdout alone.
func TestDriverStderrNeverReachesTheObservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drv")
	shellDriverStub(t, path,
		"printf 'chatty browser diagnostics that are not part of the protocol\\n' >&2\n"+
			"printf '%s' '"+stubObservation+"'")

	observation, err := browserproof.NewExternalDriver(path).Navigate(
		context.Background(), "http://127.0.0.1:1/index.html", symbolSelector)
	if err != nil {
		t.Fatalf("driver output on stderr broke the observation: %v", err)
	}
	if observation.Text != symbolText {
		t.Fatalf("observed text = %q, want %q", observation.Text, symbolText)
	}
}

// TestNavigationDeadlineIsReportedAsATimeout keeps a stalled browser distinct
// from a broken one, and keeps it from outstaying the deadline: the stub leaves
// a child holding the output pipe, which is how a real browser keeps a harness
// waiting after it was told to stop.
func TestNavigationDeadlineIsReportedAsATimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drv")
	shellDriverStub(t, path, "sleep 60 &\nsleep 60")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	failed := make(chan error, 1)
	go func() {
		_, err := browserproof.NewExternalDriver(path).Navigate(ctx, "http://127.0.0.1:1/index.html", symbolSelector)
		failed <- err
	}()

	select {
	case err := <-failed:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("a driver past the deadline was reported as %v, want a deadline", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("a driver past its deadline kept the harness waiting")
	}
}

// TestValidateRejectsAForeignSchema keeps a verdict from another contract from
// being read as this one.
func TestValidateRejectsAForeignSchema(t *testing.T) {
	for _, schema := range []string{"", "BrowserProofResultV2", "SomeOtherReport"} {
		result := provedResult(t)
		result.Schema = schema

		if err := result.Validate(); err == nil {
			t.Fatalf("schema %q was accepted as %s", schema, browserproof.SchemaBrowserProofResultV1)
		}
	}
}
