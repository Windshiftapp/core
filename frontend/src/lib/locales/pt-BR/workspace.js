/**
 * Workspace-related translations for Brazilian Portuguese locale
 *
 * This module contains the following sections:
 * - workspaces: Workspace management translations
 * - items: Work items (issues, tasks, etc.) translations
 * - comments: Comment-related translations
 * - todo: Personal tasks and todo list translations
 * - collectionTree: Tree view translations
 * - collections: Saved queries and filters translations
 * - links: Item links translations
 */

export default {
  workspaces: {
    title: 'Workspaces',
    subtitle: 'Gerencie seus workspaces e projetos',
    workspace: 'Workspace',
    workspaces_one: '{count} workspace',
    workspaces_other: '{count} workspaces',
    createWorkspace: 'Criar Workspace',
    editWorkspace: 'Editar Workspace',
    deleteWorkspace: 'Excluir Workspace',
    switchWorkspace: 'Trocar de Workspace',
    workspaceName: 'Nome do Workspace',
    workspaceKey: 'Chave do Workspace',
    workspaceDescription: 'Descrição',
    members: 'Membros',
    settings: 'Configurações do Workspace',
    noWorkspaces: 'Nenhum workspace encontrado',
    selectWorkspace: 'Selecione um workspace',
    currentWorkspace: 'Workspace Atual',
    workspaceCreated: 'Workspace criado com sucesso',
    workspaceUpdated: 'Workspace atualizado com sucesso',
    workspaceDeleted: 'Workspace excluído com sucesso',
    customers: {
      title: 'Clientes',
      subtitle: 'Gerencie clientes e organizações do portal',
      addCustomer: 'Adicionar Cliente',
      unassignedCustomers: 'Clientes Não Atribuídos',
      customerCount_one: '{count} cliente',
      customerCount_other: '{count} clientes',
      failedToLoadCustomers: 'Falha ao carregar clientes',
      failedToLoadOrganisations: 'Falha ao carregar organizações',
      failedToAssignCustomer: 'Falha ao atribuir cliente à organização',
      deleteCustomer: 'Excluir Cliente',
      confirmDeleteCustomer: 'Tem certeza de que deseja excluir "{name}"?',
      manageOrganisations: 'Gerenciar Organizações',
      searchOrganisations: 'Buscar organizações...',
      noOrganisationsFound: 'Nenhuma organização encontrada',
      noCustomersFound: 'Nenhum cliente encontrado',
      unassigned: 'Não atribuído',
      allCustomersAssigned: 'Todos os clientes estão atribuídos a organizações',
      searchCustomers: 'Buscar clientes...',
      tryAdjustingSearch: 'Tente ajustar sua busca',
      dragCustomersHere: 'Arraste clientes aqui para atribuí-los a esta organização',
      linked: 'Vinculado: ',
      loadMore: 'Carregar mais ({count} restantes)',
      addPortalCustomer: 'Adicionar Cliente do Portal',
      editPortalCustomer: 'Editar Cliente do Portal',
      createCustomer: 'Criar Cliente',
      noneUnassigned: 'Nenhum (Não atribuído)',
      customFields: 'Campos Personalizados',
      fields: {
        name: 'Nome',
        email: 'Email',
        phone: 'Telefone',
        customerOrganisation: 'Organização do Cliente',
      },
      placeholders: {
        name: 'Digite o nome do cliente',
        email: 'cliente@exemplo.com',
        phone: '+55 (11) 91234-5678',
      },
      metadata: {
        created: 'Criado em:',
        updated: 'Atualizado em:',
        linkedUser: 'Usuário Vinculado:',
      },
    },
  },

  items: {
    title: 'Itens',
    subtitle: 'Visualize e gerencie itens de trabalho',
    item: 'Item',
    items_one: '{count} item',
    items_other: '{count} itens',
    createItem: 'Criar Item',
    editItem: 'Editar Item',
    deleteItem: 'Excluir Item',
    viewItem: 'Visualizar Item',
    itemKey: 'Chave',
    itemTitle: 'Título',
    itemDescription: 'Descrição',
    itemType: 'Tipo de Item',
    itemStatus: 'Status',
    itemPriority: 'Prioridade',
    assignee: 'Responsável',
    reporter: 'Relator',
    dueDate: 'Data de Vencimento',
    startDate: 'Data de Início',
    estimate: 'Estimativa',
    timeSpent: 'Tempo Gasto',
    remaining: 'Restante',
    parent: 'Pai',
    children: 'Filhos',
    subtasks: 'Subtarefas',
    linkedItems: 'Itens Vinculados',
    attachments: 'Anexos',
    comments: 'Comentários',
    activity: 'Atividade',
    history: 'Histórico',
    noItems: 'Nenhum item encontrado',
    noItemsInFilter: 'Nenhum item corresponde ao filtro atual',
    createToStart: 'Crie um item para começar',
    itemCreated: 'Item criado com sucesso',
    itemUpdated: 'Item atualizado com sucesso',
    itemDeleted: 'Item excluído com sucesso',
    assignedToYou: 'Atribuído a você',
    createdByYou: 'Criado por você',
    recentlyViewed: 'Visualizado recentemente',
    recentlyUpdated: 'Atualizado recentemente',
    // Item Detail Tabs
    timeTracking: 'Controle de Tempo',
    details: 'Detalhes',
    created: 'Criado',
    lastUpdated: 'Última Atualização',
    by: 'por',
    workItemInformation: 'Informações do Item de Trabalho',
    id: 'ID',
    type: 'Tipo',
    workItem: 'Item de Trabalho',
    noProjectConfigured: 'Nenhum projeto configurado para controle de tempo',
    setDefaultProject:
      'Defina um projeto padrão nas configurações do workspace ou do item para registrar tempo',
    timeEntries: 'Registros de Tempo',
    startTimer: 'Iniciar Cronômetro',
    logTime: 'Registrar Tempo',
    startTimerTitle: 'Iniciar o rastreamento de tempo para este item de trabalho',
    logTimeTitle: 'Registrar manualmente o tempo trabalhado neste item',
    noTimeLogged: 'Nenhum tempo registrado ainda',

    // Item Detail additional translations
    workItemDetails: 'Detalhes do Item de Trabalho',
    fullDetails: 'Detalhes Completos',
    errorLoadingWorkItem: 'Erro ao Carregar Item de Trabalho',
    workItemNotFound: 'Item de trabalho não encontrado',
    timerBusy: 'Cronômetro ocupado',
    timerSyncingMessage: 'O cronômetro está sincronizando, aguarde um momento e tente novamente.',
    timerAlreadyRunning: 'Cronômetro já em execução',
    stopTimerFirst: 'Por favor, pare o cronômetro atual antes de iniciar um novo.',
    workingOn: 'Trabalhando em {title}',
    failedToStartTimer: 'Falha ao iniciar cronômetro',
    failedToSaveTimeEntry: 'Falha ao salvar registro de tempo',
    failedToDeleteTimeEntry: 'Falha ao excluir registro de tempo',
    deleteTimeEntry: 'Excluir Registro de Tempo',
    deleteTimeEntryConfirm:
      'Tem certeza de que deseja excluir este registro de tempo? Esta ação não pode ser desfeita.',
    noDescription: 'Sem descrição',
    itemCopiedAs: 'Item de trabalho copiado com sucesso como {key}',
    clickToViewCopied: 'Clique para visualizar o item copiado',
    failedToCopy: 'Falha ao copiar item',
    deleteWorkItem: 'Excluir Item de Trabalho',
    confirmDeleteItem:
      'Tem certeza de que deseja excluir "{title}"? Esta ação não pode ser desfeita.',
    failedToDelete: 'Falha ao excluir item',

    // Cascade delete dialog
    deleteItemWithChildren: 'Excluir Item com Filhos',
    itemHasChildren: 'Este item possui {count} itens filhos.',
    itemHasChildrenSingular: 'Este item possui 1 item filho.',
    deleteAllOption: 'Excluir todos ({count} itens)',
    deleteAllDescription: 'Excluir permanentemente este item e todos os seus descendentes',
    reparentOption: 'Reatribuir filhos',
    reparentDescription: 'Mover os filhos para o pai deste item e excluir apenas este item',
    typeToConfirm: 'Digite "{title}" para confirmar a exclusão',
    confirmationPlaceholder: 'Digite o título para confirmar...',
    deleteAllItems: 'Excluir Todos os Itens',
    reparentAndDelete: 'Reatribuir e Excluir',
    reparentFailed: 'Falha ao reatribuir filhos',
    cascadeDeleteFailed: 'Falha ao excluir árvore de itens',
    deletedItemsCount: '{count} itens excluídos',
    reparentedAndDeleted: 'Filhos reatribuídos e item excluído',
    selectNewParent: 'Selecionar novo pai para os filhos',
    selectNewParentPlaceholder: 'Escolha um item pai...',
    makeRootItem: 'Tornar itens raiz (sem pai)',
    reparentLevelHint: 'Exibindo apenas itens no mesmo nível hierárquico',
    noOtherItemsAtLevel:
      'Nenhum outro item neste nível - selecione "Tornar itens raiz" ou escolha dentre os acima',
    reparentToGrandparent: 'Os filhos serão movidos para o avô',
    childrenWillBecomeRoot: 'Os filhos se tornarão itens raiz',
    failedToUpdateWatchStatus: 'Falha ao atualizar status de observação',
    copyWorkItem: 'Copiar Item de Trabalho',
    unwatchWorkItem: 'Deixar de Observar Item de Trabalho',
    watchWorkItem: 'Observar Item de Trabalho',
    noSubIssueTypes: 'Nenhum tipo de sub-item disponível',
    cannotCreateChildItems: 'Não é possível criar itens filhos para este nível de item.',

    // Item Detail Breadcrumbs
    workItems: 'Itens de Trabalho',
    linkedTo: 'vinculado a',
    goToLinkedWorkItem: 'Ir para item de trabalho vinculado',
    goTo: 'Ir para {title}',
    noParent: 'Sem pai',
    setParent: 'Definir pai',
    changeParent: 'Alterar Pai',
    searchForParentItem: 'Buscar item pai...',
    showingItemsFromLevel: 'Exibindo apenas itens do nível hierárquico {level}',
    oneLevelAbove: 'um nível acima de {name}',
    searchParentAcrossWorkspaces: 'Buscar item pai em todos os workspaces',
    removeParent: 'Remover pai',
    noItemsAtLevel: 'Nenhum item encontrado no nível hierárquico {level}',
    failedToUpdateParent: 'Falha ao atualizar pai',
    failedToRemoveParent: 'Falha ao remover pai',
    clickToCopyKey: 'Clique para copiar a chave para a área de transferência',

    // Item Detail Description
    enterDescription: 'Digite a descrição...',
    clickToEditDescription: 'Clique para editar a descrição',
    clickToAddDescription: 'Clique para adicionar descrição',
    noDescriptionProvided: 'Nenhuma descrição fornecida - clique para adicionar uma',
    addLink: 'Adicionar Link',
    createChild: 'Criar Filho',
    child: 'Filho',
    attachFile: 'Anexar Arquivo',
    attach: 'Anexar',
    newDiagram: 'Novo Diagrama',
    diagram: 'Diagrama',

    // Item Detail Header
    previousValueRemains: 'O valor anterior permanece inalterado',
    titleCannotBeEmpty: 'O título não pode estar vazio',
    enterTitle: 'Digite o título...',
    clickToEditTitle: 'Clique para editar o título',

    // Item Detail Links
    searchTestCases: 'Buscar casos de teste...',
    searchWorkItems: 'Buscar itens de trabalho...',
    loadingLinks: 'Carregando links...',
    removeLink: 'Remover link',
    linkType: 'Tipo de Link',
    chooseRelationshipType: 'Escolha o tipo de relacionamento...',
    linkToTestCase: 'Vincular este item a um caso de teste.',
    targetItem: 'Item de Destino',
    testCase: 'Caso de Teste',
    selectLinkTypeToSearch: 'Selecione um tipo de link para iniciar a busca.',
    childWorkItems: 'Itens de Trabalho Filhos',
    loadingChildItems: 'Carregando itens de trabalho filhos...',

    // Item Detail Sidebar
    setStatus: 'Definir status',
    unassigned: 'Não atribuído',
    milestone: 'Marco',
    iteration: 'Iteração',
    project: 'Projeto',
    clickToViewDetails: 'Clique para ver os detalhes do item',

    // Clipboard
    itemLinkCopied: 'Link do item copiado para a área de transferência',
    failedToCopyToClipboard: 'Falha ao copiar para a área de transferência',
    copyError: 'Erro ao Copiar',

    // Custom field placeholders
    setField: 'Definir {field}',
    selectField: 'Selecionar {field}',
    enterField: 'Inserir {field}',
    notSet: 'Não definido',
    selectIteration: 'Selecionar iteração',
    noIteration: 'Sem iteração',
    selectOrCreateLabels: 'Selecionar ou criar rótulos',
  },

  comments: {
    failedToLoad: 'Falha ao carregar comentários',
    failedToCreate: 'Falha ao publicar comentário',
    confirmDelete: 'Tem certeza de que deseja excluir este comentário?',
    failedToDelete: 'Falha ao excluir comentário',
    failedToUpdate: 'Falha ao atualizar comentário',
    edited: 'editado',
    editComment: 'Editar comentário',
    deleteComment: 'Excluir comentário',
    editPlaceholder: 'Edite seu comentário...',
    writePlaceholder: 'Adicione um comentário...',
    markdownSupported: 'Markdown suportado',
    posting: 'Publicando...',
    comment: 'Comentário',
    noComments: 'Nenhum comentário ainda',
    beFirstToComment: 'Seja o primeiro a comentar neste item.',
    internalNote: 'Nota interna',
    internalNoteHint: 'Não visível no portal',
    internal: 'Interno',
    oldestFirst: 'Mais antigos primeiro',
    newestFirst: 'Mais recentes primeiro',
  },

  todo: {
    failedToCreate: 'Falha ao criar tarefa',
    confirmDelete: 'Tem certeza de que deseja excluir esta tarefa?',
    deleteTask: 'Excluir Tarefa',
    failedToDelete: 'Falha ao excluir tarefa',
    loadingTasks: 'Carregando tarefas...',
    myPersonalTasks: 'Minhas Tarefas Pessoais',
    whatNeedsToBeDone: 'O que precisa ser feito?',
    addPersonalTask: 'Adicionar tarefa pessoal',
    noPersonalTasks: 'Nenhuma tarefa pessoal',
    addFirstTask: 'Adicione sua primeira tarefa para acompanhar o que você precisa fazer.',
    ofPersonalTasksRemaining: '{count} de {total} tarefas pessoais restantes',
    assignedToMe: 'Atribuído a Mim',
    noAssignedWork: 'Nenhum trabalho atribuído',
    assignedItemsWillAppear: 'Itens de trabalho atribuídos a você aparecerão aqui.',
    ofAssignedItemsRemaining: '{count} de {total} itens atribuídos restantes',
    task: 'Tarefa',
    dueDate: 'Data de Vencimento',
    progress: 'Progresso',
  },

  collectionTree: {
    loading: 'Carregando...',
    tree: 'Árvore',
    noWorkItemsYet: 'Nenhum item de trabalho ainda',
    createFirstWorkItem: 'Crie seu primeiro item de trabalho para ver a árvore hierárquica.',
    expandAll: 'Expandir Todos',
    collapseAll: 'Recolher Todos',
    showTests: 'Mostrar Testes',
    hideTests: 'Ocultar Testes',
    showingRootItems: 'Exibindo {start}-{end} de {total} itens raiz',
    page: 'Página',
    pageOfTotal: 'Página {current} de {total}',
    issue: 'Issue',
    noStatus: 'Sem status',
    workspaceNotFound: 'Workspace não encontrado.',
  },

  collections: {
    // Page titles and headers
    title: 'Coleções',
    subtitle: 'Consultas e filtros salvos',
    allGlobal: 'Todas Globais',
    globalCollection: 'Coleção Global',
    workspaceCollections: 'Coleções do Workspace',
    workspaceCollectionsTitle: 'Coleções do Workspace',
    allGlobalCollections: 'Todas as Coleções Globais',
    categoryCollections: 'Coleções de {category}',

    // Collection management
    newCollection: 'Nova Coleção',
    createCollection: 'Criar Coleção',
    editCollection: 'Editar Coleção',
    deleteCollection: 'Excluir Coleção',
    viewCollection: 'Visualizar Coleção',
    saveCollection: 'Salvar Coleção',
    updateCollection: 'Atualizar Coleção',

    // Collection properties
    collectionName: 'Nome da Coleção',
    collectionDescription: 'Descrição',
    noQuery: 'Sem consulta',
    noFiltersApplied: 'Nenhum filtro aplicado',

    // Workspace association
    associateWorkspace: 'Associar Workspace',
    changeWorkspace: 'Alterar Workspace',
    changeWorkspaceAssociation: 'Alterar Associação do Workspace',
    associateWithWorkspace: 'Associar a um Workspace',
    workspaceAssociationDesc:
      'Selecionar um workspace limitará esta coleção a esse workspace. Deixe sem atribuição para mantê-la global.',
    saveAssociation: 'Salvar Associação',
    workspaceAssociationNote:
      'Apenas um workspace pode ser associado por vez. Remover a seleção converte a coleção de volta para uma visualização global.',
    searchWorkspace: 'Buscar um workspace...',

    // Categories
    manageCategories: 'Gerenciar Categorias',
    noCategory: 'Sem Categoria',

    // Filters and query
    filters: 'Filtros',
    expandSidebar: 'Expandir barra lateral',
    collapseSidebar: 'Recolher barra lateral',
    workspaces: 'Workspaces',
    selectWorkspaces: 'Selecionar workspaces...',
    status: 'Status',
    selectStatuses: 'Selecionar status...',
    priority: 'Prioridade',
    selectPriorities: 'Selecionar prioridades...',
    searchItems: 'Buscar itens...',
    addFieldFilter: 'Adicionar Filtro de Campo',
    clearSearch: 'Limpar busca',

    // Query editor
    query: 'Consulta',
    queryLanguage: 'Linguagem de Consulta',
    queryPlaceholder: 'Exemplo: workspace = "Meu Projeto" AND status = "open"',
    edit: 'Editar',
    hide: 'Ocultar',
    clear: 'Limpar',
    execute: 'Executar',
    executeShortcut: '{shortcut} para executar',
    error: 'Erro',

    // Search modal
    searchItemsTitle: 'Buscar Itens',
    enterSearchText: 'Digite o texto de busca...',
    apply: 'Aplicar',

    // Views
    board: 'Quadro',
    backlog: 'Backlog',
    configure: 'Configurar',
    map: 'Mapa',

    // Backlog view
    noItemsInBacklog: 'Nenhum Item no Backlog',
    noItemsInBacklogDesc: 'Todos os itens de trabalho estão concluídos ou ainda não existem itens.',
    showingItemsFromBacklog: 'Exibindo {count} itens do backlog',

    // Map view
    loadingStoryMap: 'Carregando mapa de histórias...',
    rootLevel: 'Nível Raiz',
    currentLevel: 'Nível Atual',
    showingItems: 'Exibindo {count} {type}{plural}',
    childItems: '{count} item filho{plural}',
    childWorkItems: 'Itens de Trabalho Filhos ({count})',
    noChildItems: 'Nenhum item filho ainda',
    noChildItemsLowest: 'Nenhum item filho (nível hierárquico mais baixo)',
    addCard: 'Adicionar cartão',
    create: 'Criar',
    enterSummary: 'Digite um resumo...',
    selectWorkspace: 'Selecionar workspace',
    selectItemType: 'Selecionar tipo de item',
    noTypesAvailable: 'Nenhum tipo disponível',
    drillDown: 'Detalhar para mostrar filhos como base',
    noTopLevelItems: 'Nenhum item de nível superior encontrado',
    noTopLevelItemsDesc: 'Crie alguns itens de trabalho para ver seu mapa de histórias',
    workspaceNotFound: 'Workspace não encontrado.',

    // Collections list
    collection: 'Coleção',
    queryColumn: 'Consulta',
    created: 'Criado',
    actions: 'Ações',
    public: 'Público',
    workspaceFilter: 'Filtro de Workspace',
    allWorkspaces: 'Todos os workspaces',
    noCollectionsTitle: 'Nenhuma coleção encontrada.',
    noCollectionsFound:
      'Crie sua primeira coleção para salvar e reutilizar consultas de itens de trabalho.',
    collectionCount: '{count} coleção',
    collectionCountPlural: '{count} coleções',

    // Results
    addFiltersToStart: 'Adicione filtros para começar',
    addFiltersDesc:
      'Use os filtros da barra lateral ou escreva uma consulta para buscar itens de trabalho.',
    loadingWorkspaces: 'Carregando workspaces...',
    loadingWorkItems: 'Carregando itens de trabalho...',
    noWorkItemsFound: 'Nenhum item de trabalho encontrado',
    tryAdjustingFilters: 'Tente ajustar seus filtros ou termos de busca.',
    showingWorkItems: 'Exibindo {count} itens de trabalho',

    // Confirmations
    confirmDeleteCollection:
      'Tem certeza de que deseja excluir a coleção "{name}"? Esta ação não pode ser desfeita.',
    confirmDeleteItem:
      'Tem certeza de que deseja excluir "{title}"? Esta ação não pode ser desfeita.',
    noQueryToSave:
      'Nenhuma consulta para salvar. Por favor, configure alguns filtros ou insira uma consulta QL primeiro.',

    // Board view
    boardSummary: 'Total: {itemCount} itens de trabalho em {columnCount} colunas',
  },

  links: {
    title: 'Links',
    subtitle: 'Gerenciar links de itens',
    addLink: 'Adicionar Link',
    removeLink: 'Remover link',
    linkText: 'Texto do link',
    linkUrl: 'URL',
  },

  workspaceSettings: {
    // Tab navigation
    tabs: {
      mode: 'Modo',
      general: 'Geral',
      appearance: 'Aparência',
      categories: 'Categorias',
      members: 'Membros',
      configurationSets: 'Conjuntos de Configuração',
      sourceControl: 'Controle de Versão',
      removeWorkspace: 'Remover Workspace',
    },

    // Page header
    title: 'Configurações',
    subtitle: 'Configurar definições para {name}',
    breadcrumbs: {
      workspaces: 'Workspaces',
      settings: 'Configurações',
    },

    // Access denied
    accessDenied: 'Acesso Negado',
    accessDeniedDescription:
      'Você precisa de permissões de administrador do workspace para acessar as configurações.',
    backToWorkspace: 'Voltar ao Workspace',

    // Mode tab
    displayMode: 'Modo de Exibição',
    displayModeDescription:
      'Escolha como este workspace é exibido. Isso afeta o layout de navegação e o comportamento padrão.',
    modeDefault: 'Padrão',
    modeDefaultDescription:
      'Barra lateral de navegação completa com todas as visualizações, coleções e ferramentas do workspace.',
    modeBoard: 'Quadro',
    modeBoardDescription:
      'Layout simplificado focado na visualização de quadro. A navegação está disponível por meio de uma barra de ferramentas compacta.',
    modeItsm: 'ITSM',
    modeItsmDescription:
      'Layout de gerenciamento de serviços otimizado para tratamento de tickets e rastreamento de SLA.',
    modeComingSoon: 'Em Breve',

    // General tab
    basicInformation: 'Informações Básicas',
    workspaceName: 'Nome do Workspace',
    workspaceNamePlaceholder: 'Digite o nome do workspace',
    workspaceKey: 'Chave do Workspace',
    workspaceKeyPlaceholder: 'ex.: DEV, TEST, PROD',
    workspaceKeyHelp:
      'Usada como prefixo de itens (ex.: DEV-123). Apenas letras maiúsculas e números.',
    description: 'Descrição',
    descriptionPlaceholder: 'Descrição opcional para este workspace',
    defaultTimeProject: 'Projeto Padrão de Controle de Tempo',
    noDefaultProject: 'Nenhum projeto padrão',
    defaultTimeProjectHelp:
      'Projeto padrão usado ao registrar tempo a partir de itens de trabalho neste workspace. Pode ser substituído por item de trabalho.',
    defaultView: 'Visualização Padrão do Workspace',
    defaultViewHelp: 'Visualização padrão exibida ao entrar neste workspace.',
    activeWorkspace: 'Workspace Ativo',
    activeWorkspaceHelp:
      'Quando inativo, apenas administradores do sistema e do workspace podem acessar este workspace. Todos os dados são preservados.',

    // View options
    views: {
      board: 'Quadro',
      backlog: 'Backlog',
      list: 'Lista',
      tree: 'Árvore',
      map: 'Mapa',
      overview: 'Visão Geral',
    },

    // Appearance tab
    visualIdentity: 'Identidade Visual',
    visualIdentityDescription:
      'Personalize a aparência visual do seu workspace com ícones, cores e avatares.',
    workspaceIconColor: 'Ícone e Cor do Workspace',
    workspaceAvatar: 'Avatar do Workspace',
    customAvatar: 'Avatar Personalizado',
    imageUploadedSuccessfully: 'Imagem enviada com sucesso',
    defaultIcon: 'Ícone Padrão',
    usingSelectedIconColor: 'Usando ícone e cor selecionados',
    changeAvatar: 'Alterar Avatar',
    uploadAvatar: 'Enviar Avatar',
    attachmentsRequired: 'Os anexos devem estar habilitados para enviar ícones do workspace',
    uploadRecommendation:
      'Recomendado: Imagens quadradas, pelo menos 256x256 pixels para melhor qualidade',
    avatarOrIconNote:
      'Você pode usar uma imagem de avatar personalizada ou a combinação de ícone e cor acima.',
    uploading: 'Enviando...',
    avatarUploadedSuccess: 'Avatar enviado com sucesso',

    // Categories tab
    projectCategoryRestrictions: 'Restrições de Categoria de Projeto',
    selectProjectCategories: 'Selecionar categorias de projeto...',
    categoryRestrictionsHelp:
      'Opcionalmente, restrinja a seleção de projetos a categorias específicas para este workspace. Quando definido, os usuários só podem selecionar projetos das categorias escolhidas.',
    leaveEmptyNote: 'Nota: Deixe vazio para permitir a seleção de todas as categorias de projeto.',

    // Configuration tab
    activeConfiguration: 'Configuração Ativa',

    // Danger zone
    permanentRemoval: 'Remoção Permanente',
    removeWarningIntro: 'Remover este workspace excluirá permanentemente:',
    removeWarningItems: 'Todos os itens de trabalho e projetos neste workspace',
    removeWarningFields: 'Todas as configurações de campos personalizados',
    removeWarningScreens: 'Todas as configurações de telas',
    removeWarningFiles: 'Todos os arquivos enviados associados a itens de trabalho',
    removeWarningFinal: 'Esta ação não pode ser desfeita.',
    removeWorkspaceButton: 'Remover Workspace',
    typeToConfirm: 'Digite {name} para confirmar a remoção:',
    typeNameHere: "Digite '{name}' aqui",
    yesRemoveWorkspace: 'Sim, Remover Workspace',

    // Actions and messages
    saveChanges: 'Salvar Alterações',
    saving: 'Salvando...',
    reset: 'Redefinir',
    remove: 'Remover',
    cancel: 'Cancelar',
    workspaceNotFound: 'Workspace não encontrado.',
    workspaceNameRequired: 'O nome do workspace é obrigatório',
    workspaceKeyRequired: 'A chave do workspace é obrigatória',
    savedSuccessfully: 'Configurações do workspace salvas com sucesso',
    failedToSave: 'Falha ao salvar configurações do workspace: {error}',
    deletedSuccessfully: 'Workspace "{name}" excluído com sucesso',
    failedToDelete: 'Falha ao excluir workspace: {error}',
    pleaseConfirmDeletion:
      'Por favor, digite o nome do workspace exatamente como mostrado para confirmar a exclusão',
    pleaseSelectImage: 'Por favor, selecione um arquivo de imagem',
    failedToUploadAvatar: 'Falha ao enviar avatar: {error}',
  },

  lookAndFeel: {
    title: 'Aparência e Estilo',
    subtitle: 'Personalize a aparência e o layout do seu workspace',
    displayModeTitle: 'Modo de Exibição',
    displayModeDescription:
      'Escolha como este workspace é exibido. Isso afeta o layout de navegação e o comportamento padrão.',
    gradientTitle: 'Fundo e Gradiente',
    gradientDescription: 'Escolha um esquema de cores para o seu workspace',
    gradients: 'Gradientes',
    backgroundImages: 'Imagens de Fundo',
    currentBackground: 'Fundo Atual',
    uploadCustomImage: 'Enviar Imagem Personalizada',
    backgroundUploadRecommendation:
      'Recomendado: Imagens de alta resolução (1920x1080 ou maior) para melhor qualidade',
    backgroundUploadedSuccess: 'Imagem de fundo enviada com sucesso',
    failedToUploadBackground: 'Falha ao enviar imagem de fundo: {error}',
    identityTitle: 'Identidade do Workspace',
    identityDescription: 'Personalize o ícone, a cor e o avatar do seu workspace',
    savedSuccessfully: 'Configurações de aparência salvas com sucesso',
    failedToSave: 'Falha ao salvar configurações de aparência: {error}',
    // Logo
    logo: 'Logo',
    currentLogo: 'Logo Atual',
    noLogoSet: 'Nenhum logo definido',
    uploadLogo: 'Enviar Logo',
    logoRecommendation:
      'Recomendado: PNG ou SVG com fundo transparente. Altura máxima no cabeçalho: 40-50px.',
  },
};
