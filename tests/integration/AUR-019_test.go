// Package integration holds structural integration checks for reconstruction
// board cards. This file belongs to AUR-019: it validates that the provider
// contracts matrix, the seven standards/providers/<slug>.yaml files and
// their capability fixtures are structurally complete, mirroring the checks
// tests/acceptance/AUR-019.sh performs inside the sealed acceptance
// container so the same regression is caught by the normal `go test ./...`
// suite.
package integration

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var expectedProvidersAUR019 = []string{"OpenAI", "LiteLLM", "Anthropic", "Ollama", "Azure", "Gemini", "Bedrock"}
var expectedCapabilitiesAUR019 = []string{"streaming", "tool_use", "structured_output"}

// TestAUR019Boundary is the AUR-019 integration selector
// (tests/integration/AUR-019_test.go::TestAUR019Boundary), following the
// Test-prefixed discoverable naming already used by AUR-020
// (tests/integration/AUR-020_test.go::TestAUR020Boundary). Being a
// `Test`-prefixed function of this shape means `go test ./...` discovers and
// runs it on its own, with no external dispatcher: this is what makes the
// regression it checks for actually run in CI instead of silently compiling
// into dead code. It re-derives the same structural checks as
// tests/acceptance/AUR-019.sh from bytes on disk, so a regression in the
// provider contracts is caught by two independent readers of the same
// source of truth.
func TestAUR019Boundary(t *testing.T) {
	t.Helper()

	root, err := findRepoRootAUR019()
	if err != nil {
		t.Fatalf("AUR-019: repo root unresolved: %v", err)
	}
	for _, dir := range []struct {
		path string
		what string
	}{
		{filepath.Join(root, "tests"), "tests directory"},
		{filepath.Join(root, "tests/specs"), "specs directory"},
		{filepath.Join(root, "tests/specs/AUR-019"), "AUR-019 specs directory"},
		{filepath.Join(root, "tests/specs/AUR-019/fixtures"), "fixture root directory"},
		{filepath.Join(root, "standards/providers"), "standards provider directory"},
		// .board/research is the research-baseline directory. The file-level
		// guards below (Lstat before read) prove providers.md itself is not
		// a symlink, but a symlinked PARENT -- .board/research pointing
		// outside the repository -- defeats them: the file resolves
		// externally while every per-file check still passes.
		// requireCanonicalDirectoryAUR019 resolves the directory and every
		// ancestor via filepath.EvalSymlinks and requires the physical path
		// to equal the lexical one, the same realpath-equivalent
		// tests/acceptance/AUR-019.sh applies through `pwd -P`.
		{filepath.Join(root, ".board/research"), "research directory"},
	} {
		requireCanonicalDirectoryAUR019(t, dir.path, dir.what)
	}

	names := readNonEmptyLinesGuardedAUR019(t, filepath.Join(root, "tests/specs/AUR-019/provider-names.txt"))
	if len(names) != 7 {
		t.Fatalf("AUR-019: provider-names fixture declares %d names, want exactly 7", len(names))
	}
	// A count of 7 is not proof all 7 are distinct: padding the list with a
	// second copy of an already-required name keeps len(names) at 7 while
	// silently excluding one real provider from every check the rest of
	// this test drives off names -- the standards-file loop below would
	// just reopen the duplicate's YAML twice and never touch the excluded
	// provider's file at all. Reject that before names drives anything.
	seenNames := map[string]bool{}
	for _, n := range names {
		if seenNames[n] {
			t.Fatalf("AUR-019: provider-names fixture declares a duplicate name: %s", n)
		}
		seenNames[n] = true
	}
	for _, want := range expectedProvidersAUR019 {
		if !seenNames[want] {
			t.Fatalf("AUR-019: provider-names fixture is not the exact required set; missing %s", want)
		}
	}
	caps := readNonEmptyLinesGuardedAUR019(t, filepath.Join(root, "tests/specs/AUR-019/capabilities.txt"))
	if len(caps) != len(expectedCapabilitiesAUR019) {
		t.Fatalf("AUR-019: capabilities fixture declares %d entries, want exactly %d", len(caps), len(expectedCapabilitiesAUR019))
	}
	seenCaps := map[string]bool{}
	for _, c := range caps {
		if seenCaps[c] {
			t.Fatalf("AUR-019: capabilities fixture declares a duplicate capability: %s", c)
		}
		seenCaps[c] = true
	}
	for _, want := range expectedCapabilitiesAUR019 {
		if !seenCaps[want] {
			t.Fatalf("AUR-019: capabilities fixture is not the exact required set; missing %s", want)
		}
	}
	allowedDomains := map[string]bool{}
	for _, d := range readNonEmptyLinesGuardedAUR019(t, filepath.Join(root, "tests/specs/AUR-019/allowed-source-domains.txt")) {
		allowedDomains[d] = true
	}

	research := filepath.Join(root, ".board/research/providers.md")
	researchBytes := readFileGuardedAUR019(t, research, "provider baseline")
	for _, needle := range []string{"OpenAI", "LiteLLM", "Anthropic", "Ollama", "Azure", "Gemini", "Bedrock"} {
		if !strings.Contains(string(researchBytes), needle) {
			t.Fatalf("AUR-019: provider baseline absent: missing %s", needle)
		}
	}

	header, rows := parseProviderMatrixAUR019(t, researchBytes)
	expectedCriteria := []string{"Wire format", "Auth", "Error taxonomy", "Capabilities"}
	if len(header) != len(expectedCriteria) {
		t.Fatalf("AUR-019: provider contracts matrix declares %d criteria columns, want exactly %d", len(header), len(expectedCriteria))
	}
	for i := range expectedCriteria {
		if header[i] != expectedCriteria[i] {
			t.Fatalf("AUR-019: provider contracts matrix criteria header %d is %q, want %q", i, header[i], expectedCriteria[i])
		}
	}

	linkRe := regexp.MustCompile(`\((https?://[^)/]+)`)
	fullURLRe := regexp.MustCompile(`\((https?://[^)]+)\)`)
	versionRe := regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,2}$`)
	dateVersionRe := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

	seen := map[string]int{}
	matrixByName := map[string][]string{}
	for _, row := range rows {
		if len(row) != 4+len(header) {
			t.Fatalf("AUR-019: matrix row has %d columns, want exactly %d", len(row), 4+len(header))
		}
		name := strings.TrimSpace(row[0])
		if !containsAUR019(names, name) {
			t.Fatalf("AUR-019: matrix contains unexpected provider: %s", name)
		}
		seen[name]++
		if seen[name] > 1 {
			t.Fatalf("AUR-019: provider cited twice in matrix: %s", name)
			continue
		}
		source := row[1]
		version := strings.TrimSpace(row[2])
		date := strings.TrimSpace(row[3])

		m := linkRe.FindStringSubmatch(source)
		if m == nil {
			t.Fatalf("AUR-019: provider %s source is not a [label](url) link", name)
			continue
		}
		host := strings.TrimPrefix(m[1], "https://")
		host = strings.TrimPrefix(host, "http://")
		if !allowedDomains[host] {
			t.Fatalf("AUR-019: provider %s source host is not allowlisted: %s", name, host)
		}
		if !versionRe.MatchString(version) && !dateVersionRe.MatchString(version) {
			t.Fatalf("AUR-019: provider %s has no versioned source: %q", name, version)
		}
		if !isRealDateAUR019(date) {
			t.Fatalf("AUR-019: provider %s source date is not a real ISO 8601 calendar date: %q", name, date)
		}
		if m := fullURLRe.FindStringSubmatch(source); m == nil {
			t.Fatalf("AUR-019: provider %s source is not a complete [label](url) link", name)
		} else {
			validateVersionedSourceAUR019(t, name, m[1], version)
		}
		for i := range header {
			cell := strings.TrimSpace(row[4+i])
			if len(cell) < 4 {
				t.Fatalf("AUR-019: provider %s has an empty criterion cell for %s", name, header[i])
			}
		}
		matrixByName[name] = row
	}
	for _, name := range names {
		if seen[name] == 0 {
			t.Fatalf("AUR-019: required provider missing from matrix: %s", name)
		}
	}

	requiredFlatKeys := []string{
		"provider", "source", "version", "date",
		"wire_endpoint", "wire_method", "wire_content_type",
		"auth_scheme", "auth_header", "auth_format",
		"error_envelope", "error_taxonomy", "transport",
	}

	// validatedYAML proves each required provider's standards file was
	// actually opened and validated, distinctly -- not just that names has
	// 7 distinct entries (checked above) or that this loop ran 7 times.
	// Those could still diverge if two distinct names ever mapped to the
	// same slug (case folding is the obvious way); tallying the actual
	// paths this loop opened, and requiring that tally equal len(names),
	// closes that regardless of how a future name and its slug could
	// collide.
	validatedYAML := map[string]bool{}

	for _, name := range names {
		slug := strings.ToLower(name)
		yamlPath := filepath.Join(root, "standards/providers", slug+".yaml")
		expectedYAMLPath := filepath.Join(root, "standards/providers", slug+".yaml")
		if filepath.Clean(yamlPath) != expectedYAMLPath || strings.Contains(filepath.ToSlash(yamlPath), "..") {
			t.Fatalf("AUR-019: standards path is not canonical for provider %s: %s", name, yamlPath)
		}
		requireCanonicalDirectoryAUR019(t, filepath.Dir(yamlPath), "standards provider directory")
		// tests/acceptance/AUR-019.sh refuses a symlinked standards file
		// with `[[ -f $yaml && ! -L $yaml ]]`; os.ReadFile alone follows a
		// symlink transparently, which would let this reader silently
		// validate bytes from outside standards/providers/ while the bash
		// gate refuses the same path. os.Lstat (not os.Stat) reports the
		// link itself rather than its target, so this catches that before
		// any read happens.
		yamlInfo, err := os.Lstat(yamlPath)
		if err != nil {
			t.Fatalf("AUR-019: standards file absent for provider %s: %v", name, err)
			continue
		}
		if yamlInfo.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("AUR-019: standards file for provider %s is a symlink, refusing to follow: %s", name, yamlPath)
			continue
		}
		yamlBytes, err := os.ReadFile(yamlPath)
		if err != nil {
			t.Fatalf("AUR-019: standards file absent for provider %s: %v", name, err)
			continue
		}
		checkAllowedBytesAUR019(t, yamlPath, yamlBytes)
		validatedYAML[yamlPath] = true
		fields := parseFlatYAMLAUR019(t, yamlPath, string(yamlBytes))

		// A capability_<name> key this card does not track is silently
		// invisible to every capability-driven check below (they all iterate
		// caps from capabilities.txt), so a YAML that smuggles
		// "capability_foo: supported" in is accepted unless this reader and
		// tests/acceptance/AUR-019.sh both refuse any capability_* key that
		// is not in capabilities.txt. Fail closed, same typed class
		// (undecided-provider) as an incomplete standards file: the provider
		// contract is not fully decided if it names a capability the card
		// does not track.
		for key := range fields {
			if strings.HasPrefix(key, "capability_") && !containsAUR019(caps, strings.TrimPrefix(key, "capability_")) {
				t.Fatalf("AUR-019: %s declares capability key %q that is not in capabilities.txt", yamlPath, key)
			}
		}

		if fields["provider"] != name {
			t.Fatalf("AUR-019: %s provider field is %q, want %q", yamlPath, fields["provider"], name)
		}
		for _, key := range requiredFlatKeys {
			if fields[key] == "" {
				t.Fatalf("AUR-019: %s has no %s value", yamlPath, key)
			}
		}

		// transport decides, explicitly and per-provider, which wire-format
		// fields a fixture for this provider must carry (see the fixture
		// loop below). auth_binding and wire_call are each required only
		// for the transport that names them -- a provider can never leave
		// both unset and let the fixture side decide which checks apply to
		// it, the exact "field absent = not applicable" shape a prior
		// round's review found, and reproduced, as a bypass.
		transport := fields["transport"]
		var authBinding, wireCall, wireStreamingSuffix, authQueryParam, authEvidence string
		switch transport {
		case "http":
			authBinding = fields["auth_binding"]
			switch authBinding {
			case "header", "query-param", "signed-request", "none":
			default:
				t.Fatalf("AUR-019: %s auth_binding has an unrecognized value: %q (must be 'header', 'query-param', 'signed-request', or 'none'; required because transport is http)", yamlPath, authBinding)
			}
			wireStreamingSuffix = fields["wire_streaming_suffix"]
			if authBinding == "query-param" {
				authQueryParam = fields["auth_query_param"]
				if authQueryParam == "" {
					t.Fatalf("AUR-019: %s has no auth_query_param value (required because auth_binding is query-param)", yamlPath)
				}
			}
			if authBinding == "signed-request" {
				authEvidence = fields["auth_evidence"]
				if authEvidence == "" {
					t.Fatalf("AUR-019: %s has no auth_evidence value (required because auth_binding is signed-request)", yamlPath)
				}
			}
		case "sdk-call":
			wireCall = fields["wire_call"]
			if wireCall == "" {
				t.Fatalf("AUR-019: %s has no wire_call value (required because transport is sdk-call)", yamlPath)
			}
		default:
			t.Fatalf("AUR-019: %s transport has an unrecognized value: %q (must be 'http' or 'sdk-call')", yamlPath, transport)
		}

		// Read once per provider (not per capability, below): every
		// capability's fixture is checked against the same provider's single
		// wire_method / wire_endpoint / auth_header, so there is no reason to
		// re-derive these on every loop iteration. The path regex is the one
		// exception -- wireStreamingSuffix only extends it for the
		// "streaming" capability's own fixture, so it is rebuilt per
		// capability below instead of once here.
		wireMethod := fields["wire_method"]
		wireEndpoint := fields["wire_endpoint"]
		authHeader := fields["auth_header"]

		sm := linkRe.FindStringSubmatch(fields["source"])
		if sm == nil {
			t.Fatalf("AUR-019: %s source is not a [label](url) link", yamlPath)
			continue
		}
		host := strings.TrimPrefix(sm[1], "https://")
		host = strings.TrimPrefix(host, "http://")
		if !allowedDomains[host] {
			t.Fatalf("AUR-019: %s source host is not allowlisted: %s", yamlPath, host)
		}
		if !versionRe.MatchString(fields["version"]) && !dateVersionRe.MatchString(fields["version"]) {
			t.Fatalf("AUR-019: %s has no versioned source: %q", yamlPath, fields["version"])
		}
		if !isRealDateAUR019(fields["date"]) {
			t.Fatalf("AUR-019: %s source date is not a real ISO 8601 calendar date: %q", yamlPath, fields["date"])
		}
		validateVersionedSourceFromMarkdownAUR019(t, name, fields["source"], fields["version"])
		matrixRow := matrixByName[name]
		if matrixRow[1] != fields["source"] || strings.TrimSpace(matrixRow[2]) != fields["version"] || strings.TrimSpace(matrixRow[3]) != fields["date"] {
			t.Fatalf("AUR-019: matrix source/version/date for %s does not match %s", name, yamlPath)
		}

		for _, capName := range caps {
			status := fields["capability_"+capName]
			if status == "" {
				t.Fatalf("AUR-019: %s has no capability_%s status", yamlPath, capName)
				continue
			}
			if status != "supported" && status != "unsupported" {
				t.Fatalf("AUR-019: %s capability_%s has an unrecognized status: %s", yamlPath, capName, status)
				continue
			}
			if status != "supported" {
				continue
			}
			fixturePath := fields["fixture_"+capName]
			if fixturePath == "" {
				t.Fatalf("AUR-019: %s marks capability_%s supported with no fixture_%s entry", yamlPath, capName, capName)
				continue
			}
			expectedFixturePath := filepath.ToSlash(filepath.Join("tests/specs/AUR-019/fixtures", slug, capName+".json"))
			if fixturePath != expectedFixturePath || strings.Contains(filepath.ToSlash(fixturePath), "..") || filepath.IsAbs(fixturePath) {
				t.Fatalf("AUR-019: %s fixture_%s is not the canonical path %s: %s", yamlPath, capName, expectedFixturePath, fixturePath)
			}
			fixtureAbs := filepath.Join(root, fixturePath)
			requireCanonicalDirectoryAUR019(t, filepath.Dir(fixtureAbs), "fixture provider directory")
			// Same reasoning as the standards-file Lstat guard above:
			// tests/acceptance/AUR-019.sh refuses a symlinked fixture with
			// `[[ -f "$fixture_path" && ! -L "$fixture_path" ]]`.
			fixtureInfo, err := os.Lstat(fixtureAbs)
			if err != nil {
				t.Fatalf("AUR-019: %s fixture_%s points at a missing file: %s", yamlPath, capName, fixturePath)
				continue
			}
			if fixtureInfo.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("AUR-019: %s fixture_%s is a symlink, refusing to follow: %s", yamlPath, capName, fixturePath)
				continue
			}
			fixtureBytes, err := os.ReadFile(fixtureAbs)
			if err != nil {
				t.Fatalf("AUR-019: %s fixture_%s points at a missing or unreadable file: %s", yamlPath, capName, fixturePath)
				continue
			}
			checkAllowedBytesAUR019(t, fixtureAbs, fixtureBytes)
			if len(strings.TrimSpace(string(fixtureBytes))) == 0 {
				t.Fatalf("AUR-019: %s fixture_%s points at an empty file: %s", yamlPath, capName, fixturePath)
				continue
			}
			var decoded struct {
				Capability string `json:"capability"`
				Provider   string `json:"provider"`
				Source     string `json:"source"`
				Request    struct {
					Method  string          `json:"method"`
					Path    string          `json:"path"`
					Headers json.RawMessage `json:"headers"`
					Call    string          `json:"call"`
					Auth    string          `json:"auth"`
					Params  json.RawMessage `json:"params"`
				} `json:"request"`
			}
			if err := json.Unmarshal(fixtureBytes, &decoded); err != nil {
				t.Fatalf("AUR-019: %s is not valid JSON: %v", fixturePath, err)
				continue
			}
			if decoded.Capability != capName {
				t.Fatalf("AUR-019: %s capability field is %q, want %q", fixturePath, decoded.Capability, capName)
			}
			if decoded.Provider != slug {
				t.Fatalf("AUR-019: %s provider field is %q, want %q", fixturePath, decoded.Provider, slug)
			}
			if decoded.Source != fields["source"] {
				t.Fatalf("AUR-019: %s source does not match %s", fixturePath, yamlPath)
			}
			validateVersionedSourceFromMarkdownAUR019(t, fixturePath, decoded.Source, fields["version"])

			// Wire-format binding: "capability"/"provider" above are the
			// fixture's own self-declared strings -- exactly what an
			// unrelated fixture with only those two fields edited would
			// still satisfy. A fixture is only actual evidence for the
			// provider it claims to document if its recorded request also
			// matches that provider's own standards/providers/<slug>.yaml
			// contract, not just its label.
			//
			// Which fields that requires is decided by transport, read from
			// this fixture's own provider's standards file above -- never
			// by whether the fixture happens to carry the field. A missing
			// field for the transport this provider declares is a typed
			// failure, not a skipped check: omitting request.method/path/
			// headers from an http-transport fixture used to make this
			// whole block a no-op (an HTTP-shaped body borrowed from a
			// different provider passed as long as those three keys were
			// deleted, not just edited); it no longer does, because "http"
			// now requires them unconditionally, and "sdk-call" now
			// requires its own corresponding request.call/request.params
			// fields unconditionally in exactly the same way -- neither
			// transport has an unchecked field left for a borrowed body to
			// hide in. json.RawMessage (not map[string]string) is what lets
			// Headers/Params distinguish "field absent" (nil) from "field
			// present but an empty object" (non-nil, "{}"): a plain
			// map[string]string cannot tell those two apart, and this check
			// is exactly the one a fixture author could otherwise satisfy
			// by declaring an empty object instead of omitting the field.
			if transport == "http" {
				if decoded.Request.Method == "" {
					t.Fatalf("AUR-019: %s has no request.method; %s declares transport: http, which requires one", fixturePath, yamlPath)
				}
				if decoded.Request.Method != wireMethod {
					t.Fatalf("AUR-019: %s request method %q does not match %s wire_method %q", fixturePath, decoded.Request.Method, yamlPath, wireMethod)
				}
				if decoded.Request.Path == "" {
					t.Fatalf("AUR-019: %s has no request.path; %s declares transport: http, which requires one", fixturePath, yamlPath)
				}
				capSuffix := ""
				if capName == "streaming" {
					capSuffix = wireStreamingSuffix
				}
				wireAction := fields["wire_action_"+capName]
				wirePathRe := wirePathPrefixRegexAUR019(t, wireEndpoint, capSuffix, wireAction)
				if !wirePathRe.MatchString(decoded.Request.Path) {
					t.Fatalf("AUR-019: %s request path %q does not match %s wire_endpoint %q", fixturePath, decoded.Request.Path, yamlPath, wireEndpoint)
				}
				switch authBinding {
				case "header":
					if decoded.Request.Headers == nil {
						t.Fatalf("AUR-019: %s has no request.headers object; %s declares auth_binding: header, which requires one", fixturePath, yamlPath)
					}
					var headers map[string]string
					if err := json.Unmarshal(decoded.Request.Headers, &headers); err != nil {
						t.Fatalf("AUR-019: %s request.headers is not a JSON object of strings: %v", fixturePath, err)
					}
					if _, ok := headers[authHeader]; !ok {
						t.Fatalf("AUR-019: %s request headers do not include %q, the auth_header %s declares", fixturePath, authHeader, yamlPath)
					}
					// Presence and JSON-string type are not enough: a header
					// value of "" declares a bound but empty credential, which
					// is exactly as useless to a real request as an absent
					// header and must fail closed here just as it does in
					// tests/acceptance/AUR-019.sh's fixture_header_present
					// (which returns a distinct "empty" status for `""`).
					if headers[authHeader] == "" {
						t.Fatalf("AUR-019: %s request header %q is empty; must have a non-empty string value, the auth_header %s declares", fixturePath, authHeader, yamlPath)
					}
				case "query-param":
					requireQueryParameterAUR019(t, fixturePath, decoded.Request.Path, authQueryParam)
				case "signed-request":
					if decoded.Request.Auth != authEvidence {
						t.Fatalf("AUR-019: %s request auth evidence does not match %s", fixturePath, yamlPath)
					}
				}
			} else {
				if decoded.Request.Call == "" {
					t.Fatalf("AUR-019: %s has no request.call; %s declares transport: sdk-call, which requires one", fixturePath, yamlPath)
				}
				if decoded.Request.Call != wireCall {
					t.Fatalf("AUR-019: %s request call %q does not match %s wire_call %q", fixturePath, decoded.Request.Call, yamlPath, wireCall)
				}
				// decoded.Request.Params == nil only catches a field that is
				// entirely absent from the JSON: json.RawMessage's slice stays
				// nil when json.Unmarshal never calls its UnmarshalJSON at
				// all, which is exactly the "key not present" case. It is
				// NOT nil for an explicit JSON `null` value -- Unmarshal
				// still invokes RawMessage.UnmarshalJSON with the four bytes
				// `null`, so the slice is non-nil (len 4, contents "null")
				// even though the fixture is not attesting to any object. A
				// prior round of this card missed that distinction here (it
				// does not for request.headers below, which re-decodes into
				// a map and so already turns a `null` value into a nil map
				// caught by the membership check) and a `"params": null`
				// fixture passed this engine while tests/acceptance/AUR-019.sh's
				// `fixture_has_request_params` correctly rejected it (that
				// awk only matches a literal `"params": {` opener, never
				// `"params": null`). Re-decoding into a real Go value, the
				// same way request.headers already does, closes the class:
				// a JSON `null` decodes into a nil map with no error, so it
				// is indistinguishable from "absent" by the check below --
				// which is the correct behavior, not a special case, since a
				// fixture with request.params: null is not recording any
				// object either. A JSON object, even an empty `{}`, decodes
				// into a non-nil (possibly empty) map and passes, matching
				// this card's doc ("must be present (an object, even if a
				// future fixture ever needed it empty)"). Anything that is
				// valid JSON but not an object or null (a string, number,
				// bool, or array -- e.g. `"params": "null"`, the quoted
				// string, which is neither absent nor an object) fails the
				// Unmarshal-into-map step itself with a type error, which is
				// the correct outcome for a different reason than absence:
				// it is present but the wrong shape, not missing.
				if decoded.Request.Params == nil {
					t.Fatalf("AUR-019: %s has no request.params object; %s declares transport: sdk-call, which requires one", fixturePath, yamlPath)
				}
				var params map[string]interface{}
				if err := json.Unmarshal(decoded.Request.Params, &params); err != nil {
					t.Fatalf("AUR-019: %s request.params is not a JSON object: %v", fixturePath, err)
				}
				if params == nil {
					t.Fatalf("AUR-019: %s has no request.params object; %s declares transport: sdk-call, which requires one", fixturePath, yamlPath)
				}
			}
		}

		expectedWire := fmt.Sprintf("transport=%s; endpoint=%s; method=%s; content_type=%s", transport, wireEndpoint, wireMethod, fields["wire_content_type"])
		binding := authBinding
		if transport == "sdk-call" {
			binding = "sdk-call"
		}
		expectedAuth := fmt.Sprintf("scheme=%s; header=%s; binding=%s; format=%s", fields["auth_scheme"], authHeader, binding, fields["auth_format"])
		if transport == "sdk-call" {
			expectedAuth += "; call=" + wireCall
		}
		expectedError := fmt.Sprintf("envelope=%s; taxonomy=%s", fields["error_envelope"], fields["error_taxonomy"])
		var capabilityCells []string
		for _, capName := range expectedCapabilitiesAUR019 {
			capabilityCells = append(capabilityCells, capName+"="+fields["capability_"+capName])
		}
		if matrixRow[4] != expectedWire || matrixRow[5] != expectedAuth || matrixRow[6] != expectedError || matrixRow[7] != strings.Join(capabilityCells, "; ") {
			t.Fatalf("AUR-019: matrix criteria for %s do not match %s", name, yamlPath)
		}
	}

	if len(validatedYAML) != len(names) {
		t.Fatalf("AUR-019: validated %d distinct standards files but required exactly %d", len(validatedYAML), len(names))
	}
}

// findRepoRootAUR019 walks up from the current working directory until it
// finds go.mod, so this file works whether `go test`/`go run` starts from
// the module root or from tests/integration itself.
func findRepoRootAUR019() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// readFileGuardedAUR019 reads path the same way tests/acceptance/AUR-019.sh
// reads every input file it opens: `[[ -f path && ! -L path ]]` before ever
// reading its bytes. os.ReadFile alone follows a symlink transparently,
// which would let this reader silently validate bytes from outside this
// card's owned paths while the bash gate refuses the identical symlinked
// path outright. os.Lstat (not os.Stat) reports the link itself rather than
// its target, so a symlink is caught here before any read happens -- the
// same guard the provider-loop's standards-file and fixture reads already
// use, now applied to every AUR-019 input file, not only those two.
func readFileGuardedAUR019(t *testing.T, path, what string) []byte {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("AUR-019: %s unreadable: %s: %v", what, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("AUR-019: %s is a symlink, refusing to follow: %s", what, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-019: %s unreadable: %s: %v", what, path, err)
	}
	checkAllowedBytesAUR019(t, path, data)
	return data
}

func checkAllowedBytesAUR019(t *testing.T, path string, data []byte) {
	t.Helper()
	line := 1
	for _, b := range data {
		if b == '\n' {
			line++
			continue
		}
		if b < 0x20 || b > 0x7e {
			t.Fatalf("AUR-019: malformed-entry: disallowed byte 0x%02x at %s:%d", b, path, line)
		}
	}
}

func requireCanonicalDirectoryAUR019(t *testing.T, path, what string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("AUR-019: %s unavailable: %s: %v", what, path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("AUR-019: %s is not a real directory: %s", what, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("AUR-019: %s absolute path unresolved: %s: %v", what, path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("AUR-019: %s real path unresolved: %s: %v", what, path, err)
	}
	if filepath.Clean(resolved) != filepath.Clean(abs) {
		t.Fatalf("AUR-019: %s resolves outside its canonical path: %s", what, path)
	}
}

// dateOnlyReAUR019 is the ISO-8601 calendar-date shape this card requires
// for the Date column and a standards file's `date` field. The shape check
// alone is not a date check: "2026-99-99" matches it. isRealDateAUR019 below
// is the real-calendar gate both readers now share, and this regexp exists
// only to give that gate its shape precondition.
var dateOnlyReAUR019 = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// isRealDateAUR019 reports whether s is a real calendar date: month 01-12,
// day between 01 and the number of days in that month for that year, with
// leap years per the Gregorian rule. It is the Go mirror of the POSIX-awk
// calendar check inside tests/acceptance/AUR-019.sh (real_calendar_date and
// the matrix walker's is_real_date): the shell gate accepted any 4-2-2 digit
// shape, so "2026-99-99" sailed through, and this reader must reject the
// same input by the same rule.
func isRealDateAUR019(s string) bool {
	if !dateOnlyReAUR019.MatchString(s) {
		return false
	}
	y, errY := strconv.Atoi(s[0:4])
	m, errM := strconv.Atoi(s[5:7])
	d, errD := strconv.Atoi(s[8:10])
	if errY != nil || errM != nil || errD != nil {
		return false
	}
	if y < 1 || m < 1 || m > 12 || d < 1 {
		return false
	}
	return d <= daysInMonthAUR019(y, m)
}

func daysInMonthAUR019(y, m int) int {
	switch m {
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func validateVersionedSourceAUR019(t *testing.T, provider, sourceURL, version string) {
	t.Helper()
	lower := strings.ToLower(sourceURL)
	for _, mutable := range []string{"/main", "/master", "/trunk", "/head", "/latest", "branch=main", "branch=master", "ref=main", "ref=master"} {
		if strings.Contains(lower, mutable) {
			t.Fatalf("AUR-019: provider %s source URL is mutable: %s", provider, sourceURL)
		}
	}
	withoutV := strings.TrimPrefix(version, "v")
	if !strings.Contains(sourceURL, version) && !strings.Contains(sourceURL, withoutV) {
		t.Fatalf("AUR-019: provider %s source URL does not carry declared version %s: %s", provider, version, sourceURL)
	}
}

func validateVersionedSourceFromMarkdownAUR019(t *testing.T, provider, source, version string) {
	t.Helper()
	re := regexp.MustCompile(`\((https?://[^)]+)\)`)
	match := re.FindStringSubmatch(source)
	if match == nil {
		t.Fatalf("AUR-019: %s source is not a complete [label](url) link", provider)
	}
	validateVersionedSourceAUR019(t, provider, match[1], version)
}

func requireQueryParameterAUR019(t *testing.T, fixture, requestPath, name string) {
	t.Helper()
	u, err := url.ParseRequestURI(requestPath)
	if err != nil {
		t.Fatalf("AUR-019: %s request path is not a valid URI: %v", fixture, err)
	}
	values, ok := u.Query()[name]
	if !ok || len(values) == 0 || values[0] == "" {
		t.Fatalf("AUR-019: %s request path lacks the declared query auth parameter %q", fixture, name)
	}
}

// wirePathPrefixRegexAUR019 builds the same anchored pattern
// tests/acceptance/AUR-019.sh's wire_path_prefix_regex builds from a
// provider's standards/providers/<slug>.yaml wire_endpoint value -- see that
// function's comment for the full rationale (placeholder substitution,
// truncation at the endpoint's first literal ':' to accommodate a
// capability-specific action suffix like Gemini's
// "{model}:generateContent" vs "{model}:streamGenerateContent", which this
// card's one-wire_endpoint-per-provider schema does not enumerate
// separately). regexp.QuoteMeta plays the same role
// tests/acceptance/AUR-019.sh's escape_ere_literal does: it does not escape
// "/", which has no special meaning in either engine's regex syntax and
// appears throughout every path this is used on.
//
// suffix is the caller's standards/providers/<slug>.yaml wire_streaming_suffix
// value, passed only while validating the "streaming" capability's own
// fixture (empty for every other capability, and empty for a provider that
// declares no such suffix at all -- e.g. every provider but Bedrock). The
// resulting pattern is anchored at a real end boundary, not left open-ended:
// after the resource segment (or the placeholder's greedy run), it accepts
// only end-of-string, a literal "?" (a query string), or -- when suffix is
// non-empty -- that exact literal suffix immediately before one of those two.
// Nothing else can follow: "/v1/chat/completions" no longer accepts
// "/v1/chat/completions-but-actually-something-else" as a match, because
// neither terminator branch matches at the position "-but-actually..."
// starts.
func wirePathPrefixRegexAUR019(t *testing.T, endpoint, suffix, action string) *regexp.Regexp {
	t.Helper()
	if i := strings.IndexByte(endpoint, ':'); i >= 0 {
		if action == "" {
			action = endpoint[i+1:]
		}
		endpoint = endpoint[:i]
	}
	var b strings.Builder
	b.WriteString("^")
	rest := endpoint
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			break
		}
		closeIdx := strings.IndexByte(rest[open:], '}')
		if closeIdx < 0 {
			break
		}
		closeIdx += open
		b.WriteString(regexp.QuoteMeta(rest[:open]))
		b.WriteString(`[^/?]+`)
		rest = rest[closeIdx+1:]
	}
	b.WriteString(regexp.QuoteMeta(rest))
	if action != "" {
		b.WriteString(":" + regexp.QuoteMeta(action))
	}
	if suffix != "" {
		b.WriteString("(")
		b.WriteString(regexp.QuoteMeta(suffix))
		b.WriteString(")")
	}
	b.WriteString(`($|\?)`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		t.Fatalf("AUR-019: wire_endpoint pattern failed to compile: %q: %v", endpoint, err)
	}
	return re
}

func readNonEmptyLinesGuardedAUR019(t *testing.T, path string) []string {
	t.Helper()
	data := readFileGuardedAUR019(t, path, "fixture")
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func containsAUR019(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

var tableRowSplitAUR019 = regexp.MustCompile(`\s*\|\s*`)
var tableRuleAUR019 = regexp.MustCompile(`^\s*\|\s*-+`)

// parseProviderMatrixAUR019 extracts the "## Provider contracts matrix" pipe
// table: it returns the criteria column names (everything right of "Date")
// and one []string per data row (Provider, Source, Version, Date, then one
// cell per criterion column, in header order). It takes the research
// document's bytes, already read by the caller through
// readFileGuardedAUR019, rather than a path to re-read: an independent,
// second os.ReadFile of the same path here would have its own, unguarded
// symlink-following read of that path, undoing the guard the caller already
// applied.
func parseProviderMatrixAUR019(t *testing.T, data []byte) ([]string, [][]string) {
	t.Helper()
	lines := strings.Split(string(data), "\n")
	inSection := false
	inTable := false
	headerSeen := false
	var header []string
	var rows [][]string

	splitRow := func(line string) []string {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "|")
		trimmed = strings.TrimSuffix(trimmed, "|")
		trimmed = strings.TrimSpace(trimmed)
		return tableRowSplitAUR019.Split(trimmed, -1)
	}

	for _, raw := range lines {
		line := raw
		if inSection && strings.HasPrefix(strings.TrimRight(line, " \t"), "## ") && strings.TrimSpace(line) != "## Provider contracts matrix" {
			break
		}
		if !inSection {
			if strings.TrimSpace(line) == "## Provider contracts matrix" {
				inSection = true
			}
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		if !headerSeen {
			cols := splitRow(line)
			if len(cols) > 4 {
				header = cols[4:]
			}
			headerSeen = true
			continue
		}
		if tableRuleAUR019.MatchString(line) {
			continue
		}
		rows = append(rows, splitRow(line))
	}

	if !headerSeen {
		t.Fatalf("AUR-019: no ## Provider contracts matrix table found in .board/research/providers.md")
	}
	return header, rows
}

// parseFlatYAMLAUR019 parses the deliberately flat "key: value" shape every
// standards/providers/<slug>.yaml file uses (see the comment at the top of
// each such file): one scalar per line, values optionally double-quoted.
// It is not a general YAML parser; it exists so this file and
// tests/acceptance/AUR-019.sh read the exact same on-disk shape without
// either one depending on a general-purpose nested-YAML walk.
//
// It fails closed the same way tests/acceptance/AUR-019.sh's flat_value
// does on two points that used to diverge:
//
//   - A duplicate key. flat_value's own comment already claimed "failing
//     closed on any duplicate is the only answer both readers agree on",
//     but this function used to be an ordinary map with silent
//     last-write-wins -- the opposite of what the comment promised. A
//     canonical-form standards file (one that already passed
//     check_canonical_form) can never legitimately declare the same key
//     twice, so this Fatals the instant it sees one, making that claim
//     true instead of aspirational.
//   - Trailing whitespace on an unquoted value. This used to run every
//     extracted value through strings.TrimSpace, so
//     "provider: OpenAI   " (three trailing spaces) would be accepted
//     here as "OpenAI" while tests/acceptance/AUR-019.sh's flat_value --
//     which only strips a value that is quote-wrapped end to end, never a
//     generic trim -- kept the trailing spaces and rejected the mismatch.
//     Stripping only a full "..." quote wrap here (matching flat_value's
//     own `sed -E 's/^"(.*)"$/\1/'` exactly) closes that gap: an unquoted
//     value's trailing whitespace now survives into fields[key] on both
//     readers.
func parseFlatYAMLAUR019(t *testing.T, path, content string) map[string]string {
	t.Helper()
	fields := map[string]string{}
	seen := map[string]int{}
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := line[idx+2:]
		if len(val) >= 2 && strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
			val = val[1 : len(val)-1]
		}
		seen[key]++
		if seen[key] > 1 {
			t.Fatalf("AUR-019: %s has key %q duplicated (line %d): tests/acceptance/AUR-019.sh's flat_value refuses this as undecided-provider, and this reader must refuse it too instead of applying last-write-wins", path, key, i+1)
		}
		fields[key] = val
	}
	return fields
}
