# Tutorial: xBOM (Build BOM e CBOM)

## Objetivo

Ao final você terá gerado, com `aurumcode xbom`, um **Build BOM** (as Actions
de terceiros e as imagens base da esteira) e um **CBOM** (os algoritmos
criptográficos citados em código e configuração) de um repositório-exemplo, e
terá visto: a evidência por arquivo e linha de cada componente, o catálogo do
repositório contra o da política central, o enriquecimento por modelo (fixture)
com descarte do que não tem evidência, e o que acontece com os tipos só
documentados (AIBOM, SaaSBOM, NetBOM) e com um catálogo inválido.

Cada comando e cada saída vêm de uma execução real, registrada em
`demo/tutoriais/xbom/out/` e conferida por `run.sh --check`. Os blocos de
configuração **são os arquivos de `demo/tutoriais/xbom/`**, byte a byte. Linhas
`RESULTADO:` e as conferências "do script" são conclusões do script da
demonstração (a condição testada está no `run.sh`), não saída do produto.

## Pré-requisitos

- `git`, `docker` e `bash`. O `xbom` não usa rede nem credencial.
- A imagem do produto, construída do `Dockerfile` da raiz:

```bash
docker build -t aurumcode:local /caminho/para/AurumCode
```

- Um atalho para chamar o programa dentro do repositório que você analisa:

```bash
export TUTORIAL_DIR=/caminho/para/AurumCode/demo/tutoriais/xbom
alias aurumcode='docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -e AURUMCODE_LLM_FIXTURE -v "$PWD:/work" -w /work -v "$TUTORIAL_DIR:/fixtures:ro" --entrypoint /app/aurumcode aurumcode:local'
```

Em uso real o modelo é um serviço compatível com OpenAI (`LLM_API_KEY` e
`LLM_BASE_URL`); **aqui nenhuma credencial é usada**: o provedor é um arquivo
JSON determinístico (`AURUMCODE_LLM_FIXTURE`).

### Rodar a demonstração

```bash
bash demo/tutoriais/xbom/run.sh all      # executa os casos e grava out/
bash demo/tutoriais/xbom/run.sh --check  # compara out/ com expected/, sem docker
```

Cada caso cria um repositório descartável em `demo/tutoriais/xbom/.estado/`
(ignorado pelo git). Para inspecionar o JSON dentro da imagem (que traz `jq`):
`docker run --rm --entrypoint jq -v "$PWD:/work:ro" -w /work aurumcode:local . build-bom.json`.

## O repositório-exemplo

Quatro arquivos pequenos dão ao `xbom` o que coletar. Os números de linha que
aparecem nas saídas são as linhas destes arquivos.

<!-- arquivo: demo/tutoriais/xbom/repo-exemplo/.github/workflows/ci.yml -->
```yaml
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@0123456789abcdef0123456789abcdef01234567
      - uses: actions/setup-go@v5
      - uses: example/deploy-action@v1.2.0
```

<!-- arquivo: demo/tutoriais/xbom/repo-exemplo/Dockerfile -->
```dockerfile
FROM golang:1.22 AS build
WORKDIR /src
COPY . .
FROM alpine:3.20
COPY --from=registry.example.com/library/busybox:1.36 /bin/busybox /bin/busybox
COPY --from=build /src/app /app
```

<!-- arquivo: demo/tutoriais/xbom/repo-exemplo/src/seguranca.go -->
```go
package seguranca

// Resumo do arquivo: SHA-256.
// Cifra dos dados em repouso: AES-256-GCM.
// Troca de chaves pos-quantica: ML-KEM-768.
func Algoritmos() []string { return []string{"sha256", "aes-256-gcm", "ml-kem-768"} }
```

<!-- arquivo: demo/tutoriais/xbom/repo-exemplo/config/app.yml -->
```yaml
tls:
  min_version: TLSv1.3
legado:
  checksum: md5
```

## Caso 1: Build BOM

```bash
aurumcode xbom --type build --repo . --out build-bom.json
```

O `build` coleta os `uses:` de workflows do GitHub Actions (com versão ou SHA)
e os `FROM` de Dockerfiles. Sem `--out` o JSON vai para a saída padrão; com
`--out` o arquivo só aparece depois de validado (CycloneDX 1.6). Sem provedor de
modelo o BOM contém só a evidência determinística.

<!-- saida: build-bom -->
```text
$ aurumcode xbom --type build --repo . --out build-bom.json
aurumcode xbom: build: 5 componente(s), 0 descartado(s) sem evidencia
actions/checkout@0123456789abcdef0123456789abcdef01234567 -> .github/workflows/ci.yml:7
alpine@3.20 -> Dockerfile:4
golang@1.22 -> Dockerfile:1
aurumcode:xbom:llm=absent
RESULTADO: Build BOM gerado e validado (CycloneDX 1.6) sem provedor de modelo
```

O que observar: cinco componentes, cada um com o arquivo e a linha de onde veio;
`aurumcode:xbom:catalog=embedded` (catálogo embutido no binário) e
`aurumcode:xbom:llm=absent` (nenhum modelo participou). Note que o `COPY
--from=...busybox` da linha 5 do Dockerfile **não** é coletado: o catálogo
padrão só conhece `FROM`. É isso que o caso 5 usa.

## Caso 2: CBOM

```bash
aurumcode xbom --type cbom --repo . --out cbom.json
```

O `cbom` procura algoritmos, modos, tamanhos de chave, hashes e protocolos TLS
em código e configuração e os descreve com `cryptoProperties` do CycloneDX 1.6;
algoritmos pós-quânticos recebem a propriedade `aurumcode:xbom:pqc`.

<!-- saida: cbom -->
```text
aurumcode xbom: cbom: 5 componente(s), 0 descartado(s) sem evidencia
AES-256-GCM@- -> src/seguranca.go:4
MD5@- -> config/app.yml:4
MLKEM768
```

O que observar: o `MD5` achado em `config/app.yml:4` está no inventário mesmo
sendo um algoritmo fraco: o BOM **inventaria**, não julga. `MLKEM768` (escrito
`ML-KEM-768` no código) é o único ativo marcado como pós-quântico. O nome é
normalizado pelo catálogo (`nodash`/`upper`).

## Caso 3: evidência por arquivo e linha

Cada componente carrega `evidence.occurrences[]` com `location` e `line`. Antes
de escrever, o gerador reabre a linha citada e exige que ela contenha o token de
evidência. O caso confere isso **de fora**, com um script que relê os arquivos
do repositório-exemplo e testa cada ocorrência do CBOM do caso 2:

<!-- saida: evidencia -->
```text
evidencia ok: src/seguranca.go:5 contem 'ML-KEM-768'
ocorrencias sem evidencia: 0
RESULTADO: toda ocorrencia do CBOM cita uma linha que contem o token (conferencia do script)
```

O que observar: o mesmo algoritmo aparece em mais de uma linha (`sha256` na
linha 6 além de `SHA-256` na 3), e cada ocorrência é um par arquivo e linha que
você pode abrir. A conferência é do script da demonstração; no produto a mesma
regra é aplicada antes de gravar o arquivo.

## Caso 4: catálogo do repositório contra o da política

O que procurar vem de um catálogo YAML por tipo. A ordem é: o da política central
(`--politica`) vence, senão o do repositório (`.aurumcode/xbom/<tipo>.yml`), senão
o embutido. Quando a política traz o catálogo do tipo, o do repositório é
**ignorado com um aviso**. Dois catálogos quase iguais, só a propriedade
`aurumcode:xbom:catalogo` difere:

<!-- arquivo: demo/tutoriais/xbom/repo-catalogo/.aurumcode/xbom/build.yml -->
```yaml
version: 1
type: build
exclude: [".git/**"]
additional:
  name_pattern: '^[^\s]*[/.:@][^\s]*$'
  reject_tokens: [FROM, RUN, AS, uses]
entries:
  - id: repo-github-actions
    files: [".github/workflows/*.yml"]
    pattern: '^\s*-?\s*uses:\s*(?P<name>[\w./-]+)@(?P<version>[\w.-]+)'
    token: name
    component:
      type: application
      name: "{name}"
      version: "{version}"
      purl: "pkg:githubactions/{name}@{version}"
      properties: {"aurumcode:xbom:ecosystem": github-actions, "aurumcode:xbom:catalogo": repositorio}
```

<!-- arquivo: demo/tutoriais/xbom/politica/.aurumcode/xbom/build.yml -->
```yaml
version: 1
type: build
exclude: [".git/**"]
additional:
  name_pattern: '^[^\s]*[/.:@][^\s]*$'
  reject_tokens: [FROM, RUN, AS, uses]
entries:
  - id: politica-github-actions
    files: [".github/workflows/*.yml"]
    pattern: '^\s*-?\s*uses:\s*(?P<name>[\w./-]+)@(?P<version>[\w.-]+)'
    token: name
    component:
      type: application
      name: "{name}"
      version: "{version}"
      purl: "pkg:githubactions/{name}@{version}"
      properties: {"aurumcode:xbom:ecosystem": github-actions, "aurumcode:xbom:catalogo": politica}
```

A política precisa de um `.aurumcode/config.yml` válido:

<!-- arquivo: demo/tutoriais/xbom/politica/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [error]
```

```bash
aurumcode xbom --type build --repo . --out repo.json
aurumcode xbom --type build --repo . --politica /fixtures/politica --out politica.json
```

<!-- saida: catalogo -->
```text
actions/checkout catalogo=repositorio
aurumcode:xbom:catalog=repository
aurumcode xbom: .aurumcode/xbom/build.yml do repositorio ignorado: a politica central decide sozinha
actions/checkout catalogo=politica
aurumcode:xbom:catalog=policy
RESULTADO: catalogo da politica usado, o do repositorio ignorado
```

O que observar: só três componentes (as Actions): os catálogos do exemplo não
têm a regra de `FROM`, e um catálogo que substitui o embutido **substitui por
inteiro**, não soma. Sob a política, o repositório não consegue trocar o que se
coleta.

## Caso 5: enriquecimento pelo modelo e descarte sem evidência

Com um provedor, o modelo classifica os candidatos (descrição, propriedades
`aurumcode:xbom:llm:*`, `keep: false` para falso positivo) e pode propor
componentes **adicionais** citando arquivo e linha. Nada do que ele propõe entra
sem a mesma verificação de evidência: o nome precisa casar `additional.name_pattern`,
não estar em `reject_tokens` e aparecer na linha citada. A resposta do modelo
desta demonstração (a fixture) propõe quatro coisas:

<!-- arquivo: demo/tutoriais/xbom/fixture-llm.json -->
```json
{"candidates":[
  {"id":"c1","description":"checkout do repositorio","properties":{"funcao":"obter-codigo"}},
  {"id":"c3","keep":false}
 ],
 "additional":[
  {"type":"container","name":"registry.example.com/library/busybox","version":"1.36","purl":"pkg:docker/registry.example.com/library/busybox@1.36","occurrences":[{"location":"Dockerfile","line":5}]},
  {"type":"library","name":"fantasma.example.com/lib","version":"9.9","occurrences":[{"location":"Dockerfile","line":1}]},
  {"type":"library","name":"inexistente.example.org/lib","occurrences":[{"location":"nao/existe.yml","line":3}]},
  {"type":"library","name":"FROM"}
 ]}
```

Um componente legítimo (`busybox`, linha 5 do Dockerfile, que contém o nome),
um citando uma linha que não contém o nome, um citando um arquivo inexistente e
a palavra `FROM`. Além disso exclui o candidato `c3` (`keep: false`) e descreve o
`c1`.

```bash
AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json aurumcode xbom --type build --repo . --out build-bom.json
```

<!-- saida: enriquecimento -->
```text
aurumcode xbom: build: 5 componente(s), 2 descartado(s) sem evidencia
actions/checkout@0123456789abcdef0123456789abcdef01234567 -> .github/workflows/ci.yml:7 descricao=checkout do repositorio
registry.example.com/library/busybox@1.36 -> Dockerfile:5
aurumcode:xbom:dropped_without_evidence=2
aurumcode:xbom:llm=present
aurumcode:xbom:llm_excluded=1
aurumcode:xbom:llm_rejected=1
RESULTADO: BOM enriquecido pelo modelo (fixture) e verificado
```

O que observar: o `busybox` entrou e aponta `Dockerfile:5`; os dois
componentes sem evidência verificável foram descartados e contados em
`dropped_without_evidence=2`; `FROM` foi rejeitado pela regra do catálogo
(`llm_rejected=1`); o candidato excluído pelo modelo some do BOM
(`llm_excluded=1`; é o `example/deploy-action`, por ordem de coleta) e a
descrição aparece só onde o modelo a deu. O token que o modelo envia é
ignorado: a evidência do componente adicional é o próprio nome.

## Caso 6: tipos só documentados e tipo desconhecido

```bash
aurumcode xbom --type aibom --repo .
aurumcode xbom --type sbomx --repo .
```

`aibom`, `saasbom` e `netbom` são **definidos, não gerados**: o comando sai com
código 2 e aponta a seção da documentação com o formato e o envio ao
Dependency-Track. Um `--type` que não existe sai com 64.

<!-- saida: tipos-documentados -->
```text
aurumcode xbom: o tipo "aibom" nao e gerado; formato e envio estao definidos em docs/configuration.md#xbom-aibom
aurumcode xbom: o tipo "saasbom" nao e gerado; formato e envio estao definidos em docs/configuration.md#xbom-saasbom
aurumcode xbom: o tipo "netbom" nao e gerado; formato e envio estao definidos em docs/configuration.md#xbom-netbom
exit_code=64
RESULTADO: tipo desconhecido: exit 64
```

O que observar: o produto não inventa um AIBOM: ele diz onde o formato está
definido. Gerar esses BOMs hoje é trabalho manual ou de outra ferramenta; o
formato e o `POST /api/v1/bom` estão na seção de xBOM de
`docs/configuration.md`. Esse envio **não é demonstrado aqui**.

## Quando falha

Um catálogo inválido (aqui, sem a regra `additional` obrigatória) é erro de
configuração, exit 2: nunca volta em silêncio ao catálogo embutido, e nenhum
arquivo parcial é deixado.

<!-- arquivo: demo/tutoriais/xbom/repo-invalido/.aurumcode/xbom/build.yml -->
```yaml
version: 1
type: build
entries:
  - id: sem-regra-additional
    files: ["Dockerfile"]
    pattern: '^FROM\s+(?P<name>\S+)'
    token: name
    component: {type: container, name: "{name}"}
```

```bash
aurumcode xbom --type build --repo . --out nao-deve-existir.json
```

<!-- saida: falha-catalogo-invalido -->
```text
aurumcode xbom: catalog build (repository): additional.name_pattern is required
exit_code=2
RESULTADO: nenhum arquivo foi escrito (conferido pelo script)
```

O que observar: a mensagem nomeia o catálogo e a regra que falta; o script
confirma que `nao-deve-existir.json` não existe. Exit 1 é falha de geração,
escrita ou validação do BOM; 2, configuração inválida ou tipo só documentado; 64,
tipo desconhecido.

## Problemas comuns

- **Componente que você esperava não aparece.** O catálogo embutido só conhece o
  que está nele (`uses:` com `owner/repo@ref`, `FROM`). Confira a linha e o
  catálogo; um catálogo seu **substitui** o embutido por inteiro (caso 4).
- **"descartado(s) sem evidencia".** Um componente proposto pelo modelo cuja
  linha citada não contém o nome, ou cujo arquivo não existe (caso 5). É o
  comportamento esperado, não um erro.
- **`llm=absent`.** Nenhum provedor configurado: o BOM é só evidência
  determinística. Com provedor falhando, `llm=failed` e a evidência é mantida.
- **Catálogo do repositório "ignorado".** Sob `--politica` (ou `AURUMCODE_POLICY`)
  a política decide sozinha, com o aviso do caso 4.
- **Arquivos com dono root no seu diretório.** Rode a imagem com
  `--user "$(id -u):$(id -g)"`, como o atalho acima.

## Não demonstrado aqui

- O envio de um xBOM ao Dependency-Track (`POST /api/v1/bom`); veja o tutorial de
  SBOM e a seção de xBOM de `docs/configuration.md`.
- Um modelo real: a fixture é determinística; um modelo de verdade pode
  classificar diferente (a verificação de evidência vale igual).
- O sobrescrito de prompt por política (`.aurumcode/xbom/<tipo>.md`).
