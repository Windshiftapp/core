/**
 * Analytics translations for Spanish locale locale
 */
export default {
  analytics: {
    title: 'Analíticas',
    loading: 'Cargando analíticas...',
    noData: 'No hay datos disponibles',
    dateRange: 'Rango de fechas',
    collection: 'Colección',
    allItems: 'Todos los elementos del espacio de trabajo',
    datasetBasis_one: '{items} elementos en {iterations} iteración',
    datasetBasis_other: '{items} elementos en {iterations} iteraciones',
    datasetBasisNoIterations: '{items} elementos, sin iteraciones en el rango',
    velocity: {
      title: 'Tendencia de velocidad',
      completed: 'Completado',
      average: 'Promedio (iteraciones completadas)',
    },
    cumulativeFlow: {
      title: 'Flujo acumulativo',
    },
    cycleTime: {
      title: 'Tiempo de ciclo',
      itemsAnalyzed: 'elementos analizados',
      avgTotal: 'Promedio total',
      median: 'Mediana',
      p85: 'Percentil 85',
    },
    forecast: {
      title: 'Pronóstico de finalización',
      remainingItems: 'Elementos restantes',
      remainingPoints: 'Puntos restantes',
      throughput: 'Rendimiento (por iteración)',
      avg: 'Prom',
      min: 'Mín',
      max: 'Máx',
      predictions: 'Predicciones',
      confidence: 'Confianza',
      iterations: 'Iteraciones',
      estimatedDate: 'Fecha est.',
      method: 'Método',
      lowDataWarning: 'El pronóstico se basa en datos históricos limitados. Las predicciones pueden no ser fiables.',
    },
    storyPoints: 'Puntos de historia',
    insufficientData: {
      no_items: 'Este espacio de trabajo aún no tiene elementos. Crea elementos para ver las analíticas.',
      no_iterations: 'No se encontraron iteraciones completadas en el rango de fechas seleccionado. Crea y completa iteraciones para ver tendencias.',
      no_iteration_items: 'Existen iteraciones en el rango pero no tienen elementos coincidentes. Asigna elementos a las iteraciones para ver la velocidad.',
      no_workflow: 'No hay flujo de trabajo configurado. Configura estados para este espacio de trabajo para ver datos de flujo.',
      no_history: 'Existen elementos pero aún no han pasado por estados. Mueve elementos entre estados para construir un diagrama de flujo.',
      no_completed_items: 'No se completaron elementos en el rango de fechas seleccionado. Completa elementos para analizar tiempos de ciclo.',
      few_completed_items: 'Solo se han completado unos pocos elementos. Las estadísticas de tiempo de ciclo se vuelven más fiables con más datos.',
      few_iterations: 'Menos de 3 iteraciones completadas en el rango. Completa más iteraciones para habilitar el pronóstico Monte Carlo.',
    },
  },
};
