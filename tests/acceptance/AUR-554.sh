#!/usr/bin/env bash
# AUR-554 acceptance (sealed, offline, pure bash): the corporate-gate guide is
# coherent with the demonstration, and the recorded output of the last REAL run
# (demo/gate-corporativo/out/, produced by run.sh with containers and public
# images, outside this sandbox) matches what the demo expects.
#
# Selectors:
#   all       every check below, then the mutation
#   AC-001    every YAML block the guide marks with "<!-- arquivo: PATH -->"
#             is byte-identical to PATH in demo/gate-corporativo
#   AC-002    run.sh --check passes on the versioned out/, and the failing run
#             names the SAST rule and the Dependency-Track violation while the
#             fixed run is approved with the inventory gate explicitly "aprovado"
#   AC-003    the SBOM signature verified ("Verified OK"), a tampered copy was
#             rejected, and no key material or API key is versioned
#   AC-004    no real host/domain/endpoint (only localhost, 127.0.0.1 and
#             reserved domains) and images.lock holds only digests
#   MUT-001   removing the planted finding from out/fail.log makes --check fail
# Unknown selectors exit 64; infrastructure failures 79; failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-554'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
demo="$repo_root/demo/gate-corporativo"
guide="$repo_root/docs/gate-corporativo.md"

for f in "$guide" "$repo_root/docs/README.md" "$repo_root/docs/specs/AUR-554.md" \
         "$demo/run.sh" "$demo/images.lock" "$demo/compose.yml" \
         "$repo_root/.board/bootstrap/locks/scanners.yml"; do
  [[ -f "$f" ]] || infra "missing:${f#"$repo_root"/}"
done
for tool in awk grep sed diff cp mktemp find; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-tool:$tool"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a554.XXXXXX")" || infra mktemp
trap 'chmod -R u+rwX "${work:?}" 2>/dev/null || true; rm -rf "${work:?}"' EXIT

ac001() {
  grep -q 'gate-corporativo.md' "$repo_root/docs/README.md" || fail "AC-001/readme-sem-link"
  local n=0 path
  while IFS= read -r path; do
    n=$((n + 1))
    [[ -f "$repo_root/$path" ]] || fail "AC-001/arquivo-inexistente:$path"
    awk -v want="<!-- arquivo: $path -->" '
      $0 == want { armed = 1; next }
      armed && !open && /^```/ { open = 1; next }
      open && /^```/ { exit }
      open { print }
    ' "$guide" > "$work/bloco"
    [[ -s "$work/bloco" ]] || fail "AC-001/bloco-ausente:$path"
    diff -u "$repo_root/$path" "$work/bloco" >/dev/null || fail "AC-001/bloco-diverge:$path"
  done < <(sed -n 's/^<!-- arquivo: \(.*\) -->$/\1/p' "$guide")
  (( n >= 4 )) || fail "AC-001/blocos-insuficientes:$n"
  printf '%s/AC-001/ok (%d blocos identicos aos arquivos da demo)\n' "$card" "$n"
}

ac002() {
  bash "$demo/run.sh" --check >"$work/check.out" 2>&1 || { cat "$work/check.out" >&2; fail "AC-002/check-falhou"; }
  grep -q '^CHECK OK$' "$work/check.out" || fail "AC-002/check-sem-ok"
  local f="$demo/out/fail.log" p="$demo/out/pass.log"
  grep -q 'policy gate: semgrep:github.policy.regras.demo-sem-eval' "$f" || fail "AC-002/regra-sast-nao-nomeada"
  grep -q 'src/calc.js:5: \[error\]' "$f" || fail "AC-002/linha-sast-nao-nomeada"
  grep -q 'policy gate: ssor_dtrack: policy_violations 1 > policy_violations 0' "$f" || fail "AC-002/violacao-dtrack-nao-nomeada"
  grep -q 'componente=lodash versao=4.17.15' "$f" || fail "AC-002/componente-nao-nomeado"
  grep -q 'Verdict:\*\* Changes requested' "$f" || fail "AC-002/veredito-reprovado-ausente"
  grep -q 'exit_code=3' "$f" || fail "AC-002/exit-reprovado-ausente"
  if grep -q 'Verdict:\*\* Approve' "$f"; then fail "AC-002/fail-aprovado"; fi
  grep -q 'Verdict:\*\* Approve' "$p" || fail "AC-002/pass-sem-aprovacao"
  grep -q 'ssor_dtrack: aprovado (critical=0, high=0, policy_violations=0)' "$p" || fail "AC-002/pass-inventario-nao-aprovado"
  grep -q 'exit_code=0' "$p" || fail "AC-002/pass-exit"
  if grep -qi 'inconclus' "$p"; then fail "AC-002/pass-inconclusivo"; fi
  if grep -q 'semgrep:' "$p"; then fail "AC-002/pass-ainda-tem-achado-sast"; fi
  printf '%s/AC-002/ok\n' "$card"
}

ac003() {
  local p="$demo/out/pass.log" v="$demo/out/verify.log"
  grep -q '^sign: sbom .* -> .*sbom_app_cyclonedx.json.sigstore.json$' "$p" || fail "AC-003/sbom-nao-assinado"
  grep -q '^Verified OK$' "$v" || fail "AC-003/verified-ok-ausente"
  grep -q '^adulterado: SBOM modificado e rejeitado$' "$v" || fail "AC-003/adulterado-aceito"
  if find "$demo" "$repo_root/docs" \( -name 'cosign.key' -o -name 'cosign.pub' -o -name '*.sigstore.json' -o -name 'dtrack.env' \) | grep -q .; then
    fail "AC-003/material-de-chave-versionado"
  fi
  if grep -rIl 'odt_[A-Za-z0-9]\{8\}_' "$demo" "$repo_root/docs" 2>/dev/null | grep -q .; then
    fail "AC-003/chave-de-api-vazada"
  fi
  if grep -rIl 'BEGIN \(ENCRYPTED \)\?\(SIGSTORE \)\?PRIVATE KEY' "$demo" "$repo_root/docs" 2>/dev/null | grep -q .; then
    fail "AC-003/chave-privada-versionada"
  fi
  printf '%s/AC-003/ok\n' "$card"
}

ac004() {
  local files=() f
  while IFS= read -r f; do files+=("$f"); done < <(find "$demo" "$repo_root/docs/specs/AUR-554.md" "$guide" -type f \
    ! -name images.lock ! -name compose.yml)
  local allowed='^(localhost|127\.0\.0\.1|([A-Za-z0-9-]+\.)*(example\.(com|org|net)|[A-Za-z0-9-]+\.invalid|[A-Za-z0-9-]+\.test)|example\.(com|org|net)|invalid|test)$'
  local host bad=''
  while IFS= read -r host; do
    host="${host#*://}"; host="${host%%[:/]*}"
    [[ "$host" =~ $allowed ]] || bad="$bad $host"
  done < <(grep -rhoE 'https?://[^/[:space:]"'"'"')`>]+' "${files[@]}" || true)
  [[ -z "$bad" ]] || fail "AC-004/url-real:$bad"
  bad=''
  while IFS= read -r host; do
    [[ "$host" =~ $allowed ]] || bad="$bad $host"
  done < <(grep -rhoE '\b([A-Za-z0-9-]+\.)+(com|org|net|io|dev|app|cloud|co|ai|local|internal|corp|lan|br|gov|edu)\b' "${files[@]}" | grep -vE '^(docker\.io|ghcr\.io)$' || true)
  [[ -z "$bad" ]] || fail "AC-004/dominio-real:$bad"
  # e-mails so em dominio reservado
  bad="$(grep -rhoE '[A-Za-z0-9._-]+@([A-Za-z0-9-]+\.)+[A-Za-z]{2,}' "${files[@]}" | grep -vE '@(example\.(com|org|net)|[A-Za-z0-9-]+\.invalid|[A-Za-z0-9-]+\.test)$|@sha256$' || true)"
  [[ -z "$bad" ]] || fail "AC-004/email-real:$bad"
  # images.lock: so digests, e as mesmas imagens que o compose usa
  local line n=0
  while IFS= read -r line; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" =~ ^[a-z_]+=(docker\.io|ghcr\.io)/[a-z0-9._/-]+@sha256:[0-9a-f]{64}$ ]] || fail "AC-004/images.lock-sem-digest:$line"
    n=$((n + 1))
  done < "$demo/images.lock"
  (( n >= 5 )) || fail "AC-004/images.lock-incompleto"
  while IFS= read -r line; do
    grep -qF "=$line" "$demo/images.lock" || fail "AC-004/compose-fora-do-lock:$line"
  done < <(sed -n 's/^[[:space:]]*image:[[:space:]]*//p' "$demo/compose.yml")
  # semgrep e trivy iguais ao lock de scanners
  local lockfile="$repo_root/.board/bootstrap/locks/scanners.yml" want
  want="$(sed -n 's/^sast_scanner_image:[[:space:]]*//p' "$lockfile")"
  grep -qxF "semgrep=$want" "$demo/images.lock" || fail "AC-004/semgrep-diverge-de-scanners.yml"
  want="$(sed -n 's/^vuln_scanner_image:[[:space:]]*//p' "$lockfile")"
  grep -qxF "trivy=$want" "$demo/images.lock" || fail "AC-004/trivy-diverge-de-scanners.yml"
  printf '%s/AC-004/ok\n' "$card"
}

mut001() {
  local copy="$work/demo"
  mkdir -p "$copy"
  cp -R "$demo/." "$copy/"
  chmod -R u+rwX "$copy"
  bash "$copy/run.sh" --check >/dev/null 2>&1 || fail "MUT-001/controle-nao-passa"
  # remove o defeito plantado (o achado de SAST) do log registrado
  sed -i '/demo-sem-eval/d; /src\/calc.js:5/d' "$copy/out/fail.log"
  if bash "$copy/run.sh" --check >"$work/mut.out" 2>&1; then
    fail "MUT-001/mutacao-sobreviveu-sast"
  fi
  grep -q 'DIVERGENCIA fase=fail' "$work/mut.out" || fail "MUT-001/divergencia-nao-nomeada"
  # idem para a dependencia plantada
  cp "$demo/out/fail.log" "$copy/out/fail.log"
  sed -i '/componente=lodash/d; /policy_violations 1/d' "$copy/out/fail.log"
  if bash "$copy/run.sh" --check >"$work/mut2.out" 2>&1; then
    fail "MUT-001/mutacao-sobreviveu-dtrack"
  fi
  printf '%s/MUT-001/ok (sem o achado plantado, --check diverge)\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  MUT-001) mut001 ;;
  all) ac001; ac002; ac003; ac004; mut001 ;;
esac
