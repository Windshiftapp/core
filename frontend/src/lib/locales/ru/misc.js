/**
 * Прочие функции приложения — русская локализация.
 */

export default {
  jiraImport: {
    title: { cloud: 'Импорт из Jira Cloud', datacenter: 'Импорт из Jira Data Center' },
    subtitle: { cloud: 'Импорт рабочих элементов из Jira Cloud', datacenter: 'Импорт рабочих элементов из Jira Data Center' },
    steps: { connect: 'Подключение', projects: 'Проекты', xray: 'Xray', mapping: 'Сопоставление', preview: 'Предпросмотр', import: 'Импорт' },
    deploymentType: { cloud: 'Jira Cloud', cloudDesc: '*.atlassian.net', datacenter: 'Data Center', datacenterDesc: 'Собственный сервер' },
    form: {
      urlCloud: 'URL Jira Cloud', urlDatacenter: 'URL Jira Data Center', email: 'Адрес электронной почты', username: 'Имя пользователя', apiToken: 'API-токен', personalAccessToken: 'Персональный токен доступа', generateToken: 'Создать токен', tokenHelpCloud: 'в настройках учётной записи Atlassian', tokenHelpDatacenter: 'Создайте персональный токен доступа в настройках профиля Jira',
    },
    buttons: {
      connect: 'Подключить', continue: 'Продолжить', analyzeAndConfigure: 'Проанализировать и настроить', startImport: 'Начать импорт', done: 'Готово', back: 'Назад', cancel: 'Отмена', selectAll: 'Выбрать всё', deselectAll: 'Снять выделение', addNewConnection: 'Добавить подключение', useExisting: 'Использовать существующее', retryImport: 'Повторить импорт',
    },
    messages: {
      connected: 'Подключено к {name}',
      selectConnection: 'Выберите существующее подключение или создайте новое',
      credentialsHelpCloud: 'Введите учётные данные Jira Cloud. API-токен можно создать в настройках учётной записи Atlassian.',
      credentialsHelpDatacenter: 'Введите персональный токен доступа из настроек профиля Jira Data Center.',
      reviewSummary: 'Проверьте сводку перед импортом. Для крупных проектов операция может занять несколько минут.',
      noAttachments: 'Вложения не будут импортированы',
      noAttachmentsDesc: 'Хранилище вложений не настроено. Чтобы импортировать вложения, задайте параметр --attachment-path при запуске сервера.',
    },
    projects: {
      selected: 'Выберите проекты для импорта: {selected} из {total}',
      openIssuesOnly: 'Импортировать только открытые задачи',
      openIssuesOnlyDesc: 'Исключает задачи с категорией статуса «Done»',
      teamManaged: 'Управляется командой',
      issues: 'Задач: {count}',
    },
    mapping: {
      workspaces: 'Рабочие пространства', workspacesDesc: 'Каждый проект Jira станет рабочим пространством Windshift',
      issueTypes: 'Типы задач', issueTypesDesc: 'Типы задач будут созданы в Windshift как типы элементов',
      statuses: 'Статусы', statusesDesc: 'Статусы будут созданы и сгруппированы по категориям',
      customFields: 'Настраиваемые поля', customFieldsDesc: 'Поля, которые можно сопоставить, будут созданы в Windshift',
      versions: 'Версии / этапы', versionsDesc: 'Версии Jira будут импортированы как этапы рабочего пространства.',
      subtask: 'Подзадача', create: 'Создать', skip: 'Пропустить',
    },
    preview: {
      workspaces: 'Рабочие пространства', workItems: 'Рабочие элементы', statuses: 'Статусы', itemTypes: 'Типы элементов', customFields: 'Настраиваемые поля', milestones: 'Этапы', users: 'Пользователи', usersNew: '({count} новых)', assets: 'Ресурсы', projectsToImport: 'Импортируемые проекты',
    },
    import: {
      importing: 'Импорт…', starting: 'Запуск импорта…', complete: 'Импорт завершён!', completeWithErrors: 'Импорт завершён с ошибками', success: 'Успешно импортировано элементов: {count}.', failed: 'Не удалось импортировать элементов: {count}.', ready: 'Готово к импорту', readyDesc: 'Нажмите «Начать импорт», чтобы импортировать элементы ({count}).', progress: 'Ход выполнения',
    },
    errors: {
      connectionFailed: 'Не удалось подключиться к Jira', loadProjectsFailed: 'Не удалось загрузить проекты', analyzeFailed: 'Не удалось проанализировать проекты', importFailed: 'Не удалось запустить импорт', loadConnectionsFailed: 'Не удалось загрузить подключения', deleteConnectionFailed: 'Не удалось удалить подключение',
    },
  },

  itemTypes: {
    title: 'Типы элементов', subtitle: 'Настройка типов элементов и их свойств', itemType: 'Тип элемента',
    itemTypes_one: '{count} тип элемента', itemTypes_few: '{count} типа элемента', itemTypes_many: '{count} типов элементов', itemTypes_other: '{count} типа элемента',
    createItemType: 'Создать тип элемента', editItemType: 'Изменить тип элемента', deleteItemType: 'Удалить тип элемента', typeName: 'Название типа', noItemTypes: 'Типы элементов не найдены', itemTypeCreated: 'Тип элемента создан', itemTypeUpdated: 'Тип элемента обновлён', itemTypeDeleted: 'Тип элемента удалён',
  },

  fields: {
    title: 'Настраиваемые поля', requestFormFields: 'Поля формы запроса', subtitle: 'Настройка дополнительных полей элементов', field: 'Поле',
    fields_one: '{count} поле', fields_few: '{count} поля', fields_many: '{count} полей', fields_other: '{count} поля',
    createField: 'Создать поле', editField: 'Изменить поле', deleteField: 'Удалить поле', returnToBoard: 'Вернуться к полям доски', fieldName: 'Название поля', fieldType: 'Тип поля', fieldDescription: 'Описание', defaultValue: 'Значение по умолчанию', placeholder: 'Текст в пустом поле', helpText: 'Подсказка', noFields: 'Поля не найдены', fieldCreated: 'Поле создано', fieldUpdated: 'Поле обновлено', fieldDeleted: 'Поле удалено', configureFields: 'Настроить поля', searchFields: 'Поиск полей…', indexSettings: 'Настройки индекса',
    text: 'Текст', number: 'Число', date: 'Дата', datetime: 'Дата и время', select: 'Выбор', multiSelect: 'Множественный выбор', checkbox: 'Флажок', user: 'Пользователь', url: 'URL',
    milestoneHint: 'Поля этапов автоматически ссылаются на системные этапы. При заполнении поля пользователи смогут выбрать существующий этап.',
    dateHint: 'Поля дат позволяют выбрать дату в календаре. Значения хранятся в формате YYYY-MM-DD.',
    assetHint: 'Поля ресурсов позволяют выбрать ресурс из указанного набора. Доступные ресурсы можно дополнительно отфильтровать запросом QL.',
    portalCustomerHint: 'Поля заказчика портала ссылаются на заказчиков. Используйте currentCustomer() в отчётах по ресурсам для фильтрации по вошедшему заказчику.',
    customerOrganisationHint: 'Поля организации заказчика ссылаются на организации. Используйте currentOrganisation() в отчётах по ресурсам для фильтрации по организации заказчика.',
    usedIn: 'Используется в', portalCustomers: 'Заказчики портала', customerOrganisations: 'Организации заказчиков',
  },

  categories: {
    title: 'Категории', subtitle: 'Управление категориями', category: 'Категория',
    categories_one: '{count} категория', categories_few: '{count} категории', categories_many: '{count} категорий', categories_other: '{count} категории',
    createCategory: 'Создать категорию', editCategory: 'Изменить категорию', deleteCategory: 'Удалить категорию', categoryName: 'Название категории', noCategories: 'Категории не найдены', noCategorizedWork: 'Работы с категорией пока нет', categoryCreated: 'Категория создана', categoryUpdated: 'Категория обновлена', categoryDeleted: 'Категория удалена', deleteWarning: 'Элементы этой категории останутся без категории', selectCategory: 'Выберите категорию', uncategorized: 'Без категории', addNewCategory: 'Добавить категорию', addCategory: 'Добавить категорию', categoryNamePlaceholder: 'Название категории…', existingCategories: 'Существующие категории', confirmDeleteCategory: 'Удалить категорию «{name}»? Её элементы останутся без категории.', failedToDeleteCategory: 'Не удалось удалить категорию. Возможно, она всё ещё используется.', noCategoriesYet: 'Категорий пока нет', addFirstCategoryHint: 'Добавьте первую категорию выше.',
  },

  projects: {
    title: 'Проекты', subtitle: 'Управление проектами', project: 'Проект',
    projects_one: '{count} проект', projects_few: '{count} проекта', projects_many: '{count} проектов', projects_other: '{count} проекта',
    createProject: 'Создать проект', editProject: 'Изменить проект', deleteProject: 'Удалить проект', projectName: 'Название проекта', projectKey: 'Ключ проекта', noProjects: 'Проекты не найдены', searchProjects: 'Поиск проектов…', loadingProjects: 'Загрузка проектов…', projectCreated: 'Проект создан', projectUpdated: 'Проект обновлён', projectDeleted: 'Проект удалён',
  },

  iterations: {
    title: 'Итерации', subtitle: 'Управление итерациями и выпусками', iteration: 'Итерация',
    iterations_one: '{count} итерация', iterations_few: '{count} итерации', iterations_many: '{count} итераций', iterations_other: '{count} итерации',
    createIteration: 'Создать итерацию', editIteration: 'Изменить итерацию', updateIteration: 'Обновить итерацию', deleteIteration: 'Удалить итерацию', startIteration: 'Начать итерацию', completeIteration: 'Завершить итерацию', start: 'Начать',
    noIterations: 'Итерации не найдены', allTypes: 'Все типы', manageTypes: 'Управление типами', backToIterations: 'Вернуться к итерациям', filterByIteration: 'Фильтр по итерации', addGlobalIteration: 'Добавить итерацию',
    status: { planned: 'Запланирована', active: 'Активна', completed: 'Завершена', cancelled: 'Отменена' },
    statusPlanned: 'Запланирована', statusActive: 'Активна', statusCompleted: 'Завершена', statusCancelled: 'Отменена',
    global: 'Глобальная', local: 'Локальная', thisWorkspace: 'Это рабочее пространство', globalIteration: 'Глобальная итерация', globalIterations: 'Глобальные итерации', globalIterationDescription: 'Видна во всех рабочих пространствах', localIteration: 'Локальная итерация', localIterations: 'Локальные итерации', localIterationDescription: 'Видна только в этом рабочем пространстве', switchTo: 'Переключиться на область «{scope}»', scope: 'Область',
    daysOverdue: 'Просрочено на {count} дн.', endsToday: 'Завершается сегодня', oneDayRemaining: 'Остался 1 день', daysRemaining: 'Осталось дней: {count}', overdue: 'Просрочена', complete: 'завершено', dateRange: 'Диапазон дат', startDate: 'Дата начала', endDate: 'Дата окончания', burndownChart: 'Диаграмма сгорания', idealProgress: 'Идеальный план', ideal: 'Идеальный', noBurndownData: 'Нет данных для диаграммы сгорания',
    noItems: 'Нет элементов', summary: 'Сводка', totalItems: 'Всего элементов', completed: 'Завершено', remaining: 'Осталось', byStatusCategory: 'По категории статуса', noStatusData: 'Нет данных о статусах', workItems: 'Рабочие элементы', noItemsAssigned: 'Элементы не назначены', assignItemsHint: 'Назначьте рабочие элементы этой итерации, чтобы отслеживать ход работы',
    iterationName: 'Название итерации', iterationNamePlaceholder: 'Например, спринт 1, Q1 2025 или выпуск 2.0', iterationDescriptionPlaceholder: 'Описание или цели итерации (необязательно)', descriptionPlaceholder: 'Описание (необязательно)', selectStatus: 'Выберите статус…', selectType: 'Выберите тип…', noType: 'Без типа',
    iterationNameRequired: 'Укажите название итерации', startDateRequired: 'Укажите дату начала', endDateRequired: 'Укажите дату окончания', endDateMustBeAfterStart: 'Дата окончания должна быть позже даты начала', typeRequired: 'Выберите тип итерации', failedToSaveIteration: 'Не удалось сохранить итерацию',
    confirmDelete: 'Удалить итерацию «{name}»?', completeIterationConfirm: 'Завершить итерацию «{name}»?', iterationStarted: 'Итерация «{name}» начата', iterationCompleted: 'Итерация «{name}» завершена', activeScopeWarning: 'Не рекомендуется менять область активной итерации',
    itemsDone: 'Готово элементов: {count}', itemsIncomplete: 'Не завершено: {count}', allItemsDone: 'Все элементы готовы!', moveToBacklog: 'Переместить в бэклог', moveToIteration: 'Переместить в другую итерацию', incompleteItemsAction: 'Незавершённые элементы:',
  },

  milestones: {
    title: 'Этапы', milestone: 'Этап', subtitle: 'Отслеживание выпусков и сроков', addMilestone: 'Добавить этап', noMilestones: 'Этапов пока нет', noMilestonesDescription: 'Создайте первый этап, чтобы отслеживать выпуски и сроки.', noMilestonesInCategory: 'В этой категории нет этапов', allCategories: 'Все категории', manageCategories: 'Управление категориями', allMilestones: 'Все этапы', workspaceMilestones: 'Этапы рабочего пространства', columnStatus: 'Статус', columnMilestone: 'Этап', columnTargetDate: 'Целевая дата', columnTimeline: 'Сроки', columnTests: 'Тесты', visibleCount: '{count} этапов', visibleCount_one: '{count} этап', visibleCount_few: '{count} этапа', visibleCount_many: '{count} этапов', visibleCount_other: '{count} этапа', visibleInCategory: '{count} этапов в этой категории', visibleInCategory_one: '{count} этап в этой категории', visibleInCategory_few: '{count} этапа в этой категории', visibleInCategory_many: '{count} этапов в этой категории', visibleInCategory_other: '{count} этапа в этой категории',
    status: { planning: 'Планирование', inProgress: 'В работе', completed: 'Завершён', cancelled: 'Отменён' },
    daysOverdue: 'Просрочено на {count} дн.', dueToday: 'Срок сегодня', oneDayRemaining: 'Остался 1 день', daysRemaining: 'Осталось дней: {count}', confirmDelete: 'Удалить этап «{name}»?', complete: 'завершено', noItems: 'Нет элементов', byStatusCategory: 'По категории статуса', noStatusData: 'Нет данных о статусах', workItems: 'Рабочие элементы', noItemsAssigned: 'Элементы не назначены', assignItemsHint: 'Назначьте рабочие элементы этому этапу, чтобы отслеживать ход работы', milestoneName: 'Название этапа', milestoneNamePlaceholder: 'Например, выпуск Q1 или запуск бета-версии', targetDate: 'Целевая дата', selectStatus: 'Выберите статус…', descriptionPlaceholder: 'Описание этапа (необязательно)', noCategory: 'Без категории', overdue: 'Просрочен', openEnded: 'Без срока', scope: 'Область', global: 'Глобальный', local: 'Локальный', globalMilestone: 'Глобальный этап', globalMilestones: 'Глобальные этапы', globalMilestoneDescription: 'Виден во всех рабочих пространствах', localMilestone: 'Локальный этап', localMilestones: 'Локальные этапы', localMilestoneDescription: 'Виден только в этом рабочем пространстве', switchTo: 'Переключиться на область «{scope}»', workspace: 'Рабочее пространство', selectWorkspace: 'Выберите рабочее пространство', manageMilestoneCategories: 'Управление категориями этапов', hideCompleted: 'Скрывать завершённые', hideCompletedHelp: 'Не показывать завершённые этапы в списке', noVisibleMilestones: 'Нет видимых этапов', dragToReorder: 'Перетащите, чтобы изменить порядок',
  },

  assets: {
    title: 'Управление ресурсами', subtitle: 'Управление наборами, типами, категориями и ресурсами', selectAssetSet: 'Выберите набор ресурсов', newSet: 'Новый набор', createAssetSet: 'Создать набор ресурсов', editSet: 'Изменить набор', deleteSet: 'Удалить набор', noAssetSets: 'Нет наборов ресурсов', noAssetSetsDesc: 'Создайте первый набор, чтобы начать управлять ресурсами.', selectAnAssetSet: 'Выберите набор ресурсов', selectAnAssetSetDesc: 'Выберите набор выше, чтобы просматривать ресурсы и управлять ими.', default: 'По умолчанию', assetTag: 'Тег ресурса', preview: 'Предпросмотр', serialNumber: 'Серийный номер',
    types: 'Типы', categories: 'Категории', permissions: 'Разрешения', automations: 'Автоматизации',
    newType: 'Новый тип', createType: 'Создать тип', editType: 'Изменить тип', noAssetTypes: 'Нет типов ресурсов', noAssetTypesDesc: 'Создайте типы, чтобы классифицировать ресурсы.', assetType: 'Тип ресурса',
    newCategory: 'Новая категория', createCategory: 'Создать категорию', editCategory: 'Изменить категорию', noCategories: 'Нет категорий', noCategoriesDesc: 'Создайте категории, чтобы упорядочить ресурсы.', parentCategory: 'Родительская категория', noParent: 'Без родителя (верхний уровень)',
    assignRole: 'Назначить роль', everyoneRole: 'Общая роль', everyoneRoleDesc: 'Стандартная роль для всех пользователей. Индивидуальные назначения имеют приоритет.', noRoleAssignments: 'Роли не назначены', noRoleAssignmentsDesc: 'Назначьте роли, чтобы управлять доступом к набору ресурсов.', assignee: 'Получатель', role: 'Роль',
    failedToDownload: 'Не удалось скачать файл', editDiagram: 'Изменить диаграмму', untitledDiagram: 'Диаграмма без названия', diagram: 'диаграмма', uploadedBy: 'Загрузил {name}', zoomOut: 'Уменьшить (-)', zoomIn: 'Увеличить (+)', rotate: 'Повернуть (R)', fitToScreen: 'Вписать в экран (F)', shortcutScroll: 'Колесо: масштаб', shortcutDrag: 'Перетаскивание: перемещение', shortcutRotate: 'R: повернуть', shortcutFit: 'F: вписать', shortcutReset: '0: сбросить', shortcutClose: 'Esc: закрыть',
  },

  personal: {
    myTasks: 'Мои задачи', reviews: 'Обзоры', weeklyCalendar: 'Календарь на неделю', personalTasks: 'Личные задачи', addPersonalTask: 'Добавить личную задачу', noPersonalWorkspace: 'Личное рабочее пространство не найдено', taskTitlePlaceholder: 'Название задачи…', noTasksYet: 'Задач пока нет', noOpenTasks: 'Открытых задач нет', markIncomplete: 'Отметить незавершённой', markComplete: 'Отметить завершённой', openTask: 'Открыть задачу', unlinkTask: 'Удалить связь с задачей', setDueDate: 'Указать срок', comments: 'Комментарии',
    personalReview: 'Личный обзор', today: 'сегодня', daily: 'Ежедневный', weekly: 'Еженедельный', dailyReview: 'Ежедневный обзор', weeklyReview: 'Еженедельный обзор', recentReviews: 'Недавние обзоры', noPreviousReviews: 'Предыдущие обзоры не найдены', exitFocusMode: 'Выйти из режима фокусировки', enterFocusMode: 'Войти в режим фокусировки', saveReview: 'Сохранить обзор',
    completedToday: 'Завершено сегодня', completedThisWeek: 'Завершено на этой неделе', loadingCompletedItems: 'Загрузка завершённых элементов…', noCompletedItemsDay: 'За этот день нет завершённых элементов', noCompletedItemsWeek: 'За эту неделю нет завершённых элементов',
    reflection: 'Рефлексия', whatAccomplished: 'Что я сделал сегодня?', whatWentWell: 'Что сегодня получилось хорошо?', whatImprove: 'Что можно улучшить завтра?', weeklyAccomplishments: 'Что мы сделали на этой неделе?', weeklyChallenges: 'С какими трудностями мы столкнулись?', weeklyPriorities: 'Каковы наши приоритеты на следующую неделю?', placeholderAccomplishments: 'Опишите основные достижения…', placeholderWentWell: 'Что получилось хорошо и почему…', placeholderImprovements: 'Что улучшить и какие сделать следующие шаги…', startWriting: 'Начните писать рефлексию…',
  },

  audit: {
    title: 'Журнал аудита', subtitle: 'Отслеживание административных действий и событий безопасности', event: 'Событие', user: 'Пользователь', action: 'Действие', resource: 'Ресурс', timestamp: 'Время', details: 'Сведения', ipAddress: 'IP-адрес', noEvents: 'События аудита не найдены',
  },
  connections: {
    title: 'Подключения', subtitle: 'Управление внешними интеграциями', connection: 'Подключение', createConnection: 'Создать подключение', editConnection: 'Изменить подключение', deleteConnection: 'Удалить подключение', connectionType: 'Тип подключения', noConnections: 'Подключения не найдены', connectionCreated: 'Подключение создано', connectionUpdated: 'Подключение обновлено', connectionDeleted: 'Подключение удалено', connectionSuccessful: 'Подключение установлено', testConnection: 'Проверить подключение',
  },
  migration: {
    title: 'Миграция', subtitle: 'Перенос данных между системами', migrateConfiguration: 'Перенести конфигурацию', migrationCompleted: 'Миграция завершена', migrationSuccess: 'Все элементы успешно перенесены', targetWorkspace: 'Целевое рабочее пространство', targetWorkspaceRequired: 'Выберите целевое рабочее пространство',
  },
  members: { title: 'Участники', subtitle: 'Управление участниками команды', addMember: 'Добавить участника', removeMember: 'Удалить участника', searchMembers: 'Поиск по имени или электронной почте…' },
  configuration: { title: 'Конфигурация', searchConfigurationSets: 'Поиск наборов конфигураций…' },

  auditLog: {
    filters: 'Фильтры', actionType: 'Тип действия', resourceType: 'Тип ресурса', status: 'Статус', searchPlaceholder: 'Пользователь, ресурс…', startDate: 'Дата начала', endDate: 'Дата окончания', applyFilters: 'Применить фильтры', clearFilters: 'Сбросить фильтры', loadingAuditLogs: 'Загрузка журнала аудита…', noAuditLogs: 'Записи аудита не найдены', exportCsv: 'Экспорт в CSV', exportJson: 'Экспорт в JSON', auditLogDetails: 'Сведения записи аудита', timestamp: 'Время', user: 'Пользователь', action: 'Действие', resource: 'Ресурс', resourceName: 'Название ресурса', ipAddress: 'IP-адрес', userAgent: 'User-Agent', errorMessage: 'Сообщение об ошибке', additionalDetails: 'Дополнительные сведения', success: 'Успешно', failed: 'Ошибка', all: 'Все',
    allActions: 'Все действия', userCreated: 'Пользователь создан', userUpdated: 'Пользователь обновлён', userDeleted: 'Пользователь удалён', userActivated: 'Пользователь активирован', userDeactivated: 'Пользователь деактивирован', passwordReset: 'Пароль сброшен', apiTokenCreated: 'API-токен создан', apiTokenRevoked: 'API-токен отозван', workspaceCreated: 'Рабочее пространство создано', workspaceUpdated: 'Рабочее пространство обновлено', workspaceDeleted: 'Рабочее пространство удалено', groupCreated: 'Группа создана', groupUpdated: 'Группа обновлена', groupDeleted: 'Группа удалена', groupMemberAdded: 'Участник добавлен в группу', groupMemberRemoved: 'Участник удалён из группы', customFieldCreated: 'Настраиваемое поле создано', customFieldUpdated: 'Настраиваемое поле обновлено', customFieldDeleted: 'Настраиваемое поле удалено', itemTypeCreated: 'Тип элемента создан', itemTypeUpdated: 'Тип элемента обновлён', itemTypeDeleted: 'Тип элемента удалён', permissionGranted: 'Разрешение выдано', permissionRevoked: 'Разрешение отозвано', roleAssigned: 'Роль назначена', roleRevoked: 'Роль отозвана',
    allResources: 'Все ресурсы', apiToken: 'API-токен', customField: 'Настраиваемое поле', itemType: 'Тип элемента', permission: 'Разрешение', group: 'Группа',
  },

  migrationAssistant: {
    configSetMigration: 'Перенос набора конфигураций', workflowMigration: 'Перенос рабочего процесса', migratingFrom: 'Перенос из', to: 'в', configurationSet: 'Набор конфигураций', analyzingMigration: 'Проверка совместимости…', analysisFailed: 'Ошибка анализа', noMigrationRequired: 'Миграция не требуется', allItemsCompatible: 'Все элементы ({count}) совместимы с новой конфигурацией.', migrationRequired: 'Нужно перенести данные', itemsNeedMigration: 'Требуется перенести элементы ({count}). Проверьте сопоставления ниже.', itemTypes: 'Типы элементов', fields: 'Поля', status: 'Статус', priority: 'Приоритет', itemTypeMigrations: 'Изменения типов элементов', customFieldChanges: 'Изменения настраиваемых полей', statusMigrations: 'Изменения статусов', priorityMigrations: 'Изменения приоритетов', noItemsToMigrate: 'Нет элементов для миграции.', noStatusChanges: 'Изменения статусов не обнаружены.', noFieldChanges: 'Изменения полей не обнаружены.', selectTargetType: 'Выберите целевой тип…', selectTargetStatus: 'Выберите целевой статус…', selectTargetPriority: 'Выберите целевой приоритет…', compatible: 'Совместимо', requiresMigration: 'Требует миграции', item: 'элемент', items: 'элементы', kept: 'Сохранено', values: 'значения', hiddenDataPreserved: 'Скрыто (данные сохранены)', newRequiredField: 'Новое обязательное поле', enterDefaultValue: 'Введите значение по умолчанию…', executeMigration: 'Перенести данные', migrating: 'Перенос данных…', migrationCompleted: 'Миграция завершена', allItemsMigrated: 'Все элементы успешно перенесены.', pleaseSelectTargetStatuses: 'Выберите целевые статусы для всех элементов, требующих миграции.', pleaseSelectTargetItemTypes: 'Выберите целевые типы для всех элементов, требующих миграции.', pleaseProvideDefaultValues: 'Укажите значения по умолчанию для всех новых обязательных полей.', pleaseSelectTargetPriorities: 'Выберите целевые приоритеты для всех элементов, требующих миграции.',
  },

  setup: {
    chooseLanguage: 'Выберите язык', chooseLanguageDesc: 'Выберите язык, который будет использоваться во время настройки и в приложении.', language: 'Язык', welcomeTo: 'Добро пожаловать в {appName}', setupMessage: 'Настроим вашу систему управления работой', setupProgress: 'Ход настройки', step: 'Шаг', of: 'из', createAdminAccount: 'Создание учётной записи администратора', adminAccountDesc: 'Эта учётная запись получит полный доступ к установке {appName}.', firstName: 'Имя', lastName: 'Фамилия', emailAddress: 'Адрес электронной почты', username: 'Имя пользователя', password: 'Пароль', confirmPassword: 'Повторите пароль', configureModules: 'Настройка модулей', configureModulesDesc: 'Выберите модули, которые нужно включить. Позже настройки можно изменить.', testManagement: 'Управление тестированием', testManagementDesc: 'Управление тест-кейсами, запусками и контролем качества', setupComplete: 'Настройка завершена', setupCompleteMessage: '{appName} готово к работе. Сейчас вы перейдёте в приложение.', whatsNext: 'Что дальше?', whatsNextCreateWorkspace: 'Создайте первое рабочее пространство', whatsNextSetupWorkflows: 'Настройте рабочие процессы и экраны', whatsNextInviteTeam: 'Пригласите участников команды', whatsNextStartCreating: 'Начните создавать рабочие элементы', back: 'Назад', next: 'Далее', completeSetup: 'Завершить настройку', settingUp: 'Настройка…', fillAllRequired: 'Заполните все обязательные поля', passwordsMustMatch: 'Пароли не совпадают', invalidEmail: 'Введите корректный адрес электронной почты', setupError: 'Во время настройки произошла ошибка. Повторите попытку.', goBackEsc: 'Назад (Esc)', continueNextStepEnter: 'Следующий шаг (Enter)', completeSetupEnter: 'Завершить настройку (Enter)',
  },

  createModal: {
    workItem: 'Рабочий элемент', milestone: 'Этап', workspace: 'Рабочее пространство', collection: 'Коллекция', planning: 'Планирование', inProgress: 'В работе', completed: 'Завершено', cancelled: 'Отменено', newChildItem: 'Новый дочерний элемент', new: 'Создать', issueTitle: 'Название задачи', milestoneName: 'Название этапа', workspaceName: 'Название: {type}', workspaceKeyPlaceholder: 'Ключ пространства, например PROJ или TEAM', addDescription: 'Добавить описание…', workspaceTemplate: 'Шаблон', workspaceTemplateBlank: 'Пустое рабочее пространство', workspaceTemplateLoading: 'Загрузка шаблонов…', workspaceTemplateError: 'Не удалось загрузить шаблоны рабочих пространств', workspaceTemplateMeta: 'Шаблонов: {templates} · элементов: {items}', type: 'Тип', template: 'Шаблон', priority: 'Приоритет', noPriority: 'Без приоритета', assignee: 'Исполнитель', unassigned: 'Не назначено', dueDate: 'Срок', milestoneField: 'Этап', noMilestone: 'Без этапа', additionalFields: 'Дополнительные поля', targetDate: 'Целевая дата', status: 'Статус', category: 'Категория', noCategory: 'Без категории', parent: 'Родитель', fillRequiredFields: 'Заполните обязательные поля:', selectWorkspaceFirst: 'Сначала выберите рабочее пространство', create: 'Создать',
  },

  scm: {
    createBranch: 'Создать ветку', createBranchFor: 'Создать ветку для {itemKey}', repository: 'Репозиторий', selectRepository: 'Выберите репозиторий…', branchName: 'Название ветки', baseBranch: 'Базовая ветка', baseBranchHelp: 'Ветка, от которой создаётся новая. По умолчанию используется основная ветка репозитория.', creating: 'Создание…', branchCreatedSuccess: 'Ветка создана', noReposLinked: 'С этим рабочим пространством не связаны репозитории', linkReposHelp: 'Свяжите репозитории в разделе «Настройки рабочего пространства → Управление кодом»', fillAllRequired: 'Заполните все обязательные поля', failedToLoadRepos: 'Не удалось загрузить репозитории', failedToCreateBranch: 'Не удалось создать ветку',
    createPullRequest: 'Создать запрос на слияние', createMergeRequest: 'Создать Merge Request', createPRFrom: 'Создать PR из ветки {branch}', prTitle: 'Название PR', description: 'Описание', baseBranchPR: 'Базовая ветка', baseBranchPRHelp: 'Оставьте пустым, чтобы использовать основную ветку репозитория.', createPR: 'Создать PR', prCreatedSuccess: 'Запрос на слияние №{prNumber} создан', mrCreatedSuccess: 'Merge Request !{prNumber} создан', failedToCreatePR: 'Не удалось создать запрос на слияние', noBranchLink: 'Ссылка на ветку не указана',
    linkDevResource: 'Связать с разработкой', linkDevResourceDesc: 'Добавьте связь с PR, веткой или коммитом', type: 'Тип', pr: 'PR', branch: 'Ветка', commit: 'Коммит', prNumber: 'Номер PR', commitSha: 'SHA коммита', titleOptional: 'Название (необязательно)', linkResource: 'Добавить связь', linking: 'Создание связи…', failedToCreateLink: 'Не удалось создать связь',
    development: 'Разработка', linkExisting: 'Добавить существующую связь', failedToStartConnection: 'Не удалось начать подключение', failedToLoadLinks: 'Не удалось загрузить связи', confirmRemoveLink: 'Удалить эту связь?', noRepositoriesLinked: 'С этим рабочим пространством не связаны репозитории', connectYourAccount: 'Подключите учётную запись {provider}', connectToCreate: 'Подключитесь, чтобы создавать ветки и запросы на слияние из рабочих элементов.', connect: 'Подключить {provider}', noLinksYet: 'PR, ветки и коммиты пока не связаны', pullRequests: 'Запросы на слияние', mergeRequests: 'Merge Requests', branches: 'Ветки', commits: 'Коммиты', smartCommitsTitle: 'Выполнять команды из коммитов', smartCommitsDescription: 'При слиянии выполнять команды «#comment …» и «#<transition-slug>» из описания PR и сообщений коммитов.', smartCommitsWarningTitle: 'Адрес автора коммита невозможно проверить', smartCommitsWarningBody: 'Команды из коммита выполняются от имени пользователя, чей адрес совпадает с адресом автора в Git. Git не подтверждает этот адрес: автор коммита может указать любое значение. Включайте функцию только для репозиториев, участникам которых вы доверяете.', smartCommitsUpdateFailed: 'Не удалось обновить настройку команд из коммитов',
  },

  organization: {
    editOrganization: 'Изменить организацию', newOrganization: 'Новая организация', organizationName: 'Название организации', email: 'Электронная почта', organizationAvatar: 'Аватар организации', customAvatar: 'Собственный аватар', uploadedImage: 'Загруженное изображение', defaultAvatar: 'Стандартный аватар', usingInitials: 'Используются инициалы', changeAvatar: 'Изменить аватар', uploadAvatar: 'Загрузить аватар', attachmentsRequired: 'Для загрузки аватара организации необходимо включить вложения', uploadRecommendation: 'Рекомендуется квадратное изображение не меньше 256 × 256 пикселей', activeOrganization: 'Организация активна', customFields: 'Настраиваемые поля', updateOrganization: 'Обновить организацию', createOrganization: 'Создать организацию', pleaseSelectImage: 'Выберите файл изображения', failedToUploadAvatar: 'Не удалось загрузить аватар',
  },
  integrations: {
    title: 'Интеграции', linkPage: 'Связать страницу', searchPages: 'Поиск страниц…', noLinksYet: 'Связанных страниц пока нет', connectAccount: 'Подключите учётную запись', connectToLink: 'Подключитесь, чтобы связывать страницы', connect: 'Подключить', disconnect: 'Отключить', connected: 'Подключено', confirmRemoveLink: 'Удалить эту связь?', confirmDisconnect: 'Отключить эту интеграцию?', failedToLoadLinks: 'Не удалось загрузить связи', failedToSearch: 'Не удалось найти страницы', refreshed: 'Связь обновлена', linked: 'Страница связана', removed: 'Связь удалена', disconnected: 'Интеграция отключена', providerManager: 'Внешние интеграции', providerManagerDesc: 'Подключайте сервисы, из которых Windshift будет получать данные', addProvider: 'Добавить интеграцию', editProvider: 'Настроить интеграцию', providerType: 'Сервис', oauthClientId: 'ID клиента OAuth', oauthClientSecret: 'Секрет клиента OAuth', callbackUrl: 'URL перенаправления OAuth', callbackUrlHint: 'Скопируйте этот адрес в настройки приложения', providerConfig: 'Параметры сервиса (JSON)', notion: 'Notion', noProviders: 'Внешние интеграции пока не настроены', selectProvider: 'Выберите сервис', page: 'Страница', database: 'База данных',
  },
};
