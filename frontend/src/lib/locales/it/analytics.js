/**
 * Analytics translations for Italian locale.
 */
export default {
  analytics: {
    title: 'Analisi',
    subtitle: 'Salute e flusso delle consegne, senza bisogno di iterazioni',
    loading: 'Caricamento delle analisi...',
    noData: 'Nessun dato disponibile',
    errorTitle: 'Impossibile caricare le analisi',
    unsupportedVersion:
      'Il server ha restituito un formato di analisi non supportato. Aggiorna la pagina al termine del deployment.',
    collectionLoadError:
      'Impossibile caricare le raccolte. Le analisi mostrano tutti gli elementi del workspace.',
    retry: 'Riprova',
    dateRange: 'Intervallo di date',
    collection: 'Raccolta',
    allItems: 'Tutti gli elementi del workspace',
    from: 'Da',
    to: 'A',
    daysValue: '{value}g',
    items_one: '{count} elemento',
    items_other: '{count} elementi',
    range: {
      last30Days: 'Ultimi 30 giorni',
      last12Weeks: 'Ultime 12 settimane',
      last6Months: 'Ultimi 6 mesi',
      lastYear: 'Ultimo anno',
      custom: 'Personalizzato',
    },
    validation: {
      invalid: 'Inserisci date di inizio e di fine valide.',
      reversed: 'La data di inizio deve essere uguale o precedente alla data di fine.',
      too_long: 'Scegli un intervallo di date di 366 giorni o meno.',
    },
    scope: {
      summary: '{items} elementi attuali · {from}–{to}',
      currentWorkspace: 'Coorte attuale del workspace',
      currentWorkspaceNote:
        'L’intervallo di date si applica ai grafici di flusso e di consegna; salute e anzianità sono istantanee attuali. I grafici storici usano gli elementi presenti oggi in questo workspace. Gli elementi spostati o eliminati non sono inclusi.',
      currentCollection: 'Coorte attuale della raccolta',
      currentCollectionNote:
        'L’intervallo di date si applica ai grafici di flusso e di consegna; salute e anzianità sono istantanee attuali. I grafici storici usano gli elementi che oggi corrispondono a questa raccolta. Cambiare raccolta può cambiare la coorte.',
    },
    health: {
      title: 'Richiede attenzione',
      description: 'Lavoro attualmente non concluso con segnali che meritano un controllo.',
      unfinished: 'Non conclusi',
      overdue: 'In ritardo',
      stale: 'Inattivi',
      staleHint: 'Nessuna attività da {days}+ giorni',
      unassigned: 'Non assegnati',
      withoutPriority: 'Senza priorità',
      withoutEstimate: 'Senza stima',
      attentionItems: 'Elementi da rivedere',
      item: 'Elemento',
      status: 'Stato',
      age: 'Anzianità',
      signals: 'Segnali',
      flags: {
        overdue: 'In ritardo',
        stale: 'Inattivo',
        unassigned: 'Non assegnato',
        without_priority: 'Senza priorità',
        without_estimate: 'Senza stima',
      },
      allClear: 'Nessun elemento non concluso corrisponde attualmente a un segnale di attenzione.',
    },
    throughput: {
      title: 'Creati vs completati',
      description:
        'Arrivi e prime conclusioni settimanali. Riaprire un elemento non modifica il suo completamento originale.',
      created: 'Creati',
      completed: 'Completati',
      net: 'Variazione netta',
      average: 'Media completati / settimana',
      period: 'Periodo',
      definition: 'Il completamento è la prima transizione verso uno stato completato.',
    },
    aging: {
      title: 'Anzianità del lavoro in corso',
      description: 'Da quanto tempo sono aperti gli elementi attualmente non conclusi.',
      total: 'Elementi attivi',
      median: 'Anzianità mediana',
      p85: '85° percentile',
      ageBand: 'Fascia di anzianità',
      itemCount: 'Elementi',
      byStatus: 'Anzianità per stato',
      oldest: 'Elementi non conclusi più vecchi',
      status: 'Stato',
      noActive: 'Non c’è lavoro non concluso in questo ambito.',
      buckets: {
        '0_7': '0–7 giorni',
        '8_14': '8–14 giorni',
        '15_30': '15–30 giorni',
        '31_60': '31–60 giorni',
        '61_plus': '61+ giorni',
      },
    },
    deliveryTime: {
      title: 'Tempo di consegna',
      description: 'Dalla creazione al primo completamento, raggruppato per settimana di completamento.',
      analyzed: 'Elementi completati',
      average: 'Media',
      median: 'Mediana',
      p85: '85° percentile',
      period: 'Periodo di completamento',
      completed: 'Completati',
      slowest: 'Tempi di consegna più lunghi',
      completedDate: 'Primo completamento',
      duration: 'Tempo di consegna',
      missingHistory:
        '{count} elementi attualmente completati sono stati esclusi perché manca la cronologia del completamento.',
      missingHistory_one:
        '1 elemento attualmente completato è stato escluso perché manca la cronologia del completamento.',
      missingHistory_other:
        '{count} elementi attualmente completati sono stati esclusi perché manca la cronologia del completamento.',
      definition:
        'Misurato dalla creazione dell’elemento alla sua prima transizione verso uno stato completato. Le riaperture successive non modificano il valore.',
    },
    dataTable: {
      show: 'Visualizza tabella dati',
    },
    insufficientData: {
      no_items: 'Questo ambito non contiene ancora elementi.',
      no_active_items: 'Non c’è lavoro non concluso in questo ambito.',
      no_completed_items: 'Nessun primo completamento registrato nell’intervallo di date selezionato.',
      few_completed_items:
        'In questo intervallo sono stati completati solo pochi elementi. Considera i percentili come indicativi.',
    },
  },
};
