/**
 * Analytics translations for Arabic locale locale
 */
export default {
  analytics: {
    title: 'التحليلات',
    loading: 'جارٍ تحميل التحليلات...',
    noData: 'لا توجد بيانات متاحة',
    dateRange: 'نطاق التاريخ',
    collection: 'المجموعة',
    allItems: 'جميع عناصر مساحة العمل',
    datasetBasis: '{items} عنصر عبر {iterations, plural, one {# تكرار} other {# تكرارات}}',
    datasetBasisNoIterations: '{items} عنصر، لا توجد تكرارات في النطاق',
    velocity: {
      title: 'اتجاه السرعة',
      completed: 'مكتمل',
      average: 'المتوسط (التكرارات المكتملة)',
    },
    cumulativeFlow: {
      title: 'التدفق التراكمي',
    },
    cycleTime: {
      title: 'وقت الدورة',
      itemsAnalyzed: 'العناصر المُحلَّلة',
      avgTotal: 'متوسط الإجمالي',
      median: 'الوسيط',
      p85: 'النسبة المئوية 85',
    },
    forecast: {
      title: 'توقع الإنجاز',
      remainingItems: 'العناصر المتبقية',
      remainingPoints: 'النقاط المتبقية',
      throughput: 'الإنتاجية (لكل تكرار)',
      avg: 'المتوسط',
      min: 'الأدنى',
      max: 'الأقصى',
      predictions: 'التوقعات',
      confidence: 'الثقة',
      iterations: 'التكرارات',
      estimatedDate: 'التاريخ المتوقع',
      method: 'الطريقة',
      lowDataWarning: 'التوقع مبني على بيانات تاريخية محدودة. قد تكون التنبؤات غير دقيقة.',
    },
    storyPoints: 'نقاط القصة',
    insufficientData: {
      no_items: 'لا تحتوي مساحة العمل هذه على عناصر بعد. أنشئ عناصر لرؤية التحليلات.',
      no_iterations: 'لم يتم العثور على تكرارات مكتملة في نطاق التاريخ المحدد. أنشئ وأكمل تكرارات لرؤية الاتجاهات.',
      no_iteration_items: 'توجد تكرارات في النطاق لكن ليس لها عناصر مطابقة. قم بتعيين عناصر للتكرارات لرؤية السرعة.',
      no_workflow: 'لم يتم تكوين سير عمل. قم بإعداد الحالات لمساحة العمل هذه لرؤية بيانات التدفق.',
      no_history: 'توجد عناصر لكنها لم تنتقل عبر الحالات بعد. انقل العناصر بين الحالات لبناء مخطط التدفق.',
      no_completed_items: 'لم يتم إكمال أي عناصر في نطاق التاريخ المحدد. أكمل عناصر لتحليل أوقات الدورة.',
      few_completed_items: 'تم إكمال عدد قليل من العناصر فقط. تصبح إحصائيات وقت الدورة أكثر موثوقية مع المزيد من البيانات.',
      few_iterations: 'أقل من 3 تكرارات مكتملة في النطاق. أكمل المزيد من التكرارات لتمكين توقعات مونت كارلو.',
    },
  },
};
