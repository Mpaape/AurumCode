#!/usr/bin/env bash
# AUR-565 acceptance: the review (--base and --pr) reads skills by language and
# alias through the skills catalog, and the skills tutorial shows it.
#
# Selectors:
#   all              AC-001..AC-004, then the mutation
#   AC-001           a `languages: [typescript]` skill reaches the prompt for a
#                    .ts diff and not for a .go diff (captured prompt)
#   AC-002           the alias `ts` selects; an unknown alias is declared in the
#                    context sent to the model and in the review text
#   AC-003           a policy skill with the same selector wins over the
#                    repository's, with a warning
#   AC-004           the regenerated skills tutorial output shows the language
#                    selection reaching the prompt; run.sh --check passes
#   AC-001-MUT-001   ignoring `languages:` and sending every skill turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-565'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for f in cmd/aurumcode/aur565_test.go internal/context/skills/catalog.go demo/tutoriais/skills/run.sh \
         demo/tutoriais/skills/expected/selecao-por-linguagem.txt demo/tutoriais/skills/out/selecao-por-linguagem.log; do
  [[ -f "$repo_root/$f" ]] || infra "missing-source:$f"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a565.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

# prepare_go NAME: a fresh sealed copy of the sources in $run_dir/NAME.
prepare_go() {
  command -v go >/dev/null 2>&1 || infra missing_go
  root="$run_dir/$1"
  mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$root"
  for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$root/$source"; done
  chmod -R u+w -- "$root"
  export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
  export GOFLAGS='-mod=mod -p=1'
  export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
  export GOMEMLIMIT=2GiB GOMAXPROCS=1
}

run_go_test() { # pattern log
  set +e
  (cd "$root" && go test -mod=mod -p 1 -count=1 -timeout 400s -v ./cmd/aurumcode/ -run "$1") >"$2" 2>&1
  local status=$?
  set -e
  cat "$2" >&2
  return $status
}
need_pass() { grep -q "^--- PASS: $2 " "$1" || fail "missing-pass:$2"; }

run_ac001() {
  prepare_go ac001
  local log="$run_dir/ac001.log"
  run_go_test '^TestAUR565LanguageSelectsSkill$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR565LanguageSelectsSkill
}
run_ac002() {
  prepare_go ac002
  local log="$run_dir/ac002.log"
  run_go_test '^TestAUR565AliasResolvesAndUnknownIsDeclared$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR565AliasResolvesAndUnknownIsDeclared
}
run_ac003() {
  prepare_go ac003
  local log="$run_dir/ac003.log"
  run_go_test '^TestAUR565PolicySkillWinsOverRepo$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR565PolicySkillWinsOverRepo
}
run_ac004() {
  local out="$repo_root/demo/tutoriais/skills/out/selecao-por-linguagem.log"
  local doc="$repo_root/docs/tutorials/skills.md"
  bash "$repo_root/demo/tutoriais/skills/run.sh" --check >&2 || fail 'tutorial-check-failed'
  grep -qF 'prompt: Em TypeScript, prefira unknown a any. MARCADOR-SKILL-TS' "$out" || fail 'out-lacks-skill-in-prompt'
  grep -qF 'prompt: a skill estilo-ts NAO chegou' "$out" || fail 'out-lacks-negative-case'
  grep -qF 'unknown language "linguagem-inexistente"' "$out" || fail 'out-lacks-unknown-alias-warning'
  grep -qF 'MARCADOR-SKILL-TS' "$doc" || fail 'doc-lacks-skill-case'
  if grep -qF 'não lê esse formato' "$doc" || grep -qF 'NAO foi lida pelo review' "$doc"; then fail 'doc-still-says-skills-do-not-arrive'; fi
}
run_mutation() {
  prepare_go mutation
  local target="$root/internal/context/skills/skills.go"
  local anchor='if len(sel.Languages) > 0 && !langs.Matches(sel.Languages, cp) {'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra mutation-anchor
  sed -i 's|if len(sel.Languages) > 0 \&\& !langs.Matches(sel.Languages, cp) {|if false { // MUT-001|' "$target"
  grep -Fq 'MUT-001' "$target" || infra mutation-not-applied
  local log="$run_dir/mutation.log"
  run_go_test '^TestAUR565LanguageSelectsSkill$' "$log" || true
  grep -Eq 'build failed|undefined:|syntax error|declared and not used' "$log" && fail 'mutation-build-failure-not-behavioral'
  grep -q '^--- FAIL: TestAUR565LanguageSelectsSkill' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  AC-001-MUT-001) run_mutation ;;
  all) run_ac001; run_ac002; run_ac003; run_ac004; run_mutation ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
