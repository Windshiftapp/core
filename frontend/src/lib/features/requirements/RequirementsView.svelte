<script>
  import {
    IconPlus,
    IconChevronLeft,
    IconExternalLink,
    IconHistory,
    IconCopy,
    IconChevronDown,
    IconUser,
  } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import PagesView from '../pages/PagesView.svelte';
  import RequirementHistoryDrawer from './RequirementHistoryDrawer.svelte';
  import RequirementTraceabilityPanel from './RequirementTraceabilityPanel.svelte';
  import RequirementListFilters from './RequirementListFilters.svelte';
  import { requirementTypeOptions } from './requirementTypes.js';
  import { requirementStatusOptions, requirementStatusLozenge } from './requirementStatuses.js';
  import { canEditRequirement } from './requirementFormHelpers.js';
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
  let filterItemLinks = $state('');
  let filterTestLinks = $state('');
  let selectedLabelIds = $state(new Set());
  let pageIndex = $state(0);
  let totalItems = $state(0);
  let assignableUsers = $state([]);
  let assignableUsersLoading = $state(true);

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

  const canEdit = $derived(canEditRequirement(workspacePermissions, workspaceId));

  const columns = $derived([
    { key: 'key', label: t('requirements.columnKey'), sortable: true, slot: 'key' },
    { key: 'page_title', label: t('requirements.columnTitle'), sortable: true },
    { key: 'requirement_type', label: t('requirements.columnType'), slot: 'requirement_type' },
    { key: 'status', label: t('requirements.columnStatus'), slot: 'status' },
    { key: 'owner_id', label: t('requirements.columnOwner'), slot: 'owner_id' },
    { key: 'linked_test_count', label: t('requirements.columnTests'), slot: 'linked_test_count' },
    { key: 'updated_at', label: t('requirements.columnUpdated'), sortable: true, slot: 'updated_at' },
  ]);

  const pageStart = $derived(totalItems === 0 ? 0 : pageIndex * PAGE_SIZE + 1);
  const pageEnd = $derived(Math.min(totalItems, (pageIndex + 1) * PAGE_SIZE));
  const hasNextPage = $derived(pageEnd < totalItems);

  const userLabelById = $derived(
    new Map(
      assignableUsers.map((user) => [
        user.id,
        [user.first_name, user.last_name].filter(Boolean).join(' ') || user.username || user.email,
      ])
    )
  );

  let metaType = $derived(detail?.requirement_type ?? '');
  let metaStatus = $derived(detail?.status ?? '');
  const owner = $derived(assignableUsers.find((user) => user.id === detail?.owner_id));

  $effect(() => {
    assignableUsersLoading = true;
    void api.getAssignableUsers(workspaceId)
      .then((users) => { assignableUsers = users || []; })
      .finally(() => { assignableUsersLoading = false; });
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
        has_item_links: filterItemLinks || undefined,
        has_test_links: filterTestLinks || undefined,
        label_ids: selectedLabelIds.size > 0 ? [...selectedLabelIds].join(',') : undefined,
        limit: PAGE_SIZE,
        offset: pageIndex * PAGE_SIZE,
      });
      rows = result.items ?? [];
      totalItems = result.pagination?.total_items ?? rows.length;
    } catch (err) {
      error = err?.message || t('requirements.loadError');
      rows = [];
      totalItems = 0;
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

  function openCreate() {
    navigate(`/workspaces/${workspaceId}/requirements/new`);
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
    detailError = '';
    try {
      detail = await api.requirements.update(workspaceId, detail.requirement_number, patch);
    } catch (err) {
      detailError = err?.message || t('requirements.updateError');
      metaType = detail.requirement_type;
      metaStatus = detail.status;
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

  function ownerLabel(ownerId) {
    if (!ownerId) return '—';
    return userLabelById.get(ownerId) || `#${ownerId}`;
  }

</script>

{#if requirementNumber}
  <div class="requirements-detail flex h-full min-h-0 flex-col">
    <header class="requirements-detail__navigation">
      <div class="requirements-detail__breadcrumb">
        <Button variant="ghost" size="sm" onclick={() => navigate(`/workspaces/${workspaceId}/requirements`)}>
          <IconChevronLeft size={16} aria-hidden="true" />
          {t('requirements.navTitle')}
        </Button>
        {#if detail}
          <span class="requirements-detail__separator" aria-hidden="true">/</span>
          <button
            type="button"
            class="requirements-key"
            onclick={() => copyKey(detail.key)}
            title={t('requirements.copyKey')}
            aria-label={`${t('requirements.copyKey')}: ${detail.key}`}
          >
            {detail.key}
            <IconCopy size={14} aria-hidden="true" />
          </button>
        {/if}
      </div>
      <div class="requirements-detail__actions">
        <Button variant="ghost" size="sm" onclick={() => (historyOpen = true)} disabled={!detail}>
          <IconHistory size={16} aria-hidden="true" />
          {t('requirements.historyTitle')}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onclick={() => navigate(`/workspaces/${workspaceId}/pages/${detail?.page_id}`)}
          disabled={!detail}
        >
          <IconExternalLink size={16} aria-hidden="true" />
          {t('requirements.openInPages')}
        </Button>
      </div>
    </header>
    {#if detailError}
      <p class="requirements-detail__error" role="alert">{detailError}</p>
    {/if}
    <div class="requirements-detail__editor min-h-0 flex-1">
      {#if detail?.page_id}
        <PagesView {workspaceId} pageId={detail.page_id} requirementContext>
          {#snippet documentMeta({ canEdit: canEditPage })}
            {@const disabled = !canEdit || !canEditPage || savingMeta}
            <div class="requirements-detail__properties" aria-busy={savingMeta}>
              <div class="meta-field">
                <ItemPicker
                  bind:value={metaType}
                  items={requirementTypeOptions(t)}
                  ariaLabel={`${t('requirements.fieldType')}: ${t(`requirements.type.${metaType}`)}`}
                  config={{ getValue: (option) => option.value }}
                  allowClear={false}
                  {disabled}
                  onSelect={onTypeChange}
                >
                  <div class="meta-value" title={t('requirements.fieldType')}>
                    <span class="meta-value__text">{t(`requirements.type.${metaType}`)}</span>
                    {#if !disabled}<IconChevronDown size={12} aria-hidden="true" />{/if}
                  </div>
                </ItemPicker>
              </div>
              <div class="meta-field">
                <ItemPicker
                  bind:value={metaStatus}
                  items={requirementStatusOptions(t)}
                  ariaLabel={`${t('requirements.fieldStatus')}: ${t(`requirements.status.${metaStatus}`)}`}
                  config={{ getValue: (option) => option.value }}
                  allowClear={false}
                  {disabled}
                  onSelect={onStatusChange}
                >
                  <div class="meta-value" title={t('requirements.fieldStatus')}>
                    <Lozenge color={requirementStatusLozenge(metaStatus)}>
                      {t(`requirements.status.${metaStatus}`)}
                    </Lozenge>
                    {#if !disabled}<IconChevronDown size={12} aria-hidden="true" />{/if}
                  </div>
                </ItemPicker>
              </div>
              <div class="meta-field">
                <UserPicker
                  value={detail.owner_id}
                  users={assignableUsers}
                  loading={assignableUsersLoading}
                  placeholder={t('pickers.unassigned')}
                  ariaLabel={`${t('requirements.fieldOwner')}: ${detail.owner_id ? ownerLabel(detail.owner_id) : t('pickers.unassigned')}`}
                  showUnassigned
                  {disabled}
                  onSelect={onOwnerChange}
                >
                  <div class="meta-value" title={t('requirements.fieldOwner')}>
                    <span class="meta-value__avatar" aria-hidden="true">
                      {#if detail.owner_id}
                        <Avatar src={owner?.avatar_url} name={ownerLabel(detail.owner_id)} size="2xs" variant="neutral" />
                      {:else}
                        <IconUser size={16} />
                      {/if}
                    </span>
                    <span class="meta-value__text">{detail.owner_id ? ownerLabel(detail.owner_id) : t('pickers.unassigned')}</span>
                    {#if !disabled}<IconChevronDown size={12} aria-hidden="true" />{/if}
                  </div>
                </UserPicker>
              </div>
            </div>
          {/snippet}
          {#snippet documentFooter({ canEdit: canEditPage })}
            {#key detail.page_id}
              <RequirementTraceabilityPanel
                {workspaceId}
                pageId={detail.page_id}
                canEdit={canEdit && canEditPage}
              />
            {/key}
          {/snippet}
        </PagesView>
      {:else if detailLoading}
        <div class="flex justify-center py-8"><Spinner /></div>
      {/if}
    </div>
    <RequirementHistoryDrawer
      {workspaceId}
      requirementNumber={detail?.requirement_number}
      bind:open={historyOpen}
    />
  </div>
{:else}
  <div class="requirements-list flex h-full min-h-0 flex-col min-w-0 p-4">
    <PageHeader title={t('requirements.navTitle')}>
      {#snippet actions()}
        {#if canCreate}
          <Button onclick={openCreate}>
            <IconPlus size={16} />
            {t('requirements.create')}
          </Button>
        {/if}
      {/snippet}
    </PageHeader>

    <RequirementListFilters
      {workspaceId}
      bind:searchQuery
      bind:filterType
      bind:filterStatus
      bind:filterOwnerId
      bind:filterItemLinks
      bind:filterTestLinks
      bind:selectedLabelIds
      onchange={applyFilters}
    />

    {#if error}
      <p class="mb-3 text-sm text-[var(--ds-text-danger)]">{error}</p>
    {/if}

    <div class="requirements-list__table min-h-0 min-w-0 flex-1">
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
      {#snippet linked_test_count(item)}
        {#if (item.linked_test_count ?? 0) > 0}
          <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
        {:else}
          <Lozenge color="red">{t('requirements.traceability.uncovered')}</Lozenge>
        {/if}
      {/snippet}
      {#snippet updated_at(item)}
        {formatDateShort(item.updated_at)}
      {/snippet}
    </DataTable>
    </div>

    <div class="mt-4 flex items-center justify-end gap-2">
      <Button variant="secondary" size="sm" onclick={prevPage} disabled={pageIndex === 0 || loading}>
        {t('common.previous')}
      </Button>
      <span class="text-sm text-[var(--ds-text-subtle)]">
        {t('requirements.pageRange', { start: pageStart, end: pageEnd, total: totalItems })}
      </span>
      <Button variant="secondary" size="sm" onclick={nextPage} disabled={!hasNextPage || loading}>
        {t('common.next')}
      </Button>
    </div>

  </div>
{/if}

<style>
  .requirements-detail {
    min-width: 0;
  }
  .requirements-detail__navigation,
  .requirements-detail__breadcrumb,
  .requirements-detail__actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .requirements-detail__navigation {
    justify-content: space-between;
    flex-wrap: wrap;
    flex-shrink: 0;
    padding: 0.625rem 1rem;
    border-bottom: 1px solid var(--ds-border);
  }
  .requirements-detail__separator {
    color: var(--ds-text-subtlest);
  }
  .requirements-key {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 2rem;
    border: none;
    border-radius: 0.25rem;
    background: transparent;
    padding: 0.25rem 0.5rem;
    color: var(--ds-text-link);
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
  }
  .requirements-key:hover {
    background: var(--ds-background-neutral-hovered);
  }
  .requirements-key:focus-visible {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 2px;
  }
  .requirements-detail__error {
    margin: 0;
    padding: 0.75rem 1.5rem;
    color: var(--ds-text-danger);
    font-size: 0.875rem;
  }
  .requirements-detail__properties {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.25rem 0.75rem;
  }
  .meta-field {
    min-width: 0;
    max-width: 100%;
  }
  .meta-value {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    min-height: 2rem;
    padding: 0.25rem 0.375rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }
  .meta-value__text {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .meta-value__avatar {
    display: flex;
    flex-shrink: 0;
  }
  .meta-value :global(svg) {
    flex-shrink: 0;
  }
  .meta-field :global([role='combobox']) {
    border-radius: 0.25rem;
  }
  .meta-field :global([role='combobox'][aria-disabled='false']) {
    cursor: pointer;
  }
  .meta-field :global([role='combobox'][aria-disabled='false']:hover),
  .meta-field :global([role='combobox'][aria-expanded='true']) {
    background: var(--ds-background-neutral-hovered);
  }
  .meta-field :global([role='combobox']:focus-visible) {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 2px;
  }
</style>
