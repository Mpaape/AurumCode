package version

import "testing"

// A stamped tag or short SHA is a real version, published as stamped.
func TestAUR611StampedVersionIsNotDev(t *testing.T) {
	for stamped, want := range map[string]string{
		"v9.9.9":          "v9.9.9",
		" 0123456789ab\n": "0123456789ab",
		"v2.0.0-rc.1":     "v2.0.0-rc.1",
	} {
		info := New(stamped)
		if info.IsDev() {
			t.Fatalf("New(%q).IsDev() = true, want a stamped version", stamped)
		}
		if got := info.Label(); got != want {
			t.Fatalf("New(%q).Label() = %q, want %q", stamped, got, want)
		}
	}
}

// A build that stamped nothing, or stamped dev or a blank value, is dev and
// is labeled dev, never an empty version.
func TestAUR611UnstampedVersionIsDev(t *testing.T) {
	for _, info := range []Info{{}, New(""), New("   "), New("dev"), New(" dev ")} {
		if !info.IsDev() {
			t.Fatalf("%+v.IsDev() = false, want dev", info)
		}
		if got := info.Label(); got != Dev {
			t.Fatalf("%+v.Label() = %q, want %q", info, got, Dev)
		}
	}
}
