<script>
  import { onMount } from 'svelte';
  import {
    AlertCircle,
    CircleCheck,
    CirclePlus,
    Clock3,
    Flag,
    Gauge,
    Hash,
    Info,
    Layers,
    Ruler,
    TrendingUp,
    UserRoundMinus,
  } from '@lucide/svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { currentWorkspace } from '../../stores/workspaces.svelte.js';
  import AlertBox from '../../components/AlertBox.svelte';
  import Badge from '../../components/Badge.svelte';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import Select from '../../components/Select.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import Text from '../../components/Text.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';
  import Chart from '../../widgets/Chart.svelte';
  import StatCard from '../../widgets/StatCard.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import {
    defaultAnalyticsRange,
    formatDateOnly,
    formatDayNumber,
    inclusiveDateRangeDays,
    localDateString,
    shiftDateString,
    validateAnalyticsRange,
  } from './analyticsView.js';

  let { workspaceId = null } = $props();

  const initialRange = defaultAnalyticsRange();
  let initialized = $state(false);
  let loading = $state(true);
  let analyticsData = $state(null);
  let loadError = $state('');
  let validationCode = $state(null);
  let collections = $state([]);
  let collectionLoadError = $state(false);
  let selectedCollection = $state('');
  let selectedPreset = $state('84');
  let startDate = $state(initialRange.startDate);
  let endDate = $state(initialRange.endDate);

  let analyticsLoadVersion = 0;
  let collectionsLoadVersion = 0;
  let lastAnalyticsLoadKey = null;
  let lastCollectionsWorkspace = null;

  const collectionOptions = $derived([
    { value: '', label: t('analytics.allItems') },
    ...collections.map((collection) => ({
      value: String(collection.id),
      label: collection.name,
    })),
  ]);

  const rangeOptions = $derived([
    { value: '30', label: t('analytics.range.last30Days') },
    { value: '84', label: t('analytics.range.last12Weeks') },
    { value: '180', label: t('analytics.range.last6Months') },
    { value: '365', label: t('analytics.range.lastYear') },
    { value: 'custom', label: t('analytics.range.custom') },
  ]);

  const analyticsLoadKey = $derived(
    initialized && workspaceId
      ? `${workspaceId}|${selectedCollection}|${startDate}|${endDate}`
      : null,
  );

  $effect(() => {
    const currentWorkspaceId = initialized && workspaceId ? String(workspaceId) : null;
    if (!currentWorkspaceId || currentWorkspaceId === lastCollectionsWorkspace) return;

    if (lastCollectionsWorkspace !== null) {
      selectedCollection = '';
    }
    lastCollectionsWorkspace = currentWorkspaceId;
    analyticsData = null;
    loadCollections(currentWorkspaceId);
  });

  $effect(() => {
    const key = analyticsLoadKey;
    if (!key || key === lastAnalyticsLoadKey) return;
    lastAnalyticsLoadKey = key;
    loadAnalytics();
  });

  onMount(() => {
    if (typeof window !== 'undefined') {
      const query = new URLSearchParams(window.location.search);
      const queryStart = query.get('start_date');
      const queryEnd = query.get('end_date');
      const queryCollection = query.get('collection_id');
      if (queryStart) startDate = queryStart;
      if (queryEnd) endDate = queryEnd;
      if (queryCollection && /^\d+$/.test(queryCollection)) {
        selectedCollection = queryCollection;
      }
      selectedPreset = detectPreset(startDate, endDate);
    }
    initialized = true;
  });

  async function loadCollections(targetWorkspace) {
    const version = ++collectionsLoadVersion;
    collectionLoadError = false;
    try {
      const response = await api.collections.getAll({ workspace_id: targetWorkspace });
      if (version !== collectionsLoadVersion) return;
      collections = response || [];
      if (
        selectedCollection &&
        !collections.some((collection) => String(collection.id) === String(selectedCollection))
      ) {
        selectedCollection = '';
      }
    } catch (error) {
      if (version !== collectionsLoadVersion) return;
      console.error('Failed to load analytics collections:', error);
      collections = [];
      selectedCollection = '';
      collectionLoadError = true;
    }
  }

  async function loadAnalytics() {
    const version = ++analyticsLoadVersion;
    const rangeError = validateAnalyticsRange(startDate, endDate);
    validationCode = rangeError;
    loadError = '';
    if (rangeError || !workspaceId) {
      loading = false;
      return;
    }

    loading = true;
    analyticsData = null;
    syncQueryString();
    try {
      const params = { start_date: startDate, end_date: endDate };
      if (selectedCollection) params.collection_id = selectedCollection;
      const response = await api.analytics.getAnalytics(workspaceId, params);
      if (version !== analyticsLoadVersion) return;
      if (response?.schema_version !== 2) {
        throw new Error(t('analytics.unsupportedVersion'));
      }
      analyticsData = response;
    } catch (error) {
      if (version !== analyticsLoadVersion) return;
      console.error('Failed to load analytics:', error);
      loadError = error?.message || t('analytics.errorTitle');
    } finally {
      if (version === analyticsLoadVersion) loading = false;
    }
  }

  function syncQueryString() {
    if (typeof window === 'undefined') return;
    const url = new URL(window.location.href);
    url.searchParams.set('start_date', startDate);
    url.searchParams.set('end_date', endDate);
    if (selectedCollection) {
      url.searchParams.set('collection_id', selectedCollection);
    } else {
      url.searchParams.delete('collection_id');
    }
    window.history.replaceState(window.history.state, '', url);
  }

  function applyPreset(value) {
    selectedPreset = value;
    if (value === 'custom') return;
    const days = Number(value);
    endDate = localDateString();
    startDate = shiftDateString(endDate, -(days - 1));
  }

  function detectPreset(from, to) {
    const days = inclusiveDateRangeDays(from, to);
    return [30, 84, 180, 365].includes(days) ? String(days) : 'custom';
  }

  function handleDateEdit() {
    selectedPreset = detectPreset(startDate, endDate);
  }

  function retry() {
    lastAnalyticsLoadKey = null;
    loadAnalytics();
  }

  function openItem(item) {
    navigate(`/workspaces/${workspaceId}/items/${item.id}`);
  }

  function days(value) {
    return t('analytics.daysValue', { value: formatDayNumber(value) });
  }

  function period(bucket) {
    return `${formatDateOnly(bucket.start_date)} – ${formatDateOnly(bucket.end_date, {
      year: 'numeric',
    })}`;
  }

  function signed(value) {
    return value > 0 ? `+${value}` : String(value);
  }

  function flagVariant(flag) {
    if (flag === 'overdue') return 'danger';
    if (flag === 'stale') return 'warning';
    return 'neutral';
  }

  const dataset = $derived(analyticsData?.dataset || null);
  const health = $derived(analyticsData?.health || null);
  const throughput = $derived(analyticsData?.throughput || null);
  const aging = $derived(analyticsData?.aging_wip || null);
  const deliveryTime = $derived(analyticsData?.delivery_time || null);

  // Shared StatCard palettes keep metric icons semantic without adding another
  // framed surface inside the analytics cards.
  const NEUTRAL_TILE = {
    iconColor: 'var(--ds-icon-subtle)',
  };
  const DANGER_TILE = {
    iconColor: 'var(--ds-icon-danger)',
  };
  const WARNING_TILE = {
    iconColor: 'var(--ds-icon-warning)',
  };
  const BLUE_TILE = {
    iconColor: 'var(--ds-icon-accent-blue)',
  };
  const GREEN_TILE = {
    iconColor: 'var(--ds-icon-accent-green)',
  };
  const ORANGE_TILE = {
    iconColor: 'var(--ds-icon-accent-orange)',
  };
  const TEAL_TILE = {
    iconColor: 'var(--ds-icon-accent-teal)',
  };
  const PURPLE_TILE = {
    iconColor: 'var(--ds-icon-accent-purple)',
  };

  const healthMetrics = $derived.by(() => [
    {
      key: 'unfinished',
      icon: Clock3,
      ...NEUTRAL_TILE,
      label: t('analytics.health.unfinished'),
      value: health?.unfinished_items || 0,
    },
    {
      key: 'overdue',
      icon: AlertCircle,
      ...DANGER_TILE,
      label: t('analytics.health.overdue'),
      value: health?.overdue || 0,
    },
    {
      key: 'stale',
      icon: Gauge,
      ...WARNING_TILE,
      label: t('analytics.health.stale'),
      value: health?.stale || 0,
    },
    {
      key: 'unassigned',
      icon: UserRoundMinus,
      ...NEUTRAL_TILE,
      label: t('analytics.health.unassigned'),
      value: health?.unassigned || 0,
    },
    {
      key: 'without-priority',
      icon: Flag,
      ...NEUTRAL_TILE,
      label: t('analytics.health.withoutPriority'),
      value: health?.without_priority || 0,
    },
    {
      key: 'without-estimate',
      icon: Ruler,
      ...NEUTRAL_TILE,
      label: t('analytics.health.withoutEstimate'),
      value: health?.without_estimate || 0,
    },
  ]);

  const throughputMetrics = $derived.by(() => [
    {
      key: 'created',
      icon: CirclePlus,
      ...BLUE_TILE,
      label: t('analytics.throughput.created'),
      value: throughput?.total_created || 0,
    },
    {
      key: 'completed',
      icon: CircleCheck,
      ...GREEN_TILE,
      label: t('analytics.throughput.completed'),
      value: throughput?.total_completed || 0,
    },
    {
      key: 'average',
      icon: TrendingUp,
      ...PURPLE_TILE,
      label: t('analytics.throughput.average'),
      value: formatDayNumber(throughput?.average_completed || 0),
    },
  ]);

  const agingMetrics = $derived.by(() => [
    {
      key: 'total',
      icon: Layers,
      ...ORANGE_TILE,
      label: t('analytics.aging.total'),
      value: aging?.total_items || 0,
    },
    {
      key: 'median',
      icon: Gauge,
      ...BLUE_TILE,
      label: t('analytics.aging.median'),
      value: days(aging?.median_days || 0),
    },
    {
      key: 'p85',
      icon: Ruler,
      ...TEAL_TILE,
      label: t('analytics.aging.p85'),
      value: days(aging?.p85_days || 0),
    },
  ]);

  const deliveryMetrics = $derived.by(() => [
    {
      key: 'analyzed',
      icon: Hash,
      ...NEUTRAL_TILE,
      label: t('analytics.deliveryTime.analyzed'),
      value: deliveryTime?.total_items_analyzed || 0,
    },
    {
      key: 'average',
      icon: TrendingUp,
      ...PURPLE_TILE,
      label: t('analytics.deliveryTime.average'),
      value: days(deliveryTime?.average_days || 0),
    },
    {
      key: 'median',
      icon: Gauge,
      ...BLUE_TILE,
      label: t('analytics.deliveryTime.median'),
      value: days(deliveryTime?.median_days || 0),
    },
    {
      key: 'p85',
      icon: Ruler,
      ...TEAL_TILE,
      label: t('analytics.deliveryTime.p85'),
      value: days(deliveryTime?.p85_days || 0),
    },
  ]);

  const throughputBuckets = $derived(throughput?.buckets || []);
  const throughputCategories = $derived(
    throughputBuckets.map((bucket) => formatDateOnly(bucket.start_date)),
  );
  const throughputSeries = $derived([
    {
      key: 'created',
      label: t('analytics.throughput.created'),
      color: 'var(--ds-icon-accent-blue)',
      values: throughputBuckets.map((bucket) => bucket.created),
      showArea: false,
    },
    {
      key: 'completed',
      label: t('analytics.throughput.completed'),
      color: 'var(--ds-icon-accent-green)',
      values: throughputBuckets.map((bucket) => bucket.completed),
      showArea: false,
    },
  ]);

  const agingBuckets = $derived(aging?.buckets || []);
  const agingCategories = $derived(
    agingBuckets.map((bucket) => t(`analytics.aging.buckets.${bucket.key}`)),
  );
  const agingSeries = $derived([
    {
      key: 'items',
      label: t('analytics.aging.itemCount'),
      color: 'var(--ds-icon-accent-orange)',
      values: agingBuckets.map((bucket) => bucket.item_count),
    },
  ]);

  const deliveryTrend = $derived(deliveryTime?.trend || []);
  const deliveryChartTrend = $derived(
    deliveryTrend.filter((point) => point.completed_items > 0),
  );
  const deliveryCategories = $derived(
    deliveryChartTrend.map((point) => formatDateOnly(point.start_date)),
  );
  const deliverySeries = $derived([
    {
      key: 'median',
      label: t('analytics.deliveryTime.median'),
      color: 'var(--ds-icon-accent-blue)',
      values: deliveryChartTrend.map((point) => point.median_days),
      showArea: false,
    },
    {
      key: 'p85',
      label: t('analytics.deliveryTime.p85'),
      color: 'var(--ds-icon-accent-teal)',
      values: deliveryChartTrend.map((point) => point.p85_days),
      dashed: true,
      showArea: false,
    },
  ]);

  // DataTable renders a falsy cell value as an em dash, so numeric columns are
  // stringified to keep a legitimate 0 visible.
  const attentionItems = $derived(health?.attention_items || []);
  const attentionColumns = $derived([
    { key: 'title', label: t('analytics.health.item'), slot: 'itemCell' },
    { key: 'status', label: t('analytics.health.status') },
    {
      key: 'age_days',
      label: t('analytics.health.age'),
      align: 'text-right',
      sortable: true,
      render: (item) => days(item.age_days),
    },
    { key: 'flags', label: t('analytics.health.signals'), slot: 'flagsCell' },
  ]);

  const throughputColumns = $derived([
    { key: 'start_date', label: t('analytics.throughput.period'), render: period },
    {
      key: 'created',
      label: t('analytics.throughput.created'),
      align: 'text-right',
      render: (bucket) => String(bucket.created),
    },
    {
      key: 'completed',
      label: t('analytics.throughput.completed'),
      align: 'text-right',
      render: (bucket) => String(bucket.completed),
    },
    {
      key: 'net_change',
      label: t('analytics.throughput.net'),
      align: 'text-right',
      render: (bucket) => signed(bucket.net_change),
    },
  ]);

  const agingBucketColumns = $derived([
    {
      key: 'key',
      label: t('analytics.aging.ageBand'),
      render: (bucket) => t(`analytics.aging.buckets.${bucket.key}`),
    },
    {
      key: 'item_count',
      label: t('analytics.aging.itemCount'),
      align: 'text-right',
      render: (bucket) => String(bucket.item_count),
    },
  ]);

  // by_status rows are keyed by status name, which can be null for items in a
  // deleted status — fall back to the index so DataTable's keyed each stays unique.
  const agingByStatus = $derived(
    (aging?.by_status || []).map((row, index) => ({
      ...row,
      rowKey: row.status || `__unset-${index}`,
    })),
  );
  const agingByStatusColumns = $derived([
    { key: 'status', label: t('analytics.aging.status') },
    {
      key: 'item_count',
      label: t('analytics.aging.itemCount'),
      align: 'text-right',
      sortable: true,
      render: (row) => String(row.item_count),
    },
    {
      key: 'median_days',
      label: t('analytics.aging.median'),
      align: 'text-right',
      sortable: true,
      render: (row) => days(row.median_days),
    },
    {
      key: 'p85_days',
      label: t('analytics.aging.p85'),
      align: 'text-right',
      sortable: true,
      render: (row) => days(row.p85_days),
    },
  ]);

  const oldestItems = $derived(aging?.oldest_items || []);
  const oldestItemColumns = $derived([
    { key: 'title', label: t('analytics.health.item'), slot: 'itemCell' },
    { key: 'status', label: t('analytics.health.status') },
    {
      key: 'age_days',
      label: t('analytics.health.age'),
      align: 'text-right',
      sortable: true,
      render: (item) => days(item.age_days),
    },
  ]);

  const deliveryTrendColumns = $derived([
    { key: 'start_date', label: t('analytics.deliveryTime.period'), render: period },
    {
      key: 'completed_items',
      label: t('analytics.deliveryTime.completed'),
      align: 'text-right',
      render: (point) => String(point.completed_items),
    },
    {
      key: 'median_days',
      label: t('analytics.deliveryTime.median'),
      align: 'text-right',
      render: (point) => (point.completed_items ? days(point.median_days) : '—'),
    },
    {
      key: 'p85_days',
      label: t('analytics.deliveryTime.p85'),
      align: 'text-right',
      render: (point) => (point.completed_items ? days(point.p85_days) : '—'),
    },
  ]);

  const slowestItems = $derived(deliveryTime?.slowest_items || []);
  const slowestItemColumns = $derived([
    { key: 'title', label: t('analytics.health.item'), slot: 'itemCell' },
    {
      key: 'completed_date',
      label: t('analytics.deliveryTime.completedDate'),
      render: (item) => formatDateOnly(item.completed_date),
    },
    {
      key: 'delivery_days',
      label: t('analytics.deliveryTime.duration'),
      align: 'text-right',
      sortable: true,
      render: (item) => days(item.delivery_days),
    },
  ]);
</script>

{#snippet itemCell(item)}
  <div class="flex min-w-0 items-baseline gap-2">
    <ItemKey {item} workspace={$currentWorkspace} />
    <button
      type="button"
      class="truncate text-left font-medium text-ds-text hover:text-ds-text-link hover:underline"
      onclick={() => openItem(item)}
    >
      {item.title}
    </button>
  </div>
{/snippet}

{#snippet flagsCell(item)}
  <div class="flex flex-wrap gap-1">
    {#each item.flags as flag}
      <Badge variant={flagVariant(flag)} size="xs" class="whitespace-nowrap">
        {t(`analytics.health.flags.${flag}`)}
      </Badge>
    {/each}
  </div>
{/snippet}

{#snippet statTiles(metrics, columnsClass, testId)}
  <div class="grid gap-4 {columnsClass}" data-testid={testId}>
    {#each metrics as metric (metric.key)}
      <StatCard
        icon={metric.icon}
        iconColor={metric.iconColor}
        label={metric.label}
        value={metric.value}
        appearance="minimal"
      />
    {/each}
  </div>
{/snippet}

<div class="mx-auto min-h-screen w-full max-w-[96rem] p-4 sm:p-6" data-testid="analytics-page">
  <PageHeader title={t('analytics.title')} subtitle={t('analytics.subtitle')} />

  <Card variant="raised" padding="default" rounded="lg" class="mb-4">
    <div class="flex flex-wrap items-end gap-4" data-testid="analytics-filters">
      <div class="flex w-full flex-col gap-1 sm:w-80">
        <Label for="analytics-collection" size="xs" color="subtle">
          {t('analytics.collection')}
        </Label>
        <Select
          id="analytics-collection"
          options={collectionOptions}
          bind:value={selectedCollection}
          size="small"
          disabled={collectionLoadError}
        />
      </div>
      <div class="flex w-full flex-col gap-1 sm:w-48">
        <Label for="analytics-range" size="xs" color="subtle">
          {t('analytics.dateRange')}
        </Label>
        <Select
          id="analytics-range"
          options={rangeOptions}
          value={selectedPreset}
          onchange={applyPreset}
          size="small"
        />
      </div>
      <div class="flex w-[calc(50%-0.5rem)] flex-col gap-1 sm:w-40">
        <Label for="analytics-start-date" size="xs" color="subtle">
          {t('analytics.from')}
        </Label>
        <Input
          id="analytics-start-date"
          type="date"
          size="small"
          max={endDate}
          bind:value={startDate}
          onchange={handleDateEdit}
        />
      </div>
      <div class="flex w-[calc(50%-0.5rem)] flex-col gap-1 sm:w-40">
        <Label for="analytics-end-date" size="xs" color="subtle">
          {t('analytics.to')}
        </Label>
        <Input
          id="analytics-end-date"
          type="date"
          size="small"
          min={startDate}
          bind:value={endDate}
          onchange={handleDateEdit}
        />
      </div>
    </div>
  </Card>

  {#if collectionLoadError}
    <div role="status">
      <AlertBox variant="warning" class="mb-4">
        {#snippet children()}{t('analytics.collectionLoadError')}{/snippet}
      </AlertBox>
    </div>
  {/if}

  {#if validationCode}
    <div role="alert">
      <AlertBox variant="error" class="mb-4">
        {#snippet children()}{t(`analytics.validation.${validationCode}`)}{/snippet}
      </AlertBox>
    </div>
  {:else if loadError}
    <div role="alert" data-testid="analytics-error">
      <StateDisplay
        type="error"
        title={t('analytics.errorTitle')}
        message={loadError}
        onRetry={retry}
        retryLabel={t('analytics.retry')}
        class="py-16"
      />
    </div>
  {:else if loading}
    <div role="status" data-testid="analytics-loading">
      <StateDisplay type="loading" message={t('analytics.loading')} class="py-20" />
    </div>
  {:else if analyticsData}
    {#if dataset}
      <div class="mb-6 flex flex-wrap items-center gap-3 border-y border-ds-border py-3">
        <Info class="h-4 w-4 shrink-0 text-ds-icon-subtle" aria-hidden="true" />
        <div class="min-w-0">
          <div class="text-xs font-semibold text-ds-text">
            {dataset.cohort_mode === 'current_collection'
              ? t('analytics.scope.currentCollection')
              : t('analytics.scope.currentWorkspace')}
          </div>
          <Text variant="subtle" size="xs">
            {t('analytics.scope.summary', {
              items: dataset.total_items,
              from: formatDateOnly(dataset.date_from, { year: 'numeric' }),
              to: formatDateOnly(dataset.date_to, { year: 'numeric' }),
            })}
          </Text>
        </div>
        <div
          class="min-w-0 flex-1 basis-full border-t border-ds-border pt-3 sm:basis-0 sm:border-t-0 sm:border-l sm:pt-0 sm:pl-3"
        >
          <Text variant="subtle" size="xs">
            {dataset.cohort_mode === 'current_collection'
              ? t('analytics.scope.currentCollectionNote')
              : t('analytics.scope.currentWorkspaceNote')}
          </Text>
        </div>
      </div>
    {/if}

    <section class="mb-8" aria-label={t('analytics.health.title')}>
      <SectionHeader title={t('analytics.health.title')} subtitle={t('analytics.health.description')}>
        {#snippet actions()}
          {#if health?.stale_after_days}
            <Text variant="subtle" size="xs">
              {t('analytics.health.staleHint', { days: health.stale_after_days })}
            </Text>
          {/if}
        {/snippet}
      </SectionHeader>

      <Card variant="raised" padding="default" rounded="lg" class="mb-6">
        {@render statTiles(
          healthMetrics,
          'grid-cols-2 md:grid-cols-3 xl:grid-cols-6',
          'analytics-health-stats',
        )}
      </Card>

      <SectionHeader title={t('analytics.health.attentionItems')} />
      <div data-testid="analytics-attention-table">
        <DataTable
          columns={attentionColumns}
          data={attentionItems}
          keyField="id"
          emptyMessage={t('analytics.health.allClear')}
          {itemCell}
          {flagsCell}
        />
      </div>
    </section>

    <!-- Splits at 2xl rather than xl: the StatCard tiles inside each panel need
         more room than the old icon-less summary strip did. Each panel spans a
         header row and a card row of a shared subgrid, so the two cards start
         at the same y even when one subtitle wraps and the other does not. -->
    <section
      class="mb-8 grid grid-cols-1 gap-6 2xl:grid-cols-2 2xl:grid-rows-[auto_1fr]"
      aria-label={t('analytics.throughput.title')}
    >
      <article
        class="min-w-0 2xl:grid 2xl:row-span-2 2xl:grid-rows-subgrid 2xl:items-start 2xl:gap-0"
        data-testid="analytics-throughput-panel"
      >
        <SectionHeader
          testId="analytics-throughput-header"
          title={t('analytics.throughput.title')}
          subtitle={t('analytics.throughput.description')}
        />
        <Card variant="raised" padding="default" rounded="lg">
          <div class="mb-4 border-b border-ds-border pb-4">
            {@render statTiles(
              throughputMetrics,
              'grid-cols-1 sm:grid-cols-3',
              'analytics-throughput-stats',
            )}
          </div>
          {#if throughputBuckets.length}
            <div aria-hidden="true">
              <Chart
                type="line"
                series={throughputSeries}
                categories={throughputCategories}
                maxXLabels={6}
                minHeight={180}
                maxHeight={240}
              />
            </div>
            <details class="mt-1 border-t border-ds-border">
              <summary class="cursor-pointer py-3 text-xs font-medium text-ds-text-link hover:underline">
                {t('analytics.dataTable.show')}
              </summary>
              <DataTable
                columns={throughputColumns}
                data={throughputBuckets}
                keyField="start_date"
                class="rounded-lg border"
              />
            </details>
            <div class="mt-3 border-t border-ds-border pt-3">
              <Text variant="subtle" size="xs">
                {t('analytics.throughput.definition')}
              </Text>
            </div>
          {/if}
        </Card>
      </article>

      <article
        class="min-w-0 2xl:grid 2xl:row-span-2 2xl:grid-rows-subgrid 2xl:items-start 2xl:gap-0"
        data-testid="analytics-aging-panel"
      >
        <SectionHeader
          testId="analytics-aging-header"
          title={t('analytics.aging.title')}
          subtitle={t('analytics.aging.description')}
        />
        <Card variant="raised" padding="default" rounded="lg">
          {#if aging?.total_items}
            <div class="mb-4 border-b border-ds-border pb-4">
              {@render statTiles(
                agingMetrics,
                'grid-cols-1 sm:grid-cols-3',
                'analytics-aging-stats',
              )}
            </div>
            <div aria-hidden="true">
              <Chart
                type="bar"
                series={agingSeries}
                categories={agingCategories}
                maxXLabels={5}
                minHeight={180}
                maxHeight={240}
              />
            </div>
            <details class="mt-1 border-t border-ds-border">
              <summary class="cursor-pointer py-3 text-xs font-medium text-ds-text-link hover:underline">
                {t('analytics.dataTable.show')}
              </summary>
              <DataTable
                columns={agingBucketColumns}
                data={agingBuckets}
                keyField="key"
                class="rounded-lg border"
              />
            </details>
          {:else}
            <StateDisplay type="empty" title={t('analytics.aging.noActive')} />
          {/if}
        </Card>
      </article>
    </section>

    {#if aging?.total_items}
      <section
        class="mb-8 grid grid-cols-1 gap-6 xl:grid-cols-2 xl:grid-rows-[auto_1fr]"
        aria-label={t('analytics.aging.byStatus')}
      >
        <article class="min-w-0 xl:grid xl:row-span-2 xl:grid-rows-subgrid xl:items-start xl:gap-0">
          <SectionHeader title={t('analytics.aging.byStatus')} />
          <DataTable columns={agingByStatusColumns} data={agingByStatus} keyField="rowKey" />
        </article>

        <article class="min-w-0 xl:grid xl:row-span-2 xl:grid-rows-subgrid xl:items-start xl:gap-0">
          <SectionHeader title={t('analytics.aging.oldest')} />
          <DataTable
            columns={oldestItemColumns}
            data={oldestItems}
            keyField="id"
            {itemCell}
          />
        </article>
      </section>
    {/if}

    <section class="mb-8" aria-label={t('analytics.deliveryTime.title')}>
      <SectionHeader
        title={t('analytics.deliveryTime.title')}
        subtitle={t('analytics.deliveryTime.description')}
      />
      <Card variant="raised" padding="default" rounded="lg">
        {#if deliveryTime?.total_items_analyzed}
          <div class="mb-4 border-b border-ds-border pb-4">
            {@render statTiles(
              deliveryMetrics,
              'grid-cols-2 lg:grid-cols-4',
              'analytics-delivery-stats',
            )}
          </div>

          {#if !deliveryTime.data_quality?.sufficient}
            <div role="status">
              <AlertBox variant="warning" class="mb-4">
                {#snippet children()}
                  {t(`analytics.insufficientData.${deliveryTime.data_quality.reason}`)}
                {/snippet}
              </AlertBox>
            </div>
          {/if}

          <div aria-hidden="true">
            <Chart
              type="line"
              series={deliverySeries}
              categories={deliveryCategories}
              valueFormat={days}
              yAxisFormat={formatDayNumber}
              maxXLabels={8}
              minHeight={200}
              maxHeight={280}
            />
          </div>

          <details class="mt-1 border-t border-ds-border">
            <summary class="cursor-pointer py-3 text-xs font-medium text-ds-text-link hover:underline">
              {t('analytics.dataTable.show')}
            </summary>
            <DataTable
              columns={deliveryTrendColumns}
              data={deliveryTrend}
              keyField="start_date"
              class="rounded-lg border"
            />
          </details>

          <div class="mt-6">
            <h4 class="mb-2 text-sm font-semibold text-ds-text">
              {t('analytics.deliveryTime.slowest')}
            </h4>
            <DataTable
              columns={slowestItemColumns}
              data={slowestItems}
              keyField="id"
              {itemCell}
            />
          </div>

          <div class="mt-3 border-t border-ds-border pt-3">
            <Text variant="subtle" size="xs">
              {t('analytics.deliveryTime.definition')}
            </Text>
          </div>
        {:else}
          <StateDisplay
            type="empty"
            title={t(
              `analytics.insufficientData.${deliveryTime?.data_quality?.reason ||
                'no_completed_items'}`,
            )}
          />
        {/if}

        {#if deliveryTime?.missing_history_items > 0}
          <div role="status">
            <AlertBox variant="warning" class="mt-4">
              {#snippet children()}
                {t('analytics.deliveryTime.missingHistory', {
                  count: deliveryTime.missing_history_items,
                })}
              {/snippet}
            </AlertBox>
          </div>
        {/if}
      </Card>
    </section>
  {/if}
</div>
