/**
 * Dutch (nl) - Knowledge pages (wiki) translations
 *
 * Surfaces:
 *   - PagesView (left tree + editor pane)
 *   - PageMoveDialog
 *   - PagePermissionsDialog
 */
export default {
  pages: {
    // Sidebar / drilldown nav
    backWorkspace: 'Werkruimte',
    treeHeading: "Pagina's",
    addPageAria: 'Pagina toevoegen',
    untitled: 'Naamloos',
    treeLoading: 'Laden…',
    treeEmptyTitle: "Nog geen pagina's",
    treeEmptyDescription: 'Gebruik de +-knop hierboven om je eerste pagina aan te maken.',

    // Per-item kebab menu
    menuAddChild: 'Subpagina toevoegen',
    menuRename: 'Hernoemen',
    menuMove: 'Verplaatsen',
    menuPermissions: 'Machtigingen',
    menuHistory: 'Geschiedenis',
    menuPrint: 'Afdrukken',
    menuArchive: 'Archiveren',

    // Chrome-free print / save-to-PDF view (opened in a new tab)
    print: {
      button: 'Afdrukken',
      back: 'Terug naar pagina',
      loading: 'Pagina voorbereiden voor afdrukken…',
      error: 'Pagina kon niet worden geladen.',
    },

    // Revision history drawer
    history: {
      title: 'Revisiegeschiedenis',
      empty: 'Nog geen revisies.',
      loadError: 'Revisiegeschiedenis kon niet worden geladen.',
      restoreTitle: 'Revisie #{rev} herstellen?',
      restoreMessage:
        'Hiermee wordt de huidige paginatekst vervangen door de inhoud van de geselecteerde revisie en wordt een nieuwe revisie aangemaakt die de herstelling vastlegt.',
      restoreConfirm: 'Herstellen',
      restoreAction: '#{rev} herstellen',
      restoring: 'Herstellen…',
      restoredOK: 'Revisie #{rev} hersteld.',
      restoreError: 'Revisie kon niet worden hersteld.',
    },

    // Page pane
    pageLoading: 'Pagina laden…',
    emptyPaneTitle: "Kennisbankpagina's",
    emptyPaneDescription: 'Selecteer een pagina in de boomstructuur of maak er een aan om te beginnen.',
    titlePlaceholder: 'Naamloos',
    editorPlaceholder: 'Begin met schrijven…',
    tocHeading: 'Op deze pagina',
    tocAriaLabel: 'Inhoudsopgave',

    // Action buttons on the open page
    save: 'Opslaan',
    move: 'Verplaatsen',
    permissions: 'Machtigingen',
    archive: 'Archiveren',

    // Auto-save status indicator next to the toolbar kebab
    statusSaving: 'Opslaan…',
    statusSaved: 'Opgeslagen',
    statusUnsaved: 'Niet opgeslagen',
    statusError: 'Opslaan mislukt',

    // Public knowledge-base publication banner (green bar above the page)
    publiclyViewableBanner:
      'Openbaar zichtbaar — deze pagina is gepubliceerd via een portaal-kennisbank en kan door portaalgebruikers worden gelezen.',
    publiclyViewableBannerWithPortals:
      'Openbaar zichtbaar — deze pagina is gepubliceerd via de kennisbank van {portals} en kan door portaalgebruikers worden gelezen.',

    // Segmented Edit / Read mode toggle
    modeEdit: 'Bewerken',
    modeRead: 'Lezen',
    modeAria: 'Weergavemodus',
    canvasWide: 'Canvas op volledige breedte gebruiken',
    canvasComfortable: 'Canvas op leesbare breedte gebruiken',

    // Error fallbacks
    errorLoadTree: "Pagina's konden niet worden geladen",
    errorLoadPage: 'Pagina kon niet worden geladen',
    errorSave: 'Opslaan mislukt',
    errorCreate: 'Pagina kon niet worden aangemaakt',
    errorArchive: 'Archiveren mislukt',

    // Discard / archive confirms
    discardTitle: 'Niet-opgeslagen wijzigingen verwerpen?',
    discardMessage:
      'Je hebt niet-opgeslagen wijzigingen op de huidige pagina. Deze gaan verloren als je wisselt.',
    discardConfirm: 'Verwerpen',
    discardCancel: 'Doorgaan met bewerken',
    archiveTitle: '“{title}” archiveren?',
    archiveMessage:
      "Hiermee worden de pagina en alle subpagina's gearchiveerd. Deze actie kan niet ongedaan worden gemaakt.",
    archiveConfirm: 'Archiveren',

    // Archived pages list (admin-only full-page view opened from the sidebar header)
    archivedOpenAria: "Gearchiveerde pagina's bekijken",
    archivedHeading: "Gearchiveerde pagina's",
    archivedSubtitle: "Bekijk en herstel pagina's die eerder zijn gearchiveerd.",
    archivedBack: "Terug naar pagina's",
    archivedEmpty: "Geen gearchiveerde pagina's",
    archivedColTitle: 'Titel',
    archivedColArchivedAt: 'Gearchiveerd',
    archivedColArchivedBy: 'Gearchiveerd door',
    archivedUnarchive: 'Dearchiveren',
    archivedUnarchiveTitle: '“{title}” dearchiveren?',
    archivedUnarchiveMessage:
      'De pagina verschijnt weer in de boomstructuur. Als de bovenliggende pagina nog gearchiveerd is, blijft de pagina verborgen tot je ook de bovenliggende pagina dearchiveert.',
    archivedUnarchiveConfirm: 'Dearchiveren',
    archivedUnarchiveOK: '“{title}” hersteld',
    archivedLoadError: "Gearchiveerde pagina's konden niet worden geladen",
    archivedUnarchiveError: 'Pagina kon niet worden gedearchiveerd',

    // Move dialog
    moveTitle: '“{title}” verplaatsen',
    moveSubtitle:
      "Kies een nieuwe bovenliggende pagina. Pagina's onder de huidige pagina zijn verborgen omdat ze een cyclus zouden vormen.",
    moveWorkspaceLabel: 'Doelwerkruimte',
    moveWorkspacePlaceholder: 'Selecteer een werkruimte…',
    moveParentLabel: 'Bovenliggende pagina',
    moveSearchPlaceholder: "Pagina's zoeken…",
    moveRoot: 'Wortel van werkruimte',
    moveCrossWorkspaceSummary: "Pagina's in deze subboom: {count}.",
    moveCrossWorkspacePolicy:
      'Overeenkomende labels blijven behouden. Expliciete machtigingen, koppelingen naar werkitems en verwijzingen naar agentvaardigheden worden verwijderd.',
    moveButton: 'Verplaatsen',
    moveCancel: 'Annuleren',
    errorLoadWorkspaces: 'Werkruimten konden niet worden geladen',
    errorMove: 'Verplaatsen mislukt',

    // Permissions dialog
    permsTitle: 'Paginamachtigingen',
    permsEffectiveAccess: 'Je effectieve toegang: {level}',
    permsEffectiveAccessNone: 'geen',
    permsLoading: 'Laden…',
    permsInheritLabel: "Machtigingen overnemen van bovenliggende pagina's",
    permsInheritHint:
      'Als overerving aanstaat en er geen expliciete rechten zijn, bepalen de rolmachtigingen van de werkruimte. Het uitschakelen van overerving zonder rechten beperkt de pagina tot beheerders.',
    permsExplicitGrants: 'Expliciete rechten',
    permsEmptyGrantsTitle: 'Geen expliciete rechten op deze pagina.',
    permsEmptyGrantsDescription: 'Overerving en werkruimterollen zijn nog steeds van toepassing.',
    permsColumnPrincipal: 'Hoofdrol',
    permsColumnLevel: 'Niveau',
    permsRemove: 'Verwijderen',
    permsRemoveTitle: 'Machtiging verwijderen?',
    permsRemoveMessage: 'Dit recht wordt van de pagina verwijderd. Je kunt het later opnieuw toevoegen.',
    permsRemoveConfirm: 'Verwijderen',
    permsRemoveCancel: 'Annuleren',
    permsClose: 'Sluiten',
    permsAdd: 'Toevoegen',
    permsPrincipalUser: 'Gebruiker',
    permsPrincipalGroup: 'Groep',
    permsPrincipalRole: 'Rol',
    permsLevelView: 'Bekijken',
    permsLevelEdit: 'Bewerken',
    permsLevelAdmin: 'Beheer',
    permsPickUser: 'Kies een gebruiker',
    permsPickGroup: 'Kies een groep',
    permsPickRole: 'Kies een rol',
    permsErrorNoPrincipal: 'Kies een hoofdrol voordat je het recht toevoegt',
    permsErrorLoad: 'Machtigingen konden niet worden geladen',
    permsErrorInherit: 'Overerving kon niet worden bijgewerkt',
    permsErrorGrant: 'Machtiging kon niet worden toegevoegd',
    permsErrorRevoke: 'Intrekken mislukt',

    // Page labels (workspace-scoped, attach to pages only)
    labelsTitle: 'Labels',
    labelsAdd: 'Label toevoegen',
    labelsCreate: 'Label aanmaken',
    labelsCreateNamed: '“{name}” aanmaken',
    labelsSearchPlaceholder: 'Zoeken of aanmaken…',
    labelsFilterTitle: 'Filteren op label',
    labelsFilterPlaceholder: "Pagina's filteren op label",
    labelsFilterClear: 'Filter wissen',
    labelsEmpty: 'Nog geen labels',
    labelsNameRequired: 'Labelnaam is verplicht',
    labelsDuplicate: 'Er bestaat al een label met die naam',
    labelsDeleteConfirm: 'Dit label verwijderen? Het wordt van elke pagina verwijderd waaraan het is gekoppeld.',
    labelsDelete: 'Label verwijderen',
    labelsRemoveAria: 'Label {name} verwijderen',
    labelsErrorLoad: 'Labels konden niet worden geladen',
    labelsErrorSave: 'Label kon niet worden opgeslagen',
    labelsErrorAttach: 'Label kon niet worden gekoppeld',
    labelsErrorDetach: 'Label kon niet worden ontkoppeld',

    // Sidebar title search
    searchAria: "Pagina's zoeken",
    searchPlaceholder: "Pagina's zoeken…",
    searchClear: 'Zoekopdracht wissen',

    // Sidebar tree expand/collapse
    toggleSubtreeAria: "Onderliggende pagina's van {title} in-/uitklappen",
    expandAllAria: 'Alles uitklappen',
    collapseAllAria: 'Alles inklappen',

    // Linked work items button + popover (top-right on page detail)
    workItemsButton: 'Werkitems',
    workItemsAria: 'Gekoppelde werkitems tonen',
    workItemsEmpty: 'Er zijn nog geen werkitems hier gekoppeld',
    workItemsLoading: 'Werkitems laden…',
    workItemsTitle: 'Werkitems',
    addWorkItem: 'Werkitem toevoegen',
    addWorkItemCancel: 'Annuleren',
    addWorkItemSearchPlaceholder: 'Werkitems zoeken…',
    removeWorkItemLink: 'Werkitem ontkoppelen',
    workItemsErrorLoad: 'Gekoppelde werkitems konden niet worden geladen',
    workItemsErrorLink: 'Werkitem kon niet worden gekoppeld',
    workItemsErrorUnlink: 'Werkitem kon niet worden ontkoppeld',
  },
};
