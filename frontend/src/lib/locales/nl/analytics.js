/**
 * Analytics translations for Dutch locale.
 */
export default {
  analytics: {
    title: 'Analyse',
    subtitle: 'Leveringsgezondheid en flow, zonder iteraties te vereisen',
    loading: 'Analyses laden...',
    noData: 'Geen gegevens beschikbaar',
    errorTitle: 'Analyses konden niet worden geladen',
    unsupportedVersion:
      'De server heeft een niet-ondersteund analyseformaat geretourneerd. Vernieuw nadat de implementatie is voltooid.',
    collectionLoadError:
      'Collecties konden niet worden geladen. Analyses tonen alle items in de werkruimte.',
    retry: 'Opnieuw proberen',
    dateRange: 'Datumbereik',
    collection: 'Collectie',
    allItems: 'Alle items in de werkruimte',
    from: 'Van',
    to: 'Tot',
    daysValue: '{value}d',
    items_one: '{count} item',
    items_other: '{count} items',
    range: {
      last30Days: 'Afgelopen 30 dagen',
      last12Weeks: 'Afgelopen 12 weken',
      last6Months: 'Afgelopen 6 maanden',
      lastYear: 'Afgelopen jaar',
      custom: 'Aangepast',
    },
    validation: {
      invalid: 'Voer geldige begin- en einddatums in.',
      reversed: 'De begindatum moet op of vóór de einddatum liggen.',
      too_long: 'Kies een datumbereik van 366 dagen of minder.',
    },
    scope: {
      summary: '{items} items in dit bereik · {from}–{to}',
      currentWorkspace: 'Huidige werkruimtecohort',
      currentWorkspaceNote:
        'Het datumbereik geldt voor flow- en leveringsgrafieken; gezondheid en veroudering zijn momentopnames. Historische grafieken gebruiken items die zich vandaag in deze werkruimte bevinden. Verplaatste of verwijderde items worden niet meegenomen.',
      currentCollection: 'Huidige collectiecohort',
      currentCollectionNote:
        'Het datumbereik geldt voor flow- en leveringsgrafieken; gezondheid en veroudering zijn momentopnames. Historische grafieken gebruiken items die vandaag bij deze collectie passen. Het wijzigen van de collectie kan het cohort veranderen.',
    },
    health: {
      title: 'Aandacht nodig',
      description: 'Huidig onvoltooid werk met signalen die een nadere blik verdienen.',
      unfinished: 'Onvoltooid',
      overdue: 'Te laat',
      stale: 'Verouderd',
      staleHint: 'Geen activiteit gedurende {days}+ dagen',
      unassigned: 'Niet toegewezen',
      withoutPriority: 'Geen prioriteit',
      withoutEstimate: 'Geen schatting',
      attentionItems: 'Items om te beoordelen',
      item: 'Item',
      status: 'Status',
      age: 'Leeftijd',
      signals: 'Signalen',
      flags: {
        overdue: 'Te laat',
        stale: 'Verouderd',
        unassigned: 'Niet toegewezen',
        without_priority: 'Geen prioriteit',
        without_estimate: 'Geen schatting',
      },
      allClear: 'Er zijn momenteel geen onvoltooide items die aan een aandachtssignaal voldoen.',
    },
    throughput: {
      title: 'Aangemaakt versus voltooid',
      description:
        'Wekelijkse aankomsten en eerste voltooiingen. Het heropenen van een item herschrijft de oorspronkelijke voltooiing niet.',
      created: 'Aangemaakt',
      completed: 'Voltooid',
      net: 'Netto verandering',
      average: 'Gem. voltooid / week',
      period: 'Periode',
      definition: 'Voltooiing betekent de eerste overgang naar een voltooide status.',
    },
    aging: {
      title: 'Verouderend werk in uitvoering',
      description: 'Hoe lang momenteel onvoltooide items al openstaan.',
      total: 'Actieve items',
      median: 'Mediane leeftijd',
      p85: '85e percentiel',
      ageBand: 'Leeftijdsband',
      itemCount: 'Items',
      byStatus: 'Leeftijd per status',
      oldest: 'Oudste onvoltooide items',
      status: 'Status',
      noActive: 'Er is geen onvoltooid werk in dit bereik.',
      buckets: {
        '0_7': '0–7 dagen',
        '8_14': '8–14 dagen',
        '15_30': '15–30 dagen',
        '31_60': '31–60 dagen',
        '61_plus': '61+ dagen',
      },
    },
    deliveryTime: {
      title: 'Levertijd',
      description: 'Van aanmaak tot eerste voltooiing, gegroepeerd per voltooiingsweek.',
      analyzed: 'Voltooide items',
      average: 'Gemiddelde',
      median: 'Mediaan',
      p85: '85e percentiel',
      period: 'Voltooiingsperiode',
      completed: 'Voltooid',
      slowest: 'Langste levertijden',
      completedDate: 'Eerst voltooid',
      duration: 'Levertijd',
      missingHistory: '{count} momenteel voltooide items zijn uitgesloten omdat hun voltooiingsgeschiedenis ontbreekt.',
      missingHistory_one:
        '1 momenteel voltooid item is uitgesloten omdat de voltooiingsgeschiedenis ontbreekt.',
      missingHistory_other:
        '{count} momenteel voltooide items zijn uitgesloten omdat hun voltooiingsgeschiedenis ontbreekt.',
      definition:
        'Gemeten vanaf het aanmaken van het item tot de eerste overgang naar een voltooide status. Later heropenen verandert de waarde niet.',
    },
    dataTable: {
      show: 'Gegevenstabel bekijken',
    },
    insufficientData: {
      no_items: 'Dit bereik heeft nog geen items.',
      no_active_items: 'Er is geen onvoltooid werk in dit bereik.',
      no_completed_items: 'Er zijn geen eerste voltooiingen geregistreerd in het geselecteerde datumbereik.',
      few_completed_items:
        'Er zijn slechts enkele items voltooid in dit bereik. Behandel percentielen als indicatief.',
    },
  },
};
