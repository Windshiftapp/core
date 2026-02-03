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
      respondToCascadesHint:
        'عند التفعيل، سيتم تشغيل هذا الإجراء أيضاً عند تفعيله بواسطة إجراءات أخرى، وليس فقط تغييرات المستخدم.',
    },

    nodes: {
      trigger: 'المشغّل',
      setField: 'تعيين حقل',
      setStatus: 'تعيين الحالة',
      addComment: 'إضافة تعليق',
      notifyUser: 'إشعار المستخدم',
      condition: 'شرط',
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
      anyStatus: 'أي حالة',
    },

    recipients: {
      assignee: 'المُعيَّن',
      creator: 'المُنشئ',
      specific: 'مستخدمون محددون',
    },

    condition: {
      true: 'نعم',
      false: 'لا',
    },

    operators: {
      equals: 'يساوي',
      notEquals: 'لا يساوي',
      contains: 'يحتوي على',
      greaterThan: 'أكبر من',
      lessThan: 'أصغر من',
      isEmpty: 'فارغ',
      isNotEmpty: 'غير فارغ',
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
      details: 'التفاصيل',
      viewDetails: 'عرض التفاصيل',
    },

    trace: {
      title: 'تفاصيل التنفيذ',
      noSteps: 'لم يتم تسجيل خطوات التنفيذ',
      setStatus: 'تم تغيير الحالة من "{from}" إلى "{to}"',
      setField: 'تم تعيين {field} من "{from}" إلى "{to}"',
      addComment: 'تمت إضافة تعليق {prefix}: "{content}"',
      notifyUser: 'تم إرسال إشعار إلى {count} مستخدم(ين)',
      notifySkipped: 'تم تخطي الإشعار: {reason}',
      conditionResult: 'نتيجة الشرط: {result}',
    },

    test: {
      title: 'اختبار الإجراء',
      description:
        'اختر عنصرًا لتشغيل هذا الإجراء عليه. سيتم تنفيذ الإجراء فورًا، متجاوزًا المشغّل العادي.',
      selectItem: 'اختر عنصرًا',
      itemPlaceholder: 'ابحث عن عنصر...',
      execute: 'تشغيل الإجراء',
      run: 'تشغيل تجريبي',
      executionFailed: 'فشل في تنفيذ الإجراء',
      executionQueued: 'تم وضع الإجراء في قائمة الانتظار للتنفيذ',
    },

    placeholders: {
      title: 'العناصر النائبة المتاحة',
      description:
        'استخدم هذه العناصر النائبة في القالب الخاص بك. سيتم استبدالها بقيم فعلية عند تشغيل الإجراء.',
      showReference: 'عرض مرجع العناصر النائبة',
      categories: {
        item: 'حقول العنصر',
        user: 'المستخدم الحالي',
        old: 'القيم السابقة',
        trigger: 'سياق المشغّل',
      },
      item: {
        title: 'عنوان العنصر',
        id: 'معرّف العنصر',
        statusId: 'معرّف الحالة',
        assigneeId: 'معرّف المستخدم المُعيَّن',
        any: 'أي حقل من حقول العنصر',
      },
      user: {
        name: 'الاسم الكامل للمستخدم',
        email: 'البريد الإلكتروني للمستخدم',
        id: 'معرّف المستخدم',
      },
      old: {
        description: 'القيمة السابقة قبل التغيير',
        example: 'القيمة السابقة لأي حقل',
      },
      trigger: {
        itemId: 'معرّف العنصر المُشغِّل',
        workspaceId: 'معرّف مساحة العمل',
      },
    },
  },
};
