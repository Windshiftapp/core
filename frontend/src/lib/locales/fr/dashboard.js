export default {
  dashboard: {
    salutation: {
      withName: '{salutation}, {name} !',
      withoutName: '{salutation} !',
    },
    sections: {
      yourDay: {
        title: 'Votre journée',
        subtitle: 'Un aperçu rapide de ce qui requiert votre attention',
      },
      work: {
        title: 'Travail',
        subtitle: 'Votre liste personnelle et les éléments qui vous sont attribués',
      },
      workspaces: {
        title: 'Espaces de travail',
        subtitle: 'Reprenez où vous vous étiez arrêté',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'Briefing quotidien',
        description: 'Résumé généré par IA de ce qui importe pour vous aujourd’hui',
      },
      yourActivity: {
        name: 'Votre activité',
        description: 'Éléments que vous avez récemment consultés, modifiés ou commentés',
      },
      whatsNew: {
        name: 'Nouveautés',
        description: 'Dernières notifications et mises à jour non lues',
      },
      personalTasks: {
        name: 'Tâches personnelles',
        description: 'Éléments de votre liste de tâches personnelles',
      },
      savedSearch: {
        name: 'Recherche enregistrée',
        description: 'Afficher les éléments de travail d’une collection enregistrée',
      },
      assignedToMe: {
        name: 'Qui m’est attribué',
        description: 'Éléments ouverts qui vous sont attribués dans tous les espaces de travail',
      },
      watchedItems: {
        name: 'Éléments suivis',
        description: 'Éléments que vous suivez',
      },
      upcomingMilestones: {
        name: 'Jalons à venir',
        description: 'Jalons dont la date cible approche',
      },
      recentWorkspaces: {
        name: 'Espaces de travail récents',
        description: 'Espaces de travail que vous avez récemment visités',
      },
      quickAccess: {
        name: 'Accès rapide',
        description: 'Liens rapides vers les espaces de travail auxquels vous avez accès',
      },
    },
    customization: {
      widgets: 'Widgets',
      activity: {
        name: 'Activité',
        description: 'Briefings, flux d’activité et notifications',
      },
      work: {
        name: 'Travail',
        description: 'Éléments, jalons et tâches qui vous sont attribués',
      },
      navigation: {
        name: 'Navigation',
        description: 'Accès rapide aux espaces de travail',
      },
      tipLabel: 'Conseil',
      tip: 'Faites glisser des widgets d’ici vers n’importe quelle section de votre tableau de bord.',
    },
    editor: {
      newSection: 'Nouvelle section',
      deleteSectionConfirm: 'Supprimer cette section ? Tous les widgets de cette section seront retirés.',
      doneEditing: 'Terminer l’édition',
      customize: 'Personnaliser',
      editModeDescription: 'Mode édition : ajoutez, renommez, réordonnez ou supprimez des sections et des widgets',
      addSection: 'Ajouter une section',
      sectionLabel: 'Section du tableau de bord',
      sectionTitlePlaceholder: 'Titre de la section',
      sectionSubtitlePlaceholder: 'Sous-titre (optionnel)',
      renameSection: 'Renommer la section',
      deleteSection: 'Supprimer la section',
      unknownWidgetType: 'Type de widget inconnu : {type}',
      noWidgets: 'Aucun widget dans cette section pour le moment',
      addWidgetsHint: 'Sélectionnez Personnaliser pour ajouter des widgets',
      noSections: 'Aucune section configurée',
      addSectionsHint: 'Sélectionnez Modifier pour ajouter des sections à votre tableau de bord',
    },
    states: {
      assignedLoadError: 'Impossible de charger les éléments qui vous sont attribués',
      assignedEmpty: 'Rien ne vous est attribué pour le moment',
      personalTasksLoadError: 'Impossible de charger vos tâches personnelles',
      personalTasksEmpty: 'Votre liste de tâches personnelles est vide',
      dailyBriefingUnavailable: 'Votre briefing quotidien n’est pas disponible pour le moment. Il repose sur une intégration IA. Si vous venez d’en configurer une, revenez dans quelques instants.',
      updatedAt: 'Mis à jour {time}',
      priorityLabel: 'Priorité : {priority}',
      noWorkspaces: 'Aucun espace de travail pour le moment',
      createWorkspace: 'En créer un',
      workspaceAvatarAlt: 'Avatar de {name}',
      visited: 'visité {time}',
      noUpcomingMilestones: 'Aucun jalon à venir',
      milestoneProgress: '{done} sur {total} terminés',
      daysOverdue: '{days} jours de retard',
      daysLeft: '{days} jours restants',
      watchedItemsEmpty: 'Vous ne suivez aucun élément',
      workspaceWithId: 'Espace de travail {id}',
    },
  },
};