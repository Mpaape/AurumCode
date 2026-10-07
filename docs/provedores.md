# Provedores de LLM

O Aurum fala um único protocolo, Chat Completions compatível com OpenAI, e
adapta os detalhes de cada provedor por um **catálogo de perfis** em YAML
(`internal/llm/provider/profiles/profiles.yml`, embutido no binário). Cada
perfil diz só o que muda no fio:

| Campo | O que define | Valores |
|---|---|---|
| `base_url` | URL base padrão; `{VAR}` vem da variável de ambiente `VAR`, `{VAR:-padrão}` usa o padrão quando ela está vazia | URL |
| `key_env` | variáveis que guardam a chave, na ordem; vale a primeira não vazia | lista |
| `auth` | como a chave viaja | `bearer`, `header` (com `auth_header`), `none` |
| `path` | caminho anexado à URL base | `/chat/completions` |
| `query` | parâmetros de query (aceitam `{VAR}`) | mapa |
| `token_field` | campo do limite de resposta | `max_tokens`, `max_completion_tokens` |
| `structured_output` | saída estruturada que o endpoint respeita | `json_schema`, `json_object`, `none` |

## Como escolher o provedor

| Variável | Papel |
|---|---|
| `LLM_PROVIDER` | nome do perfil. Ausente: o comportamento de sempre (`LLM_API_KEY` + `LLM_BASE_URL`, requisição idêntica à de antes). |
| `LLM_BASE_URL` | sobrescreve a URL do perfil. Obrigatória só para `openai-compatible`. |
| `LLM_API_KEY` | a chave; cada perfil também aceita a variável nativa do provedor (veja abaixo). |
| `LLM_MODEL` | o modelo, como o provedor o nomeia. |
| `LLM_PROVIDERS_FILE` | arquivo YAML do operador, no mesmo formato, que acrescenta perfis ou substitui um perfil inteiro pelo nome. |

Regras que valem para todos:

- Perfil desconhecido falha (exit 1) listando os válidos; perfil sem chave, sem
  a variável de um `{VAR}` ou sem URL falha nomeando o que falta. Nunca há
  recaída silenciosa para outro provedor nem para "sem modelo".
- O provedor, a URL e a chave vêm **só do ambiente do operador**, nunca da
  configuração do repositório revisado: um repositório não pode apontar a
  chave e o diff para um host dele.
- Toda resposta é conferida contra o contrato da revisão. Uma resposta sem a
  lista de achados (`issues`) deixa a revisão **inconclusiva** (exit 1,
  `could not understand the model's response (validation_failed)`), nunca
  aprovada. Isso importa para provedores sem saída estruturada.
- Um erro do provedor chega resumido à mensagem do próprio provedor (envelopes
  OpenAI `{"error":{...}}`, Anthropic `{"type":"error","error":{...}}`, Google
  `{"error":{"status",...}}` ou `[{"error":...}]`, Bedrock `{"message":...}`),
  com o nome do perfil e redigido (a chave e segredos conhecidos viram
  marcador): `anthropic API error (status 401): authentication_error: ...`.

No workflow reutilizável do Aurum (`.github/workflows/review.yml`), o perfil
vem do input `provider` (vira `LLM_PROVIDER` no contêiner) e a chave do
secret `LLM_API_KEY`; `LLM_BASE_URL` é obrigatório só sem perfil ou para os
perfis sem URL padrão:

```yaml
jobs:
  review:
    uses: Mpaape/AurumCode/.github/workflows/review.yml@main
    with:
      provider: anthropic
      model: claude-sonnet-5-5
    secrets: inherit
```

Pela CLI, pela imagem e pelo servidor MCP, `LLM_PROVIDER` vale do mesmo jeito.

Tudo isto está demonstrado, com um provedor falso e local, no tutorial
[Provedores de LLM](tutorials/provedores.md).

## openai-compatible (padrão)

Qualquer endpoint compatível com OpenAI: é o comportamento de antes deste
catálogo. Bearer, `max_tokens`, `response_format` `json_schema`/`json_object`.

```bash
export LLM_PROVIDER=openai-compatible   # ou deixe LLM_PROVIDER vazio
export LLM_BASE_URL=http://localhost:8080/v1
export LLM_API_KEY=...
```

## OpenAI

`https://api.openai.com/v1`, Bearer. Usa `max_completion_tokens`, o único limite
que os modelos de raciocínio aceitam no Chat Completions
([guia de raciocínio](https://developers.openai.com/api/docs/guides/reasoning)).

```bash
export LLM_PROVIDER=openai
export OPENAI_API_KEY=...        # ou LLM_API_KEY
export LLM_MODEL=gpt-5.4-mini
```

## Azure OpenAI

Rota por deployment com `api-version` na query e a chave no header `api-key`
(nunca `Authorization`), conforme a
[referência da API](https://learn.microsoft.com/en-us/azure/ai-foundry/openai/reference).
A versão padrão é a GA `2024-10-21`; troque por `AZURE_OPENAI_API_VERSION`.
Usa `max_completion_tokens`, exigido pelos modelos de raciocínio
([how-to](https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/reasoning)).

```bash
export LLM_PROVIDER=azure-openai
export AZURE_OPENAI_RESOURCE=meu-recurso
export AZURE_OPENAI_DEPLOYMENT=meu-deployment
export AZURE_OPENAI_API_KEY=...  # ou LLM_API_KEY
```

Limitação: a API `v1` nova (`/openai/v1/`, sem `api-version`) também funciona,
apontando `LLM_BASE_URL=https://meu-recurso.openai.azure.com/openai/v1` com
`LLM_PROVIDER=openai-compatible` se o recurso aceitar Bearer, ou com um perfil
próprio em `LLM_PROVIDERS_FILE` (header `api-key`, sem `query`).

## Anthropic

A [camada de compatibilidade com o OpenAI SDK](https://platform.claude.com/docs/en/cli-sdks-libraries/libraries/openai-sdk)
em `https://api.anthropic.com/v1`, Bearer. Ela **ignora `response_format`**: o
perfil não o envia (`structured_output: none`) e a resposta é conferida contra
o contrato da revisão; fora dele, a revisão fica inconclusiva.

```bash
export LLM_PROVIDER=anthropic
export ANTHROPIC_API_KEY=...     # ou LLM_API_KEY
export LLM_MODEL=...            # o identificador do modelo no provedor
```

## Google (Gemini)

O [endpoint compatível com OpenAI](https://ai.google.dev/gemini-api/docs/openai)
em `https://generativelanguage.googleapis.com/v1beta/openai`, Bearer com a chave
da Gemini API. O Gemini restringe o subconjunto de JSON Schema aceito; o perfil
pede só o modo `json_object` e a resposta é conferida localmente.

```bash
export LLM_PROVIDER=google
export GEMINI_API_KEY=...        # ou LLM_API_KEY
export LLM_MODEL=...            # o identificador do modelo no provedor
```

## Amazon Bedrock

Com **chave de API do Bedrock**: o
[endpoint Chat Completions do bedrock-runtime](https://docs.aws.amazon.com/bedrock/latest/userguide/inference-chat-completions.html)
em `https://bedrock-runtime.{AWS_REGION}.amazonaws.com/openai/v1`, com a chave
como Bearer ([chaves de API](https://docs.aws.amazon.com/bedrock/latest/userguide/api-keys-use.html)).

```bash
export LLM_PROVIDER=bedrock
export AWS_REGION=us-east-1
export AWS_BEARER_TOKEN_BEDROCK=...   # ou LLM_API_KEY
export LLM_MODEL=...            # o identificador do modelo no provedor
```

**Sem chave de API** (só credenciais IAM): o Aurum não assina SigV4. Use o
proxy LiteLLM na frente do Bedrock
([provedor Bedrock do LiteLLM](https://docs.litellm.ai/docs/providers/bedrock))
e aponte o perfil `litellm` para ele:

```yaml
# litellm-config.yaml
model_list:
  - model_name: bedrock-claude
    litellm_params:
      model: bedrock/us.anthropic.claude-sonnet-5
      aws_access_key_id: os.environ/AWS_ACCESS_KEY_ID
      aws_secret_access_key: os.environ/AWS_SECRET_ACCESS_KEY
      aws_region_name: os.environ/AWS_REGION_NAME
```

```bash
docker run --rm -p 4000:4000 \
  -v "$PWD/litellm-config.yaml:/app/config.yaml:ro" \
  -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_REGION_NAME \
  -e LITELLM_MASTER_KEY=sk-local-master \
  docker.litellm.ai/berriai/litellm:latest --config /app/config.yaml
export LLM_PROVIDER=litellm LITELLM_API_KEY=sk-local-master LLM_MODEL=bedrock-claude
```

Fixe a imagem do LiteLLM por digest em uso real.

## LiteLLM

O [proxy LiteLLM](https://docs.litellm.ai/docs/proxy/docker_quick_start) em
`http://localhost:4000`, Bearer com a chave mestra ou virtual. Serve como
porta única para qualquer provedor que o LiteLLM conheça.

```bash
export LLM_PROVIDER=litellm
export LITELLM_API_KEY=sk-...    # ou LLM_API_KEY
export LLM_BASE_URL=http://litellm.example.com:4000   # se não for localhost
```

## OpenRouter

`https://openrouter.ai/api/v1`, Bearer
([visão geral da API](https://openrouter.ai/docs/api-reference/overview)). A
saída `json_schema` depende do modelo e do provedor por trás
([structured outputs](https://openrouter.ai/docs/guides/features/structured-outputs)),
então o perfil pede `json_object` e confere a resposta localmente.

```bash
export LLM_PROVIDER=openrouter
export OPENROUTER_API_KEY=...    # ou LLM_API_KEY
export LLM_MODEL=...            # o identificador do modelo no provedor
```

## OpenCode Zen

O gateway [OpenCode Zen](https://opencode.ai/docs/zen/) em
`https://opencode.ai/zen/v1`, Bearer, para os modelos servidos em
`/chat/completions`.

```bash
export LLM_PROVIDER=opencode
export OPENCODE_API_KEY=...      # ou LLM_API_KEY
export LLM_MODEL=...            # o identificador do modelo no provedor
```

### O servidor MCP do Aurum no OpenCode

O OpenCode também é cliente MCP, ao lado do Claude Code, do Codex e do
Cursor (veja [Aurum no agente de código](agentes.md)). O servidor MCP do Aurum
(`aurumcode mcp`) entra no `opencode.json` como servidor `local`, no formato da
[documentação de servidores MCP do OpenCode](https://opencode.ai/docs/mcp-servers/);
as variáveis do provedor vêm do ambiente pela substituição `{env:...}` da
[configuração do OpenCode](https://opencode.ai/docs/config/), sem a chave no
arquivo:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "aurum": {
      "type": "local",
      "command": ["aurumcode", "mcp"],
      "enabled": true,
      "environment": {
        "LLM_PROVIDER": "{env:LLM_PROVIDER}",
        "LLM_API_KEY": "{env:LLM_API_KEY}"
      }
    }
  }
}
```

## Ollama

O [endpoint compatível com OpenAI](https://docs.ollama.com/api/openai-compatibility)
do Ollama em `http://localhost:11434/v1`. O servidor ignora a chave: o perfil
não envia credencial (`auth: none`) e não exige `LLM_API_KEY`.

```bash
export LLM_PROVIDER=ollama
export LLM_MODEL=...            # o identificador do modelo no provedor
```

## Outro provedor

Qualquer endpoint compatível com OpenAI que não esteja no catálogo entra por
`LLM_PROVIDERS_FILE`, sem mudar código:

```yaml
profiles:
  meu-gateway:
    description: gateway que autentica por header próprio
    base_url: http://localhost:9000/v1
    key_env: [GATEWAY_KEY]
    auth: header
    auth_header: x-gw-key
    path: /chat/completions
    token_field: max_completion_tokens
    structured_output: none
```

```bash
export LLM_PROVIDERS_FILE=$PWD/meus-perfis.yml LLM_PROVIDER=meu-gateway GATEWAY_KEY=...
```

Um valor inválido (`auth`, `token_field`, `structured_output` fora da lista,
`path` sem `/`) falha ao carregar o arquivo, nunca na hora de chamar.

## Verificação ao vivo

O workflow manual `.github/workflows/providers-smoke.yml` (`workflow_dispatch`)
roda uma revisão mínima por provedor **cujos secrets existam** no repositório
e registra os demais como "não verificado" (job pulado, nunca verde; sem nenhum secret a execução falha). Os testes
de contrato de cada dialeto rodam em todo PR, sem rede.

| Provedor | Secrets do smoke |
|---|---|
| openai | `OPENAI_API_KEY` |
| azure-openai | `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_RESOURCE`, `AZURE_OPENAI_DEPLOYMENT` |
| anthropic | `ANTHROPIC_API_KEY` |
| google | `GEMINI_API_KEY` |
| bedrock | `AWS_BEARER_TOKEN_BEDROCK`, `AWS_REGION` |
| openrouter | `OPENROUTER_API_KEY` |
| opencode | `OPENCODE_API_KEY` |
| litellm | `LITELLM_API_KEY`, `LITELLM_BASE_URL` |

O modelo de cada smoke vem da variável de Actions `SMOKE_MODEL_<PROVEDOR>`
(por exemplo `SMOKE_MODEL_OPENAI`). Ollama só é verificável numa máquina com o
servidor local.
