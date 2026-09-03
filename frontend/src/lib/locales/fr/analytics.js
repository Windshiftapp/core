/**
 * Analytics translations for French locale.
 */
export default {
  analytics: {
    title: 'Analytique',
    subtitle: 'Santé et flux de livraison, sans nécessiter d’itérations',
    loading: 'Chargement de l’analytique…',
    noData: 'Aucune donnée disponible',
    errorTitle: 'Impossible de charger l’analytique',
    unsupportedVersion:
      'Le serveur a retourné un format d’analytique non pris en charge. Rafraîchissez la page une fois le déploiement terminé.',
    collectionLoadError:
      'Impossible de charger les collections. L’analytique affiche tous les éléments de l’espace de travail.',
    retry: 'Réessayer',
    dateRange: 'Plage de dates',
    collection: 'Collection',
    allItems: 'Tous les éléments de l’espace de travail',
    from: 'Du',
    to: 'Au',
    daysValue: '{value} j',
    items_one: '{count} élément',
    items_other: '{count} éléments',
    range: {
      last30Days: '30 derniers jours',
      last12Weeks: '12 dernières semaines',
      last6Months: '6 derniers mois',
      lastYear: 'L’année dernière',
      custom: 'Personnalisée',
    },
    validation: {
      invalid: 'Saisissez des dates de début et de fin valides.',
      reversed: 'La date de début doit être antérieure ou égale à la date de fin.',
      too_long: 'Choisissez une plage de dates inférieure ou égale à 366 jours.',
    },
    scope: {
      summary: '{items} éléments actuels · {from}–{to}',
      currentWorkspace: 'Cohorte de l’espace de travail actuel',
      currentWorkspaceNote:
        'La plage de dates s’applique aux graphiques de flux et de livraison ; la santé et la maturité sont des instantanés actuels. Les graphiques historiques utilisent les éléments présents aujourd’hui dans cet espace de travail. Les éléments déplacés ou supprimés ne sont pas inclus.',
      currentCollection: 'Cohorte de la collection actuelle',
      currentCollectionNote:
        'La plage de dates s’applique aux graphiques de flux et de livraison ; la santé et la maturité sont des instantanés actuels. Les graphiques historiques utilisent les éléments correspondant aujourd’hui à cette collection. Modifier la collection peut changer la cohorte.',
    },
    health: {
      title: 'Attention requise',
      description: 'Travaux en cours non terminés présentant des signaux méritant un examen plus approfondi.',
      unfinished: 'Non terminé',
      overdue: 'En retard',
      stale: 'Inactif',
      staleHint: 'Aucune activité depuis {days}+ jours',
      unassigned: 'Non attribué',
      withoutPriority: 'Sans priorité',
      withoutEstimate: 'Sans estimation',
      attentionItems: 'Éléments à examiner',
      item: 'Élément',
      status: 'Statut',
      age: 'Âge',
      signals: 'Signaux',
      flags: {
        overdue: 'En retard',
        stale: 'Inactif',
        unassigned: 'Non attribué',
        without_priority: 'Sans priorité',
        without_estimate: 'Sans estimation',
      },
      allClear: 'Aucun élément non terminé ne correspond actuellement à un signal d’attention.',
    },
    throughput: {
      title: 'Créés vs terminés',
      description:
        'Arrivées hebdomadaires et premières finalisations. La réouverture d’un élément ne réécrit pas sa finalisation d’origine.',
      created: 'Créés',
      completed: 'Terminés',
      net: 'Variation nette',
      average: 'Moyenne terminés / semaine',
      period: 'Période',
      definition: 'La finalisation correspond au premier passage à un statut terminé.',
    },
    aging: {
      title: 'Maturité des travaux en cours',
      description: 'Durée depuis laquelle les éléments actuellement non terminés sont ouverts.',
      total: 'Éléments actifs',
      median: 'Âge médian',
      p85: '85e percentile',
      ageBand: 'Tranche d’âge',
      itemCount: 'Éléments',
      byStatus: 'Âge par statut',
      oldest: 'Éléments non terminés les plus anciens',
      status: 'Statut',
      noActive: 'Il n’y a aucun travail non terminé dans cette portée.',
      buckets: {
        '0_7': '0–7 jours',
        '8_14': '8–14 jours',
        '15_30': '15–30 jours',
        '31_60': '31–60 jours',
        '61_plus': '61+ jours',
      },
    },
    deliveryTime: {
      title: 'Délai de livraison',
      description: 'De la création à la première finalisation, regroupé par semaine de finalisation.',
      analyzed: 'Éléments terminés',
      average: 'Moyenne',
      median: 'Médiane',
      p85: '85e percentile',
      period: 'Période de finalisation',
      completed: 'Terminés',
      slowest: 'Délais de livraison les plus longs',
      completedDate: 'Premier terminé',
      duration: 'Délai de livraison',
      missingHistory: '{count} éléments actuellement terminés ont été exclus car leur historique de finalisation est manquant.',
      missingHistory_one:
        '1 élément actuellement terminé a été exclu car son historique de finalisation est manquant.',
      missingHistory_other:
        '{count} éléments actuellement terminés ont été exclus car leur historique de finalisation est manquant.',
      definition:
        'Mesuré de la création de l’élément à son premier passage à un statut terminé. Les réouvertures ultérieures ne modifient pas cette valeur.',
    },
    dataTable: {
      show: 'Afficher le tableau de données',
    },
    insufficientData: {
      no_items: 'Cette portée ne contient encore aucun élément.',
      no_active_items: 'Il n’y a aucun travail non terminé dans cette portée.',
      no_completed_items: 'Aucune première finalisation n’a été enregistrée dans la plage de dates sélectionnée.',
      few_completed_items:
        'Seuls quelques éléments ont été terminés dans cette plage. Traitez les percentiles comme une tendance indicative.',
    },
  },
};