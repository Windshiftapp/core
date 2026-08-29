export default {
  dashboard: {
    salutation: {
      withName: '¡{salutation}, {name}!',
      withoutName: '¡{salutation}!',
    },
    sections: {
      yourDay: {
        title: 'Tu día',
        subtitle: 'Un resumen rápido de lo que necesita tu atención',
      },
      work: {
        title: 'Trabajo',
        subtitle: 'Tu lista personal y los elementos que tienes asignados',
      },
      workspaces: {
        title: 'Espacios de trabajo',
        subtitle: 'Retoma el trabajo',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'Resumen diario',
        description: 'Resumen generado por IA de lo que te importa hoy',
      },
      yourActivity: {
        name: 'Tu actividad',
        description: 'Elementos que has visto, editado o comentado recientemente',
      },
      whatsNew: {
        name: 'Novedades',
        description: 'Últimas notificaciones y actualizaciones sin leer',
      },
      personalTasks: {
        name: 'Tareas personales',
        description: 'Elementos de tu lista personal de tareas',
      },
      savedSearch: {
        name: 'Búsqueda guardada',
        description: 'Muestra elementos de una colección guardada',
      },
      assignedToMe: {
        name: 'Asignados a mí',
        description: 'Elementos abiertos que tienes asignados en todos los espacios de trabajo',
      },
      watchedItems: {
        name: 'Elementos observados',
        description: 'Elementos que sigues',
      },
      upcomingMilestones: {
        name: 'Próximos hitos',
        description: 'Hitos cuya fecha objetivo se acerca',
      },
      recentWorkspaces: {
        name: 'Espacios de trabajo recientes',
        description: 'Espacios de trabajo que has visitado recientemente',
      },
      quickAccess: {
        name: 'Acceso rápido',
        description: 'Enlaces rápidos a los espacios de trabajo disponibles',
      },
    },
    customization: {
      widgets: 'Widgets',
      activity: {
        name: 'Actividad',
        description: 'Resúmenes, flujos de actividad y notificaciones',
      },
      work: {
        name: 'Trabajo',
        description: 'Elementos, hitos y tareas que tienes asignados',
      },
      navigation: {
        name: 'Navegación',
        description: 'Acceso rápido a los espacios de trabajo',
      },
      tipLabel: 'Consejo',
      tip: 'Arrastra widgets desde aquí a cualquier sección de tu panel.',
    },
    editor: {
      newSection: 'Nueva sección',
      deleteSectionConfirm: '¿Eliminar esta sección? Se quitarán todos los widgets que contiene.',
      doneEditing: 'Terminar edición',
      customize: 'Personalizar',
      editModeDescription: 'Modo de edición: añade, cambia el nombre, reordena o elimina secciones y widgets',
      addSection: 'Añadir sección',
      sectionLabel: 'Sección del panel',
      sectionTitlePlaceholder: 'Título de la sección',
      sectionSubtitlePlaceholder: 'Subtítulo (opcional)',
      renameSection: 'Cambiar nombre de la sección',
      deleteSection: 'Eliminar sección',
      unknownWidgetType: 'Tipo de widget desconocido: {type}',
      noWidgets: 'Esta sección aún no tiene widgets',
      addWidgetsHint: 'Selecciona Personalizar para añadir widgets',
      noSections: 'No hay secciones configuradas',
      addSectionsHint: 'Selecciona Editar para añadir secciones al panel',
    },
    states: {
      assignedLoadError: 'No se pudieron cargar los elementos que tienes asignados',
      assignedEmpty: 'No tienes nada asignado en este momento',
      personalTasksLoadError: 'No se pudieron cargar tus tareas personales',
      personalTasksEmpty: 'Tu lista personal de tareas está vacía',
      dailyBriefingUnavailable: 'Tu resumen diario no está disponible en este momento. Necesita una integración de IA. Si acabas de configurar una, vuelve a intentarlo en unos instantes.',
      updatedAt: 'Actualizado {time}',
      priorityLabel: 'Prioridad: {priority}',
      noWorkspaces: 'Aún no hay espacios de trabajo',
      createWorkspace: 'Crear uno',
      workspaceAvatarAlt: 'Avatar de {name}',
      visited: 'visitado {time}',
      noUpcomingMilestones: 'No hay próximos hitos',
      milestoneProgress: '{done} de {total} completados',
      daysOverdue: '{days} días de retraso',
      daysLeft: 'Quedan {days} días',
      watchedItemsEmpty: 'No estás observando ningún elemento',
      workspaceWithId: 'Espacio de trabajo {id}',
    },
  },
};
