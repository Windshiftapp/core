/**
 * Logbook / Knowledge Base translations for French locale
 */

export default {
  logbook: {
    title: 'Base de connaissances',
    subtitle: 'Documents, notes et connaissances de l’équipe',
    allDocuments: 'Tous les documents',
    createBucket: 'Créer un dossier',
    uploadDocument: 'Téléverser un document',
    newNote: 'Nouvelle note',
    noDocuments: 'Aucun document pour le moment',
    noDocumentsDescription: 'Téléversez un fichier ou créez une note pour commencer',
    noDocumentsAllDescription: 'Aucun document trouvé dans vos dossiers accessibles',
    noBuckets: 'Aucun dossier pour le moment',
    noBucketsDescription: 'Créez un dossier pour organiser vos connaissances',
    search: 'Rechercher des documents…',
    article: 'Article',
    rawContent: 'Contenu brut',
    info: 'Info',
    back: 'Retour',
    save: 'Enregistrer',
    saving: 'Enregistrement…',
    saved: 'Document enregistré',
    uploadSuccess: 'Document téléversé avec succès',
    noteCreated: 'Note créée avec succès',
    bucketCreated: 'Dossier créé avec succès',
    bucketUpdated: 'Dossier mis à jour',
    bucketDeleted: 'Dossier supprimé',
    confirmDeleteBucket:
      'Voulez-vous vraiment supprimer ce dossier ? Tous les documents qu’il contient seront archivés.',
    documentArchived: 'Document archivé',
    documentDeleted: 'Document supprimé',
    confirmDelete: 'Voulez-vous vraiment supprimer ce document ?',
    confirmArchiveDocument: 'Voulez-vous vraiment archiver ce document ?',
    viewOriginal: 'Afficher l’original',
    delete: 'Supprimer',

    // Bucket form
    bucketName: 'Nom du dossier',
    bucketNamePlaceholder: 'ex. Docs ingénierie',
    bucketDescription: 'Description',
    bucketDescriptionPlaceholder: 'Quel type de documents va ici ?',

    // Note form
    noteTitle: 'Titre',
    noteTitlePlaceholder: 'Titre de la note',
    noteContent: 'Contenu',
    noteContentPlaceholder: 'Rédigez votre note en markdown…',

    // Upload
    dropzoneTitle: 'Déposez les fichiers ici',
    dropzoneDescription:
      'ou cliquez pour parcourir. Prend en charge PDF, DOCX, PPTX, XLSX, TXT, MD, HTML et les images.',
    uploading: 'Téléversement…',
    documentTitle: 'Titre du document',
    documentTitlePlaceholder: 'Optionnel - nom du fichier par défaut',

    // Status
    status: {
      pending: 'En attente',
      processing: 'Traitement en cours',
      ready: 'Prêt',
      error: 'Erreur',
    },

    // Source type
    sourceType: {
      upload: 'Téléversement',
      note: 'Note',
      email: 'E-mail',
    },

    // Content type (classification)
    contentType: {
      knowledge: 'Connaissance',
      record: 'Registre',
      correspondence: 'Correspondance',
    },

    // Document info
    mimeType: 'Type de fichier',
    contentHash: 'Hachage du contenu',
    retrievalCount: 'Nombre d’extractions',
    chunkCount: 'Segments',
    createdAt: 'Créé le',
    updatedAt: 'Mis à jour le',
    reviewedAt: 'Révisé le',
    health: 'Santé',
    author: 'Auteur',
    processingMessage: 'Le document est en cours de traitement…',
    runAction: 'Exécuter l’action',
  },
};