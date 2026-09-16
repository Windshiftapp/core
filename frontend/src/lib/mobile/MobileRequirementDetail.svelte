<script>
  import { ChevronDown, ExternalLink, Loader } from '@lucide/svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { authStore } from '../stores';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import { successToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { workspacesStore } from '../stores';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import { renderMarkdown } from '../utils/render-markdown.js';
  import SafeMarkdown from '../components/SafeMarkdown.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import {
    requirementStatusLozenge,
    requirementStatusOptions,
  } from '../features/requirements/requirementStatuses.js';
  import { requirementTypeOptions } from '../features/requirements/requirementTypes.js';
  import {
    canEditRequirement,
    formatRequirementOwnerLabel,
  } from '../features/requirements/requirementFormHelpers.js';
  import MobileHeader from './MobileHeader.svelte';
  import MobileOptionSheet from './MobileOptionSheet.svelte';

  let { workspaceId, requirementNumber } = $props();

  // Requirement detail: editable metadata mirrors desktop RequirementsView.
  // Traceability editing stays on desktop in v1.
  let detail = $state(null);
  let page = $state(null);
  let assignableUsers = $state([]);
  let loading = $state(true);
  let errored = $state(false);
  let savingMeta = $state(false);
  let metaError = $state('');
  let permissionsReady = $state(false);
  let typeSheetOpen = $state(false);
  let statusSheetOpen = $state(false);
  let ownerSheetOpen = $state(false);
  let loadToken = 0;

  const workspaceName = $derived.by(() => {
    const store = $workspacesStore;
    const regular = store?.regularWorkspaces?.find((ws) => ws.id === workspaceId);
    if (regular) return regular.name;
    if (store?.personalWorkspace?.id === workspaceId) return store.personalWorkspace.name ?? '';
    return '';
  });

  const canEdit = $derived(
    permissionsReady && canEditRequirement(workspacePermissions, workspaceId),
  );
  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));
  const contentHtml = $derived(page ? renderMarkdown(page.content) : '');

  const ownerLabel = $derived.by(() => {
    if (!detail?.owner_id) return t('requirements.mobile.ownerUnset');
    const user = assignableUsers.find((u) => u.id === detail.owner_id);
    if (!user) return `#${detail.owner_id}`;
    return formatRequirementOwnerLabel(user) || `#${detail.owner_id}`;
  });

  $effect(() => {
    const userId = $authStore?.currentUser?.id;
    if (!userId) return;
    void workspacePermissions.loadPermissions(userId).finally(() => {
      permissionsReady = true;
    });
  });

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m/requirements');
  }

  function openInPages() {
    if (!detail?.page_id) return;
    navigate(`/m/pages/${workspaceId}/${detail.page_id}`);
  }

  async function copyKey() {
    if (!detail?.key) return;
    try {
      await navigator.clipboard.writeText(detail.key);
      successToast(t('requirements.keyCopied'));
    } catch {
      // ignore clipboard errors
    }
  }

  async function patchDetail(patch) {
    if (!detail || !canEdit) return;
    savingMeta = true;
    metaError = '';
    try {
      detail = await api.requirements.update(workspaceId, detail.requirement_number, patch);
    } catch (err) {
      metaError = err?.message || t('requirements.updateError');
    } finally {
      savingMeta = false;
    }
  }

  async function onTypeSelect(option) {
    typeSheetOpen = false;
    if (!detail || option.value === detail.requirement_type) return;
    await patchDetail({ requirement_type: option.value });
  }

  async function onStatusSelect(option) {
    statusSheetOpen = false;
    if (!detail || option.value === detail.status) return;
    await patchDetail({ status: option.value });
  }

  async function onOwnerSelect(user) {
    ownerSheetOpen = false;
    const ownerId = user?.id ?? null;
    if (!detail || ownerId === detail.owner_id) return;
    await patchDetail({ owner_id: ownerId });
  }

  async function onOwnerClear() {
    ownerSheetOpen = false;
    if (!detail || detail.owner_id == null) return;
    await patchDetail({ owner_id: null });
  }

  async function load(token) {
    loading = true;
    errored = false;
    metaError = '';
    try {
      const [detailRes, usersRes] = await Promise.allSettled([
        api.requirements.get(workspaceId, requirementNumber),
        api.getAssignableUsers(workspaceId),
      ]);
      if (token !== loadToken) return;
      if (detailRes.status === 'rejected') throw detailRes.reason;
      detail = detailRes.value;
      assignableUsers = usersRes.status === 'fulfilled' ? (usersRes.value ?? []) : [];

      const pageRes = await api.pages.getPage(workspaceId, detail.page_id);
      if (token !== loadToken) return;
      page = pageRes;
    } catch (err) {
      console.error('Failed to load requirement:', err);
      if (token === loadToken) errored = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  $effect(() => {
    const ws = workspaceId;
    const num = requirementNumber;
    if (ws == null || num == null) return;
    const token = ++loadToken;
    detail = null;
    page = null;
    assignableUsers = [];
    load(token);
  });
</script>

<MobileHeader title={detail?.page_title ?? detail?.key ?? t('requirements.navTitle')} onback={back} />

{#if loading}
  <div class="center" data-testid="mobile-requirement-loading"><Loader class="spin" size={22} /></div>
{:else if errored || !detail}
  <div class="msg" data-testid="mobile-requirement-error">
    <p>{t('requirements.mobile.detailLoadError')}</p>
    <button class="retry" onclick={() => load(++loadToken)} disabled={loading} type="button">
      {t('common.retry')}
    </button>
  </div>
{:else}
  <div class="detail" data-testid="mobile-requirement-detail">
    <button class="key-btn" onclick={copyKey} type="button" title={t('requirements.copyKey')}>
      <Lozenge color="blue">{detail.key}</Lozenge>
    </button>

    {#if workspaceName || detail.updated_at}
      <p class="context" data-testid="mobile-requirement-context">
        {#if workspaceName}{workspaceName}{/if}
        {#if workspaceName && detail.updated_at} · {/if}
        {#if detail.updated_at}{formatRelativeCompact(new Date(detail.updated_at))}{/if}
      </p>
    {/if}

    <div class="fields" data-testid="mobile-requirement-meta">
      {#if canEdit}
        <button
          class="field"
          type="button"
          onclick={() => (typeSheetOpen = true)}
          disabled={savingMeta}
          data-testid="mobile-requirement-type"
        >
          <span class="field-label">{t('requirements.fieldType')}</span>
          <span class="field-value">
            {t(`requirements.type.${detail.requirement_type}`)}
            <ChevronDown size={16} class="chev" />
          </span>
        </button>
        <button
          class="field"
          type="button"
          onclick={() => (statusSheetOpen = true)}
          disabled={savingMeta}
          data-testid="mobile-requirement-status"
        >
          <span class="field-label">{t('requirements.fieldStatus')}</span>
          <span class="field-value">
            <Lozenge color={requirementStatusLozenge(detail.status)}>
              {t(`requirements.status.${detail.status}`)}
            </Lozenge>
            <ChevronDown size={16} class="chev" />
          </span>
        </button>
        <button
          class="field"
          type="button"
          onclick={() => (ownerSheetOpen = true)}
          disabled={savingMeta}
          data-testid="mobile-requirement-owner"
        >
          <span class="field-label">{t('requirements.fieldOwner')}</span>
          <span class="field-value">
            <span class="owner-name">{ownerLabel}</span>
            <ChevronDown size={16} class="chev" />
          </span>
        </button>
      {:else}
        <div class="field field-readonly" data-testid="mobile-requirement-type">
          <span class="field-label">{t('requirements.fieldType')}</span>
          <span class="field-value">{t(`requirements.type.${detail.requirement_type}`)}</span>
        </div>
        <div class="field field-readonly" data-testid="mobile-requirement-status">
          <span class="field-label">{t('requirements.fieldStatus')}</span>
          <span class="field-value">
            <Lozenge color={requirementStatusLozenge(detail.status)}>
              {t(`requirements.status.${detail.status}`)}
            </Lozenge>
          </span>
        </div>
        <div class="field field-readonly" data-testid="mobile-requirement-owner">
          <span class="field-label">{t('requirements.fieldOwner')}</span>
          <span class="field-value owner-name">{ownerLabel}</span>
        </div>
      {/if}
    </div>

    {#if savingMeta}
      <p class="saving-meta" data-testid="mobile-requirement-saving">
        <Loader class="spin" size={14} />
        {t('common.saving')}
      </p>
    {/if}

    {#if metaError}
      <p class="meta-error" data-testid="mobile-requirement-meta-error">{metaError}</p>
    {/if}

    <button class="pages-link" onclick={openInPages} data-testid="mobile-requirement-open-pages" type="button">
      <ExternalLink size={16} />
      {t('requirements.openInPages')}
    </button>

    <section class="traceability" data-testid="mobile-requirement-traceability">
      <h2 class="section-title">{t('requirements.mobile.traceabilitySummary')}</h2>
      <div class="trace-stats">
        <span>{t('requirements.traceability.linkedItems')}: {detail.linked_item_count ?? 0}</span>
        <span>{t('requirements.traceability.linkedTests')}: {detail.linked_test_count ?? 0}</span>
        {#if (detail.linked_test_count ?? 0) > 0}
          <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
        {:else}
          <Lozenge color="red">{t('requirements.traceability.uncovered')}</Lozenge>
        {/if}
      </div>
    </section>

    {#if page?.content}
      <SafeMarkdown html={contentHtml} testid="mobile-requirement-content" />
    {:else}
      <p class="empty">{t('requirements.mobile.emptyContent')}</p>
    {/if}
  </div>
{/if}

<MobileOptionSheet
  bind:isOpen={typeSheetOpen}
  title={t('requirements.fieldType')}
  options={typeOptions}
  getValue={(option) => option.value}
  getLabel={(option) => option.label}
  selectedValue={detail?.requirement_type ?? null}
  searchable={false}
  onSelect={onTypeSelect}
  dataTestid="mobile-requirement-type-sheet"
/>

<MobileOptionSheet
  bind:isOpen={statusSheetOpen}
  title={t('requirements.fieldStatus')}
  options={statusOptions}
  getValue={(option) => option.value}
  getLabel={(option) => option.label}
  selectedValue={detail?.status ?? null}
  searchable={false}
  onSelect={onStatusSelect}
  dataTestid="mobile-requirement-status-sheet"
/>

<MobileOptionSheet
  bind:isOpen={ownerSheetOpen}
  title={t('requirements.fieldOwner')}
  options={assignableUsers}
  getValue={(user) => user.id}
  getLabel={(user) => formatRequirementOwnerLabel(user) || `#${user.id}`}
  selectedValue={detail?.owner_id ?? null}
  allowClear
  clearLabel={t('requirements.mobile.ownerUnset')}
  emptyText={t('requirements.mobile.ownerUnset')}
  onSelect={onOwnerSelect}
  onClear={onOwnerClear}
  dataTestid="mobile-requirement-owner-sheet"
/>

<style>
  .center { display: flex; justify-content: center; padding: 3rem; color: var(--ds-text-subtle); }
  :global(.spin) { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .msg { padding: 3rem 1.25rem; text-align: center; color: var(--ds-text-subtle); }
  .msg p { margin: 0; }
  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }
  .retry:disabled { opacity: 0.6; }

  .detail { padding: 0.875rem 0.875rem 2rem; }

  .key-btn {
    border: none;
    background: transparent;
    padding: 0;
    margin-bottom: 0.5rem;
    cursor: pointer;
  }

  .context {
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
    margin: 0 0 0.75rem;
    line-height: 1.45;
  }

  .fields {
    margin-bottom: 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    overflow: hidden;
  }

  .field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    width: 100%;
    min-height: 48px;
    padding: 0.5rem 0.85rem;
    cursor: pointer;
    border: none;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
  }

  .field:not(:last-child) { border-bottom: 1px solid var(--ds-border); }
  .field:active { background-color: var(--ds-background-neutral-hovered); }
  .field:disabled { opacity: 0.6; cursor: default; }

  .field-readonly {
    cursor: default;
  }

  .field-label { font-size: 0.8125rem; color: var(--ds-text-subtle); }
  .field-value {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
    color: var(--ds-text);
    font-size: 0.875rem;
  }

  .field-value :global(.chev) {
    color: var(--ds-icon-subtle, var(--ds-text-subtle));
    flex-shrink: 0;
  }

  .owner-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 11rem;
  }

  .saving-meta {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0 0 0.5rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }

  .meta-error {
    margin: 0 0 0.75rem;
    font-size: 0.875rem;
    color: var(--ds-text-danger);
  }

  .pages-link {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin-bottom: 1rem;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text-link, var(--ds-interactive));
    font-size: 0.875rem;
    cursor: pointer;
  }

  .traceability {
    margin-bottom: 1.25rem;
    padding: 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background: var(--ds-surface-raised);
  }
  .section-title {
    font-size: 0.875rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
    margin: 0 0 0.5rem;
  }
  .trace-stats {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }

  .empty { color: var(--ds-text-subtle); font-size: 0.875rem; }
</style>
