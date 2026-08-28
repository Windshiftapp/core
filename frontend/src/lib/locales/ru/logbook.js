/**
 * Журнал / база знаний — русская локализация.
 */

export default {
  logbook: {
    title: 'База знаний',
    subtitle: 'Документы, заметки и знания команды',
    allDocuments: 'Все документы',
    createBucket: 'Создать раздел',
    uploadDocument: 'Загрузить документ',
    newNote: 'Новая заметка',
    noDocuments: 'Документов пока нет',
    noDocumentsDescription: 'Загрузите файл или создайте первую заметку.',
    noDocumentsAllDescription: 'В доступных вам разделах пока нет документов.',
    noBuckets: 'Разделов пока нет',
    noBucketsDescription: 'Создайте раздел, чтобы собрать вместе документы на одну тему.',
    search: 'Поиск документов…',
    article: 'Статья',
    rawContent: 'Исходное содержимое',
    info: 'Сведения',
    back: 'Назад',
    save: 'Сохранить',
    saving: 'Сохранение…',
    saved: 'Документ сохранён',
    uploadSuccess: 'Документ загружен',
    noteCreated: 'Заметка создана',
    bucketCreated: 'Раздел создан',
    bucketUpdated: 'Раздел обновлён',
    bucketDeleted: 'Раздел удалён',
    confirmDeleteBucket:
      'Удалить этот раздел? Все документы в нём будут перемещены в архив.',
    documentArchived: 'Документ перемещён в архив',
    documentDeleted: 'Документ удалён',
    confirmDelete: 'Удалить этот документ?',
    confirmArchiveDocument: 'Переместить этот документ в архив?',
    viewOriginal: 'Открыть оригинал',
    delete: 'Удалить',

    bucketName: 'Название раздела',
    bucketNamePlaceholder: 'Например, техническая документация',
    bucketDescription: 'Описание',
    bucketDescriptionPlaceholder: 'Какие документы будут храниться здесь?',

    noteTitle: 'Заголовок',
    noteTitlePlaceholder: 'Заголовок заметки',
    noteContent: 'Содержимое',
    noteContentPlaceholder: 'Напишите заметку в формате Markdown…',

    dropzoneTitle: 'Перетащите файлы сюда',
    dropzoneDescription:
      'или нажмите, чтобы выбрать. Поддерживаются PDF, DOCX, PPTX, XLSX, TXT, MD, HTML и изображения.',
    uploading: 'Загрузка…',
    documentTitle: 'Название документа',
    documentTitlePlaceholder: 'Необязательно — по умолчанию используется имя файла',

    status: {
      pending: 'Ожидает',
      processing: 'Обрабатывается',
      ready: 'Готов',
      error: 'Ошибка',
    },

    sourceType: {
      upload: 'Загрузка',
      note: 'Заметка',
      email: 'Электронная почта',
    },

    contentType: {
      knowledge: 'Знания',
      record: 'Запись',
      correspondence: 'Переписка',
    },

    mimeType: 'Тип файла',
    contentHash: 'Хеш содержимого',
    retrievalCount: 'Обращений к документу',
    chunkCount: 'Фрагментов',
    createdAt: 'Создан',
    updatedAt: 'Обновлён',
    reviewedAt: 'Проверен',
    health: 'Состояние',
    author: 'Автор',
    processingMessage: 'Документ обрабатывается…',
    runAction: 'Выполнить действие',
  },
};
