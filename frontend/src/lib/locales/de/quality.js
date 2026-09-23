/** WI-726 terminology, orthography, and register overrides. */
export default {
  "settings": {
    "adminItems": {
      "hierarchyLevels": {
        "description": "Hierarchieebenen für Arbeitselemente konfigurieren"
      },
      "itemTypes": {
        "title": "Arbeitselementtypen",
        "description": "Arbeitselementtypen mit Symbolen und Farben verwalten"
      }
    },
    "configSets": {
      "itemTypes": "Arbeitselementtypen",
      "noItemTypesAssigned": "Keine Arbeitselementtypen zugewiesen",
      "defaultItemType": "Standard-Arbeitselementtyp",
      "configurePerItemType": "Pro Arbeitselementtyp konfigurieren",
      "preselectedItemType": "Vorausgewählter Arbeitselementtyp beim Erstellen neuer Arbeitselemente",
      "selectItemTypes": "Wählen Sie, welche Arbeitselementtypen in Arbeitsbereichen mit diesem Konfigurationsset verfügbar sind.",
      "overridesDesc": "Benutzerdefinierte Workflows und Ansichten pro Arbeitselementtyp konfigurieren. 'Standard' verwenden, um vom Allgemein-Tab zu erben.",
      "itemType": "Arbeitselementtyp"
    },
    "itemTypes": {
      "title": "Arbeitselementtypen",
      "subtitle": "Arbeitselementtypen verwalten und Hierarchieebenen zuweisen",
      "addItemType": "Arbeitselementtyp hinzufügen",
      "failedToSave": "Arbeitselementtyp konnte nicht gespeichert werden:",
      "noItemTypes": "Noch keine Arbeitselementtypen konfiguriert."
    },
    "boardConfig": {
      "backlogStatusesHelp": "Arbeitselemente mit diesen Status erscheinen im Backlog"
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
    "calendarIntegrationDesc": "Abonnieren Sie Ihre geplanten Arbeitselemente in externen Kalender-Apps",
    "calendarFeedWarning": "Teilen Sie diese URL nicht, da sie Zugriff auf Ihre geplanten Arbeitselemente gewährt."
  },
  "common": {
    "noItems": "Keine Arbeitselemente",
    "noItemsFound": "Keine Arbeitselemente gefunden",
    "ticket": "Arbeitselement",
    "tickets": "Arbeitselemente",
    "noTickets": "Keine Arbeitselemente",
    "item": "Arbeitselement",
    "items": "Arbeitselemente"
  },
  "errors": {
    "ITEM_NOT_FOUND": "Der Arbeitselement wurde nicht gefunden."
  },
  "placeholders": {
    "searchWorkItems": "Arbeitselemente suchen..."
  },
  "emptyStates": {
    "noItemsMatch": "Keine Arbeitselemente entsprechen dem Filter",
    "configureFilter": "Filter konfigurieren, um Arbeitselemente zu sehen"
  },
  "channel": {
    "allItems": "Alle Arbeitselemente (instanzweit)",
    "itemType": "Arbeitselementtyp",
    "itemTypeRequired": "Arbeitselementtyp ist erforderlich",
    "selectItemType": "Arbeitselementtyp auswählen...",
    "markAsReadHelp": "E-Mails als gelesen markieren, sobald sie in Arbeitselemente umgewandelt wurden",
    "items": "Arbeitselemente",
    "itemCreated": "Arbeitselement erstellt",
    "itemUpdated": "Arbeitselement aktualisiert",
    "itemDeleted": "Arbeitselement gelöscht",
    "itemAssigned": "Arbeitselement zugewiesen",
    "itemLinked": "Arbeitselement verknüpft",
    "emailLog": {
      "newItem": "Neuer Arbeitselement {key}"
    },
    "workspaceFieldResolution": "Arbeitsbereich-Feldauflösung"
  },
  "portal": {
    "draftsTitle": "Ihre Entwürfe",
    "draftsSubtitle": "Machen Sie dort weiter, wo Sie aufgehört hast.",
    "draftsEmpty": "Noch keine Entwürfe. Starte eine Anfrage und Ihr Fortschritt wird automatisch gespeichert.",
    "itemTypeRequired": "Arbeitselementtyp ist erforderlich",
    "createsItemType": "Erstellt Arbeitselementtyp",
    "selectItemType": "Arbeitselementtyp auswählen...",
    "submissionsCreateItemType": "Einreichungen erstellen diesen Arbeitselementtyp als Arbeitselement",
    "itemType": "Arbeitselementtyp"
  },
  "forms": {
    "createsItemType": "Erstellt Arbeitselementtyp",
    "selectItemType": "Arbeitselementtyp auswählen"
  },
  "jiraImport": {
    "subtitle": {
      "cloud": "Arbeitselemente aus Jira Cloud importieren",
      "datacenter": "Arbeitselemente aus Jira Data Center importieren"
    },
    "mapping": {
      "issueTypesDesc": "Arbeitselementtypen werden als Eintragstypen in Windshift erstellt",
      "versionsDesc": "Jira-Versionen werden als Arbeitsbereich-Meilensteine importiert."
    },
    "preview": {
      "workItems": "Arbeitselemente"
    },
    "import": {
      "success": "{count} Arbeitselemente erfolgreich importiert.",
      "failed": "{count} Arbeitselemente konnten nicht importiert werden.",
      "readyDesc": "Klicken Sie auf \"Import starten\", um {count} Arbeitselemente zu importieren."
    }
  },
  "iterations": {
    "noItems": "Keine Arbeitselemente",
    "noItemsAssigned": "Keine Arbeitselemente zugewiesen",
    "itemsDone": "{count} Arbeitselemente erledigt",
    "allItemsDone": "Alle Arbeitselemente sind erledigt!",
    "incompleteItemsAction": "Unvollständige Arbeitselemente:"
  },
  "milestones": {
    "noItems": "Keine Arbeitselemente",
    "noItemsAssigned": "Keine Arbeitselemente zugewiesen",
    "globalMilestoneDescription": "Sichtbar in allen Arbeitsbereiche",
    "localMilestoneDescription": "Nur in diesem Arbeitsbereich sichtbar"
  },
  "personal": {
    "loadingCompletedItems": "Erledigte Arbeitselemente werden geladen...",
    "noCompletedItemsDay": "Keine erledigten Arbeitselemente für diesen Tag",
    "noCompletedItemsWeek": "Keine erledigten Arbeitselemente für diese Woche"
  },
  "migration": {
    "migrationSuccess": "Alle Arbeitselemente wurden erfolgreich migriert"
  },
  "migrationAssistant": {
    "allItemsCompatible": "Alle Arbeitselemente ({count}) sind mit der neuen Konfiguration kompatibel.",
    "itemsNeedMigration": "{count} Arbeitselemente müssen migriert werden. Bitte überprüfen Sie die Zuordnungen unten.",
    "noItemsToMigrate": "Keine Arbeitselemente zu migrieren.",
    "item": "Arbeitselement",
    "items": "Arbeitselemente",
    "allItemsMigrated": "Alle Arbeitselemente wurden erfolgreich migriert.",
    "pleaseSelectTargetStatuses": "Bitte wählen Sie Zielstatus für alle Arbeitselemente aus, die eine Migration erfordern.",
    "pleaseSelectTargetItemTypes": "Bitte wählen Sie Zieltypen für alle Arbeitselemente aus, die eine Migration erfordern.",
    "pleaseSelectTargetPriorities": "Bitte wählen Sie Zielprioritäten für alle Arbeitselemente aus, die eine Migration erfordern."
  },
  "createModal": {
    "workItem": "Arbeitselement"
  },
  "scm": {
    "linkDevResourceDesc": "PR, Branch oder Commit mit diesem Arbeitselement verbinden"
  },
  "fields": {
    "subtitle": "Benutzerdefinierte Felder für Ihre Arbeitselemente definieren"
  },
  "categories": {
    "deleteWarning": "Arbeitselemente in dieser Kategorie werden unkategorisiert",
    "confirmDeleteCategory": "Kategorie \"{name}\" löschen? Arbeitselemente in dieser Kategorie werden unkategorisiert."
  },
  "commandPalette": {
    "commands": {
      "search": {
        "description": "Arbeitselemente und Inhalte durchsuchen"
      },
      "createWorkItem": {
        "label": "Arbeitselement erstellen",
        "description": "Neuen Arbeitselement oder Aufgabe erstellen"
      },
      "adminItemTypes": {
        "label": "Arbeitselementtypen",
        "description": "Arbeitselementtypen mit Icons und Farben verwalten"
      },
      "createItem": {
        "label": "Arbeitselement erstellen",
        "description": "Neuen Arbeitselement erstellen"
      },
      "assets": {
        "description": "Asset-Sets und Arbeitselemente verwalten"
      }
    },
    "recentlyViewed": {
      "description": "Springen Sie zu einem Ihrer zuletzt 20 angesehenen Artikel"
    }
  },
  "dashboard": {
    "subtitle": "Übersicht über Ihre Arbeitselemente und Projekte",
    "recentItems": "Letzte Arbeitselemente",
    "overdueItems": "Überfällige Arbeitselemente",
    "recentItemsHint": "Aktualisierte Arbeitselemente erscheinen hier",
    "dueDatesHint": "Arbeitselemente mit Fälligkeitsdaten erscheinen hier",
    "noOverdueItems": "Keine überfälligen Arbeitselemente",
    "loadingRecentItems": "Lade letzte Arbeitselemente...",
    "loadingOverdueItems": "Lade überfällige Arbeitselemente...",
    "workItemStatusOverview": "Arbeitselement-Status Übersicht",
    "createWorkItem": "Arbeitselement erstellen",
    "createWorkItemDesc": "Neuen Arbeitselement erfassen",
    "recentWorkItems": "Letzte Arbeitselemente",
    "noRecentlyViewed": "Keine kürzlich angesehenen Arbeitselemente",
    "noRecentlyEdited": "Keine kürzlich bearbeiteten Arbeitselemente",
    "noRecentlyCommented": "Keine kürzlich kommentierten Arbeitselemente"
  },
  "search": {
    "searchItems": "Arbeitselemente suchen...",
    "configureFilter": "Filter konfigurieren, um Arbeitselemente zu sehen",
    "workspace": "Arbeitsbereich"
  },
  "about": {
    "projectManagementDesc": "Organisieren Sie Arbeitselemente hierarchisch mit benutzerdefinierten Feldern, Workflows und Statusverfolgung."
  },
  "onboarding": {
    "createFirstWorkItem": "Ersten Arbeitselement erstellen",
    "createWorkItemBtn": "Arbeitselement erstellen",
    "getStartedMember": "Hier sind die für Sie verfügbaren Arbeitsbereiche",
    "selectWorkspace": "Wählen Sie einen Arbeitsbereich zum Starten",
    "noWorkspacesAvailable": "Es sind noch keine Arbeitsbereiche verfügbar. Bitte kontaktieren Sie Ihren Administrator, um Zugang zu einem Arbeitsbereich zu erhalten."
  },
  "testing": {
    "linkExistingItem": "Vorhandenen Arbeitselement verknüpfen",
    "searchItemsToLink": "Arbeitselemente zum Verknüpfen suchen...",
    "unknownItem": "Unbekannter Arbeitselement",
    "itemsWithoutTestCoverage": "Arbeitselemente ohne Testabdeckung",
    "selectWorkspacesAndTypes": "Wählen Sie Arbeitsbereiche und Arbeitselementtypen aus, die in den Bericht aufgenommen werden sollen",
    "itemTypes": "Arbeitselementtypen",
    "selectItemTypes": "Arbeitselementtypen auswählen",
    "allItemTypes": "Alle Arbeitselementtypen",
    "selectItemTypesForCoverage": "Wählen Sie Arbeitselementtypen aus, die für die Abdeckungsberichte verfolgt werden sollen",
    "selectItemTypesForCoverageAnalysis": "Wählen Sie Arbeitselementtypen für die Abdeckungsanalyse aus",
    "noItemsMatchingRequirements": "Keine Arbeitselemente entsprechen den konfigurierten Anforderungstypen",
    "noItemTypesAvailable": "Keine Arbeitselementtypen verfügbar"
  },
  "testCase": {
    "backToItem": "Zurück zum Arbeitselement"
  },
  "time": {
    "subtitle": "Zeit für Arbeitselemente erfassen",
    "workItemOptional": "Arbeitselement (Optional)",
    "timer": {
      "goToWorkItem": "Zum Arbeitselement gehen: {title}"
    },
    "calendar": {
      "itemCount": "{count} Arbeitselemente",
      "myWorkItems": "Meine Arbeitselemente",
      "dragToSchedule": "Arbeitselemente ziehen zum Planen",
      "noWorkItems": "Keine Arbeitselemente zugewiesen",
      "workItemsWillAppear": "Arbeitselemente erscheinen hier, wenn sie Ihnen zugewiesen werden"
    }
  },
  "pickers": {
    "noItemsFound": "Keine Arbeitselemente gefunden",
    "noItemsAvailable": "Keine Arbeitselemente verfügbar",
    "fields": {
      "reporter": {
        "description": "Wer den Arbeitselement erstellt hat"
      },
      "createdAt": {
        "description": "Wann das Arbeitselement erstellt wurde"
      },
      "updatedAt": {
        "description": "Wann das Arbeitselement zuletzt aktualisiert wurde"
      },
      "dueDate": {
        "description": "Wann das Arbeitselement fällig ist"
      },
      "parent": {
        "description": "Übergeordneter Arbeitselement"
      },
      "children": {
        "description": "Untergeordnete Arbeitselemente"
      },
      "links": {
        "description": "Verwandte Arbeitselemente"
      },
      "watchers": {
        "description": "Benutzer, die dieses Arbeitselement beobachten"
      }
    },
    "agentOffline": "Agent offline – sein Läuferpool hat keinen Live-Läufer; Zugewiesene Arbeitselemente werden in die Warteschlange gestellt"
  },
  "dialogs": {
    "confirmations": {
      "deleteScreen": "Möchten Sie den Screen \"{name}\" wirklich löschen? Dies betrifft alle Arbeitsbereiche, die diesen Screen verwenden."
    }
  },
  "components": {
    "pagination": {
      "limitedTo": "begrenzt auf {max} Arbeitselemente",
      "itemsPerPage": "Arbeitselemente pro Seite:"
    }
  },
  "layout": {
    "items": "Arbeitselemente"
  },
  "widgets": {
    "chart": {
      "items": "Arbeitselemente"
    },
    "milestoneProgress": {
      "item": "Arbeitselement",
      "items": "Arbeitselemente",
      "noItems": "Keine Arbeitselemente"
    },
    "myTasks": {
      "loadingText": "Ihre Aufgaben werden geladen...",
      "emptyTitle": "Ihnen sind keine Aufgaben zugewiesen",
      "emptySubtitle": "Ihnen zugewiesene Aufgaben erscheinen hier"
    },
    "overdueItems": {
      "itemCount": "{count} überfällige Arbeitselemente",
      "refreshAriaLabel": "Überfällige Arbeitselemente aktualisieren",
      "loadingText": "Überfällige Arbeitselemente werden geladen...",
      "emptyTitle": "Keine überfälligen Arbeitselemente",
      "emptySubtitle": "Alle Arbeitselemente sind im Zeitplan",
      "loadError": "Überfällige Arbeitselemente konnten nicht geladen werden"
    },
    "upcomingDeadlines": {
      "emptySubtitle": "Arbeitselemente mit Fälligkeitsdatum erscheinen hier"
    },
    "recentItems": {
      "loadingText": "Letzte Arbeitselemente werden geladen...",
      "emptyTitle": "Keine aktuellen Arbeitselemente",
      "emptySubtitle": "Kürzlich angesehene Arbeitselemente werden angezeigt hier",
      "loadError": "Letzte Arbeitselemente konnten nicht geladen werden"
    }
  },
  "statusCategory": {
    "marksWorkCompletedHelp": "Arbeitselemente, die in Status dieser Kategorie verschoben werden, werden in Berichten und Rückblicken als abgeschlossen behandelt."
  },
  "items": {
    "title": "Arbeitselemente",
    "subtitle": "Arbeitselemente anzeigen und verwalten",
    "item": "Arbeitselement",
    "items_one": "{count} Arbeitselement",
    "items_other": "{count} Arbeitselemente",
    "createItem": "Arbeitselement erstellen",
    "editItem": "Arbeitselement bearbeiten",
    "deleteItem": "Arbeitselement löschen",
    "viewItem": "Arbeitselement anzeigen",
    "linkedItems": "Verknüpfte Arbeitselemente",
    "noItems": "Keine Arbeitselemente gefunden",
    "noItemsInFilter": "Keine Arbeitselemente entsprechen dem aktuellen Filter",
    "createToStart": "Erstellen Sie ein Arbeitselement, um zu beginnen",
    "itemCreated": "Arbeitselement erfolgreich erstellt",
    "itemUpdated": "Arbeitselement erfolgreich aktualisiert",
    "itemDeleted": "Arbeitselement erfolgreich gelöscht",
    "workItem": "Arbeitselement",
    "startTimerTitle": "Zeiterfassung für dieses Arbeitselement starten",
    "logTimeTitle": "Arbeitszeit für dieses Arbeitselement manuell erfassen",
    "includeChildItems": "Untergeordnete Arbeitselemente einbeziehen",
    "rollupTruncated": "Auf erste {max} untergeordnete Arbeitselemente begrenzt",
    "rollupItemCount_one": "aggregiert über {count} Arbeitselement",
    "rollupItemCount_other": "aggregiert über {count} Arbeitselemente",
    "workItemNotFound": "Arbeitselement nicht gefunden",
    "itemCopiedAs": "Arbeitselement erfolgreich als {key} kopiert",
    "clickToViewCopied": "Klicken, um kopierten Arbeitselement anzuzeigen",
    "deleteWorkItem": "Arbeitselement löschen",
    "deleteItemWithChildren": "Arbeitselement mit untergeordneten Einträgen löschen",
    "itemHasChildren": "Dieser Arbeitselement hat {count} untergeordnete Arbeitselemente.",
    "itemHasChildrenSingular": "Dieser Arbeitselement hat 1 untergeordneten Arbeitselement.",
    "deleteAllOption": "Alle löschen ({count} Arbeitselemente)",
    "deleteAllDescription": "Diesen Arbeitselement und alle untergeordneten Arbeitselemente dauerhaft löschen",
    "reparentDescription": "Untergeordnete zum übergeordneten Arbeitselement verschieben, dann nur dieses Arbeitselement löschen",
    "deleteAllItems": "Alle Arbeitselemente löschen",
    "deletedItemsCount": "{count} Arbeitselemente gelöscht",
    "reparentedAndDeleted": "Untergeordnete verschoben und Arbeitselement gelöscht",
    "selectNewParentPlaceholder": "Übergeordneten Arbeitselement auswählen...",
    "reparentLevelHint": "Zeigt nur Arbeitselemente auf derselben Hierarchieebene",
    "noOtherItemsAtLevel": "Keine anderen Arbeitselemente auf dieser Ebene - wählen Sie \"Zu Stammeinträgen machen\" oder wählen Sie aus der Liste oben",
    "copyWorkItem": "Arbeitselement kopieren",
    "unwatchWorkItem": "Arbeitselement nicht mehr beobachten",
    "watchWorkItem": "Arbeitselement beobachten",
    "cannotCreateChildItems": "Für diese Hierarchieebene können keine untergeordneten Arbeitselemente erstellt werden.",
    "workItems": "Arbeitselemente",
    "goToLinkedWorkItem": "Zum verknüpften Arbeitselement gehen",
    "searchForParentItem": "Nach übergeordnetem Arbeitselement suchen...",
    "showingItemsFromLevel": "Zeige nur Arbeitselemente der Hierarchieebene {level}",
    "searchParentAcrossWorkspaces": "Nach übergeordnetem Arbeitselement in allen Arbeitsbereichen suchen",
    "noItemsAtLevel": "Keine Arbeitselemente auf Hierarchieebene {level} gefunden",
    "searchWorkItems": "Arbeitselemente suchen...",
    "linkToTestCase": "Diesen Arbeitselement mit einem Testfall verknüpfen.",
    "childWorkItems": "Untergeordnete Arbeitselemente",
    "loadingChildItems": "Untergeordnete Arbeitselemente werden geladen...",
    "agentLogEmpty": "Für dieses Arbeitselement wird noch kein Agent ausgeführt",
    "agentRerunTitle": "Führen Sie den Agenten für dieses Arbeitselement erneut aus.",
    "rollupItemCount": "zusammengefasst über {count} Arbeitselemente",
    "linkToPage": "Verknüpfen Sie dieses Arbeitselement mit einer Wissensseite im selben Arbeitsbereich."
  },
  "comments": {
    "beFirstToComment": "Seien Sie der Erste, der dieses Arbeitselement kommentiert."
  },
  "todo": {
    "assignedItemsWillAppear": "Ihnen zugewiesene Arbeitselemente werden hier angezeigt."
  },
  "collectionTree": {
    "noWorkItemsYet": "Noch keine Arbeitselemente",
    "createFirstWorkItem": "Erstellen Sie Ihren ersten Arbeitselement, um den Hierarchiebaum zu sehen."
  },
  "collections": {
    "searchItems": "Arbeitselemente suchen...",
    "searchItemsTitle": "Arbeitselemente suchen",
    "noItemsInBacklog": "Keine Arbeitselemente im Backlog",
    "noItemsInBacklogDesc": "Alle Arbeitselemente sind entweder abgeschlossen oder es existieren noch keine Arbeitselemente.",
    "showingItemsFromBacklog": "{count} Arbeitselemente aus dem Backlog anzeigen",
    "childItems": "{count} untergeordnete(r) Arbeitselement/Arbeitselemente",
    "childWorkItems": "Untergeordnete Arbeitselemente ({count})",
    "noChildItems": "Noch keine untergeordneten Arbeitselemente",
    "noChildItemsLowest": "Keine untergeordneten Arbeitselemente (niedrigste Hierarchieebene)",
    "noTopLevelItems": "Keine Arbeitselemente auf oberster Ebene gefunden",
    "noTopLevelItemsDesc": "Erstellen Sie einige Arbeitselemente, um Ihre Story Map zu sehen",
    "loadingWorkItems": "Arbeitselemente werden geladen...",
    "noWorkItemsFound": "Keine Arbeitselemente gefunden",
    "showingWorkItems": "{count} Arbeitselemente anzeigen",
    "boardSummary": "Gesamt: {itemCount} Arbeitselemente in {columnCount} Spalten",
    "roadmapNoItems": "Keine Arbeitselemente mit Daten im aktuellen Zeitraum.",
    "dragItemsHere": "Arbeitselemente hierher ziehen, um sie diesem Sprint hinzuzufügen",
    "allItems": "Alle Arbeitselemente"
  },
  "links": {
    "subtitle": "Arbeitselement-Links verwalten"
  },
  "workspaceSettings": {
    "workspaceKeyHelp": "Wird für Arbeitselement-Präfixe verwendet (z.B. DEV-123). Nur Großbuchstaben und Zahlen.",
    "defaultTimeProjectHelp": "Standardprojekt für die Zeiterfassung bei Vorgängen in diesem Arbeitsbereich. Kann pro Arbeitselement überschrieben werden.",
    "removeWarningItems": "Alle Arbeitselemente und Projekte in diesem Arbeitsbereich",
    "headers": {
      "recurrence": "Regeln für wiederkehrende Arbeitselemente verwalten",
      "templates": "Wiederverwendbare Beschreibungsgerüste für neue Arbeitselemente"
    }
  },
  "issueSync": {
    "subtitle": "GitHub Issues als Arbeitselemente in diesen Arbeitsbereich synchronisieren",
    "noConfig": "Die Issue-Synchronisierung ist für diesen Arbeitsbereich nicht konfiguriert.",
    "noConfigDescription": "Verknüpfen Sie ein Repository und konfigurieren Sie, wie GitHub Issues in Windshift-Arbeitselemente synchronisiert werden sollen.",
    "enabledDescription": "Wenn aktiviert, werden GitHub Issues regelmäßig in diesen Arbeitsbereich synchronisiert.",
    "itemType": "Arbeitselementtyp",
    "itemTypeDescription": "Wählen Sie den Arbeitselementtyp für synchronisierte Issues. Dies bestimmt, welche Workflow-Status für die Zuordnung verfügbar sind.",
    "selectItemType": "Arbeitselementtyp auswählen",
    "selectItemTypeHint": "Wählen Sie oben einen Arbeitselementtyp, um die Status-Zuordnung zu konfigurieren.",
    "syncedItems": "Synchronisierte Arbeitselemente",
    "noLinkedRepos": "Keine Repositories mit diesem Arbeitsbereich verknüpft. Verknüpfen Sie zuerst ein Repository in den Quellcodeverwaltungseinstellungen.",
    "confirmDelete": "Möchten Sie die Issue-Synchronisierungskonfiguration wirklich entfernen? Synchronisierte Arbeitselemente bleiben erhalten, werden aber nicht mehr aktualisiert."
  },
  "actions": {
    "noActionsDescription": "Erstellen Sie Aktionen, um Ihre Workflows basierend auf Arbeitselement-Ereignissen zu automatisieren",
    "trigger": {
      "itemCreated": "Arbeitselement erstellt",
      "itemUpdated": "Arbeitselement aktualisiert",
      "itemLinked": "Arbeitselement verknüpft"
    },
    "nodes": {
      "relatedItems": "Für jedes verwandte Arbeitselement",
      "transitionItem": "Arbeitselement überführen"
    },
    "config": {
      "includeLink": "Link zum Arbeitselement einfügen",
      "sourceAssetField": "Asset-Feld am Arbeitselement",
      "sourceTypeItemField": "Arbeitselement-Feld"
    },
    "test": {
      "description": "Wählen Sie ein Arbeitselement aus, für das diese Aktion ausgeführt werden soll. Die Aktion wird sofort ausgeführt und umgeht den normalen Auslöser.",
      "selectItem": "Arbeitselement auswählen",
      "itemPlaceholder": "Nach einem Arbeitselement suchen..."
    },
    "placeholders": {
      "categories": {
        "item": "Arbeitselement-Felder"
      },
      "item": {
        "title": "Arbeitselement-Titel",
        "id": "Arbeitselement-ID",
        "any": "Beliebiges Arbeitselement-Feld"
      },
      "trigger": {
        "itemId": "Auslösendes Arbeitselement-ID"
      }
    }
  },
  "analytics": {
    "collectionLoadError": "Sammlungen konnten nicht geladen werden. Die Analyse zeigt alle Arbeitsbereich-Arbeitselemente.",
    "allItems": "Alle Arbeitsbereich-Arbeitselemente",
    "items_one": "{count} Arbeitselement",
    "items_other": "{count} Arbeitselemente",
    "scope": {
      "summary": "{items} aktuelle Arbeitselemente · {from}–{to}",
      "currentWorkspace": "Aktuelle Arbeitsbereich-Kohorte",
      "currentWorkspaceNote": "Der Zeitraum gilt für Fluss- und Lieferdiagramme; Zustand und Alter sind aktuelle Momentaufnahmen. Historische Diagramme verwenden Arbeitselemente, die heute in diesem Arbeitsbereich liegen. Verschobene oder gelöschte Arbeitselemente sind nicht enthalten.",
      "currentCollectionNote": "Der Zeitraum gilt für Fluss- und Lieferdiagramme; Zustand und Alter sind aktuelle Momentaufnahmen. Historische Diagramme verwenden Arbeitselemente, die heute zur Sammlung passen. Änderungen an der Sammlung können die Kohorte verändern."
    },
    "health": {
      "attentionItems": "Zu prüfende Arbeitselemente",
      "item": "Arbeitselement",
      "allClear": "Keine unerledigten Arbeitselemente entsprechen derzeit einem Warnsignal."
    },
    "aging": {
      "description": "Wie lange aktuell unerledigte Arbeitselemente bereits offen sind.",
      "total": "Aktive Arbeitselemente",
      "itemCount": "Arbeitselemente",
      "oldest": "Älteste unerledigte Arbeitselemente"
    },
    "deliveryTime": {
      "analyzed": "Abgeschlossene Arbeitselemente",
      "missingHistory_one": "1 aktuell abgeschlossenes Arbeitselement wurde wegen fehlender Abschlusshistorie ausgeschlossen.",
      "missingHistory_other": "{count} aktuell abgeschlossene Arbeitselemente wurden wegen fehlender Abschlusshistorie ausgeschlossen.",
      "missingHistory": "{count} Derzeit abgeschlossene Arbeitselemente wurden ausgeschlossen, da ihr Abschlussverlauf fehlt."
    },
    "insufficientData": {
      "no_items": "Dieser Bereich enthält noch keine Arbeitselemente.",
      "few_completed_items": "In diesem Zeitraum wurden nur wenige Arbeitselemente abgeschlossen. Perzentile sind daher nur als Tendenz zu verstehen."
    }
  },
  "recurrence": {
    "emptyDesc": "Hier werden Wiederholungsregeln angezeigt wenn für Arbeitselemente wiederkehrende Zeitpläne konfiguriert sind.",
    "createFromItem": "Um eine Wiederholungsregel zu erstellen, öffnen Sie ein Arbeitselement und richten Sie die Wiederholung in der Detailseitenleiste ein."
  },
  "conditionSets": {
    "errorMessagePlaceholder": "z. B. „Nur Tester können Arbeitselemente zur Qualitätssicherung verschieben“"
  },
  "approvalSets": {
    "allowSelfApproval": "Selbstgenehmigung zulassen – Erlaubt demselben Benutzer, das Arbeitselement sowohl zu übertragen als auch zu genehmigen."
  }
};

