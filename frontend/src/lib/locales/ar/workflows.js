/**
 * Arabic (ar) - Workflows locale module
 * RTL language
 * Contains translations for statuses, priorities, workflows, screens, and status categories
 */

export default {
  statuses: {
    title: 'الحالات',
    subtitle: 'إدارة الحالات الفردية',
    status: 'حالة',
    statuses_one: 'حالة واحدة',
    statuses_other: '{count} حالات',
    createStatus: 'إنشاء حالة',
    editStatus: 'تعديل الحالة',
    deleteStatus: 'حذف الحالة',
    statusName: 'اسم الحالة',
    statusCategory: 'الفئة',
    noStatuses: 'لم يتم العثور على حالات',
    statusCreated: 'تم إنشاء الحالة بنجاح',
    statusUpdated: 'تم تحديث الحالة بنجاح',
    statusDeleted: 'تم حذف الحالة بنجاح'
  },

  priorities: {
    title: 'الأولويات',
    subtitle: 'تكوين مستويات الأولوية مع الأيقونات والألوان',
    priority: 'أولوية',
    priorities_one: 'أولوية واحدة',
    priorities_other: '{count} أولويات',
    createPriority: 'إنشاء أولوية',
    editPriority: 'تعديل الأولوية',
    deletePriority: 'حذف الأولوية',
    priorityName: 'اسم الأولوية',
    noPriorities: 'لم يتم العثور على أولويات',
    priorityCreated: 'تم إنشاء الأولوية بنجاح',
    priorityUpdated: 'تم تحديث الأولوية بنجاح',
    priorityDeleted: 'تم حذف الأولوية بنجاح',
    critical: 'حرج',
    high: 'عالي',
    medium: 'متوسط',
    low: 'منخفض',
    lowest: 'أدنى'
  },

  workflows: {
    title: 'سير العمل',
    subtitle: 'تكوين انتقالات سير العمل',
    workflow: 'سير عمل',
    workflows_one: 'سير عمل واحد',
    workflows_other: '{count} سير عمل',
    createWorkflow: 'إنشاء سير عمل',
    editWorkflow: 'تعديل سير العمل',
    deleteWorkflow: 'حذف سير العمل',
    workflowName: 'اسم سير العمل',
    transitions: 'الانتقالات',
    addTransition: 'إضافة انتقال',
    noWorkflows: 'لم يتم العثور على سير عمل',
    workflowCreated: 'تم إنشاء سير العمل بنجاح',
    workflowUpdated: 'تم تحديث سير العمل بنجاح',
    workflowDeleted: 'تم حذف سير العمل بنجاح'

    // Note: Additional workflow designer keys
    // fall back to English for missing keys
  },

  screens: {
    title: 'الشاشات',
    subtitle: 'تعريف تخطيطات الحقول للمشاكل والمشاريع',
    screen: 'شاشة',
    screens_one: 'شاشة واحدة',
    screens_other: '{count} شاشات',
    createScreen: 'إنشاء شاشة',
    editScreen: 'تعديل الشاشة',
    deleteScreen: 'حذف الشاشة',
    screenName: 'اسم الشاشة',
    noScreens: 'لم يتم العثور على شاشات',
    screenCreated: 'تم إنشاء الشاشة بنجاح',
    screenUpdated: 'تم تحديث الشاشة بنجاح',
    screenDeleted: 'تم حذف الشاشة بنجاح',
    cannotDeleteDefault: 'لا يمكن حذف الشاشة الافتراضية'
  },

  screensPage: {
    // Screens page section - falls back to English for missing keys
  },

  statusCategory: {
    // Status category section - falls back to English for missing keys
  }
};
