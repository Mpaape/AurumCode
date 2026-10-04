package scanner

import (
	"fmt"
	"strconv"
	"strings"
)

// LookupEnv reads one variable of the reviewing process, as os.LookupEnv.
type LookupEnv func(name string) (string, bool)

// BaseEnvironment names the only variables of the reviewing process every
// engine's child process receives: where to find binaries, a home and a
// temporary directory, the locale and the system trust store. Everything
// else the review holds (the model's API key, GITHUB_TOKEN, a publication
// token, any other secret of the CI job) never reaches the child, because a
// scanner runs over content the pull request's author controls.
var BaseEnvironment = []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR"}

// Environment is what one engine adds to BaseEnvironment: variable names
// copied from the reviewing process when set, and fixed NAME=value entries
// that always win over the reviewing process.
type Environment struct {
	Pass  []string
	Fixed []string
	// Extra derives further entries from the reviewing process (for
	// instance only the safe.directory entries of GIT_CONFIG_*).
	Extra func(LookupEnv) []string
}

// ChildEnvironment is the explicit environment of an engine's child
// process. It is never nil, so exec never falls back to inheriting the
// reviewing process's whole environment.
func ChildEnvironment(lookup LookupEnv, extra Environment) []string {
	env := make([]string, 0, len(BaseEnvironment)+len(extra.Pass)+len(extra.Fixed))
	fixed := map[string]bool{}
	for _, entry := range extra.Fixed {
		fixed[strings.SplitN(entry, "=", 2)[0]] = true
	}
	for _, names := range [][]string{BaseEnvironment, extra.Pass} {
		for _, name := range names {
			if value, ok := lookup(name); ok && !fixed[name] {
				env = append(env, name+"="+value)
			}
		}
	}
	env = append(env, extra.Fixed...)
	if extra.Extra != nil {
		env = append(env, extra.Extra(lookup)...)
	}
	return env
}

// gitSafeDirectoryKey is the one git configuration key a child may receive
// through GIT_CONFIG_*: the CI job uses it to let git read a checkout owned
// by another user. Any other key (http.extraheader carries a credential) is
// dropped.
const gitSafeDirectoryKey = "safe.directory"

// GitSafeDirectories keeps, from the GIT_CONFIG_COUNT/KEY_n/VALUE_n entries
// of the reviewing process, only the safe.directory ones, renumbered.
func GitSafeDirectories(lookup LookupEnv) []string {
	raw, ok := lookup("GIT_CONFIG_COUNT")
	if !ok {
		return nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count <= 0 {
		return nil
	}
	var env []string
	kept := 0
	for i := 0; i < count; i++ {
		key, _ := lookup(fmt.Sprintf("GIT_CONFIG_KEY_%d", i))
		value, ok := lookup(fmt.Sprintf("GIT_CONFIG_VALUE_%d", i))
		if !ok || !strings.EqualFold(strings.TrimSpace(key), gitSafeDirectoryKey) {
			continue
		}
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", kept, gitSafeDirectoryKey), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", kept, value))
		kept++
	}
	if kept == 0 {
		return nil
	}
	return append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", kept))
}
