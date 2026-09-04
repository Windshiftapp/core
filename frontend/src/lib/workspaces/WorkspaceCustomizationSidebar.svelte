<script>
  import { BarChart3, Package } from '@lucide/svelte';
  import { widgetCategories, getWidgetsByCategory } from '../services/widgetRegistry.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import { t } from '../stores/i18n.svelte.js';
  import WidgetCustomizationSidebar from '../layout/WidgetCustomizationSidebar.svelte';

  let { isOpen = $bindable(false), activeCategory = $bindable('built-in') } = $props();

  const categories = $derived([
    {
      id: widgetCategories.BUILT_IN,
      name: t('workspaceDashboard.customization.builtIn'),
      description: t('workspaceDashboard.customization.builtInDescription'),
      icon: BarChart3,
    },
    {
      id: widgetCategories.ADDITIONAL,
      name: t('workspaceDashboard.customization.additional'),
      description: t('workspaceDashboard.customization.additionalDescription'),
      icon: Package,
    },
  ]);

  const widgets = $derived(
    getWidgetsByCategory(activeCategory).map((widget) => ({
      ...widget,
      name: t(widget.nameKey),
      description: t(widget.descriptionKey),
      categoryLabel: categories.find((category) => category.id === widget.category)?.name || widget.category,
      widthLabel: t('workspaceDashboard.customization.defaultWidth', { width: widget.defaultWidth }),
    })),
  );
</script>

<WidgetCustomizationSidebar
  bind:isOpen
  bind:activeCategory
  {categories}
  widgets={widgets}
  iconMap={workspaceIconMap}
  fallbackIcon={BarChart3}
  cardAttributes={{ 'data-widget-card': '' }}
  fallbackTitle={t('workspaceDashboard.customization.widgets')}
  tipLabel={t('workspaceDashboard.customization.tipTitle')}
  tip={t('workspaceDashboard.customization.tip')}
/>
