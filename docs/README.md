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

## Evidência histórica em `tests/legacy`

Pacotes que o binário não alcança, mas que aceites de cards `done` ainda
exercitam (`evidence`, `governance/dag`, `governance/taskspec`,
`sandbox/profile`, `config/policy`, `llm/httpbase`), vivem em
`tests/legacy/<pacote>` e não em `internal/`. `apply/applycheck` fica em
`internal/apply` porque `cmd/aurumcode` o importa em teste. Um job noturno
(`acceptance-sample.yml`) roda uma amostra fixa de aceites `done`. Detalhes em
[specs/AUR-585.md](specs/AUR-585.md).
