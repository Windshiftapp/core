/**
 * Spanish (es) - UI-related translations
 * Latin American neutral Spanish
 * Contains: pickers, editors, dialogs, components, aria, layout, widgets, footer
 */

export default {
  pickers: {
    // General
    select: 'Seleccionar',
    search: 'Buscar',
    options: 'Opciones',
    clearSelection: 'Limpiar selección',
    noResultsFor: 'Sin resultados para "{query}"',
    createItem: 'Crear "{value}"',
    noItemsFound: 'No se encontraron elementos',
    noItemsAvailable: 'No hay elementos disponibles',

    // Asset Picker
    selectAsset: 'Seleccionar activo',
    noTag: 'Sin etiqueta',
    showingOfTotal: 'Mostrando {shown} de {total} — escribe para buscar',

    // User/Assignee Picker
    selectUser: 'Seleccionar usuario',
    searchUsers: 'Buscar usuarios...',
    users: 'Usuarios',
    noUsersFound: 'No se encontraron usuarios',
    noUsersAvailable: 'No hay usuarios disponibles',
    assignTo: 'Asignar a',
    unassigned: 'Sin asignar',
    assignee: 'Asignado',
    user: 'Usuario',
    group: 'Grupo',
    searchUser: 'Buscar usuario...',
    searchGroup: 'Buscar grupo...',

    // Group Picker
    selectGroup: 'Seleccionar grupo',

    // Category Picker
    selectCategories: 'Seleccionar categorías',
    removeCategory: 'Quitar categoría',
    categoriesSelected: '{count} categorías seleccionadas',
    searchCategories: 'Buscar categorías...',
    noCategoriesFound: 'No se encontraron categorías',

    // Collection Picker
    selectCollections: 'Seleccionar colecciones',

    // Workspace Picker
    selectWorkspaces: 'Seleccionar espacios de trabajo',
    searchWorkspaces: 'Buscar espacios de trabajo...',
    noWorkspacesFound: 'No se encontraron espacios de trabajo',

    // Configuration Set Picker
    selectConfigurationSet: 'Seleccionar conjunto de configuración',
    searchConfigurationSets: 'Buscar conjuntos de configuración...',
    configurationSets: 'Conjuntos de configuración',
    defaultConfiguration: 'Configuración predeterminada',
    defaultConfigurationDescription: 'Usa la configuración predeterminada del espacio de trabajo',
    noConfigurationSetsFound: 'No se encontraron conjuntos de configuración',

    // Configuration Set Entity Picker
    entityAlreadyAssigned: '{label} ya está asignado',
    itemType: 'Tipo de elemento',
    priorities: 'Prioridades',
    itemTypes: 'Tipos de elemento',
    level: 'Nivel {level}',
    assigned: 'Asignado',
    noEntitiesAssigned: 'No hay {entities} asignados',
    available: 'Disponible',
    noEntitiesMatchSearch: 'Ningún {entities} coincide con tu búsqueda',
    allEntitiesAssigned: 'Todos los {entities} están asignados',
    inConfigSet: 'En conjunto de configuración',
    searchEntities: 'Buscar {entities}...',

    // Field Selector
    selectField: 'Seleccionar campo',
    searchFields: 'Buscar campos...',
    noFieldsFound: 'No se encontraron campos',
    customFields: 'Campos personalizados',
    custom: 'Personalizado',
    customFieldDesc: 'Campo personalizado',
    fieldTypes: {
      text: 'Texto',
      number: 'Número',
      date: 'Fecha',
      select: 'Selección',
      multiselect: 'Selección múltiple',
      checkbox: 'Casilla de verificación',
      url: 'URL',
      email: 'Correo electrónico',
      phone: 'Teléfono',
      textarea: 'Área de texto',
      textArea: 'Área de texto',
      user: 'Usuario',
      rating: 'Calificación',
      boolean: 'Booleano',
      reference: 'Referencia',
      identifier: 'Identificador',
    },
    fieldCategories: {
      basic: 'Campos básicos',
      dates: 'Campos de fecha',
      people: 'Personas',
      workflow: 'Flujo de trabajo',
      custom: 'Campos personalizados',
    },
    fields: {
      title: { name: 'Título', description: 'Título del elemento' },
      description: { name: 'Descripción', description: 'Descripción del elemento' },
      status: { name: 'Estado', description: 'Estado actual' },
      priority: { name: 'Prioridad', description: 'Nivel de prioridad' },
      type: { name: 'Tipo', description: 'Tipo de elemento' },
      assignee: { name: 'Asignado', description: 'Usuario asignado' },
      reporter: { name: 'Reportador', description: 'Quién reportó el elemento' },
      createdAt: { name: 'Fecha de creación', description: 'Cuándo se creó el elemento' },
      updatedAt: {
        name: 'Fecha de actualización',
        description: 'Cuándo se actualizó el elemento por última vez',
      },
      dueDate: { name: 'Fecha de vencimiento', description: 'Cuándo vence el elemento' },
      startDate: { name: 'Fecha de inicio', description: 'Cuándo comienza el trabajo' },
      estimate: { name: 'Estimación', description: 'Esfuerzo estimado' },
      labels: { name: 'Etiquetas', description: 'Etiquetas del elemento' },
      sprint: { name: 'Sprint', description: 'Sprint asociado' },
      milestone: { name: 'Hito', description: 'Hito objetivo' },
      parent: { name: 'Padre', description: 'Elemento padre' },
      children: { name: 'Hijos', description: 'Elementos hijos' },
      links: { name: 'Enlaces', description: 'Elementos relacionados' },
      attachments: { name: 'Adjuntos', description: 'Archivos adjuntos' },
      comments: { name: 'Comentarios', description: 'Comentarios de discusión' },
      watchers: { name: 'Observadores', description: 'Usuarios que observan este elemento' },
    },

    // Icon Selector
    iconAndColor: 'Icono y color',
    searchIcons: 'Buscar iconos...',
    icons: 'Iconos',
    colors: 'Colores',
    icon: 'Icono',
    color: 'Color',

    // Label Combobox
    allLabels: 'Todas las etiquetas',
    selectLabels: 'Seleccionar etiquetas',
    noLabelsFoundFor: 'No se encontraron etiquetas para "{query}"',

    // Mention Picker
    mentionUsers: 'Mencionar usuarios',
    searching: 'Buscando...',
    noNotificationPersonalTask: 'Las tareas personales no envían notificaciones',

    // Milestone Combobox
    selectMilestone: 'Seleccionar hito',
    noMilestone: 'Sin hito',
    milestones: 'Hitos',
    noMilestonesFound: 'No se encontraron hitos',

    // Priority Picker
    selectPriority: 'Seleccionar prioridad',
    noPriority: 'Sin prioridad',
    loadingPriorities: 'Cargando prioridades...',
    noPrioritiesConfigured: 'No hay prioridades configuradas',

    // Project Picker
    selectProject: 'Seleccionar proyecto',

    // Repository Selector
    linkRepositories: 'Vincular repositorios',
    selectRepositoriesFrom: 'Seleccionar repositorios de {provider}',
    searchRepositories: 'Buscar repositorios...',
    loadingRepositories: 'Cargando repositorios...',
    noRepositoriesMatchSearch: 'Ningún repositorio coincide con tu búsqueda',
    noRepositoriesAvailable: 'No hay repositorios disponibles',
    alreadyLinked: 'Ya vinculado',
    linkSelected: 'Vincular seleccionados',
    linking: 'Vinculando...',
    repositoriesSelected: '{count} seleccionados',

    // Role Picker
    selectRole: 'Seleccionar rol',

    // Screen Picker
    selectScreen: 'Seleccionar pantalla',

    // Test Case Picker
    searchTestCases: 'Buscar casos de prueba...',

    // Workflow Picker
    selectWorkflow: 'Seleccionar flujo de trabajo',
  },

  editors: {
    enterText: 'Ingresa texto...',
    selectDate: 'Selecciona una fecha...',
    clickToChangeColor: 'Clic para cambiar el color',
    saveEnter: 'Guardar (Enter)',
    cancelEscape: 'Cancelar (Escape)',
    availableFields: 'Campos disponibles',
    selectedFields: 'Campos seleccionados',
    dragFieldsToAdd: 'Arrastra campos para agregarlos',
    dragToReorderOrDrop: 'Arrastra para reordenar o suelta campos aquí',
    dropFieldsHere: 'Suelta campos aquí para configurar',
    noFieldsMatchSearch: 'Ningún campo coincide con tu búsqueda',
    noFieldsAvailable: 'No hay campos disponibles',
    allFieldsAdded: 'Todos los campos disponibles han sido agregados',
    bold: 'Negrita (Ctrl+B)',
    italic: 'Cursiva (Ctrl+I)',
    strikethrough: 'Tachado',
    inlineCode: 'Código en línea',
    bulletList: 'Lista con viñetas',
    numberedList: 'Lista numerada',
    insertImage: 'Insertar imagen',
    userNotFound: 'Usuario no encontrado',
  },

  dialogs: {
    cancel: 'Cancelar',
    confirm: 'Confirmar',
    save: 'Guardar',
    close: 'Cerrar',
    delete: 'Eliminar',
    update: 'Actualizar',
    // Confirmation messages for confirm() dialogs
    confirmations: {
      deleteItem:
        '¿Estás seguro de que deseas eliminar "{name}"? Esta acción no se puede deshacer.',
      deleteSection: '¿Estás seguro de que deseas eliminar esta sección?',
      discardChanges: 'Tienes cambios sin guardar. ¿Estás seguro de que deseas cancelar?',
      dismissAllNotifications:
        '¿Estás seguro de que deseas descartar todas las notificaciones? Esta acción no se puede deshacer.',
      removeAvatar: '¿Estás seguro de que deseas eliminar tu foto de perfil?',
      revokeCalendarFeed:
        '¿Estás seguro de que deseas revocar la URL de tu feed de calendario? Los calendarios que usen esta URL dejarán de sincronizarse.',
      deleteTheme:
        '¿Estás seguro de que deseas eliminar este tema? Esta acción no se puede deshacer.',
      resetBoardConfig:
        '¿Estás seguro de que deseas restablecer la configuración del tablero por defecto? Esto eliminará tu configuración personalizada.',
      deleteCustomField:
        '¿Estás seguro de que deseas eliminar el campo personalizado "{name}"? Se eliminará de todos los proyectos.',
      deleteLinkType:
        '¿Estás seguro de que deseas eliminar este tipo de enlace? También se eliminarán todos los enlaces de este tipo.',
      deleteAsset: '¿Estás seguro de que deseas eliminar este activo?',
      deleteAssetSet:
        '¿Estás seguro de que deseas eliminar este conjunto de activos? Se eliminarán todos los activos, tipos y categorías dentro de él.',
      deleteAssetType:
        '¿Estás seguro de que deseas eliminar este tipo de activo? Los activos que usen este tipo ya no tendrán un tipo asignado.',
      deleteCategory:
        '¿Estás seguro de que deseas eliminar esta categoría? Las subcategorías se moverán a la categoría principal.',
      revokeRole: '¿Estás seguro de que deseas revocar este rol?',
      quitApplication:
        '¿Estás seguro de que deseas salir de la aplicación? El servidor se apagará.',
      deleteConnection:
        '¿Estás seguro de que deseas eliminar esta conexión? Esta acción no se puede deshacer.',
      deleteWidget: '¿Eliminar esta sección? Todos los widgets en esta sección serán eliminados.',
      deleteScreen:
        '¿Estás seguro de que deseas eliminar la pantalla "{name}"? Esto afectará a todos los espacios de trabajo que usen esta pantalla.',
    },
    // Alert messages for alert() dialogs
    alerts: {
      nameRequired: 'El nombre es requerido',
      pleaseSelectImage: 'Por favor selecciona un archivo de imagen',
      timerAlreadyRunning:
        'Ya hay un temporizador en ejecución. Por favor detenlo antes de iniciar uno nuevo.',
      noTimerRunning: 'No hay ningún temporizador en ejecución actualmente.',
      timerSyncing: 'El temporizador se está sincronizando. Por favor espera e intenta de nuevo.',
      startTimerFromItem:
        'Por favor inicia un temporizador desde un elemento de trabajo para proporcionar contexto.',
      cannotDeleteDefaultScreen:
        'No se puede eliminar la pantalla por defecto. Esta pantalla es requerida para espacios de trabajo sin conjunto de configuración.',
      applicationShuttingDown: 'La aplicación se está cerrando...',
      pdfExportComingSoon:
        'Exportación a PDF para vista de bloques de tiempo disponible próximamente',
      configUpdatedSuccess:
        'Conjunto de configuración actualizado exitosamente. Todos los elementos de trabajo ya están usando estados del nuevo flujo de trabajo.',
      failedToSave: 'Error al guardar: {error}',
      failedToDelete: 'Error al eliminar: {error}',
      failedToUpdate: 'Error al actualizar: {error}',
      failedToLoad: 'Error al cargar: {error}',
      failedToCreate: 'Error al crear: {error}',
      failedToUpload: 'Error al subir: {error}',
      failedToGeneratePdf: 'Error al generar PDF. Por favor intenta de nuevo.',
      failedToApplyConfig: 'Error al aplicar cambio de configuración: {error}',
      failedToAddManager: 'Error al agregar administrador: {error}',
      failedToRemoveManager: 'Error al eliminar administrador: {error}',
      failedToSaveWorkspace:
        'Error al guardar proyecto. Por favor verifica tu entrada e intenta de nuevo.',
      failedToResetConfig: 'Error al restablecer configuración: {error}',
      failedToToggleStatus: 'Error al cambiar estado del tipo de enlace: {error}',
      failedToAssignRole: 'Error al asignar rol: {error}',
      failedToRevokeRole: 'Error al revocar rol: {error}',
      failedToUpdateRole: 'Error al actualizar rol de todos: {error}',
      failedToLoadFields: 'Error al cargar campos: {error}',
      failedToSaveFields: 'Error al guardar asignaciones de campos: {error}',
      errorAddingTestCase: 'Error al agregar caso de prueba: {error}',
      failedToCreateLabel: 'Error al crear etiqueta: {error}',
      failedToSaveLayout: 'Error al guardar cambios de diseño',
      statusInUseByTransitions:
        'No se puede eliminar "{name}" porque está siendo usada en {count} transición(es) del flujo de trabajo. Para eliminar este estado, ve a Gestión de flujos de trabajo, elimina todas las transiciones que usen este estado e intenta eliminarlo de nuevo.',
    },
  },

  components: {
    // Avatar component
    avatar: {
      defaultAlt: 'Avatar',
    },

    // DataTable component
    dataTable: {
      showingRange: 'Mostrando {start}–{end} de {total}',
    },

    // Diagram components
    diagram: {
      loading: 'Cargando diagramas...',
      loadError: 'Error al cargar diagramas',
      deleteError: 'Error al eliminar diagrama',
      confirmDelete: '¿Estás seguro de que deseas eliminar este diagrama?',
      edit: 'Editar diagrama',
      untitled: 'Diagrama sin título',
      namePlaceholder: 'Nombre del diagrama',
      nameRequired: 'Por favor ingresa un nombre para el diagrama',
      saveError: 'Error al guardar diagrama',
      unsavedChanges: 'Cambios sin guardar',
      unsavedChangesConfirm: 'Tienes cambios sin guardar. ¿Estás seguro de que deseas cerrar?',
    },

    // ErrorState component
    errorState: {
      title: 'Algo salió mal',
    },

    // Pagination component
    pagination: {
      showingRange: 'Mostrando {start}-{end} de {total}',
      limitedTo: 'limitado a {max} elementos',
      itemsPerPage: 'Elementos por página:',
      previousPage: 'Página anterior',
      nextPage: 'Página siguiente',
      goToPage: 'Ir a la página {page}',
      pageOf: 'Página {current} de {total}',
    },

    // UserAvatar component
    userAvatar: {
      myWorkspace: 'Mi espacio de trabajo',
      myWorkspaceSubtitle: 'Espacio de trabajo personal para tareas y notas',
      profileSubtitle: 'Administra tu perfil y configuración',
      security: 'Seguridad',
      securitySubtitle: 'Administra contraseñas, 2FA y tokens de API',
      themeTitle: 'Tema: {mode}',
      themeCycle: 'Clic para alternar: Claro → Oscuro → Sistema',
      themeLight: 'Claro',
      themeDark: 'Oscuro',
      themeSystem: 'Sistema',
    },
  },

  aria: {
    close: 'Cerrar',
    dragToReorder: 'Arrastrar para reordenar',
    refresh: 'Actualizar',
    removeField: 'Quitar campo',
    removeFromSection: 'Quitar de la sección',
    addNewStep: 'Agregar nuevo paso',
    removeCurrentStep: 'Quitar paso actual',
    dismissNotification: 'Descartar notificación',
    mainNavigation: 'Navegación principal',
    mentionUsers: 'Mencionar usuarios',
    notifications: 'Notificaciones',
    adminSettings: 'Configuración de administrador',
    userMenu: 'Menú de usuario',
    clearSearch: 'Limpiar búsqueda',
  },

  layout: {
    addSection: 'Agregar sección',
    moveUp: 'Mover sección arriba',
    moveDown: 'Mover sección abajo',
    deleteSection: 'Eliminar sección',
    editMode: 'Modo de edición',
    editDisplaySettings: 'Editar configuración de visualización',
    items: 'elementos',
  },

  widgets: {
    removeWidget: 'Quitar widget',
    narrowWidth: 'Estrecho (1/3 de ancho)',
    mediumWidth: 'Medio (2/3 de ancho)',
    fullWidth: 'Ancho completo',
    chart: {
      items: 'elementos',
    },
    completionChart: {
      emptyMessage: 'No hay datos de finalización disponibles',
    },
    createdChart: {
      emptyMessage: 'No hay datos de creación disponibles',
    },
    milestoneProgress: {
      emptyTitle: 'Sin hitos',
      emptySubtitle: 'Crea hitos para seguir el progreso',
      due: 'Vence',
      done: 'completados',
      item: 'elemento',
      items: 'elementos',
      noItems: 'Sin elementos',
      noStatus: 'Sin estado',
      activeMilestone: 'Activo',
      noCategorizedWork: 'Sin trabajo categorizado',
    },
  },

  footer: {
    platformName: 'Plataforma de gestión de trabajo Windshift',
    aboutWindshift: 'Acerca de Windshift',
    reportProblem: 'Reportar un problema',
  },
};
