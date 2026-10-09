package redaction

import (
	"strings"
	"testing"
)

// aur609Expressions are the seven spellings of a secret-bearing key whose
// value is code, not a secret: before AUR-609 the filter replaced (part of)
// each of them with the marker and handed the model a broken line.
var aur609Expressions = []struct {
	name, line string
}{
	{"python call", `API_KEY = os.environ.get("API_KEY")`},
	{"go short declaration of a call", `token := os.Getenv("TOKEN")`},
	{"go comparison", `if token == nil {`},
	{"javascript strict comparison", `if (password === undefined) {`},
	{"identifier in a struct literal", `p := Profile{APIKey: key}`},
	{"member access", `const secret = this.config.secret;`},
	{"workflow secret reference", `LLM_API_KEY: ${{ secrets.LLM_API_KEY }}`},
}

// AC-001: expressions, identifiers and operators pass through intact.
func TestAUR609KeyValueKeepsExpressionsAndIdentifiers(t *testing.T) {
	f := NewFilter()
	for _, tc := range aur609Expressions {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.Redact(tc.line); got != tc.line {
				t.Fatalf("Redact(%q) = %q, want the line intact", tc.line, got)
			}
		})
	}
}

// AC-002: a quoted literal, a generated token, a JWT and an AWS key id stay
// masked. Every secret-shaped value is assembled at run time so no literal
// credential is committed.
func TestAUR609KeyValueStillMasksLiteralsAndTokens(t *testing.T) {
	f := NewFilter()
	tokenLive := "tok_live_" + "9f8e7d6c5b4a"
	generated := "Zx9Qw3Lp" + "7Rt5Vn2K"
	jwt := "eyJ" + "hbGciOiJIUzI1NiJ9" + ".eyJ" + "zdWIiOiIxMjM0NTY3ODkwIn0" + "." + "c2lnbmF0dXJlMDEyMzQ"
	akia := "AKIA" + "QX7Z" + "M4P8" + "K2N6" + "R9T3"
	cases := []struct{ name, in, want string }{
		{"double-quoted literal", `password = "hunter2"`, `password = "` + Marker + `"`},
		{"literal after go short declaration", `password := "hunter2"`, `password := "` + Marker + `"`},
		{"single-quoted literal", `secret: 'hunter2'`, `secret: '` + Marker + `'`},
		{"bare env token", "DEMO_API_TOKEN=" + tokenLive, "DEMO_API_TOKEN=" + Marker},
		{"bare generated token", "api_key: " + generated, "api_key: " + Marker},
		{"bare jwt", "token=" + jwt, "token=" + Marker},
		{"bare aws key id", "aws_access_key=" + akia, "aws_access_key=" + Marker},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.Redact(tc.in); got != tc.want {
				t.Fatalf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	// The generated token is no known credential shape, so only the
	// key/value rule can mask it: the shape rule alone must leave it.
	if got := redactCredentialShapes(generated); got != generated {
		t.Fatalf("the generated token %q is a known shape (%q); it proves nothing about the key/value rule", generated, got)
	}
}

// The `=` of a comparison and the `=` of Go's `:=` are operators, never the
// first byte of a value.
func TestAUR609AssignDoesNotEatComparisonOperators(t *testing.T) {
	f := NewFilter()
	for _, line := range []string{
		`if token == nil {`,
		`if token==expected {`,
		`return password === other`,
		`ok := secret != ""`,
		`token := os.Getenv("TOKEN")`,
	} {
		got := f.Redact(line)
		if got != line {
			t.Fatalf("Redact(%q) = %q, want the line intact", line, got)
		}
	}
	if m := reKVBare.FindString(`if token == nil {`); m != "" {
		t.Fatalf("the bare rule matched %q inside a comparison", m)
	}
	if m := reKVBare.FindString(`token := value`); m != "token := value" {
		t.Fatalf("the bare rule matched %q, want the whole `:=` separator taken", m)
	}
}

// secretShaped classifies a bare value: a known shape or a generated token
// is a secret; an identifier, a word with a digit or an expression is not.
func TestAUR609SecretShaped(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"a1b2c3d4", true}, // eight distinct characters: exactly 3.0 bits
		{"a1b2c3d", false}, // seven characters cannot reach 3.0 bits
		{"aabbcc11", false},
		{"abcdefghij", false},
		{"1234567890", false},
		{"hunter2", false},
		{"key", false},
		{"nil", false},
		{"os.environ.get(", false},
		{"os.Getenv(", false},
		{"${{", false},
		{"$TOKEN_VALUE_1", false},
		{"cfg->token_v2x9", false},
		{"Config::TOKEN_V2x9", false},
		{"Zx9Qw3Lp" + "7Rt5Vn2K", true},
		{"tok_live_" + "9f8e7d6c5b4a", true},
		{"AKIA" + strings.Repeat("Q", 16), true},
		{"eyJ" + "hbGciOiJIUzI1NiJ9" + ".eyJ" + "zdWIiOiIxMjM0NTY3ODkwIn0" + ".c2ln", true},
	}
	for _, tc := range cases {
		if got := secretShaped(tc.value); got != tc.want {
			t.Fatalf("secretShaped(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
	if h := shannonEntropy("a1b2c3d4"); h != 3.0 {
		t.Fatalf("shannonEntropy(a1b2c3d4) = %v, want exactly 3.0", h)
	}
}
