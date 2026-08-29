export default {
  dashboard: {
    salutation: {
      withName: '{salutation}, {name}!',
      withoutName: '{salutation}!',
    },
    sections: {
      yourDay: {
        title: 'На сегодня',
        subtitle: 'Кратко о том, что требует вашего внимания',
      },
      work: {
        title: 'Работа',
        subtitle: 'Личные задачи и назначенные вам элементы',
      },
      workspaces: {
        title: 'Рабочие пространства',
        subtitle: 'Вернуться к работе',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'Сводка дня',
        description: 'Сформированная ИИ сводка важных событий на сегодня',
      },
      yourActivity: {
        name: 'Ваша активность',
        description: 'Недавно просмотренные, изменённые и прокомментированные элементы',
      },
      whatsNew: {
        name: 'Что нового',
        description: 'Последние уведомления и непрочитанные обновления',
      },
      personalTasks: {
        name: 'Личные задачи',
        description: 'Элементы из вашего личного списка задач',
      },
      savedSearch: {
        name: 'Сохранённый поиск',
        description: 'Рабочие элементы из сохранённой коллекции',
      },
      assignedToMe: {
        name: 'Назначено мне',
        description: 'Открытые элементы, назначенные вам во всех рабочих пространствах',
      },
      watchedItems: {
        name: 'Отслеживаемые элементы',
        description: 'Элементы, за которыми вы следите',
      },
      upcomingMilestones: {
        name: 'Ближайшие этапы',
        description: 'Этапы с приближающимися сроками',
      },
      recentWorkspaces: {
        name: 'Недавние пространства',
        description: 'Рабочие пространства, которые вы недавно посещали',
      },
      quickAccess: {
        name: 'Быстрый доступ',
        description: 'Ссылки на доступные рабочие пространства',
      },
    },
    customization: {
      widgets: 'Виджеты',
      activity: {
        name: 'Активность',
        description: 'Сводки, лента активности и уведомления',
      },
      work: {
        name: 'Работа',
        description: 'Элементы, этапы и назначенная вам работа',
      },
      navigation: {
        name: 'Навигация',
        description: 'Быстрый доступ к рабочим пространствам',
      },
      tipLabel: 'Совет',
      tip: 'Перетащите отсюда виджеты в любой раздел панели.',
    },
    editor: {
      newSection: 'Новый раздел',
      deleteSectionConfirm: 'Удалить раздел? Все виджеты в нём также будут удалены.',
      doneEditing: 'Завершить редактирование',
      customize: 'Настроить',
      editModeDescription: 'Режим редактирования: добавляйте, переименовывайте, перемещайте и удаляйте разделы и виджеты',
      addSection: 'Добавить раздел',
      sectionLabel: 'Раздел панели',
      sectionTitlePlaceholder: 'Название раздела',
      sectionSubtitlePlaceholder: 'Подзаголовок (необязательно)',
      renameSection: 'Переименовать раздел',
      deleteSection: 'Удалить раздел',
      unknownWidgetType: 'Неизвестный тип виджета: {type}',
      noWidgets: 'В этом разделе пока нет виджетов',
      addWidgetsHint: 'Нажмите «Настроить», чтобы добавить виджеты',
      noSections: 'На панели нет разделов',
      addSectionsHint: 'Нажмите «Изменить», чтобы добавить разделы на панель',
    },
    states: {
      assignedLoadError: 'Не удалось загрузить назначенные вам элементы',
      assignedEmpty: 'Сейчас вам ничего не назначено',
      personalTasksLoadError: 'Не удалось загрузить личные задачи',
      personalTasksEmpty: 'Ваш список личных задач пуст',
      dailyBriefingUnavailable: 'Сводка дня сейчас недоступна. Для неё требуется интеграция с ИИ. Если вы только что настроили интеграцию, попробуйте немного позже.',
      updatedAt: 'Обновлено: {time}',
      priorityLabel: 'Приоритет: {priority}',
      noWorkspaces: 'Рабочих пространств пока нет',
      createWorkspace: 'Создать пространство',
      workspaceAvatarAlt: 'Аватар пространства «{name}»',
      visited: 'посещено {time}',
      noUpcomingMilestones: 'Нет ближайших этапов',
      milestoneProgress: 'Выполнено: {done} из {total}',
      daysOverdue: 'Просрочено на {days} дн.',
      daysLeft: 'Осталось {days} дн.',
      watchedItemsEmpty: 'Вы пока не отслеживаете элементы',
      workspaceWithId: 'Рабочее пространство {id}',
    },
  },
};
