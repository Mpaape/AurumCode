#!/usr/bin/env bash
# AUR-501 acceptance: every delivered capability has a tutorial with a
# reproducible demonstration, tracked in tests/fixtures/tutorials/mapa.tsv
# (capacidade -> tutorial -> caso -> comando -> fixture -> evidencia), and
# what is not delivered is listed as planned. Offline: bash only, no Go, no
# docker; `run.sh --check` and the recorded out/ belong to the final flow.
#
# Selectors:
#   all        AC-001, AC-002, AC-003, AC-004, MUT-001
#   AC-001     the map is complete and every row points at real artifacts
#   AC-002     cases assert their exit; the no-provider and model fixtures are identified
#   AC-003     review, context, memory, fix, publication, changelog and release journeys
#   AC-004     no credential shape and no host Go command in the tutorials
#   MUT-001    dropping a public case or changing a promised output turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-501'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly map_rel='tests/fixtures/tutorials/mapa.tsv'
for input in "$map_rel" docs/tutorials demo/tutoriais cmd/aurumcode; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a501.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$work" >/dev/null 2>&1 || true' EXIT INT TERM HUP

# rows ROOT: the data rows of the map (no comments, no header).
rows() { grep -v '^#' "$1/$map_rel" | tail -n +2; }

# casos ROOT TUTORIAL: the CASOS of a tutorial's run.sh, one per line.
casos() {
  sed -n 's/^CASOS=(\(.*\))$/\1/p' "$1/demo/tutoriais/$2/run.sh" | tr ' ' '\n' | sed '/^$/d'
}

subcommands() {
  grep -rhoE 'name:[[:space:]]+"[a-z-]+"' "$1/cmd/aurumcode/subcommands.go" "$1/cmd/aurumcode"/cmd_*.go | sed -E 's/.*"([a-z-]+)"/\1/' | sort -u
}

# check_map ROOT prints the first violation and returns 1, or returns 0.
check_map() {
  local root="$1" capacidade tutorial caso comando fixture esperado evidencia jornada estado t c n=0
  subcommands "$root" >"$work/subs.txt"
  [[ -s "$work/subs.txt" ]] || { echo "no-subcommands"; return 1; }
  while IFS=$'\t' read -r capacidade tutorial caso comando fixture esperado evidencia jornada estado; do
    n=$((n + 1))
    [[ -n "$capacidade" && -n "$estado" ]] || { echo "row-$n-incomplete"; return 1; }
    case "$estado" in
      entregue)
        [[ -f "$root/docs/tutorials/$tutorial.md" ]] || { echo "doc-missing:$tutorial"; return 1; }
        [[ -f "$root/demo/tutoriais/$tutorial/run.sh" ]] || { echo "run-missing:$tutorial"; return 1; }
        [[ "$fixture" == "demo/tutoriais/$tutorial" ]] || { echo "fixture-mismatch:$tutorial/$caso"; return 1; }
        # No `| grep -q` under pipefail: grep exits on the first match and
        # the producer's SIGPIPE would fail the pipeline.
        grep -Fxq -- "$caso" <<<"$(casos "$root" "$tutorial")" || { echo "case-not-in-run:$tutorial/$caso"; return 1; }
        [[ -f "$root/demo/tutoriais/$tutorial/$esperado" ]] || { echo "expected-missing:$tutorial/$caso"; return 1; }
        grep -Fxq -- "$evidencia" "$root/demo/tutoriais/$tutorial/$esperado" || { echo "promised-output-changed:$tutorial/$caso"; return 1; }
        if [[ "$comando" == aurumcode\ * ]]; then
          grep -Fxq -- "${comando#aurumcode }" "$work/subs.txt" || { echo "unknown-command:$comando"; return 1; }
        fi
        ;;
      planejado)
        [[ "$tutorial" == '-' && -n "$evidencia" && "$evidencia" != '-' ]] || { echo "planned-without-reason:$capacidade"; return 1; }
        ;;
      *) echo "unknown-state:$estado"; return 1 ;;
    esac
  done < <(rows "$root")
  [[ $n -ge 10 ]] || { echo "map-too-small:$n"; return 1; }
  # Completeness: every public case of every tutorial is in the map.
  for t in "$root"/demo/tutoriais/*/run.sh; do
    t="${t%/run.sh}"; t="${t##*/}"
    while IFS= read -r c; do
      rows "$root" | awk -F'\t' -v t="$t" -v c="$c" '$2==t && $3==c && $9=="entregue"{f=1} END{exit !f}' || { echo "case-missing-from-map:$t/$c"; return 1; }
    done < <(casos "$root" "$t")
  done
  return 0
}

ac001() {
  local why
  why="$(check_map "$repo_root")" || fail "$why"
  printf '%s/AC-001/pass\n' "$card"
}

ac002() {
  local tutorial caso esperado file
  local evidencia
  while IFS=$'\t' read -r _ tutorial caso _ _ esperado evidencia _ estado; do
    [[ "$estado" == entregue ]] || continue
    file="$repo_root/demo/tutoriais/$tutorial/$esperado"
    # Every delivered case asserts its outcome in expected/: an exit code,
    # a `RESULTADO:` line (expect_rc prints it only after the exit code
    # matched), an explicit "not executed here", or the evidence line the map
    # promises (AC-001 requires it verbatim) for cases that check content.
    grep -Eq '^(exit_code=[0-9]+|RESULTADO: .+|NAO EXECUTADO( AQUI)?: .+)$' "$file" ||
      { [[ "$evidencia" != '-' ]] && grep -Fxq -- "$evidencia" "$file"; } ||
      fail "no-outcome-asserted:$tutorial/$caso"
  done < <(rows "$repo_root")
  # Without a provider the review explains the deterministic analysis.
  grep -q 'TUT_FIXTURE=none' "$repo_root/demo/tutoriais/revisao/run.sh" || fail no-provider-case
  grep -qi 'determin' "$repo_root/docs/tutorials/revisao.md" || fail no-provider-not-explained
  # A tutorial that runs a review carries the identified model fixture.
  for t in "$repo_root"/demo/tutoriais/*/run.sh; do
    grep -Eq '^[[:space:]]*aurum review' "$t" || continue
    t="${t%/run.sh}"
    ls "$t"/fixture*.json >/dev/null 2>&1 || grep -q 'TUT_FIXTURE=' "$t/run.sh" || fail "model-fixture-not-identified:${t##*/}"
  done
  printf '%s/AC-002/pass\n' "$card"
}

ac003() {
  local j tutorial all
  # Read the map once: awk that exits early would SIGPIPE `rows` under pipefail.
  all="$(rows "$repo_root")"
  for j in review contexto memoria sugestao-fix publicacao changelog release; do
    awk -F'\t' -v j="$j" '$8==j{f=1} END{exit !f}' <<<"$all" || fail "journey-missing:$j"
    tutorial="$(awk -F'\t' -v j="$j" '$8==j && $9=="entregue" && !t{t=$2} END{print t}' <<<"$all")"
    [[ -z "$tutorial" ]] && continue
    grep -Eq '^## (Problemas comuns|Quando falha)' "$repo_root/docs/tutorials/$tutorial.md" || fail "errors-not-explained:$tutorial"
  done
  printf '%s/AC-003/pass\n' "$card"
}

ac004() {
  local hit
  hit="$(grep -rEl --exclude-dir=.estado 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----' "$repo_root/docs/tutorials" "$repo_root/demo/tutoriais" 2>/dev/null | head -n1 || true)"
  [[ -z "$hit" ]] || fail "credential-shape:${hit#"$repo_root"/}"
  hit="$(awk '/^```bash/{f=1;next} /^```/{f=0} f && /^[[:space:]]*(\$ )?go (build|test|run|vet|install)/{print FILENAME; exit}' "$repo_root"/docs/tutorials/*.md)"
  [[ -z "$hit" ]] || fail "host-go-command:${hit#"$repo_root"/}"
  # Documentation is rebuilt on every pull request that changes it.
  grep -q 'Documentation browser checks' "$repo_root/.github/workflows/ci.yml" || fail docs-job-missing
  grep -Eq '^[[:space:]]+pull_request:' "$repo_root/.github/workflows/ci.yml" || fail docs-job-not-on-pr
  printf '%s/AC-004/pass\n' "$card"
}

# stage copies what check_map reads into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root/tests/fixtures" "$root/docs" "$root/demo" "$root/cmd"
  cp -R "$repo_root/tests/fixtures/tutorials" "$root/tests/fixtures/tutorials"
  cp -R "$repo_root/docs/tutorials" "$root/docs/tutorials"
  cp -R "$repo_root/cmd/aurumcode" "$root/cmd/aurumcode"
  mkdir -p "$root/demo/tutoriais"
  for t in "$repo_root"/demo/tutoriais/*/; do
    t="${t%/}"; t="${t##*/}"
    mkdir -p "$root/demo/tutoriais/$t"
    cp "$repo_root/demo/tutoriais/$t/run.sh" "$root/demo/tutoriais/$t/" 2>/dev/null || true
    [[ -d "$repo_root/demo/tutoriais/$t/expected" ]] && cp -R "$repo_root/demo/tutoriais/$t/expected" "$root/demo/tutoriais/$t/expected"
  done
  chmod -R u+w -- "$root"
}

mut001() {
  local root="$work/mut-a" why line
  stage "$root"
  check_map "$root" >/dev/null || infra mutation-baseline-not-green
  # (a) a public case disappears from the map.
  awk -F'\t' '$2=="revisao" && $3=="fix"{f=1} END{exit !f}' "$root/$map_rel" || infra victim-row-absent
  awk -F'\t' '!($2=="revisao" && $3=="fix")' "$repo_root/$map_rel" >"$root/$map_rel"
  if why="$(check_map "$root")"; then fail mutation-survived:row-removed; fi
  [[ "$why" == 'case-missing-from-map:revisao/fix' ]] || fail "mutation-wrong-reason:$why"
  # (b) a promised output changes.
  root="$work/mut-b"
  stage "$root"
  line="$(grep -m1 '^RESULTADO: ' "$root/demo/tutoriais/changelog/expected/entrada-valida.txt")" || infra victim-output-absent
  sed -i 's/^RESULTADO: .*/RESULTADO: outra promessa/' "$root/demo/tutoriais/changelog/expected/entrada-valida.txt"
  ! grep -Fxq -- "$line" "$root/demo/tutoriais/changelog/expected/entrada-valida.txt" || infra mutation-not-applied
  if why="$(check_map "$root")"; then fail mutation-survived:output-changed; fi
  [[ "$why" == 'promised-output-changed:changelog/entrada-valida' ]] || fail "mutation-wrong-reason:$why"
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  MUT-001) mut001 ;;
  all)
    ac001
    ac002
    ac003
    ac004
    mut001
    printf '%s/all/pass\n' "$card"
    ;;
esac
