---
name: coerencia-da-mudanca
version: 1
paths: ["cmd/**", "internal/**", "pkg/**", "scripts/**", ".github/workflows/**", "action.yml"]
---
Uma mudança fica coerente com tudo o que depende dela. Cada `## ` abaixo é uma
regra citável (`coerencia-da-mudanca#<slug do titulo>`).

## COER-001 Quem usa o que mudou continua concordando
severity: error
Quando a mudança altera o significado de uma função, método, tipo ou campo,
todos os chamadores dentro do repositório continuam coerentes entre si. Use
`find_symbol` e `search_text` para listar os usos antes de concluir. O caso
típico é um método que passa a contar algo (`Count`) enquanto outro do mesmo
tipo (`Fails`, `Event`) não passa, e o parecer, o status e a saída do processo
discordam.

Violação: `Count()` soma o limiar também sem gate declarado, mas `Fails()` e
`Event()` só olham o gate; o título diz "Bloqueado" e o evento formal fica
COMMENT.

Correção: os métodos do mesmo tipo leem o mesmo conjunto, ou o chamador que
discorda é ajustado junto.

## COER-002 Nada é afirmado sem ter sido calculado
severity: error
Uma mensagem, título ou status não afirma um número, uma estimativa ou um
veredito que não foi calculado. Uma estimativa que falhou não imprime "$0";
um fallback que muda o resultado (comparar com outra base, pular um lado) não
acontece em silêncio.

Violação: `forkPoint` volta à árvore da base em qualquer erro, e o diff
revisado passa a ser outro, sem aviso.

Correção: erro com a causa, ou aviso explícito quando o fallback é legítimo.

## COER-003 Texto ao usuário sem plural entre parênteses
severity: warning
Mensagem, título, log e doc em português usam singular e plural de verdade,
nunca "achado(s)", "linha(s)" ou "file(s)".

Correção: "1 achado", "3 achados".

## COER-004 Recurso sem teto não cresce com o repositório
severity: warning
Cache, mapa ou buffer que guarda dados de todos os arquivos do repositório
durante a execução inteira precisa de motivo: num monorepo isso estoura a
memória do runner e derruba a revisão. Guarde só o que a próxima etapa usa.
