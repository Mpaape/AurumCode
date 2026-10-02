#!/bin/sh
# Wrapper passado em --cosign-bin: executa o Cosign fixado por digest com um
# par de chaves efemero (gerado pela fase pass) e sem log de transparencia,
# para rodar sem rede. Em CI o fluxo e keyless (OIDC), sem este wrapper.
mode="$1"; shift
COSIGN_PASSWORD=""
export COSIGN_PASSWORD
case "$mode" in
  sign-blob)
    exec /demo/bin/cosign sign-blob --key /demo/keys/cosign.key --tlog-upload=false --use-signing-config=false "$@"
    ;;
  *)
    exec /demo/bin/cosign "$mode" "$@"
    ;;
esac
