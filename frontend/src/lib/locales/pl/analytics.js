/**
 * Tłumaczenia analityki dla języka polskiego.
 */
export default {
  analytics: {
    title: 'Analityka',
    subtitle: 'Kondycja i przepływ realizacji, bez potrzeby iteracji',
    loading: 'Ładowanie analityki...',
    noData: 'Brak dostępnych danych',
    errorTitle: 'Nie udało się załadować analityki',
    unsupportedVersion:
      'Serwer zwrócił nieobsługiwany format analityki. Odśwież po zakończeniu wdrożenia.',
    collectionLoadError:
      'Nie udało się załadować kolekcji. Analityka pokazuje wszystkie elementy przestrzeni roboczej.',
    retry: 'Ponów',
    dateRange: 'Zakres dat',
    collection: 'Kolekcja',
    allItems: 'Wszystkie elementy przestrzeni roboczej',
    from: 'Od',
    to: 'Do',
    daysValue: '{value} dni',
    items_one: '{count} element',
    items_few: '{count} elementy',
    items_many: '{count} elementów',
    items_other: '{count} elementu',
    range: {
      last30Days: 'Ostatnie 30 dni',
      last12Weeks: 'Ostatnie 12 tygodni',
      last6Months: 'Ostatnie 6 miesięcy',
      lastYear: 'Ostatni rok',
      custom: 'Niestandardowy',
    },
    validation: {
      invalid: 'Wprowadź prawidłowe daty początkową i końcową.',
      reversed: 'Data początkowa musi być wcześniejsza lub równa dacie końcowej.',
      too_long: 'Wybierz zakres dat nieprzekraczający 366 dni.',
    },
    scope: {
      summary: '{items} bieżących elementów · {from}–{to}',
      currentWorkspace: 'Bieżąca kohorta przestrzeni roboczej',
      currentWorkspaceNote:
        'Zakres dat dotyczy wykresów przepływu i realizacji; kondycja i wiek to bieżące migawki. Wykresy historyczne używają elementów, które są dziś w tej przestrzeni roboczej. Elementy przeniesione lub usunięte nie są uwzględniane.',
      currentCollection: 'Bieżąca kohorta kolekcji',
      currentCollectionNote:
        'Zakres dat dotyczy wykresów przepływu i realizacji; kondycja i wiek to bieżące migawki. Wykresy historyczne używają elementów, które dziś pasują do tej kolekcji. Zmiana kolekcji może zmienić kohortę.',
    },
    health: {
      title: 'Wymaga uwagi',
      description: 'Bieżąca niedokończona praca z sygnałami, którym warto się przyjrzeć.',
      unfinished: 'Niedokończone',
      overdue: 'Po terminie',
      stale: 'Bezczynne',
      staleHint: 'Brak aktywności od {days}+ dni',
      unassigned: 'Nieprzypisane',
      withoutPriority: 'Bez priorytetu',
      withoutEstimate: 'Bez szacunku',
      attentionItems: 'Elementy do przejrzenia',
      item: 'Element',
      status: 'Status',
      age: 'Wiek',
      signals: 'Sygnały',
      flags: {
        overdue: 'Po terminie',
        stale: 'Bezczynne',
        unassigned: 'Nieprzypisane',
        without_priority: 'Bez priorytetu',
        without_estimate: 'Bez szacunku',
      },
      allClear: 'Żadne niedokończone elementy nie pasują obecnie do sygnału uwagi.',
    },
    throughput: {
      title: 'Utworzone a ukończone',
      description:
        'Tygodniowe napływy i pierwsze ukończenia. Ponowne otwarcie elementu nie zmienia jego pierwotnego ukończenia.',
      created: 'Utworzone',
      completed: 'Ukończone',
      net: 'Zmiana netto',
      average: 'Śr. ukończone / tydzień',
      period: 'Okres',
      definition: 'Ukończenie oznacza pierwsze przejście do statusu ukończenia.',
    },
    aging: {
      title: 'Starzejąca się praca w toku',
      description: 'Jak długo otwarte są obecnie niedokończone elementy.',
      total: 'Aktywne elementy',
      median: 'Mediana wieku',
      p85: '85. percentyl',
      ageBand: 'Przedział wieku',
      itemCount: 'Elementy',
      byStatus: 'Wiek według statusu',
      oldest: 'Najstarsze niedokończone elementy',
      status: 'Status',
      noActive: 'W tym zakresie nie ma niedokończonej pracy.',
      buckets: {
        '0_7': '0–7 dni',
        '8_14': '8–14 dni',
        '15_30': '15–30 dni',
        '31_60': '31–60 dni',
        '61_plus': '61+ dni',
      },
    },
    deliveryTime: {
      title: 'Czas realizacji',
      description: 'Od utworzenia do pierwszego ukończenia, pogrupowane według tygodnia ukończenia.',
      analyzed: 'Ukończone elementy',
      average: 'Średnia',
      median: 'Mediana',
      p85: '85. percentyl',
      period: 'Okres ukończenia',
      completed: 'Ukończone',
      slowest: 'Najdłuższe czasy realizacji',
      completedDate: 'Pierwsze ukończenie',
      duration: 'Czas realizacji',
      missingHistory: '{count} obecnie ukończonych elementów zostało wykluczonych, ponieważ brakuje ich historii ukończenia.',
      missingHistory_one:
        '1 obecnie ukończony element został wykluczony, ponieważ brakuje jego historii ukończenia.',
      missingHistory_few:
        '{count} obecnie ukończone elementy zostały wykluczone, ponieważ brakuje ich historii ukończenia.',
      missingHistory_many:
        '{count} obecnie ukończonych elementów zostało wykluczonych, ponieważ brakuje ich historii ukończenia.',
      missingHistory_other:
        '{count} obecnie ukończonego elementu zostało wykluczone, ponieważ brakuje jego historii ukończenia.',
      definition:
        'Mierzony od utworzenia elementu do jego pierwszego przejścia do statusu ukończenia. Późniejsze ponowne otwarcia nie zmieniają wartości.',
    },
    dataTable: {
      show: 'Zobacz tabelę danych',
    },
    insufficientData: {
      no_items: 'Ten zakres nie ma jeszcze elementów.',
      no_active_items: 'W tym zakresie nie ma niedokończonej pracy.',
      no_completed_items: 'W wybranym zakresie dat nie odnotowano pierwszych ukończeń.',
      few_completed_items:
        'W tym zakresie ukończono tylko kilka elementów. Percentyle należy traktować orientacyjnie.',
    },
  },
};
