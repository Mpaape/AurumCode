---
name: sem-bool-exportado
version: 1
paths: ["**/*.go"]
---
APIs com intencao nomeada. Cada `## ` abaixo e uma regra citavel
(`SKILL#<slug do titulo>`).

## BOOL-001 Sem parametro bool em funcao exportada
severity: error
Funcao ou metodo exportado nao recebe parametro `bool`: na chamada,
`Publish(r, true)` nao diz o que `true` liga.

Violacao:

    func Publish(r Review, inline bool) error

Correcao: um tipo nomeado ou duas funcoes.

    type Placement int
    const (Conversation Placement = iota; Inline)
    func Publish(r Review, where Placement) error
