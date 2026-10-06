---
name: sem-numero-de-card
version: 1
paths: ["cmd/**", "internal/**", "pkg/**"]
---
Codigo e doc de pacote explicam o conceito, nao a historia do board. Cada
`## ` abaixo e uma regra citavel (`SKILL#<slug do titulo>`).

## CARD-001 Sem numero de card em codigo ou doc de pacote
severity: warning
Linha adicionada em codigo, comentario, doc de pacote ou nome de arquivo de
producao nao cita id de card do board (`AUR-` seguido de numero). O historico
fica no commit e em `docs/specs/`.

Violacao:

    // AUR-123: bloqueia quando o scanner falha.
    func blockOnScannerError() {}

Correcao:

    // Um scanner que falhou nunca conta como varredura limpa.
    func blockOnScannerError() {}
