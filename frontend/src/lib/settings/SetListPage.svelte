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
  import DataTable from '../components/DataTable.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Panel from '../components/Panel.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  /**
   * List page shared by the approval-set and condition-set managers: header,
   * search, loading/empty/result states, and a column table whose rows expose
   * edit/delete through an actions menu.
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
   *   columns?: Array<any>,
   *   cellSnippets?: Record<string, any>,
   * }}
   */
  let { config, columns = [], cellSnippets = {} } = $props();

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

  // The actions column is owned by this scaffold so every list gets the same
  // menu trigger; callers only describe their data columns.
  const tableColumns = $derived([
    ...columns,
    { key: 'actions', label: t('common.actions'), align: 'text-right' }
  ]);

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

  function buildRowActions(item) {
    return [
      {
        id: 'edit',
        icon: Edit,
        title: t('common.edit'),
        testid: `${config.rowTestidPrefix}-edit-${item.id}`,
        onClick: () => startEditing(item),
      },
      {
        id: 'delete',
        icon: Trash2,
        title: t('common.delete'),
        testid: `${config.rowTestidPrefix}-delete-${item.id}`,
        color: 'var(--ds-text-danger)',
        onClick: () => deleteItem(item),
      },
    ];
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
  <div data-testid={config.listTestid}>
    <DataTable
      columns={tableColumns}
      data={filtered}
      keyField="id"
      actionItems={buildRowActions}
      actionTriggerTestid={(item) => `${config.rowTestidPrefix}-actions-${item.id}`}
      rowAttrs={(item) => ({ 'data-testid': `${config.rowTestidPrefix}-row-${item.id}` })}
      {...cellSnippets}
    />
  </div>
{/if}
