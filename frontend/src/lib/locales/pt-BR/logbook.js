/**
 * Logbook / Knowledge Base translations for Brazilian Portuguese locale
 */

export default {
  logbook: {
    title: 'Base de Conhecimento',
    subtitle: 'Documentos, notas e conhecimento da equipe',
    allDocuments: 'Todos os Documentos',
    createBucket: 'Criar Bucket',
    uploadDocument: 'Enviar Documento',
    newNote: 'Nova Nota',
    noDocuments: 'Nenhum documento ainda',
    noDocumentsDescription: 'Envie um arquivo ou crie uma nota para começar',
    noDocumentsAllDescription: 'Nenhum documento encontrado nos seus buckets acessíveis',
    noBuckets: 'Nenhum bucket ainda',
    noBucketsDescription: 'Crie um bucket para organizar seu conhecimento',
    search: 'Pesquisar documentos...',
    article: 'Artigo',
    rawContent: 'Conteúdo Bruto',
    info: 'Informações',
    back: 'Voltar',
    save: 'Salvar',
    saving: 'Salvando...',
    saved: 'Documento salvo',
    uploadSuccess: 'Documento enviado com sucesso',
    noteCreated: 'Nota criada com sucesso',
    bucketCreated: 'Bucket criado com sucesso',
    bucketUpdated: 'Bucket atualizado',
    bucketDeleted: 'Bucket excluído',
    confirmDeleteBucket:
      'Tem certeza de que deseja excluir este bucket? Todos os documentos nele serão arquivados.',
    documentArchived: 'Documento arquivado',
    documentDeleted: 'Documento excluído',
    confirmDelete: 'Tem certeza de que deseja excluir este documento?',
    confirmArchiveDocument: 'Tem certeza de que deseja arquivar este documento?',
    viewOriginal: 'Ver Original',
    delete: 'Excluir',

    // Bucket form
    bucketName: 'Nome do bucket',
    bucketNamePlaceholder: 'ex: Docs de Engenharia',
    bucketDescription: 'Descrição',
    bucketDescriptionPlaceholder: 'Que tipo de documentos pertencem aqui?',

    // Note form
    noteTitle: 'Título',
    noteTitlePlaceholder: 'Título da nota',
    noteContent: 'Conteúdo',
    noteContentPlaceholder: 'Escreva sua nota em Markdown...',

    // Upload
    dropzoneTitle: 'Solte os arquivos aqui',
    dropzoneDescription:
      'ou clique para procurar. Suporta PDF, DOCX, PPTX, XLSX, TXT, MD, HTML e imagens.',
    uploading: 'Enviando...',
    documentTitle: 'Título do documento',
    documentTitlePlaceholder: 'Opcional - usa o nome do arquivo por padrão',

    // Status
    status: {
      pending: 'Pendente',
      processing: 'Processando',
      ready: 'Pronto',
      error: 'Erro',
    },

    // Source type
    sourceType: {
      upload: 'Upload',
      note: 'Nota',
      email: 'E-mail',
    },

    // Content type (classification)
    contentType: {
      knowledge: 'Conhecimento',
      record: 'Registro',
      correspondence: 'Correspondência',
    },

    // Document info
    mimeType: 'Tipo de arquivo',
    contentHash: 'Hash do conteúdo',
    retrievalCount: 'Vezes recuperado',
    chunkCount: 'Chunks',
    createdAt: 'Criado em',
    updatedAt: 'Atualizado em',
    reviewedAt: 'Revisado em',
    health: 'Saúde',
    author: 'Autor',
    processingMessage: 'O documento está sendo processado...',
  },
};
