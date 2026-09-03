/**
 * Actions automation translations (French)
 */
export default {
  actions: {
    title: 'Actions',
    description: 'Automatisez les flux de travail avec des actions basées sur des règles',
    create: 'Créer une action',
    createFirst: 'Créer votre première action',
    noActions: 'Aucune action pour le moment',
    noActionsDescription: 'Créez des actions pour automatiser vos flux de travail en fonction des événements d’éléments',
    enabled: 'Activé',
    disabled: 'Désactivé',
    enable: 'Activer',
    disable: 'Désactiver',
    viewLogs: 'Afficher les journaux',
    confirmDelete: 'Voulez-vous vraiment supprimer l’action « {name} » ?',
    failedToSave: 'Échec de l’enregistrement de l’action',
    newAction: 'Nouvelle action',

    // Action templates (shipped automation blueprints)
    templates: {
      pickTitle: 'Choisir un modèle d’action',
      fromTemplate: 'À partir d’un modèle',
      empty: 'Aucun modèle disponible.',
      help: 'Appliquez un modèle d’automatisation préconfiguré à cet espace de travail. Chaque application crée une nouvelle action que vous pouvez modifier ultérieurement.',
      apply: 'Appliquer',
    },

    // Trigger types
    trigger: {
      statusTransition: 'Changement de statut',
      itemCreated: 'Élément créé',
      itemUpdated: 'Élément mis à jour',
      itemLinked: 'Élément lié',
      manual: 'Manuel',
      respondToCascades: 'Réagir aux modifications déclenchées par d’autres actions',
      respondToCascadesHint:
        'Lorsqu’elle est activée, cette action s’exécutera également si elle est déclenchée par d’autres actions, et pas seulement par des modifications d’utilisateurs.',
    },

    manualAccess: {
      label: 'Qui peut exécuter cette action manuelle ?',
      allEditors: 'Tous les éditeurs de l’espace de travail',
      unrestrictedHint:
        'Aucune restriction de rôle. Toute personne disposant des droits de modification peut voir et exécuter cette action.',
      restrictedHint:
        'Seuls les membres possédant au moins un rôle sélectionné peuvent voir et exécuter cette action. Les administrateurs de l’espace de travail conservent toujours l’accès.',
    },

    // Node types
    nodes: {
      trigger: 'Déclencheur',
      setField: 'Définir un champ',
      setStatus: 'Définir le statut',
      addComment: 'Ajouter un commentaire',
      notifyUser: 'Notifier un utilisateur',
      condition: 'Condition',
      updateAsset: 'Mettre à jour l’actif',
      createAsset: 'Créer un actif',
      httpRequest: 'Requête HTTP',
      containerRun: 'Exécuter le conteneur',
      aiExtract: 'Extraction par IA',
      aiAgent: 'Agent IA',
      relatedItems: 'Pour chaque élément lié',
      transitionItem: 'Faire évoluer l’élément',
      roundRobinAssign: 'Assignation à tour de rôle',
      createMilestone: 'Créer un jalon',
    },

    // Toast shown when the AI chat updates the open action via update_action.
    aiUpdated: 'Action mise à jour par l’IA',

    // Actor override (run-as)
    runAs: 'Exécuter en tant que',
    runAsTriggerUser: 'Exécuter en tant qu’utilisateur déclencheur',
    runAsHint:
      'L’action s’exécute avec les autorisations de cet utilisateur. Laissez vide pour exécuter en tant que déclencheur initial.',
    runAsReadonlyHint: 'Nécessite la permission « Définir l’acteur de l’action » pour être modifié.',

    // Node palette and tips
    addNodes: 'Ajouter des nœuds',
    tips: 'Conseils',
    tipDragToConnect: 'Faites glisser à partir des poignées pour connecter les nœuds',
    tipClickToEdit: 'Cliquez sur un nœud pour le configurer',
    tipConditionBranches: 'Les conditions ont des branches vrai/faux',

    // Config panel
    nodeConfig: 'Configuration du nœud',
    config: {
      from: 'De',
      to: 'Vers',
      selectField: 'Sélectionner un champ...',
      selectStatus: 'Sélectionner un statut...',
      config: 'Configuration',
      configure: 'Configurer',
      selectConfig: 'Sélectionner la configuration',
      enterComment: 'Saisir un commentaire...',
      selectRecipient: 'Sélectionner un destinataire...',
      setCondition: 'Définir une condition...',
      targetStatus: 'Statut cible',
      fieldName: 'Nom du champ',
      value: 'Valeur',
      commentContent: 'Contenu du commentaire',
      commentPlaceholder: 'Saisissez le texte du commentaire. Utilisez {{item.title}} pour les variables.',
      privateComment: 'Commentaire privé (interne uniquement)',
      fieldToCheck: 'Champ à vérifier',
      operator: 'Opérateur',
      compareValue: 'Valeur de comparaison',
      private: 'Privé',
      triggerType: 'Type de déclencheur',
      fromStatus: 'Depuis le statut',
      toStatus: 'Vers le statut',
      anyStatus: 'N’importe quel statut',
      triggerField: 'Champ modifié',
      anyField: 'N’importe quel champ (toutes les modifications)',
      recipientType: 'Destinataire',
      notifyMessage: 'Message',
      notifyPlaceholder: 'Saisissez le message. Utilisez {{item.title}} pour les variables.',
      includeLink: 'Inclure le lien vers l’élément',
      // Update Asset config
      sourceAssetField: 'Champ d’actif sur l’élément',
      selectAssetField: 'Sélectionner le champ d’actif...',
      sourceAssetFieldHint: 'Sélectionnez le champ d’élément qui contient l’actif lié',
      targetAssetType: 'Type d’actif cible',
      selectAssetType: 'Sélectionner le type d’actif...',
      fieldMappingsLabel: 'Mappages de champs',
      fieldMappings: '{count} mappages de champs',
      fieldMappings_one: '{count} mappage de champ',
      fieldMappings_other: '{count} mappages de champs',
      configureAssetUpdate: 'Configurer la mise à jour de l’actif...',
      fromField: 'Depuis le champ',
      sourceTypeVariable: 'Variable/Modèle',
      sourceTypeItemField: 'Champ d’élément',
      sourceTypeLiteral: 'Valeur littérale',
      selectTargetField: 'Sélectionner le champ cible...',
      addMapping: 'Ajouter un mappage',
      milestonePickerHint: 'Stocke les identifiants de jalons pour l’action ; les noms sont affichés uniquement pour la modification.',
      userPickerHint: 'Choisissez un utilisateur spécifique, ou saisissez un ID utilisateur/modèle ci-dessous.',
      // Create Asset config
      assetSet: 'Ensemble d’actifs',
      selectAssetSet: 'Sélectionner l’ensemble d’actifs...',
      assetTitle: 'Titre de l’actif',
      assetTitleHint: 'Utilisez {{item.title}} ou d’autres variables',
      assetDescription: 'Description',
      assetTagLabel: 'Étiquette de l’actif',
      assetCategory: 'Catégorie',
      selectCategory: 'Sélectionner une catégorie (facultatif)...',
      assetStatus: 'Statut',
      selectStatusOptional: 'Sélectionner un statut (facultatif)...',
      requiredField: 'Requis',
      configureAssetCreation: 'Configurer la création de l’actif...',
      // Capability picker (HTTP, Docker, LLM nodes)
      capability: 'Capacité',
      selectCapability: 'Sélectionner une capacité...',
      noCapabilitiesForWorkspace:
        'Aucune capacité disponible dans cet espace de travail. Demandez à un administrateur d’en approvisionner une.',
      configureRequest: 'Configurer la requête HTTP...',
      configureExtract: 'Configurer l’extraction IA...',
      selectModelAndTools: 'Sélectionner le modèle et les outils...',
      // HTTP request node
      httpCapability: 'Capacité de client HTTP',
      httpMethod: 'Méthode',
      urlTemplate: 'Modèle d’URL',
      requestBody: 'Corps de la requête',
      requestBodyPlaceholder: 'Facultatif. Corps JSON, peut utiliser {{variables}}.',
      httpHeaders: 'En-têtes',
      addHeader: 'Ajouter un en-tête',
      headerName: 'Nom de l’en-tête',
      headerValue: 'Valeur',
      // Container run node
      dockerCapability: 'Environnement Docker',
      timeoutSecs: 'Délai d’expiration (secondes)',
      // AI nodes
      llmCapability: 'Connexion LLM',
      model: 'Modèle',
      tools: 'Outils',
      aiPrompt: 'Invite (Prompt)',
      aiExtractPromptPlaceholder:
        'Extrayez des données structurées à partir de l’entrée. Soyez précis sur ce qu’il faut extraire.',
      systemPrompt: 'Invite système',
      systemPromptPlaceholder: 'Vous êtes un assistant utile. Utilisez les outils pour...',
      inputField: 'Champ d’entrée',
      inputFieldPlaceholder: 'nom de la variable dans laquelle lire l’entrée',
      inputFields: 'Champs d’entrée',
      inputFieldsPlaceholder: 'noms de variables séparés par des virgules',
      outputField: 'Champ de sortie',
      outputFieldPlaceholder: 'nom de la variable dans laquelle écrire la sortie',
      outputSchema: 'Schéma JSON de sortie',
      agentTools: 'Outils',
      agentToolsHint:
        'Capacités du client HTTP que l’agent peut appeler. Seules les capacités restreintes à cet espace de travail sont répertoriées.',
      noToolsAvailable: 'Aucune capacité de client HTTP disponible pour cet espace de travail.',
      maxSteps: 'Iterations max',
    },

    // Recipients
    recipients: {
      assignee: 'Assigné',
      creator: 'Créateur',
      specific: 'Utilisateurs spécifiques',
    },

    // Condition
    condition: {
      true: 'Oui',
      false: 'Non',
    },

    // Operators
    operators: {
      equals: 'Égal à',
      notEquals: 'Différent de',
      contains: 'Contient',
      greaterThan: 'Supérieur à',
      lessThan: 'Inférieur à',
      isEmpty: 'Est vide',
      isNotEmpty: 'N’est pas vide',
    },

    // Execution logs
    logs: {
      title: 'Journaux d’exécution',
      noLogs: 'Aucun journal d’exécution',
      status: 'Statut',
      running: 'En cours d’exécution',
      completed: 'Terminé',
      failed: 'Échoué',
      skipped: 'Ignoré',
      startedAt: 'Démarre à',
      completedAt: 'Terminé à',
      error: 'Erreur',
      details: 'Détails',
      viewDetails: 'Afficher les détails',
    },

    // Execution trace
    trace: {
      title: 'Détails de l’exécution',
      noSteps: 'Aucune étape d’exécution enregistrée',
      setStatus: 'Statut modifié de « {from} » à « {to} »',
      setField: 'Champ {field} modifié de « {from} » à « {to} »',
      addComment: 'Commentaire {prefix}ajouté : « {content} »',
      notifyUser: 'Notification envoyée à {count} utilisateurs',
      notifyUser_one: 'Notification envoyée à {count} utilisateur',
      notifyUser_other: 'Notification envoyée à {count} utilisateurs',
      notifySkipped: 'Notification ignorée : {reason}',
      conditionResult: 'La condition a été évaluée à {result}',
      updateAsset: 'Actif #{asset_id} mis à jour',
      updateAssetSkipped: 'Mise à jour de l’actif ignorée : {reason}',
      createAsset: 'Actif #{asset_id} créé : {title}',
      createAssetFailed: 'Échec de la création de l’actif : {reason}',
    },

    // Test/manual execution
    test: {
      title: 'Tester l’action',
      description:
        'Sélectionnez un élément sur lequel exécuter cette action. Cela exécutera l’action immédiatement, en contournant le déclencheur habituel.',
      selectItem: 'Sélectionner un élément',
      itemPlaceholder: 'Rechercher un élément...',
      execute: 'Exécuter l’action',
      run: 'Exécution de test',
      executionFailed: 'Échec de l’exécution de l’action',
      executionQueued: 'Action mise en file d’attente pour exécution',
    },

    // Placeholder reference
    placeholders: {
      title: 'Espaces réservés disponibles',
      description:
        'Utilisez ces espaces réservés dans votre modèle. Ils seront remplacés par des valeurs réelles lorsque l’action s’exécutera.',
      showReference: 'Afficher la référence des espaces réservés',
      categories: {
        item: 'Champs de l’élément',
        user: 'Utilisateur actuel',
        old: 'Anciennes valeurs',
        trigger: 'Contexte du déclencheur',
      },
      item: {
        title: 'Titre de l’élément',
        id: 'ID de l’élément',
        statusId: 'ID du statut',
        assigneeId: 'ID de l’utilisateur assigné',
        any: 'N’importe quel champ de l’élément',
      },
      user: {
        name: 'Nom complet de l’utilisateur',
        email: 'E-mail de l’utilisateur',
        id: 'ID de l’utilisateur',
      },
      old: {
        description: 'Valeur précédente avant la modification',
        example: 'Valeur précédente de n’importe quel champ',
      },
      trigger: {
        itemId: 'ID de l’élément déclencheur',
        workspaceId: 'ID de l’espace de travail',
      },
    },
    switchToVertical: 'Passer à la disposition verticale',
    switchToHorizontal: 'Passer à la disposition horizontale',
  },
};