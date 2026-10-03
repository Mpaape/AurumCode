#!/usr/bin/env bash
# Tutorial executavel: benchmark de recall (AUR-564). Veja ../README.md e docs/tutorials/benchmark.md.
#
#   run.sh rodar-corpus|ler-relatorio|adicionar-caso|aprovado-com-defeito|falha-caso-sem-manifest
#   run.sh all | --check | limpar
#
# Estes casos nao chamam o aurumcode na imagem do produto: o proprio harness
# (tests/benchmark) compila e executa o binario. Go roda SO no container
# compartilhado (.board/bin/go-shared); nada de Go no host. O run.sh precisa do
# container no ar (go-shared up) e roda a partir de um worktree montado nele.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(rodar-corpus ler-relatorio adicionar-caso aprovado-com-defeito falha-caso-sem-manifest)

GOSHARED="${GO_SHARED:-$REPO_ROOT/.board/bin/go-shared}"
REPORT_REL=tests/benchmark/out/multilang-report
CORPUS_REL=tests/benchmark/testdata/multilang

# gt DIR ARGS...: roda `go test ARGS` no go-shared com cwd DIR e guarda o exit em
# LAST_RC. Os tempos do go test variam e sao trocados por <t>s (so no registro).
gt() {
  local dir="$1"; shift
  printf '$ go test %s\n' "$*"
  set +e
  LAST_OUT="$("$GOSHARED" exec -w "$dir" go test "$@" 2>&1)"; LAST_RC=$?
  set -e
  printf '%s\n' "$LAST_OUT" | sed -E 's/\([0-9.]+s\)/(<t>s)/; s/[[:space:]][0-9.]+s$/ <t>s/; s#github.com/[A-Za-z0-9]+/AurumCode#<modulo>#g'
  echo "exit_code=$LAST_RC"
}

# copia_limpa NOME: o repositorio como comitado (git archive), numa copia
# descartavel em .estado/NOME; o repositorio de verdade nunca e alterado.
copia_limpa() {
  TUT_WORK="$STATE/$1"
  rm -rf "$TUT_WORK"; mkdir -p "$TUT_WORK"
  git -C "$REPO_ROOT" archive HEAD | tar -x -C "$TUT_WORK"
}

# devolve ao usuario os arquivos que o container (root) criou na copia
devolve_dono() { "$GOSHARED" exec chown -R "$(id -u):$(id -g)" "$TUT_WORK"; }

junta_caso() {
  mkdir -p "$TUT_WORK/$CORPUS_REL/cases/java-cmd"
  cp "$HERE/caso-novo/case.json" "$HERE/caso-novo/Runner.java" "$TUT_WORK/$CORPUS_REL/cases/java-cmd/"
}

# 1. Roda o corpus de recall contra o repositorio como esta: o teste compara o
# relatorio que o harness produz com o versionado.
caso_rodar_corpus() {
  "$GOSHARED" status >/dev/null || { echo "ERRO: go-shared nao esta no ar (go-shared up)"; return 1; }
  gt "$REPO_ROOT" ./tests/benchmark -run TestAUR523 -count=1 -v
  expect_rc 0 "o harness reproduziu o relatorio versionado (corpus e politica inalterados)"
  # um relatorio regenerado numa copia e identico ao versionado
  copia_limpa rodar-corpus
  gt "$TUT_WORK" ./tests/benchmark -run TestAUR523 -count=1 -update-aur523
  devolve_dono
  if cmp -s "$TUT_WORK/$REPORT_REL.md" "$REPO_ROOT/$REPORT_REL.md" && cmp -s "$TUT_WORK/$REPORT_REL.json" "$REPO_ROOT/$REPORT_REL.json"; then
    echo "RESULTADO: -update-aur523 numa copia gerou relatorio .md e .json identicos aos versionados (byte a byte)"
  else
    echo "ERRO: o relatorio regenerado difere do versionado"; return 1
  fi
}

# 2. Le o relatorio: tabela por linguagem e total, com os intervalos.
caso_ler_relatorio() {
  echo "--- $REPORT_REL.md (versionado)"
  sed -n '/^| language/,$p' "$REPO_ROOT/$REPORT_REL.md"
  python3 - "$REPO_ROOT/$REPORT_REL.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print("modo:", d["mode"])
t = d["total"]
print("total: casos=%d defeitos=%d detectados=%d perdidos=%d achados=%d falsos_positivos=%d"
      % (t["cases"], t["defects"], t["detected"], t["missed"], t["findings"], t["false_positives"]))
print("total: recall=%s IC95=[%s, %s]" % (t["recall"], t["recall_interval_95"]["low"], t["recall_interval_95"]["high"]))
print("total: precisao=%s IC95=[%s, %s]" % (t["precision"], t["precision_interval_95"]["low"], t["precision_interval_95"]["high"]))
assert all(l["defects"] == 2 for l in d["languages"])
print("conclusao do script: toda linguagem tem so 2 defeitos; o IC95 de recall 1.0 comeca em 0.3424 (amostra pequena)")
PY
  expect_rc 0 "relatorio lido; recall e precisao sempre com intervalo de Wilson"
}

# 3. Adiciona um caso por PR: passo a passo numa copia. O corpus muda -> o teste
# recusa; -update-aur523 regenera manifest e relatorio; o teste volta a passar.
caso_adicionar_caso() {
  copia_limpa adicionar-caso
  junta_caso
  echo "--- caso novo: $CORPUS_REL/cases/java-cmd (case.json + Runner.java), na copia"
  cat "$TUT_WORK/$CORPUS_REL/cases/java-cmd/case.json"
  gt "$TUT_WORK" ./tests/benchmark -run TestAUR523 -count=1 -update-aur523
  # Achado: o teste que confere o corpus (TestAUR523RealBinaryOutputIsTheSource) roda
  # ANTES do que regenera o manifest e reprova uma vez; os arquivos sao gravados
  # na mesma execucao. Por isso o passo seguinte roda sem -update.
  expect_rc 1 "o primeiro teste reprova (caso fora do manifest); os testes seguintes gravaram manifest e relatorio"
  case "$LAST_OUT" in *"case java-cmd is on disk but not in the manifest"*) echo "RESULTADO: a recusa nomeia o caso: java-cmd esta no disco e nao esta no manifest";; *) echo "ERRO: recusa sem o nome do caso"; return 1;; esac
  devolve_dono
  echo "--- git diff do manifest e do relatorio .md (copia versus o comitado)"
  diff -u "$REPO_ROOT/$REPORT_REL.md" "$TUT_WORK/$REPORT_REL.md" | grep -E '^[+-]\| (java|total)|^[+-]- corpus' || true
  grep -c '"java-cmd"' "$TUT_WORK/$CORPUS_REL/manifest.json" | sed 's/^/entradas java-cmd no manifest: /'
  gt "$TUT_WORK" ./tests/benchmark -run TestAUR523 -count=1
  expect_rc 0 "sem -update, o teste passa com o corpus, o manifest e o relatorio novos (o caso foi aceito)"
}

# 4. "Aprovado com defeito": um defeito rotulado que o gate deixou passar (exit 0).
caso_aprovado_com_defeito() {
  python3 - "$REPO_ROOT/$REPORT_REL.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
n = 0
for l in d["languages"]:
    if l["approved_with_defect"]:
        n += l["approved_with_defect"]
        print("aprovado com defeito: linguagem=%s casos=%s" % (l["language"], ",".join(l["approved_with_defect_cases"])))
print("aprovado com defeito (total):", d["total"]["approved_with_defect"], "de", d["total"]["defects"], "defeitos rotulados")
PY
  local c=$REPO_ROOT/$CORPUS_REL/cases/php-path/case.json
  grep -E '"(label|rule|simulated_model)"' "$c"
  echo "conclusao do script: php-path e um defeito de verdade cujo modelo falso foi configurado para ficar calado (miss); o gate real aprovou (pass, exit 0) apesar do defeito rotulado"
  expect_rc 0 "o relatorio expoe o caso aprovado com defeito, com o nome do caso"
}

# Falha: caso sem manifest e recusado.
caso_falha_caso_sem_manifest() {
  copia_limpa falha-caso-sem-manifest
  junta_caso
  echo "--- o caso foi copiado para o corpus, mas o manifest NAO foi regenerado"
  gt "$TUT_WORK" ./tests/benchmark -run TestAUR523 -count=1
  devolve_dono
  if [ "$LAST_RC" -ne 0 ]; then
    echo "RESULTADO: caso sem manifest recusado pelo harness (exit_code diferente de 0)"
  else
    echo "ERRO: o harness aceitou um caso fora do manifest"; return 1
  fi
}

tut_main "$@"
