package browserproof

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ScriptedDriverKind marks observations produced by the in-process test double
// instead of a real browser. A lock that pins ExternalDriverKind refuses it, so
// the double can never stand in for a pinned browser in a production run.
const ScriptedDriverKind = "scripted-test-double"

// ScriptedDriverDigest pins the identity of the double. It is the digest of the
// double's protocol identity, not of a browser binary.
var ScriptedDriverDigest = func() string {
	sum := sha256.Sum256([]byte("browserproof.ScriptedDriver/v1"))
	return "sha256:" + hex.EncodeToString(sum[:])
}()

// ForgeMode makes the double report observations the served artifact does not
// support. It exists so tests can prove the harness refuses driver-reported
// success that the local server ledger does not corroborate.
type ForgeMode int

const (
	ForgeNone ForgeMode = iota
	// ForgeClaimSuccess navigates for real and then reports success regardless
	// of what came back.
	ForgeClaimSuccess
	// ForgeWithoutFetch reports success without navigating at all.
	ForgeWithoutFetch
)

// ScriptedDriver is the test double for the browser seam. It fetches over
// loopback only and resolves selectors without a scripting engine, which it
// declares through its fingerprint.
type ScriptedDriver struct {
	Forge      ForgeMode
	ForgedText string
	Delay      time.Duration

	client *http.Client
}

func NewScriptedDriver() *ScriptedDriver {
	return &ScriptedDriver{
		ForgedText: "forged-observation",
		client:     loopbackClient(),
	}
}

func (d *ScriptedDriver) Fingerprint(context.Context) (DriverFingerprint, error) {
	return DriverFingerprint{
		Kind:               ScriptedDriverKind,
		Digest:             ScriptedDriverDigest,
		ExecutesJavaScript: false,
	}, nil
}

func (d *ScriptedDriver) Navigate(ctx context.Context, url, selector string) (PageObservation, error) {
	if d.Delay > 0 {
		timer := time.NewTimer(d.Delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return PageObservation{}, ctx.Err()
		case <-timer.C:
		}
	}

	if d.Forge == ForgeWithoutFetch {
		return d.forged(0), nil
	}

	start := time.Now()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PageObservation{}, fmt.Errorf("browserproof: build request: %w", err)
	}

	response, err := d.client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return PageObservation{}, ctxErr
		}
		return PageObservation{}, fmt.Errorf("browserproof: navigate: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, MaxPageBytes+1))
	if err != nil {
		return PageObservation{}, fmt.Errorf("browserproof: read page: %w", err)
	}
	if int64(len(body)) > MaxPageBytes {
		return PageObservation{}, fmt.Errorf("browserproof: page exceeds %d bytes", int64(MaxPageBytes))
	}

	if d.Forge == ForgeClaimSuccess {
		return d.forged(time.Since(start)), nil
	}

	observation := PageObservation{
		Status:      response.StatusCode,
		Bytes:       int64(len(body)),
		StableAfter: time.Since(start),
	}
	if response.StatusCode == http.StatusOK {
		tokens := tokenizeHTML(string(body))
		observation.Links = extractLinks(tokens)
		observation.BodyTextLength = len(visibleText(tokens))
		observation.RequiresJavaScript = hasExecutableScript(tokens)

		if selector != "" {
			spec, err := parseSelector(selector)
			if err != nil {
				return PageObservation{}, err
			}
			observation.Text, observation.SelectorFound = selectorText(tokens, spec)
		}
	}

	return observation, nil
}

func (d *ScriptedDriver) forged(elapsed time.Duration) PageObservation {
	return PageObservation{
		Status:         http.StatusOK,
		Bytes:          int64(len(d.ForgedText)),
		SelectorFound:  true,
		Text:           d.ForgedText,
		BodyTextLength: len(d.ForgedText),
		StableAfter:    elapsed,
	}
}

// loopbackClient refuses to open a connection to anything but the local
// artifact server, so a proof run reaches no external network.
func loopbackClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("browserproof: unusable address %q", address)
			}
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return fmt.Errorf("browserproof: refusing non-loopback address %q", address)
			}
			return nil
		},
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
}
