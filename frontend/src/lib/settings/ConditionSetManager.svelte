<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Plus, Edit, Trash2, Search } from '@lucide/svelte';
  import { IconBarrierBlock } from '@tabler/icons-svelte-runes';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Panel from '../components/Panel.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let conditionSets = $state([]);
  let loading = $state(true);
  let searchQuery = $state('');
  let searchTimeout;

  // Filter condition sets by search query (client-side)
  const filteredConditionSets = $derived(
    searchQuery.trim() === ''
      ? conditionSets
      : conditionSets.filter(cs =>
          cs.name?.toLowerCase().includes(searchQuery.toLowerCase()) ||
          cs.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
          cs.workflow_name?.toLowerCase().includes(searchQuery.toLowerCase())
        )
  );

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;
      conditionSets = (await api.conditionSets.getAll()) || [];
    } catch (error) {
      console.error('Failed to load condition sets:', error);
      conditionSets = [];
    } finally {
      loading = false;
    }
  }

  function startCreating() {
    navigate('/admin/condition-sets/new');
  }

  function startEditing(cs) {
    navigate(`/admin/condition-sets/${cs.id}`);
  }

  async function deleteConditionSet(cs) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteItem', { name: cs.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.conditionSets.delete(cs.id);
      conditionSets = conditionSets.filter(c => c.id !== cs.id);
    } catch (error) {
      console.error('Failed to delete condition set:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  function getGatedTransitions(cs) {
    return cs.gated_transitions || [];
  }
</script>

{#snippet headerActions()}
  <Button
    variant="primary"
    icon={Plus}
    onclick={startCreating}
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString('conditionSets', 'add') }}
  >
    {t('conditionSets.add')}
  </Button>
{/snippet}

<PageHeader
  icon={IconBarrierBlock}
  title={t('conditionSets.title')}
  subtitle={t('conditionSets.subtitle')}
  actions={headerActions}
/>

<!-- Search Bar -->
<div class="mb-6">
  <div class="relative max-w-md">
    <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-icon-subtle);" />
    <input
      type="text"
      placeholder={t('conditionSets.searchPlaceholder')}
      bind:value={searchQuery}
      class="w-full pl-9 pr-4 py-2 border rounded text-sm focus:outline-none focus:ring-2"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
    />
  </div>
</div>

{#if loading}
  <Panel padding="spacious" class="text-center">
    <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
  </Panel>
{:else if filteredConditionSets.length === 0 && searchQuery.trim() === ''}
  <Panel padding="spacious">
    <EmptyState
      icon={IconBarrierBlock}
      title={t('conditionSets.empty')}
      description={t('conditionSets.emptyDesc')}
    >
      {#snippet action()}
        <Button variant="primary" icon={Plus} onclick={startCreating}>
          {t('conditionSets.createFirst')}
        </Button>
      {/snippet}
    </EmptyState>
  </Panel>
{:else if filteredConditionSets.length === 0}
  <Panel padding="spacious">
    <EmptyState
      icon={Search}
      title={t('search.noSearchResults')}
      description={t('conditionSets.noMatchingResults')}
    />
  </Panel>
{:else}
  <div class="space-y-3">
    {#each filteredConditionSets as cs (cs.id)}
      <Panel padding="spacious" hoverable>
        <div class="flex items-center justify-between">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-3 mb-2">
              <h3 class="text-lg font-medium" style="color: var(--ds-text);">{cs.name}</h3>
            </div>

            {#if cs.description}
              <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">{cs.description}</p>
            {/if}

            <div class="flex items-center gap-4 text-sm">
              <div class="flex items-center gap-1.5">
                <span style="color: var(--ds-text-subtle);">{t('conditionSets.workflow')}:</span>
                <span class="font-medium" style="color: var(--ds-text);">
                  {cs.workflow_name || t('common.none')}
                </span>
              </div>
              {#if getGatedTransitions(cs).length > 0}
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span style="color: var(--ds-text-subtle);">{t('conditionSets.transitions')}:</span>
                  {#each getGatedTransitions(cs) as gt (gt.transition_id)}
                    <Lozenge color="blue" text={`${gt.from_status_name || 'Initial'} → ${gt.to_status_name}`} />
                  {/each}
                </div>
              {/if}
            </div>
          </div>

          <div class="flex items-center gap-2 ml-4 flex-shrink-0">
            <Button
              variant="default"
              size="small"
              icon={Edit}
              onclick={() => startEditing(cs)}
            >
              {t('common.edit')}
            </Button>
            <Button
              variant="default"
              size="small"
              icon={Trash2}
              onclick={() => deleteConditionSet(cs)}
            >
              {t('common.delete')}
            </Button>
          </div>
        </div>
      </Panel>
    {/each}
  </div>
{/if}
