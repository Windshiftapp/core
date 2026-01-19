/**
 * Actions automation translations (German)
 */
export default {
  actions: {
    title: 'Aktionen',
    description: 'Workflows mit regelbasierten Aktionen automatisieren',
    create: 'Aktion erstellen',
    createFirst: 'Erste Aktion erstellen',
    noActions: 'Noch keine Aktionen',
    noActionsDescription: 'Erstellen Sie Aktionen, um Ihre Workflows basierend auf Element-Ereignissen zu automatisieren',
    enabled: 'Aktiviert',
    disabled: 'Deaktiviert',
    enable: 'Aktivieren',
    disable: 'Deaktivieren',
    viewLogs: 'Protokolle anzeigen',
    confirmDelete: 'Sind Sie sicher, dass Sie die Aktion "{name}" löschen möchten?',
    failedToSave: 'Aktion konnte nicht gespeichert werden',
    newAction: 'Neue Aktion',

    trigger: {
      statusTransition: 'Statusänderung',
      itemCreated: 'Element erstellt',
      itemUpdated: 'Element aktualisiert',
      itemLinked: 'Element verknüpft'
    },

    nodes: {
      trigger: 'Auslöser',
      setField: 'Feld setzen',
      setStatus: 'Status setzen',
      addComment: 'Kommentar hinzufügen',
      notifyUser: 'Benutzer benachrichtigen',
      condition: 'Bedingung'
    },

    addNodes: 'Knoten hinzufügen',
    tips: 'Tipps',
    tipDragToConnect: 'Ziehen Sie von den Griffen, um Knoten zu verbinden',
    tipClickToEdit: 'Klicken Sie auf einen Knoten, um ihn zu konfigurieren',
    tipConditionBranches: 'Bedingungen haben Ja/Nein-Verzweigungen',

    nodeConfig: 'Knotenkonfiguration',
    config: {
      from: 'Von',
      to: 'Nach',
      selectField: 'Feld auswählen...',
      selectStatus: 'Status auswählen...',
      enterComment: 'Kommentar eingeben...',
      selectRecipient: 'Empfänger auswählen...',
      setCondition: 'Bedingung festlegen...',
      targetStatus: 'Zielstatus',
      fieldName: 'Feldname',
      value: 'Wert',
      commentContent: 'Kommentarinhalt',
      commentPlaceholder: 'Kommentartext eingeben. Verwenden Sie {{item.title}} für Variablen.',
      privateComment: 'Privater Kommentar (nur intern)',
      fieldToCheck: 'Zu prüfendes Feld',
      operator: 'Operator',
      compareValue: 'Vergleichswert',
      private: 'Privat',
      triggerType: 'Auslösertyp',
      fromStatus: 'Von Status',
      toStatus: 'Nach Status',
      anyStatus: 'Beliebiger Status'
    },

    recipients: {
      assignee: 'Zugewiesener',
      creator: 'Ersteller',
      specific: 'Bestimmte Benutzer'
    },

    condition: {
      true: 'Ja',
      false: 'Nein'
    },

    operators: {
      equals: 'Gleich',
      notEquals: 'Ungleich',
      contains: 'Enthält',
      greaterThan: 'Größer als',
      lessThan: 'Kleiner als',
      isEmpty: 'Ist leer',
      isNotEmpty: 'Ist nicht leer'
    },

    logs: {
      title: 'Ausführungsprotokolle',
      noLogs: 'Keine Ausführungsprotokolle',
      status: 'Status',
      running: 'Läuft',
      completed: 'Abgeschlossen',
      failed: 'Fehlgeschlagen',
      skipped: 'Übersprungen',
      startedAt: 'Gestartet um',
      completedAt: 'Abgeschlossen um',
      error: 'Fehler',
      viewDetails: 'Details anzeigen'
    }
  }
};
