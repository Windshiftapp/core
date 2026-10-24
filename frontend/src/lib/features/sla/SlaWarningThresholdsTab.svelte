<script>
  import { sla } from '../../api/sla.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import BellRing from '@lucide/svelte/icons/bell-ring';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Edit from '@lucide/svelte/icons/edit';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { invalidateSLAThresholds } from './slaState.js';

  let { workspaceId = null } = $props();

  let thresholds = $state([]);
  let metrics = $state([]);
  let loading = $state(true);
  let error = $state(null);
  let showEditor = $state(false);
  let editing = $state(null);
  let saving = $state(false);
  let formError = $state(null);

  let formData = $state({ label: '', percent: 75, metric_id: null, is_active: true });

  const metricOptions = $derived([
    { value: null, label: t('workspaceSettings.serviceLevels.allMetrics') },
    ...metrics.map((metric) => ({ value: metric.id, label: metric.name })),
  ]);

  function blankForm() {
    return { label: '', percent: 75, metric_id: null, is_active: true };
  }

  async function loadThresholds() {
    try {
      loading = true;
      error = null;
      thresholds = (await sla.getWarningThresholds(workspaceId)) ?? [];
    } catch (err) {
      error = err?.message || t('workspaceSettings.serviceLevels.loadThresholdsFailed');
    } finally {
      loading = false;
    }
  }

  async function loadMetrics() {
    metrics = (await sla.getMetrics(workspaceId).catch(() => [])) ?? [];
  }

  // WorkspaceSettings keeps one component mounted across routes.
  let loadedWorkspaceId = null;
  $effect(() => {
    if (!workspaceId || workspaceId === loadedWorkspaceId) return;
    loadedWorkspaceId = workspaceId;
    void Promise.all([loadThresholds(), loadMetrics()]);
  });

  function startCreate() {
    editing = null;
    formData = blankForm();
    formError = null;
    showEditor = true;
  }

  function startEdit(threshold) {
    editing = threshold;
    formData = {
      label: threshold.label || '',
      percent: threshold.percent,
      metric_id: threshold.metric_id ?? null,
      is_active: threshold.is_active !== false,
    };
    formError = null;
    showEditor = true;
  }

  async function save() {
    formError = null;
    const percent = Number(formData.percent);
    if (!Number.isInteger(percent) || percent <= 0 || percent >= 100) {
      formError = t('workspaceSettings.serviceLevels.percentRange');
      return;
    }
    saving = true;
    try {
      const payload = {
        label: formData.label.trim(),
        percent,
        metric_id: formData.metric_id ?? null,
        is_active: formData.is_active,
      };
      if (editing) {
        await sla.updateWarningThreshold(workspaceId, editing.id, payload);
        successToast(t('workspaceSettings.serviceLevels.thresholdUpdated'));
      } else {
        await sla.createWarningThreshold(workspaceId, payload);
        successToast(t('workspaceSettings.serviceLevels.thresholdCreated'));
      }
      // Successful mutation: drop cached thresholds and refresh mounted badges.
      invalidateSLAThresholds(workspaceId);
      showEditor = false;
      await loadThresholds();
    } catch (err) {
      formError = err?.message || t('workspaceSettings.serviceLevels.saveFailed');
    } finally {
      saving = false;
    }
  }

  async function remove(threshold) {
    const ok = await confirm({
      title: t('workspaceSettings.serviceLevels.deleteThreshold'),
      message: t('workspaceSettings.serviceLevels.deleteThresholdConfirm', {
        label: threshold.label || `${threshold.percent}%`,
      }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await sla.deleteWarningThreshold(workspaceId, threshold.id);
      successToast(t('workspaceSettings.serviceLevels.thresholdDeleted'));
      invalidateSLAThresholds(workspaceId);
      await loadThresholds();
    } catch (err) {
      errorToast(err?.message || t('workspaceSettings.serviceLevels.saveFailed'));
    }
  }

  const columns = $derived([
    { key: 'label', label: t('common.name') },
    { key: 'scope', label: t('workspaceSettings.serviceLevels.scope'), width: '180px', slot: 'scope' },
    { key: 'percent', label: t('workspaceSettings.serviceLevels.percent'), width: '100px', slot: 'percent' },
    { key: 'is_active', label: t('common.active'), width: '110px', slot: 'is_active' },
    { key: 'actions', label: t('common.actions') },
  ]);

  function buildActions(threshold) {
    return [
      {
        id: 'edit',
        testid: `sla-threshold-edit-${threshold.id}`,
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => startEdit(threshold),
      },
      {
        id: 'delete',
        testid: `sla-threshold-delete-${threshold.id}`,
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(threshold),
      },
    ];
  }
</script>

<PageHeader
  icon={BellRing}
  title={t('workspaceSettings.serviceLevels.thresholdsTitle')}
  subtitle={t('workspaceSettings.serviceLevels.thresholdsSubtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={loading}
      dataTestid="sla-threshold-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('serviceLevels', 'add'), guard: () => !showEditor }}
    >
      {t('workspaceSettings.serviceLevels.createThreshold')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-thresholds-error">
    {error}
  </div>
{/if}

<DataTable
  columns={columns}
  data={thresholds}
  keyField="id"
  {loading}
  emptyMessage={t('workspaceSettings.serviceLevels.noThresholds')}
  emptyIcon={BellRing}
  actionItems={buildActions}
  actionTriggerTestid={(threshold) => `sla-threshold-actions-${threshold.id}`}
  rowAttrs={(threshold) => ({ 'data-testid': `sla-threshold-row-${threshold.id}` })}
>
  {#snippet scope(threshold)}
    <span class="text-xs" style="color: var(--ds-text-subtle)">
      {threshold.metric_name || t('workspaceSettings.serviceLevels.allMetrics')}
    </span>
  {/snippet}
  {#snippet percent(threshold)}
    <span>{threshold.percent}%</span>
  {/snippet}
  {#snippet is_active(threshold)}
    {#if threshold.is_active}
      <Lozenge color="green" text={t('common.active')} />
    {:else}
      <Lozenge color="gray" text={t('common.inactive')} />
    {/if}
  {/snippet}
</DataTable>

<Modal
  isOpen={showEditor}
  onclose={() => (showEditor = false)}
  maxWidth="max-w-xl"
  onSubmit={save}
  submitDisabled={saving}
  dataTestid="sla-threshold-dialog"
>
  {#snippet children()}
    <ModalHeader
      title={editing
        ? t('workspaceSettings.serviceLevels.editThreshold')
        : t('workspaceSettings.serviceLevels.createThreshold')}
      showCloseButton={false}
    />
    <div class="px-6 py-4">
      {#if formError}
        <div class="mb-3 text-sm" style="color: var(--ds-text-danger)" data-testid="sla-threshold-error">
          {formError}
        </div>
      {/if}
      <div class="form-group">
        <label for="sla-threshold-label">{t('common.name')}</label>
        <Input
          id="sla-threshold-label"
          type="text"
          bind:value={formData.label}
          placeholder={t('workspaceSettings.serviceLevels.thresholdLabelPlaceholder')}
          dataTestid="sla-threshold-label"
        />
      </div>
      <div class="form-group">
        <label for="sla-threshold-percent">{t('workspaceSettings.serviceLevels.percent')}</label>
        <Input
          id="sla-threshold-percent"
          type="number"
          min="1"
          max="99"
          bind:value={formData.percent}
          dataTestid="sla-threshold-percent"
        />
      </div>
      <div class="form-group">
        <label for="sla-threshold-scope">{t('workspaceSettings.serviceLevels.scope')}</label>
        <Select
          id="sla-threshold-scope"
          bind:value={formData.metric_id}
          options={metricOptions}
        />
      </div>
      <div class="form-group flex items-center gap-2">
        <Toggle bind:checked={formData.is_active} dataTestid="sla-threshold-active" />
        <span>{t('workspaceSettings.serviceLevels.thresholdActive')}</span>
      </div>
    </div>
    <DialogFooter
      onCancel={() => (showEditor = false)}
      onConfirm={save}
      confirmLabel={editing ? t('common.save') : t('common.create')}
      loading={saving}
      showKeyboardHint
      confirmTestid="sla-threshold-submit"
    />
  {/snippet}
</Modal>
