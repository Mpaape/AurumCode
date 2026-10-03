package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

// builtinProfilePromptDigest is the sha256 of every built-in profile's
// signature and prompt prefix, in Names() order, measured on the tree before
// the built-ins moved from Go literals to embedded YAML. The same bytes must
// reach the model whichever form holds the profiles.
const builtinProfilePromptDigest = "f7c22d8d6452d8b8de3c36ccf8e9844aa19741156373570d7e02f3ff1841f2bd"

func TestAUR584BuiltinProfilesKeepThePromptDigest(t *testing.T) {
	h := sha256.New()
	names := reviewprofile.Names()
	if len(names) != 4 {
		t.Fatalf("built-in profiles = %v, want the four shipped ones", names)
	}
	for _, name := range names {
		p, ok := reviewprofile.Builtin(name)
		if !ok {
			t.Fatalf("built-in %q not found", name)
		}
		h.Write([]byte(p.Signature() + "\x00" + profileProvider{profile: p}.prefix() + "\x00"))
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != builtinProfilePromptDigest {
		t.Fatalf("built-in profile prompt digest = %s, want %s (names %s)", got, builtinProfilePromptDigest, strings.Join(names, ","))
	}
}
