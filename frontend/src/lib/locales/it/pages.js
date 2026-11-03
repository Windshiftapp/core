/**
 * Italian (it) - Knowledge pages (wiki) translations
 */
export default {
  pages: {
    // Sidebar / drilldown nav
    backWorkspace: 'Workspace',
    treeHeading: 'Pagine',
    addPageAria: 'Aggiungi pagina',
    untitled: 'Senza titolo',
    treeLoading: 'Caricamento…',
    treeEmptyTitle: 'Ancora nessuna pagina',
    treeEmptyDescription: 'Usa il pulsante + qui sopra per creare la tua prima pagina.',

    // Per-item kebab menu
    menuAddChild: 'Aggiungi sottopagina',
    menuRename: 'Rinomina',
    menuMove: 'Sposta',
    menuPermissions: 'Permessi',
    menuHistory: 'Cronologia',
    menuPrint: 'Stampa',
    menuArchive: 'Archivia',

    // Chrome-free print / save-to-PDF view (opened in a new tab)
    print: {
      button: 'Stampa',
      back: 'Torna alla pagina',
      loading: 'Preparazione della pagina per la stampa…',
      error: 'Impossibile caricare la pagina.',
    },

    // Revision history drawer
    history: {
      title: 'Cronologia delle revisioni',
      empty: 'Ancora nessuna revisione.',
      loadError: 'Impossibile caricare la cronologia delle revisioni.',
      restoreTitle: 'Ripristinare la revisione #{rev}?',
      restoreMessage:
        'Il corpo attuale della pagina verrà sostituito con il contenuto della revisione selezionata e verrà creata una nuova revisione che registra il ripristino.',
      restoreConfirm: 'Ripristina',
      restoreAction: 'Ripristina #{rev}',
      restoring: 'Ripristino…',
      restoredOK: 'Revisione #{rev} ripristinata.',
      restoreError: 'Impossibile ripristinare la revisione.',
    },

    // Page pane
    pageLoading: 'Caricamento della pagina…',
    emptyPaneTitle: 'Pagine della knowledge base',
    emptyPaneDescription: 'Seleziona una pagina dall’albero oppure creane una per iniziare.',
    titlePlaceholder: 'Senza titolo',
    editorPlaceholder: 'Inizia a scrivere…',
    tocHeading: 'In questa pagina',
    tocAriaLabel: 'Indice',

    // Action buttons on the open page
    save: 'Salva',
    move: 'Sposta',
    permissions: 'Permessi',
    archive: 'Archivia',

    // Auto-save status indicator next to the toolbar kebab
    statusSaving: 'Salvataggio…',
    statusSaved: 'Salvato',
    statusUnsaved: 'Non salvato',
    statusError: 'Salvataggio non riuscito',

    // Public knowledge-base publication banner (green bar above the page)
    publiclyViewableBanner:
      'Visibile pubblicamente: questa pagina è pubblicata tramite la knowledge base di un portale e può essere letta dagli utenti del portale.',
    publiclyViewableBannerWithPortals:
      'Visibile pubblicamente: questa pagina è pubblicata tramite la knowledge base di {portals} e può essere letta dagli utenti del portale.',

    // Segmented Edit / Read mode toggle
    modeEdit: 'Modifica',
    modeRead: 'Lettura',
    modeAria: 'Modalità di visualizzazione',
    canvasWide: 'Usa l’area a larghezza piena',
    canvasComfortable: 'Usa l’area a larghezza di lettura',

    // Error fallbacks
    errorLoadTree: 'Impossibile caricare le pagine',
    errorLoadPage: 'Impossibile caricare la pagina',
    errorSave: 'Salvataggio non riuscito',
    errorCreate: 'Impossibile creare la pagina',
    errorArchive: 'Impossibile archiviare',

    // Discard / archive confirms
    discardTitle: 'Scartare le modifiche non salvate?',
    discardMessage:
      'Ci sono modifiche non salvate nella pagina corrente. Andranno perse se cambi pagina.',
    discardConfirm: 'Scarta',
    discardCancel: 'Continua a modificare',
    archiveTitle: 'Archiviare "{title}"?',
    archiveMessage:
      'Verranno archiviate la pagina e tutte le sue sottopagine. L’azione non può essere annullata.',
    archiveConfirm: 'Archivia',

    // Archived pages list
    archivedOpenAria: 'Visualizza pagine archiviate',
    archivedHeading: 'Pagine archiviate',
    archivedSubtitle: 'Rivedi e ripristina le pagine archiviate in precedenza.',
    archivedBack: 'Torna alle pagine',
    archivedEmpty: 'Nessuna pagina archiviata',
    archivedColTitle: 'Titolo',
    archivedColArchivedAt: 'Archiviata',
    archivedColArchivedBy: 'Archiviata da',
    archivedUnarchive: 'Ripristina',
    archivedUnarchiveTitle: 'Ripristinare "{title}"?',
    archivedUnarchiveMessage:
      'La pagina ricompare nell’albero. Se la pagina padre è ancora archiviata, la pagina resta nascosta finché non ripristini anche la pagina padre.',
    archivedUnarchiveConfirm: 'Ripristina',
    archivedUnarchiveOK: '"{title}" ripristinata',
    archivedLoadError: 'Impossibile caricare le pagine archiviate',
    archivedUnarchiveError: 'Impossibile ripristinare la pagina',

    // Move dialog
    moveTitle: 'Sposta "{title}"',
    moveSubtitle:
      'Scegli una nuova pagina padre. Le pagine sotto la pagina corrente sono nascoste perché creerebbero un ciclo.',
    moveWorkspaceLabel: 'Workspace di destinazione',
    moveWorkspacePlaceholder: 'Seleziona un workspace…',
    moveParentLabel: 'Pagina padre',
    moveSearchPlaceholder: 'Cerca pagine…',
    moveRoot: 'Radice del workspace',
    moveCrossWorkspaceSummary: 'Pagine in questo sottoalbero: {count}.',
    moveCrossWorkspacePolicy:
      'Le etichette corrispondenti vengono mantenute. Permessi espliciti, collegamenti agli elementi di lavoro e riferimenti alle skill degli agenti vengono rimossi.',
    moveButton: 'Sposta',
    moveCancel: 'Annulla',
    errorLoadWorkspaces: 'Impossibile caricare i workspace',
    errorMove: 'Spostamento non riuscito',

    // Permissions dialog
    permsTitle: 'Permessi della pagina',
    permsEffectiveAccess: 'Il tuo accesso effettivo: {level}',
    permsEffectiveAccessNone: 'nessuno',
    permsLoading: 'Caricamento…',
    permsInheritLabel: 'Eredita i permessi dalle pagine superiori',
    permsInheritHint:
      'Quando l’ereditarietà è attiva e non esistono autorizzazioni esplicite, decidono i permessi dei ruoli del workspace. Interrompere l’ereditarietà senza autorizzazioni limita la pagina agli amministratori.',
    permsExplicitGrants: 'Autorizzazioni esplicite',
    permsEmptyGrantsTitle: 'Nessuna autorizzazione esplicita su questa pagina.',
    permsEmptyGrantsDescription: 'L’ereditarietà e i ruoli del workspace continuano ad applicarsi.',
    permsColumnPrincipal: 'Soggetto',
    permsColumnLevel: 'Livello',
    permsRemove: 'Rimuovi',
    permsRemoveTitle: 'Rimuovere il permesso?',
    permsRemoveMessage: 'Questa autorizzazione verrà rimossa dalla pagina. Potrai aggiungerla di nuovo in seguito.',
    permsRemoveConfirm: 'Rimuovi',
    permsRemoveCancel: 'Annulla',
    permsClose: 'Chiudi',
    permsAdd: 'Aggiungi',
    permsPrincipalUser: 'Utente',
    permsPrincipalGroup: 'Gruppo',
    permsPrincipalRole: 'Ruolo',
    permsLevelView: 'Visualizzazione',
    permsLevelEdit: 'Modifica',
    permsLevelAdmin: 'Amministrazione',
    permsPickUser: 'Scegli un utente',
    permsPickGroup: 'Scegli un gruppo',
    permsPickRole: 'Scegli un ruolo',
    permsErrorNoPrincipal: 'Scegli un soggetto prima di aggiungere l’autorizzazione',
    permsErrorLoad: 'Impossibile caricare i permessi',
    permsErrorInherit: 'Impossibile aggiornare l’ereditarietà',
    permsErrorGrant: 'Impossibile aggiungere il permesso',
    permsErrorRevoke: 'Impossibile revocare',

    // Page labels
    labelsTitle: 'Etichette',
    labelsAdd: 'Aggiungi etichetta',
    labelsCreate: 'Crea etichetta',
    labelsCreateNamed: 'Crea "{name}"',
    labelsSearchPlaceholder: 'Cerca o crea…',
    labelsFilterTitle: 'Filtra per etichetta',
    labelsFilterPlaceholder: 'Filtra le pagine per etichetta',
    labelsFilterClear: 'Cancella filtro',
    labelsEmpty: 'Ancora nessuna etichetta',
    labelsNameRequired: 'Il nome dell’etichetta è obbligatorio',
    labelsDuplicate: 'Esiste già un’etichetta con questo nome',
    labelsDeleteConfirm: 'Eliminare questa etichetta? Verrà rimossa da tutte le pagine a cui è applicata.',
    labelsDelete: 'Elimina etichetta',
    labelsRemoveAria: 'Rimuovi etichetta {name}',
    labelsErrorLoad: 'Impossibile caricare le etichette',
    labelsErrorSave: 'Impossibile salvare l’etichetta',
    labelsErrorAttach: 'Impossibile applicare l’etichetta',
    labelsErrorDetach: 'Impossibile rimuovere l’etichetta',

    // Sidebar title search
    searchAria: 'Cerca pagine',
    searchPlaceholder: 'Cerca pagine…',
    searchClear: 'Cancella ricerca',

    // Sidebar tree expand/collapse
    toggleSubtreeAria: 'Espandi/comprimi le sottopagine di {title}',
    expandAllAria: 'Espandi tutto',
    collapseAllAria: 'Comprimi tutto',

    // Linked work items
    workItemsButton: 'Elementi di lavoro',
    workItemsAria: 'Mostra elementi di lavoro collegati',
    workItemsEmpty: 'Ancora nessun elemento di lavoro collegato qui',
    workItemsLoading: 'Caricamento degli elementi di lavoro…',
    workItemsTitle: 'Elementi di lavoro',
    addWorkItem: 'Aggiungi elemento di lavoro',
    addWorkItemCancel: 'Annulla',
    addWorkItemSearchPlaceholder: 'Cerca elementi di lavoro…',
    removeWorkItemLink: 'Scollega elemento di lavoro',
    workItemsErrorLoad: 'Impossibile caricare gli elementi di lavoro collegati',
    workItemsErrorLink: 'Impossibile collegare l’elemento di lavoro',
    workItemsErrorUnlink: 'Impossibile scollegare l’elemento di lavoro',
  },
};
