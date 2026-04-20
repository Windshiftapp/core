/**
 * Analytics translations for German locale locale
 */
export default {
  analytics: {
    title: 'Analysen',
    loading: 'Analysen werden geladen...',
    noData: 'Keine Daten verfügbar',
    dateRange: 'Zeitraum',
    collection: 'Sammlung',
    allItems: 'Alle Workspace-Elemente',
    datasetBasis_one: '{items} Elemente über {iterations} Iteration',
    datasetBasis_other: '{items} Elemente über {iterations} Iterationen',
    datasetBasisNoIterations: '{items} Elemente, keine Iterationen im Zeitraum',
    velocity: {
      title: 'Velocity-Trend',
      completed: 'Abgeschlossen',
      average: 'Durchschnitt (abgeschlossene Iterationen)',
    },
    cumulativeFlow: {
      title: 'Kumulativer Fluss',
    },
    cycleTime: {
      title: 'Zykluszeit',
      itemsAnalyzed: 'analysierte Elemente',
      avgTotal: 'Durchschn. Gesamt',
      median: 'Median',
      p85: '85. Perzentil',
    },
    forecast: {
      title: 'Abschlussprognose',
      remainingItems: 'Verbleibende Elemente',
      remainingPoints: 'Verbleibende Punkte',
      throughput: 'Durchsatz (pro Iteration)',
      avg: 'Durchschn.',
      min: 'Min',
      max: 'Max',
      predictions: 'Prognosen',
      confidence: 'Konfidenz',
      iterations: 'Iterationen',
      estimatedDate: 'Gesch. Datum',
      method: 'Methode',
      lowDataWarning: 'Die Prognose basiert auf begrenzten historischen Daten. Vorhersagen können unzuverlässig sein.',
    },
    storyPoints: 'Story Points',
    insufficientData: {
      no_items: 'Dieser Workspace hat noch keine Elemente. Erstellen Sie Elemente, um Analysen zu sehen.',
      no_iterations: 'Keine abgeschlossenen Iterationen im ausgewählten Zeitraum gefunden. Erstellen und schließen Sie Iterationen ab, um Trends zu sehen.',
      no_iteration_items: 'Iterationen existieren im Zeitraum, haben aber keine zugehörigen Elemente. Weisen Sie Iterationen Elemente zu, um die Velocity zu sehen.',
      no_workflow: 'Kein Workflow konfiguriert. Richten Sie Statuswerte für diesen Workspace ein, um Flussdaten zu sehen.',
      no_history: 'Elemente existieren, haben aber noch keine Statusänderungen durchlaufen. Verschieben Sie Elemente zwischen Statuswerten, um ein Flussdiagramm aufzubauen.',
      no_completed_items: 'Im ausgewählten Zeitraum wurden keine Elemente abgeschlossen. Schließen Sie Elemente ab, um Zykluszeiten zu analysieren.',
      few_completed_items: 'Nur wenige Elemente wurden abgeschlossen. Zykluszeitstatistiken werden mit mehr Daten zuverlässiger.',
      few_iterations: 'Weniger als 3 abgeschlossene Iterationen im Zeitraum. Schließen Sie mehr Iterationen ab, um Monte-Carlo-Prognosen zu ermöglichen.',
    },
  },
};
