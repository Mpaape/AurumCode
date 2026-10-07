#!/usr/bin/env bash
# AUR-510 acceptance: scripts/release.sh prepares an official release only
# from an approved SHA of main, with a monotonic version, coherent notes and
# an installation example pinned to that version, and publishes only with the
# identity and credentials already configured. Git runs on throwaway
# repositories; nothing is published (no remote, no gh).
#
# Selectors:
#   all        AC-001, AC-002, AC-003, AC-004, MUT-001
#   AC-001     dry-run derives version and notes; refuses off-main SHA,
#              regressive version, tag on another SHA, incoherent changelog
#   AC-002     publish needs the configured identity; never forces or sets one
#   AC-003     the pinned example is identical in site and workflow
#   AC-004     notes carry delivered, limits and migration from v1
#   MUT-001    moving the tag or the example reference fails the check
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-510'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
readonly release="$repo_root/scripts/release.sh"
[[ -f "$release" ]] || infra missing-release-script
command -v git >/dev/null 2>&1 || infra missing-git

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a510.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$work" >/dev/null 2>&1 || true' EXIT INT TERM HUP
export HOME="$work/home" GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0
mkdir -p "$HOME"

fgit() { git -C "$repo" -c user.name=Demo -c user.email=demo@example.invalid -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }

notas_ok='Destaques: changelog obrigatório nas PRs e realimentação da política.

### Entregue

- Revisão de PR com gate de política e changelog obrigatório.

### Limites

- Benchmark com modelo real não medido.

### Migração do v1

- O produto v1 gerava documentação; o v2 só revisa código. Troque o workflow pelo exemplo abaixo.'

# sem_secao SECAO: as notas validas com SECAO trocada por outro titulo.
sem_secao() {
  local secao="$1" novo='### Outra'
  printf '%s\n' "${notas_ok/"$secao"/"$novo"}"
}

exemplo() {
  printf 'jobs:\n  review:\n    uses: Mpaape/AurumCode/.github/workflows/review.yml@%s\n' "$1"
}

# fixture NAME VERSION NOTES EXAMPLE_REF: a main whose HEAD is a release commit.
fixture() {
  repo="$work/$1"
  mkdir -p "$repo/.github/workflows/examples" "$repo/docs/site"
  git init -q -b main "$repo"
  printf '# Changelog\n\n## Unreleased\n' >"$repo/CHANGELOG.md"
  exemplo main >"$repo/.github/workflows/examples/code-review.yml"
  exemplo main >"$repo/docs/site/workflow.yml"
  printf 'uses: Mpaape/AurumCode@main\n' >"$repo/docs/getting-started.md"
  fgit add -A && fgit commit -q -m base
  fgit tag -a v1.1.0 -m historica
  printf '# Changelog\n\n## Unreleased\n\n## %s - 2026-10-07\n\n%s\n\n## 1.1.0\n\n- Produto histórico.\n' "$2" "$3" >"$repo/CHANGELOG.md"
  exemplo "$4" >"$repo/.github/workflows/examples/code-review.yml"
  exemplo "$4" >"$repo/docs/site/workflow.yml"
  printf 'uses: Mpaape/AurumCode@%s\n' "$4" >"$repo/docs/getting-started.md"
  fgit add -A && fgit commit -q -m "release $2"
  sha="$(fgit rev-parse HEAD)"
}

# prepara EXPECTED_RC PATTERN [ARGS...]: runs preparar in $repo.
prepara() {
  local want="$1" pattern="$2" rc=0
  shift 2
  ( cd "$repo" && bash "$release" preparar --main main "$@" ) >"$work/out" 2>&1 || rc=$?
  [[ "$rc" == "$want" ]] || { cat "$work/out" >&2; fail "preparar-rc-$rc-want-$want:$pattern"; }
  grep -q -- "$pattern" "$work/out" || { cat "$work/out" >&2; fail "preparar-output-lacks:$pattern"; }
}

ac001() {
  fixture ok 2.0.0 "$notas_ok" v2.0.0
  prepara 0 "release: v2.0.0 em $sha" --sha "$sha"
  grep -Fq 'Destaques: changelog obrigatório' "$work/out" || fail notes-not-derived
  prepara 1 'nao 2.1.0' --sha "$sha" --versao 2.1.0
  fgit checkout -q -b lateral
  printf 'x\n' >"$repo/lateral.txt"; fgit add -A; fgit commit -q -m lateral
  prepara 1 'fora de main' --sha "$(fgit rev-parse HEAD)"
  fgit checkout -q main
  fixture regressiva 1.0.5 "$notas_ok" v1.0.5
  prepara 1 'nao e maior que v1.1.0' --sha "$sha"
  fixture incoerente 2.0.0 "$(sem_secao '### Limites')" v2.0.0
  prepara 1 "sem '### Limites'" --sha "$sha"
  fixture idempotente 2.0.0 "$notas_ok" v2.0.0
  fgit tag -a v2.0.0 "$sha" -m ja-marcada
  prepara 0 "release: v2.0.0 em $sha" --sha "$sha"
  printf '%s/AC-001/pass\n' "$card"
}

ac002() {
  local rc=0
  fixture publica 2.0.0 "$notas_ok" v2.0.0
  ( cd "$repo" && bash "$release" publicar --sha "$sha" --versao 2.0.0 --main main --evidencia-consumidor "$work/nada" ) >"$work/out" 2>&1 || rc=$?
  [[ "$rc" == 79 ]] && grep -q 'identidade git' "$work/out" || { cat "$work/out" >&2; fail "publish-without-identity-rc-$rc"; }
  [[ -z "$(fgit tag -l v2.0.0)" ]] || fail tag-created-without-identity
  ! grep -Eq 'git config (--global )?user\.(name|email) [^|]' "$release" || fail script-sets-identity
  ! grep -Eq 'push[^#]*(-f|--force)|tag -f|--force-with-lease' "$release" || fail script-forces
  grep -q 'evidencia_ok "$dir" "$sha"' "$release" || fail consumer-check-not-before-publish
  printf '%s/AC-002/pass\n' "$card"
}

ac003() {
  cmp -s "$repo_root/.github/workflows/examples/code-review.yml" "$repo_root/docs/site/workflow.yml" || fail examples-differ
  fixture divergente 2.0.0 "$notas_ok" v2.0.0
  exemplo v2.0.0 | sed 's/review:/revisao:/' >"$repo/docs/site/workflow.yml"
  fgit add -A && fgit commit -q -m divergente
  prepara 1 'diferem' --sha "$(fgit rev-parse HEAD)"
  fixture main-solto 2.0.0 "$notas_ok" main
  prepara 1 'nao fixa @v2.0.0' --sha "$sha"
  printf '%s/AC-003/pass\n' "$card"
}

ac004() {
  local s
  for s in '### Entregue' '### Migração do v1'; do
    fixture "sem-${s//[^a-z]/}" 2.0.0 "$(sem_secao "$s")" v2.0.0
    prepara 1 "sem '$s'" --sha "$sha"
  done
  [[ -f "$repo_root/docs/releases.md" ]] || fail releases-doc-missing
  grep -q 'Migração do v1' "$repo_root/docs/releases.md" || fail migration-not-documented
  printf '%s/AC-004/pass\n' "$card"
}

mut001() {
  local outro
  fixture mut-tag 2.0.0 "$notas_ok" v2.0.0
  outro="$(fgit rev-parse HEAD~1)"
  fgit tag -a v2.0.0 "$outro" -m outro-sha
  prepara 1 'ja existe' --sha "$sha"
  fixture mut-exemplo 2.0.0 "$notas_ok" v2.0.1
  prepara 1 'nao fixa @v2.0.0' --sha "$sha"
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
