# Entrega de ergonomia e funcionalidade

Pedido atual do usuario: deixar o produto ergonomico e funcional, revisar
pendencias e produzir paginas bonitas. Este plano registra escopo e provas;
nao substitui cards, review independente ou acceptance.

## Jornadas que precisam estar completas

1. Um novo usuario encontra as tres features, configura review pelo workflow
   copiavel e entende quais credenciais/modelo sao necessarios.
2. Uso local tem instalacao documentada, ajuda clara e diagnostico acionavel;
   flags/ambiente nao surpreendem nem escrevem em destino inesperado.
3. Review devolve achados uteis e sugestoes visiveis, limita ruido sem alterar
   o gate, revela cobertura parcial e trata diff grande dentro do orcamento.
4. Documentacao gerada representa o projeto documentado, preserva conteudo
   autoral e tem navegacao/apresentacao cuidadas. Nao injeta um tutorial do
   AurumCode em todo projeto consumidor.
5. Site oficial bonito e responsivo, com exemplos, guias e API acessiveis.
   Assets acompanham staging/build e links nao dependem de relatorio opcional.
6. CI, build e navegador verificam o mesmo candidato; entrega externa usa
   somente identidade existente e evidencias reais, sem promessas de runtime
   baseadas apenas em testes unitarios.

## Sequencia

- AUR-485: destinos de documentacao por flags e ambiente.
- AUR-487: site oficial e inicio rapido; desktop e celular.
- AUR-477: orçamento real para diffs grandes.
- AUR-476: cobertura chega a CLI e ao PR (aguarda posse de cmd/aurumcode).
- AUR-453: fechar lacuna de sugestoes/resumo na CLI e conferir caminho PR.
- AUR-454: limite de ruido preservando decisao do gate.
- Novo card de gerador: separar conteudo de projeto e promocao do AurumCode,
  preservar pagina autoral e dar apresentacao cuidada ao scaffold padrao.
- Novo card de diagnostico/instalacao: caminho local e verificacao de config.
- Reconciliar AUR-479/480 com implementacao existente; executar prova relevante
  antes de qualquer fechamento. AUR-473/478/486: medir e resolver os defeitos
  que afetem a jornada e as linguagens anunciadas.
- Executar jornada final em consumidor limpo, testar links e inspecionar
  paginas renderizadas. Atualizar versao/publicacao conforme fluxo autorizado.

MCP, RAG, politicas ISO e seletores avancados de skills sao expansoes; nao sao
pre-requisitos da jornada basica. Continuam no backlog, sem cancelamento nem
alegacao de entrega. Noxy deve ser descrito pelo suporte efetivamente provado.

## Evidencia inicial

HEAD inicial 0ba1632. Release v1.1.0 sem binarios anexados. CI run 33772665278 e
Documentation site run 33772665550 verdes. Pipeline local exit 0: 86 done,
15 backlog, ready vazio. Essas evidencias nao provam ergonomia; screenshots
iniciais do site foram capturadas em desktop 1440px e celular 390px.
