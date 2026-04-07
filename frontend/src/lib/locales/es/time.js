/**
 * Spanish (es) - Time tracking related translations
 * Latin American neutral Spanish
 * Includes: time, timeProject, timeProjectCategory sections
 */

export default {
  time: {
    title: 'Seguimiento de tiempo',
    subtitle: 'Registrar tiempo empleado en elementos de trabajo',
    logTime: 'Registrar tiempo',
    editTimeEntry: 'Editar entrada de tiempo',
    updateEntry: 'Actualizar entrada',
    timeSpent: 'Tiempo empleado',
    remaining: 'Restante',
    estimate: 'Estimación',
    originalEstimate: 'Estimación original',
    hours: 'horas',
    minutes: 'minutos',
    days: 'días',
    weeks: 'semanas',
    startTimer: 'Iniciar temporizador',
    stopTimer: 'Detener temporizador',
    pauseTimer: 'Pausar temporizador',
    resumeTimer: 'Reanudar temporizador',
    timeLogged: 'Tiempo registrado correctamente',
    timeTrackingProject: 'Proyecto de seguimiento de tiempo',
    workItemOptional: 'Elemento de trabajo (Opcional)',
    whatDidYouWorkOn: '¿En qué trabajaste?',
    start: 'Inicio',
    end: 'Fin',
    duration: 'Duración',
    durationHelperText:
      'Ingresa hora de inicio + duración (2h) para calcular la hora de fin automáticamente, o ingresa hora de inicio + fin para calcular la duración automáticamente. Formatos de tiempo: 1h, 30m, 1h30m, 2h15m, 1d (=8h)',

    // Timesheet
    timesheet: {
      title: 'Hoja de tiempo',
      subtitle: 'Resumen semanal de hoja de tiempo',
      total: 'Total',
      noEntries: 'No hay entradas de tiempo esta semana. Agrega un proyecto para comenzar.',
      projectItem: 'Proyecto / Elemento de trabajo',
      addProject: 'Agregar proyecto a la hoja de tiempo...',
      removeProject: 'Eliminar de la hoja de tiempo',
      showWeekends: 'Fines de semana',
    },

    // Onboarding
    onboarding: {
      title: 'Configurar seguimiento de tiempo',
      subtitle: 'Vamos a crear tu primera organización de cliente y proyecto para comenzar',
      setupProgress: 'Progreso de configuración',
      stepOf: 'Paso {current} de {total}',
      createCustomerTitle: 'Crea tu primera organización de cliente',
      createCustomerDescription:
        'Una organización de cliente representa la empresa o entidad para la que estás trabajando. Puede ser una organización cliente, tu empleador o tu propia empresa.',
      createProjectTitle: 'Crea tu primer proyecto',
      createProjectDescription:
        'Los proyectos ayudan a organizar tu trabajo dentro de un cliente. Puedes registrar tiempo en proyectos específicos.',
      organizationNameRequired: 'El nombre de la organización es obligatorio',
      projectNameRequired: 'El nombre del proyecto es obligatorio',
      failedToCreateCustomer:
        'Error al crear la organización de cliente. Por favor, inténtalo de nuevo.',
      failedToCreateProject: 'Error al crear el proyecto. Por favor, inténtalo de nuevo.',
      customerCreatedSuccess: '¡Organización de cliente "{name}" creada exitosamente!',
      organizationNamePlaceholder: 'p. ej., Corporación Acme, TechStart Inc, Trabajo freelance',
      emailPlaceholder: 'facturacion@cliente.com',
      contactPersonPlaceholder: 'Juan Pérez',
      projectNamePlaceholder: 'p. ej., Desarrollo web, Consultoría, Trabajo general',
      projectDescriptionPlaceholder: 'Descripción breve del proyecto...',
      hourlyRateHint: 'Puedes configurar esto después si no estás seguro',
      skipForNow: 'Omitir por ahora',
      completeSetup: 'Completar configuración',
    },

    // Categories
    categories: {
      title: 'Categorías de proyectos',
      subtitle: 'Organiza proyectos en categorías para una mejor gestión',
      newCategory: 'Nueva categoría',
      noCategories: 'Aún no hay categorías',
      createFirstHint: 'Crea tu primera categoría para organizar proyectos',
      failedToSave: 'Error al guardar la categoría',
      failedToDelete: 'Error al eliminar la categoría',
      confirmDelete: '¿Estás seguro de que deseas eliminar "{name}"?',
    },

    // Reports
    reports: {
      title: 'Reportes',
      subtitle: 'Analiza tus datos de seguimiento de tiempo y exporta reportes',
      exportCSV: 'Exportar CSV',
      exportPDF: 'Exportar PDF',
      filters: 'Filtros',
      customer: 'Cliente',
      allCustomers: 'Todos los clientes',
      allProjects: 'Todos los proyectos',
      descriptionFilter: 'Filtro de descripción',
      searchDescriptions: 'Buscar descripciones...',
      fromDate: 'Desde',
      toDate: 'Hasta',
      applyFilters: 'Aplicar filtros',
      totalHours: 'Total de horas',
      totalEntries: 'Total de entradas',
      averagePerDay: 'Promedio por día',
      topProject: 'Proyecto principal',
      loadingReports: 'Cargando reportes...',
      noEntriesFound: 'No se encontraron entradas de tiempo para los filtros seleccionados.',
      totalTime: 'Tiempo total',
      entriesShown: '{count} entradas mostradas',
      // Project reporting
      projectReports: 'Reportes de proyectos',
      personal: 'Personal',
      project: 'Proyecto',
      selectProject: 'Selecciona un proyecto',
      budget: 'Presupuesto',
      budgetUsage: 'Uso del presupuesto',
      contributors: 'Colaboradores',
      memberBreakdown: 'Desglose del equipo',
      member: 'Miembro',
      hoursLogged: 'Horas registradas',
      entries: 'Entradas',
      avgPerDay: 'Prom/Día',
      dailyHours: 'Horas diarias',
      noBudgetSet: 'Sin presupuesto definido',
      noProjectSelected: 'Selecciona un proyecto para ver su reporte',
    },

    // Timer
    timer: {
      goToWorkItem: 'Ir al elemento de trabajo: {title}',
      expandTimer: 'Expandir temporizador',
      collapseTimer: 'Contraer temporizador',
      project: 'Proyecto',
      workspace: 'Espacio de trabajo',
    },

    // Projects
    projects: {
      title: 'Proyectos',
      subtitle:
        'Administrar proyectos globales para seguimiento de tiempo entre espacios de trabajo',
      addProject: 'Agregar proyecto',
      projectsTab: 'Proyectos',
      categoriesTab: 'Categorías',
      searchProjects: 'Buscar proyectos...',
      allCategories: 'Todas las categorías',
      allStatuses: 'Todos los estados',
      statusCount: '{count} estados',
      noProjects:
        'No se encontraron proyectos. Crea tu primer proyecto para comenzar a registrar tiempo.',
      noProjectsInCategory: 'No hay proyectos en esta categoría.',
      failedToSave: 'Error al guardar el proyecto',
      deleteProject: 'Eliminar proyecto',
      confirmDelete: '¿Estás seguro de que deseas eliminar "{name}"?',
      unknownCustomer: 'Cliente desconocido',
      project: 'Proyecto',
      customer: 'Cliente',
      rate: 'Tarifa',
      projectName: 'Nombre del proyecto',
      descriptionOptional: 'Descripción (Opcional)',
      hourlyRateOptional: 'Tarifa por hora (Opcional)',
    },

    // Calendar
    calendar: {
      title: 'Calendario semanal',
      itemCount: '{count} elementos',
      exportWeekToICS: 'Exportar semana a ICS',
      myWorkItems: 'Mis elementos de trabajo',
      dragToSchedule: 'Arrastra elementos para programarlos',
      noWorkItems: 'No hay elementos de trabajo asignados',
      workItemsWillAppear: 'Los elementos de trabajo aparecerán aquí cuando te sean asignados',
      itemsCompleted: '{completed} de {total} elementos completados',
      previousWeek: 'Semana anterior',
      thisWeek: 'esta semana',
      nextWeek: 'Semana siguiente',
      newTaskPlaceholder: 'Título de nueva tarea...',
      failedToCreateTask: 'Error al crear la tarea',
    },

    // Time Entry
    entry: {
      title: 'Entrada de tiempo',
      subtitle: 'Registra tus horas de trabajo y administra entradas de tiempo',
      addTimeEntry: 'Agregar una nueva entrada de tiempo',
      failedToSave: 'Error al guardar la entrada de tiempo. Por favor, verifica tu entrada.',
      confirmDelete: '¿Estás seguro de que deseas eliminar esta entrada de tiempo?',
      needProjects: 'Necesitas crear proyectos activos antes de registrar tiempo.',
      goToProjects: 'Ir a Proyectos',
      startSetupWizard: 'iniciar el asistente de configuración',
      applyFiltersTitle: 'Aplicar los filtros seleccionados a la lista de entradas de tiempo',
      clearFiltersTitle: 'Borrar todos los filtros y mostrar todas las entradas de tiempo',
      noEntries:
        'No se encontraron entradas de tiempo. Registra tu primera entrada de tiempo para comenzar.',
      clickToView: 'Clic para ver {key}-{number}',
      budgetExceeded: '- presupuesto excedido',
    },

    // Organizations (formerly Customers)
    organizations: {
      title: 'Organizaciones',
      subtitle: 'Administra tus organizaciones de clientes',
      addOrganization: 'Agregar organización',
      noOrganizations:
        'No se encontraron organizaciones. Crea tu primera organización para comenzar.',
      name: 'Nombre de la organización',
      emailOptional: 'Correo electrónico (Opcional)',
      contactPersonOptional: 'Persona de contacto (Opcional)',
      failedToSave: 'Error al guardar la organización',
      deleteOrganization: 'Eliminar organización',
      confirmDelete: '¿Estás seguro de que deseas eliminar "{name}"?',
    },

    // Permissions
    permissions: {
      title: 'Permisos del proyecto',
      managePermissions: 'Administrar permisos',
      managers: 'Administradores',
      members: 'Miembros',
      addManager: 'Agregar administrador',
      addMember: 'Agregar miembro',
      removeManager: 'Eliminar administrador',
      removeMember: 'Eliminar miembro',
      noManagers: 'No hay administradores asignados',
      noManagersHint:
        'Cuando no hay administradores asignados, cualquiera puede administrar este proyecto',
      noMembers: 'No hay miembros asignados',
      noMembersHint:
        'Cuando no hay miembros asignados, cualquiera puede registrar tiempo en este proyecto',
      grantedAt: 'Agregado',
      confirmRemove: '¿Estás seguro de que deseas eliminar a {name}?',
      failedToAdd: 'Error al agregar',
      failedToRemove: 'Error al eliminar',
      managersNote: 'Administradores:',
      managersNoteText:
        'Pueden editar la configuración del proyecto, administrar miembros y ver todas las entradas de tiempo de este proyecto.',
      membersNote: 'Miembros:',
      membersNoteText:
        'Pueden registrar tiempo en este proyecto y ver sus propias entradas de tiempo.',
    },
  },

  timeProject: {
    editProject: 'Editar proyecto',
    newProject: 'Nuevo proyecto',
    projectName: 'Nombre del proyecto',
    status: 'Estado',
    customerOptional: 'Cliente (Opcional)',
    none: 'Ninguno',
    categoryOptional: 'Categoría (Opcional)',
    hourlyRate: 'Tarifa por hora ($)',
    maxHours: 'Máximo de horas',
    maxHoursPlaceholder: 'Sin límite',
    maxHoursHint: 'Presupuesto opcional para reportes',
    projectColor: 'Color del proyecto',
    updateProject: 'Actualizar proyecto',
    createProject: 'Crear proyecto',
  },

  timeProjectCategory: {
    editCategory: 'Editar categoría',
    newCategory: 'Nueva categoría',
    categoryName: 'Nombre de la categoría',
    categoryNamePlaceholder: 'Desarrollo, Marketing, Operaciones...',
    optionalDescription: 'Descripción opcional...',
    updateCategory: 'Actualizar categoría',
    createCategory: 'Crear categoría',
  },
};
