---
name: gate-fail-closed
version: 1
paths: ["cmd/**", "internal/**", "pkg/**", ".github/workflows/**"]
---
O gate falha fechado. Cada `## ` abaixo e uma regra citavel
(`gate-fail-closed#<slug do titulo>`).

## GATE-001 Erro, ausencia ou duvida nunca aprova
severity: error
Scanner ausente, erro de execucao, saida invalida, provedor sem credencial,
cobertura parcial ou limite estourado viram inconclusivo ou falha, nunca
"zero achados" nem status `success`.

Violacao:

    out, err := runScanner(ctx)
    if err != nil {
        return nil, nil // sem achados
    }

Correcao:

    if err != nil {
        return nil, fmt.Errorf("scanner indisponivel: %w", err) // inconclusivo
    }
