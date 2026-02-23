/**
 * Traduções de interface para o idioma Português Brasil (pt-BR)
 * Inclui: pickers, editors, dialogs, components, aria, layout, widgets, footer
 */

export default {
  pickers: {
    // Geral
    select: 'Selecionar',
    search: 'Pesquisar',
    options: 'Opções',
    clearSelection: 'Limpar seleção',
    noResultsFor: 'Nenhum resultado para "{query}"',
    createItem: 'Criar "{value}"',
    noItemsFound: 'Nenhum item encontrado',
    noItemsAvailable: 'Nenhum item disponível',

    // Seletor de Ativo
    selectAsset: 'Selecionar ativo',
    noTag: 'Sem tag',

    // Seletor de Usuário/Responsável
    selectUser: 'Selecionar usuário',
    searchUsers: 'Pesquisar usuários...',
    users: 'Usuários',
    noUsersFound: 'Nenhum usuário encontrado',
    noUsersAvailable: 'Nenhum usuário disponível',
    assignTo: 'Atribuir a',
    unassigned: 'Não atribuído',
    assignee: 'Responsável',
    user: 'Usuário',
    group: 'Grupo',
    searchUser: 'Pesquisar usuário...',
    searchGroup: 'Pesquisar grupo...',

    // Seletor de Grupo
    selectGroup: 'Selecionar grupo',

    // Seletor de Categoria
    selectCategories: 'Selecionar categorias',
    removeCategory: 'Remover categoria',
    categoriesSelected: '{count} categorias selecionadas',
    searchCategories: 'Pesquisar categorias...',
    noCategoriesFound: 'Nenhuma categoria encontrada',

    // Seletor de Coleção
    selectCollections: 'Selecionar coleções',

    // Seletor de Espaço de Trabalho
    selectWorkspaces: 'Selecionar espaços de trabalho',
    searchWorkspaces: 'Pesquisar espaços de trabalho...',
    noWorkspacesFound: 'Nenhum espaço de trabalho encontrado',

    // Seletor de Conjunto de Configuração
    selectConfigurationSet: 'Selecionar conjunto de configuração',
    searchConfigurationSets: 'Pesquisar conjuntos de configuração...',
    configurationSets: 'Conjuntos de configuração',
    defaultConfiguration: 'Configuração Padrão',
    defaultConfigurationDescription: 'Usa as configurações padrão do espaço de trabalho',
    noConfigurationSetsFound: 'Nenhum conjunto de configuração encontrado',

    // Seletor de Entidade do Conjunto de Configuração
    entityAlreadyAssigned: '{label} já está atribuído',
    itemType: 'Tipo de Item',
    priorities: 'Prioridades',
    itemTypes: 'Tipos de Item',
    level: 'Nível {level}',
    assigned: 'Atribuído',
    noEntitiesAssigned: 'Nenhum(a) {entities} atribuído(a)',
    available: 'Disponível',
    noEntitiesMatchSearch: 'Nenhum(a) {entities} corresponde à pesquisa',
    allEntitiesAssigned: 'Todos(as) {entities} estão atribuídos(as)',
    inConfigSet: 'No conjunto de configuração',
    searchEntities: 'Pesquisar {entities}...',

    // Seletor de Campo
    selectField: 'Selecionar campo',
    searchFields: 'Pesquisar campos...',
    noFieldsFound: 'Nenhum campo encontrado',
    customFields: 'Campos Personalizados',
    custom: 'Personalizado',
    customFieldDesc: 'Campo personalizado',
    fieldTypes: {
      text: 'Texto',
      number: 'Número',
      date: 'Data',
      select: 'Seleção',
      multiselect: 'Seleção Múltipla',
      checkbox: 'Caixa de Seleção',
      url: 'URL',
      email: 'E-mail',
      phone: 'Telefone',
      textarea: 'Área de Texto',
      textArea: 'Área de Texto',
      user: 'Usuário',
      rating: 'Avaliação',
      boolean: 'Booleano',
      reference: 'Referência',
      identifier: 'Identificador',
    },
    fieldCategories: {
      basic: 'Campos Básicos',
      dates: 'Campos de Data',
      people: 'Pessoas',
      workflow: 'Fluxo de Trabalho',
      custom: 'Campos Personalizados',
    },
    fields: {
      title: { name: 'Título', description: 'Título do item' },
      description: { name: 'Descrição', description: 'Descrição do item' },
      status: { name: 'Status', description: 'Status atual' },
      priority: { name: 'Prioridade', description: 'Nível de prioridade' },
      type: { name: 'Tipo', description: 'Tipo do item' },
      assignee: { name: 'Responsável', description: 'Usuário atribuído' },
      reporter: { name: 'Relator', description: 'Quem relatou o item' },
      createdAt: { name: 'Criado em', description: 'Quando o item foi criado' },
      updatedAt: { name: 'Atualizado em', description: 'Quando o item foi atualizado pela última vez' },
      dueDate: { name: 'Data de Vencimento', description: 'Quando o item deve ser concluído' },
      startDate: { name: 'Data de Início', description: 'Quando o trabalho começa' },
      estimate: { name: 'Estimativa', description: 'Esforço estimado' },
      labels: { name: 'Rótulos', description: 'Rótulos do item' },
      sprint: { name: 'Sprint', description: 'Sprint associada' },
      milestone: { name: 'Marco', description: 'Marco alvo' },
      parent: { name: 'Pai', description: 'Item pai' },
      children: { name: 'Filhos', description: 'Itens filhos' },
      links: { name: 'Links', description: 'Itens relacionados' },
      attachments: { name: 'Anexos', description: 'Anexos de arquivo' },
      comments: { name: 'Comentários', description: 'Comentários de discussão' },
      watchers: { name: 'Observadores', description: 'Usuários observando este item' },
    },

    // Seletor de Ícone
    iconAndColor: 'Ícone e Cor',
    searchIcons: 'Pesquisar ícones...',
    icons: 'Ícones',
    colors: 'Cores',
    icon: 'Ícone',
    color: 'Cor',

    // Combobox de Rótulo
    allLabels: 'Todos os rótulos',
    selectLabels: 'Selecionar rótulos',
    noLabelsFoundFor: 'Nenhum rótulo encontrado para "{query}"',

    // Seletor de Menção
    mentionUsers: 'Mencionar usuários',
    searching: 'Pesquisando...',
    noNotificationPersonalTask: 'Tarefas pessoais não enviam notificações',

    // Combobox de Marco
    selectMilestone: 'Selecionar marco',
    noMilestone: 'Sem marco',
    milestones: 'Marcos',
    noMilestonesFound: 'Nenhum marco encontrado',

    // Seletor de Prioridade
    selectPriority: 'Selecionar prioridade',
    noPriority: 'Sem prioridade',
    loadingPriorities: 'Carregando prioridades...',
    noPrioritiesConfigured: 'Nenhuma prioridade configurada',

    // Seletor de Projeto
    selectProject: 'Selecionar projeto',

    // Seletor de Repositório
    linkRepositories: 'Vincular Repositórios',
    selectRepositoriesFrom: 'Selecionar repositórios de {provider}',
    searchRepositories: 'Pesquisar repositórios...',
    loadingRepositories: 'Carregando repositórios...',
    noRepositoriesMatchSearch: 'Nenhum repositório corresponde à pesquisa',
    noRepositoriesAvailable: 'Nenhum repositório disponível',
    alreadyLinked: 'Já vinculado',
    linkSelected: 'Vincular Selecionados',
    linking: 'Vinculando...',
    repositoriesSelected: '{count} selecionados',

    // Seletor de Função
    selectRole: 'Selecionar função',

    // Seletor de Tela
    selectScreen: 'Selecionar tela',

    // Seletor de Caso de Teste
    searchTestCases: 'Pesquisar casos de teste...',

    // Seletor de Fluxo de Trabalho
    selectWorkflow: 'Selecionar fluxo de trabalho',
  },

  editors: {
    enterText: 'Digite o texto...',
    selectDate: 'Selecionar data...',
    clickToChangeColor: 'Clique para alterar a cor',
    saveEnter: 'Salvar (Enter)',
    cancelEscape: 'Cancelar (Escape)',
    availableFields: 'Campos Disponíveis',
    selectedFields: 'Campos Selecionados',
    dragFieldsToAdd: 'Arraste os campos para adicioná-los',
    dragToReorderOrDrop: 'Arraste para reordenar ou solte os campos aqui',
    dropFieldsHere: 'Solte os campos aqui para configurar',
    noFieldsMatchSearch: 'Nenhum campo corresponde à pesquisa',
    noFieldsAvailable: 'Nenhum campo disponível',
    allFieldsAdded: 'Todos os campos disponíveis foram adicionados',
    bold: 'Negrito (Ctrl+B)',
    italic: 'Itálico (Ctrl+I)',
    strikethrough: 'Tachado',
    inlineCode: 'Código Inline',
    bulletList: 'Lista com Marcadores',
    numberedList: 'Lista Numerada',
    insertImage: 'Inserir Imagem',
    userNotFound: 'Usuário não encontrado',
  },

  dialogs: {
    cancel: 'Cancelar',
    confirm: 'Confirmar',
    save: 'Salvar',
    close: 'Fechar',
    delete: 'Excluir',
    update: 'Atualizar',
    // Mensagens de confirmação para diálogos confirm()
    confirmations: {
      deleteItem: 'Tem certeza de que deseja excluir "{name}"? Esta ação não pode ser desfeita.',
      deleteSection: 'Tem certeza de que deseja excluir esta seção?',
      discardChanges: 'Você tem alterações não salvas. Tem certeza de que deseja cancelar?',
      dismissAllNotifications:
        'Tem certeza de que deseja dispensar todas as notificações? Esta ação não pode ser desfeita.',
      removeAvatar: 'Tem certeza de que deseja remover sua foto de perfil?',
      revokeCalendarFeed:
        'Tem certeza de que deseja revogar a URL do feed do calendário? Todos os calendários que usam esta URL deixarão de sincronizar.',
      deleteTheme: 'Tem certeza de que deseja excluir este tema? Esta ação não pode ser desfeita.',
      resetBoardConfig:
        'Tem certeza de que deseja restaurar a configuração padrão do quadro? Isso excluirá sua configuração personalizada.',
      deleteCustomField:
        'Tem certeza de que deseja excluir o campo personalizado "{name}"? Isso o removerá de todos os projetos.',
      deleteLinkType:
        'Tem certeza de que deseja excluir este tipo de link? Isso também removerá todos os links deste tipo.',
      deleteAsset: 'Tem certeza de que deseja excluir este ativo?',
      deleteAssetSet:
        'Tem certeza de que deseja excluir este conjunto de ativos? Isso excluirá todos os ativos, tipos e categorias contidos nele.',
      deleteAssetType:
        'Tem certeza de que deseja excluir este tipo de ativo? Os ativos que usam este tipo não terão mais um tipo atribuído.',
      deleteCategory:
        'Tem certeza de que deseja excluir esta categoria? As categorias filhas serão movidas para a categoria pai.',
      revokeRole: 'Tem certeza de que deseja revogar esta função?',
      quitApplication: 'Tem certeza de que deseja sair do aplicativo? O servidor será encerrado.',
      deleteConnection:
        'Tem certeza de que deseja excluir esta conexão? Esta ação não pode ser desfeita.',
      deleteWidget: 'Excluir esta seção? Todos os widgets nesta seção serão removidos.',
      deleteScreen:
        'Tem certeza de que deseja excluir a tela "{name}"? Isso afetará todos os espaços de trabalho que usam esta tela.',
    },
    // Mensagens de alerta para diálogos alert()
    alerts: {
      nameRequired: 'O nome é obrigatório',
      pleaseSelectImage: 'Por favor, selecione um arquivo de imagem',
      timerAlreadyRunning: 'Um cronômetro já está em execução. Por favor, pare-o antes de iniciar um novo.',
      noTimerRunning: 'Nenhum cronômetro está em execução no momento.',
      timerSyncing: 'O cronômetro está sincronizando. Por favor, aguarde e tente novamente.',
      startTimerFromItem: 'Por favor, inicie um cronômetro a partir de um item de trabalho para fornecer contexto.',
      cannotDeleteDefaultScreen:
        'Não é possível excluir a tela padrão. Esta tela é necessária para espaços de trabalho sem um conjunto de configuração.',
      applicationShuttingDown: 'O aplicativo está sendo encerrado...',
      pdfExportComingSoon: 'Exportação em PDF em breve para a visualização de blocos de tempo',
      configUpdatedSuccess:
        'Conjunto de configuração atualizado com sucesso. Todos os itens de trabalho já estão usando os status do novo fluxo de trabalho.',
      failedToSave: 'Falha ao salvar: {error}',
      failedToDelete: 'Falha ao excluir: {error}',
      failedToUpdate: 'Falha ao atualizar: {error}',
      failedToLoad: 'Falha ao carregar: {error}',
      failedToCreate: 'Falha ao criar: {error}',
      failedToUpload: 'Falha ao enviar: {error}',
      failedToGeneratePdf: 'Falha ao gerar o PDF. Por favor, tente novamente.',
      failedToApplyConfig: 'Falha ao aplicar a alteração de configuração: {error}',
      failedToAddManager: 'Falha ao adicionar gerente: {error}',
      failedToRemoveManager: 'Falha ao remover gerente: {error}',
      failedToSaveWorkspace: 'Falha ao salvar o projeto. Por favor, verifique suas informações e tente novamente.',
      failedToResetConfig: 'Falha ao redefinir a configuração: {error}',
      failedToToggleStatus: 'Falha ao alternar o status do tipo de link: {error}',
      failedToAssignRole: 'Falha ao atribuir função: {error}',
      failedToRevokeRole: 'Falha ao revogar função: {error}',
      failedToUpdateRole: 'Falha ao atualizar a função de todos: {error}',
      failedToLoadFields: 'Falha ao carregar campos: {error}',
      failedToSaveFields: 'Falha ao salvar atribuições de campos: {error}',
      errorAddingTestCase: 'Erro ao adicionar caso de teste: {error}',
      failedToCreateLabel: 'Falha ao criar rótulo: {error}',
      failedToSaveLayout: 'Falha ao salvar alterações de layout',
      statusInUseByTransitions:
        'Não é possível excluir "{name}" porque está sendo usado em {count} transição(ões) de fluxo de trabalho. Para excluir este status, vá para Gerenciamento de Fluxo de Trabalho, remova todas as transições que usam este status e tente excluir o status novamente.',
    },
  },

  components: {
    // Componente Avatar
    avatar: {
      defaultAlt: 'Avatar',
    },

    // Componente DataTable
    dataTable: {
      showingRange: 'Exibindo {start}–{end} de {total}',
    },

    // Componentes de Diagrama
    diagram: {
      loading: 'Carregando diagramas...',
      loadError: 'Falha ao carregar diagramas',
      deleteError: 'Falha ao excluir diagrama',
      confirmDelete: 'Tem certeza de que deseja excluir este diagrama?',
      edit: 'Editar diagrama',
      untitled: 'Diagrama Sem Título',
      namePlaceholder: 'Nome do diagrama',
      nameRequired: 'Por favor, insira um nome para o diagrama',
      saveError: 'Falha ao salvar diagrama',
      unsavedChanges: 'Alterações não salvas',
      unsavedChangesConfirm: 'Você tem alterações não salvas. Tem certeza de que deseja fechar?',
    },

    // Componente ErrorState
    errorState: {
      title: 'Algo deu errado',
    },

    // Componente Pagination
    pagination: {
      showingRange: 'Exibindo {start}-{end} de {total}',
      limitedTo: 'limitado a {max} itens',
      itemsPerPage: 'Itens por página:',
      previousPage: 'Página anterior',
      nextPage: 'Próxima página',
      goToPage: 'Ir para a página {page}',
      pageOf: 'Página {current} de {total}',
    },

    // Componente UserAvatar
    userAvatar: {
      myWorkspace: 'Meu Espaço de Trabalho',
      myWorkspaceSubtitle: 'Espaço de trabalho pessoal para tarefas e anotações',
      profileSubtitle: 'Gerencie seu perfil e configurações',
      security: 'Segurança',
      securitySubtitle: 'Gerencie senhas, 2FA e tokens de API',
      themeTitle: 'Tema: {mode}',
      themeCycle: 'Clique para alternar: Claro → Escuro → Sistema',
      themeLight: 'Claro',
      themeDark: 'Escuro',
      themeSystem: 'Sistema',
    },
  },

  aria: {
    close: 'Fechar',
    dragToReorder: 'Arraste para reordenar',
    refresh: 'Atualizar',
    removeField: 'Remover campo',
    removeFromSection: 'Remover da seção',
    addNewStep: 'Adicionar nova etapa',
    removeCurrentStep: 'Remover etapa atual',
    dismissNotification: 'Dispensar notificação',
    mainNavigation: 'Navegação principal',
    mentionUsers: 'Mencionar usuários',
    notifications: 'Notificações',
    adminSettings: 'Configurações de administração',
    userMenu: 'Menu do usuário',
    clearSearch: 'Limpar pesquisa',
  },

  layout: {
    addSection: 'Adicionar Seção',
    moveUp: 'Mover seção para cima',
    moveDown: 'Mover seção para baixo',
    deleteSection: 'Excluir seção',
    editMode: 'Modo de Edição',
    editDisplaySettings: 'Editar configurações de exibição',
    items: 'itens',
  },

  widgets: {
    removeWidget: 'Remover widget',
    narrowWidth: 'Estreito (1/3 da largura)',
    mediumWidth: 'Médio (2/3 da largura)',
    fullWidth: 'Largura total',
    chart: {
      items: 'itens',
    },
    completionChart: {
      emptyMessage: 'Nenhum dado de conclusão disponível',
    },
    createdChart: {
      emptyMessage: 'Nenhum dado de criação disponível',
    },
    milestoneProgress: {
      emptyTitle: 'Nenhum marco',
      emptySubtitle: 'Crie marcos para acompanhar o progresso',
      due: 'Vencimento',
      done: 'concluído',
      item: 'item',
      items: 'itens',
      noItems: 'Nenhum item',
      noStatus: 'Sem status',
      activeMilestone: 'Ativo',
      noCategorizedWork: 'Nenhum trabalho categorizado',
    },
  },

  footer: {
    platformName: 'Windshift Plataforma de Gestão de Trabalho',
    aboutWindshift: 'Sobre o Windshift',
    reportProblem: 'Relatar um problema',
  },
};
