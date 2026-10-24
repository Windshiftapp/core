<script>
  import { sla } from '../../api/sla.js';
  import { statusCategories as statusCategoriesApi, statuses as statusesApi } from '../../api/workflows.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import SlaMetricEditor from './SlaMetricEditor.svelte';
  import Gauge from '@lucide/svelte/icons/gauge';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Edit from '@lucide/svelte/icons/edit';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';

  let { workspaceId = null } = $props();

  let metrics = $state([]);
  let calendars = $state([]);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showEditor = $state(false);
  let editing = $state(null);

  function metricToPayload(metric) {
    return {
      name: metric.name,
      display_format: metric.display_format,
      position: metric.position ?? 0,
      is_active: metric.is_active,
      import_status: metric.import_status || 'native',
      conditions: (metric.conditions ?? []).map((condition, index) => ({
        phase: condition.phase,
        position: condition.position ?? index,
        condition_type: condition.condition_type,
        config: condition.config ?? {},
      })),
      goals: (metric.goals ?? []).map((goal, index) => ({
        position: goal.position ?? index,
        ql_query: goal.ql_query ?? '',
        import_status: goal.import_status || 'native',
        // A goal has exactly one target; priority-specific durations are separate goals.
        targets: (goal.targets ?? []).slice(0, 1).map((target, targetIndex) => ({
          position: target.position ?? targetIndex,
          target_ms: target.target_ms,
          calendar_id: target.calendar_id,
        })),
      })),
    };
  }

  async function loadMetrics(workspace = workspaceId, generation = requestGeneration) {
    try {
      loading = true;
      error = null;
      const result = await sla.getMetrics(workspace);
      if (generation === requestGeneration) metrics = result ?? [];
    } catch (err) {
      if (generation === requestGeneration) {
        error = err?.message || t('workspaceSettings.serviceLevels.loadMetricsFailed');
      }
    } finally {
      if (generation === requestGeneration) loading = false;
    }
  }

  async function loadReferenceData(workspace, generation) {
    const [calendarList, statusList, categoryList] = await Promise.all([
      sla.getAvailableCalendars(workspace).catch(() => []),
      statusesApi.getAll().catch(() => []),
      statusCategoriesApi.getAll().catch(() => []),
    ]);
    if (generation !== requestGeneration) return;
    calendars = calendarList ?? [];
    statuses = statusList ?? [];
    statusCategories = categoryList ?? [];
  }

  let requestGeneration = 0;
  $effect(() => {
    const workspace = workspaceId;
    if (!workspace) return;
    const generation = ++requestGeneration;
    void Promise.all([loadMetrics(workspace, generation), loadReferenceData(workspace, generation)]);
    return () => {
      if (generation === requestGeneration) requestGeneration++;
    };
  });

  function startCreate() {
    editing = null;
    showEditor = true;
  }

  function startEdit(metric) {
    editing = metric;
    showEditor = true;
  }

  async function save(payload) {
    if (editing) {
      await sla.updateMetric(workspaceId, editing.id, payload);
      successToast(t('workspaceSettings.serviceLevels.metricUpdated'));
    } else {
      await sla.createMetric(workspaceId, payload);
      successToast(t('workspaceSettings.serviceLevels.metricCreated'));
    }
    await loadMetrics();
  }

  async function remove(metric) {
    const ok = await confirm({
      title: t('workspaceSettings.serviceLevels.deleteMetric'),
      message: t('workspaceSettings.serviceLevels.deleteMetricConfirm', { name: metric.name }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await sla.deleteMetric(workspaceId, metric.id);
      successToast(t('workspaceSettings.serviceLevels.metricDeleted'));
      await loadMetrics();
    } catch (err) {
      errorToast(err?.message || t('workspaceSettings.serviceLevels.deleteMetricFailed'));
    }
  }

  async function toggleActive(metric) {
    try {
      await sla.updateMetric(workspaceId, metric.id, {
        ...metricToPayload(metric),
        is_active: !metric.is_active,
      });
      await loadMetrics();
    } catch (err) {
      errorToast(err?.message || t('workspaceSettings.serviceLevels.saveFailed'));
    }
  }

  async function move(index, delta) {
    const target = index + delta;
    if (target < 0 || target >= metrics.length) return;
    const current = metrics[index];
    const swapped = metrics[target];
    try {
      await sla.updateMetric(workspaceId, current.id, {
        ...metricToPayload(current),
        position: swapped.position ?? target,
      });
      await sla.updateMetric(workspaceId, swapped.id, {
        ...metricToPayload(swapped),
        position: current.position ?? index,
      });
      await loadMetrics();
    } catch (err) {
      errorToast(err?.message || t('workspaceSettings.serviceLevels.reorderFailed'));
    }
  }

  const columns = $derived([
    { key: 'position', label: t('workspaceSettings.serviceLevels.order'), width: '80px' },
    { key: 'name', label: t('common.name') },
    { key: 'display_format', label: t('workspaceSettings.serviceLevels.displayFormat'), width: '140px' },
    { key: 'conditions', label: t('workspaceSettings.serviceLevels.conditions'), width: '120px', slot: 'conditions' },
    { key: 'is_active', label: t('common.active'), width: '120px', slot: 'is_active' },
    { key: 'actions', label: t('common.actions') },
  ]);

  function conditionCount(metric) {
    return (metric.conditions ?? []).length;
  }

  function buildActions(metric, index) {
    return [
      {
        id: 'up',
        testid: `sla-metric-up-${metric.id}`,
        type: 'regular',
        icon: ArrowUp,
        title: t('common.moveUp'),
        onClick: () => move(index, -1),
      },
      {
        id: 'down',
        testid: `sla-metric-down-${metric.id}`,
        type: 'regular',
        icon: ArrowDown,
        title: t('common.moveDown'),
        onClick: () => move(index, 1),
      },
      {
        id: 'edit',
        testid: `sla-metric-edit-${metric.id}`,
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => startEdit(metric),
      },
      {
        id: 'delete',
        testid: `sla-metric-delete-${metric.id}`,
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(metric),
      },
    ];
  }
</script>

<PageHeader
  icon={Gauge}
  title={t('workspaceSettings.serviceLevels.metricsTitle')}
  subtitle={t('workspaceSettings.serviceLevels.metricsSubtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={loading}
      dataTestid="sla-metric-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('serviceLevels', 'add'), guard: () => !showEditor }}
    >
      {t('workspaceSettings.serviceLevels.createMetric')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-metrics-error">
    {error}
  </div>
{/if}

<DataTable
  columns={columns}
  data={metrics}
  keyField="id"
  {loading}
  emptyMessage={t('workspaceSettings.serviceLevels.noMetrics')}
  emptyIcon={Gauge}
  actionItems={buildActions}
  actionTriggerTestid={(metric) => `sla-metric-actions-${metric.id}`}
  rowAttrs={(metric) => ({ 'data-testid': `sla-metric-row-${metric.id}` })}
>
  {#snippet conditions(metric)}
    <span class="text-xs" style="color: var(--ds-text-subtle)">{conditionCount(metric)}</span>
  {/snippet}
  {#snippet is_active(metric)}
    <Toggle
      checked={metric.is_active}
      dataTestid="sla-metric-active-toggle-{metric.id}"
      onchange={() => toggleActive(metric)}
    />
  {/snippet}
</DataTable>

<SlaMetricEditor
  bind:isOpen={showEditor}
  metric={editing}
  {workspaceId}
  {calendars}
  {statuses}
  {statusCategories}
  onSave={save}
  onClose={() => {
    showEditor = false;
    editing = null;
  }}
/>
