/**
 * Navigation and general UI translations for Brazilian Portuguese locale
 * Contains: nav, commandPalette, dashboard, search, about, onboarding
 */

export default {
  nav: {
    workspaces: 'Workspaces',
    collections: 'Coleções',
    timeAndProjects: 'Tempo & Projetos',
    milestones: 'Marcos',
    iterations: 'Iterações',
    assets: 'Ativos',
    channels: 'Canais',
    portalHub: 'Central do Portal',
    customers: 'Clientes',
    create: 'Criar',
    search: 'Pesquisar',
    admin: 'Administração',
    notifications: 'Notificações',
    profile: 'Perfil',
    expand: 'Expandir',
    collapse: 'Recolher',
    manageWorkspaces: 'Gerenciar Workspaces',
    manageWorkspacesSubtitle: 'Criar, editar e organizar workspaces',
    searchWorkspaces: 'Pesquisar workspaces...',
    noWorkspacesMatch: 'Nenhum workspace corresponde à sua pesquisa',
    noWorkspacesFound: 'Nenhum workspace encontrado',
    searchToFindMore: 'Pesquise para encontrar mais workspaces...',
    loadingSearch: 'Carregando pesquisa...',
    loadingCreateForm: 'Carregando formulário de criação...',
    loading: 'Carregando...',
    retry: 'Tentar novamente',
    knowledgeBase: 'Base de Conhecimento',
  },

  commandPalette: {
    searchPlaceholder: 'Pesquisar tudo...',
    noCommandsFound: 'Nenhum comando encontrado',
    toSelect: 'para selecionar',
    toNavigate: 'para navegar',
    toClose: 'para fechar',
    pressToOpen: 'Pressione {shortcut} para abrir',
    advancedSearch: 'Pesquisa Avançada',
    context: 'Contexto',
    // Command labels and descriptions
    commands: {
      // Navigation
      workspaces: { label: 'Workspaces', description: 'Gerenciar workspaces de projetos' },
      search: { label: 'Pesquisar', description: 'Pesquisar itens de trabalho e conteúdo' },
      dashboard: { label: 'Painel', description: 'Ver análises e relatórios' },
      milestones: { label: 'Marcos', description: 'Acompanhar marcos do projeto' },
      channels: { label: 'Canais', description: 'Canais de comunicação e suporte' },
      collections: {
        label: 'Coleções',
        description: 'Gerenciar coleções e visualizações de itens de trabalho',
      },
      adminPanel: {
        label: 'Painel de Administração',
        description: 'Administração e configurações do sistema',
      },

      // Test Management (labels aligned with workspace navigation)
      tests: { label: 'Testes', description: 'Gerenciar casos de teste, planos e execução' },
      testCases: { label: 'Casos de Teste', description: 'Visualizar e gerenciar casos de teste' },
      testPlans: {
        label: 'Planos de Teste',
        description: 'Visualizar e gerenciar planos de teste',
      },
      testTemplates: {
        label: 'Modelos',
        description: 'Execuções de modelos e passos compartilhados',
      },
      testRuns: {
        label: 'Execuções de Teste',
        description: 'Visualizar e gerenciar execuções de teste',
      },
      testReports: { label: 'Relatórios', description: 'Ver relatórios de execução de testes' },
      createTestCase: {
        label: 'Criar Caso de Teste',
        description: 'Criar um novo caso de teste para garantia de qualidade',
      },
      createTestPlan: {
        label: 'Criar Plano de Teste',
        description: 'Criar um novo plano de teste com casos de teste',
      },
      createTestRun: {
        label: 'Criar Execução de Teste',
        description: 'Criar um novo modelo de execução de teste',
      },

      // Time Management
      timeTracking: {
        label: 'Controle de Tempo',
        description: 'Registrar e gerenciar entradas de tempo',
      },
      timeReports: {
        label: 'Relatórios de Tempo',
        description: 'Ver relatórios e análises de controle de tempo',
      },
      timeProjects: {
        label: 'Projetos de Tempo',
        description: 'Gerenciar projetos de controle de tempo',
      },
      logTime: {
        label: 'Registrar Tempo',
        description: 'Adicionar rapidamente uma nova entrada de tempo',
      },
      startTimer: {
        label: 'Iniciar Cronômetro',
        description: 'Iniciar o controle de tempo para o trabalho atual',
      },
      stopTimer: { label: 'Parar Cronômetro', description: 'Parar o controle de tempo atual' },

      // Create Commands
      createWorkItem: {
        label: 'Criar Item de Trabalho',
        description: 'Criar um novo item de trabalho ou tarefa',
      },
      createWorkspace: {
        label: 'Criar Workspace',
        description: 'Criar um novo workspace de projeto',
      },
      createMilestone: { label: 'Criar Marco', description: 'Criar um novo marco de projeto' },
      createCollection: {
        label: 'Criar Coleção',
        description: 'Criar uma nova coleção de itens de trabalho',
      },

      // Admin Commands
      adminCustomFields: {
        label: 'Campos Personalizados',
        description: 'Gerenciar definições de campos personalizados',
      },
      adminScreens: {
        label: 'Gerenciamento de Telas',
        description: 'Configurar telas e layouts de formulários',
      },
      adminHierarchyLevels: {
        label: 'Níveis de Hierarquia',
        description: 'Configurar níveis de hierarquia de itens de trabalho',
      },
      adminItemTypes: {
        label: 'Tipos de Item',
        description: 'Gerenciar tipos de itens de trabalho com ícones e cores',
      },
      adminPriorities: {
        label: 'Prioridades',
        description: 'Configurar níveis de prioridade com ícones e cores',
      },
      adminConfigSets: {
        label: 'Conjuntos de Configuração',
        description: 'Gerenciar conjuntos de configuração com workflows e telas',
      },
      adminStatuses: {
        label: 'Status',
        description: 'Gerenciar status individuais de itens de trabalho',
      },
      adminStatusCategories: {
        label: 'Categorias de Status',
        description: 'Gerenciar categorias de status e cores',
      },
      adminWorkflows: {
        label: 'Construtor de Workflow',
        description: 'Projetar e gerenciar transições de workflow',
      },
      adminLinkTypes: {
        label: 'Tipos de Link',
        description: 'Gerenciar tipos de link entre itens de trabalho',
      },
      adminScmProviders: {
        label: 'Provedores SCM',
        description: 'Configurar integrações com GitHub, GitLab, Gitea e Bitbucket',
      },
      adminAttachments: {
        label: 'Anexos',
        description: 'Gerenciar configurações de anexos',
      },
      adminModules: {
        label: 'Configurações de Módulos',
        description: 'Ativar ou desativar módulos de controle de tempo e gerenciamento de testes',
      },
      adminThemes: {
        label: 'Configurações de Tema',
        description: 'Gerenciar temas e aparência da aplicação',
      },
      adminUsers: {
        label: 'Gerenciamento de Usuários',
        description: 'Gerenciar usuários, papéis e permissões',
      },
      adminGroups: {
        label: 'Gerenciamento de Grupos',
        description: 'Gerenciar grupos de usuários e membros',
      },
      adminPermissions: {
        label: 'Permissões',
        description: 'Gerenciar permissões de usuários e controle de acesso',
      },
      adminPermissionSets: {
        label: 'Conjuntos de Permissões',
        description: 'Gerenciar pacotes de permissões para conjuntos de configuração',
      },
      adminWorkspaceRoles: {
        label: 'Papéis do Workspace',
        description: 'Visualizar papéis do workspace e suas permissões',
      },
      adminSso: {
        label: 'Login Único',
        description: 'Configurar provedores de identidade OIDC para SSO',
      },
      adminSecurity: {
        label: 'Configurações de Segurança',
        description: 'Configurar feeds de calendário e execução de plugins',
      },
      adminSystemImport: {
        label: 'Importação de Sistema',
        description: 'Importar dados de outros sistemas',
      },
      adminAssets: {
        label: 'Gerenciamento de Ativos',
        description: 'Configurar tipos e categorias de ativos',
      },
      adminWorkspaces: {
        label: 'Administração de Workspaces',
        description: 'Gerenciar workspaces e configurações',
      },
      adminNotifications: {
        label: 'Configurações de Notificação',
        description: 'Gerenciar configurações de notificações',
      },

      // System Commands
      quitApp: {
        label: 'Encerrar Aplicação',
        description: 'Encerrar o servidor da aplicação de forma segura',
      },

      // Other
      portal: { label: 'Portal', description: 'Gerenciamento do portal do cliente' },
      iterations: { label: 'Iterações', description: 'Planejamento e acompanhamento de sprints' },
      createItem: { label: 'Criar Item', description: 'Criar um novo item de trabalho' },
      myProfile: { label: 'Meu Perfil', description: 'Visualizar e editar seu perfil' },
      notifications: { label: 'Notificações', description: 'Ver notificações' },
      about: { label: 'Sobre', description: 'Sobre esta aplicação' },
      assets: { label: 'Ativos', description: 'Gerenciar conjuntos e itens de ativos' },
      customers: { label: 'Clientes', description: 'Gerenciar organizações de clientes' },

      // Dynamic workspace commands
      goToWorkspace: { label: 'Ir para {name}', description: 'Navegar para o workspace {name}' },
      workspaceOverview: {
        label: 'Visão geral de {name}',
        description: 'Ver painel do workspace com estatísticas e gráficos',
      },
    },
  },

  dashboard: {
    title: 'Painel',
    subtitle: 'Visão geral dos seus itens de trabalho e projetos',
    welcome: 'Bem-vindo',
    overview: 'Visão Geral',
    recentActivity: 'Atividade Recente',
    recentItems: 'Itens Recentes',
    myTasks: 'Minhas Tarefas',
    assignedToMe: 'Atribuído a Mim',
    openTasks: 'Tarefas Abertas',
    overdueItems: 'Itens Atrasados',
    upcomingDeadlines: 'Prazos Próximos',
    activeSprints: 'Sprints Ativos',
    inProgressMilestones: 'Marcos em Andamento',
    noRecentActivity: 'Nenhuma atividade recente',
    recentItemsHint: 'Itens que você atualizar aparecerão aqui',
    noActiveSprints: 'Nenhum sprint ativo',
    sprintTimelinesHint: 'Cronogramas de sprints aparecerão aqui',
    noUpcomingDeadlines: 'Nenhum prazo próximo',
    dueDatesHint: 'Itens com datas de vencimento aparecerão aqui',
    noOpenTasks: 'Nenhuma tarefa aberta atribuída a você',
    allCaughtUp: 'Você está em dia!',
    noOverdueItems: 'Nenhum item atrasado',
    loadingRecentItems: 'Carregando itens recentes...',
    loadingTasks: 'Carregando suas tarefas...',
    loadingOverdueItems: 'Carregando itens atrasados...',
    workItemStatusOverview: 'Visão Geral de Status dos Itens de Trabalho',
    statusOpen: 'Aberto',
    statusInProgress: 'Em Andamento',
    statusClosed: 'Fechado',
    quickActions: 'Ações Rápidas',
    createWorkItem: 'Criar Item de Trabalho',
    createWorkItemDesc: 'Começar a acompanhar um novo item de trabalho',
    manageMilestones: 'Gerenciar Marcos',
    manageMilestonesDesc: 'Visualizar e organizar marcos',
    manageWorkspaces: 'Gerenciar Workspaces',
    manageWorkspacesDesc: 'Configurar workspaces',
    recentWorkItems: 'Itens de Trabalho Recentes',
    whatsNew: 'Novidades',
    noNotifications: 'Nenhuma notificação ainda',
    yourActivity: 'Sua Atividade',
    viewed: 'Visualizado',
    edited: 'Editado',
    commented: 'Comentado',
    noRecentlyViewed: 'Nenhum item visualizado recentemente',
    noRecentlyEdited: 'Nenhum item editado recentemente',
    noRecentlyCommented: 'Nenhum item comentado recentemente',
    quickAccess: 'Acesso Rápido',
    quickAccessHint: 'Pressione {shortcut} para abrir a paleta de comandos',
    upcomingMilestones: 'Próximos Marcos',
    watching: 'Acompanhando',
    recentWorkspaces: 'Workspaces Recentes',
    noRecentWorkspaces: 'Nenhum workspace recente',
    goodMorning: 'Bom dia',
    goodAfternoon: 'Boa tarde',
    goodEvening: 'Boa noite',
    goodNight: 'Boa noite',
    dueIn: 'Vence em {days}d',
    dueToday: 'Vence hoje',
    overdue: '{days}d atrasado',
    done: '{done}/{total} concluído',
  },

  search: {
    title: 'Pesquisa',
    subtitle: 'Pesquisar em todos os workspaces com opções avançadas de filtragem',
    searchPlaceholder: 'Pesquisar...',
    searchItems: 'Pesquisar itens...',
    searchResults: 'Resultados da Pesquisa',
    noSearchResults: 'Nenhum resultado encontrado',
    configureFilter: 'Configure um filtro para ver os itens',
    savedSearch: 'Pesquisa Salva',
    saveSearch: 'Salvar Pesquisa',
    clearSearch: 'Limpar Pesquisa',
    advancedSearch: 'Pesquisa Avançada',
    recentSearches: 'Pesquisas Recentes',
    workItem: 'Item de Trabalho',
    workspace: 'Workspace',
  },

  about: {
    title: 'Sobre o Windshift',
    subtitle:
      'Uma plataforma abrangente de gerenciamento de trabalho projetada para equipes modernas',
    whatIs: 'O que é o Windshift?',
    description:
      'O Windshift é uma poderosa aplicação de gerenciamento de trabalho que reúne acompanhamento de projetos, gerenciamento de testes, controle de tempo e colaboração em equipe em uma plataforma unificada. Desenvolvido com tecnologias modernas e projetado para flexibilidade, ele se adapta ao fluxo de trabalho exclusivo da sua equipe.',
    keyFeatures: 'Principais Funcionalidades',
    projectManagement: 'Gerenciamento de Projetos',
    projectManagementDesc:
      'Organize itens de trabalho hierarquicamente com campos personalizados, workflows e acompanhamento de status.',
    testManagement: 'Gerenciamento de Testes',
    testManagementDesc:
      'Crie casos de teste, organize planos de teste, execute rodadas de teste e acompanhe defeitos de forma eficiente.',
    timeTracking: 'Controle de Tempo',
    timeTrackingDesc:
      'Acompanhe o tempo gasto em projetos e tarefas com relatórios e análises detalhadas.',
    builtWith: 'Desenvolvido Com',
    frontend: 'Frontend',
    backend: 'Backend',
    database: 'Banco de Dados',
    styling: 'Estilização',
    versionInfo: 'Informações de Versão',
    version: 'Versão',
    builtBy: 'Desenvolvido com',
    builtByTeam: 'pela equipe de desenvolvimento',
    reportIssues: 'Reportar Problemas',
    getStarted: 'Começar',
  },

  onboarding: {
    welcomeTo: 'Bem-vindo ao Windshift',
    getStartedMessage: 'Vamos começar criando seu primeiro workspace e item de trabalho',
    progress: 'Progresso',
    completed: 'concluído',
    of: 'de',
    createWorkspace: 'Crie seu workspace',
    workspacesHelp: 'Workspaces ajudam você a organizar projetos e equipes',
    createWorkspaceBtn: 'Criar Workspace',
    createFirstWorkItem: 'Crie seu primeiro item de trabalho',
    trackTasks: 'Acompanhe tarefas, bugs, funcionalidades e muito mais',
    createWorkItemBtn: 'Criar Item de Trabalho',
    dismissAssistant: 'Dispensar Assistente',
    dismissOnboarding: 'Dispensar integração',
  },
};
