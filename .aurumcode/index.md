---
title: AurumCode
layout: default
nav_order: 1
permalink: /
description: Code review, documentação e publicação no GitHub Pages.
---

# AurumCode

> Code review e documentação de código que chegam ao seu repositório como uma
> ferramenta simples de GitHub Actions.

{: .fs-6 .fw-300 }

[Começar em 10 minutos](guides/getting-started.html){: .btn .btn-primary }
[Ver o projeto no GitHub](https://github.com/Mpaape/AurumCode){: .btn }

## O que você encontra aqui

Esta página é a entrada para a documentação publicada. Os guias explicam o uso
da ferramenta; a referência abaixo é reconstruída diretamente do código a cada
build.

| Área | Para quem é | O que resolve |
| --- | --- | --- |
| Code review | Quem quer revisar pull requests | Achados priorizados, comentários e sugestões aplicáveis |
| Documentação | Quem mantém um projeto de código | Páginas de API e guias navegáveis no Pages |
| Revisão editorial | Quem publica documentação | Checagem de clareza, lacunas e exemplos com contexto do próprio repositório |

## Comece pelo caminho principal

1. [Guia de início rápido](guides/getting-started.html) — compile, rode o review e publique o site.
2. [Uso da Action](https://github.com/Mpaape/AurumCode/blob/main/ACTION_USAGE.md) — entradas, saídas e permissões.
3. [Configuração do GitHub Pages](https://github.com/Mpaape/AurumCode/blob/main/PAGES_SETUP.md) — a única configuração do Pages necessária.

## Como esta página é mantida

O bloco de referência abaixo é determinístico: ele lista somente páginas que o
extrator realmente produziu. Quando um provider LLM está configurado, a página
[Revisão da documentação](reviews/docs-review.html) é regenerada no mesmo ciclo
e avalia apenas o conteúdo publicado, nunca a configuração do gerador.

<!-- aurumcode:pages:start -->
<!-- aurumcode:pages:end -->
