/**
 * Tłumaczenia dziennika / bazy wiedzy dla języka polskiego
 */

export default {
  logbook: {
    title: 'Baza wiedzy',
    subtitle: 'Dokumenty, notatki i wiedza zespołu',
    allDocuments: 'Wszystkie dokumenty',
    createBucket: 'Utwórz zasobnik',
    uploadDocument: 'Wgraj dokument',
    newNote: 'Nowa notatka',
    noDocuments: 'Brak dokumentów',
    noDocumentsDescription: 'Wgraj plik lub utwórz notatkę, aby rozpocząć',
    noDocumentsAllDescription: 'Nie znaleziono dokumentów w dostępnych zasobnikach',
    noBuckets: 'Brak zasobników',
    noBucketsDescription: 'Utwórz zasobnik, aby uporządkować wiedzę',
    search: 'Szukaj dokumentów...',
    article: 'Artykuł',
    rawContent: 'Treść źródłowa',
    info: 'Informacje',
    back: 'Wstecz',
    save: 'Zapisz',
    saving: 'Zapisywanie...',
    saved: 'Dokument zapisany',
    uploadSuccess: 'Dokument wgrany pomyślnie',
    noteCreated: 'Notatka utworzona pomyślnie',
    bucketCreated: 'Zasobnik utworzony pomyślnie',
    bucketUpdated: 'Zasobnik zaktualizowany',
    bucketDeleted: 'Zasobnik usunięty',
    confirmDeleteBucket:
      'Czy na pewno chcesz usunąć ten zasobnik? Wszystkie dokumenty w nim zostaną zarchiwizowane.',
    documentArchived: 'Dokument zarchiwizowany',
    documentDeleted: 'Dokument usunięty',
    confirmDelete: 'Czy na pewno chcesz usunąć ten dokument?',
    confirmArchiveDocument: 'Czy na pewno chcesz zarchiwizować ten dokument?',
    viewOriginal: 'Zobacz oryginał',
    delete: 'Usuń',

    // Formularz zasobnika
    bucketName: 'Nazwa zasobnika',
    bucketNamePlaceholder: 'np. Dokumentacja inżynierska',
    bucketDescription: 'Opis',
    bucketDescriptionPlaceholder: 'Jakie dokumenty powinny się tu znajdować?',

    // Formularz notatki
    noteTitle: 'Tytuł',
    noteTitlePlaceholder: 'Tytuł notatki',
    noteContent: 'Treść',
    noteContentPlaceholder: 'Napisz notatkę w formacie Markdown...',

    // Wgrywanie
    dropzoneTitle: 'Upuść pliki tutaj',
    dropzoneDescription:
      'lub kliknij, aby wybrać. Obsługiwane formaty: PDF, DOCX, PPTX, XLSX, TXT, MD, HTML i obrazy.',
    uploading: 'Wgrywanie...',
    documentTitle: 'Tytuł dokumentu',
    documentTitlePlaceholder: 'Opcjonalnie — domyślnie nazwa pliku',

    // Status
    status: {
      pending: 'Oczekuje',
      processing: 'Przetwarzanie',
      ready: 'Gotowy',
      error: 'Błąd',
    },

    // Typ źródła
    sourceType: {
      upload: 'Wgrany',
      note: 'Notatka',
      email: 'E-mail',
    },

    // Typ treści (klasyfikacja)
    contentType: {
      knowledge: 'Wiedza',
      record: 'Zapis',
      correspondence: 'Korespondencja',
    },

    // Informacje o dokumencie
    mimeType: 'Typ pliku',
    contentHash: 'Skrót treści',
    retrievalCount: 'Liczba pobrań',
    chunkCount: 'Fragmenty',
    createdAt: 'Utworzono',
    updatedAt: 'Zaktualizowano',
    reviewedAt: 'Sprawdzono',
    health: 'Kondycja',
    author: 'Autor',
    processingMessage: 'Dokument jest przetwarzany...',
    runAction: 'Uruchom akcję',
  },
};
