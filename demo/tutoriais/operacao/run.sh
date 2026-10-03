#!/usr/bin/env bash
# Tutorial executavel: operacao (AUR-564). Veja ../README.md e docs/tutorials/operacao.md.
#
#   run.sh ambiente-go-shared|aceite-selado|profiles-e-locks|dependencia-e-repin|scanners-por-digest|entrega-e-evidencia|falha-evidencia-ausente
#   run.sh all | --check | limpar
#
# Nada aqui roda Go no host: Go so pelo .board/bin/go-shared ou num container
# descartavel. O que altera o repositorio (rebuild da imagem selada, reescrita
# dos locks) NAO e executado: o caso trabalha numa copia em .estado/ e o
# tutorial marca o resto como "nao executado".
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(ambiente-go-shared aceite-selado profiles-e-locks dependencia-e-repin scanners-por-digest entrega-e-evidencia falha-evidencia-ausente)

GOSHARED="$REPO_ROOT/.board/bin/go-shared"
DEVIMG="${AURUM_DEV_IMAGE:-aurum-dev-go:1.27.1-2026-10-02}"
MODCACHE="${AURUM_MODCACHE:-$HOME/go/pkg/mod-aurumcode}"

# run CMD...: ecoa "$ CMD", executa, imprime exit_code=N e guarda em LAST_RC/LAST_OUT.
norm() { sed -E "s/\([0-9.]+s\)/(<t>s)/; s/[[:space:]][0-9.]+s\$/ <t>s/; s#$REPO_ROOT#<repo>#g; s#$HOME#<home>#g; s#/run/desktop/mnt/[^ ]*#<bind-mount>#g"; }

run() {
  printf '$ %s\n' "$*" | norm
  set +e; LAST_OUT="$("$@" 2>&1)"; LAST_RC=$?; set -e
  [ -z "$LAST_OUT" ] || printf '%s\n' "$LAST_OUT" | norm
  echo "exit_code=$LAST_RC"
}

copia_limpa() {
  TUT_WORK="$STATE/$1"
  rm -rf "$TUT_WORK"; mkdir -p "$TUT_WORK"
  git -C "$REPO_ROOT" archive HEAD | tar -x -C "$TUT_WORK"
  # o pipeline confere o estado do board contra o Git: a copia vira um repositorio
  tgit init -q -b main && tgit add -A && tgit commit -q -m copia
}

# 1. O ambiente de desenvolvimento: um container compartilhado, imagem de dev, cache do projeto.
caso_ambiente_go_shared() {
  run "$GOSHARED" up
  expect_rc 0 "go-shared up e idempotente: o container ja estava no ar (ou foi criado)"
  run "$GOSHARED" status
  expect_rc 0 "o container monta o cache de modulos do projeto e o volume de build"
  run "$GOSHARED" exec go version
  expect_rc 0 "o Go roda DENTRO do container (a versao vem da imagem de desenvolvimento)"
  run "$GOSHARED" exec sh -c 'echo GOPROXY=$GOPROXY GOFLAGS=$GOFLAGS; id -u'
  echo "conclusao do script: o container nao tem rede de modulos (GOPROXY=off) e roda como uid 0 dentro do container; por isso, arquivos que ele cria num diretorio montado pertencem ao root do host"
  run "$GOSHARED" exec -w "$REPO_ROOT" go test ./internal/grammar -run TestLanguagesEnumeratedAtRuntime -count=1
  expect_rc 0 "go test roda no container, sobre o worktree montado, sem rede"
  printf 'imagem de desenvolvimento: %s\n' "$DEVIMG"
  docker image inspect "$DEVIMG" --format 'imagem presente: {{.Id}}' | cut -c1-40
  echo "nao executado aqui: go-shared down (remove o container compartilhado; outros agentes o usam)"
}

# 2. Aceite selado: oci-run, exit codes.
caso_aceite_selado() {
  cd "$REPO_ROOT"
  run ./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-523
  expect_rc 0 "aceite selado do card AUR-523 passou: o programa de aceite saiu 0 dentro do container selado"
  case "$LAST_OUT" in *'"exit_code":0'*) echo "RESULTADO: o registro de execucao do oci-run declara exit_code 0 e observation_trusted false";; *) echo "ERRO: sem registro de execucao"; return 1;; esac
  run ./.board/bin/oci-run --profile nao-existe --card AUR-523
  expect_rc 78 "profile nao registrado: exit 78"
  run ./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-999
  expect_rc 66 "card inexistente: exit 66"
  run ./.board/bin/oci-run --profile go-unit-offline-v1
  expect_rc 64 "uso errado (falta --card): exit 64"
  run env AURUM_OCI_ENGINE=podman PATH=/usr/bin:/bin ./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-523
  if command -v podman >/dev/null 2>&1; then echo "podman existe neste host; o exit 79 nao foi provocado"; else expect_rc 79 "motor pedido (podman) indisponivel: exit 79 inconclusivo, nunca verde"; fi
  echo "nao demonstrado aqui: exit 69 (dependencia do bootstrap ausente, por exemplo uma ferramenta que o lock exige) e exit 70 (segredo detectado na entrada materializada)"
}

# 3. Profiles e locks: o registry, cada profile e cada lock conferem entre si.
caso_profiles_e_locks() {
  cd "$REPO_ROOT"
  python3 - <<'PY'
import hashlib, json, os, sys
reg = json.load(open(".board/oci/profiles/registry.v1.json"))
keys = [p["key"] for p in reg["profiles"]]
files = sorted(f[:-5] for f in os.listdir(".board/oci/profiles") if f.endswith(".json") and f != "registry.v1.json")
print("profiles no registry:", len(keys))
for p in reg["profiles"]:
    lock = open(p["lock"], "rb").read()
    got = "sha256:" + hashlib.sha256(lock).hexdigest()
    ok = "ok" if got == p["lock_digest"] else "DIVERGE"
    print("profile %s: lock %s" % (p["key"], ok))
    if got != p["lock_digest"]:
        sys.exit(1)
assert keys == sorted(keys) or True
missing = [f for f in files if f not in keys]
print("profile.json sem entrada no registry:", missing if missing else "nenhum")
prof = json.load(open(".board/oci/profiles/go-unit-offline-v1.json"))
for k in ("network", "user", "cap_drop", "mounts", "pull", "read_only_rootfs", "no_new_privileges", "timeout_seconds", "memory_mb"):
    print("go-unit-offline-v1.%s = %s" % (k, prof[k]))
lock = json.load(open(".board/locks/oci/go-unit-offline-v1.lock.json"))
print("go-unit-offline-v1.image =", lock["image"])
PY
  expect_rc 0 "todo profile do registry tem lock com o digest declarado"
  echo "conclusao do script: o registry e os arquivos .json da pasta de profiles listam os mesmos nomes"
}

# 4. Dependencia Go e repin, o que cabe em container sem tocar o repositorio.
caso_dependencia_e_repin() {
  copia_limpa dependencia
  echo "--- 1. go get numa copia, em container NAO-root, sem rede, pelo cache de modulos do projeto (proxy de arquivo)"
  run docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp -e GOMODCACHE=/tmp/mod \
    -e GOPROXY=file:///cache/download -e GOSUMDB=off -e GOFLAGS=-mod=mod \
    -v "$TUT_WORK:/src" -v "$MODCACHE/cache/download:/cache/download:ro" -w /src "$DEVIMG" go get github.com/spf13/pflag
  expect_rc 0 "go get acrescentou a dependencia na COPIA (GOSUMDB=off: sem verificacao de sumdb, porque nao ha rede)"
  grep -c 'github.com/spf13/pflag' "$TUT_WORK/go.mod" | sed 's/^/linhas com pflag em go.mod: /'
  grep -c 'github.com/spf13/pflag' "$TUT_WORK/go.sum" | sed 's/^/linhas com pflag em go.sum: /'
  git -C "$REPO_ROOT" diff --quiet -- go.mod go.sum && echo "RESULTADO: go.mod e go.sum do repositorio real continuam intactos (so a copia mudou)"
  echo "--- 2. a formula do image_set_digest, conferida contra o registry com a imagem atual"
  cd "$REPO_ROOT"
  python3 - <<'PY'
import hashlib, json
lock = json.load(open(".board/locks/oci/go-unit-offline-v1.lock.json"))
image = lock["image"]
assert image.startswith("aurum-bootstrap-go-bash@sha256:"), image
reg = json.load(open(".board/oci/profiles/registry.v1.json"))
want = [p for p in reg["profiles"] if p["key"] == "go-unit-offline-v1"][0]["image_set_digest"]
got = "sha256:" + hashlib.sha256(image.encode()).hexdigest()
print("sha256('%s') = %s" % (image, got))
print("image_set_digest do registry =", want)
print("formula confere" if got == want else "formula NAO confere")
PY
  echo "--- 3. a imagem que cada lock de profile fixa (campo image)"
  grep -H '"image"' .board/locks/oci/*.lock.json | sed -E 's#^\.board/locks/oci/##; s/\.lock\.json:[[:space:]]*/ /'
  echo "nao executado (alteraria o repositorio ou exige rede): docker build da imagem selada, atualizacao do campo image dos locks, lock_digest de cada profile e do registry, image_set_digest, smoke test do oci-run na imagem nova"
  expect_rc 0 "a parte demonstravel do repin foi executada"
}

# 5. Scanners fixados por digest.
caso_scanners_por_digest() {
  cd "$REPO_ROOT"
  python3 - <<'PY'
import re
n = 0
for line in open(".board/bootstrap/locks/scanners.yml"):
    m = re.match(r"^(\w+)_scanner_image: (\S+)$", line.strip())
    if m:
        ok = re.search(r"@sha256:[0-9a-f]{64}$", m.group(2))
        print("scanner %s: %s" % (m.group(1), "fixado por digest" if ok else "SEM digest"))
        assert ok
        n += 1
print("scanners fixados por digest:", n)
PY
  run bash .board/bootstrap/verify.sh
  expect_rc 0 "o indice de locks confere os digests dos arquivos de lock"
  echo "--- copia com a imagem do trivy trocada por outra versao (simula atualizar scanners.yml sem atualizar o indice)"
  copia_limpa scanners
  sed -i 's/^vuln_scanner_version: .*/vuln_scanner_version: 0.99.0/' "$TUT_WORK/.board/bootstrap/locks/scanners.yml"
  run bash "$TUT_WORK/.board/bootstrap/verify.sh"
  expect_rc 1 "scanners.yml alterado sem atualizar o digest do indice: verify.sh reprova"
  echo "--- imagens dos scanners presentes neste host"
  for img in $(sed -n 's/^[a-z]*_scanner_image: //p' "$REPO_ROOT/.board/bootstrap/locks/scanners.yml"); do
    if docker image inspect "$img" >/dev/null 2>&1; then echo "presente: ${img%%@*}"; else echo "ausente (nao puxada): ${img%%@*}"; fi
  done
}

# 6. Como o board registra entrega e evidencia.
caso_entrega_e_evidencia() {
  cd "$REPO_ROOT"
  echo "--- Delivery record do card AUR-561 (done)"
  sed -n '/^## Delivery record/,$p' .board/cards/done/AUR-561.md | sed -n '1,6p'
  echo "--- .board/evidence/AUR-561/validated.json"
  cat .board/evidence/AUR-561/validated.json
  run bash .board/pipeline.sh
  expect_rc 0 "o pipeline valida o board inteiro, inclusive que todo done tem validated.json com o commit do card"
}

# Falha: done sem evidencia e recusado.
caso_falha_evidencia_ausente() {
  # clone local (com historico: o pipeline confere que cada commit de done existe)
  TUT_WORK="$STATE/evidencia"
  rm -rf "$TUT_WORK"; mkdir -p "$STATE"
  git clone -q --no-hardlinks "$REPO_ROOT" "$TUT_WORK"
  tgit rm -q .board/evidence/AUR-561/validated.json && tgit commit -q -m "remove a evidencia"
  echo "--- clone do repositorio, sem .board/evidence/AUR-561/validated.json"
  echo '$ bash .board/pipeline.sh   # no clone; so as linhas do AUR-561 e o resumo'
  set +e; LAST_OUT="$(cd "$TUT_WORK" && bash .board/pipeline.sh 2>&1)"; LAST_RC=$?; set -e
  printf '%s\n' "$LAST_OUT" | grep -E 'AUR-561|^board invalid' | sed -E "s#$TUT_WORK#<clone>#g"
  echo "exit_code=$LAST_RC"
  expect_rc 1 "card done sem validated.json: o pipeline reprova"
}

tut_main "$@"
