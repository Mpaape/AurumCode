#!/usr/bin/env bash
# E2E check for AUR-486: build (or reuse) the real aurumcode binary and run
# it, as a user would, against a bare Git repository this script writes into
# a temp directory at runtime.
#
# WHY A RUNTIME-GENERATED REPOSITORY, NOT A COMMITTED FIXTURE
#
#   The sealed acceptance profile (bootstrap-readonly-v1) carries bash and a
#   Go toolchain but no `git` binary, and this card's `paths` do not include
#   tests/fixtures. So the generator below writes the exact loose-object
#   format tests/fixtures/repos/git-demo/build-fixture.sh emits (zlib +
#   "type size\0content" headers, a tree, two commits, refs/heads/main) --
#   the same shape internal/analyzer's pure-Go reader inflates. Nothing here
#   shells out to `git`.
#
# WHAT THIS PROVES, DISTINCT FROM tests/unit/AUR-486.go (package-boundary,
# synthetic types.Diff) AND tests/integration/AUR-486.go (CLI-boundary, Go
# test):
#
#   The same four command-injection idioms (Go `exec.Command("sh","-c",...)`,
#   C# `Process.Start(...)` with shell, PowerShell `Invoke-Expression`, bash
#   `eval`/`sh -c`) and the SQL-by-shell-interpolation shape all appear at
#   the full-process boundary with exact citation counts (AC-001); the
#   card-named safe forms do not (AC-002, proven by the exact total count);
#   the run is deterministic; and a repository carrying only these six real
#   findings closes the `--fail-on high` gate with exit 3.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-486
selector="${1:-E2EAUR486}"
[[ "$selector" == "E2EAUR486" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a486.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS='-mod=mod -p=1' go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# --- provider-free bare-repository generator (stdlib only) -----------------
gen_dir="$run_dir/gen"
mkdir -p "$gen_dir"
cat >"$gen_dir/go.mod" <<'EOF'
module aurum486gen

go 1.21
EOF
cat >"$gen_dir/main.go" <<'EOF'
package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func writeObject(gitDir, typ string, content []byte) (string, error) {
	full := append([]byte(fmt.Sprintf("%s %d\x00", typ, len(content))), content...)
	sum := sha1.Sum(full)
	id := hex.EncodeToString(sum[:])
	dir := filepath.Join(gitDir, "objects", id[:2])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(full); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return id, os.WriteFile(filepath.Join(dir, id[2:]), buf.Bytes(), 0o644)
}

func tree(gitDir string, files map[string]string) (string, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	for _, name := range names {
		id, err := writeObject(gitDir, "blob", []byte(files[name]))
		if err != nil {
			return "", err
		}
		raw, err := hex.DecodeString(id)
		if err != nil {
			return "", err
		}
		buf.WriteString("100644 " + name + "\x00")
		buf.Write(raw)
	}
	return writeObject(gitDir, "tree", buf.Bytes())
}

func commit(gitDir, treeID, parent, msg string, epoch int64) (string, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "tree %s\n", treeID)
	if parent != "" {
		fmt.Fprintf(&buf, "parent %s\n", parent)
	}
	fmt.Fprintf(&buf, "author Aurum Test <test@aurum.invalid> %d +0000\n", epoch)
	fmt.Fprintf(&buf, "committer Aurum Test <test@aurum.invalid> %d +0000\n", epoch)
	fmt.Fprintf(&buf, "\n%s\n", msg)
	return writeObject(gitDir, "commit", buf.Bytes())
}

func main() {
	gitDir := os.Args[1]
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		panic(err)
	}
	seed, err := tree(gitDir, map[string]string{"README.md": "AUR-486 ephemeral fixture. Nothing here is a real application.\n"})
	if err != nil {
		panic(err)
	}
	c1, err := commit(gitDir, seed, "", "seed: add the fixture skeleton", 1700000000)
	if err != nil {
		panic(err)
	}
	files := map[string]string{
		"main.go":    "package main\n\nimport \"os/exec\"\n\nfunc pingUnsafe(host string) {\n\texec.Command(\"sh\", \"-c\", \"ping \"+host).Run()\n}\n\nfunc pingSafe(host string) {\n\texec.Command(\"ping\", host).Run()\n}\n",
		"App.cs":     "using System.Diagnostics;\n\nclass App {\n    static void Ping(string host) {\n        Process.Start(\"cmd.exe\", \"/c ping \" + host);\n        Process.Start(\"ping\", host);\n    }\n}\n",
		"script.ps1": "function Invoke-Ping($host) {\n    Invoke-Expression \"ping $host\"\n}\nGet-Process\n",
		"deploy.sh":  "#!/bin/bash\nset -euo pipefail\nping_unsafe() {\n    eval \"ping $1\"\n}\nping_shell_unsafe() {\n    sh -c \"ping $1\"\n}\nping_safe() {\n    ping \"$1\"\n}\n# eval \"ping $1\" in a comment must not fire\necho \"the literal eval \\\"ping $1\\\" must not fire\"\nquery=\"SELECT * FROM users WHERE name = '$1'\"\n",
	}
	head, err := tree(gitDir, files)
	if err != nil {
		panic(err)
	}
	c2, err := commit(gitDir, head, c1, "chore: plant go csharp powershell bash shapes", 1700000060)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte(c2+"\n"), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		panic(err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n\tfilemode = false\n\tbare = true\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		panic(err)
	}
}
EOF

repo="$run_dir/repo.git"
if ! (cd "$gen_dir" && GOFLAGS='-mod=mod -p=1' go run . "$repo") >"$run_dir/gen.log" 2>&1; then
  cat "$run_dir/gen.log" >&2
  fail fixture_generation_failed
fi
test -d "$repo" || fail fixture_missing

header='Security findings (standards/security-review):'
cmd_citation='(rule security/command-injection: Command Injection)'
sql_citation='(rule security/sql-injection: SQL Injection Vulnerability)'

out_sec="$(cd "$repo" && "$bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing
after_header="${out_sec#*"$header"}"

# AC-001: the five command-injection shapes and the SQL-by-shell shape.
for ln in 6; do
  grep -Fq "main.go:$ln: [error]" <<<"$after_header" || fail "true-positive-go-line-$ln-not-found"
done
grep -Fq 'App.cs:5: [error]' <<<"$after_header" || fail true-positive-csharp-not-found
grep -Fq 'script.ps1:2: [error]' <<<"$after_header" || fail true-positive-powershell-not-found
grep -Fq 'deploy.sh:4: [error]' <<<"$after_header" || fail true-positive-bash-eval-not-found
grep -Fq 'deploy.sh:7: [error]' <<<"$after_header" || fail true-positive-bash-shc-not-found
grep -Fq 'deploy.sh:14: [error]' <<<"$after_header" || fail true-positive-sql-not-found

# AC-002: exactly six findings -- no benign line ever appears.
[[ "$(grep -Fo "$cmd_citation" <<<"$after_header" | wc -l)" -eq 5 ]] || fail unexpected-cmd-count
[[ "$(grep -Fo "$sql_citation" <<<"$after_header" | wc -l)" -eq 1 ]] || fail unexpected-sql-count
[[ "$(grep -Fo '[error]' <<<"$after_header" | wc -l)" -eq 6 ]] || fail unexpected-total-count

# Determinism.
out_again="$(cd "$repo" && "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic

# The six real findings close --fail-on high.
set +e
(cd "$repo" && "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# Without --seguranca: no security section leaks in.
out_base="$(cd "$repo" && "$bin" review --base HEAD~1 2>/dev/null || true)"
if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi

printf '%s/AC-001/e2e-ok\n' "$card"
