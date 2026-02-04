/**
 * German (de) - UI components related translations
 * Includes: pickers, editors, dialogs, components, aria, layout, widgets, footer
 */
export default {
  pickers: {
    // General
    select: 'Auswählen',
    search: 'Suchen',
    options: 'Optionen',
    clearSelection: 'Auswahl aufheben',
    noResultsFor: 'Keine Ergebnisse für "{query}"',
    createItem: '"{value}" erstellen',
    noItemsFound: 'Keine Einträge gefunden',
    noItemsAvailable: 'Keine Einträge verfügbar',

    // Asset Picker
    selectAsset: 'Asset auswählen',
    noTag: 'Kein Tag',

    // User/Assignee Picker
    selectUser: 'Benutzer auswählen',
    searchUsers: 'Benutzer suchen...',
    users: 'Benutzer',
    noUsersFound: 'Keine Benutzer gefunden',
    noUsersAvailable: 'Keine Benutzer verfügbar',
    assignTo: 'Zuweisen an',
    unassigned: 'Nicht zugewiesen',
    assignee: 'Bearbeiter',
    user: 'Benutzer',
    group: 'Gruppe',
    searchUser: 'Benutzer suchen...',
    searchGroup: 'Gruppe suchen...',

    // Group Picker
    selectGroup: 'Gruppe auswählen',

    // Category Picker
    selectCategories: 'Kategorien auswählen',
    removeCategory: 'Kategorie entfernen',
    categoriesSelected: '{count} Kategorien ausgewählt',
    searchCategories: 'Kategorien suchen...',
    noCategoriesFound: 'Keine Kategorien gefunden',

    // Collection Picker
    selectCollections: 'Sammlungen auswählen',

    // Workspace Picker
    selectWorkspaces: 'Arbeitsbereiche auswählen',
    searchWorkspaces: 'Arbeitsbereiche suchen...',
    noWorkspacesFound: 'Keine Arbeitsbereiche gefunden',

    // Configuration Set Picker
    selectConfigurationSet: 'Konfigurationssatz auswählen',
    searchConfigurationSets: 'Konfigurationssätze suchen...',
    configurationSets: 'Konfigurationssätze',
    defaultConfiguration: 'Standardkonfiguration',
    defaultConfigurationDescription: 'Verwendet die Standard-Arbeitsbereichseinstellungen',
    noConfigurationSetsFound: 'Keine Konfigurationssätze gefunden',

    // Configuration Set Entity Picker
    entityAlreadyAssigned: '{label} ist bereits zugewiesen',
    itemType: 'Eintragstyp',
    priorities: 'Prioritäten',
    itemTypes: 'Eintragstypen',
    level: 'Ebene {level}',
    assigned: 'Zugewiesen',
    noEntitiesAssigned: 'Keine {label} zugewiesen',
    available: 'Verfügbar',
    noEntitiesMatchSearch: 'Keine {label} entsprechen Ihrer Suche',
    allEntitiesAssigned: 'Alle {label} sind zugewiesen',
    inConfigSet: 'Im Konfigurationssatz',
    searchEntities: '{label} suchen...',

    // Field Selector
    selectField: 'Feld auswählen',
    searchFields: 'Felder suchen...',
    noFieldsFound: 'Keine Felder gefunden',
    customFields: 'Benutzerdefinierte Felder',
    custom: 'Benutzerdefiniert',
    customFieldDesc: 'Benutzerdefiniertes Feld',
    fieldTypes: {
      text: 'Text',
      number: 'Zahl',
      date: 'Datum',
      select: 'Auswahl',
      multiselect: 'Mehrfachauswahl',
      checkbox: 'Kontrollkästchen',
      url: 'URL',
      email: 'E-Mail',
      phone: 'Telefon',
      textarea: 'Textbereich',
      user: 'Benutzer',
      rating: 'Bewertung',
    },
    fieldCategories: {
      basic: 'Grundfelder',
      dates: 'Datumsfelder',
      people: 'Personen',
      workflow: 'Workflow',
      custom: 'Benutzerdefinierte Felder',
    },
    fields: {
      title: { name: 'Titel', description: 'Eintragstitel' },
      description: { name: 'Beschreibung', description: 'Eintragsbeschreibung' },
      status: { name: 'Status', description: 'Aktueller Status' },
      priority: { name: 'Priorität', description: 'Prioritätsstufe' },
      type: { name: 'Typ', description: 'Eintragstyp' },
      assignee: { name: 'Bearbeiter', description: 'Zugewiesener Benutzer' },
      reporter: { name: 'Ersteller', description: 'Wer den Eintrag erstellt hat' },
      createdAt: { name: 'Erstellt am', description: 'Wann der Eintrag erstellt wurde' },
      updatedAt: {
        name: 'Aktualisiert am',
        description: 'Wann der Eintrag zuletzt aktualisiert wurde',
      },
      dueDate: { name: 'Fälligkeitsdatum', description: 'Wann der Eintrag fällig ist' },
      startDate: { name: 'Startdatum', description: 'Wann die Arbeit beginnt' },
      estimate: { name: 'Schätzung', description: 'Geschätzter Aufwand' },
      labels: { name: 'Labels', description: 'Eintragslabels' },
      sprint: { name: 'Sprint', description: 'Zugehöriger Sprint' },
      milestone: { name: 'Meilenstein', description: 'Zielmeilenstein' },
      parent: { name: 'Übergeordnet', description: 'Übergeordneter Eintrag' },
      children: { name: 'Untergeordnet', description: 'Untergeordnete Einträge' },
      links: { name: 'Verknüpfungen', description: 'Verwandte Einträge' },
      attachments: { name: 'Anhänge', description: 'Dateianhänge' },
      comments: { name: 'Kommentare', description: 'Diskussionskommentare' },
      watchers: { name: 'Beobachter', description: 'Benutzer, die diesen Eintrag beobachten' },
    },

    // Icon Selector
    iconAndColor: 'Symbol & Farbe',
    searchIcons: 'Symbole suchen...',
    icons: 'Symbole',
    colors: 'Farben',
    icon: 'Symbol',
    color: 'Farbe',

    // Label Combobox
    allLabels: 'Alle Labels',
    selectLabels: 'Labels auswählen',
    noLabelsFoundFor: 'Keine Labels gefunden für "{query}"',

    // Mention Picker
    mentionUsers: 'Benutzer erwähnen',
    searching: 'Suche...',
    noNotificationPersonalTask: 'Persönliche Aufgaben senden keine Benachrichtigungen',

    // Milestone Combobox
    selectMilestone: 'Meilenstein auswählen',
    noMilestone: 'Kein Meilenstein',
    milestones: 'Meilensteine',
    noMilestonesFound: 'Keine Meilensteine gefunden',

    // Priority Picker
    selectPriority: 'Priorität auswählen',
    noPriority: 'Keine Priorität',
    loadingPriorities: 'Prioritäten werden geladen...',
    noPrioritiesConfigured: 'Keine Prioritäten konfiguriert',

    // Project Picker
    selectProject: 'Projekt auswählen',

    // Repository Selector
    linkRepositories: 'Repositories verknüpfen',
    selectRepositoriesFrom: 'Repositories von {provider} auswählen',
    searchRepositories: 'Repositories suchen...',
    loadingRepositories: 'Repositories werden geladen...',
    noRepositoriesMatchSearch: 'Keine Repositories entsprechen Ihrer Suche',
    noRepositoriesAvailable: 'Keine Repositories verfügbar',
    alreadyLinked: 'Bereits verknüpft',
    linkSelected: 'Ausgewählte verknüpfen',
    linking: 'Verknüpfe...',
    repositoriesSelected: '{count} ausgewählt',

    // Role Picker
    selectRole: 'Rolle auswählen',

    // Screen Picker
    selectScreen: 'Bildschirm auswählen',

    // Test Case Picker
    searchTestCases: 'Testfälle suchen...',

    // Workflow Picker
    selectWorkflow: 'Workflow auswählen',
  },

  editors: {
    enterText: 'Text eingeben...',
    selectDate: 'Datum auswählen...',
    clickToChangeColor: 'Klicken zum Ändern der Farbe',
    saveEnter: 'Speichern (Enter)',
    cancelEscape: 'Abbrechen (Escape)',
    availableFields: 'Verfügbare Felder',
    selectedFields: 'Ausgewählte Felder',
    dragFieldsToAdd: 'Felder hierher ziehen zum Hinzufügen',
    dragToReorderOrDrop: 'Zum Sortieren ziehen oder Felder hier ablegen',
    dropFieldsHere: 'Felder hier ablegen zum Konfigurieren',
    noFieldsMatchSearch: 'Keine Felder entsprechen Ihrer Suche',
    noFieldsAvailable: 'Keine Felder verfügbar',
    allFieldsAdded: 'Alle verfügbaren Felder wurden hinzugefügt',
    bold: 'Fett (Strg+B)',
    italic: 'Kursiv (Strg+I)',
    strikethrough: 'Durchgestrichen',
    inlineCode: 'Inline-Code',
    bulletList: 'Aufzählungsliste',
    numberedList: 'Nummerierte Liste',
    insertImage: 'Bild einfügen',
    userNotFound: 'Benutzer nicht gefunden',
  },

  dialogs: {
    cancel: 'Abbrechen',
    confirm: 'Bestätigen',
    save: 'Speichern',
    close: 'Schließen',
    delete: 'Löschen',
    update: 'Aktualisieren',
    // Confirmation messages for confirm() dialogs
    confirmations: {
      deleteItem:
        'Möchten Sie "{name}" wirklich löschen? Dies kann nicht rückgängig gemacht werden.',
      deleteSection: 'Möchten Sie diesen Abschnitt wirklich löschen?',
      discardChanges: 'Sie haben ungespeicherte Änderungen. Möchten Sie wirklich abbrechen?',
      dismissAllNotifications:
        'Möchten Sie wirklich alle Benachrichtigungen verwerfen? Dies kann nicht rückgängig gemacht werden.',
      removeAvatar: 'Möchten Sie Ihr Profilbild wirklich entfernen?',
      revokeCalendarFeed:
        'Möchten Sie Ihre Kalender-Feed-URL wirklich widerrufen? Kalender, die diese URL verwenden, werden nicht mehr synchronisiert.',
      deleteTheme:
        'Möchten Sie dieses Design wirklich löschen? Dies kann nicht rückgängig gemacht werden.',
      resetBoardConfig:
        'Möchten Sie wirklich auf die Standard-Board-Konfiguration zurücksetzen? Ihre benutzerdefinierte Konfiguration wird gelöscht.',
      deleteCustomField:
        'Möchten Sie das benutzerdefinierte Feld "{name}" wirklich löschen? Es wird aus allen Projekten entfernt.',
      deleteLinkType:
        'Möchten Sie diesen Verknüpfungstyp wirklich löschen? Alle Verknüpfungen dieses Typs werden ebenfalls entfernt.',
      deleteAsset: 'Möchten Sie dieses Asset wirklich löschen?',
      deleteAssetSet:
        'Möchten Sie dieses Asset-Set wirklich löschen? Alle Assets, Typen und Kategorien darin werden gelöscht.',
      deleteAssetType:
        'Möchten Sie diesen Asset-Typ wirklich löschen? Assets mit diesem Typ haben dann keinen Typ mehr zugewiesen.',
      deleteCategory:
        'Möchten Sie diese Kategorie wirklich löschen? Unterkategorien werden zur übergeordneten Kategorie verschoben.',
      revokeRole: 'Möchten Sie diese Rolle wirklich widerrufen?',
      quitApplication:
        'Möchten Sie die Anwendung wirklich beenden? Der Server wird heruntergefahren.',
      deleteConnection:
        'Möchten Sie diese Verbindung wirklich löschen? Dies kann nicht rückgängig gemacht werden.',
      deleteWidget: 'Diesen Abschnitt löschen? Alle Widgets in diesem Abschnitt werden entfernt.',
    },
    // Alert messages for alert() dialogs
    alerts: {
      nameRequired: 'Name ist erforderlich',
      pleaseSelectImage: 'Bitte wählen Sie eine Bilddatei aus',
      timerAlreadyRunning:
        'Ein Timer läuft bereits. Bitte stoppen Sie ihn, bevor Sie einen neuen starten.',
      noTimerRunning: 'Kein Timer läuft derzeit.',
      timerSyncing: 'Timer wird synchronisiert. Bitte warten Sie und versuchen Sie es erneut.',
      startTimerFromItem:
        'Bitte starten Sie einen Timer innerhalb eines Arbeitselements, um Kontext zu liefern.',
      cannotDeleteDefaultScreen:
        'Der Standardbildschirm kann nicht gelöscht werden. Dieser Bildschirm ist für Arbeitsbereiche ohne Konfigurationssatz erforderlich.',
      applicationShuttingDown: 'Anwendung wird heruntergefahren...',
      pdfExportComingSoon: 'PDF-Export für Zeitblock-Ansicht kommt bald',
      configUpdatedSuccess:
        'Konfigurationssatz erfolgreich aktualisiert. Alle Arbeitselemente verwenden bereits Status aus dem neuen Workflow.',
      failedToSave: 'Speichern fehlgeschlagen: {error}',
      failedToDelete: 'Löschen fehlgeschlagen: {error}',
      failedToUpdate: 'Aktualisieren fehlgeschlagen: {error}',
      failedToLoad: 'Laden fehlgeschlagen: {error}',
      failedToCreate: 'Erstellen fehlgeschlagen: {error}',
      failedToUpload: 'Hochladen fehlgeschlagen: {error}',
      failedToGeneratePdf: 'PDF konnte nicht generiert werden. Bitte versuchen Sie es erneut.',
      failedToApplyConfig: 'Konfigurationsänderung konnte nicht angewendet werden: {error}',
      failedToAddManager: 'Manager konnte nicht hinzugefügt werden: {error}',
      failedToRemoveManager: 'Manager konnte nicht entfernt werden: {error}',
      failedToSaveWorkspace:
        'Projekt konnte nicht gespeichert werden. Bitte überprüfen Sie Ihre Eingabe und versuchen Sie es erneut.',
      failedToResetConfig: 'Konfiguration konnte nicht zurückgesetzt werden: {error}',
      failedToToggleStatus: 'Verknüpfungstyp-Status konnte nicht umgeschaltet werden: {error}',
      failedToAssignRole: 'Rolle konnte nicht zugewiesen werden: {error}',
      failedToRevokeRole: 'Rolle konnte nicht widerrufen werden: {error}',
      failedToUpdateRole: 'Jeder-Rolle konnte nicht aktualisiert werden: {error}',
      failedToLoadFields: 'Felder konnten nicht geladen werden: {error}',
      failedToSaveFields: 'Feldzuweisungen konnten nicht gespeichert werden: {error}',
      errorAddingTestCase: 'Fehler beim Hinzufügen des Testfalls: {error}',
      failedToCreateLabel: 'Label konnte nicht erstellt werden: {error}',
      failedToSaveLayout: 'Layout-Änderungen konnten nicht gespeichert werden',
      statusInUseByTransitions:
        '"{name}" kann nicht gelöscht werden, da es in {count} Workflow-Übergang(en) verwendet wird. Um diesen Status zu löschen, gehen Sie zur Workflow-Verwaltung, entfernen Sie alle Übergänge, die diesen Status verwenden, und versuchen Sie dann erneut, den Status zu löschen.',
    },
  },

  components: {
    // Avatar component
    avatar: {
      defaultAlt: 'Avatar',
    },

    // DataTable component
    dataTable: {
      showingRange: 'Zeige {start}–{end} von {total}',
    },

    // Diagram components
    diagram: {
      loading: 'Diagramme werden geladen...',
      loadError: 'Diagramme konnten nicht geladen werden',
      deleteError: 'Diagramm konnte nicht gelöscht werden',
      confirmDelete: 'Sind Sie sicher, dass Sie dieses Diagramm löschen möchten?',
      edit: 'Diagramm bearbeiten',
      untitled: 'Unbenanntes Diagramm',
      namePlaceholder: 'Diagrammname',
      nameRequired: 'Bitte geben Sie einen Diagrammnamen ein',
      saveError: 'Diagramm konnte nicht gespeichert werden',
      unsavedChanges: 'Ungespeicherte Änderungen',
      unsavedChangesConfirm: 'Sie haben ungespeicherte Änderungen. Möchten Sie wirklich schließen?',
    },

    // ErrorState component
    errorState: {
      title: 'Etwas ist schiefgelaufen',
    },

    // Pagination component
    pagination: {
      showingRange: 'Zeige {start}-{end} von {total}',
      limitedTo: 'begrenzt auf {max} Einträge',
      itemsPerPage: 'Einträge pro Seite:',
      previousPage: 'Vorherige Seite',
      nextPage: 'Nächste Seite',
      goToPage: 'Gehe zu Seite {page}',
      pageOf: 'Seite {current} von {total}',
    },

    // UserAvatar component
    userAvatar: {
      myWorkspace: 'Mein Arbeitsbereich',
      myWorkspaceSubtitle: 'Persönlicher Arbeitsbereich für Aufgaben und Notizen',
      profileSubtitle: 'Profil und Einstellungen verwalten',
      security: 'Sicherheit',
      securitySubtitle: 'Passwörter, 2FA und API-Tokens verwalten',
      themeTitle: 'Design: {mode}',
      themeCycle: 'Klicken zum Wechseln: Hell → Dunkel → System',
      themeLight: 'Hell',
      themeDark: 'Dunkel',
      themeSystem: 'System',
    },
  },

  aria: {
    close: 'Schließen',
    dragToReorder: 'Ziehen zum Neuordnen',
    refresh: 'Aktualisieren',
    removeField: 'Feld entfernen',
    removeFromSection: 'Aus Abschnitt entfernen',
    addNewStep: 'Neuen Schritt hinzufügen',
    removeCurrentStep: 'Aktuellen Schritt entfernen',
    dismissNotification: 'Benachrichtigung schließen',
    mainNavigation: 'Hauptnavigation',
    mentionUsers: 'Benutzer erwähnen',
    notifications: 'Benachrichtigungen',
    adminSettings: 'Admin-Einstellungen',
    userMenu: 'Benutzermenü',
    clearSearch: 'Suche löschen',
  },

  layout: {
    addSection: 'Abschnitt hinzufügen',
    moveUp: 'Abschnitt nach oben',
    moveDown: 'Abschnitt nach unten',
    deleteSection: 'Abschnitt löschen',
    editMode: 'Bearbeitungsmodus',
    editDisplaySettings: 'Anzeigeeinstellungen bearbeiten',
    items: 'Einträge',
  },

  widgets: {
    removeWidget: 'Widget entfernen',
    narrowWidth: 'Schmal (1/3 Breite)',
    mediumWidth: 'Mittel (2/3 Breite)',
    fullWidth: 'Volle Breite',
    chart: {
      items: 'Einträge',
    },
    completionChart: {
      emptyMessage: 'Keine Abschlussdaten verfügbar',
    },
    createdChart: {
      emptyMessage: 'Keine Erstellungsdaten verfügbar',
    },
    milestoneProgress: {
      emptyTitle: 'Keine Meilensteine',
      emptySubtitle: 'Erstellen Sie Meilensteine, um den Fortschritt zu verfolgen',
    },
  },

  footer: {
    platformName: 'Windshift Arbeitsmanagement-Plattform',
    aboutWindshift: 'Über Windshift',
    reportProblem: 'Problem melden',
  },
};
