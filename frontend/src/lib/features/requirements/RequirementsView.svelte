<script>
  import { IconPlus, IconChevronLeft, IconExternalLink, IconHistory } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import SearchInput from '../../components/SearchInput.svelte';
  import Select from '../../components/Select.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import PagesView from '../pages/PagesView.svelte';
  import RequirementCreateDialog from './RequirementCreateDialog.svelte';
  import RequirementHistoryDrawer from './RequirementHistoryDrawer.svelte';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions, requirementStatusLozenge } from './requirementStatuses.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import { successToast } from '../../stores/toasts.svelte.js';

  let { workspaceId, requirementNumber = null } = $props();

  const PAGE_SIZE = 50;

  let rows = $state([]);
  let loading = $state(false);
  let error = $state('');
  let searchQuery = $state('');
  let filterType = $state('');
  let filterStatus = $state('');
  let filterOwnerId = $state(null);
  let pageIndex = $state(0);
  let hasNextPage = $state(false);
  let showCreateDialog = $state(false);
  let assignableUsers = $state([]);

  let detail = $state(null);
  let detailLoading = $state(false);
  let detailError = $state('');
  let historyOpen = $state(false);
  let savingMeta = $state(false);

  const canCreate = $derived(
    workspacePermissions.isSystemAdmin ||
      workspacePermissions.hasPermission(workspaceId, 'page.create') ||
      workspacePermissions.hasPermission(workspaceId, 'page.admin') ||
      workspacePermissions.hasPermission(workspaceId, 'workspace.admin')
  );

  const canEdit = $derived(
    workspacePermissions.isSystemAdmin ||
      workspacePermissions.hasPermission(workspaceId, 'page.edit') ||
      workspacePermissions.hasPermission(workspaceId, 'page.admin') ||
      workspacePermissions.hasPermission(workspaceId, 'workspace.admin')
  );

  const typeOptions = $derived([
    { value: '', label: t('requirements.filters.allTypes') },
    ...requirementTypeOptions(t),
  ]);
  const statusOptions = $derived([
    { value: '', label: t('requirements.filters.allStatuses') },
    ...requirementStatusOptions(t),
  ]);

  const columns = $derived([
    { key: 'key', label: t('requirements.columnKey'), sortable: true, slot: 'key' },
    { key: 'page_title', label: t('requirements.columnTitle'), sortable: true },
    { key: 'requirement_type', label: t('requirements.columnType'), slot: 'requirement_type' },
    { key: 'status', label: t('requirements.columnStatus'), slot: 'status' },
    { key: 'owner_id', label: t('requirements.columnOwner'), slot: 'owner_id' },
    { key: 'updated_at', label: t('requirements.columnUpdated'), sortable: true, slot: 'updated_at' },
  ]);

  const userLabelById = $derived(
    new Map(
      assignableUsers.map((user) => [
        user.id,
        [user.first_name, user.last_name].filter(Boolean).join(' ') || user.username || user.email,
      ])
    )
  );

  let metaType = $state('');
  let metaStatus = $state('');

  $effect(() => {
    if (detail) {
      metaType = detail.requirement_type;
      metaStatus = detail.status;
    }
  });

  $effect(() => {
    void api.getAssignableUsers(workspaceId).then((users) => {
      assignableUsers = users || [];
    });
  });

  $effect(() => {
    if (requirementNumber) {
      void loadDetail();
      return;
    }
    void loadList();
  });

  async function loadList() {
    loading = true;
    error = '';
    try {
      const result = await api.requirements.list(workspaceId, {
        q: searchQuery.trim() || undefined,
        requirement_type: filterType || undefined,
        status: filterStatus || undefined,
        owner_id: filterOwnerId || undefined,
        limit: PAGE_SIZE,
        offset: pageIndex * PAGE_SIZE,
      });
      rows = Array.isArray(result) ? result : [];
      hasNextPage = rows.length === PAGE_SIZE;
    } catch (err) {
      error = err?.message || t('requirements.loadError');
      rows = [];
    } finally {
      loading = false;
    }
  }

  async function loadDetail() {
    if (!requirementNumber) return;
    detailLoading = true;
    detailError = '';
    try {
      detail = await api.requirements.get(workspaceId, requirementNumber);
    } catch (err) {
      detailError = err?.message || t('requirements.loadError');
      detail = null;
    } finally {
      detailLoading = false;
    }
  }

  function openRow(row) {
    navigate(`/workspaces/${workspaceId}/requirements/${row.requirement_number}`);
  }

  function handleCreated(created) {
    navigate(`/workspaces/${workspaceId}/requirements/${created.requirement_number}`);
  }

  function applyFilters() {
    pageIndex = 0;
    void loadList();
  }

  function nextPage() {
    if (!hasNextPage) return;
    pageIndex += 1;
    void loadList();
  }

  function prevPage() {
    if (pageIndex === 0) return;
    pageIndex -= 1;
    void loadList();
  }

  async function copyKey(key) {
    try {
      await navigator.clipboard.writeText(key);
      successToast(t('requirements.keyCopied'));
    } catch {
      // ignore clipboard errors
    }
  }

  async function patchDetail(patch) {
    if (!detail || !canEdit) return;
    savingMeta = true;
    try {
      detail = await api.requirements.update(workspaceId, detail.requirement_number, patch);
    } catch (err) {
      detailError = err?.message || t('requirements.updateError');
    } finally {
      savingMeta = false;
    }
  }

  function onTypeChange() {
    if (!detail || metaType === detail.requirement_type) return;
    void patchDetail({ requirement_type: metaType });
  }

  function onStatusChange() {
    if (!detail || metaStatus === detail.status) return;
    void patchDetail({ status: metaStatus });
  }

  function onOwnerChange(user) {
    const ownerId = user?.id ?? null;
    if (!detail || ownerId === detail.owner_id) return;
    void patchDetail({ owner_id: ownerId });
  }

  function onSearchKeydown(event) {
    if (event.key === 'Enter') applyFilters();
  }

  function ownerLabel(ownerId) {
    if (!ownerId) return '—';
    return userLabelById.get(ownerId) || `#${ownerId}`;
  }
</script>

{#if requirementNumber}
  <div class="requirements-detail flex h-full min-h-0 flex-col">
    <div class="requirements-detail__meta border-b px-4 py-3">
      <div class="mb-3 flex items-center gap-2">
        <Button variant="ghost" size="sm" onclick={() => navigate(`/workspaces/${workspaceId}/requirements`)}>
          <IconChevronLeft size={16} />
          {t('requirements.backToList')}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onclick={() => navigate(`/workspaces/${workspaceId}/pages/${detail?.page_id}`)}
          disabled={!detail}
        >
          <IconExternalLink size={16} />
          {t('requirements.openInPages')}
        </Button>
        <Button variant="ghost" size="sm" onclick={() => (historyOpen = true)} disabled={!detail}>
          <IconHistory size={16} />
          {t('requirements.historyTitle')}
        </Button>
      </div>
      {#if detailLoading}
        <div class="flex justify-center py-4"><Spinner /></div>
      {:else if detailError}
        <p class="text-sm text-[var(--ds-text-danger)]">{detailError}</p>
      {:else if detail}
        <div class="flex flex-wrap items-end gap-4">
          <button type="button" class="requirements-key" onclick={() => copyKey(detail.key)} title={t('requirements.copyKey')}>
            <Lozenge color="blue">{detail.key}</Lozenge>
          </button>
          <div class="meta-field">
            <span class="meta-label">{t('requirements.fieldType')}</span>
            <Select
              bind:value={metaType}
              options={requirementTypeOptions(t)}
              disabled={!canEdit || savingMeta}
              onchange={onTypeChange}
            />
          </div>
          <div class="meta-field">
            <span class="meta-label">{t('requirements.fieldStatus')}</span>
            <Select
              bind:value={metaStatus}
              options={requirementStatusOptions(t)}
              disabled={!canEdit || savingMeta}
              onchange={onStatusChange}
            />
          </div>
          <div class="meta-field meta-field--owner">
            <span class="meta-label">{t('requirements.fieldOwner')}</span>
            <UserPicker
              value={detail.owner_id}
              {workspaceId}
              disabled={!canEdit || savingMeta}
              onSelect={onOwnerChange}
            />
          </div>
        </div>
      {/if}
    </div>
    <div class="requirements-detail__editor min-h-0 flex-1">
      {#if detail?.page_id}
        <PagesView {workspaceId} pageId={detail.page_id} />
      {/if}
    </div>
    <RequirementHistoryDrawer
      {workspaceId}
      requirementNumber={detail?.requirement_number}
      bind:open={historyOpen}
    />
  </div>
{:else}
  <div class="requirements-list flex h-full min-h-0 flex-col p-4">
    <PageHeader title={t('requirements.navTitle')}>
      {#snippet actions()}
        {#if canCreate}
          <Button onclick={() => (showCreateDialog = true)}>
            <IconPlus size={16} />
            {t('requirements.create')}
          </Button>
        {/if}
      {/snippet}
    </PageHeader>

    <div class="mb-4 flex flex-wrap items-end gap-3">
      <SearchInput
        bind:value={searchQuery}
        placeholder={t('requirements.filters.search')}
        on_keydown={onSearchKeydown}
        class="min-w-[200px] flex-1"
      />
      <Select bind:value={filterType} options={typeOptions} onchange={applyFilters} />
      <Select bind:value={filterStatus} options={statusOptions} onchange={applyFilters} />
      <div class="min-w-[180px]">
        <UserPicker bind:value={filterOwnerId} {workspaceId} onSelect={applyFilters} />
      </div>
      <Button variant="secondary" onclick={applyFilters}>{t('common.apply')}</Button>
    </div>

    {#if error}
      <p class="mb-3 text-sm text-[var(--ds-text-danger)]">{error}</p>
    {/if}

    <DataTable
      {columns}
      data={rows}
      keyField="requirement_number"
      {loading}
      emptyMessage={t('requirements.emptyTitle')}
      emptyDescription={t('requirements.emptyDescription')}
      onRowClick={openRow}
    >
      {#snippet key(item)}
        <Lozenge color="blue">{item.key}</Lozenge>
      {/snippet}
      {#snippet requirement_type(item)}
        {t(`requirements.type.${item.requirement_type}`)}
      {/snippet}
      {#snippet status(item)}
        <Lozenge color={requirementStatusLozenge(item.status)}>
          {t(`requirements.status.${item.status}`)}
        </Lozenge>
      {/snippet}
      {#snippet owner_id(item)}
        {ownerLabel(item.owner_id)}
      {/snippet}
      {#snippet updated_at(item)}
        {formatDateShort(item.updated_at)}
      {/snippet}
    </DataTable>

    <div class="mt-4 flex items-center justify-end gap-2">
      <Button variant="secondary" size="sm" onclick={prevPage} disabled={pageIndex === 0 || loading}>
        {t('common.previous')}
      </Button>
      <span class="text-sm text-[var(--ds-text-subtle)]">
        {t('requirements.pageIndicator', { page: pageIndex + 1 })}
      </span>
      <Button variant="secondary" size="sm" onclick={nextPage} disabled={!hasNextPage || loading}>
        {t('common.next')}
      </Button>
    </div>

    <RequirementCreateDialog {workspaceId} bind:open={showCreateDialog} onCreated={handleCreated} />
  </div>
{/if}

<style>
  .requirements-key {
    border: none;
    background: transparent;
    padding: 0;
    cursor: pointer;
  }
  .meta-field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 160px;
  }
  .meta-field--owner {
    min-width: 200px;
  }
  .meta-label {
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }
  .requirements-detail__editor :global(.pages-shell) {
    height: 100%;
  }
</style>
