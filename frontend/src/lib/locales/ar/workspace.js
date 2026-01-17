/**
 * Arabic (ar) - Workspace-related translations
 * RTL language
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
    title: 'مساحات العمل',
    subtitle: 'إدارة مساحات العمل والمشاريع',
    workspace: 'مساحة عمل',
    workspaces_one: 'مساحة عمل واحدة',
    workspaces_other: '{count} مساحات عمل',
    createWorkspace: 'إنشاء مساحة عمل',
    editWorkspace: 'تعديل مساحة العمل',
    deleteWorkspace: 'حذف مساحة العمل',
    switchWorkspace: 'تبديل مساحة العمل',
    workspaceName: 'اسم مساحة العمل',
    workspaceKey: 'مفتاح مساحة العمل',
    workspaceDescription: 'الوصف',
    members: 'الأعضاء',
    settings: 'إعدادات مساحة العمل',
    noWorkspaces: 'لم يتم العثور على مساحات عمل',
    selectWorkspace: 'اختر مساحة عمل',
    currentWorkspace: 'مساحة العمل الحالية',
    workspaceCreated: 'تم إنشاء مساحة العمل بنجاح',
    workspaceUpdated: 'تم تحديث مساحة العمل بنجاح',
    workspaceDeleted: 'تم حذف مساحة العمل بنجاح',
    customers: {
      title: 'العملاء',
      subtitle: 'إدارة عملاء ومؤسسات البوابة',
      addCustomer: 'إضافة عميل',
      unassignedCustomers: 'العملاء غير المعينين',
      customerCount: '{count} عميل',
      failedToLoadCustomers: 'فشل تحميل العملاء',
      failedToLoadOrganisations: 'فشل تحميل المؤسسات',
      failedToAssignCustomer: 'فشل تعيين العميل للمؤسسة',
      deleteCustomer: 'حذف العميل',
      confirmDeleteCustomer: 'هل أنت متأكد أنك تريد حذف "{name}"؟',
      manageOrganisations: 'إدارة المؤسسات',
      searchOrganisations: 'البحث عن المؤسسات...',
      noOrganisationsFound: 'لم يتم العثور على مؤسسات',
      noCustomersFound: 'لم يتم العثور على عملاء',
      unassigned: 'غير معين',
      allCustomersAssigned: 'تم تعيين جميع العملاء للمؤسسات'
    }
  },

  items: {
    title: 'العناصر',
    subtitle: 'عرض وإدارة عناصر العمل',
    item: 'عنصر',
    items_one: 'عنصر واحد',
    items_other: '{count} عناصر',
    createItem: 'إنشاء عنصر',
    editItem: 'تعديل العنصر',
    deleteItem: 'حذف العنصر',
    viewItem: 'عرض العنصر',
    itemKey: 'المفتاح',
    itemTitle: 'العنوان',
    itemDescription: 'الوصف',
    itemType: 'نوع العنصر',
    itemStatus: 'الحالة',
    itemPriority: 'الأولوية',
    assignee: 'المكلف',
    reporter: 'المُبلغ',
    dueDate: 'تاريخ الاستحقاق',
    startDate: 'تاريخ البدء',
    estimate: 'التقدير',
    timeSpent: 'الوقت المستغرق',
    remaining: 'المتبقي',
    parent: 'الأصل',
    children: 'الفروع',
    subtasks: 'المهام الفرعية',
    linkedItems: 'العناصر المرتبطة',
    attachments: 'المرفقات',
    comments: 'التعليقات',
    activity: 'النشاط',
    history: 'السجل',
    noItems: 'لم يتم العثور على عناصر',
    noItemsInFilter: 'لا توجد عناصر تطابق الفلتر الحالي',
    createToStart: 'أنشئ عنصراً للبدء',
    itemCreated: 'تم إنشاء العنصر بنجاح',
    itemUpdated: 'تم تحديث العنصر بنجاح',
    itemDeleted: 'تم حذف العنصر بنجاح',
    assignedToYou: 'معين إليك',
    createdByYou: 'تم إنشاؤه بواسطتك',
    recentlyViewed: 'شوهد مؤخراً',
    recentlyUpdated: 'تم التحديث مؤخراً'
  },

  comments: {
    // Comments section - falls back to English for missing keys
  },

  todo: {
    // Todo section - falls back to English for missing keys
  },

  collectionTree: {
    // Collection tree section - falls back to English for missing keys
  },

  collections: {
    // Collections section - falls back to English for missing keys
  },

  links: {
    title: 'الروابط',
    subtitle: 'إدارة روابط العناصر',
    addLink: 'إضافة رابط',
    removeLink: 'إزالة الرابط',
    linkText: 'نص الرابط',
    linkUrl: 'الرابط'
  }
};
