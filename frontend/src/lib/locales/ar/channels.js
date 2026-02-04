/**
 * Arabic (ar) - Channels, Notifications, Portal, and Request Form translations
 * RTL language
 *
 * This module contains translations for:
 * - notifications: Notification UI and email verification
 * - channels: Channel management and integration settings
 * - channel: Channel configuration (portal, webhook, email)
 * - portal: Customer portal settings and request types
 * - requestForm: Portal request submission form
 * - requestTypeFields: Field configuration for request types
 */

export default {
  notifications: {
    title: 'الإشعارات',
    subtitle: 'إدارة إعدادات الإشعارات',
    notification: 'إشعار',
    notifications_one: 'إشعار واحد',
    notifications_other: '{count} إشعارات',
    markAsRead: 'وضع علامة مقروء',
    markAllAsRead: 'وضع علامة مقروء للكل',
    clearAll: 'مسح الكل',
    noNotifications: 'لا توجد إشعارات',
    notificationSettings: 'إعدادات الإشعارات',
    emailNotifications: 'إشعارات البريد الإلكتروني',
    pushNotifications: 'إشعارات الدفع',
    channels: 'القنوات',
    searchNotifications: 'البحث في الإشعارات...',

    // Note: Additional notifications keys (email verification, etc.)
    // fall back to English for missing keys
  },

  channels: {
    title: 'القنوات',
    subtitle: 'تكوين قنوات الإشعارات',
    channel: 'قناة',
    channels_one: 'قناة واحدة',
    channels_other: '{count} قنوات',
    createChannel: 'إنشاء قناة',
    editChannel: 'تعديل القناة',
    deleteChannel: 'حذف القناة',
    channelName: 'اسم القناة',
    channelType: 'نوع القناة',
    noChannels: 'لم يتم العثور على قنوات',
    channelCreated: 'تم إنشاء القناة بنجاح',
    channelUpdated: 'تم تحديث القناة بنجاح',
    channelDeleted: 'تم حذف القناة بنجاح',
    cannotDeleteDefault: 'لا يمكن حذف قناة الإشعارات الافتراضية',
    searchChannels: 'البحث في القنوات...',
    testWebhook: 'اختبار Webhook',
    webhookSent: 'تم إرسال اختبار Webhook بنجاح!',
    webhookFailed: 'فشل اختبار Webhook',

    // Note: Additional channels keys (types, portal, webhook, email, categories)
    // fall back to English for missing keys
  },

  channel: {
    enableEmail: 'تفعيل قناة البريد الإلكتروني',
    emailIsActive: 'قناة البريد الإلكتروني نشطة وتعالج الرسائل',
    emailIsInactive: 'قناة البريد الإلكتروني معطلة حاليًا',

    // Processing Log
    processingLog: 'سجل المعالجة',
    emailLog: {
      syncStatus: 'حالة المزامنة',
      lastChecked: 'آخر فحص',
      never: 'أبداً',
      errors: 'أخطاء',
      noErrors: 'لا أخطاء',
      from: 'من',
      subject: 'الموضوع',
      result: 'النتيجة',
      processedAt: 'تمت المعالجة',
      newItem: 'عنصر جديد {key}',
      commentOn: 'تعليق على {key}',
      noEmails: 'لم تتم معالجة أي رسائل بعد',
      previous: 'السابق',
      next: 'التالي',
      page: 'صفحة {page} من {total}',
    },
  },

  portal: {
    title: 'البوابة',
    subtitle: 'إعدادات بوابة العملاء',
    portalTitle: 'عنوان البوابة',
    portalDescription: 'وصف البوابة (اختياري)',
    portalSlug: 'معرف البوابة',
    requestTypes: 'أنواع الطلبات',
    createRequestType: 'إنشاء نوع طلب',
    editRequestType: 'تعديل نوع الطلب',
    addRequestTypeSubtitle: 'إضافة نوع طلب جديد لبوابتك',
    editRequestTypeSubtitle: 'تحديث تفاصيل نوع الطلب',
    requestTypeCreated: 'تم إنشاء نوع الطلب بنجاح!',
    requestTypeUpdated: 'تم تحديث نوع الطلب بنجاح!',
    exampleTitle: 'مثال: بوابة دعم العملاء',
    exampleStatus: 'مثال: مفتوح، قيد التقدم، تم الحل',
    describeRequest: 'يرجى وصف طلبك',
    removeFromSection: 'إزالة من القسم',
    removeLink: 'إزالة الرابط',
    backToApp: 'العودة للتطبيق',
    customize: 'تخصيص',
    lightMode: 'الوضع الفاتح',
    darkMode: 'الوضع الداكن',
    signIn: 'تسجيل الدخول',
    signOut: 'تسجيل الخروج',
    myRequests: 'طلباتي',
    backToPortal: 'العودة للبوابة',
    guestUser: 'مستخدم زائر',
    notSignedIn: 'غير مسجل الدخول',
    addLink: 'إضافة رابط',
    addSection: 'إضافة قسم',
    noContentSections: 'لم يتم تكوين أقسام محتوى بعد.',
    dropHereToAdd: 'اسحب هنا لإضافة نوع الطلب',
    noRequestTypesInSection:
      'لا توجد أنواع طلبات في هذا القسم بعد. اسحب أنواع الطلبات هنا من الشريط الجانبي.',
  },

  requestForm: {
    // Request form section - falls back to English for missing keys
  },

  requestTypeFields: {
    addNewStep: 'إضافة خطوة جديدة',
    removeCurrentStep: 'إزالة الخطوة الحالية',
    removeField: 'إزالة الحقل',

    // Note: Additional request type fields keys
    // fall back to English for missing keys
  },
};
