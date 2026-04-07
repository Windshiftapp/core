/**
 * Spanish (es) - Navigation and general UI translations
 * Latin American neutral Spanish
 * Contains: nav, commandPalette, dashboard, search, about, onboarding
 */

export default {
  nav: {
    workspaces: 'Espacios de trabajo',
    collections: 'Colecciones',
    timeAndProjects: 'Tiempo y Proyectos',
    milestones: 'Hitos',
    iterations: 'Iteraciones',
    assets: 'Activos',
    channels: 'Canales',
    portalHub: 'Centro del Portal',
    customers: 'Clientes',
    create: 'Crear',
    search: 'Buscar',
    admin: 'Administración',
    notifications: 'Notificaciones',
    profile: 'Perfil',
    expand: 'Expandir',
    collapse: 'Contraer',
    manageWorkspaces: 'Administrar espacios de trabajo',
    manageWorkspacesSubtitle: 'Crear, editar y organizar espacios de trabajo',
    searchWorkspaces: 'Buscar espacios de trabajo...',
    noWorkspacesMatch: 'Ningún espacio de trabajo coincide con su búsqueda',
    noWorkspacesFound: 'No se encontraron espacios de trabajo',
    searchToFindMore: 'Busque para encontrar más espacios de trabajo...',
    loadingSearch: 'Cargando búsqueda...',
    loadingCreateForm: 'Cargando formulario de creación...',
    loading: 'Cargando...',
    retry: 'Reintentar',
    knowledgeBase: 'Base de conocimiento',
  },

  commandPalette: {
    searchPlaceholder: 'Buscar en todo...',
    noCommandsFound: 'No se encontraron comandos',
    toSelect: 'para seleccionar',
    toNavigate: 'para navegar',
    toClose: 'para cerrar',
    pressToOpen: 'Presione {shortcut} para abrir',
    advancedSearch: 'Búsqueda avanzada',
    context: 'Contexto',
    // Command labels and descriptions
    commands: {
      // Navigation
      workspaces: {
        label: 'Espacios de trabajo',
        description: 'Administrar espacios de trabajo de proyectos',
      },
      search: { label: 'Buscar', description: 'Buscar elementos de trabajo y contenido' },
      dashboard: { label: 'Panel', description: 'Ver analíticas e informes' },
      milestones: { label: 'Hitos', description: 'Seguimiento de hitos del proyecto' },
      channels: { label: 'Canales', description: 'Canales de comunicación y soporte' },
      collections: {
        label: 'Colecciones',
        description: 'Administrar colecciones y vistas de elementos de trabajo',
      },
      adminPanel: {
        label: 'Panel de administración',
        description: 'Administración del sistema y configuración',
      },

      // Test Management (labels aligned with workspace navigation)
      tests: { label: 'Pruebas', description: 'Administrar casos de prueba, planes y ejecución' },
      testCases: { label: 'Casos de prueba', description: 'Ver y administrar casos de prueba' },
      testPlans: { label: 'Planes de prueba', description: 'Ver y administrar planes de prueba' },
      testTemplates: {
        label: 'Plantillas',
        description: 'Ejecuciones de plantilla y pasos compartidos',
      },
      testRuns: {
        label: 'Ejecuciones de prueba',
        description: 'Ver y administrar ejecuciones de prueba',
      },
      testReports: { label: 'Informes', description: 'Ver informes de ejecución de pruebas' },
      createTestCase: {
        label: 'Crear caso de prueba',
        description: 'Crear un nuevo caso de prueba para aseguramiento de calidad',
      },
      createTestPlan: {
        label: 'Crear plan de prueba',
        description: 'Crear un nuevo plan de prueba con casos de prueba',
      },
      createTestRun: {
        label: 'Crear ejecución de prueba',
        description: 'Crear una nueva plantilla de ejecución de prueba',
      },

      // Time Management
      timeTracking: {
        label: 'Seguimiento de tiempo',
        description: 'Registrar y administrar entradas de tiempo de trabajo',
      },
      timeReports: {
        label: 'Informes de tiempo',
        description: 'Ver informes y analíticas de seguimiento de tiempo',
      },
      timeProjects: {
        label: 'Proyectos de tiempo',
        description: 'Administrar proyectos de seguimiento de tiempo',
      },
      logTime: {
        label: 'Registrar tiempo',
        description: 'Agregar rápidamente una nueva entrada de tiempo',
      },
      startTimer: {
        label: 'Iniciar temporizador',
        description: 'Iniciar el seguimiento de tiempo del trabajo actual',
      },
      stopTimer: {
        label: 'Detener temporizador',
        description: 'Detener el seguimiento de tiempo actual',
      },

      // Create Commands
      createWorkItem: {
        label: 'Crear elemento de trabajo',
        description: 'Crear un nuevo elemento de trabajo o tarea',
      },
      createWorkspace: {
        label: 'Crear espacio de trabajo',
        description: 'Crear un nuevo espacio de trabajo de proyecto',
      },
      createMilestone: { label: 'Crear hito', description: 'Crear un nuevo hito de proyecto' },
      createCollection: {
        label: 'Crear colección',
        description: 'Crear una nueva colección de elementos de trabajo',
      },

      // Admin Commands
      adminCustomFields: {
        label: 'Campos personalizados',
        description: 'Administrar definiciones de campos personalizados',
      },
      adminScreens: {
        label: 'Gestión de pantallas',
        description: 'Configurar pantallas de formularios y diseños',
      },
      adminHierarchyLevels: {
        label: 'Niveles de jerarquía',
        description: 'Configurar niveles de jerarquía de elementos de trabajo',
      },
      adminItemTypes: {
        label: 'Tipos de elementos',
        description: 'Administrar tipos de elementos de trabajo con iconos y colores',
      },
      adminPriorities: {
        label: 'Prioridades',
        description: 'Configurar niveles de prioridad con iconos y colores',
      },
      adminConfigSets: {
        label: 'Conjuntos de configuración',
        description: 'Administrar conjuntos de configuración con flujos de trabajo y pantallas',
      },
      adminStatuses: {
        label: 'Estados',
        description: 'Administrar estados individuales de elementos de trabajo',
      },
      adminStatusCategories: {
        label: 'Categorías de estado',
        description: 'Administrar categorías de estado y colores',
      },
      adminWorkflows: {
        label: 'Constructor de flujos de trabajo',
        description: 'Diseñar y administrar transiciones de flujos de trabajo',
      },
      adminLinkTypes: {
        label: 'Tipos de enlace',
        description: 'Administrar tipos de enlace entre elementos de trabajo',
      },
      adminScmProviders: {
        label: 'Proveedores SCM',
        description: 'Configurar integraciones de GitHub, GitLab, Gitea y Bitbucket',
      },
      adminAttachments: {
        label: 'Adjuntos',
        description: 'Administrar configuración de adjuntos',
      },
      adminModules: {
        label: 'Configuración de módulos',
        description:
          'Habilitar o deshabilitar módulos de seguimiento de tiempo y gestión de pruebas',
      },
      adminThemes: {
        label: 'Configuración de temas',
        description: 'Administrar temas y apariencia de la aplicación',
      },
      adminUsers: {
        label: 'Gestión de usuarios',
        description: 'Administrar usuarios, roles y permisos',
      },
      adminGroups: {
        label: 'Gestión de grupos',
        description: 'Administrar grupos de usuarios y membresías',
      },
      adminPermissions: {
        label: 'Permisos',
        description: 'Administrar permisos de usuario y control de acceso',
      },
      adminPermissionSets: {
        label: 'Conjuntos de permisos',
        description: 'Administrar paquetes de permisos para conjuntos de configuración',
      },
      adminWorkspaceRoles: {
        label: 'Roles del espacio de trabajo',
        description: 'Ver roles del espacio de trabajo y sus permisos',
      },
      adminSso: {
        label: 'Inicio de sesión único',
        description: 'Configurar proveedores de identidad OIDC para SSO',
      },
      adminSecurity: {
        label: 'Configuración de seguridad',
        description: 'Configurar fuentes de calendario y ejecución de complementos',
      },
      adminSystemImport: {
        label: 'Importación del sistema',
        description: 'Importar datos de otros sistemas',
      },
      adminAssets: {
        label: 'Gestión de activos',
        description: 'Configurar tipos y categorías de activos',
      },
      adminWorkspaces: {
        label: 'Administración de espacios de trabajo',
        description: 'Administrar espacios de trabajo y configuración',
      },
      adminNotifications: {
        label: 'Configuración de notificaciones',
        description: 'Administrar configuraciones de notificaciones',
      },

      // System Commands
      quitApp: {
        label: 'Cerrar aplicación',
        description: 'Apagar el servidor de la aplicación de forma segura',
      },

      // Other
      portal: { label: 'Portal', description: 'Gestión del portal de clientes' },
      iterations: { label: 'Iteraciones', description: 'Planificación y seguimiento de sprints' },
      createItem: { label: 'Crear elemento', description: 'Crear un nuevo elemento de trabajo' },
      myProfile: { label: 'Mi perfil', description: 'Ver y editar su perfil' },
      notifications: { label: 'Notificaciones', description: 'Ver notificaciones' },
      about: { label: 'Acerca de', description: 'Acerca de esta aplicación' },
      assets: { label: 'Activos', description: 'Administrar conjuntos y elementos de activos' },
      customers: { label: 'Clientes', description: 'Administrar organizaciones de clientes' },

      // Dynamic workspace commands
      goToWorkspace: { label: 'Ir a {name}', description: 'Navegar al espacio de trabajo {name}' },
      workspaceOverview: {
        label: 'Vista general de {name}',
        description: 'Ver panel del espacio de trabajo con estadísticas y gráficos',
      },
    },
  },

  dashboard: {
    title: 'Panel',
    subtitle: 'Vista general de sus elementos de trabajo y proyectos',
    welcome: 'Bienvenido',
    overview: 'Vista general',
    recentActivity: 'Actividad reciente',
    recentItems: 'Elementos recientes',
    myTasks: 'Mis tareas',
    assignedToMe: 'Asignados a mí',
    openTasks: 'Tareas abiertas',
    overdueItems: 'Elementos vencidos',
    upcomingDeadlines: 'Próximas fechas límite',
    activeSprints: 'Sprints activos',
    inProgressMilestones: 'Hitos en progreso',
    noRecentActivity: 'Sin actividad reciente',
    recentItemsHint: 'Los elementos que actualice aparecerán aquí',
    noActiveSprints: 'Sin sprints activos',
    sprintTimelinesHint: 'Las líneas de tiempo del sprint aparecerán aquí',
    noUpcomingDeadlines: 'Sin fechas límite próximas',
    dueDatesHint: 'Los elementos con fechas de vencimiento aparecerán aquí',
    noOpenTasks: 'Sin tareas abiertas asignadas a usted',
    allCaughtUp: '¡Todo al día!',
    noOverdueItems: 'Sin elementos vencidos',
    loadingRecentItems: 'Cargando elementos recientes...',
    loadingTasks: 'Cargando sus tareas...',
    loadingOverdueItems: 'Cargando elementos vencidos...',
    workItemStatusOverview: 'Vista general del estado de elementos de trabajo',
    statusOpen: 'Abierto',
    statusInProgress: 'En progreso',
    statusClosed: 'Cerrado',
    quickActions: 'Acciones rápidas',
    createWorkItem: 'Crear elemento de trabajo',
    createWorkItemDesc: 'Comience a dar seguimiento a un nuevo elemento de trabajo',
    manageMilestones: 'Administrar hitos',
    manageMilestonesDesc: 'Ver y organizar hitos',
    manageWorkspaces: 'Administrar espacios de trabajo',
    manageWorkspacesDesc: 'Configurar espacios de trabajo',
    recentWorkItems: 'Elementos de trabajo recientes',
    whatsNew: 'Novedades',
    noNotifications: 'Sin notificaciones aún',
    yourActivity: 'Su actividad',
    viewed: 'Visto',
    edited: 'Editado',
    commented: 'Comentado',
    noRecentlyViewed: 'Sin elementos vistos recientemente',
    noRecentlyEdited: 'Sin elementos editados recientemente',
    noRecentlyCommented: 'Sin elementos comentados recientemente',
    quickAccess: 'Acceso rápido',
    quickAccessHint: 'Presione {shortcut} para abrir la paleta de comandos',
    upcomingMilestones: 'Próximos hitos',
    watching: 'Siguiendo',
    recentWorkspaces: 'Espacios de trabajo recientes',
    noRecentWorkspaces: 'Sin espacios de trabajo recientes',
    goodMorning: 'Buenos días',
    goodAfternoon: 'Buenas tardes',
    goodEvening: 'Buenas tardes',
    goodNight: 'Buenas noches',
    dueIn: 'Vence en {days}d',
    dueToday: 'Vence hoy',
    overdue: '{days}d de retraso',
    done: '{done}/{total} completados',
  },

  search: {
    title: 'Buscar',
    subtitle: 'Buscar en todos los espacios de trabajo con opciones de filtrado avanzadas',
    searchPlaceholder: 'Buscar...',
    searchItems: 'Buscar elementos...',
    searchResults: 'Resultados de búsqueda',
    noSearchResults: 'Sin resultados de búsqueda',
    configureFilter: 'Configure el filtro para ver elementos',
    savedSearch: 'Búsqueda guardada',
    saveSearch: 'Guardar búsqueda',
    clearSearch: 'Limpiar búsqueda',
    advancedSearch: 'Búsqueda avanzada',
    recentSearches: 'Búsquedas recientes',
    workItem: 'Elemento de trabajo',
    workspace: 'Espacio de trabajo',
  },

  about: {
    title: 'Acerca de Windshift',
    subtitle: 'Una plataforma integral de gestión de trabajo diseñada para equipos modernos',
    whatIs: '¿Qué es Windshift?',
    description:
      'Windshift es una poderosa aplicación de gestión de trabajo que integra seguimiento de proyectos, gestión de pruebas, seguimiento de tiempo y colaboración en equipo en una plataforma unificada. Construida con tecnologías modernas y diseñada para la flexibilidad, se adapta al flujo de trabajo único de su equipo.',
    keyFeatures: 'Características principales',
    projectManagement: 'Gestión de proyectos',
    projectManagementDesc:
      'Organice elementos de trabajo jerárquicamente con campos personalizados, flujos de trabajo y seguimiento de estado.',
    testManagement: 'Gestión de pruebas',
    testManagementDesc:
      'Cree casos de prueba, organice planes de prueba, ejecute pruebas y realice seguimiento de defectos de manera eficiente.',
    timeTracking: 'Seguimiento de tiempo',
    timeTrackingDesc:
      'Realice seguimiento del tiempo dedicado a proyectos y tareas con informes y analíticas detalladas.',
    builtWith: 'Bibliotecas destacadas',
    libSvelte: 'Framework de frontend',
    libTailwind: 'Framework CSS basado en utilidades',
    libMilkdown: 'Editor de texto enriquecido',
    libExcalidraw: 'Diagramas y bocetos',
    libMeltUI: 'Primitivas de componentes UI',
    libExtism: 'Sistema de complementos WebAssembly',
    libCharmbracelet: 'TUI y acceso SSH (Bubble Tea, Wish)',
    libSvelteFlow: 'Diagramas de flujo',
    versionInfo: 'Información de versión',
    version: 'Versión',
    developedBy: 'Desarrollado por',
    reportIssues: 'Reportar problemas',
    getStarted: 'Comenzar',
  },

  onboarding: {
    welcomeTo: 'Bienvenido a Windshift',
    getStartedMessage: 'Comencemos creando su primer espacio de trabajo y elemento de trabajo',
    progress: 'Progreso',
    completed: 'completado',
    of: 'de',
    createWorkspace: 'Cree su espacio de trabajo',
    workspacesHelp: 'Los espacios de trabajo lo ayudan a organizar proyectos y equipos',
    createWorkspaceBtn: 'Crear espacio de trabajo',
    createFirstWorkItem: 'Cree su primer elemento de trabajo',
    trackTasks: 'Dé seguimiento a tareas, errores, funcionalidades y más',
    createWorkItemBtn: 'Crear elemento de trabajo',
    dismissAssistant: 'Ocultar asistente',
    dismissOnboarding: 'Ocultar guía de inicio',
  },
};
