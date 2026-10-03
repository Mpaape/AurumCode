# Tutorial: assinatura do SBOM com `aurumcode sign` (Cosign)

## Objetivo

Ao final você terá assinado um SBOM CycloneDX com `aurumcode sign`, offline e
com uma chave efêmera; terá verificado a assinatura como um terceiro, só com a
chave pública; terá visto como a assinatura keyless é ligada no GitHub Actions
e como o bundle sai do runner como artefato; e terá visto três falhas que o
comando nunca transforma em aprovação.

Cada comando e saída abaixo vêm de uma execução real, registrada em
`demo/tutoriais/assinatura/out/` e conferida por `run.sh --check`. Os blocos de
configuração **são os arquivos de `demo/tutoriais/assinatura/`**, byte a byte
(`tests/acceptance/AUR-563.sh AC-004` compara).

O que **não** é demonstrado aqui, e leva a marca "não executado": a assinatura
**keyless** (exige o OIDC do GitHub Actions, Fulcio e Rekor reais) e o upload do
artefato do job (só existe no Actions). Eles aparecem como workflow e como
conferência estática, nunca como execução.

## Pré-requisitos

- `git`, `docker`, `bash` e `python3`.
- A imagem do produto, construída do `Dockerfile` da raiz (a demonstração a
  constrói sozinha com a tag `aurum-tutoriais:<12 hex>` derivada da árvore):

```bash
docker build -t aurumcode:local /caminho/para/AurumCode
```

- O Cosign **fixado por digest**: é a mesma imagem do guia corporativo
  (`demo/gate-corporativo/images.lock`, chave `cosign`). `_lib/cadeia.sh` lê
  esse arquivo (nenhum digest é copiado para cá) e extrai o binário estático
  para `.estado/bin/`. A versão é a v3.1.3, a mesma que o workflow reutilizável
  instala.
- Sem rede: todos os casos rodam com `--network none`.

```bash
bash demo/tutoriais/assinatura/run.sh all      # executa os casos e grava out/
bash demo/tutoriais/assinatura/run.sh --check  # compara out/ com expected/, sem docker
```

A primeira execução baixa a imagem do Cosign por digest (a demonstração não a
remove sozinha; remova com `docker rmi` se quiser liberar o espaço).

O que `aurumcode sign` precisa ver no repositório é só a configuração:

<!-- arquivo: demo/tutoriais/assinatura/repo-exemplo/.aurumcode/config.yml -->
```yaml
quality_gates:
  ssor_dtrack:
    enabled: false
    sbom_generator:
      tool: trivy
      format: cyclonedx
      spec_version: "1.6"
      output_file: sbom_app_cyclonedx.json
  supply_chain:
    engine: cosign
    sign_sbom: true
    sign_artifacts: false
```

`sign_sbom: true` assina o arquivo de `sbom_generator.output_file`; o SBOM de
exemplo é um CycloneDX mínimo escrito à mão (para gerar um de verdade, veja o
tutorial de SBOM):

<!-- arquivo: demo/tutoriais/assinatura/repo-exemplo/sbom_app_cyclonedx.json -->
```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.6",
  "version": 1,
  "metadata": {"component": {"type": "application", "name": "servico-exemplo", "version": "1.0.0"}},
  "components": [
    {"type": "library", "name": "left-pad", "version": "1.3.0", "purl": "pkg:npm/left-pad@1.3.0"}
  ]
}
```

O `--cosign-bin` aponta para um wrapper que executa o Cosign real com a chave
efêmera da demonstração e sem log de transparência (para rodar sem rede). É uma
cópia do wrapper do guia corporativo, com os caminhos deste tutorial. Em CI não
há wrapper: o fluxo é keyless.

<!-- arquivo: demo/tutoriais/assinatura/wrappers/cosign-wrap.sh -->
```bash
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
```

## Caso 1: assinar com chave efêmera, offline

A chave nasce no caso (`cosign generate-key-pair`, senha vazia) em `.estado/keys/`,
diretório ignorado pelo git; nenhuma chave é versionada.

```bash
aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-wrap.sh
```

<!-- saida: chave-efemera -->
```text
cosign: Private key written to cosign.key
$ aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-wrap.sh
sign: sbom /work/sbom_app_cyclonedx.json -> /work/sbom_app_cyclonedx.json.sigstore.json
exit_code=0
RESULTADO: sign terminou com exit 0 e relatou o bundle
RESULTADO: o bundle sbom_app_cyclonedx.json.sigstore.json existe e nao esta vazio (conferido pelo script)
bundle: campos de primeiro nivel: mediaType,messageSignature,verificationMaterial
```

O que observar: `sign: sbom ... -> ....sigstore.json` é o bundle ao lado do SBOM.
O script confirma que o arquivo existe e não é vazio; os campos de primeiro
nível (`mediaType`, `messageSignature`, `verificationMaterial`) são o formato de
bundle do Sigstore. As linhas `RESULTADO:` e `bundle:` são **conclusões do
script**, não do produto.

## Caso 2: verificação por terceiro

Quem recebe o SBOM e o bundle verifica com o Cosign e a chave **pública**; o
terceiro nunca vê a chave privada (no caso, só `cosign.pub` é montado no
container de verificação). `cosign verify-blob` é do Cosign, não do `aurumcode`;
a demonstração só mostra que o bundle que `aurumcode sign` escreveu é aceito.

<!-- saida: verificacao-por-terceiro -->
```text
exit_code=0
RESULTADO: sbom assinado
$ cosign verify-blob --key /pub/cosign.pub --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct sbom_app_cyclonedx.json
Verified OK
exit_code=0
RESULTADO: o terceiro verificou a assinatura com a chave publica
$ cosign verify-blob --key /pub/cosign.pub --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct sbom-adulterado.json
exit_code=1
RESULTADO: o SBOM adulterado foi rejeitado pelo Cosign (invalid signature)
```

O que observar: `Verified OK` com o SBOM original; com o mesmo bundle contra uma
cópia adulterada (um espaço a mais) o Cosign recusa (`invalid signature`) e sai 1.
O aviso sobre a tlog (`--insecure-ignore-tlog`) existe porque a chave é local, sem
Rekor; em produção keyless o log de transparência faz parte da prova.

## Caso 3: keyless no GitHub Actions (não executado aqui)

Em CI não há chave. O Cosign usa a identidade OIDC do próprio job, e isso exige
`id-token: write` no **chamador**: o workflow reutilizável (`review.yml`) nunca
declara `permissions` de `id-token`, porque um reutilizável não pode conceder a
si mesmo o que o chamador não tem.

<!-- arquivo: demo/tutoriais/assinatura/workflow/aurumcode-assinado.yml -->
```yaml
# .github/workflows/aurumcode.yml do repositorio que assina o SBOM (keyless).
# OWNER e o SHA sao placeholders: troque pelos seus.
name: AurumCode
on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: write
  statuses: write
  checks: read
  id-token: write

jobs:
  review:
    uses: OWNER/AurumCode/.github/workflows/review.yml@0000000000000000000000000000000000000000
    with:
      security: true
    secrets:
      LLM_API_KEY: ${{ secrets.LLM_API_KEY }}
      LLM_BASE_URL: ${{ secrets.LLM_BASE_URL }}
```

Com `sign_sbom: true` na configuração do repositório, o workflow assina o SBOM
depois de um review bem-sucedido. **Keyless: não executado aqui, exige OIDC do
GitHub Actions.** O que o script faz é conferência estática: o `uses` está
fixado por SHA, o chamador concede `id-token: write`, os `with` existem no
reutilizável, o `review.yml` fixa o Cosign por action em SHA e chama
`aurumcode sign` sem chave, e as flags da verificação keyless existem no Cosign
fixado.

<!-- saida: keyless-actions -->
```text
keyless: nao executado aqui, exige OIDC do GitHub Actions (Fulcio/Rekor reais)
permissions do chamador: id-token: write (exigido pelo keyless)
review.yml nao declara permissions de id-token: quem concede e o chamador
review.yml instala o Cosign por action fixada em SHA
review.yml fixa cosign-release v3.1.3
review.yml chama aurumcode sign sem chave (keyless)
cosign verify-blob --certificate-identity-regexp: existe
cosign verify-blob --certificate-oidc-issuer: existe
cosign verify-blob --bundle: existe
RESULTADO: conferencia estatica do workflow e das flags; a assinatura keyless em si nao foi demonstrada
```

O que observar: a última linha diz o que foi e o que não foi provado. Quem baixa
o bundle verifica sem chave do projeto (comando **não executado aqui**; só as
flags foram conferidas contra o Cosign):

```bash
export IDENTIDADE_REGEX='<regex da identidade do workflow: dono/repositorio/.github/workflows/...>'
export EMISSOR_OIDC='<emissor OIDC do GitHub Actions, como publicado na documentacao do GitHub>'
cosign verify-blob --bundle sbom_app_cyclonedx.json.sigstore.json --certificate-identity-regexp "$IDENTIDADE_REGEX" --certificate-oidc-issuer "$EMISSOR_OIDC" sbom_app_cyclonedx.json
```

O valor exato do emissor e o formato da regex estão em
`docs/configuration.md` ("Verificação por terceiros"); este tutorial usa
placeholders para não citar domínios reais.

## Caso 4: o bundle como artefato do job

Um bundle que só existe no disco efêmero do runner não é verificável por
ninguém. O `review.yml` o envia como artefato `aurumcode-sbom-bundle-<PR>` (glob
`.aurumcode-target/**/*.sigstore.json`, `if-no-files-found: ignore`). O caso confere o
texto do `review.yml` e que o arquivo que `aurumcode sign` escreve casa com o glob.
**O upload em si não foi executado**: só acontece no Actions.

<!-- saida: bundle-artefato -->
```text
review.yml: artefato aurumcode-sbom-bundle-<PR>
review.yml: path .aurumcode-target/**/*.sigstore.json
review.yml: if-no-files-found: ignore
RESULTADO: sbom assinado
./sbom_app_cyclonedx.json.sigstore.json
RESULTADO: o bundle que aurumcode sign escreve casa com o glob do upload-artifact (conferido pelo script; o upload em si so ocorre no Actions)
```

O que observar: as linhas `review.yml:` são `grep` no workflow real; a última é
conclusão do script sobre o nome do arquivo.

## Quando falha

Assinar não tem modo "warn": qualquer falha reprova o comando (exit 1) e nomeia
o que ficou sem assinatura.

### Falha 1: o Cosign falha

O Cosign falso sai 1.

<!-- saida: falha-cosign -->
```text
$ aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-falha.sh
aurumcode sign: supplychain: signing sbom /work/sbom_app_cyclonedx.json: cosign sign-blob --yes --bundle /work/sbom_app_cyclonedx.json.sigstore.json -- /work/sbom_app_cyclonedx.json: exit status 1: cosign-falso: nao foi possivel obter identidade para assinar
exit_code=1
RESULTADO: sign falhou (exit 1) e nomeou o SBOM sem assinatura
RESULTADO: nenhum bundle foi deixado no repositorio
```

O que observar: exit 1, a mensagem traz o arquivo (`/work/sbom_app_cyclonedx.json`),
o comando do Cosign e o erro dele; nenhum bundle fica no repositório.

### Falha 2: o Cosign sai 0 sem escrever o bundle

<!-- saida: falha-sem-bundle -->
```text
$ aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-sem-bundle.sh
aurumcode sign: supplychain: signing sbom /work/sbom_app_cyclonedx.json: cosign exited 0 but wrote no signature bundle to /work/sbom_app_cyclonedx.json.sigstore.json: stat /work/sbom_app_cyclonedx.json.sigstore.json: no such file or directory
exit_code=1
RESULTADO: cosign saiu 0 sem bundle e mesmo assim sign falhou (exit 1)
```

O que observar: um Cosign que "deu certo" sem produzir o bundle é tratado como
falha (`cosign exited 0 but wrote no signature bundle`). O produto confere o
arquivo, não o exit do Cosign.

### Falha 3: imagem sem digest

Com `sign_artifacts: true`, a imagem precisa vir por digest. Este caso usa outra
configuração:

<!-- arquivo: demo/tutoriais/assinatura/imagem/.aurumcode/config.yml -->
```yaml
quality_gates:
  supply_chain:
    engine: cosign
    sign_sbom: false
    sign_artifacts: true
```

```bash
aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-marcador.sh --image example.com/org/app:latest
```

<!-- saida: falha-imagem-sem-digest -->
```text
$ aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-marcador.sh --image example.com/org/app:latest
aurumcode sign: image reference "example.com/org/app:latest" must be pinned by digest (@sha256:<64 hex>), not a tag
exit_code=1
RESULTADO: tag sem digest recusada (exit 1)
RESULTADO: o Cosign nao foi chamado (cosign-chamado.txt nao existe)
$ aurumcode sign --repo . --cosign-bin /fixtures/wrappers/cosign-marcador.sh --image example.com/org/app@sha256:0000000000000000000000000000000000000000000000000000000000000000
exit_code=1
RESULTADO: com digest de 64 hex a referencia passou da validacao e o Cosign (falso) foi chamado (cosign-chamado.txt existe)
RESULTADO: o Cosign falso saiu 0 sem bundle, e sign falhou: a recusa anterior foi a da validacao, nao a do Cosign
```

O que observar: a tag é recusada **antes** de chamar o Cosign (o marcador
`cosign-chamado.txt` não existe). Com um digest de 64 hex a validação passa e o
Cosign falso é chamado; como ele não escreve bundle, o comando falha pelo motivo do
caso 2. A assinatura real de imagem não foi demonstrada (exigiria registry).

## Problemas comuns

- **`quality_gates.supply_chain nao declarado; nada a fazer`**: sem a seção o
  comando sai 0 sem assinar. Uma pipeline que "assina" sem a seção não assinou.
- **Keyless falha com erro de identidade**: falta `id-token: write` no
  workflow ou job **chamador**.
- **`--image` com tag**: recusado; fixe por `@sha256:<64 hex>`.
- **`verify-blob` com a chave local e sem `--insecure-ignore-tlog`**: o bundle
  da demonstração não tem prova de Rekor; a flag existe só para esse caso. Em
  keyless real, não a use.
- **A chave privada**: nunca vá para o repositório. Aqui ela vive em `.estado/`,
  ignorado pelo git.
