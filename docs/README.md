# Documentação do AurumCode

**[Guia interativo: instalar, configurar e usar](https://mpaape.github.io/AurumCode/)**

- [Primeiro review e uso local](getting-started.md)
- [Configuração, prompts, skills e referência de opções](configuration.md)
- [Gate corporativo: SAST, SBOM, inventário e assinatura (guia e demonstração)](gate-corporativo.md)
- [Arquitetura: mapa de módulos, fluxo do review, pipeline do gate e pontos de extensão](architecture.md)
- [Qualidade e limitações atuais](review-quality.md)
- [Desenvolvimento e QA](qa.md)
- [Visão geral por capacidade (home do site pesquisável)](index.md)

O produto é gratuito e de código aberto (MIT). Instale copiando o workflow,
configure no máximo `LLM_API_KEY`, `LLM_BASE_URL` e, se necessário, `LLM_MODEL`,
e use os comandos `aurumcode review` e `aurumcode fix`. A análise determinística
funciona sem credencial; a revisão por modelo depende do endpoint escolhido.

O site publicado fica em `site/`; não requer gerador, Ruby ou framework.
O workflow de Pages publica somente esse diretório a partir de `main`.

`specs/` é um arquivo histórico da reconstrução, preservado porque o board e
suas evidências o referenciam. Não use essas especificações como manual de
instalação ou como lista de funcionalidades entregues.

## Site de documentação pesquisável

`mkdocs.yml` gera, com MkDocs Material numa imagem fixada por digest, um site
com busca de texto completo e navegação por capacidade. Nada é instalado no
host; só é preciso Docker:

```bash
scripts/docs/build.sh   # gera ./site com --strict (warning vira erro)
scripts/docs/serve.sh   # serve em http://127.0.0.1:8000
```

O workflow `docs.yml` publica esse site no GitHub Pages a cada push ao `main`.
Detalhes e prova da geração em [specs/AUR-560.md](specs/AUR-560.md).

### Capturas de tela (`scripts/docs/capturas.sh`)

Cada página de capacidade (tutoriais, guia de extensão, arquitetura,
configuração, guia corporativo e início) tem captura desktop e mobile, e cada
caso de tutorial tem a renderização do que o usuário vê (terminal, comentário
do PR e status checks, quando o caso publica), feita a partir de
`demo/tutoriais/<t>/out/<caso>.log`. Tudo é gerado por um comando, com a
imagem Playwright fixada por digest em `scripts/docs/playwright.lock` (a mesma
do CI):

```bash
scripts/docs/capturas.sh        # reescreve "Como fica", captura, gera o manifesto
scripts/docs/capturas-check.sh  # confere sem docker: manifesto, imagens, digests
```

O manifesto `docs/assets/capturas/capturas.json` registra, por imagem, o
digest do insumo (`out/` ou página-fonte), a imagem Playwright e o comando. Ele
não registra o digest do PNG nem data: duas execuções sobre os mesmos insumos
dão o mesmo manifesto e o mesmo conjunto de arquivos, enquanto os bytes do PNG
podem variar entre máquinas. Quem muda um `out/` (ou uma página capturada)
roda `capturas.sh` de novo; senão `capturas-check.sh`, o `docs.yml` e o CI
reprovam. O teste em Chromium do site construído está em
`tests/docs/mkdocs.test.cjs` ([specs/AUR-588.md](specs/AUR-588.md)).

## Evidência histórica em `tests/legacy`

Pacotes que o binário não alcança, mas que aceites de cards `done` ainda
exercitam (`evidence`, `governance/dag`, `governance/taskspec`,
`sandbox/profile`, `config/policy`, `llm/httpbase`), vivem em
`tests/legacy/<pacote>` e não em `internal/`. `apply/applycheck` fica em
`internal/apply` porque `cmd/aurumcode` o importa em teste. Detalhes em
[specs/AUR-585.md](specs/AUR-585.md). Um job noturno (`acceptance-sample.yml`)
faz a varredura completa dos aceites de cards `done` em 6 shards, na mesma imagem
Go do CI: cada aceite precisa sair com o estado da tabela de
[specs/AUR-589.md](specs/AUR-589.md) (0 verde, ou 69 aposentado com motivo de
produto), entao podridao nova aparece no dia seguinte.
