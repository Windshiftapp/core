/**
 * Actions automation translations (Brazilian Portuguese)
 */
export default {
  actions: {
    title: 'Ações',
    description: 'Automatize fluxos de trabalho com ações baseadas em regras',
    create: 'Criar Ação',
    createFirst: 'Crie Sua Primeira Ação',
    noActions: 'Nenhuma ação ainda',
    noActionsDescription: 'Crie ações para automatizar seus fluxos de trabalho com base em eventos de itens',
    enabled: 'Ativada',
    disabled: 'Desativada',
    enable: 'Ativar',
    disable: 'Desativar',
    viewLogs: 'Ver Logs',
    confirmDelete: 'Tem certeza de que deseja excluir a ação "{name}"?',
    failedToSave: 'Falha ao salvar ação',
    newAction: 'Nova Ação',

    // Trigger types
    trigger: {
      statusTransition: 'Transição de Status',
      itemCreated: 'Item Criado',
      itemUpdated: 'Item Atualizado',
      itemLinked: 'Item Vinculado',
      manual: 'Manual',
      respondToCascades: 'Responder a alterações disparadas por ações',
      respondToCascadesHint:
        'Quando ativada, esta ação também será executada quando disparada por outras ações, não apenas por alterações do usuário.',
    },

    // Node types
    nodes: {
      trigger: 'Gatilho',
      setField: 'Definir Campo',
      setStatus: 'Definir Status',
      addComment: 'Adicionar Comentário',
      notifyUser: 'Notificar Usuário',
      condition: 'Condição',
      updateAsset: 'Atualizar Ativo',
      createAsset: 'Criar Ativo',
    },

    // Node palette and tips
    addNodes: 'Adicionar Nós',
    tips: 'Dicas',
    tipDragToConnect: 'Arraste das alças para conectar nós',
    tipClickToEdit: 'Clique em um nó para configurá-lo',
    tipConditionBranches: 'Condições têm ramificações verdadeiro/falso',

    // Config panel
    nodeConfig: 'Configuração do Nó',
    config: {
      from: 'De',
      to: 'Para',
      selectField: 'Selecionar campo...',
      selectStatus: 'Selecionar status...',
      enterComment: 'Digite o comentário...',
      selectRecipient: 'Selecionar destinatário...',
      setCondition: 'Definir condição...',
      targetStatus: 'Status de Destino',
      fieldName: 'Nome do Campo',
      value: 'Valor',
      commentContent: 'Conteúdo do Comentário',
      commentPlaceholder: 'Digite o texto do comentário. Use {{item.title}} para variáveis.',
      privateComment: 'Comentário privado (apenas interno)',
      fieldToCheck: 'Campo a Verificar',
      operator: 'Operador',
      compareValue: 'Valor de Comparação',
      private: 'Privado',
      triggerType: 'Tipo de Gatilho',
      fromStatus: 'Status de Origem',
      toStatus: 'Status de Destino',
      anyStatus: 'Qualquer Status',
      recipientType: 'Destinatário',
      notifyMessage: 'Mensagem',
      notifyPlaceholder: 'Digite a mensagem. Use {{item.title}} para variáveis.',
      includeLink: 'Incluir link para o item',
      // Update Asset config
      sourceAssetField: 'Campo de Ativo no Item',
      selectAssetField: 'Selecionar campo de ativo...',
      sourceAssetFieldHint: 'Selecione o campo do item que contém o ativo vinculado',
      targetAssetType: 'Tipo de Ativo de Destino',
      selectAssetType: 'Selecionar tipo de ativo...',
      fieldMappingsLabel: 'Mapeamentos de Campos',
      fieldMappings: '{count} mapeamento(s) de campo',
      configureAssetUpdate: 'Configurar atualização de ativo...',
      fromField: 'Do campo',
      sourceTypeVariable: 'Variável/Template',
      sourceTypeItemField: 'Campo do Item',
      sourceTypeLiteral: 'Valor Literal',
      selectTargetField: 'Selecionar campo de destino...',
      addMapping: 'Adicionar Mapeamento',
      // Create Asset config
      assetSet: 'Conjunto de Ativos',
      selectAssetSet: 'Selecionar conjunto de ativos...',
      assetTitle: 'Título do Ativo',
      assetTitleHint: 'Use {{item.title}} ou outras variáveis',
      assetDescription: 'Descrição',
      assetTagLabel: 'Tag do Ativo',
      assetCategory: 'Categoria',
      selectCategory: 'Selecionar categoria (opcional)...',
      assetStatus: 'Status',
      selectStatusOptional: 'Selecionar status (opcional)...',
      requiredField: 'Obrigatório',
      configureAssetCreation: 'Configurar criação de ativo...',
    },

    // Recipients
    recipients: {
      assignee: 'Responsável',
      creator: 'Criador',
      specific: 'Usuários Específicos',
    },

    // Condition
    condition: {
      true: 'Sim',
      false: 'Não',
    },

    // Operators
    operators: {
      equals: 'Igual a',
      notEquals: 'Diferente de',
      contains: 'Contém',
      greaterThan: 'Maior que',
      lessThan: 'Menor que',
      isEmpty: 'Está Vazio',
      isNotEmpty: 'Não Está Vazio',
    },

    // Execution logs
    logs: {
      title: 'Logs de Execução',
      noLogs: 'Nenhum log de execução',
      status: 'Status',
      running: 'Em Execução',
      completed: 'Concluído',
      failed: 'Falhou',
      skipped: 'Ignorado',
      startedAt: 'Iniciado em',
      completedAt: 'Concluído em',
      error: 'Erro',
      details: 'Detalhes',
      viewDetails: 'Ver Detalhes',
    },

    // Execution trace
    trace: {
      title: 'Detalhes da Execução',
      noSteps: 'Nenhuma etapa de execução registrada',
      setStatus: 'Status alterado de "{from}" para "{to}"',
      setField: '{field} alterado de "{from}" para "{to}"',
      addComment: 'Comentário {prefix}adicionado: "{content}"',
      notifyUser: 'Notificação enviada para {count} usuário(s)',
      notifySkipped: 'Notificação ignorada: {reason}',
      conditionResult: 'Condição avaliada como {result}',
      updateAsset: 'Ativo #{asset_id} atualizado',
      updateAssetSkipped: 'Atualização de ativo ignorada: {reason}',
      createAsset: 'Ativo #{asset_id} criado: {title}',
      createAssetFailed: 'Falha na criação do ativo: {reason}',
    },

    // Test/manual execution
    test: {
      title: 'Testar Ação',
      description:
        'Selecione um item para executar esta ação. Isso executará a ação imediatamente, ignorando o gatilho normal.',
      selectItem: 'Selecionar Item',
      itemPlaceholder: 'Buscar um item...',
      execute: 'Executar Ação',
      run: 'Execução de Teste',
      executionFailed: 'Falha ao executar ação',
      executionQueued: 'Ação enfileirada para execução',
    },

    // Placeholder reference
    placeholders: {
      title: 'Placeholders Disponíveis',
      description:
        'Use esses placeholders no seu template. Eles serão substituídos pelos valores reais quando a ação for executada.',
      showReference: 'Mostrar referência de placeholders',
      categories: {
        item: 'Campos do Item',
        user: 'Usuário Atual',
        old: 'Valores Anteriores',
        trigger: 'Contexto do Gatilho',
      },
      item: {
        title: 'Título do item',
        id: 'ID do item',
        statusId: 'ID do status',
        assigneeId: 'ID do usuário responsável',
        any: 'Qualquer campo do item',
      },
      user: {
        name: 'Nome completo do usuário',
        email: 'E-mail do usuário',
        id: 'ID do usuário',
      },
      old: {
        description: 'Valor anterior antes da alteração',
        example: 'Valor anterior de qualquer campo',
      },
      trigger: {
        itemId: 'ID do item que disparou',
        workspaceId: 'ID do workspace',
      },
    },
    switchToVertical: 'Mudar para layout vertical',
    switchToHorizontal: 'Mudar para layout horizontal',
  },
};
