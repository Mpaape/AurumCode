#!/bin/sh
# Wrapper passado em --trivy-bin: executa o Trivy fixado por digest (binario
# extraido da imagem fixada em images.lock pela fase build). Somente SBOM:
# o scanner de licencas nao precisa de base de vulnerabilidades, e quem
# aponta vulnerabilidades neste fluxo e o servidor de inventario.
mode="$1"; shift
exec /demo/bin/trivy "$mode" --cache-dir /tmp/trivy-cache --scanners license --skip-version-check "$@"
