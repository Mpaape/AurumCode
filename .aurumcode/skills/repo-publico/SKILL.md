---
name: repo-publico
version: 1
paths: ["**"]
---
O repositorio e publico. Cada `## ` abaixo e uma regra citavel
(`SKILL#<slug do titulo>`).

## PUB-001 Sem empresa, dominio interno ou repositorio privado
severity: error
Linha adicionada nao cita nome de empresa cliente, dominio ou host interno,
repositorio privado ou estudo interno. Exemplos usam `example.com` ou
`localhost`.

Violacao: `baseURL: https://gitlab.<empresa>.corp/time/projeto`

Correcao: `baseURL: https://git.example.com/time/projeto`

## PUB-002 Sem segredo no repositorio
severity: error
Linha adicionada nao contem token, senha, chave privada ou URL com
credencial. Segredo vem do ambiente ou de secret do CI.

Violacao: `apiKey := "<valor literal do token>"`

Correcao: `apiKey := os.Getenv("LLM_API_KEY")`
