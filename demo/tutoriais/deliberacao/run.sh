#!/usr/bin/env bash
# Tutorial executavel: deliberacao com ferramentas (AUR-580). Veja ../README.md
# e docs/tutorials/deliberacao.md.
#
#   run.sh diff-grande-pede-semgrep|diff-pequeno-nao-pede|estoura-rodadas|falha-semgrep-ausente
#   run.sh all | --check | limpar
#
# O modelo falso decide como um modelo real decidiria pelo que o prompt mostra:
# pede o scanner_semgrep quando o resumo da mudanca passa de 30 linhas e nao
# pede num diff pequeno. Semgrep roda DENTRO da imagem do produto, sem rede.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(diff-grande-pede-semgrep diff-pequeno-nao-pede estoura-rodadas falha-semgrep-ausente)

# 1. Diff grande: o modelo pede o Semgrep, o achado conta no gate com origem sast.
caso_diff_grande_pede_semgrep() {
  tut_repo diff-grande-pede-semgrep repo-exemplo/base repo-exemplo/grande
  aurum review --base main --auditoria auditoria.json
  expect_rc 3 "o modelo pediu o scanner_semgrep, a varredura achou eval() e o gate reprovou"
  python3 - "$TUT_WORK/auditoria.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))["deliberation"]
print("auditoria deliberation: oferecidas=%s pedidas=%s nao_pedidas=%s rodadas=%d desfecho=%s" % (
    ",".join(d["offered"]), ",".join(d["requested"]), ",".join(d["not_requested"]), d["rounds"], d["outcome"]))
for c in d["calls"]:
    print("auditoria chamada: rodada=%d ferramenta=%s argumentos=%s status=%s resultado=%s duracao_ms_registrada=%s" % (
        c["round"], c["tool"], c["arguments"], c["status"], c["result"], "duration_ms" in c))
PY
}

# 2. Diff pequeno: o modelo nao pede; o Semgrep nao roda e a auditoria registra.
caso_diff_pequeno_nao_pede() {
  tut_repo diff-pequeno-nao-pede repo-exemplo/base repo-exemplo/pequeno
  aurum review --base main --auditoria auditoria.json
  expect_rc 0 "diff pequeno: o modelo nao pediu o scanner_semgrep e ele nao rodou"
  python3 - "$TUT_WORK/auditoria.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))["deliberation"]
print("auditoria deliberation: pedidas=%s nao_pedidas=%s chamadas=%d" % (
    ",".join(d["requested"]) or "-", ",".join(d["not_requested"]), len(d["calls"])))
PY
}

# 3. Estouro de rodadas: o modelo so pede ferramenta; com max_rounds 2 a revisao
#    e inconclusiva e nenhum parecer e publicado.
caso_estoura_rodadas() {
  tut_repo estoura-rodadas repo-exemplo/base repo-exemplo/pequeno
  mkdir -p "$TUT_WORK/.aurumcode"
  sed 's/max_rounds: 3/max_rounds: 2/' "$HERE/repo-exemplo/base/.aurumcode/config.yml" > "$TUT_WORK/.aurumcode/config.yml"
  TUT_FIXTURE=fixture-rodadas.json
  aurum review --base main --auditoria auditoria.json
  expect_rc 1 "max_rounds estourado: inconclusivo, exit 1, nenhum parecer publicado"
  if [ -e "$TUT_WORK/auditoria.json" ]; then echo "ERRO: auditoria escrita para um parecer inconclusivo"; return 1; fi
  echo "nenhuma auditoria nem parecer foram escritos"
  TUT_FIXTURE=
}

# 4. Falha: o modelo pede o Semgrep e o binario nao existe => a varredura e
#    inconclusiva pela regra unica dos scanners, nunca "limpa".
caso_falha_semgrep_ausente() {
  tut_repo falha-semgrep-ausente repo-exemplo/base repo-exemplo/grande
  TUT_ENVS=(-e PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin)
  aurum review --base main
  expect_rc 1 "scanner_semgrep pedido sem binario: sast_unavailable, inconclusivo e o gate reprova"
  TUT_ENVS=()
}

tut_main "$@"
