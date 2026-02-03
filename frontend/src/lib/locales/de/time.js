/**
 * German (de) - Time tracking related translations
 * Includes: time, timeProject, timeProjectCategory
 */
export default {
  time: {
    title: 'Zeiterfassung',
    subtitle: 'Zeit für Vorgänge erfassen',
    logTime: 'Zeit erfassen',
    editTimeEntry: 'Zeiteintrag bearbeiten',
    updateEntry: 'Eintrag aktualisieren',
    timeSpent: 'Aufgewendete Zeit',
    remaining: 'Verbleibend',
    estimate: 'Schätzung',
    originalEstimate: 'Ursprüngliche Schätzung',
    hours: 'Stunden',
    minutes: 'Minuten',
    days: 'Tage',
    weeks: 'Wochen',
    startTimer: 'Timer starten',
    stopTimer: 'Timer stoppen',
    pauseTimer: 'Timer pausieren',
    resumeTimer: 'Timer fortsetzen',
    timeLogged: 'Zeit erfolgreich erfasst',
    timeTrackingProject: 'Zeiterfassungsprojekt',
    workItemOptional: 'Vorgang (Optional)',
    whatDidYouWorkOn: 'Woran haben Sie gearbeitet?',
    start: 'Start',
    end: 'Ende',
    duration: 'Dauer',
    durationHelperText:
      'Geben Sie Startzeit + Dauer (2h) ein, um die Endzeit automatisch zu berechnen, oder geben Sie Start + Ende ein, um die Dauer automatisch zu berechnen. Zeitformate: 1h, 30m, 1h30m, 2h15m, 1d (=8h)',

    // Onboarding
    onboarding: {
      title: 'Zeiterfassung einrichten',
      subtitle: 'Lassen Sie uns Ihre erste Kundenorganisation und Ihr erstes Projekt erstellen',
      setupProgress: 'Einrichtungsfortschritt',
      stepOf: 'Schritt {current} von {total}',
      createCustomerTitle: 'Erstellen Sie Ihre erste Kundenorganisation',
      createCustomerDescription:
        'Eine Kundenorganisation repräsentiert das Unternehmen oder die Einheit, für die Sie arbeiten. Dies könnte eine Kundenorganisation, Ihr Arbeitgeber oder Ihr eigenes Unternehmen sein.',
      createProjectTitle: 'Erstellen Sie Ihr erstes Projekt',
      createProjectDescription:
        'Projekte helfen dabei, Ihre Arbeit innerhalb eines Kunden zu organisieren. Sie können Zeit gegen bestimmte Projekte erfassen.',
      organizationNameRequired: 'Organisationsname ist erforderlich',
      projectNameRequired: 'Projektname ist erforderlich',
      failedToCreateCustomer:
        'Kundenorganisation konnte nicht erstellt werden. Bitte versuchen Sie es erneut.',
      failedToCreateProject: 'Projekt konnte nicht erstellt werden. Bitte versuchen Sie es erneut.',
      customerCreatedSuccess: 'Kundenorganisation "{name}" erfolgreich erstellt!',
      organizationNamePlaceholder: 'z.B. Acme GmbH, TechStart AG, Freiberufliche Arbeit',
      emailPlaceholder: 'buchhaltung@kunde.de',
      contactPersonPlaceholder: 'Max Mustermann',
      projectNamePlaceholder: 'z.B. Webseitenentwicklung, Beratung, Allgemeine Arbeit',
      projectDescriptionPlaceholder: 'Kurze Beschreibung des Projekts...',
      hourlyRateHint: 'Sie können dies später festlegen, wenn Sie unsicher sind',
      skipForNow: 'Vorerst überspringen',
      completeSetup: 'Einrichtung abschließen',
    },

    // Categories
    categories: {
      title: 'Projektkategorien',
      subtitle: 'Projekte in Kategorien organisieren für bessere Verwaltung',
      newCategory: 'Neue Kategorie',
      noCategories: 'Noch keine Kategorien',
      createFirstHint: 'Erstellen Sie Ihre erste Kategorie, um Projekte zu organisieren',
      failedToSave: 'Kategorie konnte nicht gespeichert werden',
      failedToDelete: 'Kategorie konnte nicht gelöscht werden',
      confirmDelete: 'Sind Sie sicher, dass Sie "{name}" löschen möchten?',
    },

    // Reports
    reports: {
      title: 'Berichte',
      subtitle: 'Analysieren Sie Ihre Zeiterfassungsdaten und exportieren Sie Berichte',
      exportCSV: 'Als CSV exportieren',
      exportPDF: 'Als PDF exportieren',
      filters: 'Filter',
      customer: 'Kunde',
      project: 'Projekt',
      allCustomers: 'Alle Kunden',
      allProjects: 'Alle Projekte',
      descriptionFilter: 'Beschreibungsfilter',
      searchDescriptions: 'Beschreibungen durchsuchen...',
      fromDate: 'Von Datum',
      toDate: 'Bis Datum',
      applyFilters: 'Filter anwenden',
      totalHours: 'Gesamtstunden',
      totalEntries: 'Gesamteinträge',
      averagePerDay: 'Durchschnitt pro Tag',
      topProject: 'Top-Projekt',
      loadingReports: 'Berichte werden geladen...',
      noEntriesFound: 'Keine Zeiteinträge für die ausgewählten Filter gefunden.',
      totalTime: 'Gesamtzeit',
      entriesShown: '{count} Einträge angezeigt',
    },

    // Timer
    timer: {
      goToWorkItem: 'Zum Vorgang gehen: {title}',
      expandTimer: 'Timer erweitern',
      collapseTimer: 'Timer minimieren',
      project: 'Projekt',
      workspace: 'Arbeitsbereich',
    },

    // Projects
    projects: {
      title: 'Projekte',
      subtitle: 'Globale Projekte für die Zeiterfassung über Arbeitsbereiche hinweg verwalten',
      addProject: 'Projekt hinzufügen',
      projectsTab: 'Projekte',
      categoriesTab: 'Kategorien',
      searchProjects: 'Projekte suchen...',
      allCategories: 'Alle Kategorien',
      allStatuses: 'Alle Status',
      statusCount: '{count} Status',
      noProjects:
        'Keine Projekte gefunden. Erstellen Sie Ihr erstes Projekt, um mit der Zeiterfassung zu beginnen.',
      noProjectsInCategory: 'Keine Projekte in dieser Kategorie.',
      failedToSave: 'Projekt konnte nicht gespeichert werden',
      deleteProject: 'Projekt löschen',
      confirmDelete: 'Sind Sie sicher, dass Sie "{name}" löschen möchten?',
      unknownCustomer: 'Unbekannter Kunde',
      project: 'Projekt',
      customer: 'Kunde',
      rate: 'Satz',
      projectName: 'Projektname',
      descriptionOptional: 'Beschreibung (Optional)',
      hourlyRateOptional: 'Stundensatz (Optional)',
    },

    // Calendar
    calendar: {
      title: 'Wochenkalender',
      itemCount: '{count} Einträge',
      exportWeekToICS: 'Woche als ICS exportieren',
      myWorkItems: 'Meine Vorgänge',
      dragToSchedule: 'Einträge ziehen zum Planen',
      noWorkItems: 'Keine Vorgänge zugewiesen',
      workItemsWillAppear: 'Vorgänge erscheinen hier, wenn sie Ihnen zugewiesen werden',
      itemsCompleted: '{completed} von {total} Einträgen erledigt',
      previousWeek: 'Vorherige Woche',
      thisWeek: 'diese Woche',
      nextWeek: 'Nächste Woche',
      newTaskPlaceholder: 'Neuer Aufgabentitel...',
      failedToCreateTask: 'Aufgabe konnte nicht erstellt werden',
    },

    // Entry
    entry: {
      title: 'Zeiteintrag',
      subtitle: 'Erfassen Sie Ihre Arbeitsstunden und verwalten Sie Zeiteinträge',
      addTimeEntry: 'Neuen Zeiteintrag hinzufügen',
      failedToSave:
        'Zeiteintrag konnte nicht gespeichert werden. Bitte überprüfen Sie Ihre Eingaben.',
      confirmDelete: 'Sind Sie sicher, dass Sie diesen Zeiteintrag löschen möchten?',
      needProjects: 'Sie müssen aktive Projekte erstellen, bevor Sie Zeit erfassen können.',
      goToProjects: 'Zu Projekten gehen',
      startSetupWizard: 'Einrichtungsassistenten starten',
      applyFiltersTitle: 'Ausgewählte Filter auf die Zeiteinträge anwenden',
      clearFiltersTitle: 'Alle Filter löschen und alle Zeiteinträge anzeigen',
      noEntries:
        'Keine Zeiteinträge gefunden. Erfassen Sie Ihren ersten Zeiteintrag, um loszulegen.',
      clickToView: 'Klicken Sie hier, um {key}-{number} anzuzeigen',
      budgetExceeded: '- Budget überschritten',
    },

    // Organizations (formerly Customers)
    organizations: {
      title: 'Organisationen',
      subtitle: 'Verwalten Sie Ihre Kundenorganisationen',
      addOrganization: 'Organisation hinzufügen',
      noOrganizations:
        'Keine Organisationen gefunden. Erstellen Sie Ihre erste Organisation, um loszulegen.',
      name: 'Organisationsname',
      emailOptional: 'E-Mail (Optional)',
      contactPersonOptional: 'Kontaktperson (Optional)',
      failedToSave: 'Organisation konnte nicht gespeichert werden',
      deleteOrganization: 'Organisation löschen',
      confirmDelete: 'Sind Sie sicher, dass Sie "{name}" löschen möchten?',
    },

    // Permissions
    permissions: {
      title: 'Projektberechtigungen',
      managePermissions: 'Berechtigungen verwalten',
      managers: 'Manager',
      members: 'Mitglieder',
      addManager: 'Manager hinzufügen',
      addMember: 'Mitglied hinzufügen',
      removeManager: 'Manager entfernen',
      removeMember: 'Mitglied entfernen',
      noManagers: 'Keine Manager zugewiesen',
      noManagersHint: 'Wenn keine Manager zugewiesen sind, kann jeder dieses Projekt verwalten',
      noMembers: 'Keine Mitglieder zugewiesen',
      noMembersHint:
        'Wenn keine Mitglieder zugewiesen sind, kann jeder Zeit auf dieses Projekt buchen',
      grantedAt: 'Hinzugefügt',
      confirmRemove: 'Sind Sie sicher, dass Sie {name} entfernen möchten?',
      failedToAdd: 'Hinzufügen fehlgeschlagen',
      failedToRemove: 'Entfernen fehlgeschlagen',
      managersNote: 'Manager:',
      managersNoteText:
        'Können Projekteinstellungen bearbeiten, Mitglieder verwalten und alle Zeiteinträge für dieses Projekt einsehen.',
      membersNote: 'Mitglieder:',
      membersNoteText:
        'Können Zeit auf dieses Projekt buchen und ihre eigenen Zeiteinträge einsehen.',
    },
  },

  timeProject: {
    editProject: 'Projekt bearbeiten',
    newProject: 'Neues Projekt',
    projectName: 'Projektname',
    status: 'Status',
    customerOptional: 'Kunde (Optional)',
    none: 'Keine',
    categoryOptional: 'Kategorie (Optional)',
    hourlyRate: 'Stundensatz (€)',
    maxHours: 'Max. Stunden',
    maxHoursPlaceholder: 'Kein Limit',
    maxHoursHint: 'Optionales Budget für Berichte',
    projectColor: 'Projektfarbe',
    updateProject: 'Projekt aktualisieren',
    createProject: 'Projekt erstellen',
  },

  timeProjectCategory: {
    editCategory: 'Kategorie bearbeiten',
    newCategory: 'Neue Kategorie',
    categoryName: 'Kategoriename',
    categoryNamePlaceholder: 'Entwicklung, Marketing, Betrieb...',
    optionalDescription: 'Optionale Beschreibung...',
    updateCategory: 'Kategorie aktualisieren',
    createCategory: 'Kategorie erstellen',
  },
};
