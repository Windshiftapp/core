/**
 * Analytics translations for German locale.
 */
export default {
  analytics: {
    title: 'Analysen',
    subtitle: 'Lieferzustand und Arbeitsfluss – ganz ohne Iterationen',
    loading: 'Analysen werden geladen...',
    noData: 'Keine Daten verfügbar',
    errorTitle: 'Analysen konnten nicht geladen werden',
    unsupportedVersion:
      'Der Server hat ein nicht unterstütztes Analyseformat zurückgegeben. Aktualisieren Sie die Seite nach Abschluss der Bereitstellung.',
    collectionLoadError:
      'Sammlungen konnten nicht geladen werden. Die Analyse zeigt alle Workspace-Vorgänge.',
    retry: 'Erneut versuchen',
    dateRange: 'Zeitraum',
    collection: 'Sammlung',
    allItems: 'Alle Workspace-Vorgänge',
    from: 'Von',
    to: 'Bis',
    daysValue: '{value} T.',
    items_one: '{count} Vorgang',
    items_other: '{count} Vorgänge',
    range: {
      last30Days: 'Letzte 30 Tage',
      last12Weeks: 'Letzte 12 Wochen',
      last6Months: 'Letzte 6 Monate',
      lastYear: 'Letztes Jahr',
      custom: 'Benutzerdefiniert',
    },
    validation: {
      invalid: 'Geben Sie ein gültiges Start- und Enddatum ein.',
      reversed: 'Das Startdatum muss vor oder am Enddatum liegen.',
      too_long: 'Wählen Sie einen Zeitraum von höchstens 366 Tagen.',
    },
    scope: {
      summary: '{items} aktuelle Vorgänge · {from}–{to}',
      currentWorkspace: 'Aktuelle Workspace-Kohorte',
      currentWorkspaceNote:
        'Der Zeitraum gilt für Fluss- und Lieferdiagramme; Zustand und Alter sind aktuelle Momentaufnahmen. Historische Diagramme verwenden Vorgänge, die heute in diesem Workspace liegen. Verschobene oder gelöschte Vorgänge sind nicht enthalten.',
      currentCollection: 'Aktuelle Sammlungskohorte',
      currentCollectionNote:
        'Der Zeitraum gilt für Fluss- und Lieferdiagramme; Zustand und Alter sind aktuelle Momentaufnahmen. Historische Diagramme verwenden Vorgänge, die heute zur Sammlung passen. Änderungen an der Sammlung können die Kohorte verändern.',
    },
    health: {
      title: 'Handlungsbedarf',
      description: 'Aktuelle unerledigte Arbeit mit Signalen, die geprüft werden sollten.',
      unfinished: 'Unerledigt',
      overdue: 'Überfällig',
      stale: 'Inaktiv',
      staleHint: 'Keine Aktivität seit {days}+ Tagen',
      unassigned: 'Nicht zugewiesen',
      withoutPriority: 'Keine Priorität',
      withoutEstimate: 'Keine Schätzung',
      attentionItems: 'Zu prüfende Vorgänge',
      item: 'Vorgang',
      status: 'Status',
      age: 'Alter',
      signals: 'Signale',
      flags: {
        overdue: 'Überfällig',
        stale: 'Inaktiv',
        unassigned: 'Nicht zugewiesen',
        without_priority: 'Keine Priorität',
        without_estimate: 'Keine Schätzung',
      },
      allClear: 'Keine unerledigten Vorgänge entsprechen derzeit einem Warnsignal.',
    },
    throughput: {
      title: 'Erstellt vs. abgeschlossen',
      description:
        'Wöchentliche Zugänge und erste Abschlüsse. Eine Wiedereröffnung ändert den ursprünglichen Abschluss nicht.',
      created: 'Erstellt',
      completed: 'Abgeschlossen',
      net: 'Nettoänderung',
      average: 'Ø abgeschlossen / Woche',
      period: 'Zeitraum',
      definition: 'Als Abschluss gilt der erste Wechsel in einen abgeschlossenen Status.',
    },
    aging: {
      title: 'Alter der laufenden Arbeit',
      description: 'Wie lange aktuell unerledigte Vorgänge bereits offen sind.',
      total: 'Aktive Vorgänge',
      median: 'Medianalter',
      p85: '85. Perzentil',
      ageBand: 'Altersgruppe',
      itemCount: 'Vorgänge',
      byStatus: 'Alter nach Status',
      oldest: 'Älteste unerledigte Vorgänge',
      status: 'Status',
      noActive: 'In diesem Bereich gibt es keine unerledigte Arbeit.',
      buckets: {
        '0_7': '0–7 Tage',
        '8_14': '8–14 Tage',
        '15_30': '15–30 Tage',
        '31_60': '31–60 Tage',
        '61_plus': '61+ Tage',
      },
    },
    deliveryTime: {
      title: 'Lieferzeit',
      description: 'Von der Erstellung bis zum ersten Abschluss, gruppiert nach Abschlusswoche.',
      analyzed: 'Abgeschlossene Vorgänge',
      average: 'Durchschnitt',
      median: 'Median',
      p85: '85. Perzentil',
      period: 'Abschlusszeitraum',
      completed: 'Abgeschlossen',
      slowest: 'Längste Lieferzeiten',
      completedDate: 'Erstmals abgeschlossen',
      duration: 'Lieferzeit',
      missingHistory_one:
        '1 aktuell abgeschlossener Vorgang wurde wegen fehlender Abschlusshistorie ausgeschlossen.',
      missingHistory_other:
        '{count} aktuell abgeschlossene Vorgänge wurden wegen fehlender Abschlusshistorie ausgeschlossen.',
      definition:
        'Gemessen von der Erstellung bis zum ersten Wechsel in einen abgeschlossenen Status. Spätere Wiedereröffnungen ändern den Wert nicht.',
    },
    dataTable: {
      show: 'Datentabelle anzeigen',
    },
    insufficientData: {
      no_items: 'Dieser Bereich enthält noch keine Vorgänge.',
      no_active_items: 'In diesem Bereich gibt es keine unerledigte Arbeit.',
      no_completed_items: 'Im ausgewählten Zeitraum wurden keine ersten Abschlüsse erfasst.',
      few_completed_items:
        'In diesem Zeitraum wurden nur wenige Vorgänge abgeschlossen. Perzentile sind daher nur als Tendenz zu verstehen.',
    },
  },
};
