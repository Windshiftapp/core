/**
 * Logbook / Knowledge Base translations for English locale
 */

export default {
  logbook: {
    title: 'Knowledge Base',
    subtitle: 'Documents, notes, and team knowledge',
    allDocuments: 'All Documents',
    createBucket: 'Create Bucket',
    uploadDocument: 'Upload Document',
    newNote: 'New Note',
    noDocuments: 'No documents yet',
    noDocumentsDescription: 'Upload a file or create a note to get started',
    noDocumentsAllDescription: 'No documents found across your accessible buckets',
    noBuckets: 'No buckets yet',
    noBucketsDescription: 'Create a bucket to organize your knowledge',
    search: 'Search documents...',
    article: 'Article',
    rawContent: 'Raw Content',
    info: 'Info',
    back: 'Back',
    save: 'Save',
    saving: 'Saving...',
    saved: 'Document saved',
    uploadSuccess: 'Document uploaded successfully',
    noteCreated: 'Note created successfully',
    bucketCreated: 'Bucket created successfully',
    bucketUpdated: 'Bucket updated',
    bucketDeleted: 'Bucket deleted',
    confirmDeleteBucket:
      'Are you sure you want to delete this bucket? All documents in it will be archived.',
    documentArchived: 'Document archived',
    documentDeleted: 'Document deleted',
    confirmDelete: 'Are you sure you want to delete this document?',
    confirmArchiveDocument: 'Are you sure you want to archive this document?',
    viewOriginal: 'View Original',
    delete: 'Delete',

    // Bucket form
    bucketName: 'Bucket name',
    bucketNamePlaceholder: 'e.g. Engineering Docs',
    bucketDescription: 'Description',
    bucketDescriptionPlaceholder: 'What kind of documents belong here?',

    // Note form
    noteTitle: 'Title',
    noteTitlePlaceholder: 'Note title',
    noteContent: 'Content',
    noteContentPlaceholder: 'Write your note in markdown...',

    // Upload
    dropzoneTitle: 'Drop files here',
    dropzoneDescription:
      'or click to browse. Supports PDF, DOCX, PPTX, XLSX, TXT, MD, HTML, and images.',
    uploading: 'Uploading...',
    documentTitle: 'Document title',
    documentTitlePlaceholder: 'Optional - defaults to filename',

    // Status
    status: {
      pending: 'Pending',
      processing: 'Processing',
      ready: 'Ready',
      error: 'Error',
    },

    // Source type
    sourceType: {
      upload: 'Upload',
      note: 'Note',
      email: 'Email',
    },

    // Content type (classification)
    contentType: {
      knowledge: 'Knowledge',
      record: 'Record',
      correspondence: 'Correspondence',
    },

    // Document info
    mimeType: 'File type',
    contentHash: 'Content hash',
    retrievalCount: 'Times retrieved',
    chunkCount: 'Chunks',
    createdAt: 'Created',
    updatedAt: 'Updated',
    reviewedAt: 'Reviewed',
    health: 'Health',
    author: 'Author',
    processingMessage: 'Document is being processed...',
  },
};
