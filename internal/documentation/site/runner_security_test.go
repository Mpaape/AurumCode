package site

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// syntheticCredential builds an obviously fake, credential-shaped token at test
// runtime. It is never a real secret, and no test in this file ever prints it.
func syntheticCredential(prefix string, filler byte, length int) string {
	return prefix + strings.Repeat(string(filler), length)
}

// shellSingleQuote quotes a token for /bin/sh. The tokens used here never
// contain a single quote, so the simple form is enough and is asserted.
func shellSingleQuote(t *testing.T, s string) string {
	t.Helper()

	if strings.ContainsRune(s, '\'') {
		t.Fatalf("test token must not contain a single quote")
	}

	return "'" + s + "'"
}

func newTestRunner() *DefaultRunner {
	return NewDefaultRunner().WithTimeout(60 * time.Second)
}

// TestDefaultRunnerDoesNotInheritParentEnvironment reproduces the leak: a
// canary planted in the parent process environment must never be visible to the
// child tool. The parent of this runner (the Action entrypoint) holds the
// provider API key, so full inheritance hands it to every external tool.
func TestDefaultRunnerDoesNotInheritParentEnvironment(t *testing.T) {
	const canaryName = "AURUMCODE_CANARY_MUST_NOT_ESCAPE"

	canaryValue := "canary-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Setenv(canaryName, canaryValue)
	// Names the Action actually exports into the parent process.
	t.Setenv("LLM_API_KEY", syntheticCredential("sk-", 'A', 32))
	t.Setenv("OPENAI_API_KEY", syntheticCredential("sk-", 'A', 32))

	out, err := newTestRunner().Run(context.Background(), "/bin/sh", []string{"-c", "env"}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}

	if strings.Contains(out, canaryName) || strings.Contains(out, canaryValue) {
		t.Fatal("parent-only canary variable reached the child process environment")
	}

	for _, name := range []string{"LLM_API_KEY", "OPENAI_API_KEY"} {
		if strings.Contains(out, name) {
			t.Fatalf("provider credential variable %s reached the child process environment", name)
		}
	}

	if strings.Contains(out, "sk-") {
		t.Fatal("a credential-shaped value reached the child process environment")
	}

	// Positive control: the allowlist must still give the tool a usable PATH,
	// otherwise this test would pass simply by breaking every extractor.
	if !strings.Contains(out, "PATH=") {
		t.Fatal("allowlisted PATH was not forwarded to the child process")
	}
}

// TestDefaultRunnerInheritsNothingWhenExplicitEnvIsSupplied covers the same leak
// on the branch that already set command.Env: the explicit map must be additive
// to the allowlist only, never to the whole parent environment.
func TestDefaultRunnerInheritsNothingWhenExplicitEnvIsSupplied(t *testing.T) {
	const canaryName = "AURUMCODE_CANARY_EXPLICIT_ENV"

	canaryValue := "canary-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Setenv(canaryName, canaryValue)

	env := map[string]string{"TOOL_SETTING": "explicit-value"}

	out, err := newTestRunner().Run(
		context.Background(),
		"/bin/sh",
		[]string{"-c", "env"},
		t.TempDir(),
		env,
	)
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}

	if strings.Contains(out, canaryName) || strings.Contains(out, canaryValue) {
		t.Fatal("parent-only canary variable reached the child process environment")
	}

	if !strings.Contains(out, "TOOL_SETTING=explicit-value") {
		t.Fatal("explicitly supplied environment variable was not forwarded")
	}
}

// TestDefaultRunnerStderrNeverReachesSuccessOutput reproduces the second half of
// the finding: tool stderr was appended to the success return value, which the
// pipeline log.Printf's straight into the public Action log.
func TestDefaultRunnerStderrNeverReachesSuccessOutput(t *testing.T) {
	token := syntheticCredential("sk-", 'A', 32)
	script := fmt.Sprintf("printf '%%s\\n' %s >&2; printf 'ok'", shellSingleQuote(t, token))

	out, err := newTestRunner().Run(context.Background(), "/bin/sh", []string{"-c", script}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}

	if out != "ok" {
		t.Fatalf("stderr contaminated the success output (returned %d bytes, want 2)", len(out))
	}

	if strings.Contains(out, "sk-") {
		t.Fatal("credential-shaped stderr token reached the success return value")
	}
}

// TestDefaultRunnerRedactsCredentialShapesInErrorDiagnostics proves the error
// path keeps stderr usable for debugging while stripping credential shapes.
func TestDefaultRunnerRedactsCredentialShapesInErrorDiagnostics(t *testing.T) {
	pemBody := strings.Repeat("Z", 64)
	cases := []struct {
		name  string
		token string
	}{
		{"openai", syntheticCredential("sk-", 'A', 32)},
		{"aws-access-key", syntheticCredential("AKIA", 'B', 16)},
		{"github-pat", syntheticCredential("ghp_", 'C', 36)},
		{"github-oauth", syntheticCredential("gho_", 'C', 36)},
		{"github-user", syntheticCredential("ghu_", 'C', 36)},
		{"github-server", syntheticCredential("ghs_", 'C', 36)},
		{"github-refresh", syntheticCredential("ghr_", 'C', 36)},
		{"private-key", "-----BEG" + "IN RSA PRIVATE KEY----- " + pemBody + " -----END RSA PRIVATE KEY-----"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := fmt.Sprintf("printf '%%s\\n' %s >&2; exit 3", shellSingleQuote(t, tc.token))

			out, err := newTestRunner().Run(context.Background(), "/bin/sh", []string{"-c", script}, t.TempDir(), nil)
			if err == nil {
				t.Fatal("expected a non-nil error for a failing command")
			}

			if out != "" {
				t.Fatalf("failing command returned %d bytes of output, want 0", len(out))
			}

			msg := err.Error()
			if strings.Contains(msg, tc.token) {
				t.Fatal("raw credential-shaped token survived into the error diagnostic")
			}

			if strings.Contains(msg, pemBody) {
				t.Fatal("private key body survived into the error diagnostic")
			}

			if !strings.Contains(msg, RedactionMarker) {
				t.Fatalf("error diagnostic is missing the redaction marker: %q", RedactionMarker)
			}

			// The diagnostic must still be useful.
			if !strings.Contains(msg, "exit status 3") {
				t.Fatalf("error diagnostic lost the exit status")
			}
		})
	}
}

// TestDefaultRunnerTruncatesStderrDiagnostics proves the explicit size bound.
func TestDefaultRunnerTruncatesStderrDiagnostics(t *testing.T) {
	// Larger than redactScanLimit so both size cuts run, and below the kernel's
	// per-argument limit so the child actually execs.
	huge := strings.Repeat("Z", 100_000)

	_, err := newTestRunner().Run(
		context.Background(),
		"/bin/sh",
		[]string{"-c", `printf '%s' "$0" >&2; exit 1`, huge},
		t.TempDir(),
		nil,
	)
	if err == nil {
		t.Fatal("expected a non-nil error for a failing command")
	}

	msg := err.Error()
	if len(msg) > MaxStderrDiagnosticBytes+512 {
		t.Fatalf("error diagnostic is %d bytes, want at most %d", len(msg), MaxStderrDiagnosticBytes+512)
	}

	if !strings.Contains(msg, "truncated") {
		t.Fatal("truncated diagnostic does not say it was truncated")
	}
}

func TestRedactCredentialShapes(t *testing.T) {
	pemBody := strings.Repeat("Q", 40)
	tokens := []string{
		syntheticCredential("sk-", 'A', 32),
		syntheticCredential("sk-proj-", 'A', 32),
		syntheticCredential("AKIA", 'B', 16),
		syntheticCredential("ghp_", 'C', 36),
		syntheticCredential("gho_", 'C', 36),
		syntheticCredential("ghu_", 'C', 36),
		syntheticCredential("ghs_", 'C', 36),
		syntheticCredential("ghr_", 'C', 36),
		"-----BEG" + "IN OPENSSH PRIVATE KEY----- " + pemBody + " -----END OPENSSH PRIVATE KEY-----",
		"-----BEG" + "IN PRIVATE KEY----- " + pemBody,
	}

	for i, token := range tokens {
		got := Redact("tool output: " + token + " :end")
		if strings.Contains(got, token) {
			t.Fatalf("token case %d survived redaction", i)
		}

		if strings.Contains(got, pemBody) {
			t.Fatalf("token case %d leaked the private key body", i)
		}

		if !strings.Contains(got, RedactionMarker) {
			t.Fatalf("token case %d produced no redaction marker", i)
		}

		if !strings.Contains(got, "tool output:") {
			t.Fatalf("token case %d destroyed surrounding diagnostic text", i)
		}
	}
}

func TestRedactLeavesOrdinaryDiagnosticsIntact(t *testing.T) {
	in := "doxygen: warning: Tag 'PERL_PATH' at line 10 of file Doxyfile has become obsolete"
	if got := Redact(in); got != in {
		t.Fatalf("ordinary diagnostic was altered: %q", got)
	}
}

func TestRedactBoundsSize(t *testing.T) {
	got := Redact(strings.Repeat("Z", 4*MaxStderrDiagnosticBytes))
	if len(got) > MaxStderrDiagnosticBytes+128 {
		t.Fatalf("Redact returned %d bytes, want at most %d", len(got), MaxStderrDiagnosticBytes+128)
	}
}

// TestDefaultRunnerRefusesInjectionEnvNamesCaseInsensitively closes the variant
// the exact-match denylist left open: Windows resolves environment names
// case-insensitively, and npm_config_node_options / GIT_CONFIG_KEY_n reach the
// same code-execution effect as NODE_OPTIONS and GIT_CONFIG_GLOBAL under other
// names. Each of these must abort the run, not be forwarded or silently dropped.
func TestDefaultRunnerRefusesInjectionEnvNamesCaseInsensitively(t *testing.T) {
	refused := []string{
		"nOdE_oPtIoNs", "Ld_Preload", "ld_preload", "PsModulePath",
		"npm_config_node_options", "NPM_CONFIG_REGISTRY",
		"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0",
		"GIT_PROXY_COMMAND", "GIT_EDITOR", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "JDK_JAVA_OPTIONS",
		"PYTHONHOME", "PATH", "path",
	}

	for _, name := range refused {
		t.Run(name, func(t *testing.T) {
			out, err := newTestRunner().Run(
				context.Background(),
				"/bin/sh",
				[]string{"-c", "env"},
				t.TempDir(),
				map[string]string{name: "injected"},
			)
			if err == nil {
				t.Fatalf("runner accepted injection-capable variable %s", name)
			}

			if out != "" {
				t.Fatalf("refused call still produced %d bytes of output", len(out))
			}

			if !strings.Contains(err.Error(), name) {
				t.Fatalf("refusal does not name the offending variable: %q", err.Error())
			}
		})
	}
}

// TestDefaultRunnerStillAcceptsOrdinaryToolSettings is the positive control: the
// hardened denylist must not refuse the settings extractors legitimately pass.
func TestDefaultRunnerStillAcceptsOrdinaryToolSettings(t *testing.T) {
	env := map[string]string{"JEKYLL_ENV": "production", "TOOL_SETTING": "on"}

	out, err := newTestRunner().Run(context.Background(), "/bin/sh", []string{"-c", "env"}, t.TempDir(), env)
	if err != nil {
		t.Fatalf("runner refused an ordinary tool setting: %v", err)
	}

	for name, value := range env {
		if !strings.Contains(out, name+"="+value) {
			t.Fatalf("ordinary setting %s was not forwarded", name)
		}
	}
}
