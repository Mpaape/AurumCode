package redaction

import (
	"encoding/base64"
	"strings"
	"testing"
)

// aur609Expressions are the seven spellings of a secret-bearing key whose
// value is code or an operator, not a secret: before AUR-609 the filter
// replaced (part of) each of them with the marker and handed the model a
// broken line.
var aur609Expressions = []struct {
	name, line string
}{
	{"python call", `API_KEY = os.environ.get("API_KEY")`},
	{"go short declaration of a call", `token := os.Getenv("TOKEN")`},
	{"go comparison", `if token == nil {`},
	{"javascript strict comparison", `if (password === undefined) {`},
	{"method call in a struct literal", `p := Profile{APIKey: cfg.APIKey()}`},
	{"chained method call", `const secret = this.config.getSecret();`},
	{"workflow secret reference", `LLM_API_KEY: ${{ secrets.LLM_API_KEY }}`},
}

// AC-001: expressions and operators pass through intact.
func TestAUR609KeyValueKeepsExpressionsAndOperators(t *testing.T) {
	f := NewFilter()
	for _, tc := range aur609Expressions {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.Redact(tc.line); got != tc.line {
				t.Fatalf("Redact(%q) = %q, want the line intact", tc.line, got)
			}
		})
	}
}

// AC-002: a quoted literal, a bare token, a JWT and an AWS key id stay
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

// AC-002: a weak secret written bare is still a secret and never reaches
// the model provider; so is a bare identifier after a secret key.
func TestAUR609BareWeakSecretStaysMasked(t *testing.T) {
	f := NewFilter()
	for _, tc := range []struct{ in, want string }{
		{"password=hunter2", "password=" + Marker},
		{"password: supersecretvalue", "password: " + Marker},
		{"DB_PASSWORD=changeme", "DB_PASSWORD=" + Marker},
		{"token = tok", "token = " + Marker},
		{"secret:x", "secret:" + Marker},
	} {
		if got := f.Redact(tc.in); got != tc.want {
			t.Fatalf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// discordShaped assembles a Discord-shaped token at run time: the base64 of
// a numeric id, a 6-character middle and a 27-character tail.
func discordShaped() string {
	return base64.RawURLEncoding.EncodeToString([]byte("198622483471925248")) +
		"." + "Cl2F" + "MQ" + "." + "ZnCjm1XVW7vR" + "ze4b7Cq4se7kKWs"
}

// A bare value after a secret key is masked unless the whole expression is
// code. Each masked case below leaked between AUR-609's first rule (one code
// byte anywhere made the value code) and the anchored grammar; every
// secret-shaped value is assembled at run time.
func TestAUR609BareSecretOrCode(t *testing.T) {
	f := NewFilter()
	masked := []struct{ name, key, value, secret string }{
		{"dot inside a password", "DB_PASSWORD=", "Tr0ub4dor" + ".9xQ", "Tr0ub4dor"},
		{"parenthesis inside a password", "secret: ", "p@ss" + "(1)", "p@ss"},
		{"dollar inside a token", "token: ", "abc" + "$def", "abc"},
		{"bracketed password", "password: ", "[hunter" + "2]", "hunter2"},
		{"braced api key", "api_key = ", "{abc123" + "xyz}", "abc123xyz"},
		{"trailing dot", "password=", "s3cr3t" + ".", "s3cr3t"},
		{"leading dot", "password=", "." + "s3cr3t", "s3cr3t"},
		{"equals then member chain", "secret=", "abc=" + "def.ghi", "abc=def"},
		{"member-chain shaped password", "password=", "abc" + ".def123", "def123"},
		{"member-chain shaped client secret", "client_secret=", "ab" + ".cd", "ab.cd"},
		{"call shape with a trailing byte", "password=", "Passw0rd" + "(1)x", "Passw0rd"},
		{"variable with a literal default", "password: ", "${DB_PASS:-" + "hunter2}", "hunter2"},
		{"sendgrid", "api_key=", "SG." + "aB3dE5fG7hJ9kL1mN3pQ5r" + "." + "xY2zW4vU6tS8rQ0pO1nM3lK5jI7hG9fE2dC4bA6zY8x", "aB3dE5fG7hJ9kL1mN3pQ5r"},
		{"discord", "token=", discordShaped(), "Cl2FMQ"},
		{"slack", "token=", "xox" + "b-" + "123456789012-1234567890123-" + "abcdEFGHijklMNOP" + ".mnop", "abcdEFGHijklMNOP"},
		{"aws secret with slash and dot", "access_key=", "wJalrXUtnFEMI" + "/K7MDENG." + "bPxRfiCYzLmQ2Kv", "K7MDENG"},
		{"der key body with a dot", "PRIVATE_KEY=", "MIIEvQIBADANBgkq" + "." + "hkiG9w0BAQEFAASC", "hkiG9w0"},
		{"weak password inside a call", "f(password=", "hunter" + "2)", "hunter2"},
	}
	for _, tc := range masked {
		t.Run("masked/"+tc.name, func(t *testing.T) {
			got := f.Redact(tc.key + tc.value)
			if strings.Contains(got, tc.secret) || !strings.Contains(got, Marker) {
				t.Fatalf("Redact(%q) = %q, want %q masked", tc.key+tc.value, got, tc.secret)
			}
		})
	}
	if got := f.Redact("f(password=hunter" + "2)"); got != "f(password="+Marker+")" {
		t.Fatalf("the closer of the enclosing call was masked: %q", got)
	}
	for _, line := range []string{
		`token = getToken()`,
		`API_KEY = os.getenv("API_KEY").strip()`,
		`token = cfg->token()`,
		`token = Config::token()`,
		`password: $DB_PASSWORD`,
		`password: ${DB_PASSWORD}`,
		`token=$(cat /run/secrets/token)`,
		`connect(password=cfg.dbPassword())`,
	} {
		t.Run("code/"+line, func(t *testing.T) {
			if got := f.Redact(line); got != line {
				t.Fatalf("Redact(%q) = %q, want the line intact", line, got)
			}
		})
	}
}

// A member chain or an identifier after a secret key is masked whole; the
// line stays valid code, with no dangling fragment and no operator eaten.
func TestAUR609MemberChainMaskedWhole(t *testing.T) {
	f := NewFilter()
	for in, want := range map[string]string{
		`apiKey = cfg.APIKey`:                "apiKey = " + Marker,
		`p := Profile{APIKey: cfg.Key}`:      "p := Profile{APIKey: " + Marker + "}",
		`const secret = this.config.secret;`: "const secret = " + Marker + ";",
		`connect(password=cfg.db_password)`:  "connect(password=" + Marker + ")",
	} {
		if got := f.Redact(in); got != want {
			t.Fatalf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
