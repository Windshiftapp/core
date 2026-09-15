<script>
  import { Loader } from '@lucide/svelte';
  import { api } from '../api.js';
  import NativeSelect from '../components/NativeSelect.svelte';
  import MobileSheet from './MobileSheet.svelte';
  import MobileOptionSheet from './MobileOptionSheet.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { requirementTypeOptions } from '../features/requirements/requirementTypes.js';
  import { requirementStatusOptions } from '../features/requirements/requirementStatuses.js';
  import {
    DEFAULT_REQUIREMENT_STATUS,
    DEFAULT_REQUIREMENT_TYPE,
    applyStarterTemplate,
    formatRequirementOwnerLabel,
  } from '../features/requirements/requirementFormHelpers.js';

  let {
    workspaceId,
    pageId,
    isOpen = $bindable(false),
    onPromoted = () => {},
  } = $props();

  let requirementType = $state(DEFAULT_REQUIREMENT_TYPE);
  let status = $state(DEFAULT_REQUIREMENT_STATUS);
  let ownerId = $state(null);
  let saving = $state(false);
  let error = $state('');
  let pageContent = $state('');
  let pageLoading = $state(false);
  let insertTemplate = $state(true);
  let loadSeq = 0;

  let assignableUsers = $state([]);
  let assignableLoading = $state(false);
  let ownerSheetOpen = $state(false);

  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));
  const pageIsEmpty = $derived(!pageContent.trim());
  const ownerLabel = $derived.by(() => {
    if (!ownerId) return t('requirements.mobile.ownerUnset');
    const user = assignableUsers.find((u) => u.id === ownerId);
    return formatRequirementOwnerLabel(user) || `#${ownerId}`;
  });

  async function loadPage() {
    const seq = ++loadSeq;
    pageLoading = true;
    error = '';
    try {
      const page = await api.pages.getPage(workspaceId, pageId);
      if (seq !== loadSeq) return;
      pageContent = page?.content ?? '';
      insertTemplate = !pageContent.trim();
    } catch (err) {
      if (seq !== loadSeq) return;
      console.error('Failed to load page for promote:', err);
      pageContent = '';
      insertTemplate = true;
    } finally {
      if (seq === loadSeq) pageLoading = false;
    }
  }

  async function loadAssignableUsers() {
    assignableLoading = true;
    try {
      assignableUsers = (await api.getAssignableUsers(workspaceId)) ?? [];
    } catch {
      assignableUsers = [];
    } finally {
      assignableLoading = false;
    }
  }

  $effect(() => {
    if (isOpen && workspaceId && pageId) {
      requirementType = DEFAULT_REQUIREMENT_TYPE;
      status = DEFAULT_REQUIREMENT_STATUS;
      ownerId = null;
      error = '';
      void loadPage();
      void loadAssignableUsers();
    }
    if (!isOpen) {
      loadSeq += 1;
      error = '';
      pageContent = '';
      pageLoading = false;
      insertTemplate = true;
      ownerSheetOpen = false;
    }
  });

  async function submit() {
    saving = true;
    error = '';
    try {
      if (pageIsEmpty && insertTemplate) {
        const template = applyStarterTemplate(requirementType, t);
        if (template) {
          await api.pages.updatePage(workspaceId, pageId, { content: template });
        }
      }
      const promoted = await api.requirements.promote(workspaceId, pageId, {
        requirement_type: requirementType,
        status,
        owner_id: ownerId,
      });
      isOpen = false;
      onPromoted(promoted);
    } catch (err) {
      error = err?.message || t('requirements.promoteError');
    } finally {
      saving = false;
    }
  }
</script>

<MobileSheet bind:isOpen title={t('requirements.promote')} dataTestid="mobile-promote-requirement-sheet">
  <div class="promote-form">
    {#if pageLoading}
      <div class="loading"><Loader class="spin" size={20} /></div>
    {/if}
    {#if error}
      <p class="error">{error}</p>
    {/if}

    <label class="field">
      <span>{t('requirements.fieldType')}</span>
      <NativeSelect
        bind:value={requirementType}
        disabled={pageLoading || saving}
        options={typeOptions.map((opt) => ({ value: opt.value, label: opt.label }))}
        dataTestid="mobile-promote-requirement-type"
      />
    </label>

    <label class="field">
      <span>{t('requirements.fieldStatus')}</span>
      <NativeSelect
        bind:value={status}
        disabled={pageLoading || saving}
        options={statusOptions.map((opt) => ({ value: opt.value, label: opt.label }))}
        dataTestid="mobile-promote-requirement-status"
      />
    </label>

    <label class="field">
      <span>{t('requirements.fieldOwner')}</span>
      <button
        class="owner-btn"
        type="button"
        onclick={() => (ownerSheetOpen = true)}
        disabled={pageLoading || saving}
        data-testid="mobile-promote-requirement-owner"
      >
        {ownerLabel}
      </button>
    </label>

    {#if !pageLoading && pageIsEmpty}
      <label class="checkbox-row">
        <input type="checkbox" bind:checked={insertTemplate} disabled={saving} />
        <span>{t('requirements.templates.insertOnPromote')}</span>
      </label>
    {/if}

    <div class="actions">
      <button class="btn secondary" type="button" onclick={() => (isOpen = false)} disabled={saving}>
        {t('common.cancel')}
      </button>
      <button class="btn primary" type="button" onclick={submit} disabled={saving || pageLoading}>
        {#if saving}<Loader class="spin" size={16} />{/if}
        {t('requirements.promote')}
      </button>
    </div>
  </div>
</MobileSheet>

<MobileOptionSheet
  bind:isOpen={ownerSheetOpen}
  title={t('requirements.fieldOwner')}
  options={assignableUsers}
  getValue={(user) => user.id}
  getLabel={(user) => formatRequirementOwnerLabel(user) || `#${user.id}`}
  selectedValue={ownerId}
  allowClear
  clearLabel={t('requirements.mobile.ownerUnset')}
  loading={assignableLoading}
  emptyText={t('requirements.mobile.ownerUnset')}
  onSelect={(user) => {
    ownerId = user.id;
    ownerSheetOpen = false;
  }}
  onClear={() => {
    ownerId = null;
    ownerSheetOpen = false;
  }}
  dataTestid="mobile-promote-requirement-owner-sheet"
/>

<style>
  .promote-form {
    display: flex;
    flex-direction: column;
    gap: 0.875rem;
    padding: 0 0.25rem 0.5rem;
  }

  .loading {
    display: flex;
    justify-content: center;
    padding: 0.5rem 0;
  }

  .error {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ds-text-danger);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }

  .owner-btn {
    width: 100%;
    text-align: left;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background: var(--ds-surface);
    color: var(--ds-text);
    padding: 0.625rem 0.75rem;
    font-size: 0.9375rem;
  }

  .checkbox-row {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    font-size: 0.875rem;
    color: var(--ds-text);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    padding-top: 0.25rem;
  }

  .btn {
    border: none;
    border-radius: var(--radius-lg, 8px);
    padding: 0.625rem 0.875rem;
    font-size: 0.9375rem;
    cursor: pointer;
  }

  .btn.secondary {
    background: var(--ds-surface-raised);
    color: var(--ds-text);
  }

  .btn.primary {
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>
