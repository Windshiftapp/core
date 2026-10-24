<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { recurrence } from '../api/recurrence.js';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Repeat from '@lucide/svelte/icons/repeat';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { rruleToText } from '../editors/rruleUtils.js';
  import RecurrenceDetail from './RecurrenceDetail.svelte';

  let { workspaceId } = $props();

  let rules = $state([]);
  let loading = $state(true);
  let searchQuery = $state('');
  let selectedRuleId = $state(null);

  const filteredRules = $derived(
    searchQuery.trim() === ''
      ? rules
      : rules.filter(r =>
          r.template_title?.toLowerCase().includes(searchQuery.toLowerCase()) ||
          rruleToText(r.rrule)?.toLowerCase().includes(searchQuery.toLowerCase())
        )
  );

  const ruleColumns = $derived([
    { key: 'template_title', label: t('recurrence.templateItem'), slot: 'template' },
    { key: 'rrule', label: t('recurrence.rule'), render: (rule) => rruleToText(rule.rrule) },
    { key: 'instance_count', label: t('recurrence.instances'), render: (rule) => String(rule.instance_count ?? 0) },
    { key: 'is_active', label: t('common.status'), slot: 'status' },
    { key: 'actions', label: t('common.actions') },
  ]);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;
      const wsRules = await recurrence.listByWorkspace(workspaceId);
      rules = wsRules || [];
    } catch (error) {
      console.error('Failed to load recurrence rules:', error);
      rules = [];
    } finally {
      loading = false;
    }
  }

  function viewRule(rule) {
    selectedRuleId = rule.id;
  }

  function handleBack() {
    selectedRuleId = null;
    loadData();
  }

  async function deleteRule(rule) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('recurrence.deleteConfirm'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      // Delete via the template item endpoint
      await recurrence.delete(rule.template_item_id);
      rules = rules.filter(r => r.id !== rule.id);
    } catch (error) {
      console.error('Failed to delete recurrence rule:', error);
      errorToast(error?.message || t('errors.UNKNOWN'));
    }
  }

  function buildRuleActions(rule) {
    return [
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        testid: 'recurrence-rule-delete',
        color: 'var(--ds-text-danger)',
        onClick: () => deleteRule(rule),
      },
    ];
  }
</script>

{#if selectedRuleId}
  <RecurrenceDetail {workspaceId} ruleId={selectedRuleId} onback={handleBack} />
{:else}

  <div data-testid="recurrence-manager" class="space-y-4">
    <SearchInput
      bind:value={searchQuery}
      placeholder={t('recurrence.searchPlaceholder')}
      dataTestid="recurrence-search"
      class="max-w-md"
    />

    {#if loading}
      <div class="rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
        <StateDisplay type="loading" message={t('common.loading')} />
      </div>
    {:else}
      <DataTable
        columns={ruleColumns}
        data={filteredRules}
        keyField="id"
        emptyIcon={Repeat}
        emptyMessage={searchQuery.trim() ? t('search.noSearchResults') : t('recurrence.empty')}
        emptyDescription={searchQuery.trim() ? t('recurrence.noMatchingResults') : t('recurrence.emptyDesc')}
        onRowClick={viewRule}
        actionItems={buildRuleActions}
        actionTriggerTestid={(rule) => `recurrence-rule-actions-${rule.template_item_id}`}
        rowAttrs={(rule) => ({ 'data-testid': `recurrence-rule-row-${rule.template_item_id}` })}
      >
        {#snippet template(rule)}
          <Button
            variant="link"
            dataTestid="recurrence-rule-open"
            onclick={(event) => {
              event.stopPropagation();
              viewRule(rule);
            }}
          >
            {rule.template_title || `Item #${rule.template_item_id}`}
          </Button>
        {/snippet}
        {#snippet status(rule)}
          <Lozenge
            color={rule.is_active ? 'green' : 'neutral'}
            text={rule.is_active ? t('recurrence.active') : t('recurrence.inactive')}
          />
        {/snippet}
      </DataTable>
    {/if}
  </div>
{/if}
