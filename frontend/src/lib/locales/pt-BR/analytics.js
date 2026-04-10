/**
 * Analytics translations for Brazilian Portuguese locale locale
 */
export default {
  analytics: {
    title: 'Análises',
    loading: 'Carregando análises...',
    noData: 'Nenhum dado disponível',
    dateRange: 'Período',
    collection: 'Coleção',
    allItems: 'Todos os itens do workspace',
    datasetBasis: '{items} itens em {iterations, plural, one {# iteração} other {# iterações}}',
    datasetBasisNoIterations: '{items} itens, sem iterações no período',
    velocity: {
      title: 'Tendência de velocidade',
      completed: 'Concluído',
      average: 'Média (iterações concluídas)',
    },
    cumulativeFlow: {
      title: 'Fluxo acumulativo',
    },
    cycleTime: {
      title: 'Tempo de ciclo',
      itemsAnalyzed: 'itens analisados',
      avgTotal: 'Média total',
      median: 'Mediana',
      p85: 'Percentil 85',
    },
    forecast: {
      title: 'Previsão de conclusão',
      remainingItems: 'Itens restantes',
      remainingPoints: 'Pontos restantes',
      throughput: 'Throughput (por iteração)',
      avg: 'Méd',
      min: 'Mín',
      max: 'Máx',
      predictions: 'Previsões',
      confidence: 'Confiança',
      iterations: 'Iterações',
      estimatedDate: 'Data est.',
      method: 'Método',
      lowDataWarning: 'A previsão é baseada em dados históricos limitados. As previsões podem não ser confiáveis.',
    },
    storyPoints: 'Story Points',
    insufficientData: {
      no_items: 'Este workspace ainda não tem itens. Crie itens para ver as análises.',
      no_iterations: 'Nenhuma iteração concluída encontrada no período selecionado. Crie e conclua iterações para ver tendências.',
      no_iteration_items: 'Existem iterações no período, mas não têm itens correspondentes. Atribua itens às iterações para ver a velocidade.',
      no_workflow: 'Nenhum fluxo de trabalho configurado. Configure status para este workspace para ver dados de fluxo.',
      no_history: 'Existem itens, mas ainda não passaram por mudanças de status. Mova itens entre status para construir um diagrama de fluxo.',
      no_completed_items: 'Nenhum item foi concluído no período selecionado. Conclua itens para analisar tempos de ciclo.',
      few_completed_items: 'Apenas alguns itens foram concluídos. As estatísticas de tempo de ciclo se tornam mais confiáveis com mais dados.',
      few_iterations: 'Menos de 3 iterações concluídas no período. Conclua mais iterações para habilitar a previsão Monte Carlo.',
    },
  },
};
