/**
 * Time tracking related translations for Brazilian Portuguese locale
 * Includes: time, timeProject, timeProjectCategory sections
 */

export default {
  time: {
    title: 'Registro de Horas',
    subtitle: 'Registre o tempo gasto em itens de trabalho',
    logTime: 'Registrar Tempo',
    editTimeEntry: 'Editar Registro de Tempo',
    updateEntry: 'Atualizar Registro',
    timeSpent: 'Tempo Gasto',
    remaining: 'Restante',
    estimate: 'Estimativa',
    originalEstimate: 'Estimativa Original',
    hours: 'horas',
    minutes: 'minutos',
    days: 'dias',
    weeks: 'semanas',
    startTimer: 'Iniciar Cronometro',
    stopTimer: 'Parar Cronometro',
    pauseTimer: 'Pausar Cronometro',
    resumeTimer: 'Retomar Cronometro',
    timeLogged: 'Tempo registrado com sucesso',
    timeTrackingProject: 'Projeto de Registro de Horas',
    workItemOptional: 'Item de Trabalho (Opcional)',
    whatDidYouWorkOn: 'No que voce trabalhou?',
    start: 'Inicio',
    end: 'Fim',
    duration: 'Duracao',
    durationHelperText:
      'Insira hora de inicio + duracao (2h) para calcular automaticamente a hora de termino, ou insira inicio + fim para calcular automaticamente a duracao. Formatos de tempo: 1h, 30m, 1h30m, 2h15m, 1d (=8h)',

    // Onboarding
    onboarding: {
      title: 'Configurar Registro de Horas',
      subtitle: 'Vamos criar sua primeira organizacao de cliente e projeto para comecar',
      setupProgress: 'Progresso da Configuracao',
      stepOf: 'Etapa {current} de {total}',
      createCustomerTitle: 'Crie Sua Primeira Organizacao de Cliente',
      createCustomerDescription:
        'Uma organizacao de cliente representa a empresa ou entidade para a qual voce esta trabalhando. Pode ser uma organizacao cliente, seu empregador ou sua propria empresa.',
      createProjectTitle: 'Crie Seu Primeiro Projeto',
      createProjectDescription:
        'Projetos ajudam a organizar seu trabalho dentro de um cliente. Voce pode registrar tempo em projetos especificos.',
      organizationNameRequired: 'O nome da organizacao e obrigatorio',
      projectNameRequired: 'O nome do projeto e obrigatorio',
      failedToCreateCustomer:
        'Falha ao criar a organizacao de cliente. Por favor, tente novamente.',
      failedToCreateProject: 'Falha ao criar o projeto. Por favor, tente novamente.',
      customerCreatedSuccess: 'Organizacao de cliente "{name}" criada com sucesso!',
      organizationNamePlaceholder: 'ex.: Empresa ABC, TechStart Ltda, Trabalho Freelance',
      emailPlaceholder: 'cobranca@cliente.com.br',
      contactPersonPlaceholder: 'Joao Silva',
      projectNamePlaceholder: 'ex.: Desenvolvimento de Site, Consultoria, Trabalho Geral',
      projectDescriptionPlaceholder: 'Breve descricao do projeto...',
      hourlyRateHint: 'Voce pode definir isso depois, se nao tiver certeza',
      skipForNow: 'Pular por enquanto',
      completeSetup: 'Concluir Configuracao',
    },

    // Categories
    categories: {
      title: 'Categorias de Projeto',
      subtitle: 'Organize projetos em categorias para uma melhor gestao',
      newCategory: 'Nova Categoria',
      noCategories: 'Nenhuma categoria ainda',
      createFirstHint: 'Crie sua primeira categoria para organizar projetos',
      failedToSave: 'Falha ao salvar categoria',
      failedToDelete: 'Falha ao excluir categoria',
      confirmDelete: 'Tem certeza de que deseja excluir "{name}"?',
    },

    // Reports
    reports: {
      title: 'Relatorios',
      subtitle: 'Analise seus dados de registro de horas e exporte relatorios',
      exportCSV: 'Exportar CSV',
      exportPDF: 'Exportar PDF',
      filters: 'Filtros',
      customer: 'Cliente',
      project: 'Projeto',
      allCustomers: 'Todos os clientes',
      allProjects: 'Todos os projetos',
      descriptionFilter: 'Filtro de Descricao',
      searchDescriptions: 'Buscar descricoes...',
      fromDate: 'Data Inicial',
      toDate: 'Data Final',
      applyFilters: 'Aplicar Filtros',
      totalHours: 'Total de Horas',
      totalEntries: 'Total de Registros',
      averagePerDay: 'Media por Dia',
      topProject: 'Projeto Principal',
      loadingReports: 'Carregando relatorios...',
      noEntriesFound: 'Nenhum registro de tempo encontrado para os filtros selecionados.',
      totalTime: 'Tempo Total',
      entriesShown: '{count} registros exibidos',
    },

    // Timer
    timer: {
      goToWorkItem: 'Ir para o item de trabalho: {title}',
      expandTimer: 'Expandir cronometro',
      collapseTimer: 'Recolher cronometro',
      project: 'Projeto',
      workspace: 'Workspace',
    },

    // Projects
    projects: {
      title: 'Projetos',
      subtitle: 'Gerencie projetos globais para registro de horas entre workspaces',
      addProject: 'Adicionar Projeto',
      projectsTab: 'Projetos',
      categoriesTab: 'Categorias',
      searchProjects: 'Buscar projetos...',
      allCategories: 'Todas as Categorias',
      allStatuses: 'Todos os Status',
      statusCount: '{count} status',
      noProjects:
        'Nenhum projeto encontrado. Crie seu primeiro projeto para comecar a registrar horas.',
      noProjectsInCategory: 'Nenhum projeto nesta categoria.',
      failedToSave: 'Falha ao salvar projeto',
      deleteProject: 'Excluir Projeto',
      confirmDelete: 'Tem certeza de que deseja excluir "{name}"?',
      unknownCustomer: 'Cliente Desconhecido',
      project: 'Projeto',
      customer: 'Cliente',
      rate: 'Tarifa',
      projectName: 'Nome do Projeto',
      descriptionOptional: 'Descricao (Opcional)',
      hourlyRateOptional: 'Tarifa por Hora (Opcional)',
    },

    // Calendar
    calendar: {
      title: 'Calendario Semanal',
      itemCount: '{count} itens',
      exportWeekToICS: 'Exportar Semana para ICS',
      myWorkItems: 'Meus Itens de Trabalho',
      dragToSchedule: 'Arraste itens para agenda-los',
      noWorkItems: 'Nenhum item de trabalho atribuido',
      workItemsWillAppear: 'Itens de trabalho aparecerao aqui quando atribuidos a voce',
      itemsCompleted: '{completed} de {total} itens concluidos',
      previousWeek: 'Semana anterior',
      thisWeek: 'esta semana',
      nextWeek: 'Proxima semana',
      newTaskPlaceholder: 'Titulo da nova tarefa...',
      failedToCreateTask: 'Falha ao criar tarefa',
    },

    // Time Entry
    entry: {
      title: 'Registro de Tempo',
      subtitle: 'Registre suas horas de trabalho e gerencie os registros de tempo',
      addTimeEntry: 'Adicionar um novo registro de tempo',
      failedToSave: 'Falha ao salvar registro de tempo. Por favor, verifique seus dados.',
      confirmDelete: 'Tem certeza de que deseja excluir este registro de tempo?',
      needProjects: 'Voce precisa criar projetos ativos antes de registrar tempo.',
      goToProjects: 'Ir para Projetos',
      startSetupWizard: 'iniciar o assistente de configuracao',
      applyFiltersTitle: 'Aplicar os filtros selecionados a lista de registros de tempo',
      clearFiltersTitle: 'Limpar todos os filtros e mostrar todos os registros de tempo',
      noEntries:
        'Nenhum registro de tempo encontrado. Registre seu primeiro tempo para comecar.',
      clickToView: 'Clique para visualizar {key}-{number}',
      budgetExceeded: '- orcamento excedido',
    },

    // Organizations (formerly Customers)
    organizations: {
      title: 'Organizacoes',
      subtitle: 'Gerencie suas organizacoes de clientes',
      addOrganization: 'Adicionar Organizacao',
      noOrganizations:
        'Nenhuma organizacao encontrada. Crie sua primeira organizacao para comecar.',
      name: 'Nome da Organizacao',
      emailOptional: 'E-mail (Opcional)',
      contactPersonOptional: 'Pessoa de Contato (Opcional)',
      failedToSave: 'Falha ao salvar organizacao',
      deleteOrganization: 'Excluir Organizacao',
      confirmDelete: 'Tem certeza de que deseja excluir "{name}"?',
    },

    // Permissions
    permissions: {
      title: 'Permissoes do Projeto',
      managePermissions: 'Gerenciar Permissoes',
      managers: 'Gerentes',
      members: 'Membros',
      addManager: 'Adicionar Gerente',
      addMember: 'Adicionar Membro',
      removeManager: 'Remover Gerente',
      removeMember: 'Remover Membro',
      noManagers: 'Nenhum gerente atribuido',
      noManagersHint:
        'Quando nenhum gerente esta atribuido, qualquer pessoa pode gerenciar este projeto',
      noMembers: 'Nenhum membro atribuido',
      noMembersHint:
        'Quando nenhum membro esta atribuido, qualquer pessoa pode registrar tempo neste projeto',
      grantedAt: 'Adicionado',
      confirmRemove: 'Tem certeza de que deseja remover {name}?',
      failedToAdd: 'Falha ao adicionar',
      failedToRemove: 'Falha ao remover',
      managersNote: 'Gerentes:',
      managersNoteText:
        'Podem editar configuracoes do projeto, gerenciar membros e visualizar todos os registros de tempo deste projeto.',
      membersNote: 'Membros:',
      membersNoteText:
        'Podem registrar tempo neste projeto e visualizar seus proprios registros de tempo.',
    },
  },

  timeProject: {
    editProject: 'Editar Projeto',
    newProject: 'Novo Projeto',
    projectName: 'Nome do Projeto',
    status: 'Status',
    customerOptional: 'Cliente (Opcional)',
    none: 'Nenhum',
    categoryOptional: 'Categoria (Opcional)',
    hourlyRate: 'Tarifa por Hora (R$)',
    maxHours: 'Horas Maximas',
    maxHoursPlaceholder: 'Sem limite',
    maxHoursHint: 'Orcamento opcional para relatorios',
    projectColor: 'Cor do Projeto',
    updateProject: 'Atualizar Projeto',
    createProject: 'Criar Projeto',
  },

  timeProjectCategory: {
    editCategory: 'Editar Categoria',
    newCategory: 'Nova Categoria',
    categoryName: 'Nome da Categoria',
    categoryNamePlaceholder: 'Desenvolvimento, Marketing, Operacoes...',
    optionalDescription: 'Descricao opcional...',
    updateCategory: 'Atualizar Categoria',
    createCategory: 'Criar Categoria',
  },
};
