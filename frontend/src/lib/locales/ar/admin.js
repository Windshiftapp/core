/**
 * Arabic (ar) - Admin locale strings
 * RTL language
 * Contains settings, roles, and permissions translations
 */

export default {
  // Settings (merged from general and detailed settings sections)
  settings: {
    title: 'الإعدادات',
    subtitle: 'تكوين إعدادات التطبيق',
    general: 'عام',
    appearance: 'المظهر',
    notifications: 'الإشعارات',
    security: 'الأمان',
    privacy: 'الخصوصية',
    language: 'اللغة',
    timezone: 'المنطقة الزمنية',
    dateFormat: 'تنسيق التاريخ',
    timeFormat: 'تنسيق الوقت',
    theme: 'السمة',
    lightMode: 'الوضع الفاتح',
    darkMode: 'الوضع الداكن',
    systemDefault: 'افتراضي النظام',
    admin: 'الإدارة',
    systemSettings: 'إعدادات النظام',
    organizationSettings: 'إعدادات المؤسسة',
    workspaceSettings: 'إعدادات مساحة العمل'

    // Note: Additional admin settings sections (attachments, groups, notifications, etc.)
    // fall back to English for missing keys
  },

  // Roles and Permissions
  roles: {
    title: 'الأدوار',
    subtitle: 'إدارة الأدوار ومستويات الوصول',
    role: 'دور',
    roles_one: 'دور واحد',
    roles_other: '{count} أدوار',
    createRole: 'إنشاء دور',
    editRole: 'تعديل الدور',
    deleteRole: 'حذف الدور',
    roleName: 'اسم الدور',
    roleDescription: 'الوصف',
    permissions: 'الصلاحيات',
    members: 'الأعضاء',
    assignRole: 'تعيين دور',
    removeRole: 'إزالة الدور',
    noRoles: 'لم يتم العثور على أدوار',
    roleCreated: 'تم إنشاء الدور بنجاح',
    roleUpdated: 'تم تحديث الدور بنجاح',
    roleDeleted: 'تم حذف الدور بنجاح',
    cannotDeleteSystemRole: 'لا يمكن حذف أدوار النظام'
  },

  // Permissions
  permissions: {
    title: 'الصلاحيات',
    subtitle: 'تكوين مجموعات الصلاحيات',
    permission: 'صلاحية',
    permissionSet: 'مجموعة صلاحيات',
    grantPermission: 'منح صلاحية',
    revokePermission: 'إلغاء صلاحية',
    read: 'قراءة',
    write: 'كتابة',
    delete: 'حذف',
    admin: 'مسؤول',
    manage: 'إدارة',
    view: 'عرض',
    edit: 'تعديل',
    create: 'إنشاء'
  }
};
