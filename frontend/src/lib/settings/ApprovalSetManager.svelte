<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Plus, Edit, Trash2, Search } from 'lucide-svelte';
  import { IconRubberStamp } from '@tabler/icons-svelte-runes';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Card from '../components/Card.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  let approvalSets = $state([]);
  let loading = $state(true);
  let searchQuery = $state('');

  const filtered = $derived(
    searchQuery.trim() === ''
      ? approvalSets
      : approvalSets.filter(s =>
          s.name?.toLowerCase().includes(searchQuery.toLowerCase()) ||
          s.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
          s.workflow_name?.toLowerCase().includes(searchQuery.toLowerCase())
        )
  );

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;
      approvalSets = (await api.approvalSets.getAll()) || [];
    } catch (error) {
      console.error('Failed to load approval sets:', error);
      approvalSets = [];
    } finally {
      loading = false;
    }
  }

  function startCreating() {
    navigate('/admin/approval-sets/new');
  }

  function startEditing(s) {
    navigate(`/admin/approval-sets/${s.id}`);
  }

  async function deleteApprovalSet(s) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteItem', { name: s.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.approvalSets.delete(s.id);
      approvalSets = approvalSets.filter(c => c.id !== s.id);
    } catch (error) {
      console.error('Failed to delete approval set:', error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }

  function getGatedStatuses(s) {
    return s.gated_statuses || [];
  }
</script>

{#snippet headerActions()}
  <Button
    variant="primary"
    icon={Plus}
    onclick={startCreating}
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString('approvalSets', 'add') }}
    dataTestid="approval-sets-add"
  >
    {t('approvalSets.add')}
  </Button>
{/snippet}

<PageHeader
  icon={IconRubberStamp}
  title={t('approvalSets.title')}
  subtitle={t('approvalSets.subtitle')}
  actions={headerActions}
/>

<div class="mb-6">
  <div class="relative max-w-md">
    <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-icon-subtle);" />
    <input
      type="text"
      placeholder={t('approvalSets.searchPlaceholder')}
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
{:else if filtered.length === 0 && searchQuery.trim() === ''}
  <Card rounded="xl" shadow padding="generous">
    <EmptyState
      icon={IconRubberStamp}
      title={t('approvalSets.empty')}
      description={t('approvalSets.emptyDesc')}
    >
      {#snippet action()}
        <Button variant="primary" icon={Plus} onclick={startCreating}>
          {t('approvalSets.createFirst')}
        </Button>
      {/snippet}
    </EmptyState>
  </Card>
{:else if filtered.length === 0}
  <Card rounded="xl" shadow padding="generous">
    <EmptyState
      icon={Search}
      title={t('search.noSearchResults')}
      description={t('approvalSets.noMatchingResults')}
    />
  </Card>
{:else}
  <div class="space-y-3" data-testid="approval-sets-list">
    {#each filtered as s (s.id)}
      <Card rounded="xl" shadow padding="spacious">
        <div class="flex items-center justify-between">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-3 mb-2">
              <h3 class="text-lg font-medium" style="color: var(--ds-text);">{s.name}</h3>
            </div>

            {#if s.description}
              <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">{s.description}</p>
            {/if}

            <div class="flex items-center gap-4 text-sm">
              <div class="flex items-center gap-1.5">
                <span style="color: var(--ds-text-subtle);">{t('approvalSets.workflow')}:</span>
                <span class="font-medium" style="color: var(--ds-text);">
                  {s.workflow_name || t('common.none')}
                </span>
              </div>
              {#if getGatedStatuses(s).length > 0}
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span style="color: var(--ds-text-subtle);">{t('approvalSets.statuses')}:</span>
                  {#each getGatedStatuses(s) as gs (gs.status_id)}
                    <Lozenge customBg={gs.category_color || '#3b82f6'} text={gs.status_name} />
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
              onclick={() => startEditing(s)}
              dataTestid="approval-set-edit-{s.id}"
            >
              {t('common.edit')}
            </Button>
            <Button
              variant="default"
              size="small"
              icon={Trash2}
              onclick={() => deleteApprovalSet(s)}
              dataTestid="approval-set-delete-{s.id}"
            >
              {t('common.delete')}
            </Button>
          </div>
        </div>
      </Card>
    {/each}
  </div>
{/if}
