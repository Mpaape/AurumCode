---
name: arquitetura
version: 1
paths: ["cmd/**", "internal/**", "pkg/**"]
---
Convencao de arquitetura do AurumCode. Cada `## ` abaixo e uma regra citavel
(`arquitetura#<slug do titulo>`).

## ARQ-001 Logica fica em internal, cmd so monta
severity: warning
Decisao, regra de negocio e parsing moram em `internal/`. `cmd/aurumcode` so
le flags, monta dependencias e chama `internal/`. Um arquivo por
responsabilidade.

Violacao (em `cmd/aurumcode/review.go`):

    if f.Severity == "error" && len(f.File) > 0 && !strings.HasPrefix(f.File, "tests/") {
        blocked = true
    }

Correcao: a decisao vira `gate.Blocks(f)` em `internal/gate/`, testada la, e
`cmd/` so chama `gate.Blocks`.
