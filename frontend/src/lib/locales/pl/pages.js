/**
 * Polish (pl) - Tłumaczenia stron wiedzy (wiki)
 *
 * Powierzchnie:
 *   - PagesView (drzewo po lewej + panel edytora)
 *   - PageMoveDialog
 *   - PagePermissionsDialog
 */
export default {
  pages: {
    // Nawigacja boczna / przechodzenie w głąb
    backWorkspace: 'Przestrzeń robocza',
    treeHeading: 'Strony',
    addPageAria: 'Dodaj stronę',
    untitled: 'Bez tytułu',
    treeLoading: 'Ładowanie…',
    treeEmptyTitle: 'Brak stron',
    treeEmptyDescription: 'Użyj przycisku + powyżej, aby utworzyć pierwszą stronę.',

    // Menu kontekstowe elementu
    menuAddChild: 'Dodaj stronę podrzędną',
    menuRename: 'Zmień nazwę',
    menuMove: 'Przenieś',
    menuPermissions: 'Uprawnienia',
    menuHistory: 'Historia',
    menuPrint: 'Drukuj',
    menuArchive: 'Archiwizuj',

    // Widok do druku / zapisu do PDF (otwierany w nowej karcie)
    print: {
      button: 'Drukuj',
      back: 'Powrót do strony',
      loading: 'Przygotowywanie strony do druku…',
      error: 'Nie udało się załadować strony.',
    },

    // Panel historii wersji
    history: {
      title: 'Historia wersji',
      empty: 'Brak wersji.',
      loadError: 'Nie udało się załadować historii wersji.',
      restoreTitle: 'Przywrócić wersję #{rev}?',
      restoreMessage:
        'Spowoduje to zastąpienie bieżącej treści strony treścią wybranej wersji i utworzenie nowej wersji rejestrującej przywrócenie.',
      restoreConfirm: 'Przywróć',
      restoreAction: 'Przywróć #{rev}',
      restoring: 'Przywracanie…',
      restoredOK: 'Przywrócono wersję #{rev}.',
      restoreError: 'Nie udało się przywrócić wersji.',
    },

    // Panel strony
    pageLoading: 'Ładowanie strony…',
    emptyPaneTitle: 'Strony wiedzy',
    emptyPaneDescription: 'Wybierz stronę z drzewa lub utwórz nową, aby rozpocząć.',
    titlePlaceholder: 'Bez tytułu',
    editorPlaceholder: 'Zacznij pisać…',
    tocHeading: 'Na tej stronie',
    tocAriaLabel: 'Spis treści',

    // Przyciski działań na otwartej stronie
    save: 'Zapisz',
    move: 'Przenieś',
    permissions: 'Uprawnienia',
    archive: 'Archiwizuj',

    // Wskaźnik automatycznego zapisu obok menu narzędzi
    statusSaving: 'Zapisywanie…',
    statusSaved: 'Zapisano',
    statusUnsaved: 'Niezapisane',
    statusError: 'Zapisywanie nie powiodło się',

    // Baner publikacji publicznej bazy wiedzy (zielony pasek nad stroną)
    publiclyViewableBanner:
      'Publicznie widoczna — ta strona jest publikowana przez bazę wiedzy portalu i mogą ją czytać użytkownicy portalu.',
    publiclyViewableBannerWithPortals:
      'Publicznie widoczna — ta strona jest publikowana przez bazę wiedzy {portals} i mogą ją czytać użytkownicy portalu.',

    // Przełącznik trybu Edycja / Czytanie
    modeEdit: 'Edytuj',
    modeRead: 'Czytaj',
    modeAria: 'Tryb widoku',
    canvasWide: 'Użyj szerokości pełnej',
    canvasComfortable: 'Użyj szerokości do czytania',

    // Komunikaty błędów
    errorLoadTree: 'Nie udało się załadować stron',
    errorLoadPage: 'Nie udało się załadować strony',
    errorSave: 'Nie udało się zapisać',
    errorCreate: 'Nie udało się utworzyć strony',
    errorArchive: 'Nie udało się zarchiwizować',

    // Potwierdzenia odrzucenia / archiwizacji
    discardTitle: 'Odrzucić niezapisane zmiany?',
    discardMessage:
      'Masz niezapisane zmiany na bieżącej stronie. Zostaną utracone, jeśli przełączysz stronę.',
    discardConfirm: 'Odrzuć',
    discardCancel: 'Kontynuuj edycję',
    archiveTitle: 'Zarchiwizować „{title}”?',
    archiveMessage:
      'Spowoduje to zarchiwizowanie strony i wszystkich stron podrzędnych. Tej operacji nie można cofnąć.',
    archiveConfirm: 'Archiwizuj',

    // Lista zarchiwizowanych stron (widok administratora otwierany z nagłówka panelu)
    archivedOpenAria: 'Zobacz zarchiwizowane strony',
    archivedHeading: 'Zarchiwizowane strony',
    archivedSubtitle: 'Przejrzyj i przywróć wcześniej zarchiwizowane strony.',
    archivedBack: 'Powrót do stron',
    archivedEmpty: 'Brak zarchiwizowanych stron',
    archivedColTitle: 'Tytuł',
    archivedColArchivedAt: 'Zarchiwizowano',
    archivedColArchivedBy: 'Zarchiwizował',
    archivedUnarchive: 'Przywróć',
    archivedUnarchiveTitle: 'Przywrócić „{title}”?',
    archivedUnarchiveMessage:
      'Strona pojawi się ponownie w drzewie. Jeśli jej strona nadrzędna jest nadal zarchiwizowana, strona pozostanie ukryta, dopóki nie przywrócisz również strony nadrzędnej.',
    archivedUnarchiveConfirm: 'Przywróć',
    archivedUnarchiveOK: 'Przywrócono „{title}”',
    archivedLoadError: 'Nie udało się załadować zarchiwizowanych stron',
    archivedUnarchiveError: 'Nie udało się przywrócić strony',

    // Okno przenoszenia
    moveTitle: 'Przenieś „{title}”',
    moveSubtitle:
      'Wybierz nową stronę nadrzędną. Strony znajdujące się pod bieżącą stroną są ukryte, ponieważ utworzyłyby cykl.',
    moveWorkspaceLabel: 'Docelowa przestrzeń robocza',
    moveWorkspacePlaceholder: 'Wybierz przestrzeń roboczą…',
    moveParentLabel: 'Strona nadrzędna',
    moveSearchPlaceholder: 'Szukaj stron…',
    moveRoot: 'Katalog główny przestrzeni roboczej',
    moveCrossWorkspaceSummary: 'Strony w tym poddrzewie: {count}.',
    moveCrossWorkspacePolicy:
      'Pasujące etykiety zostaną zachowane. Jawne uprawnienia, powiązania elementów pracy i odwołania do umiejętności agenta zostaną usunięte.',
    moveButton: 'Przenieś',
    moveCancel: 'Anuluj',
    errorLoadWorkspaces: 'Nie udało się załadować przestrzeni roboczych',
    errorMove: 'Przenoszenie nie powiodło się',

    // Okno uprawnień
    permsTitle: 'Uprawnienia strony',
    permsEffectiveAccess: 'Twój efektywny dostęp: {level}',
    permsEffectiveAccessNone: 'brak',
    permsLoading: 'Ładowanie…',
    permsInheritLabel: 'Dziedzicz uprawnienia od stron nadrzędnych',
    permsInheritHint:
      'Gdy dziedziczenie jest włączone i nie istnieją jawne przyznania, decydują uprawnienia ról w przestrzeni roboczej. Przerwanie dziedziczenia bez przyznań ogranicza stronę do administratorów.',
    permsExplicitGrants: 'Jawne przyznania',
    permsEmptyGrantsTitle: 'Brak jawnych przyznań na tej stronie.',
    permsEmptyGrantsDescription: 'Dziedziczenie i role w przestrzeni roboczej nadal obowiązują.',
    permsColumnPrincipal: 'Podmiot',
    permsColumnLevel: 'Poziom',
    permsRemove: 'Usuń',
    permsRemoveTitle: 'Usunąć uprawnienie?',
    permsRemoveMessage: 'To przyznanie zostanie usunięte ze strony. Możesz je później dodać ponownie.',
    permsRemoveConfirm: 'Usuń',
    permsRemoveCancel: 'Anuluj',
    permsClose: 'Zamknij',
    permsAdd: 'Dodaj',
    permsPrincipalUser: 'Użytkownik',
    permsPrincipalGroup: 'Grupa',
    permsPrincipalRole: 'Rola',
    permsLevelView: 'Podgląd',
    permsLevelEdit: 'Edycja',
    permsLevelAdmin: 'Administrator',
    permsPickUser: 'Wybierz użytkownika',
    permsPickGroup: 'Wybierz grupę',
    permsPickRole: 'Wybierz rolę',
    permsErrorNoPrincipal: 'Wybierz podmiot przed dodaniem przyznania',
    permsErrorLoad: 'Nie udało się załadować uprawnień',
    permsErrorInherit: 'Nie udało się zaktualizować dziedziczenia',
    permsErrorGrant: 'Nie udało się dodać uprawnienia',
    permsErrorRevoke: 'Nie udało się odebrać',

    // Etykiety stron (w obrębie przestrzeni roboczej, przypisywane tylko do stron)
    labelsTitle: 'Etykiety',
    labelsAdd: 'Dodaj etykietę',
    labelsCreate: 'Utwórz etykietę',
    labelsCreateNamed: 'Utwórz „{name}”',
    labelsSearchPlaceholder: 'Szukaj lub utwórz…',
    labelsFilterTitle: 'Filtruj według etykiety',
    labelsFilterPlaceholder: 'Filtruj strony według etykiety',
    labelsFilterClear: 'Wyczyść filtr',
    labelsEmpty: 'Brak etykiet',
    labelsNameRequired: 'Nazwa etykiety jest wymagana',
    labelsDuplicate: 'Etykieta o tej nazwie już istnieje',
    labelsDeleteConfirm: 'Usunąć tę etykietę? Zostanie usunięta z każdej strony, do której jest przypisana.',
    labelsDelete: 'Usuń etykietę',
    labelsRemoveAria: 'Usuń etykietę {name}',
    labelsErrorLoad: 'Nie udało się załadować etykiet',
    labelsErrorSave: 'Nie udało się zapisać etykiety',
    labelsErrorAttach: 'Nie udało się przypisać etykiety',
    labelsErrorDetach: 'Nie udało się odłączyć etykiety',

    // Wyszukiwanie w panelu bocznym
    searchAria: 'Szukaj stron',
    searchPlaceholder: 'Szukaj stron…',
    searchClear: 'Wyczyść wyszukiwanie',

    // Rozwijanie / zwijanie drzewa w panelu bocznym
    toggleSubtreeAria: 'Przełącz strony podrzędne {title}',
    expandAllAria: 'Rozwiń wszystko',
    collapseAllAria: 'Zwiń wszystko',

    // Przycisk powiązanych elementów pracy + popover (prawy górny róg szczegółów strony)
    workItemsButton: 'Elementy pracy',
    workItemsAria: 'Pokaż powiązane elementy pracy',
    workItemsEmpty: 'Brak powiązanych elementów pracy',
    workItemsLoading: 'Ładowanie elementów pracy…',
    workItemsTitle: 'Elementy pracy',
    addWorkItem: 'Dodaj element pracy',
    addWorkItemCancel: 'Anuluj',
    addWorkItemSearchPlaceholder: 'Szukaj elementów pracy…',
    removeWorkItemLink: 'Odłącz element pracy',
    workItemsErrorLoad: 'Nie udało się załadować powiązanych elementów pracy',
    workItemsErrorLink: 'Nie udało się powiązać elementu pracy',
    workItemsErrorUnlink: 'Nie udało się odłączyć elementu pracy',
  },
};
