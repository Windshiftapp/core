<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { api } from '../api.js';
  import { Trash2, Search, Repeat } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Card from '../components/Card.svelte';
  import Lozenge from '../components/Lozenge.svelte';
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
          r.template_item_title?.toLowerCase().includes(searchQuery.toLowerCase()) ||
          rruleToText(r.rrule)?.toLowerCase().includes(searchQuery.toLowerCase())
        )
  );

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;
      const wsRules = await api.recurrence.listByWorkspace(workspaceId);
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
      await api.recurrence.delete(rule.template_item_id);
      rules = rules.filter(r => r.id !== rule.id);
    } catch (error) {
      console.error('Failed to delete recurrence rule:', error);
      errorToast(error?.message || t('errors.UNKNOWN'));
    }
  }
</script>

{#if selectedRuleId}
  <RecurrenceDetail {workspaceId} ruleId={selectedRuleId} onback={handleBack} />
{:else}

<!-- Search Bar -->
<div class="mb-6">
  <div class="relative max-w-md">
    <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-icon-subtle);" />
    <input
      type="text"
      placeholder={t('recurrence.searchPlaceholder')}
      bind:value={searchQuery}
      class="w-full pl-9 pr-4 py-2 border rounded text-sm focus:outline-none focus:ring-2"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
    />
  </div>
</div>

{#if loading}
  <Card rounded="xl" shadow padding="loose" class="text-center">
    <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
  </Card>
{:else if filteredRules.length === 0 && searchQuery.trim() === ''}
  <Card rounded="xl" shadow padding="generous">
    <EmptyState
      icon={Repeat}
      title={t('recurrence.empty')}
      description={t('recurrence.emptyDesc')}
    />
  </Card>
{:else if filteredRules.length === 0}
  <Card rounded="xl" shadow padding="generous">
    <EmptyState
      icon={Search}
      title={t('search.noSearchResults')}
      description={t('recurrence.noMatchingResults')}
    />
  </Card>
{:else}
  <div class="space-y-3">
    {#each filteredRules as rule (rule.id)}
      <Card rounded="xl" shadow padding="spacious">
        <div class="flex items-center justify-between">
          <button
            class="flex-1 min-w-0 text-left cursor-pointer"
            onclick={() => viewRule(rule)}
          >
            <div class="flex items-center gap-3 mb-2">
              <h3 class="text-lg font-medium" style="color: var(--ds-text);">
                {rule.template_item_title || `Item #${rule.template_item_id}`}
              </h3>
              <Lozenge
                color={rule.is_active ? 'green' : 'neutral'}
                text={rule.is_active ? t('recurrence.active') : t('recurrence.inactive')}
              />
            </div>

            <div class="flex items-center gap-4 text-sm">
              <div class="flex items-center gap-1.5">
                <span style="color: var(--ds-text-subtle);">{t('recurrence.rule')}:</span>
                <span class="font-medium" style="color: var(--ds-text);">
                  {rruleToText(rule.rrule)}
                </span>
              </div>
              {#if rule.instance_count != null}
                <div class="flex items-center gap-1.5">
                  <span style="color: var(--ds-text-subtle);">{t('recurrence.instances')}:</span>
                  <Lozenge color="blue" text="{rule.instance_count}" />
                </div>
              {/if}
            </div>
          </button>

          <div class="flex items-center gap-2 ml-4 flex-shrink-0">
            <Button
              variant="default"
              size="small"
              icon={Trash2}
              onclick={() => deleteRule(rule)}
            >
              {t('common.delete')}
            </Button>
          </div>
        </div>
      </Card>
    {/each}
  </div>
{/if}

{/if}
