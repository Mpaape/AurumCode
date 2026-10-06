// Package gittest builds the environment a test hands to a real git binary
// when it assembles a repository fixture.
//
// A fixture built with os.Environ() alone inherits whatever global or system
// git configuration the machine running the test happens to carry: a shared
// development container keeps HOME=/tmp alive across sessions, so a single
// `git config --global` (commit signing, a hooks path, a gc or pack policy)
// left there by anyone changes what `git commit`, `git gc` or `git repack`
// produce, and the test then fails in that container while passing in CI.
// HermeticEnv removes that dependency: the fixture sees no global and no
// system configuration, a private HOME and no interactive prompt.
package gittest

import "os"

// Identity is the author and committer every fixture commit carries.
var Identity = []string{
	"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
	"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
}

// HermeticEnv returns the process environment with git isolated from any
// ambient configuration: GIT_CONFIG_GLOBAL points to the null device,
// GIT_CONFIG_NOSYSTEM drops the system file, HOME is home (a directory the
// caller owns, normally t.TempDir()) and prompts are disabled. Later entries
// win in os/exec, so these override whatever the caller's environment has.
func HermeticEnv(home string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+home,
		"XDG_CONFIG_HOME="+home,
		"GIT_TERMINAL_PROMPT=0",
	)
	return append(env, Identity...)
}
