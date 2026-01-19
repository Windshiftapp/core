/**
 * Actions automation translations (Arabic)
 */
export default {
  actions: {
    title: 'الإجراءات',
    description: 'أتمتة سير العمل بإجراءات قائمة على القواعد',
    create: 'إنشاء إجراء',
    createFirst: 'أنشئ إجراءك الأول',
    noActions: 'لا توجد إجراءات بعد',
    noActionsDescription: 'أنشئ إجراءات لأتمتة سير العمل بناءً على أحداث العناصر',
    enabled: 'مفعّل',
    disabled: 'معطّل',
    enable: 'تفعيل',
    disable: 'تعطيل',
    viewLogs: 'عرض السجلات',
    confirmDelete: 'هل أنت متأكد أنك تريد حذف الإجراء "{name}"؟',
    failedToSave: 'فشل في حفظ الإجراء',
    newAction: 'إجراء جديد',

    trigger: {
      statusTransition: 'تغيير الحالة',
      itemCreated: 'تم إنشاء عنصر',
      itemUpdated: 'تم تحديث عنصر',
      itemLinked: 'تم ربط عنصر',
      respondToCascades: 'الاستجابة للتغييرات التي تطلقها الإجراءات',
      respondToCascadesHint: 'عند التفعيل، سيتم تشغيل هذا الإجراء أيضاً عند تفعيله بواسطة إجراءات أخرى، وليس فقط تغييرات المستخدم.'
    },

    nodes: {
      trigger: 'المشغّل',
      setField: 'تعيين حقل',
      setStatus: 'تعيين الحالة',
      addComment: 'إضافة تعليق',
      notifyUser: 'إشعار المستخدم',
      condition: 'شرط'
    },

    addNodes: 'إضافة عقد',
    tips: 'نصائح',
    tipDragToConnect: 'اسحب من المقابض لربط العقد',
    tipClickToEdit: 'انقر على عقدة لتكوينها',
    tipConditionBranches: 'الشروط لها فروع صحيح/خطأ',

    nodeConfig: 'تكوين العقدة',
    config: {
      from: 'من',
      to: 'إلى',
      selectField: 'اختر حقلاً...',
      selectStatus: 'اختر الحالة...',
      enterComment: 'أدخل تعليقاً...',
      selectRecipient: 'اختر المستلم...',
      setCondition: 'حدد الشرط...',
      targetStatus: 'الحالة المستهدفة',
      fieldName: 'اسم الحقل',
      value: 'القيمة',
      commentContent: 'محتوى التعليق',
      commentPlaceholder: 'أدخل نص التعليق. استخدم {{item.title}} للمتغيرات.',
      privateComment: 'تعليق خاص (داخلي فقط)',
      fieldToCheck: 'الحقل للتحقق',
      operator: 'المشغّل',
      compareValue: 'قيمة المقارنة',
      private: 'خاص',
      triggerType: 'نوع المشغّل',
      fromStatus: 'من الحالة',
      toStatus: 'إلى الحالة',
      anyStatus: 'أي حالة'
    },

    recipients: {
      assignee: 'المُعيَّن',
      creator: 'المُنشئ',
      specific: 'مستخدمون محددون'
    },

    condition: {
      true: 'نعم',
      false: 'لا'
    },

    operators: {
      equals: 'يساوي',
      notEquals: 'لا يساوي',
      contains: 'يحتوي على',
      greaterThan: 'أكبر من',
      lessThan: 'أصغر من',
      isEmpty: 'فارغ',
      isNotEmpty: 'غير فارغ'
    },

    logs: {
      title: 'سجلات التنفيذ',
      noLogs: 'لا توجد سجلات تنفيذ',
      status: 'الحالة',
      running: 'قيد التشغيل',
      completed: 'مكتمل',
      failed: 'فشل',
      skipped: 'تم تخطيه',
      startedAt: 'بدأ في',
      completedAt: 'اكتمل في',
      error: 'خطأ',
      viewDetails: 'عرض التفاصيل'
    }
  }
};
