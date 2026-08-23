#!/usr/bin/env bash
#
# E2E program for card AUR-463 (selector E2EAUR463).
#
# WHAT THIS PROVES
#
#   A small, realistic multi-file ESM tree (several .mjs modules, mirroring
#   the "ALVO REAL" Node ESM POC the card names) run through
#   javascript.NativeExtractor.Extract as a single package boundary to
#   boundary call produces real Markdown pages: real exported function,
#   class-with-methods, const-function, and default-export symbols, each
#   with its real JSDoc when present, and NO synthesized prose when absent.
#
#   SCOPE NOTE: this exercises the package's own public API end to end
#   (SourceDir -> OutputDir), not the `aurumcode docs` CLI: see
#   docs/specs/AUR-463.md "Limitacao de escopo medida" for why the CLI path
#   is out of this card's `paths`.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   69 = inconclusive / infrastructure
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-463'
selector="${1:-E2EAUR463}"

case "$selector" in
  E2EAUR463) ;;
  *) printf '%s/E2E/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

fail() { printf '%s/E2E/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/E2E/infrastructure/%s\n' "$card" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e463.XXXXXX")" || infra mktemp
cleanup() { chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM HUP

src="$run_dir/src"
out="$run_dir/out"
mkdir -p "$src" "$out"

cat >"$src/math.mjs" <<'EOF'
/**
 * Adds two numbers.
 */
export function add(a, b) {
  return a + b;
}

export function internalHelperLookingPublic(x) {
  return x * 2;
}
EOF

cat >"$src/service.mjs" <<'EOF'
/**
 * A small request service.
 */
export class RequestService {
  /**
   * Creates a service bound to a base URL.
   */
  constructor(baseUrl) {
    this.baseUrl = baseUrl;
  }

  fetchOne(id) {
    return `${this.baseUrl}/${id}`;
  }
}

/**
 * Formats a duration in milliseconds as seconds.
 */
export const formatDuration = (ms) => `${ms / 1000}s`;

/**
 * Entry point.
 */
export default function run() {
  return new RequestService("https://example.test");
}
EOF

cat >"$src/no_exports.mjs" <<'EOF'
// nothing exported here
function privateHelper() { return 1; }
const x = privateHelper();
EOF

# The probe program must import the internal extractors package, so it must
# live inside the module tree rooted at repo_root (Go's internal-import rule
# is keyed on the importer's own path under that root, not on cwd). It is
# written under repo_root as a throwaway, hidden directory and removed by the
# trap below; when this script runs through tests/acceptance/AUR-463.sh's
# e2e_case, repo_root is already an isolated, scratch, mktemp'd copy of the
# repository, so this write touches nothing durable either way.
gopath_probe="$repo_root/tests/e2e/.aur463_e2e_probe_tmp"
rm -rf -- "$gopath_probe"
mkdir -p "$gopath_probe"
trap 'cleanup; rm -rf -- "'"$gopath_probe"'" >/dev/null 2>&1 || true' EXIT INT TERM HUP
cat >"$gopath_probe/main.go" <<'EOF'
package main

import (
	"context"
	"fmt"
	"os"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	javascript "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
)

func main() {
	n := javascript.NewNativeExtractor()
	res, err := n.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageJavaScript,
		SourceDir: os.Args[1],
		OutputDir: os.Args[2],
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "EXTRACT_ERROR:", err)
		os.Exit(1)
	}
	if len(res.Errors) != 0 {
		fmt.Fprintln(os.Stderr, "EXTRACT_ERRORS:", res.Errors)
		os.Exit(1)
	}
	for _, f := range res.Files {
		fmt.Println(f)
	}
}
EOF

host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache"
export HOME="$run_dir/home"
mkdir -p "$GOCACHE" "$GOTMPDIR" "$HOME"

files_out="$run_dir/generated.txt"
set +e
( cd "$repo_root" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    go run "$gopath_probe/main.go" "$src" "$out" ) >"$files_out" 2>"$run_dir/run.err"
rc=$?
set -e
((rc == 0)) || { cat "$run_dir/run.err" >&2; fail extract-run-failed; }

generated_count="$(grep -c . "$files_out" || true)"
((generated_count == 2)) || fail "wrong-page-count:$generated_count"

math_page="$out/math.md"
service_page="$out/service.md"
[[ -s "$math_page" ]] || fail missing-math-page
[[ -s "$service_page" ]] || fail missing-service-page
[[ ! -e "$out/no_exports.md" ]] || fail empty-file-produced-a-page

grep -Fq 'add(a, b)' "$math_page" || fail missing-add-signature
grep -Fq 'Adds two numbers.' "$math_page" || fail missing-add-jsdoc
grep -Fq 'internalHelperLookingPublic(x)' "$math_page" || fail missing-undocumented-signature

grep -Fq 'RequestService' "$service_page" || fail missing-class-signature
grep -Fq 'A small request service.' "$service_page" || fail missing-class-jsdoc
grep -Fq 'fetchOne(id)' "$service_page" || fail missing-method-signature
grep -Fq 'formatDuration' "$service_page" || fail missing-const-signature
grep -Fq 'run()' "$service_page" || fail missing-default-signature

printf '%s/E2E/ok\n' "$card"
