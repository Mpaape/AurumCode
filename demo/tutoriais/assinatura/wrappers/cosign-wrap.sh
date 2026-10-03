#!/bin/sh
# Passado em --cosign-bin. Executa o Cosign fixado por digest (extraido de
# images.lock do guia corporativo por _lib/cadeia.sh) com a chave efemera da
# demonstracao e sem log de transparencia, para rodar sem rede. Copia de
# demo/gate-corporativo/wrappers/cosign-wrap.sh com os caminhos deste tutorial.
# Em CI o fluxo e keyless (OIDC), sem este wrapper.
mode="$1"; shift
COSIGN_PASSWORD=""
export COSIGN_PASSWORD
case "$mode" in
  sign-blob)
    exec /demo/bin/cosign sign-blob --key /keys/cosign.key --tlog-upload=false --use-signing-config=false "$@"
    ;;
  *)
    exec /demo/bin/cosign "$mode" "$@"
    ;;
esac
