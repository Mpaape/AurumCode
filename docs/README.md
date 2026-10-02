# Documentação do AurumCode

**[Guia interativo: instalar, configurar e usar](https://mpaape.github.io/AurumCode/)**

- [Primeiro review e uso local](getting-started.md)
- [Configuração, prompts, skills e referência de opções](configuration.md)
- [Gate corporativo: SAST, SBOM, inventário e assinatura (guia e demonstração)](gate-corporativo.md)
- [Qualidade e limitações atuais](review-quality.md)
- [Desenvolvimento e QA](qa.md)

O produto é gratuito e de código aberto (MIT). Instale copiando o workflow,
configure no máximo `LLM_API_KEY`, `LLM_BASE_URL` e, se necessário, `LLM_MODEL`,
e use os comandos `aurumcode review` e `aurumcode fix`. A análise determinística
funciona sem credencial; a revisão por modelo depende do endpoint escolhido.

O site publicado fica em `site/`; não requer gerador, Ruby ou framework.
O workflow de Pages publica somente esse diretório a partir de `main`.

`specs/` é um arquivo histórico da reconstrução, preservado porque o board e
suas evidências o referenciam. Não use essas especificações como manual de
instalação ou como lista de funcionalidades entregues.
