---
name: tamanho
version: 1
paths: ["**/*.go"]
---
Limites de tamanho do codigo de producao Go (testes fora). Cada `## ` abaixo
e uma regra citavel (`tamanho#<slug do titulo>`).

## TAM-001 Funcao com no maximo 150 linhas
severity: error
Uma funcao de producao tem no maximo 150 linhas, da assinatura ao `}`.

Violacao: `func runReview(...) error {` com 160 linhas que leem flags,
carregam config, chamam o modelo e publicam.

Correcao: extraia etapas nomeadas (`loadInputs`, `callModel`, `publish`),
cada uma com menos de 150 linhas.

## TAM-002 Arquivo com no maximo 400 linhas
severity: error
Um arquivo `.go` de producao tem no maximo 400 linhas.

Violacao: `internal/gate/gate.go` com 520 linhas misturando limiar, triagem e
formatacao.

Correcao: um arquivo por responsabilidade (`threshold.go`, `triage.go`,
`format.go`).
