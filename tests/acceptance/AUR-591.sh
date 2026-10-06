#!/usr/bin/env bash
# AUR-591 acceptance: the AurumCode repository reviews itself with a minimal,
# strictly parsed .aurumcode/config.yml and its conventions as citable skills.
#
# Selectors:
#   all       AC-003, MUT-001, MUT-002
#   AC-003    .aurumcode/config.yml has at most 30 useful lines (not blank,
#             not comment), declares gate, scanners and deliberation, passes
#             the strict Parse (a typo key is a named load error), and a local
#             `review --base HEAD~1` with an offline provider fixture sends the
#             directory skills selected by path to the model (none listed in
#             the config) and fails the check on a finding citing one of them
#             by its <dir>#<slug> id
#   MUT-001   without the skill file, the same fixture finding is discarded as
#             an unknown rule and the check passes: the finding depends on the
#             skill, not on a fixed rule
#   MUT-002   the self-review provider guard of code-review.yml, run with empty
#             LLM secrets and a stub `gh`, exits non-zero and publishes
#             aurumcode/policy-gate as failure, never success; the same guard
#             with its final `exit 1` mutated to `exit 0` is caught
# AC-001, AC-002 and AC-004 need a real pull request (see docs/specs/AUR-591.md).
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-591'
selector="${1:-all}"
case "$selector" in
  all|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
readonly config="$repo_root/.aurumcode/config.yml"
readonly workflow="$repo_root/.github/workflows/code-review.yml"
readonly skill_rule='tamanho#tam-001-funcao-com-no-maximo-150-linhas'
for input in go.mod go.sum cmd internal pkg .aurumcode/config.yml .aurumcode/skills/tamanho/SKILL.md .github/workflows/code-review.yml; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a591.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

bin="$run_dir/aurumcode"
mkrepo="$run_dir/mkrepo-bin"
# build compiles the product from a copy of the whole module, and mkrepo: a
# stdlib-only helper that writes a two-commit git repository as loose objects
# (the sealed profile has no git CLI; the product reads the objects itself).
build() {
  [[ -x "$bin" ]] && return 0
  command -v go >/dev/null 2>&1 || infra missing_go
  local src="$run_dir/src" source
  mkdir -p "$src" "$run_dir/mkrepo"
  for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$src/$source"; done
  chmod -R u+w -- "$src"
  ( cd "$src" && go build -buildvcs=false -o "$bin" ./cmd/aurumcode ) >"$run_dir/build.log" 2>&1 ||
    { cat "$run_dir/build.log" >&2; infra build; }
  printf 'module mkrepo\n\ngo 1.22\n' >"$run_dir/mkrepo/go.mod"
  cat >"$run_dir/mkrepo/main.go" <<'GO'
// Command mkrepo DIR NAME BASE HEAD writes DIR/.git with a base commit whose
// tree holds NAME with the content of file BASE, and a head commit (on main)
// with the content of file HEAD; NAME in the working tree gets HEAD.
package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

func object(dir, kind string, body []byte) string {
	payload := append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...)
	sum := sha1.Sum(payload)
	id := hex.EncodeToString(sum[:])
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	must(w.Write(payload))
	must(0, w.Close())
	p := filepath.Join(dir, ".git", "objects", id[:2], id[2:])
	must(0, os.MkdirAll(filepath.Dir(p), 0o700))
	must(0, os.WriteFile(p, z.Bytes(), 0o600))
	return id
}

func must[T any](_ T, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func commit(dir, name string, content []byte, parent, msg string) string {
	blob := object(dir, "blob", content)
	raw, _ := hex.DecodeString(blob)
	tree := object(dir, "tree", append([]byte("100644 "+name+"\x00"), raw...))
	body := "tree " + tree + "\n"
	if parent != "" {
		body += "parent " + parent + "\n"
	}
	body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\n" + msg + "\n"
	return object(dir, "commit", []byte(body))
}

func main() {
	dir, name := os.Args[1], os.Args[2]
	base, err := os.ReadFile(os.Args[3])
	must(0, err)
	head, err := os.ReadFile(os.Args[4])
	must(0, err)
	first := commit(dir, name, base, "", "base")
	last := commit(dir, name, head, first, "mudanca")
	must(0, os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))
	must(0, os.MkdirAll(filepath.Join(dir, ".git", "refs", "heads"), 0o700))
	must(0, os.WriteFile(filepath.Join(dir, ".git", "refs", "heads", "main"), []byte(last+"\n"), 0o600))
	must(0, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"), 0o600))
	must(0, os.WriteFile(filepath.Join(dir, name), head, 0o600))
}
GO
  ( cd "$run_dir/mkrepo" && go build -buildvcs=false -o "$mkrepo" . ) >"$run_dir/mkrepo.log" 2>&1 ||
    { cat "$run_dir/mkrepo.log" >&2; infra build-mkrepo; }
}

# repo NAME CONFIG: a repository whose working tree carries this repository's
# .aurumcode (with CONFIG as its config.yml) and whose last commit adds a
# 160-line function to longa.go.
repo() {
  local dir="$run_dir/$1" i
  mkdir -p "$dir"
  cp -R "$repo_root/.aurumcode" "$dir/.aurumcode"
  cp "$2" "$dir/.aurumcode/config.yml"
  printf 'module example.com/convencao\n\ngo 1.22\n' >"$dir/go.mod"
  printf 'package convencao\n' >"$run_dir/$1.base"
  {
    printf 'package convencao\n\n// Longa soma uma linha por passo.\nfunc Longa() int {\n\ttotal := 0\n'
    for ((i = 0; i < 155; i++)); do printf '\ttotal++\n'; done
    printf '\treturn total\n}\n'
  } >"$run_dir/$1.head"
  "$mkrepo" "$dir" longa.go "$run_dir/$1.base" "$run_dir/$1.head" || infra "mkrepo:$1"
  printf '%s' "$dir"
}

# fixture: the offline model cites the size convention on the added function.
fixture="$run_dir/fixture.json"
printf '{"issues":[{"file":"longa.go","line":4,"severity":"warning","rule_id":"%s","message":"Funcao Longa tem 160 linhas.","impact":"Funcao longa demais para revisar.","evidence":"As linhas adicionadas declaram Longa em longa.go com 160 linhas.","suggestion":"Divida em etapas nomeadas.","verification":"Conte as linhas da funcao."}],"summary":"Convencao de tamanho violada."}\n' "$skill_rule" >"$fixture"

# review DIR NAME: runs `review --base HEAD~1` in DIR; sets rc, out, err, prompt.
review() {
  out="$run_dir/$2.out"; err="$run_dir/$2.err"; prompt="$run_dir/$2.prompt"
  set +e
  mkdir -p "$run_dir/cache-$2"
  ( cd "$1" && env -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL XDG_CACHE_HOME="$run_dir/cache-$2" AURUMCODE_CACHE_DIR="$run_dir/cache-$2" \
      AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$prompt" "$bin" review --base HEAD~1 ) >"$out" 2>"$err"
  rc=$?
  set -e
  printf '%s rc=%s\n' "$2" "$rc" >&2
}

# no_scanners writes CONFIG without its quality_gates section: the sealed
# profile has neither gitleaks nor semgrep, and an absent engine is
# (correctly) inconclusive, which would hide the skill verdict behind rc 1.
no_scanners() {
  awk '/^quality_gates:/{skip=1; next} /^[^[:space:]#]/{skip=0} !skip' "$config" >"$1"
}

run_ac003() {
  local useful
  useful="$(grep -cvE '^[[:space:]]*(#|$)' "$config")"
  printf 'useful lines: %s\n' "$useful" >&2
  (( useful <= 30 )) || fail "config-too-long:$useful"
  for key in '^gate:' '^  fail_on: \[error\]' '^  inconclusive: block' '^quality_gates:' '^  scanners:' \
             '^    - engine: gitleaks' '^      required: true' '^    - engine: semgrep' '^    - engine: govet' \
             '^deliberation:' '^  enabled: true' '^  max_rounds: ' '^  max_cost_tokens: ' '^  per_tool_timeout_seconds: ' \
             '^  - "tests/\*\*"' '^  - "docs/assets/capturas/\*\*"' '^  language: pt-BR'; do
    grep -Eq "$key" "$config" || fail "config-lacks:$key"
  done
  build

  # Strict Parse: the real file loads; the gate section is enforced (the
  # absent scanners of this profile are inconclusive and block: rc 1).
  local dir
  dir="$(repo real "$config")"
  review "$dir" real
  grep -q 'parsing .*config.yml' "$err" && fail 'real-config-did-not-parse'
  [[ "$rc" == 1 ]] || fail "real-config-rc:$rc"
  grep -q 'secrets_unavailable' "$err" || fail 'gitleaks-not-declared-effective'
  grep -q 'deliberation' "$err" || fail 'deliberation-not-effective'

  # A typo key is a named load error before any model call.
  sed 's/^  fail_on: /  fial_on: /' "$config" >"$run_dir/typo.yml"
  dir="$(repo typo "$run_dir/typo.yml")"
  review "$dir" typo
  [[ "$rc" != 0 ]] || fail 'typo-config-accepted'
  grep -q 'field fial_on not found' "$err" || fail 'typo-not-named'
  [[ ! -s "$prompt" ]] || fail 'typo-config-reached-model'

  # The repository skills reach the model and the gate cites one of them.
  no_scanners "$run_dir/skills.yml"
  dir="$(repo skills "$run_dir/skills.yml")"
  review "$dir" skills
  local heading
  # Directory skills, selected by their paths: the changed longa.go selects
  # the Go-wide and repository-wide skills, not the cmd/internal/pkg ones.
  for heading in 'TAM-001' 'TAM-002' 'PUB-001' 'PUB-002' 'BOOL-001'; do
    grep -q "## $heading " "$prompt" || fail "skill-not-in-prompt:$heading"
  done
  for heading in 'ARQ-001' 'CARD-001' 'GATE-001'; do
    grep -q "## $heading " "$prompt" && fail "skill-outside-its-paths-in-prompt:$heading"
  done
  grep -q 'context:' "$config" && fail 'skills-listed-in-config'
  [[ "$rc" == 3 ]] || fail "skill-finding-did-not-fail-check:$rc"
  grep -qF "(rule $skill_rule: TAM-001 Funcao com no maximo 150 linhas)" "$out" || fail 'finding-does-not-cite-skill'
  grep -qF "policy gate: $skill_rule" "$err" || fail 'gate-does-not-name-skill'
  tail -n 2 "$out" >&2
}

run_mut001() {
  build
  no_scanners "$run_dir/skills.yml"
  local dir
  dir="$(repo mutation "$run_dir/skills.yml")"
  rm -f -- "$dir/.aurumcode/skills/tamanho/SKILL.md"
  review "$dir" mutation
  grep -q 'parsing .*config.yml' "$err" && infra 'mutation-config-did-not-parse'
  [[ -s "$prompt" ]] || infra 'mutation-did-not-reach-model'
  grep -q '## TAM-001 ' "$prompt" && fail 'mutation-not-applied'
  grep -qF "citing an unknown rule_id ($skill_rule)" "$err" || fail 'finding-survived-without-skill'
  grep -qF "rule $skill_rule" "$out" && fail 'finding-survived-without-skill'
  [[ "$rc" == 0 ]] || fail "check-not-green-without-finding:$rc"
  grep -F 'unknown rule_id' "$err" >&2
}

# guard_script WORKFLOW DEST: the provider guard step, between its markers.
guard_script() {
  awk '/# provider-guard-begin/{on=1} on{sub(/^          /, ""); print} /# provider-guard-end/{on=0}' "$1" >"$2"
  [[ -s "$2" ]] || infra 'guard-markers-missing'
}

# run_guard SCRIPT KEY: runs the guard with a stub gh; sets grc and glog.
run_guard() {
  local stub="$run_dir/stub"
  mkdir -p "$stub"
  glog="$run_dir/gh.log"; : >"$glog"
  printf '#!/usr/bin/env bash\nprintf "%%s\\n" "$*" >>"%s"\n' "$glog" >"$stub/gh"
  chmod +x "$stub/gh"
  set +e
  PATH="$stub:$PATH" LLM_API_KEY="$2" LLM_BASE_URL="$2" GH_TOKEN=t REPOSITORY=example/repo \
    HEAD_SHA=0123456789abcdef0123456789abcdef01234567 bash "$1" >"$run_dir/guard.out" 2>&1
  grc=$?
  set -e
}

# guard_fails_closed SCRIPT: the guard without secrets must fail and post failure.
guard_fails_closed() {
  run_guard "$1" ''
  [[ "$grc" != 0 ]] || return 1
  grep -q 'state=failure' "$glog" || return 1
  grep -q 'context=aurumcode/policy-gate' "$glog" || return 1
  grep -q 'secrets LLM_API_KEY e LLM_BASE_URL ausentes' "$glog" || return 1
  if grep -q 'state=success' "$glog"; then return 1; fi
  return 0
}

run_mut002() {
  local guard="$run_dir/guard.sh"
  guard_script "$workflow" "$guard"
  guard_fails_closed "$guard" || fail 'guard-does-not-fail-closed'
  cat "$glog" >&2
  run_guard "$guard" configured
  [[ "$grc" == 0 && ! -s "$glog" ]] || fail 'guard-blocks-or-publishes-with-secrets'
  grep -q 'needs:' "$workflow" && fail 'guard-or-review-chained-by-needs'
  local mutated="$run_dir/guard-mutated.sh"
  sed 's/^exit 1$/exit 0/' "$guard" >"$mutated"
  cmp -s "$guard" "$mutated" && infra 'guard-mutation-not-applied'
  if guard_fails_closed "$mutated"; then fail 'guard-mutation-survived'; fi
  printf 'mutated guard exit=%s caught\n' "$grc" >&2
}

case "$selector" in
  AC-003) run_ac003 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all) run_ac003; run_mut001; run_mut002 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
