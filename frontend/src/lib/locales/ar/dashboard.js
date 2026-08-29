export default {
  dashboard: {
    salutation: {
      withName: '{salutation}، {name}!',
      withoutName: '{salutation}!',
    },
    sections: {
      yourDay: {
        title: 'يومك',
        subtitle: 'نظرة سريعة على ما يحتاج إلى انتباهك',
      },
      work: {
        title: 'العمل',
        subtitle: 'قائمتك الشخصية والعناصر المسندة إليك',
      },
      workspaces: {
        title: 'مساحات العمل',
        subtitle: 'تابع من حيث توقفت',
      },
    },
    widgetCatalog: {
      dailyBriefing: {
        name: 'الموجز اليومي',
        description: 'ملخص مولد بالذكاء الاصطناعي لما يهمك اليوم',
      },
      yourActivity: {
        name: 'نشاطك',
        description: 'العناصر التي عرضتها أو عدلتها أو علقت عليها مؤخراً',
      },
      whatsNew: {
        name: 'ما الجديد',
        description: 'أحدث الإشعارات والتحديثات غير المقروءة',
      },
      personalTasks: {
        name: 'المهام الشخصية',
        description: 'عناصر من قائمة مهامك الشخصية',
      },
      savedSearch: {
        name: 'بحث محفوظ',
        description: 'عرض عناصر العمل من مجموعة محفوظة',
      },
      assignedToMe: {
        name: 'مسند إليّ',
        description: 'العناصر المفتوحة المسندة إليك في جميع مساحات العمل',
      },
      watchedItems: {
        name: 'العناصر المراقبة',
        description: 'العناصر التي تتابعها',
      },
      upcomingMilestones: {
        name: 'المعالم القادمة',
        description: 'المعالم ذات التواريخ المستهدفة القريبة',
      },
      recentWorkspaces: {
        name: 'مساحات العمل الأخيرة',
        description: 'مساحات العمل التي زرتها مؤخراً',
      },
      quickAccess: {
        name: 'الوصول السريع',
        description: 'روابط سريعة إلى مساحات العمل المتاحة لك',
      },
    },
    customization: {
      widgets: 'الأدوات',
      activity: {
        name: 'النشاط',
        description: 'الموجزات وتدفقات النشاط والإشعارات',
      },
      work: {
        name: 'العمل',
        description: 'العناصر والمعالم والمهام المسندة إليك',
      },
      navigation: {
        name: 'التنقل',
        description: 'وصول سريع إلى مساحات العمل',
      },
      tipLabel: 'تلميح',
      tip: 'اسحب الأدوات من هنا إلى أي قسم في لوحة المعلومات.',
    },
    editor: {
      newSection: 'قسم جديد',
      deleteSectionConfirm: 'هل تريد حذف هذا القسم؟ ستتم إزالة جميع الأدوات الموجودة فيه.',
      doneEditing: 'إنهاء التحرير',
      customize: 'تخصيص',
      editModeDescription: 'وضع التحرير: أضف الأقسام والأدوات أو أعد تسميتها أو ترتيبها أو احذفها',
      addSection: 'إضافة قسم',
      sectionLabel: 'قسم لوحة المعلومات',
      sectionTitlePlaceholder: 'عنوان القسم',
      sectionSubtitlePlaceholder: 'العنوان الفرعي (اختياري)',
      renameSection: 'إعادة تسمية القسم',
      deleteSection: 'حذف القسم',
      unknownWidgetType: 'نوع أداة غير معروف: {type}',
      noWidgets: 'لا توجد أدوات في هذا القسم بعد',
      addWidgetsHint: 'اختر تخصيص لإضافة أدوات',
      noSections: 'لم يتم إعداد أي أقسام',
      addSectionsHint: 'اختر تحرير لإضافة أقسام إلى لوحة المعلومات',
    },
    states: {
      assignedLoadError: 'تعذر تحميل العناصر المسندة إليك',
      assignedEmpty: 'لا يوجد شيء مسند إليك حالياً',
      personalTasksLoadError: 'تعذر تحميل مهامك الشخصية',
      personalTasksEmpty: 'قائمة مهامك الشخصية فارغة',
      dailyBriefingUnavailable: 'موجزك اليومي غير متاح حالياً. يعتمد على تكامل للذكاء الاصطناعي. إذا أعددت تكاملاً للتو، فحاول مرة أخرى بعد قليل.',
      updatedAt: 'تم التحديث في {time}',
      priorityLabel: 'الأولوية: {priority}',
      noWorkspaces: 'لا توجد مساحات عمل بعد',
      createWorkspace: 'إنشاء مساحة',
      workspaceAvatarAlt: 'الصورة الرمزية لـ {name}',
      visited: 'تمت الزيارة {time}',
      noUpcomingMilestones: 'لا توجد معالم قادمة',
      milestoneProgress: 'تم إنجاز {done} من {total}',
      daysOverdue: 'متأخر {days} أيام',
      daysLeft: 'متبقي {days} أيام',
      watchedItemsEmpty: 'أنت لا تراقب أي عناصر',
      workspaceWithId: 'مساحة العمل {id}',
    },
  },
};
