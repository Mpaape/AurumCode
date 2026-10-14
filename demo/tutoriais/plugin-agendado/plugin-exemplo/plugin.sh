#!/usr/bin/env bash
# Plugin de exemplo OFFLINE: faz o papel do harness sem modelo e sem rede.
# Copia para <arvore>/security-scan/ o findings.json e o SARIF do harness do
# cenario pedido e sai com exit 0, como o harness quando conclui (inclusive
# quando perdeu cobertura: o ingest le degraded e o SARIF do harness).
#
#   plugin.sh <cenario> <arvore>
#   cenarios: tres-achados | misto | nao-verificado | degradado | cobertura-perdida
set -euo pipefail
cenario="$1"; arvore="$2"
aqui="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
saida="$arvore/security-scan"
rm -rf "$saida"; mkdir -p "$saida"
cp "$aqui/fixtures/$cenario/findings.json" "$saida/findings.json"
cp "$aqui/fixtures/$cenario/report.sarif" "$saida/alvo_20261014T000000_report.sarif"
exit 0
