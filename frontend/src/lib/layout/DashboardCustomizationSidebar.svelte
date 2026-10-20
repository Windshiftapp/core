<script>
  import {
    Sparkles,
    CheckSquare,
    Compass,
    LifeBuoy,
    Bell,
    Clock,
    Eye,
    Target,
    Briefcase,
    Grip,
    ListChecks,
    PenLine,
    Search,
    Palette,
  } from '@lucide/svelte';
  import {
    dashboardWidgetCategories,
    getDashboardWidgetsByCategory,
  } from '../services/dashboardWidgetRegistry.js';
  import { t } from '../stores/i18n.svelte.js';
  import WidgetCustomizationSidebar from './WidgetCustomizationSidebar.svelte';
  import DashboardBackgroundPanel from './DashboardBackgroundPanel.svelte';

  let { isOpen = $bindable(false), activeCategory = $bindable('activity') } = $props();

  const iconMap = {
    Sparkles,
    CheckSquare,
    Compass,
    LifeBuoy,
    Bell,
    Clock,
    Eye,
    Target,
    Briefcase,
    Grip,
    ListChecks,
    PenLine,
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
      id: dashboardWidgetCategories.SUPPORT,
      name: t('dashboard.customization.support.name'),
      description: t('dashboard.customization.support.description'),
      icon: LifeBuoy,
    },
    {
      id: dashboardWidgetCategories.NAVIGATION,
      name: t('dashboard.customization.navigation.name'),
      description: t('dashboard.customization.navigation.description'),
      icon: Compass,
    },
    {
      id: 'background',
      name: t('dashboard.customization.background.name'),
      description: t('dashboard.customization.background.description'),
      icon: Palette,
    },
  ]);

  const widgets = $derived(
    getDashboardWidgetsByCategory(activeCategory).map((widget) => ({
      ...widget,
      name: t(widget.nameKey),
      description: t(widget.descriptionKey),
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
  panelCategory="background"
>
  {#snippet panelContent()}
    <DashboardBackgroundPanel />
  {/snippet}
</WidgetCustomizationSidebar>
