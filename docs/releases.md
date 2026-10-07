# Releases

Uma release oficial do AurumCode aponta para um SHA aprovado da `main`, tem
versão semântica maior que qualquer tag anterior, notas curtas e um exemplo de
instalação fixado nela. O script `scripts/release.sh` confere tudo isso antes
de criar qualquer coisa; nada é publicado sem a pessoa que publica rodar o
passo `publicar` com a própria identidade.

As tags históricas `v1.x` são do produto anterior e não representam o conteúdo
atual da `main`; a primeira release deste produto é maior que todas elas.

## 1. A PR de release

Na PR de release (que passa pelo changelog obrigatório como qualquer outra):

1. Mova as linhas de `## Unreleased` para uma seção nova `## X.Y.Z - AAAA-MM-DD`
   e escreva as notas com estas três subseções, em uma ou duas páginas:

   ```markdown
   ## 2.0.0 - 2026-10-07

   Destaques em uma frase.

   ### Entregue

   - O que quem usa ganha, por capacidade.

   ### Limites

   - O que não foi medido ou ainda não existe.

   ### Migração do v1

   - O que muda para quem vinha do produto v1 e como trocar o workflow.
   ```

2. Fixe os exemplos de instalação na versão nova (edita os arquivos; você
   revisa e commita):

   ```bash
   scripts/release.sh fixar-exemplos --versao 2.0.0
   ```

   Isso troca `@main` por `@v2.0.0` em `.github/workflows/examples/code-review.yml`,
   `docs/site/workflow.yml` (o download do site, que precisa ser idêntico ao
   exemplo) e `docs/getting-started.md`.

## 2. Preparar (dry-run)

Depois do merge, com o SHA da `main`:

```bash
git fetch origin
scripts/release.sh preparar --sha <SHA completo>
```

O passo é só leitura. Ele imprime a versão e as notas derivadas do
`CHANGELOG.md` daquele SHA e recusa (exit 1):

| Recusa | Quando |
| --- | --- |
| `fora de origin/main` | O SHA não é ancestral da `main`. |
| `nao e maior que vA.B.C` | A versão não é maior que a maior tag existente. |
| `tag vX.Y.Z ja existe em ...` | A tag já existe apontando para outro SHA. |
| `sem '### Entregue'` (e as outras) | As notas não têm as três subseções, estão vazias ou passam de 120 linhas. |
| `nao fixa @vX.Y.Z` / `diferem` | Os exemplos não fixam a versão ou o download do site difere do exemplo. |

Rodar de novo depois de publicada é idempotente: a tag no mesmo SHA passa.

## 3. Verificar no consumidor

Antes do anúncio, rode o QA do consumidor ([Desenvolvimento e QA](qa.md#qa-no-repositorio-consumidor-aur-512))
com o SHA da release e guarde a evidência:

```bash
tests/consumer/run.sh --repo OWNER/CONSUMIDOR --sha <SHA completo> --evidencia qa-evidencia
```

## 4. Publicar

```bash
scripts/release.sh publicar --sha <SHA completo> --versao 2.0.0 --evidencia-consumidor qa-evidencia
```

O passo repete todas as conferências, exige que toda a evidência seja do mesmo
SHA e que o verificador do QA aprove (no container compartilhado), e então cria
a tag anotada `v2.0.0` naquele SHA e a release com as notas. Ele usa a
identidade git e o `gh` **já configurados** por quem publica: se faltar
`user.name`, `user.email` ou a autenticação do `gh`, ele para (exit 79) e não
define nada. Nunca força push nem move tag; repetir não sobrescreve a release.

Ative "Immutable releases" nas configurações do repositório para que a tag e
a release publicadas não possam ser alteradas.
