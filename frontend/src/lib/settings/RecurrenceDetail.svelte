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
  import Card from '../components/Card.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import { ArrowLeft, Repeat, Zap, FileText } from 'lucide-svelte';

  let { workspaceId, ruleId, onback } = $props();

  let rule = $state(null);
  let loading = $state(true);
  let activeTab = $state('settings');

  // Instances
  let instances = $state([]);
  let instancesPagination = $state({ limit: 20, offset: 0, total: 0 });
  let loadingInstances = $state(false);
  let generating = $state(false);

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
        limit: instancesPagination.limit,
        offset: instancesPagination.offset,
      });
      instances = result.instances || [];
      instancesPagination = { ...instancesPagination, ...result.pagination };
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
        message: t('recurrence.generated', { count: result.instances_generated }),
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

  function handleInstancePageChange(newOffset) {
    instancesPagination.offset = newOffset;
    loadInstances();
  }
</script>

<div class="flex flex-col">
  <!-- Header -->
  <div class="pb-4 mb-4 border-b" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3 mb-4">
      <button
        onclick={goBack}
        class="p-1.5 rounded-md transition-colors"
        style="color: var(--ds-text-subtle);"
        onmouseenter={(e) => { e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'; e.currentTarget.style.color = 'var(--ds-text)'; }}
        onmouseleave={(e) => { e.currentTarget.style.background = ''; e.currentTarget.style.color = 'var(--ds-text-subtle)'; }}
      >
        <ArrowLeft class="w-5 h-5" />
      </button>
      <div>
        <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
          {#if rule}
            {rule.template_item_title || `Item #${rule.template_item_id}`}
          {:else}
            {t('recurrence.title')}
          {/if}
        </h1>
        {#if rule}
          <div class="flex items-center gap-2 mt-1">
            <span class="text-sm" style="color: var(--ds-text-subtle);">{rruleToText(rule.rrule)}</span>
            <Lozenge
              color={rule.is_active ? 'green' : 'neutral'}
              text={rule.is_active ? t('recurrence.active') : t('recurrence.inactive')}
            />
          </div>
        {/if}
      </div>
    </div>

    <!-- Tabs -->
    <div class="flex gap-1">
      <button
        class="px-4 py-2 text-sm font-medium rounded-t-md transition-colors"
        style={activeTab === 'settings' ? 'background: var(--ds-surface); color: var(--ds-text); border: 1px solid var(--ds-border); border-bottom: none;' : 'color: var(--ds-text-subtle);'}
        onclick={() => switchTab('settings')}
      >
        {t('recurrence.settingsTab')}
      </button>
      <button
        class="px-4 py-2 text-sm font-medium rounded-t-md transition-colors"
        style={activeTab === 'instances' ? 'background: var(--ds-surface); color: var(--ds-text); border: 1px solid var(--ds-border); border-bottom: none;' : 'color: var(--ds-text-subtle);'}
        onclick={() => switchTab('instances')}
      >
        {t('recurrence.instancesTab')}
        {#if instancesPagination.total > 0}
          <span class="ml-1.5 px-1.5 py-0.5 text-xs rounded-full" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
            {instancesPagination.total}
          </span>
        {/if}
      </button>
    </div>
  </div>

  <!-- Content -->
  <div>
    {#if loading}
      <div class="text-center py-12 animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
    {:else if !rule}
      <EmptyState icon={Repeat} title={t('common.notFound')} description={t('recurrence.empty')} />
    {:else if activeTab === 'settings'}
      <div class="max-w-xl">
        <RecurrenceEditor
          itemId={rule.template_item_id}
          existingRule={rule}
          onsave={handleSave}
          oncancel={goBack}
          ondelete={handleDelete}
        />
      </div>
    {:else if activeTab === 'instances'}
      <!-- Instances Tab -->
      <div class="space-y-4">
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
          <div class="text-center py-8 animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
        {:else if instances.length === 0}
          <Card rounded="xl" shadow padding="generous">
            <EmptyState
              icon={FileText}
              title={t('recurrence.noInstances')}
              description={t('recurrence.noInstances')}
            />
          </Card>
        {:else}
          <!-- Instances table -->
          <div class="border rounded-lg overflow-hidden" style="border-color: var(--ds-border);">
            <table class="w-full text-sm">
              <thead>
                <tr style="background: var(--ds-surface-raised);">
                  <th class="text-left px-4 py-2.5 font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">
                    {t('recurrence.sequenceNumber')}
                  </th>
                  <th class="text-left px-4 py-2.5 font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">
                    {t('recurrence.scheduledDate')}
                  </th>
                  <th class="text-left px-4 py-2.5 font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">
                    {t('recurrence.templateItem')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {#each instances as instance (instance.id)}
                  <tr
                    style="border-bottom: 1px solid var(--ds-border);"
                    onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
                    onmouseleave={(e) => e.currentTarget.style.background = ''}
                  >
                    <td class="px-4 py-2.5" style="color: var(--ds-text-subtle);">
                      #{instance.sequence_number}
                    </td>
                    <td class="px-4 py-2.5" style="color: var(--ds-text);">
                      {formatDate(instance.scheduled_date)}
                    </td>
                    <td class="px-4 py-2.5" style="color: var(--ds-text);">
                      {#if instance.instance_item_id}
                        Item #{instance.instance_item_id}
                      {:else}
                        -
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>

          <!-- Pagination -->
          {#if instancesPagination.total > instancesPagination.limit}
            <div class="flex items-center justify-between text-sm" style="color: var(--ds-text-subtle);">
              <span>
                Showing {instancesPagination.offset + 1}-{Math.min(instancesPagination.offset + instancesPagination.limit, instancesPagination.total)} of {instancesPagination.total}
              </span>
              <div class="flex gap-2">
                <Button
                  variant="default"
                  size="small"
                  disabled={instancesPagination.offset === 0}
                  onclick={() => handleInstancePageChange(Math.max(0, instancesPagination.offset - instancesPagination.limit))}
                >
                  Previous
                </Button>
                <Button
                  variant="default"
                  size="small"
                  disabled={instancesPagination.offset + instancesPagination.limit >= instancesPagination.total}
                  onclick={() => handleInstancePageChange(instancesPagination.offset + instancesPagination.limit)}
                >
                  Next
                </Button>
              </div>
            </div>
          {/if}
        {/if}
      </div>
    {/if}
  </div>
</div>
