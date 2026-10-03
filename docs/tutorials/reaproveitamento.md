# Tutorial: reaproveitamento de revisão e cache

## Objetivo

Mostrar o que o AurumCode reaproveita entre execuções, o que prova que
reaproveitou, e os casos em que **não** reaproveita: política mudou, modelo
mudou, cache corrompido. O reaproveitamento é um ganho de custo e de estabilidade;
nunca um jeito de transformar uma reprovação em aprovação (ele só acrescenta
achados, nunca remove). Seis fases executadas.

Os blocos são os arquivos de `demo/tutoriais/reaproveitamento/` e as saídas vêm
de `demo/tutoriais/reaproveitamento/out/`. Referência:
[review-cache.md](../review-cache.md) e [AUR-524](../specs/AUR-524.md).

## Pré-requisitos

- `git`, `docker`, `bash`, `python3` e a imagem do produto, como em
  [revisao.md](revisao.md); sem rede, modelo falso.
- O cache só existe com `AURUMCODE_CACHE_DIR` apontando para um diretório que
  sobrevive entre as execuções; a demo usa um diretório dentro do repositório
  descartável do caso e fixa `GITHUB_SHA`.

```bash
bash demo/tutoriais/reaproveitamento/run.sh all
bash demo/tutoriais/reaproveitamento/run.sh --check
```

## Duas camadas

1. **Por arquivo** (`--base`): o arquivo cujo diff, modelo, contexto e versão do
   prompt (um digest do conteúdo fixo do prompt, não uma constante) não mudaram
   **não é reenviado ao modelo**; a saída diz `reused N file(s) from cache`.
2. **Do veredito** (`--base` e `--pr`): o conjunto de achados de uma revisão
   **concluída** é guardado por uma chave (modelo, contexto, digest da política,
   digest do diff, SHA, identidade do repositório e do binário) e, numa execução
   seguinte com a mesma chave, é **somado** aos achados novos. Ele é invisível
   quando não acrescenta nada.

## Caso 1: mesmo SHA, política e modelo

<!-- arquivo: demo/tutoriais/reaproveitamento/politica-medium/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [medium]
review:
  context:
    skills:
      - skills/seguranca.md
```

```bash
export AURUMCODE_CACHE_DIR=/caminho/do/cache
aurumcode review --base main --politica /caminho/da/politica
aurumcode review --base main --politica /caminho/da/politica
```

<!-- saida: reuso-por-arquivo -->
```text
--- primeira execucao: cache vazio
exit_code=3
RESULTADO: o achado reprova
cache: 1 entrada(s) de veredito, 1 entrada(s) por arquivo
--- segunda execucao: mesmo SHA, politica e modelo
aurumcode review: reused 1 file(s) from cache (not resent to the model)
RESULTADO: o mesmo veredito, sem reenviar o arquivo ao modelo
```

O que observar: a segunda execução imprime `reused 1 file(s) from cache (not
resent to the model)` e chega ao mesmo exit 3; a contagem de entradas do cache
(uma de veredito e uma por arquivo) não cresce.

## Caso 2: `--base` e `--pr` compartilham o veredito?

A intenção do produto é que o gate de CI (`--pr`) e uma execução local (`--base`)
sobre o mesmo commit cheguem ao mesmo veredito. A demo tenta provar o
compartilhamento contra o GitHub falso local (`--pr` roda com `--publicar`):

<!-- saida: veredito-base-e-pr -->
```text
exit_code=3
cache: 1 entrada(s) de veredito, 1 entrada(s) por arquivo
RESULTADO: o --pr chega ao mesmo veredito
cache: 2 entrada(s) de veredito, 1 entrada(s) por arquivo
NAO DEMONSTRADO: o compartilhamento da mesma chave entre --base e --pr; neste experimento (diff do GitHub falso) cada caminho gravou a sua
```

O que observar: os dois caminhos chegam ao mesmo veredito (exit 3), mas neste
experimento cada um gravou **a sua** entrada de veredito (2 entradas, não 1).
**Não demonstrado aqui:** o compartilhamento da mesma chave entre `--base` e
`--pr`. Pode ser o diff do GitHub falso (que não é o do GitHub real) ou uma
divergência real entre as chaves; o tutorial não afirma nenhuma das duas. O
teste Go `TestAUR524AC001PRStickyFailInsideHunk` cobre o `--pr` isolado.

## Caso 3: o modelo mudou

O "modelo" da demo é o JSON da fixture; trocar o conteúdo muda a identidade do
modelo, como trocar `LLM_MODEL` ou `LLM_BASE_URL` numa execução real:

<!-- saida: modelo-mudou -->
```text
--- modelo A (a resposta tem um achado)
exit_code=3
RESULTADO: modelo A reprova
--- modelo B (outra resposta; mesmo SHA, politica e arquivos)
exit_code=0
RESULTADO: com outro modelo nada e reaproveitado: a resposta nova vale
cache: 2 entrada(s) de veredito, 2 entrada(s) por arquivo
```

O que observar: o modelo B responde sem achado e a execução sai 0; nada do
modelo A é reaproveitado (nenhuma linha `reused`) e o cache ganha entradas novas.

## Caso 4: a política mudou

<!-- saida: politica-mudou -->
```text
exit_code=0
RESULTADO: politica high: o warning passa
cache: 1 entrada(s) de veredito, 1 entrada(s) por arquivo
aurumcode review: reused 1 file(s) from cache (not resent to the model)
exit_code=3
RESULTADO: politica medium: o mesmo warning reprova; a politica nova gera outra entrada de veredito (a resposta por arquivo segue reaproveitada)
cache: 2 entrada(s) de veredito, 1 entrada(s) por arquivo
```

O que observar: com a política `high` o warning passa (exit 0); com a `medium` o
mesmo achado reprova (exit 3). A política nova gera **outra entrada de veredito**
(o digest da política faz parte da chave). A resposta do modelo por arquivo, que
não mudou, segue reaproveitada (`reused 1 file(s)`): quem reavalia é o gate, com
a política nova.

## Caso 5: cache degradado

<!-- saida: cache-degradado -->
```text
exit_code=3
RESULTADO: primeira execucao reprova e grava o cache
--- todas as entradas do cache corrompidas
RESULTADO: cache corrompido: revisao nova, o achado continua reprovando
nenhuma linha 'reused': nada foi reaproveitado do cache corrompido
```

O que observar: com todas as entradas corrompidas a revisão é feita de novo, o
achado continua reprovando (exit 3) e nada é reaproveitado. Um cache corrompido
degrada para revisão nova, nunca para aprovação nem para erro fatal.

## Quando falha

Sem `AURUMCODE_CACHE_DIR` não há reaproveitamento, e o produto declara isso. E
uma revisão inconclusiva nunca grava veredito:

<!-- saida: falha-sem-cache -->
```text
--- sem AURUMCODE_CACHE_DIR
aurumcode review: gate verdict reuse unavailable (AURUMCODE_CACHE_DIR not set): this run's verdict cannot be shared with another run, and could not reuse one either
exit_code=3
RESULTADO: sem cache a revisao funciona, mas o veredito nao e compartilhado
--- revisao inconclusiva nunca grava veredito
exit_code=1
RESULTADO: inconclusivo em block reprova
cache: 0 entrada(s) de veredito, 0 entrada(s) por arquivo
```

O que observar: `gate verdict reuse unavailable` na primeira execução (a revisão
funciona normalmente); na segunda, a cobertura parcial reprova (exit 1) e o cache
fica com 0 entradas.

## Problemas comuns

- **Nunca aparece `reused`:** `AURUMCODE_CACHE_DIR` não persiste entre as
  execuções, ou algo da chave mudou (modelo, política, contexto, diff).
- **Cache restaurado de CI:** não aponte `AURUMCODE_CACHE_DIR` de um `--base`
  usado como gate para um cache restaurado de outro job: o cache por arquivo trata
  um acerto como "revisado e limpo" (risco residual descrito em AUR-524).
- **O veredito reaproveitado não aprova:** ele só acrescenta achados; a execução
  atual nunca é descartada e uma revisão inconclusiva nunca reaproveita nem grava.
