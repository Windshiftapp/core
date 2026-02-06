/**
 * Spanish (es) - UI-related translations
 * Contains: pickers, editors, dialogs, components, aria, layout, widgets, footer
 */

export default {
  pickers: {},

  editors: {},

  dialogs: {
    cancel: 'Cancelar',
    confirm: 'Confirmar',
    save: 'Guardar',
    close: 'Cerrar',
    delete: 'Eliminar',
    update: 'Actualizar',
    // Confirmation messages for confirm() dialogs
    confirmations: {
      deleteItem: 'Esta seguro de que desea eliminar "{name}"? Esta accion no se puede deshacer.',
      deleteSection: 'Esta seguro de que desea eliminar esta seccion?',
      discardChanges: 'Tiene cambios sin guardar. Esta seguro de que desea cancelar?',
      dismissAllNotifications:
        'Esta seguro de que desea descartar todas las notificaciones? Esta accion no se puede deshacer.',
      removeAvatar: 'Esta seguro de que desea eliminar su foto de perfil?',
      revokeCalendarFeed:
        'Esta seguro de que desea revocar la URL de su feed de calendario? Los calendarios que usen esta URL dejaran de sincronizarse.',
      deleteTheme: 'Esta seguro de que desea eliminar este tema? Esta accion no se puede deshacer.',
      resetBoardConfig:
        'Esta seguro de que desea restablecer la configuracion del tablero por defecto? Esto eliminara su configuracion personalizada.',
      deleteCustomField:
        'Esta seguro de que desea eliminar el campo personalizado "{name}"? Se eliminara de todos los proyectos.',
      deleteLinkType:
        'Esta seguro de que desea eliminar este tipo de enlace? Tambien se eliminaran todos los enlaces de este tipo.',
      deleteAsset: 'Esta seguro de que desea eliminar este activo?',
      deleteAssetSet:
        'Esta seguro de que desea eliminar este conjunto de activos? Se eliminaran todos los activos, tipos y categorias dentro de el.',
      deleteAssetType:
        'Esta seguro de que desea eliminar este tipo de activo? Los activos que usen este tipo ya no tendran un tipo asignado.',
      deleteCategory:
        'Esta seguro de que desea eliminar esta categoria? Las subcategorias se moveran a la categoria principal.',
      revokeRole: 'Esta seguro de que desea revocar este rol?',
      quitApplication: 'Esta seguro de que desea salir de la aplicacion? El servidor se apagara.',
      deleteConnection:
        'Esta seguro de que desea eliminar esta conexion? Esta accion no se puede deshacer.',
      deleteWidget: 'Eliminar esta seccion? Todos los widgets en esta seccion seran eliminados.',
    },
    // Alert messages for alert() dialogs
    alerts: {
      nameRequired: 'El nombre es requerido',
      pleaseSelectImage: 'Por favor seleccione un archivo de imagen',
      timerAlreadyRunning:
        'Ya hay un temporizador en ejecucion. Por favor detengalo antes de iniciar uno nuevo.',
      noTimerRunning: 'No hay ningun temporizador en ejecucion actualmente.',
      timerSyncing: 'El temporizador se esta sincronizando. Por favor espere e intente de nuevo.',
      startTimerFromItem:
        'Por favor inicie un temporizador desde un elemento de trabajo para proporcionar contexto.',
      cannotDeleteDefaultScreen:
        'No se puede eliminar la pantalla por defecto. Esta pantalla es requerida para espacios de trabajo sin conjunto de configuracion.',
      applicationShuttingDown: 'La aplicacion se esta cerrando...',
      pdfExportComingSoon: 'Exportacion a PDF para vista de bloques de tiempo disponible pronto',
      configUpdatedSuccess:
        'Conjunto de configuracion actualizado exitosamente. Todos los elementos de trabajo ya estan usando estados del nuevo flujo de trabajo.',
      failedToSave: 'Error al guardar: {error}',
      failedToDelete: 'Error al eliminar: {error}',
      failedToUpdate: 'Error al actualizar: {error}',
      failedToLoad: 'Error al cargar: {error}',
      failedToCreate: 'Error al crear: {error}',
      failedToUpload: 'Error al subir: {error}',
      failedToGeneratePdf: 'Error al generar PDF. Por favor intente de nuevo.',
      failedToApplyConfig: 'Error al aplicar cambio de configuracion: {error}',
      failedToAddManager: 'Error al agregar administrador: {error}',
      failedToRemoveManager: 'Error al eliminar administrador: {error}',
      failedToSaveWorkspace:
        'Error al guardar proyecto. Por favor verifique su entrada e intente de nuevo.',
      failedToResetConfig: 'Error al restablecer configuracion: {error}',
      failedToToggleStatus: 'Error al cambiar estado del tipo de enlace: {error}',
      failedToAssignRole: 'Error al asignar rol: {error}',
      failedToRevokeRole: 'Error al revocar rol: {error}',
      failedToUpdateRole: 'Error al actualizar rol de todos: {error}',
      failedToLoadFields: 'Error al cargar campos: {error}',
      failedToSaveFields: 'Error al guardar asignaciones de campos: {error}',
      errorAddingTestCase: 'Error al agregar caso de prueba: {error}',
      failedToCreateLabel: 'Error al crear etiqueta: {error}',
      failedToSaveLayout: 'Error al guardar cambios de diseno',
    },
  },

  components: {},

  aria: {
    close: 'Cerrar',
    dragToReorder: 'Arrastrar para reordenar',
    refresh: 'Actualizar',
    removeField: 'Quitar campo',
    removeFromSection: 'Quitar de la seccion',
    addNewStep: 'Agregar nuevo paso',
    removeCurrentStep: 'Quitar paso actual',
    dismissNotification: 'Descartar notificacion',
    mainNavigation: 'Navegacion principal',
    mentionUsers: 'Mencionar usuarios',
    notifications: 'Notificaciones',
    adminSettings: 'Configuracion de administrador',
    userMenu: 'Menu de usuario',
  },

  layout: {
    addSection: 'Agregar seccion',
    moveUp: 'Mover seccion arriba',
    moveDown: 'Mover seccion abajo',
    deleteSection: 'Eliminar seccion',
    editMode: 'Modo de edicion',
    editDisplaySettings: 'Editar configuracion de visualizacion',
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

  footer: {},
};
