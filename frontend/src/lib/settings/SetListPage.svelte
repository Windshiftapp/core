<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { navigate } from '../router.js';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Search from '@lucide/svelte/icons/search';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Panel from '../components/Panel.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  /**
   * List page shared by the approval-set and condition-set managers: header,
   * search, loading/empty/result states, and per-row edit/delete actions.
   *
   * @type {{
   *   config: {
   *     api: { getAll: () => Promise<any>, delete: (id: number) => Promise<any> },
   *     icon: any,
   *     titleKey: string,
   *     subtitleKey: string,
   *     addKey: string,
   *     searchPlaceholderKey: string,
   *     emptyKey: string,
   *     emptyDescKey: string,
   *     createFirstKey: string,
   *     noMatchingResultsKey: string,
   *     hotkeyScope: string,
   *     newRoute: string,
   *     detailRoute: (id: number) => string,
   *     addTestid: string,
   *     searchTestid: string,
   *     listTestid: string,
   *     rowTestidPrefix: string,
   *     errorLabel: string,
   *   },
   *   meta?: import('svelte').Snippet<[any]>,
   * }}
   */
  let { config, meta } = $props();

  let items = $state([]);
  let loading = $state(true);
  let searchQuery = $state('');

  const filtered = $derived.by(() => {
    const query = searchQuery.trim().toLowerCase();
    if (query === '') return items;
    return items.filter(
      (item) =>
        item.name?.toLowerCase().includes(query) ||
        item.description?.toLowerCase().includes(query) ||
        item.workflow_name?.toLowerCase().includes(query)
    );
  });

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      loading = true;
      items = (await config.api.getAll()) || [];
    } catch (error) {
      console.error(`Failed to load ${config.errorLabel}:`, error);
      items = [];
    } finally {
      loading = false;
    }
  }

  function startCreating() {
    navigate(config.newRoute);
  }

  function startEditing(item) {
    navigate(config.detailRoute(item.id));
  }

  async function deleteItem(item) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteItem', { name: item.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await config.api.delete(item.id);
      items = items.filter((candidate) => candidate.id !== item.id);
    } catch (error) {
      console.error(`Failed to delete ${config.errorLabel}:`, error);
      errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
    }
  }
</script>

{#snippet headerActions()}
  <Button
    variant="primary"
    icon={Plus}
    onclick={startCreating}
    keyboardHint="A"
    hotkeyConfig={{ key: toHotkeyString(config.hotkeyScope, 'add') }}
    dataTestid={config.addTestid}
  >
    {t(config.addKey)}
  </Button>
{/snippet}

<PageHeader
  icon={config.icon}
  title={t(config.titleKey)}
  subtitle={t(config.subtitleKey)}
  actions={headerActions}
/>

<div class="mb-6">
  <SearchInput
    bind:value={searchQuery}
    placeholder={t(config.searchPlaceholderKey)}
    class="max-w-md"
    dataTestid={config.searchTestid}
  />
</div>

{#if loading}
  <Panel padding="spacious" class="text-center">
    <div class="animate-pulse" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
  </Panel>
{:else if filtered.length === 0 && searchQuery.trim() === ''}
  <Panel padding="spacious">
    <EmptyState
      icon={config.icon}
      title={t(config.emptyKey)}
      description={t(config.emptyDescKey)}
    >
      {#snippet action()}
        <!-- shortcut-guard-exempt: duplicate of the header Add button, which owns the A shortcut -->
        <Button variant="primary" icon={Plus} onclick={startCreating}>
          {t(config.createFirstKey)}
        </Button>
      {/snippet}
    </EmptyState>
  </Panel>
{:else if filtered.length === 0}
  <Panel padding="spacious">
    <EmptyState
      icon={Search}
      title={t('search.noSearchResults')}
      description={t(config.noMatchingResultsKey)}
    />
  </Panel>
{:else}
  <div class="space-y-3" data-testid={config.listTestid}>
    {#each filtered as item (item.id)}
      <Panel padding="spacious" hoverable>
        <div class="flex items-center justify-between" data-testid={`${config.rowTestidPrefix}-${item.id}`}>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-3 mb-2">
              <h3 class="text-lg font-medium" style="color: var(--ds-text);">{item.name}</h3>
            </div>

            {#if item.description}
              <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">{item.description}</p>
            {/if}

            {@render meta?.(item)}
          </div>

          <div class="flex items-center gap-2 ml-4 flex-shrink-0">
            <Button
              variant="default"
              size="small"
              icon={Edit}
              onclick={() => startEditing(item)}
              dataTestid={`${config.rowTestidPrefix}-edit-${item.id}`}
            >
              {t('common.edit')}
            </Button>
            <Button
              variant="danger-ghost"
              size="small"
              icon={Trash2}
              onclick={() => deleteItem(item)}
              dataTestid={`${config.rowTestidPrefix}-delete-${item.id}`}
            >
              {t('common.delete')}
            </Button>
          </div>
        </div>
      </Panel>
    {/each}
  </div>
{/if}
