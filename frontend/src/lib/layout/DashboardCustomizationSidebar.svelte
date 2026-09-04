<script>
  import {
    Sparkles,
    CheckSquare,
    Compass,
    Bell,
    Clock,
    Eye,
    Target,
    Briefcase,
    Grip,
    ListChecks,
    Search,
  } from '@lucide/svelte';
  import {
    DASHBOARD_GRID_COLUMNS,
    dashboardWidgetCategories,
    getDashboardWidgetsByCategory,
  } from '../services/dashboardWidgetRegistry.js';
  import { t } from '../stores/i18n.svelte.js';
  import WidgetCustomizationSidebar from './WidgetCustomizationSidebar.svelte';

  let { isOpen = $bindable(false), activeCategory = $bindable('activity') } = $props();

  const iconMap = {
    Sparkles,
    CheckSquare,
    Compass,
    Bell,
    Clock,
    Eye,
    Target,
    Briefcase,
    Grip,
    ListChecks,
    Search,
  };

  const categories = $derived([
    {
      id: dashboardWidgetCategories.ACTIVITY,
      name: t('dashboard.customization.activity.name'),
      description: t('dashboard.customization.activity.description'),
      icon: Clock,
    },
    {
      id: dashboardWidgetCategories.WORK,
      name: t('dashboard.customization.work.name'),
      description: t('dashboard.customization.work.description'),
      icon: CheckSquare,
    },
    {
      id: dashboardWidgetCategories.NAVIGATION,
      name: t('dashboard.customization.navigation.name'),
      description: t('dashboard.customization.navigation.description'),
      icon: Compass,
    },
  ]);

  const widgets = $derived(
    getDashboardWidgetsByCategory(activeCategory).map((widget) => ({
      ...widget,
      name: t(widget.nameKey),
      description: t(widget.descriptionKey),
      categoryLabel: t(`dashboard.customization.${widget.category}.name`),
      widthLabel: t('widgets.defaultWidth', {
        width: widget.defaultWidth,
        columns: DASHBOARD_GRID_COLUMNS,
      }),
    })),
  );
</script>

<WidgetCustomizationSidebar
  bind:isOpen
  bind:activeCategory
  {categories}
  {widgets}
  {iconMap}
  fallbackIcon={Sparkles}
  cardAttributes={{ 'data-dashboard-widget-card': '' }}
  fallbackTitle={t('dashboard.customization.widgets')}
  tipLabel={t('dashboard.customization.tipLabel')}
  tip={t('dashboard.customization.tip')}
/>
