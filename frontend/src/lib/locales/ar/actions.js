/**
 * Actions automation translations (Arabic - RTL language)
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

    // Trigger types
    trigger: {
      statusTransition: 'تغيير الحالة',
      itemCreated: 'تم إنشاء عنصر',
      itemUpdated: 'تم تحديث عنصر',
      itemLinked: 'تم ربط عنصر',
      manual: 'يدوي',
      respondToCascades: 'الاستجابة للتغييرات التي تطلقها الإجراءات',
      respondToCascadesHint:
        'عند التفعيل، سيتم تشغيل هذا الإجراء أيضاً عند تفعيله بواسطة إجراءات أخرى، وليس فقط تغييرات المستخدم.',
    },

    // Node types
    nodes: {
      trigger: 'المشغّل',
      setField: 'تعيين حقل',
      setStatus: 'تعيين الحالة',
      addComment: 'إضافة تعليق',
      notifyUser: 'إشعار المستخدم',
      condition: 'شرط',
      updateAsset: 'تحديث أصل',
      createAsset: 'إنشاء أصل',
    },

    // Node palette and tips
    addNodes: 'إضافة عقد',
    tips: 'نصائح',
    tipDragToConnect: 'اسحب من المقابض لربط العقد',
    tipClickToEdit: 'انقر على عقدة لتكوينها',
    tipConditionBranches: 'الشروط لها فروع صحيح/خطأ',

    // Config panel
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
      recipientType: 'المستلم',
      notifyMessage: 'الرسالة',
      notifyPlaceholder: 'أدخل الرسالة. استخدم {{item.title}} للمتغيرات.',
      includeLink: 'تضمين رابط للعنصر',
      // Update Asset config
      sourceAssetField: 'حقل الأصل في العنصر',
      selectAssetField: 'اختر حقل الأصل...',
      sourceAssetFieldHint: 'اختر حقل العنصر الذي يحتوي على الأصل المرتبط',
      targetAssetType: 'نوع الأصل المستهدف',
      selectAssetType: 'اختر نوع الأصل...',
      fieldMappingsLabel: 'تعيينات الحقول',
      fieldMappings: '{count} تعيين(ات) حقول',
      configureAssetUpdate: 'تكوين تحديث الأصل...',
      fromField: 'من الحقل',
      sourceTypeVariable: 'متغير/قالب',
      sourceTypeItemField: 'حقل العنصر',
      sourceTypeLiteral: 'قيمة ثابتة',
      selectTargetField: 'اختر الحقل المستهدف...',
      addMapping: 'إضافة تعيين',
      // Create Asset config
      assetSet: 'مجموعة الأصول',
      selectAssetSet: 'اختر مجموعة الأصول...',
      assetTitle: 'عنوان الأصل',
      assetTitleHint: 'استخدم {{item.title}} أو متغيرات أخرى',
      assetDescription: 'الوصف',
      assetTagLabel: 'وسم الأصل',
      assetCategory: 'الفئة',
      selectCategory: 'اختر الفئة (اختياري)...',
      assetStatus: 'الحالة',
      selectStatusOptional: 'اختر الحالة (اختياري)...',
      requiredField: 'مطلوب',
      configureAssetCreation: 'تكوين إنشاء الأصل...',
    },

    // Recipients
    recipients: {
      assignee: 'المُعيَّن',
      creator: 'المُنشئ',
      specific: 'مستخدمون محددون',
    },

    // Condition
    condition: {
      true: 'نعم',
      false: 'لا',
    },

    // Operators
    operators: {
      equals: 'يساوي',
      notEquals: 'لا يساوي',
      contains: 'يحتوي على',
      greaterThan: 'أكبر من',
      lessThan: 'أصغر من',
      isEmpty: 'فارغ',
      isNotEmpty: 'غير فارغ',
    },

    // Execution logs
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

    // Execution trace
    trace: {
      title: 'تفاصيل التنفيذ',
      noSteps: 'لم يتم تسجيل خطوات التنفيذ',
      setStatus: 'تم تغيير الحالة من "{from}" إلى "{to}"',
      setField: 'تم تعيين {field} من "{from}" إلى "{to}"',
      addComment: 'تمت إضافة تعليق {prefix}: "{content}"',
      notifyUser: 'تم إرسال إشعار إلى {count} مستخدم(ين)',
      notifySkipped: 'تم تخطي الإشعار: {reason}',
      conditionResult: 'نتيجة الشرط: {result}',
      updateAsset: 'تم تحديث الأصل #{asset_id}',
      updateAssetSkipped: 'تم تخطي تحديث الأصل: {reason}',
      createAsset: 'تم إنشاء الأصل #{asset_id}: {title}',
      createAssetFailed: 'فشل إنشاء الأصل: {reason}',
    },

    // Test/manual execution
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

    // Placeholder reference
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
    switchToVertical: 'التبديل إلى التخطيط العمودي',
    switchToHorizontal: 'التبديل إلى التخطيط الأفقي',
  },
};
