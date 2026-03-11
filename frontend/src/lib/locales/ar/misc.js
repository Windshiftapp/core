/**
 * Arabic (ar) - Miscellaneous locale strings
 * RTL language
 * Contains: sprints, iterations, milestones, assets, personal, connections,
 * migration, migrationAssistant, setup, createModal, scm, organization,
 * fields, itemTypes, categories, members, configuration, audit, auditLog, projects
 */

export default {
  itemTypes: {
    title: 'أنواع العناصر',
    subtitle: 'تكوين أنواع العناصر وخصائصها',
    itemType: 'نوع العنصر',
    itemTypes_one: 'نوع عنصر واحد',
    itemTypes_other: '{count} أنواع عناصر',
    createItemType: 'إنشاء نوع عنصر',
    editItemType: 'تعديل نوع العنصر',
    deleteItemType: 'حذف نوع العنصر',
    typeName: 'اسم النوع',
    noItemTypes: 'لم يتم العثور على أنواع عناصر',
    itemTypeCreated: 'تم إنشاء نوع العنصر بنجاح',
    itemTypeUpdated: 'تم تحديث نوع العنصر بنجاح',
    itemTypeDeleted: 'تم حذف نوع العنصر بنجاح',
  },

  fields: {
    title: 'الحقول المخصصة',
    subtitle: 'تعريف حقول مخصصة لعناصرك',
    field: 'حقل',
    fields_one: 'حقل واحد',
    fields_other: '{count} حقول',
    createField: 'إنشاء حقل',
    editField: 'تعديل الحقل',
    deleteField: 'حذف الحقل',
    fieldName: 'اسم الحقل',
    fieldType: 'نوع الحقل',
    fieldDescription: 'الوصف',
    defaultValue: 'القيمة الافتراضية',
    placeholder: 'نص توضيحي',
    helpText: 'نص المساعدة',
    noFields: 'لم يتم العثور على حقول',
    fieldCreated: 'تم إنشاء الحقل بنجاح',
    fieldUpdated: 'تم تحديث الحقل بنجاح',
    fieldDeleted: 'تم حذف الحقل بنجاح',
    configureFields: 'تكوين الحقول',
    searchFields: 'البحث في الحقول...',
    indexSettings: 'إعدادات الفهرس',
    text: 'نص',
    number: 'رقم',
    date: 'تاريخ',
    datetime: 'تاريخ ووقت',
    select: 'اختيار',
    multiSelect: 'اختيار متعدد',
    checkbox: 'مربع اختيار',
    user: 'مستخدم',
    url: 'رابط',
    usedIn: 'مستخدم في',
    portalCustomers: 'عملاء البوابة',
    customerOrganisations: 'منظمات العملاء',
  },

  categories: {
    title: 'الفئات',
    subtitle: 'إدارة الفئات',
    category: 'فئة',
    categories_one: 'فئة واحدة',
    categories_other: '{count} فئات',
    createCategory: 'إنشاء فئة',
    editCategory: 'تعديل الفئة',
    deleteCategory: 'حذف الفئة',
    categoryName: 'اسم الفئة',
    noCategories: 'لم يتم العثور على فئات',
    noCategorizedWork: 'لا يوجد عمل مصنف بعد',
    categoryCreated: 'تم إنشاء الفئة بنجاح',
    categoryUpdated: 'تم تحديث الفئة بنجاح',
    categoryDeleted: 'تم حذف الفئة بنجاح',
    deleteWarning: 'ستصبح العناصر في هذه الفئة غير مصنفة',
    selectCategory: 'اختر فئة',
    uncategorized: 'غير مصنف',
  },

  projects: {
    title: 'المشاريع',
    subtitle: 'إدارة مشاريعك',
    project: 'مشروع',
    projects_one: 'مشروع واحد',
    projects_other: '{count} مشاريع',
    createProject: 'إنشاء مشروع',
    editProject: 'تعديل المشروع',
    deleteProject: 'حذف المشروع',
    projectName: 'اسم المشروع',
    projectKey: 'مفتاح المشروع',
    noProjects: 'لم يتم العثور على مشاريع',
    searchProjects: 'البحث في المشاريع...',
    loadingProjects: 'جاري تحميل المشاريع...',
    projectCreated: 'تم إنشاء المشروع بنجاح',
    projectUpdated: 'تم تحديث المشروع بنجاح',
    projectDeleted: 'تم حذف المشروع بنجاح',
  },

  sprints: {
    title: 'السبرنتات',
    subtitle: 'إدارة تكرارات السبرنت',
    sprint: 'سبرنت',
    sprints_one: 'سبرنت واحد',
    sprints_other: '{count} سبرنتات',
    createSprint: 'إنشاء سبرنت',
    editSprint: 'تعديل السبرنت',
    deleteSprint: 'حذف السبرنت',
    startSprint: 'بدء السبرنت',
    completeSprint: 'إكمال السبرنت',
    sprintName: 'اسم السبرنت',
    sprintGoal: 'هدف السبرنت',
    noSprints: 'لم يتم العثور على سبرنتات',
    backlog: 'قائمة المهام المعلقة',
    sprintCreated: 'تم إنشاء السبرنت بنجاح',
    sprintUpdated: 'تم تحديث السبرنت بنجاح',
    sprintDeleted: 'تم حذف السبرنت بنجاح',
    sprintStarted: 'تم بدء السبرنت بنجاح',
    sprintCompleted: 'تم إكمال السبرنت بنجاح',

    // Note: Additional sprint/iteration modal keys
    // fall back to English for missing keys
  },

  iterations: {
    title: 'التكرارات',
    subtitle: 'إدارة السبرنتات والإصدارات',

    // Note: Additional iterations keys
    // fall back to English for missing keys
  },

  milestones: {
    title: 'المعالم',
    subtitle: 'تتبع الإصدارات والمواعيد النهائية',

    // Note: Additional milestones keys
    // fall back to English for missing keys
  },

  assets: {
    // Assets section - falls back to English for missing keys
  },

  personal: {
    // Personal section - falls back to English for missing keys
  },

  audit: {
    title: 'سجل التدقيق',
    subtitle: 'تتبع ومراجعة جميع الإجراءات الإدارية وأحداث الأمان',
    event: 'الحدث',
    user: 'المستخدم',
    action: 'الإجراء',
    resource: 'المورد',
    timestamp: 'الطابع الزمني',
    details: 'التفاصيل',
    ipAddress: 'عنوان IP',
    noEvents: 'لم يتم العثور على أحداث تدقيق',
  },

  connections: {
    title: 'الاتصالات',
    subtitle: 'إدارة التكاملات الخارجية',
    connection: 'اتصال',
    createConnection: 'إنشاء اتصال',
    editConnection: 'تعديل الاتصال',
    deleteConnection: 'حذف الاتصال',
    connectionType: 'نوع الاتصال',
    noConnections: 'لم يتم العثور على اتصالات',
    connectionCreated: 'تم إنشاء الاتصال بنجاح',
    connectionUpdated: 'تم تحديث الاتصال بنجاح',
    connectionDeleted: 'تم حذف الاتصال',
    connectionSuccessful: 'الاتصال ناجح',
    testConnection: 'اختبار الاتصال',
  },

  migration: {
    title: 'الترحيل',
    subtitle: 'ترحيل البيانات بين الأنظمة',
    migrateConfiguration: 'ترحيل التكوين',
    migrationCompleted: 'اكتمل الترحيل',
    migrationSuccess: 'تم ترحيل جميع العناصر بنجاح',
    targetWorkspace: 'مساحة العمل المستهدفة',
    targetWorkspaceRequired: 'مساحة العمل المستهدفة مطلوبة',
  },

  members: {
    title: 'الأعضاء',
    subtitle: 'إدارة أعضاء الفريق',
    addMember: 'إضافة عضو',
    removeMember: 'إزالة العضو',
    searchMembers: 'البحث عن أعضاء بالاسم أو البريد الإلكتروني...',
  },

  configuration: {
    title: 'التكوين',
    searchConfigurationSets: 'البحث في مجموعات التكوين...',
  },

  auditLog: {
    // Audit log section - falls back to English for missing keys
  },

  migrationAssistant: {
    // Migration assistant section - falls back to English for missing keys
  },

  setup: {
    goBackEsc: 'رجوع (Esc)',
    continueNextStepEnter: 'متابعة للخطوة التالية (Enter)',
    completeSetupEnter: 'إكمال الإعداد (Enter)',

    // Note: Additional setup keys
    // fall back to English for missing keys
  },

  createModal: {
    // Create modal section - falls back to English for missing keys
  },

  scm: {
    // SCM section - falls back to English for missing keys
  },

  organization: {
    // Organization section - falls back to English for missing keys
  },
};
