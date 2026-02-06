/**
 * Arabic (ar) - UI-related translations
 * RTL language
 * Contains: pickers, editors, dialogs, components, aria, layout, widgets, footer
 */

export default {
  pickers: {
    // Pickers section - falls back to English for missing keys
  },

  editors: {
    // Editors section - falls back to English for missing keys
  },

  dialogs: {
    cancel: 'إلغاء',
    confirm: 'تأكيد',
    save: 'حفظ',
    close: 'إغلاق',
    delete: 'حذف',
    update: 'تحديث',
    // Confirmation messages for confirm() dialogs
    confirmations: {
      deleteItem: 'هل أنت متأكد أنك تريد حذف "{name}"؟ لا يمكن التراجع عن هذا الإجراء.',
      deleteSection: 'هل أنت متأكد أنك تريد حذف هذا القسم؟',
      discardChanges: 'لديك تغييرات غير محفوظة. هل أنت متأكد أنك تريد الإلغاء؟',
      dismissAllNotifications:
        'هل أنت متأكد أنك تريد تجاهل جميع الإشعارات؟ لا يمكن التراجع عن هذا الإجراء.',
      removeAvatar: 'هل أنت متأكد أنك تريد إزالة صورة ملفك الشخصي؟',
      revokeCalendarFeed:
        'هل أنت متأكد أنك تريد إلغاء رابط تغذية التقويم؟ ستتوقف التقويمات التي تستخدم هذا الرابط عن المزامنة.',
      deleteTheme: 'هل أنت متأكد أنك تريد حذف هذا المظهر؟ لا يمكن التراجع عن هذا الإجراء.',
      resetBoardConfig:
        'هل أنت متأكد أنك تريد إعادة التعيين إلى تكوين اللوحة الافتراضي؟ سيتم حذف تكوينك المخصص.',
      deleteCustomField:
        'هل أنت متأكد أنك تريد حذف الحقل المخصص "{name}"؟ سيتم إزالته من جميع المشاريع.',
      deleteLinkType:
        'هل أنت متأكد أنك تريد حذف نوع الرابط هذا؟ سيتم أيضًا إزالة جميع الروابط من هذا النوع.',
      deleteAsset: 'هل أنت متأكد أنك تريد حذف هذا الأصل؟',
      deleteAssetSet:
        'هل أنت متأكد أنك تريد حذف مجموعة الأصول هذه؟ سيتم حذف جميع الأصول والأنواع والفئات بداخلها.',
      deleteAssetType:
        'هل أنت متأكد أنك تريد حذف نوع الأصل هذا؟ الأصول التي تستخدم هذا النوع لن يكون لها نوع معين.',
      deleteCategory:
        'هل أنت متأكد أنك تريد حذف هذه الفئة؟ سيتم نقل الفئات الفرعية إلى الفئة الأم.',
      revokeRole: 'هل أنت متأكد أنك تريد إلغاء هذا الدور؟',
      quitApplication: 'هل أنت متأكد أنك تريد إنهاء التطبيق؟ سيتم إيقاف الخادم.',
      deleteConnection: 'هل أنت متأكد أنك تريد حذف هذا الاتصال؟ لا يمكن التراجع عن هذا الإجراء.',
      deleteWidget: 'حذف هذا القسم؟ سيتم إزالة جميع الأدوات في هذا القسم.',
    },
    // Alert messages for alert() dialogs
    alerts: {
      nameRequired: 'الاسم مطلوب',
      pleaseSelectImage: 'الرجاء تحديد ملف صورة',
      timerAlreadyRunning: 'المؤقت يعمل بالفعل. يرجى إيقافه قبل بدء مؤقت جديد.',
      noTimerRunning: 'لا يوجد مؤقت يعمل حاليًا.',
      timerSyncing: 'المؤقت قيد المزامنة. يرجى الانتظار والمحاولة مرة أخرى.',
      startTimerFromItem: 'يرجى بدء المؤقت من داخل عنصر عمل لتوفير السياق.',
      cannotDeleteDefaultScreen:
        'لا يمكن حذف الشاشة الافتراضية. هذه الشاشة مطلوبة لمساحات العمل بدون مجموعة تكوين.',
      applicationShuttingDown: 'جارٍ إيقاف التطبيق...',
      pdfExportComingSoon: 'تصدير PDF لعرض كتلة الوقت قادم قريبًا',
      configUpdatedSuccess:
        'تم تحديث مجموعة التكوين بنجاح. جميع عناصر العمل تستخدم بالفعل حالات من سير العمل الجديد.',
      failedToSave: 'فشل في الحفظ: {error}',
      failedToDelete: 'فشل في الحذف: {error}',
      failedToUpdate: 'فشل في التحديث: {error}',
      failedToLoad: 'فشل في التحميل: {error}',
      failedToCreate: 'فشل في الإنشاء: {error}',
      failedToUpload: 'فشل في الرفع: {error}',
      failedToGeneratePdf: 'فشل في إنشاء PDF. يرجى المحاولة مرة أخرى.',
      failedToApplyConfig: 'فشل في تطبيق تغيير التكوين: {error}',
      failedToAddManager: 'فشل في إضافة المدير: {error}',
      failedToRemoveManager: 'فشل في إزالة المدير: {error}',
      failedToSaveWorkspace: 'فشل في حفظ المشروع. يرجى التحقق من المدخلات والمحاولة مرة أخرى.',
      failedToResetConfig: 'فشل في إعادة تعيين التكوين: {error}',
      failedToToggleStatus: 'فشل في تبديل حالة نوع الرابط: {error}',
      failedToAssignRole: 'فشل في تعيين الدور: {error}',
      failedToRevokeRole: 'فشل في إلغاء الدور: {error}',
      failedToUpdateRole: 'فشل في تحديث دور الجميع: {error}',
      failedToLoadFields: 'فشل في تحميل الحقول: {error}',
      failedToSaveFields: 'فشل في حفظ تعيينات الحقول: {error}',
      errorAddingTestCase: 'خطأ في إضافة حالة الاختبار: {error}',
      failedToCreateLabel: 'فشل في إنشاء التسمية: {error}',
      failedToSaveLayout: 'فشل في حفظ تغييرات التخطيط',
    },
  },

  components: {
    // Components section - falls back to English for missing keys
  },

  aria: {
    close: 'إغلاق',
    dragToReorder: 'اسحب لإعادة الترتيب',
    refresh: 'تحديث',
    removeField: 'إزالة الحقل',
    removeFromSection: 'إزالة من القسم',
    addNewStep: 'إضافة خطوة جديدة',
    removeCurrentStep: 'إزالة الخطوة الحالية',
    dismissNotification: 'إغلاق الإشعار',
    mainNavigation: 'التنقل الرئيسي',
    mentionUsers: 'ذكر المستخدمين',
    notifications: 'الإشعارات',
    adminSettings: 'إعدادات المسؤول',
    userMenu: 'قائمة المستخدم',
  },

  layout: {
    addSection: 'إضافة قسم',
    moveUp: 'نقل القسم للأعلى',
    moveDown: 'نقل القسم للأسفل',
    deleteSection: 'حذف القسم',
    editMode: 'وضع التحرير',
    editDisplaySettings: 'تعديل إعدادات العرض',
  },

  widgets: {
    removeWidget: 'إزالة الأداة',
    narrowWidth: 'ضيق (1/3 العرض)',
    mediumWidth: 'متوسط (2/3 العرض)',
    fullWidth: 'عرض كامل',
    chart: {
      items: 'عناصر',
    },
    completionChart: {
      emptyMessage: 'لا تتوفر بيانات الإنجاز',
    },
    createdChart: {
      emptyMessage: 'لا تتوفر بيانات الإنشاء',
    },
    milestoneProgress: {
      emptyTitle: 'لا توجد معالم',
      emptySubtitle: 'أنشئ معالم لتتبع التقدم',
      due: 'الاستحقاق',
      done: 'مكتمل',
      item: 'عنصر',
      items: 'عناصر',
      noItems: 'لا توجد عناصر',
      noStatus: 'بدون حالة',
      activeMilestone: 'نشط',
      noCategorizedWork: 'لا يوجد عمل مصنف',
    },
  },

  footer: {
    // Footer section - falls back to English for missing keys
  },
};
