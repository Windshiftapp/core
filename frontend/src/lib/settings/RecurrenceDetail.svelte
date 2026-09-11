<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { confirm } from '../composables/useConfirm.js';
  import { addToast, errorToast } from '../stores/toasts.svelte.js';
  import { rruleToText } from '../editors/rruleUtils.js';
  import RecurrenceEditor from '../editors/RecurrenceEditor.svelte';
  import Button from '../components/Button.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import DataTable from '../components/DataTable.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import Tabs from '../components/Tabs.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { ArrowLeft, Repeat, Zap, FileText } from '@lucide/svelte';

  let { workspaceId, ruleId, onback } = $props();

  let rule = $state(null);
  let loading = $state(true);
  let activeTab = $state('settings');
  let instancesPage = $state(1);

  // Instances
  let instances = $state([]);
  let instancesPagination = $state({ page: 1, page_size: 20, total_items: 0, total_pages: 0 });
  let loadingInstances = $state(false);
  let generating = $state(false);

  const tabs = $derived([
    {
      id: 'settings',
      label: t('recurrence.settingsTab'),
      testid: 'recurrence-settings-tab',
    },
    {
      id: 'instances',
      label: t('recurrence.instancesTab'),
      badge: instancesPagination.total_items > 0 ? String(instancesPagination.total_items) : null,
      testid: 'recurrence-instances-tab',
    },
  ]);

  onMount(() => {
    loadRule();
  });

  async function loadRule() {
    loading = true;
    try {
      const rules = await api.recurrence.listByWorkspace(workspaceId);
      const found = rules?.find(r => String(r.id) === String(ruleId));
      if (found) {
        rule = found;
      }
    } catch (err) {
      console.error('Failed to load recurrence rule:', err);
    } finally {
      loading = false;
    }
  }

  async function loadInstances() {
    if (!rule) return;
    loadingInstances = true;
    try {
      const result = await api.recurrence.getInstances(rule.template_item_id, {
        page: instancesPage,
        page_size: instancesPagination.page_size,
      });
      instances = result.data || [];
      instancesPagination = result.pagination;
    } catch (err) {
      console.error('Failed to load instances:', err);
      instances = [];
    } finally {
      loadingInstances = false;
    }
  }

  async function handleForceGenerate() {
    if (!rule) return;
    generating = true;
    try {
      const result = await api.recurrence.forceGenerate(rule.template_item_id);
      addToast({
        message: t('recurrence.generated', { count: result.generated_count }),
        variant: 'success',
      });
      await loadInstances();
    } catch (err) {
      errorToast(err.message || t('errors.UNKNOWN'));
    } finally {
      generating = false;
    }
  }

  async function handleSave(updatedRule) {
    rule = updatedRule;
    addToast({ message: t('common.saved'), variant: 'success' });
  }

  async function handleDelete() {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('recurrence.deleteConfirm'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await api.recurrence.delete(rule.template_item_id);
      goBack();
    } catch (err) {
      errorToast(err.message || t('errors.UNKNOWN'));
    }
  }

  function goBack() {
    onback?.();
  }

  function switchTab(tab) {
    activeTab = tab;
    if (tab === 'instances' && instances.length === 0) {
      loadInstances();
    }
  }

  function formatDate(dateStr) {
    if (!dateStr) return '-';
    try {
      return new Date(dateStr).toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      });
    } catch {
      return dateStr;
    }
  }

  const instanceColumns = [
    { key: 'sequence_number', label: t('recurrence.sequenceNumber'), render: (i) => `#${i.sequence_number}`, textColor: 'var(--ds-text-subtle)' },
    { key: 'scheduled_date', label: t('recurrence.scheduledDate'), render: (i) => formatDate(i.scheduled_date) },
    { key: 'instance_item_id', label: t('recurrence.templateItem'), render: (i) => i.instance_item_id ? `Item #${i.instance_item_id}` : '-' },
  ];

  function handleInstancePageChange(page) {
    instancesPage = page;
    loadInstances();
  }
</script>

<div class="flex flex-col" data-testid="recurrence-detail">
  {#if loading}
    <StateDisplay type="loading" message={t('common.loading')} />
  {:else if !rule}
    <EmptyState icon={Repeat} title={t('common.notFound')} description={t('recurrence.empty')} />
  {:else}
    <div class="flex items-start gap-3">
      <Button
        variant="ghost"
        size="small"
        icon={ArrowLeft}
        onclick={goBack}
        dataTestid="recurrence-detail-back"
        title={t('common.back')}
        class="mt-0.5 px-2"
      />
      <div class="min-w-0 flex-1">
        <PageHeader
          title={rule.template_title || `Item #${rule.template_item_id}`}
          subtitle={rruleToText(rule.rrule)}
        >
          {#snippet actions()}
            <Lozenge
              color={rule.is_active ? 'green' : 'neutral'}
              text={rule.is_active ? t('recurrence.active') : t('recurrence.inactive')}
            />
          {/snippet}
        </PageHeader>
      </div>
    </div>

    <Tabs {tabs} bind:activeTab onTabChange={({ tab }) => switchTab(tab)}>
      {#if activeTab === 'settings'}
        <div class="max-w-2xl">
          <RecurrenceEditor
            itemId={rule.template_item_id}
            existingRule={rule}
            onsave={handleSave}
            oncancel={goBack}
            ondelete={handleDelete}
          />
        </div>
      {:else if activeTab === 'instances'}
        <div class="space-y-4" data-testid="recurrence-instances-panel">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-medium" style="color: var(--ds-text-subtle);">
              {t('recurrence.instances')}
            </h3>
            <Button
              variant="default"
              size="small"
              icon={Zap}
              onclick={handleForceGenerate}
              disabled={generating}
            >
              {generating ? t('recurrence.generating') : t('recurrence.forceGenerate')}
            </Button>
          </div>

          {#if loadingInstances}
            <StateDisplay type="loading" message={t('common.loading')} />
          {:else}
            <DataTable
              columns={instanceColumns}
              data={instances}
              keyField="id"
              emptyIcon={FileText}
              emptyMessage={t('recurrence.noInstances')}
              emptyDescription={t('recurrence.noInstances')}
              pagination
              pageSize={instancesPagination.page_size}
              bind:currentPage={instancesPage}
              totalItems={instancesPagination.total_items}
              onPageChange={handleInstancePageChange}
            />
          {/if}
        </div>
      {/if}
    </Tabs>
  {/if}
</div>
