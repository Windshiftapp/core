export default {
  dashboard: {
    salutation: {
      withName: '{salutation}, {name}!',
      withoutName: '{salutation}!',
    },
    sections: {
      yourDay: {
        title: 'Seu dia',
        subtitle: 'Uma visão rápida do que precisa da sua atenção',
      },
      work: {
        title: 'Trabalho',
        subtitle: 'Sua lista pessoal e os itens atribuídos a você',
      },
      workspaces: {
        title: 'Espaços de trabalho',
        subtitle: 'Continue de onde parou',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'Resumo diário',
        description: 'Resumo gerado por IA do que importa para você hoje',
      },
      yourActivity: {
        name: 'Sua atividade',
        description: 'Itens que você viu, editou ou comentou recentemente',
      },
      whatsNew: {
        name: 'Novidades',
        description: 'Notificações recentes e atualizações não lidas',
      },
      personalTasks: {
        name: 'Tarefas pessoais',
        description: 'Itens da sua lista pessoal de tarefas',
      },
      savedSearch: {
        name: 'Pesquisa salva',
        description: 'Exibe itens de uma coleção salva',
      },
      assignedToMe: {
        name: 'Atribuídos a mim',
        description: 'Itens abertos atribuídos a você em todos os espaços de trabalho',
      },
      watchedItems: {
        name: 'Itens observados',
        description: 'Itens que você acompanha',
      },
      upcomingMilestones: {
        name: 'Próximos marcos',
        description: 'Marcos com datas-alvo próximas',
      },
      recentWorkspaces: {
        name: 'Espaços de trabalho recentes',
        description: 'Espaços de trabalho visitados recentemente',
      },
      quickAccess: {
        name: 'Acesso rápido',
        description: 'Links rápidos para os espaços de trabalho disponíveis',
      },
    },
    customization: {
      widgets: 'Widgets',
      activity: {
        name: 'Atividade',
        description: 'Resumos, fluxos de atividade e notificações',
      },
      work: {
        name: 'Trabalho',
        description: 'Itens, marcos e tarefas atribuídos a você',
      },
      navigation: {
        name: 'Navegação',
        description: 'Acesso rápido aos espaços de trabalho',
      },
      tipLabel: 'Dica',
      tip: 'Arraste widgets daqui para qualquer seção do seu painel.',
    },
    editor: {
      newSection: 'Nova seção',
      deleteSectionConfirm: 'Excluir esta seção? Todos os widgets nela serão removidos.',
      doneEditing: 'Concluir edição',
      customize: 'Personalizar',
      editModeDescription: 'Modo de edição: adicione, renomeie, reordene ou exclua seções e widgets',
      addSection: 'Adicionar seção',
      sectionLabel: 'Seção do painel',
      sectionTitlePlaceholder: 'Título da seção',
      sectionSubtitlePlaceholder: 'Subtítulo (opcional)',
      renameSection: 'Renomear seção',
      deleteSection: 'Excluir seção',
      unknownWidgetType: 'Tipo de widget desconhecido: {type}',
      noWidgets: 'Ainda não há widgets nesta seção',
      addWidgetsHint: 'Selecione Personalizar para adicionar widgets',
      noSections: 'Nenhuma seção configurada',
      addSectionsHint: 'Selecione Editar para adicionar seções ao painel',
    },
    states: {
      assignedLoadError: 'Não foi possível carregar os itens atribuídos a você',
      assignedEmpty: 'Nada foi atribuído a você no momento',
      personalTasksLoadError: 'Não foi possível carregar suas tarefas pessoais',
      personalTasksEmpty: 'Sua lista pessoal de tarefas está vazia',
      dailyBriefingUnavailable: 'Seu resumo diário não está disponível no momento. Ele depende de uma integração de IA. Se você acabou de configurar uma, tente novamente em instantes.',
      updatedAt: 'Atualizado em {time}',
      priorityLabel: 'Prioridade: {priority}',
      noWorkspaces: 'Ainda não há espaços de trabalho',
      createWorkspace: 'Criar um',
      workspaceAvatarAlt: 'Avatar de {name}',
      visited: 'visitado {time}',
      noUpcomingMilestones: 'Nenhum marco próximo',
      milestoneProgress: '{done} de {total} concluídos',
      daysOverdue: '{days} dias de atraso',
      daysLeft: 'Faltam {days} dias',
      watchedItemsEmpty: 'Você não está observando nenhum item',
      workspaceWithId: 'Espaço de trabalho {id}',
    },
  },
};
