#!/bin/sh
# Cosign falso: falha como um Cosign sem identidade (sem OIDC, sem chave).
echo "cosign-falso: nao foi possivel obter identidade para assinar" >&2
exit 1
