#!/usr/bin/env bash
# AUR-429 E2E: the card's declared command at the raw process boundary.
#
# `aurumcode docs verify --url https://usuario.github.io/projeto` from AC-001
# is realized here the same way AUR-425 realized `aurumcode docs`: no
# `aurumcode docs` subcommand exists in this repository (cmd/aurumcode
# publishes `review`, owned by AUR-430/431, and is outside this card's
# paths), so the declared command's only real shape is the verifier binary
# this card ships: internal/qa/browserproof/docsverify. The sandbox denies
# network, so the verification serves the built site on loopback and
# navigates it with the offline scripted driver — exactly the instrument the
# card and AUR-428's acceptance name for this proof; --url names the
# published location the verdict is about and is recorded in the output.
#
# What this selector proves, per AC-001:
#   1. Generate the real site from tests/fixtures/docs/goproject with the
#      real cmd/regenerate-docs binary.
#   2. `docsverify --url ... --docs <dir> --content 'func NewGreeting'`
#      exits 0 and prints a proved DocsVerifyResultV1: home page opened,
#      one index link followed (/go/root), expected content confirmed.
#   3. Repeating the run over the same input produces the same output
#      (navigation timing fields are normalized before comparison).
#   4. A site whose symbol page lost the expected content is REFUSED: exit 1,
#      proved:false, BROWSERPROOF_TEXT_MISMATCH. If that run ever exits 0 the
#      card's MUT-001 came back: this script prints AUR-429/AC-001/MUT-001
#      and fails.
#   5. A site whose index lost its links is REFUSED with
#      BROWSERPROOF_UNREACHABLE_ROUTE (the "follows a link" half).
#   6. The AURUM_SECRET_CANARY value handed to both binaries never appears
#      in any output, and misuse (no --url) exits 64.
#
# Missing compile closure or toolchain is infrastructure: exit 69, never a
# pass and never a behavioral failure. An absent docsverify package is the
# card's RED: the promised command does not exist, reported as
# behavior-missing (exit 1), mirroring AUR-428's absent-LICENSE RED.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR429}" == E2EAUR429 ]] || { printf 'AUR-429/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-429/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-429/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# The generator's compile closure; missing pieces are infrastructure.
for input in \
  go.mod go.sum \
  cmd/regenerate-docs/main.go \
  internal/pipeline \
  internal/documentation/extractors \
  internal/documentation/site \
  internal/llm \
  internal/qa/browserproof \
  tests/fixtures/docs/goproject/greeting.go
do
  [[ -e "$repo_root/$input" ]] || infra "closure_not_materialized:$input"
done

# The promised verifier is this card's behavior, not its infrastructure: its
# absence is the RED the card's TDD clause names.
[[ -f "$repo_root/internal/qa/browserproof/docsverify/main.go" ]] \
  || fail 'behavior-missing:internal/qa/browserproof/docsverify is not delivered'

# Resolved BEFORE HOME is redirected: the offline build of the generator
# needs yaml.v3 from the already-populated module cache.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e429.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp" "$run_dir/home" "$run_dir/emptypath" "$run_dir/work"

gocache="${GOCACHE:-$run_dir/cache}"
mkdir -p "$gocache"

build() {
  local out="$1" pkg="$2"
  local log="$run_dir/build-${out##*/}.log"
  if ! ( ulimit -v 8388608
    cd "$repo_root" && \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
    go build -o "$out" "$pkg" ) >"$log" 2>&1; then
    cat "$log" >&2
    infra "build_failed:${pkg}"
  fi
}

build "$run_dir/regenerate-docs" ./cmd/regenerate-docs
build "$run_dir/docsverify" ./internal/qa/browserproof/docsverify

readonly canary='AUR429-E2E-CANARY-4c9d'
readonly url='https://usuario.github.io/projeto'

# 1. Generate the real site from the repository fixture.
docs="$run_dir/docs"
gen_rc=0
( cd "$run_dir/work" && \
  env -i \
    PATH="$run_dir/emptypath" \
    HOME="$run_dir/home" \
    AURUMCODE_SOURCE_DIR="$repo_root/tests/fixtures/docs/goproject" \
    AURUMCODE_OUTPUT_DIR="$docs" \
    AURUM_SECRET_CANARY="$canary" \
    "$run_dir/regenerate-docs" ) >"$run_dir/generate.log" 2>&1 || gen_rc=$?
if (( gen_rc != 0 )); then
  cat "$run_dir/generate.log" >&2
  infra "generator_failed:$gen_rc"
fi
[[ -f "$docs/index.md" && -f "$docs/go/root.md" ]] || infra generator_pages_missing

# run_verify <docs-dir> <stdout-file> <stderr-file>; returns the raw exit code.
run_verify() {
  local docs_dir="$1" out="$2" err="$3" rc=0
  ( cd "$run_dir/work" && \
    env -i \
      PATH="$run_dir/emptypath" \
      HOME="$run_dir/home" \
      TMPDIR="$run_dir/work" \
      AURUM_SECRET_CANARY="$canary" \
      "$run_dir/docsverify" \
        --url "$url" \
        --docs "$docs_dir" \
        --content 'func NewGreeting' \
        --content-selector h4 ) >"$out" 2>"$err" || rc=$?
  return "$rc"
}

# 2. Nominal: the published site opens, navigates and shows the content.
rc1=0; run_verify "$docs" "$run_dir/out1.json" "$run_dir/err1.log" || rc1=$?
if (( rc1 != 0 )); then
  cat "$run_dir/out1.json" "$run_dir/err1.log" >&2
  fail "behavior-missing:nominal-exit:$rc1"
fi
grep -Fq '"schema":"DocsVerifyResultV1"' "$run_dir/out1.json" || { cat "$run_dir/out1.json" >&2; fail schema-missing; }
grep -Fq '"proved":true' "$run_dir/out1.json" || { cat "$run_dir/out1.json" >&2; fail verdict-not-proved; }
grep -Fq '"outcome":"proved"' "$run_dir/out1.json" || { cat "$run_dir/out1.json" >&2; fail outcome-not-proved; }
grep -Fq "\"published_url\":\"$url\"" "$run_dir/out1.json" || { cat "$run_dir/out1.json" >&2; fail url-not-recorded; }
grep -Fq '"followed_route":"/go/root"' "$run_dir/out1.json" || { cat "$run_dir/out1.json" >&2; fail link-not-followed; }
grep -Fq '"expected_content":"func NewGreeting"' "$run_dir/out1.json" || { cat "$run_dir/out1.json" >&2; fail content-not-recorded; }

# 3. Determinism: same input, same output, modulo navigation timing.
rc2=0; run_verify "$docs" "$run_dir/out2.json" "$run_dir/err2.log" || rc2=$?
(( rc2 == 0 )) || { cat "$run_dir/err2.log" >&2; fail "repeat-exit:$rc2"; }
normalize() { sed -E 's/"stable_after_ms":[0-9]+/"stable_after_ms":0/g' "$1"; }
if ! diff <(normalize "$run_dir/out1.json") <(normalize "$run_dir/out2.json") >"$run_dir/diff.log"; then
  cat "$run_dir/diff.log" >&2
  fail nondeterministic-output
fi

# 4. Symbol stripped from the generated page: the run must refuse.
docs_broken="$run_dir/docs-sem-conteudo"
cp -R "$docs" "$docs_broken"
grep -Fq 'NewGreeting' "$docs_broken/go/root.md" || infra fixture_lost_symbol
sed -i 's/NewGreeting/Suprimido/g' "$docs_broken/go/root.md"
rc3=0; run_verify "$docs_broken" "$run_dir/out3.json" "$run_dir/err3.log" || rc3=$?
if (( rc3 == 0 )); then
  cat "$run_dir/out3.json" >&2
  printf 'AUR-429/AC-001/behavior-missing\n' >&2
  printf 'AUR-429/AC-001/MUT-001\n' >&2
  fail 'page-without-expected-content-was-accepted'
fi
# This probe is MUT-001's: a mutant that stops demanding the content shows up
# here either as acceptance (above) or as a garbled verdict (below), and both
# register the marker.
if (( rc3 != 1 )); then
  cat "$run_dir/out3.json" "$run_dir/err3.log" >&2
  printf 'AUR-429/AC-001/MUT-001\n' >&2
  fail "wrong-refusal-exit:$rc3"
fi
grep -Fq '"proved":false' "$run_dir/out3.json" || { cat "$run_dir/out3.json" >&2; fail refusal-json-says-proved; }
grep -Fq '"code":"BROWSERPROOF_TEXT_MISMATCH"' "$run_dir/out3.json" \
  || { cat "$run_dir/out3.json" >&2; printf 'AUR-429/AC-001/MUT-001\n' >&2; fail refusal-code-wrong; }

# 5. Index links stripped: nothing to follow, the run must refuse.
docs_unlinked="$run_dir/docs-sem-link"
cp -R "$docs" "$docs_unlinked"
grep -q '^- \[' "$docs_unlinked/index.md" || infra fixture_lost_index_link
sed -i '/^- \[/d' "$docs_unlinked/index.md"
rc4=0; run_verify "$docs_unlinked" "$run_dir/out4.json" "$run_dir/err4.log" || rc4=$?
if (( rc4 == 0 )); then
  cat "$run_dir/out4.json" >&2
  printf 'AUR-429/AC-001/MUT-001\n' >&2
  fail 'index-without-links-was-accepted'
fi
(( rc4 == 1 )) || { cat "$run_dir/out4.json" "$run_dir/err4.log" >&2; fail "wrong-unlinked-exit:$rc4"; }
grep -Fq '"code":"BROWSERPROOF_UNREACHABLE_ROUTE"' "$run_dir/out4.json" \
  || { cat "$run_dir/out4.json" >&2; fail unlinked-code-wrong; }

# 6a. Misuse: a run without --url is usage, exit 64.
rc5=0
( cd "$run_dir/work" && \
  env -i PATH="$run_dir/emptypath" HOME="$run_dir/home" TMPDIR="$run_dir/work" \
    "$run_dir/docsverify" --docs "$docs" --content x ) \
  >"$run_dir/out5.json" 2>"$run_dir/err5.log" || rc5=$?
(( rc5 == 64 )) || fail "usage-exit:$rc5"

# 6b. The canary never reaches any output of any run.
for artifact in generate.log out1.json err1.log out2.json err2.log \
  out3.json err3.log out4.json err4.log out5.json err5.log; do
  if grep -Fq "$canary" "$run_dir/$artifact"; then
    fail "canary-leak:$artifact"
  fi
done

printf 'AUR-429/AC-001/E2EAUR429: pass (proved, deterministic, refusals distinct, canary clean)\n'
