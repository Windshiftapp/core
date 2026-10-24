/** WI-726 terminology, orthography, and register overrides. */
export default {
  "settings": {
    "adminItems": {
      "hierarchyLevels": {
        "description": "Hierarchieebenen für Vorgänge konfigurieren"
      },
      "itemTypes": {
        "title": "Vorgangstypen",
        "description": "Vorgangstypen mit Symbolen und Farben verwalten"
      }
    },
    "configSets": {
      "itemTypes": "Vorgangstypen",
      "noItemTypesAssigned": "Keine Vorgangstypen zugewiesen",
      "defaultItemType": "Standard-Vorgangstyp",
      "configurePerItemType": "Pro Vorgangstyp konfigurieren",
      "preselectedItemType": "Vorausgewählter Vorgangstyp beim Erstellen neuer Vorgänge",
      "selectItemTypes": "Wählen Sie, welche Vorgangstypen in Arbeitsbereichen mit diesem Konfigurationsset verfügbar sind.",
      "overridesDesc": "Benutzerdefinierte Workflows und Ansichten pro Vorgangstyp konfigurieren. 'Standard' verwenden, um vom Allgemein-Tab zu erben.",
      "itemType": "Vorgangstyp"
    },
    "itemTypes": {
      "title": "Vorgangstypen",
      "subtitle": "Vorgangstypen verwalten und Hierarchieebenen zuweisen",
      "addItemType": "Vorgangstyp hinzufügen",
      "failedToSave": "Vorgangstyp konnte nicht gespeichert werden:",
      "noItemTypes": "Noch keine Vorgangstypen konfiguriert."
    },
    "boardConfig": {
      "backlogStatusesHelp": "Vorgänge mit diesen Status erscheinen im Backlog"
    },
    "aiFeatures": {
      "features": {
        "ai_chat": {
          "description": "Interaktiver Chat mit Arbeitsbereich-bewusstem KI-Assistenten"
        }
      }
    }
  },
  "users": {
    "calendarIntegrationDesc": "Abonnieren Sie Ihre geplanten Vorgänge in externen Kalender-Apps",
    "calendarFeedWarning": "Teilen Sie diese URL nicht, da sie Zugriff auf Ihre geplanten Vorgänge gewährt."
  },
  "common": {
    "noItems": "Keine Vorgänge",
    "noItemsFound": "Keine Vorgänge gefunden",
    "ticket": "Vorgang",
    "tickets": "Vorgänge",
    "noTickets": "Keine Vorgänge",
    "item": "Vorgang",
    "items": "Vorgänge"
  },
  "errors": {
    "ITEM_NOT_FOUND": "Der Vorgang wurde nicht gefunden."
  },
  "placeholders": {
    "searchWorkItems": "Vorgänge suchen..."
  },
  "emptyStates": {
    "noItemsMatch": "Keine Vorgänge entsprechen dem Filter",
    "configureFilter": "Filter konfigurieren, um Vorgänge zu sehen"
  },
  "channel": {
    "allItems": "Alle Vorgänge (instanzweit)",
    "itemType": "Vorgangstyp",
    "itemTypeRequired": "Vorgangstyp ist erforderlich",
    "selectItemType": "Vorgangstyp auswählen...",
    "markAsReadHelp": "E-Mails als gelesen markieren, sobald sie in Vorgänge umgewandelt wurden",
    "items": "Vorgänge",
    "itemCreated": "Vorgang erstellt",
    "itemUpdated": "Vorgang aktualisiert",
    "itemDeleted": "Vorgang gelöscht",
    "itemAssigned": "Vorgang zugewiesen",
    "itemLinked": "Vorgang verknüpft",
    "emailLog": {
      "newItem": "Neuer Vorgang {key}"
    },
    "workspaceFieldResolution": "Arbeitsbereich-Feldauflösung"
  },
  "portal": {
    "draftsTitle": "Ihre Entwürfe",
    "draftsSubtitle": "Machen Sie dort weiter, wo Sie aufgehört hast.",
    "draftsEmpty": "Noch keine Entwürfe. Starte eine Anfrage und Ihr Fortschritt wird automatisch gespeichert.",
    "itemTypeRequired": "Vorgangstyp ist erforderlich",
    "createsItemType": "Erstellt Vorgangstyp",
    "selectItemType": "Vorgangstyp auswählen...",
    "submissionsCreateItemType": "Einreichungen erstellen diesen Vorgangstyp als Vorgang",
    "itemType": "Vorgangstyp"
  },
  "forms": {
    "createsItemType": "Erstellt Vorgangstyp",
    "selectItemType": "Vorgangstyp auswählen"
  },
  "jiraImport": {
    "subtitle": {
      "cloud": "Vorgänge aus Jira Cloud importieren",
      "datacenter": "Vorgänge aus Jira Data Center importieren"
    },
    "mapping": {
      "issueTypesDesc": "Ordnen Sie jeden Jira-Issuetyp einem neuen oder vorhandenen Windshift-Vorgangstyp zu",
      "versionsDesc": "Jira-Versionen werden als Arbeitsbereich-Meilensteine importiert."
    },
    "preview": {
      "workItems": "Vorgänge"
    },
    "import": {
      "success": "{count} Vorgänge erfolgreich importiert.",
      "failed": "{count} Vorgänge konnten nicht importiert werden.",
      "readyDesc": "Klicken Sie auf \"Import starten\", um {count} Vorgänge zu importieren."
    }
  },
  "iterations": {
    "noItems": "Keine Vorgänge",
    "noItemsAssigned": "Keine Vorgänge zugewiesen",
    "itemsDone": "{count} Vorgänge erledigt",
    "allItemsDone": "Alle Vorgänge sind erledigt!",
    "incompleteItemsAction": "Unvollständige Vorgänge:"
  },
  "milestones": {
    "noItems": "Keine Vorgänge",
    "noItemsAssigned": "Keine Vorgänge zugewiesen",
    "globalMilestoneDescription": "Sichtbar in allen Arbeitsbereiche",
    "localMilestoneDescription": "Nur in diesem Arbeitsbereich sichtbar"
  },
  "personal": {
    "loadingCompletedItems": "Erledigte Vorgänge werden geladen...",
    "noCompletedItemsDay": "Keine erledigten Vorgänge für diesen Tag",
    "noCompletedItemsWeek": "Keine erledigten Vorgänge für diese Woche"
  },
  "migration": {
    "migrationSuccess": "Alle Vorgänge wurden erfolgreich migriert"
  },
  "migrationAssistant": {
    "allItemsCompatible": "Alle Vorgänge ({count}) sind mit der neuen Konfiguration kompatibel.",
    "itemsNeedMigration": "{count} Vorgänge müssen migriert werden. Bitte überprüfen Sie die Zuordnungen unten.",
    "noItemsToMigrate": "Keine Vorgänge zu migrieren.",
    "item": "Vorgang",
    "items": "Vorgänge",
    "allItemsMigrated": "Alle Vorgänge wurden erfolgreich migriert.",
    "pleaseSelectTargetStatuses": "Bitte wählen Sie Zielstatus für alle Vorgänge aus, die eine Migration erfordern.",
    "pleaseSelectTargetItemTypes": "Bitte wählen Sie Zieltypen für alle Vorgänge aus, die eine Migration erfordern.",
    "pleaseSelectTargetPriorities": "Bitte wählen Sie Zielprioritäten für alle Vorgänge aus, die eine Migration erfordern."
  },
  "createModal": {
    "workItem": "Vorgang"
  },
  "scm": {
    "linkDevResourceDesc": "PR, Branch oder Commit mit diesem Vorgang verbinden"
  },
  "fields": {
    "subtitle": "Benutzerdefinierte Felder für Ihre Vorgänge definieren"
  },
  "categories": {
    "deleteWarning": "Vorgänge in dieser Kategorie werden unkategorisiert",
    "confirmDeleteCategory": "Kategorie \"{name}\" löschen? Vorgänge in dieser Kategorie werden unkategorisiert."
  },
  "commandPalette": {
    "commands": {
      "search": {
        "description": "Vorgänge und Inhalte durchsuchen"
      },
      "createWorkItem": {
        "label": "Vorgang erstellen",
        "description": "Neuen Vorgang oder Aufgabe erstellen"
      },
      "adminItemTypes": {
        "label": "Vorgangstypen",
        "description": "Vorgangstypen mit Icons und Farben verwalten"
      },
      "createItem": {
        "label": "Vorgang erstellen",
        "description": "Neuen Vorgang erstellen"
      },
      "assets": {
        "description": "Asset-Sets und Vorgänge verwalten"
      }
    },
    "recentlyViewed": {
      "description": "Springen Sie zu einem Ihrer zuletzt 20 angesehenen Artikel"
    }
  },
  "dashboard": {
    "subtitle": "Übersicht über Ihre Vorgänge und Projekte",
    "recentItems": "Letzte Vorgänge",
    "overdueItems": "Überfällige Vorgänge",
    "recentItemsHint": "Aktualisierte Vorgänge erscheinen hier",
    "dueDatesHint": "Vorgänge mit Fälligkeitsdaten erscheinen hier",
    "noOverdueItems": "Keine überfälligen Vorgänge",
    "loadingRecentItems": "Lade letzte Vorgänge...",
    "loadingOverdueItems": "Lade überfällige Vorgänge...",
    "workItemStatusOverview": "Vorgangs-Status Übersicht",
    "createWorkItem": "Vorgang erstellen",
    "createWorkItemDesc": "Neuen Vorgang erfassen",
    "recentWorkItems": "Letzte Vorgänge",
    "noRecentlyViewed": "Keine kürzlich angesehenen Vorgänge",
    "noRecentlyEdited": "Keine kürzlich bearbeiteten Vorgänge",
    "noRecentlyCommented": "Keine kürzlich kommentierten Vorgänge"
  },
  "search": {
    "searchItems": "Vorgänge suchen...",
    "configureFilter": "Filter konfigurieren, um Vorgänge zu sehen",
    "workspace": "Arbeitsbereich"
  },
  "about": {
    "projectManagementDesc": "Organisieren Sie Vorgänge hierarchisch mit benutzerdefinierten Feldern, Workflows und Statusverfolgung."
  },
  "onboarding": {
    "createFirstWorkItem": "Ersten Vorgang erstellen",
    "createWorkItemBtn": "Vorgang erstellen",
    "getStartedMember": "Hier sind die für Sie verfügbaren Arbeitsbereiche",
    "selectWorkspace": "Wählen Sie einen Arbeitsbereich zum Starten",
    "noWorkspacesAvailable": "Es sind noch keine Arbeitsbereiche verfügbar. Bitte kontaktieren Sie Ihren Administrator, um Zugang zu einem Arbeitsbereich zu erhalten."
  },
  "testing": {
    "linkExistingItem": "Vorhandenen Vorgang verknüpfen",
    "searchItemsToLink": "Vorgänge zum Verknüpfen suchen...",
    "unknownItem": "Unbekannter Vorgang",
    "itemsWithoutTestCoverage": "Vorgänge ohne Testabdeckung",
    "selectWorkspacesAndTypes": "Wählen Sie Arbeitsbereiche und Vorgangstypen aus, die in den Bericht aufgenommen werden sollen",
    "itemTypes": "Vorgangstypen",
    "selectItemTypes": "Vorgangstypen auswählen",
    "allItemTypes": "Alle Vorgangstypen",
    "selectItemTypesForCoverage": "Wählen Sie Vorgangstypen aus, die für die Abdeckungsberichte verfolgt werden sollen",
    "selectItemTypesForCoverageAnalysis": "Wählen Sie Vorgangstypen für die Abdeckungsanalyse aus",
    "noItemsMatchingRequirements": "Keine Vorgänge entsprechen den konfigurierten Anforderungstypen",
    "noItemTypesAvailable": "Keine Vorgangstypen verfügbar"
  },
  "testCase": {
    "backToItem": "Zurück zum Vorgang"
  },
  "time": {
    "subtitle": "Zeit für Vorgänge erfassen",
    "workItemOptional": "Vorgang (Optional)",
    "timer": {
      "goToWorkItem": "Zum Vorgang gehen: {title}"
    },
    "calendar": {
      "itemCount": "{count} Vorgänge",
      "myWorkItems": "Meine Vorgänge",
      "dragToSchedule": "Vorgänge ziehen zum Planen",
      "noWorkItems": "Keine Vorgänge zugewiesen",
      "workItemsWillAppear": "Vorgänge erscheinen hier, wenn sie Ihnen zugewiesen werden"
    }
  },
  "pickers": {
    "noItemsFound": "Keine Vorgänge gefunden",
    "noItemsAvailable": "Keine Vorgänge verfügbar",
    "fields": {
      "reporter": {
        "description": "Wer den Vorgang erstellt hat"
      },
      "createdAt": {
        "description": "Wann der Vorgang erstellt wurde"
      },
      "updatedAt": {
        "description": "Wann der Vorgang zuletzt aktualisiert wurde"
      },
      "dueDate": {
        "description": "Wann der Vorgang fällig ist"
      },
      "parent": {
        "description": "Übergeordneter Vorgang"
      },
      "children": {
        "description": "Untergeordnete Vorgänge"
      },
      "links": {
        "description": "Verwandte Vorgänge"
      },
      "watchers": {
        "description": "Benutzer, die diesen Vorgang beobachten"
      }
    },
    "agentOffline": "Agent offline – sein Läuferpool hat keinen Live-Läufer; Zugewiesene Vorgänge werden in die Warteschlange gestellt"
  },
  "dialogs": {
    "confirmations": {
      "deleteScreen": "Möchten Sie den Screen \"{name}\" wirklich löschen? Dies betrifft alle Arbeitsbereiche, die diesen Screen verwenden."
    }
  },
  "components": {
    "pagination": {
      "limitedTo": "begrenzt auf {max} Vorgänge",
      "itemsPerPage": "Vorgänge pro Seite:"
    }
  },
  "layout": {
    "items": "Vorgänge"
  },
  "widgets": {
    "chart": {
      "items": "Vorgänge"
    },
    "milestoneProgress": {
      "item": "Vorgang",
      "items": "Vorgänge",
      "noItems": "Keine Vorgänge"
    },
    "myTasks": {
      "loadingText": "Ihre Aufgaben werden geladen...",
      "emptyTitle": "Ihnen sind keine Aufgaben zugewiesen",
      "emptySubtitle": "Ihnen zugewiesene Aufgaben erscheinen hier"
    },
    "overdueItems": {
      "itemCount": "{count} überfällige Vorgänge",
      "refreshAriaLabel": "Überfällige Vorgänge aktualisieren",
      "loadingText": "Überfällige Vorgänge werden geladen...",
      "emptyTitle": "Keine überfälligen Vorgänge",
      "emptySubtitle": "Alle Vorgänge sind im Zeitplan",
      "loadError": "Überfällige Vorgänge konnten nicht geladen werden"
    },
    "upcomingDeadlines": {
      "emptySubtitle": "Vorgänge mit Fälligkeitsdatum erscheinen hier"
    },
    "recentItems": {
      "loadingText": "Letzte Vorgänge werden geladen...",
      "emptyTitle": "Keine aktuellen Vorgänge",
      "emptySubtitle": "Kürzlich angesehene Vorgänge werden angezeigt hier",
      "loadError": "Letzte Vorgänge konnten nicht geladen werden"
    }
  },
  "statusCategory": {
    "marksWorkCompletedHelp": "Vorgänge, die in Status dieser Kategorie verschoben werden, werden in Berichten und Rückblicken als abgeschlossen behandelt."
  },
  "items": {
    "title": "Vorgänge",
    "subtitle": "Vorgänge anzeigen und verwalten",
    "item": "Vorgang",
    "items_one": "{count} Vorgang",
    "items_other": "{count} Vorgänge",
    "createItem": "Vorgang erstellen",
    "editItem": "Vorgang bearbeiten",
    "deleteItem": "Vorgang löschen",
    "viewItem": "Vorgang anzeigen",
    "linkedItems": "Verknüpfte Vorgänge",
    "noItems": "Keine Vorgänge gefunden",
    "noItemsInFilter": "Keine Vorgänge entsprechen dem aktuellen Filter",
    "createToStart": "Erstellen Sie einen Vorgang, um zu beginnen",
    "itemCreated": "Vorgang erfolgreich erstellt",
    "itemUpdated": "Vorgang erfolgreich aktualisiert",
    "itemDeleted": "Vorgang erfolgreich gelöscht",
    "workItem": "Vorgang",
    "startTimerTitle": "Zeiterfassung für diesen Vorgang starten",
    "logTimeTitle": "Arbeitszeit für diesen Vorgang manuell erfassen",
    "includeChildItems": "Untergeordnete Vorgänge einbeziehen",
    "rollupTruncated": "Auf erste {max} untergeordnete Vorgänge begrenzt",
    "rollupItemCount_one": "aggregiert über {count} Vorgang",
    "rollupItemCount_other": "aggregiert über {count} Vorgänge",
    "workItemNotFound": "Vorgang nicht gefunden",
    "itemCopiedAs": "Vorgang erfolgreich als {key} kopiert",
    "clickToViewCopied": "Klicken, um kopierten Vorgang anzuzeigen",
    "deleteWorkItem": "Vorgang löschen",
    "deleteItemWithChildren": "Vorgang mit untergeordneten Vorgängen löschen",
    "itemHasChildren": "Dieser Vorgang hat {count} untergeordnete Vorgänge.",
    "itemHasChildrenSingular": "Dieser Vorgang hat 1 untergeordneten Vorgang.",
    "deleteAllOption": "Alle löschen ({count} Vorgänge)",
    "deleteAllDescription": "Diesen Vorgang und alle untergeordneten Vorgänge dauerhaft löschen",
    "reparentDescription": "Untergeordnete zum übergeordneten Vorgang verschieben, dann nur diesen Vorgang löschen",
    "deleteAllItems": "Alle Vorgänge löschen",
    "deletedItemsCount": "{count} Vorgänge gelöscht",
    "reparentedAndDeleted": "Untergeordnete verschoben und Vorgang gelöscht",
    "selectNewParentPlaceholder": "Übergeordneten Vorgang auswählen...",
    "reparentLevelHint": "Zeigt nur Vorgänge auf derselben Hierarchieebene",
    "noOtherItemsAtLevel": "Keine anderen Vorgänge auf dieser Ebene - wählen Sie \"Zu Stammeinträgen machen\" oder wählen Sie aus der Liste oben",
    "copyWorkItem": "Vorgang kopieren",
    "unwatchWorkItem": "Vorgang nicht mehr beobachten",
    "watchWorkItem": "Vorgang beobachten",
    "cannotCreateChildItems": "Für diese Hierarchieebene können keine untergeordneten Vorgänge erstellt werden.",
    "workItems": "Vorgänge",
    "goToLinkedWorkItem": "Zum verknüpften Vorgang gehen",
    "searchForParentItem": "Nach übergeordnetem Vorgang suchen...",
    "showingItemsFromLevel": "Zeige nur Vorgänge der Hierarchieebene {level}",
    "searchParentAcrossWorkspaces": "Nach übergeordnetem Vorgang in allen Arbeitsbereichen suchen",
    "noItemsAtLevel": "Keine Vorgänge auf Hierarchieebene {level} gefunden",
    "searchWorkItems": "Vorgänge suchen...",
    "linkToTestCase": "Diesen Vorgang mit einem Testfall verknüpfen.",
    "childWorkItems": "Untergeordnete Vorgänge",
    "loadingChildItems": "Untergeordnete Vorgänge werden geladen...",
    "agentLogEmpty": "Für diesen Vorgang wird noch kein Agent ausgeführt",
    "agentRerunTitle": "Führen Sie den Agenten für diesen Vorgang erneut aus.",
    "rollupItemCount": "zusammengefasst über {count} Vorgänge",
    "linkToPage": "Verknüpfen Sie diesen Vorgang mit einer Wissensseite im selben Arbeitsbereich."
  },
  "comments": {
    "beFirstToComment": "Seien Sie der Erste, der diesen Vorgang kommentiert."
  },
  "todo": {
    "assignedItemsWillAppear": "Ihnen zugewiesene Vorgänge werden hier angezeigt."
  },
  "collectionTree": {
    "noWorkItemsYet": "Noch keine Vorgänge",
    "createFirstWorkItem": "Erstellen Sie Ihren ersten Vorgang, um den Hierarchiebaum zu sehen."
  },
  "collections": {
    "searchItems": "Vorgänge suchen...",
    "searchItemsTitle": "Vorgänge suchen",
    "noItemsInBacklog": "Keine Vorgänge im Backlog",
    "noItemsInBacklogDesc": "Alle Vorgänge sind entweder abgeschlossen oder es existieren noch keine Vorgänge.",
    "showingItemsFromBacklog": "{count} Vorgänge aus dem Backlog anzeigen",
    "childItems": "{count} untergeordnete(r) Vorgang/Vorgänge",
    "childWorkItems": "Untergeordnete Vorgänge ({count})",
    "noChildItems": "Noch keine untergeordneten Vorgänge",
    "noChildItemsLowest": "Keine untergeordneten Vorgänge (niedrigste Hierarchieebene)",
    "noTopLevelItems": "Keine Vorgänge auf oberster Ebene gefunden",
    "noTopLevelItemsDesc": "Erstellen Sie einige Vorgänge, um Ihre Story Map zu sehen",
    "loadingWorkItems": "Vorgänge werden geladen...",
    "noWorkItemsFound": "Keine Vorgänge gefunden",
    "showingWorkItems": "{count} Vorgänge anzeigen",
    "boardSummary": "Gesamt: {itemCount} Vorgänge in {columnCount} Spalten",
    "roadmapNoItems": "Keine Vorgänge mit Daten im aktuellen Zeitraum.",
    "dragItemsHere": "Vorgänge hierher ziehen, um sie diesem Sprint hinzuzufügen",
    "allItems": "Alle Vorgänge"
  },
  "links": {
    "subtitle": "Vorgangs-Links verwalten"
  },
  "workspaceSettings": {
    "workspaceKeyHelp": "Wird für Vorgangs-Präfixe verwendet (z.B. DEV-123). Nur Großbuchstaben und Zahlen.",
    "defaultTimeProjectHelp": "Standardprojekt für die Zeiterfassung bei Vorgängen in diesem Arbeitsbereich. Kann pro Vorgang überschrieben werden.",
    "removeWarningItems": "Alle Vorgänge und Projekte in diesem Arbeitsbereich",
    "headers": {
      "recurrence": "Regeln für wiederkehrende Vorgänge verwalten",
      "templates": "Wiederverwendbare Beschreibungsgerüste für neue Vorgänge"
    }
  },
  "issueSync": {
    "subtitle": "GitHub Issues als Vorgänge in diesen Arbeitsbereich synchronisieren",
    "noConfig": "Die Issue-Synchronisierung ist für diesen Arbeitsbereich nicht konfiguriert.",
    "noConfigDescription": "Verknüpfen Sie ein Repository und konfigurieren Sie, wie GitHub Issues in Windshift-Vorgänge synchronisiert werden sollen.",
    "enabledDescription": "Wenn aktiviert, werden GitHub Issues regelmäßig in diesen Arbeitsbereich synchronisiert.",
    "itemType": "Vorgangstyp",
    "itemTypeDescription": "Wählen Sie den Vorgangstyp für synchronisierte Issues. Dies bestimmt, welche Workflow-Status für die Zuordnung verfügbar sind.",
    "selectItemType": "Vorgangstyp auswählen",
    "selectItemTypeHint": "Wählen Sie oben einen Vorgangstyp, um die Status-Zuordnung zu konfigurieren.",
    "syncedItems": "Synchronisierte Vorgänge",
    "noLinkedRepos": "Keine Repositories mit diesem Arbeitsbereich verknüpft. Verknüpfen Sie zuerst ein Repository in den Quellcodeverwaltungseinstellungen.",
    "confirmDelete": "Möchten Sie die Issue-Synchronisierungskonfiguration wirklich entfernen? Synchronisierte Vorgänge bleiben erhalten, werden aber nicht mehr aktualisiert."
  },
  "actions": {
    "noActionsDescription": "Erstellen Sie Aktionen, um Ihre Workflows basierend auf Vorgangs-Ereignissen zu automatisieren",
    "trigger": {
      "itemCreated": "Vorgang erstellt",
      "itemUpdated": "Vorgang aktualisiert",
      "itemLinked": "Vorgang verknüpft"
    },
    "nodes": {
      "relatedItems": "Für jeden verwandten Vorgang",
      "transitionItem": "Vorgang überführen"
    },
    "config": {
      "includeLink": "Link zum Vorgang einfügen",
      "sourceAssetField": "Asset-Feld am Vorgang",
      "sourceTypeItemField": "Vorgangs-Feld"
    },
    "test": {
      "description": "Wählen Sie einen Vorgang aus, für den diese Aktion ausgeführt werden soll. Die Aktion wird sofort ausgeführt und umgeht den normalen Auslöser.",
      "selectItem": "Vorgang auswählen",
      "itemPlaceholder": "Nach einem Vorgang suchen..."
    },
    "placeholders": {
      "categories": {
        "item": "Vorgangs-Felder"
      },
      "item": {
        "title": "Vorgangs-Titel",
        "id": "Vorgangs-ID",
        "any": "Beliebiges Vorgangs-Feld"
      },
      "trigger": {
        "itemId": "Auslösende Vorgangs-ID"
      }
    }
  },
  "analytics": {
    "collectionLoadError": "Sammlungen konnten nicht geladen werden. Die Analyse zeigt alle Arbeitsbereich-Vorgänge.",
    "allItems": "Alle Arbeitsbereich-Vorgänge",
    "items_one": "{count} Vorgang",
    "items_other": "{count} Vorgänge",
    "scope": {
      "summary": "{items} aktuelle Vorgänge · {from}–{to}",
      "currentWorkspace": "Aktuelle Arbeitsbereich-Kohorte",
      "currentWorkspaceNote": "Der Zeitraum gilt für Fluss- und Lieferdiagramme; Zustand und Alter sind aktuelle Momentaufnahmen. Historische Diagramme verwenden Vorgänge, die heute in diesem Arbeitsbereich liegen. Verschobene oder gelöschte Vorgänge sind nicht enthalten.",
      "currentCollectionNote": "Der Zeitraum gilt für Fluss- und Lieferdiagramme; Zustand und Alter sind aktuelle Momentaufnahmen. Historische Diagramme verwenden Vorgänge, die heute zur Sammlung passen. Änderungen an der Sammlung können die Kohorte verändern."
    },
    "health": {
      "attentionItems": "Zu prüfende Vorgänge",
      "item": "Vorgang",
      "allClear": "Keine unerledigten Vorgänge entsprechen derzeit einem Warnsignal."
    },
    "aging": {
      "description": "Wie lange aktuell unerledigte Vorgänge bereits offen sind.",
      "total": "Aktive Vorgänge",
      "itemCount": "Vorgänge",
      "oldest": "Älteste unerledigte Vorgänge"
    },
    "deliveryTime": {
      "analyzed": "Abgeschlossene Vorgänge",
      "missingHistory_one": "1 aktuell abgeschlossener Vorgang wurde wegen fehlender Abschlusshistorie ausgeschlossen.",
      "missingHistory_other": "{count} aktuell abgeschlossene Vorgänge wurden wegen fehlender Abschlusshistorie ausgeschlossen.",
      "missingHistory": "{count} Derzeit abgeschlossene Vorgänge wurden ausgeschlossen, da ihr Abschlussverlauf fehlt."
    },
    "insufficientData": {
      "no_items": "Dieser Bereich enthält noch keine Vorgänge.",
      "few_completed_items": "In diesem Zeitraum wurden nur wenige Vorgänge abgeschlossen. Perzentile sind daher nur als Tendenz zu verstehen."
    }
  },
  "recurrence": {
    "emptyDesc": "Hier werden Wiederholungsregeln angezeigt wenn für Vorgänge wiederkehrende Zeitpläne konfiguriert sind.",
    "createFromItem": "Um eine Wiederholungsregel zu erstellen, öffnen Sie einen Vorgang und richten Sie die Wiederholung in der Detailseitenleiste ein."
  },
  "conditionSets": {
    "errorMessagePlaceholder": "z. B. „Nur Tester können Vorgänge zur Qualitätssicherung verschieben“"
  },
  "approvalSets": {
    "allowSelfApproval": "Selbstgenehmigung zulassen – Erlaubt demselben Benutzer, den Vorgang sowohl zu übertragen als auch zu genehmigen."
  }
};

