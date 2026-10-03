#!/usr/bin/env bash
# Tutorial executavel: assinatura (AUR-563). Veja ../README.md e docs/tutorials/assinatura.md.
#
#   run.sh chave-efemera|verificacao-por-terceiro|keyless-actions|bundle-artefato|
#          falha-cosign|falha-sem-bundle|falha-imagem-sem-digest
#   run.sh all | --check | limpar
#
# O Cosign e o fixado por digest em demo/gate-corporativo/images.lock (extraido
# por _lib/cadeia.sh); a chave efemera nasce em .estado/ (ignorado pelo git).
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"
# shellcheck source=../_lib/cadeia.sh
. "$HERE/../_lib/cadeia.sh"

CASOS=(chave-efemera verificacao-por-terceiro keyless-actions bundle-artefato falha-cosign falha-sem-bundle falha-imagem-sem-digest)

# aurumcode neste tutorial: sem rede, com os binarios em /demo/bin e a chave
# efemera em /keys (so este tutorial precisa de ambos; o _lib monta so /fixtures).
aurum_raw() {
  local envs=()
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do envs+=("$1" "$2"); shift 2; done
  [ "${1:-}" = "--" ] && shift
  mkdir -p "$STATE/keys"
  cad_run -v "$STATE/keys:/keys" "${envs[@]}" --entrypoint /app/aurumcode -- "$@"
}

# cosign_raw [-v host:cont ...] -- args: o binario do Cosign direto, sem rede.
cosign_raw() { cad_run "$@" ; }
cosign_ep() {
  local opts=()
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do opts+=("$1"); shift; done
  shift
  cad_run "${opts[@]}" --entrypoint /demo/bin/cosign -- "$@"
}

# Chave efemera: nasce uma vez por execucao, sem senha, so em .estado/keys.
ensure_keys() {
  cad_bins
  [ -s "$STATE/keys/cosign.key" ] && return 0
  mkdir -p "$STATE/keys"
  docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp -e COSIGN_PASSWORD= \
    -v "$STATE/bin:/demo/bin:ro" -v "$STATE/keys:/keys" -w /keys \
    --entrypoint /demo/bin/cosign "$TUT_IMAGE" generate-key-pair 2>&1 | sed 's/^/cosign: /'
  mkdir -p "$STATE/pub"; cp "$STATE/keys/cosign.pub" "$STATE/pub/"
}

# 1. aurumcode sign com chave efemera, offline.
caso_chave_efemera() {
  tut_repo chave-efemera repo-exemplo
  rm -rf "$STATE/keys" "$STATE/pub"
  echo "--- chave efemera (senha vazia, so em .estado/keys, ignorada pelo git)"
  ensure_keys
  echo "--- antes: nao ha bundle"
  ls "$TUT_WORK" | grep -c 'sigstore' || true
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-wrap.sh
  expect_rc 0 "sign terminou com exit 0 e relatou o bundle"
  if [ -s "$TUT_WORK/sbom_app_cyclonedx.json.sigstore.json" ]; then
    echo "RESULTADO: o bundle sbom_app_cyclonedx.json.sigstore.json existe e nao esta vazio (conferido pelo script)"
  else
    echo "ERRO: bundle ausente"; return 1
  fi
  echo "bundle: campos de primeiro nivel: $(python3 -c 'import json,sys; print(",".join(sorted(json.load(open(sys.argv[1])))))' "$TUT_WORK/sbom_app_cyclonedx.json.sigstore.json")"
}

# 2. Verificacao por terceiro: so a chave publica, o SBOM e o bundle.
caso_verificacao_por_terceiro() {
  tut_repo verificacao-por-terceiro repo-exemplo
  rm -rf "$STATE/keys" "$STATE/pub"; ensure_keys >/dev/null
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-wrap.sh
  expect_rc 0 "sbom assinado"
  echo "--- o terceiro so tem cosign.pub, o SBOM e o bundle (sem rede)"
  printf '$ cosign verify-blob --key /pub/cosign.pub --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct sbom_app_cyclonedx.json\n'
  set +e
  cosign_ep -v "$STATE/pub:/pub:ro" -- verify-blob --key /pub/cosign.pub \
    --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct sbom_app_cyclonedx.json 2>&1
  LAST_RC=$?; set -e
  echo "exit_code=$LAST_RC"
  expect_rc 0 "o terceiro verificou a assinatura com a chave publica"
  echo "--- prova negativa: o mesmo bundle contra um SBOM adulterado"
  { cat "$TUT_WORK/sbom_app_cyclonedx.json"; echo " "; } > "$TUT_WORK/sbom-adulterado.json"
  printf '$ cosign verify-blob --key /pub/cosign.pub --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct sbom-adulterado.json\n'
  set +e
  LAST_OUT="$(cosign_ep -v "$STATE/pub:/pub:ro" -- verify-blob --key /pub/cosign.pub \
    --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct sbom-adulterado.json 2>&1)"
  LAST_RC=$?; set -e
  printf '%s\n' "$LAST_OUT" | grep -E 'invalid signature|Error' | head -n1
  echo "exit_code=$LAST_RC"
  if [ "$LAST_RC" -ne 0 ] && printf '%s' "$LAST_OUT" | grep -q 'invalid signature'; then
    echo "RESULTADO: o SBOM adulterado foi rejeitado pelo Cosign (invalid signature)"
  else
    echo "ERRO: adulterado nao foi rejeitado como esperado"; return 1
  fi
}

# Confere um workflow de chamador contra o reutilizavel (estatico; nada de runner).
# conferencia: itens de $1 (workflow) contra $REPO_ROOT/.github/workflows/review.yml.
wf_conferir() {
  local wf="$1" reuse="$REPO_ROOT/.github/workflows/review.yml" k
  grep -qE 'uses: OWNER/AurumCode/\.github/workflows/review\.yml@[0-9a-f]{40}$' "$wf" && echo "uses: workflow reutilizavel fixado em SHA"
  grep -qE '^  id-token: write$' "$wf" && echo "permissions do chamador: id-token: write (exigido pelo keyless)"
  if grep -qE '^\s+id-token:' "$reuse"; then echo "ERRO: review.yml declara id-token"; return 1; fi
  echo "review.yml nao declara permissions de id-token: quem concede e o chamador"
  for k in $(awk '/^    with:/{w=1;next} w&&/^    [a-z]/{w=0} w&&/^      [a-z_]+:/{sub(":","",$1);print $1}' "$wf"); do
    grep -qE "^      $k:\$" "$reuse" || { echo "ERRO: input $k nao existe em review.yml"; return 1; }
    echo "input $k: existe em review.yml"
  done
}

# 3. Keyless no GitHub Actions: so conferencia estatica + flags do verify-blob.
caso_keyless_actions() {
  tut_repo keyless-actions repo-exemplo
  cad_bins
  echo "keyless: nao executado aqui, exige OIDC do GitHub Actions (Fulcio/Rekor reais)"
  echo "workflow do chamador: workflow/aurumcode-assinado.yml"
  wf_conferir "$HERE/workflow/aurumcode-assinado.yml"
  grep -q 'sigstore/cosign-installer@[0-9a-f]\{40\}' "$REPO_ROOT/.github/workflows/review.yml" && echo "review.yml instala o Cosign por action fixada em SHA"
  grep -q "cosign-release: 'v3.1.3'" "$REPO_ROOT/.github/workflows/review.yml" && echo "review.yml fixa cosign-release v3.1.3"
  grep -q 'sign_args=(sign --repo . --cosign-bin cosign)' "$REPO_ROOT/.github/workflows/review.yml" && echo "review.yml chama aurumcode sign sem chave (keyless)"
  echo "--- as flags da verificacao keyless existem no Cosign fixado (nao executada)"
  local h; h="$(cosign_ep -- verify-blob --help 2>&1)"
  for k in --certificate-identity-regexp --certificate-oidc-issuer --bundle; do
    printf '%s\n' "$h" | grep -qF -- "$k" && echo "cosign verify-blob $k: existe"
  done
  echo "RESULTADO: conferencia estatica do workflow e das flags; a assinatura keyless em si nao foi demonstrada"
}

# 4. O bundle sai do runner como artefato do job.
caso_bundle_artefato() {
  tut_repo bundle-artefato repo-exemplo
  rm -rf "$STATE/keys" "$STATE/pub"; ensure_keys >/dev/null
  local reuse="$REPO_ROOT/.github/workflows/review.yml"
  grep -q 'name: aurumcode-sbom-bundle-${{ github.event.pull_request.number }}' "$reuse" && echo "review.yml: artefato aurumcode-sbom-bundle-<PR>"
  grep -q 'path: .aurumcode-target/\*\*/\*.sigstore.json' "$reuse" && echo "review.yml: path .aurumcode-target/**/*.sigstore.json"
  grep -q 'if-no-files-found: ignore' "$reuse" && echo "review.yml: if-no-files-found: ignore"
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-wrap.sh
  expect_rc 0 "sbom assinado"
  echo "--- arquivos que o glob **/*.sigstore.json alcanca:"
  (cd "$TUT_WORK" && find . -name '*.sigstore.json' | sort)
  echo "RESULTADO: o bundle que aurumcode sign escreve casa com o glob do upload-artifact (conferido pelo script; o upload em si so ocorre no Actions)"
}

# Falha 1: o Cosign falha.
caso_falha_cosign() {
  tut_repo falha-cosign repo-exemplo
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-falha.sh
  [ "$LAST_RC" -ne 0 ] || { echo "ERRO: sign aprovou"; return 1; }
  printf '%s' "$LAST_OUT" | grep -q 'sbom /work/sbom_app_cyclonedx.json' || { echo "ERRO: o arquivo nao foi nomeado"; return 1; }
  echo "RESULTADO: sign falhou (exit $LAST_RC) e nomeou o SBOM sem assinatura"
  [ ! -e "$TUT_WORK/sbom_app_cyclonedx.json.sigstore.json" ] && echo "RESULTADO: nenhum bundle foi deixado no repositorio"
}

# Falha 2: o Cosign sai 0 mas nao escreve bundle.
caso_falha_sem_bundle() {
  tut_repo falha-sem-bundle repo-exemplo
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-sem-bundle.sh
  [ "$LAST_RC" -ne 0 ] && echo "RESULTADO: cosign saiu 0 sem bundle e mesmo assim sign falhou (exit $LAST_RC)" || { echo "ERRO: sign aprovou sem bundle"; return 1; }
}

# Falha 3: imagem sem digest e recusada antes de chamar o Cosign.
caso_falha_imagem_sem_digest() {
  tut_repo falha-imagem-sem-digest repo-exemplo imagem
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-marcador.sh --image example.com/org/app:latest
  [ "$LAST_RC" -ne 0 ] && echo "RESULTADO: tag sem digest recusada (exit $LAST_RC)" || { echo "ERRO: tag aceita"; return 1; }
  if [ -e "$TUT_WORK/cosign-chamado.txt" ]; then echo "ERRO: o cosign foi chamado"; return 1; fi
  echo "RESULTADO: o Cosign nao foi chamado (cosign-chamado.txt nao existe)"
  aurum sign --repo . --cosign-bin /fixtures/wrappers/cosign-marcador.sh --image example.com/org/app@sha256:0000000000000000000000000000000000000000000000000000000000000000
  if [ -e "$TUT_WORK/cosign-chamado.txt" ]; then
    echo "RESULTADO: com digest de 64 hex a referencia passou da validacao e o Cosign (falso) foi chamado (cosign-chamado.txt existe)"
  else
    echo "ERRO: o cosign nao foi chamado"; return 1
  fi
  expect_rc 1 "o Cosign falso saiu 0 sem bundle, e sign falhou: a recusa anterior foi a da validacao, nao a do Cosign"
}

tut_main "$@"
