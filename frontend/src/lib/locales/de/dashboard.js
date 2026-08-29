export default {
  dashboard: {
    salutation: {
      withName: '{salutation}, {name}!',
      withoutName: '{salutation}!',
    },
    sections: {
      yourDay: {
        title: 'Ihr Tag',
        subtitle: 'Ein schneller Überblick darüber, was Ihre Aufmerksamkeit braucht',
      },
      work: {
        title: 'Arbeit',
        subtitle: 'Ihre persönliche Liste und die Ihnen zugewiesenen Einträge',
      },
      workspaces: {
        title: 'Arbeitsbereiche',
        subtitle: 'Direkt weitermachen',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'Täglicher Überblick',
        description: 'KI-generierte Zusammenfassung dessen, was heute für Sie wichtig ist',
      },
      yourActivity: {
        name: 'Ihre Aktivität',
        description: 'Kürzlich angesehene, bearbeitete oder kommentierte Einträge',
      },
      whatsNew: {
        name: 'Neuigkeiten',
        description: 'Neueste Benachrichtigungen und ungelesene Aktualisierungen',
      },
      personalTasks: {
        name: 'Persönliche Aufgaben',
        description: 'Einträge aus Ihrer persönlichen Aufgabenliste',
      },
      savedSearch: {
        name: 'Gespeicherte Suche',
        description: 'Einträge aus einer gespeicherten Sammlung anzeigen',
      },
      assignedToMe: {
        name: 'Mir zugewiesen',
        description: 'Ihnen zugewiesene offene Einträge aus allen Arbeitsbereichen',
      },
      watchedItems: {
        name: 'Beobachtete Einträge',
        description: 'Einträge, denen Sie folgen',
      },
      upcomingMilestones: {
        name: 'Anstehende Meilensteine',
        description: 'Meilensteine mit nahendem Zieldatum',
      },
      recentWorkspaces: {
        name: 'Letzte Arbeitsbereiche',
        description: 'Kürzlich besuchte Arbeitsbereiche',
      },
      quickAccess: {
        name: 'Schnellzugriff',
        description: 'Direkte Verknüpfungen zu erreichbaren Arbeitsbereichen',
      },
    },
    customization: {
      widgets: 'Widgets',
      activity: {
        name: 'Aktivität',
        description: 'Überblicke, Aktivitätsverläufe und Benachrichtigungen',
      },
      work: {
        name: 'Arbeit',
        description: 'Einträge, Meilensteine und Ihnen zugewiesene Aufgaben',
      },
      navigation: {
        name: 'Navigation',
        description: 'Schnellzugriff auf Arbeitsbereiche',
      },
      tipLabel: 'Tipp',
      tip: 'Ziehen Sie Widgets von hier in einen beliebigen Bereich Ihres Dashboards.',
    },
    editor: {
      newSection: 'Neuer Bereich',
      deleteSectionConfirm: 'Diesen Bereich löschen? Alle darin enthaltenen Widgets werden entfernt.',
      doneEditing: 'Bearbeitung beenden',
      customize: 'Anpassen',
      editModeDescription: 'Bearbeitungsmodus: Bereiche und Widgets hinzufügen, umbenennen, neu anordnen oder löschen',
      addSection: 'Bereich hinzufügen',
      sectionLabel: 'Dashboard-Bereich',
      sectionTitlePlaceholder: 'Bereichstitel',
      sectionSubtitlePlaceholder: 'Untertitel (optional)',
      renameSection: 'Bereich umbenennen',
      deleteSection: 'Bereich löschen',
      unknownWidgetType: 'Unbekannter Widget-Typ: {type}',
      noWidgets: 'Dieser Bereich enthält noch keine Widgets',
      addWidgetsHint: 'Wählen Sie Anpassen, um Widgets hinzuzufügen',
      noSections: 'Keine Bereiche konfiguriert',
      addSectionsHint: 'Wählen Sie Bearbeiten, um Bereiche zum Dashboard hinzuzufügen',
    },
    states: {
      assignedLoadError: 'Die Ihnen zugewiesenen Einträge konnten nicht geladen werden',
      assignedEmpty: 'Ihnen ist derzeit nichts zugewiesen',
      personalTasksLoadError: 'Ihre persönlichen Aufgaben konnten nicht geladen werden',
      personalTasksEmpty: 'Ihre persönliche Aufgabenliste ist leer',
      dailyBriefingUnavailable: 'Ihr täglicher Überblick ist derzeit nicht verfügbar. Er benötigt eine KI-Integration. Falls Sie gerade eine eingerichtet haben, versuchen Sie es in Kürze erneut.',
      updatedAt: 'Aktualisiert: {time}',
      priorityLabel: 'Priorität: {priority}',
      noWorkspaces: 'Noch keine Arbeitsbereiche',
      createWorkspace: 'Arbeitsbereich erstellen',
      workspaceAvatarAlt: 'Avatar von {name}',
      visited: 'besucht {time}',
      noUpcomingMilestones: 'Keine anstehenden Meilensteine',
      milestoneProgress: '{done} von {total} erledigt',
      daysOverdue: '{days} Tage überfällig',
      daysLeft: 'Noch {days} Tage',
      watchedItemsEmpty: 'Sie beobachten derzeit keine Einträge',
      workspaceWithId: 'Arbeitsbereich {id}',
    },
  },
};
