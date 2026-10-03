# Tutoriais

Cada tutorial é executável: os blocos de configuração são os arquivos de
`demo/tutoriais/<tutorial>/`, cada caso roda como uma fase (`run.sh <caso>`),
a saída da última execução real fica em `out/` e `run.sh --check` a compara
com `expected/` (depois de conferir que `out/.imagem` foi gravado pela imagem desta
árvore). Valores que mudam com o board ou o relógio aparecem nos blocos pela forma
(`board valid: <N> atomic cards`, `<timestamp>`, `<duracao>`); a notação está em
`demo/tutoriais/README.md`. Para criar um tutorial novo, veja
`demo/tutoriais/README.md`.

| Tutorial | Casos de uso |
|---|---|
| [Revisão de código](revisao.md) | 1. primeira revisão local (`review --base`); 2. sem provedor (análise determinística); 3. com provedor (achados que citam a regra); 4. `fix` aplicando uma sugestão; 5. revisão de PR pelo workflow reutilizável; falha: arquivo não revisado nunca conta como aprovado |
| [Skills de convenção](skills.md) | 1. skill de convenção do repositório; 2. escopo por caminho (`applyTo`); 3. seleção por linguagem e apelidos (o que existe hoje); 4. skill do repositório contra skill da política; 5. seção de skill como regra citável do gate; falha: skill referenciada que não existe |
| [Política central](politica-central.md) | 1. repositório de política e `--politica`; 2. `policy_repository` no workflow; 3. precedência por seção; 4. `analysis_data`; 5. o repositório tenta afrouxar o `fail_on` e não consegue; falha: política inválida |
| [Qualquer linguagem](qualquer-linguagem.md) | 1. repositório poliglota e contexto estrutural por gramática; 2. arquivo sem gramática, declarado; 3. binário e gerado retêm a aprovação (sem e com gate); 4. achado de política em Terraform bloqueia; 5. apelidos de linguagem (o que existe hoje); falha: extensão desconhecida ainda é revisada |
| [Benchmark de recall](benchmark.md) | 1. rodar o corpus com o provedor falso; 2. ler recall, precisão e intervalo; 3. "aprovado com defeito"; 4. adicionar um caso por PR; falha: caso sem manifest é recusado |
| [Operação](operacao.md) | 1. ambiente de desenvolvimento em container; 2. aceite selado e exit codes; 3. profiles e locks; 4. dependência Go e repin; 5. scanners por digest; 6. entrega e evidência; falha: `done` sem evidência |
| [O gate de política](gate.md) | 1. `fail_on` por severidade; 2. inconclusivo por provedor ausente; 3. inconclusivo por cobertura parcial; 4. inconclusivo por SAST falhando; 5. inconclusivo por `analysis_data`; 6. `gate.sources` e a origem de cada achado; 7. status `aurumcode/policy-gate` e exit codes; 8. o repositório tenta afrouxar; falha: `gate.sources` inválido |
| [Exceções aprovadas](excecoes.md) | 1. exceção válida com dono e validade; 2. exceção vencida; 3. a exceção cobre só o achado exato; 4. exceção do repositório ignorada sob política; falha: exceção sem dono |
| [Auditoria e SARIF](auditoria-sarif.md) | 1. auditoria campo a campo; 2. SARIF; 3. revisão inconclusiva; 4. canário de redação; 5. upload para o Code Scanning pelo chamador (conferência); falha: auditoria não gravada |
| [Reaproveitamento](reaproveitamento.md) | 1. mesmo SHA, política e modelo; 2. `--base` e `--pr` (não demonstrado); 3. modelo mudou; 4. política mudou; 5. cache degradado; falha: sem `AURUMCODE_CACHE_DIR` |
| [SAST com Semgrep](sast.md) | 1. regra local offline; 2. pacote do registry sem rede; 3. `nosemgrep` e `.semgrepignore` sem política e sob política; 4. origem `sast` no gate e na auditoria; falha: Semgrep que falha vira inconclusivo |
| [SBOM e Dependency-Track](sbom-dependency-track.md) | 1. `aurumcode sbom` e a versão mínima do CycloneDX; 2. upload e métricas; 3. limiares; 4. violação de política do servidor; 5. um projeto por microsserviço; falhas: secret ausente e timeout |
| [Assinatura com Cosign](assinatura.md) | 1. chave efêmera offline; 2. verificação por terceiro; 3. keyless no Actions (não executado); 4. bundle como artefato; falhas: assinatura que falha, Cosign que sai 0 sem bundle, imagem sem digest |
| [xBOM](xbom.md) | 1. Build BOM; 2. CBOM; 3. evidência por arquivo e linha; 4. catálogo do repositório e da política; 5. enriquecimento pelo modelo; 6. tipos só documentados; falha: catálogo inválido |
| [Dados de análise](dados-de-analise.md) | 1. declarado ou não (artefato válido); 2. vencido; 3. cache; 4. workflow agendado que publica; falhas: adulterado e indisponível |
