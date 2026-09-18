<script>
  import { Loader } from '@lucide/svelte';
  import { api } from '../api.js';
  import { currentRoute, navigate, setNavigationInterceptor } from '../router.js';
  import { authStore, workspacesStore } from '../stores';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import { successToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import NativeSelect from '../components/NativeSelect.svelte';
  import MobileEditorPage from './MobileEditorPage.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';
  import MobileOptionSheet from './MobileOptionSheet.svelte';
  import { autoGrow, enterMovesFocus } from './autoGrowTextarea.js';
  import { requirementTypeOptions } from '../features/requirements/requirementTypes.js';
  import { requirementStatusOptions } from '../features/requirements/requirementStatuses.js';
  import {
    DEFAULT_REQUIREMENT_STATUS,
    DEFAULT_REQUIREMENT_TYPE,
    applyStarterTemplate,
    canCreateRequirement,
    formatRequirementOwnerLabel,
    shouldConfirmTemplateReplace,
  } from '../features/requirements/requirementFormHelpers.js';

  const queryWorkspaceId = $derived(
    $currentRoute.query.workspace ? Number($currentRoute.query.workspace) : null
  );

  let workspaceId = $state(null);
  let title = $state('');
  let content = $state('');
  let contentField = $state(null);
  let requirementType = $state(DEFAULT_REQUIREMENT_TYPE);
  let status = $state(DEFAULT_REQUIREMENT_STATUS);
  let ownerId = $state(null);
  let previousRequirementType = $state(DEFAULT_REQUIREMENT_TYPE);
  let contentTouched = $state(false);
  let saving = $state(false);
  let error = $state('');
  let permissionsReady = $state(false);

  let assignableUsers = $state([]);
  let assignableLoading = $state(false);
  let ownerSheetOpen = $state(false);
  let confirmDiscardOpen = $state(false);
  let confirmTemplateOpen = $state(false);
  let pendingRequirementType = $state(null);
  let templateInitialized = $state(false);

  const personalWorkspace = $derived($workspacesStore.personalWorkspace ?? null);
  const regularWorkspaces = $derived($workspacesStore.regularWorkspaces ?? []);
  const creatableWorkspaces = $derived.by(() => {
    const all = [
      ...(personalWorkspace ? [personalWorkspace] : []),
      ...regularWorkspaces,
    ];
    return all.filter((ws) => canCreateRequirement(workspacePermissions, ws.id));
  });
  const typeOptions = $derived(requirementTypeOptions(t));
  const statusOptions = $derived(requirementStatusOptions(t));
  const ownerLabel = $derived.by(() => {
    if (!ownerId) return t('requirements.mobile.ownerUnset');
    const user = assignableUsers.find((u) => u.id === ownerId);
    return formatRequirementOwnerLabel(user) || `#${ownerId}`;
  });
  const isDirty = $derived(title.trim() !== '' || contentTouched);
  const canSave = $derived(
    Boolean(workspaceId) && title.trim() !== '' && !saving
  );

  $effect(() => {
    setNavigationInterceptor(() => {
      if (!isDirty) return false;
      confirmDiscardOpen = true;
      return true;
    });
    return () => setNavigationInterceptor(null);
  });

  $effect(() => {
    const userId = $authStore?.currentUser?.id;
    if (!userId) return;
    void workspacePermissions.loadPermissions(userId).finally(() => {
      permissionsReady = true;
    });
  });

  $effect(() => {
    if (!permissionsReady || creatableWorkspaces.length === 0) return;
    const preferred =
      queryWorkspaceId &&
      creatableWorkspaces.some((ws) => ws.id === queryWorkspaceId)
        ? queryWorkspaceId
        : creatableWorkspaces[0]?.id ?? null;
    if (!workspaceId || !creatableWorkspaces.some((ws) => ws.id === workspaceId)) {
      workspaceId = preferred;
    }
  });

  $effect(() => {
    const wsId = workspaceId;
    if (!wsId) {
      assignableUsers = [];
      ownerId = null;
      return;
    }
    let cancelled = false;
    assignableLoading = true;
    api
      .getAssignableUsers(wsId)
      .then((users) => {
        if (!cancelled) assignableUsers = users ?? [];
      })
      .catch(() => {
        if (!cancelled) assignableUsers = [];
      })
      .finally(() => {
        if (!cancelled) assignableLoading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function applyTemplate(type) {
    content = applyStarterTemplate(type, t);
    contentTouched = false;
  }

  function applyTypeChange(newType) {
    applyTemplate(newType);
    previousRequirementType = newType;
    requirementType = newType;
  }

  function handleTypeChange() {
    const newType = requirementType;
    const oldType = previousRequirementType;
    if (newType === oldType) return;
    if (shouldConfirmTemplateReplace(content, contentTouched)) {
      pendingRequirementType = newType;
      requirementType = oldType;
      confirmTemplateOpen = true;
      return;
    }
    applyTypeChange(newType);
  }

  function confirmTemplateReplace() {
    if (pendingRequirementType) {
      applyTypeChange(pendingRequirementType);
      pendingRequirementType = null;
    }
    confirmTemplateOpen = false;
  }

  function cancelTemplateReplace() {
    pendingRequirementType = null;
    confirmTemplateOpen = false;
  }

  function onContentInput() {
    contentTouched = true;
  }

  async function save() {
    if (!canSave || !workspaceId) {
      if (!workspaceId) error = t('requirements.mobile.workspaceRequired');
      return;
    }
    saving = true;
    error = '';
    try {
      const created = await api.requirements.create(workspaceId, {
        title: title.trim(),
        content,
        requirement_type: requirementType,
        status,
        owner_id: ownerId,
      });
      setNavigationInterceptor(null);
      successToast(t('requirements.mobile.createSuccess'));
      navigate(`/m/requirements/${workspaceId}/${created.requirement_number}`, { replace: true });
    } catch (err) {
      console.error('Failed to create requirement:', err);
      error = err?.message || t('requirements.createError');
    } finally {
      saving = false;
    }
  }

  function discardAndLeave() {
    confirmDiscardOpen = false;
    setNavigationInterceptor(null);
    navigate('/m/requirements', { replace: true });
  }

  function requestCancel() {
    if (isDirty) {
      confirmDiscardOpen = true;
      return;
    }
    navigate('/m/requirements', { replace: true });
  }

  $effect(() => {
    if (templateInitialized) return;
    applyTemplate(DEFAULT_REQUIREMENT_TYPE);
    templateInitialized = true;
  });
</script>

<MobileEditorPage
  title={t('requirements.mobile.createTitle')}
  saveLabel={t('requirements.create')}
  cancelLabel={t('common.cancel')}
  {canSave}
  {saving}
  {error}
  onsave={save}
  oncancel={requestCancel}
  dataTestid="mobile-requirement-create-page"
>
  {#if !permissionsReady}
    <div class="center" data-testid="mobile-requirement-create-loading">
      <Loader class="spin" size={22} />
    </div>
  {:else if creatableWorkspaces.length === 0}
    <div class="msg" data-testid="mobile-requirement-create-forbidden">
      <p>{t('requirements.mobile.createForbidden')}</p>
    </div>
  {:else}
    <div class="create-form" data-testid="mobile-requirement-create-form">
      <textarea
        class="hero-title"
        bind:value={title}
        placeholder={t('requirements.fieldTitle')}
        autocomplete="off"
        rows={1}
        enterkeyhint="next"
        use:autoGrow={title}
        use:enterMovesFocus={{ next: contentField }}
        data-testid="mobile-requirement-create-title"
      ></textarea>
      <textarea
        class="hero-content"
        bind:value={content}
        bind:this={contentField}
        rows={12}
        placeholder={t('requirements.fieldContent')}
        oninput={onContentInput}
        data-testid="mobile-requirement-create-content"
      ></textarea>
    </div>
  {/if}

  {#snippet footer()}
    {#if permissionsReady && creatableWorkspaces.length > 0}
      <div class="chips-bar" data-testid="mobile-requirement-create-properties">
        <div class="chips-scroll">
          <div class="chip chip-control">
            <NativeSelect
              bind:value={workspaceId}
              dataTestid="mobile-requirement-create-workspace"
              ariaLabel={t('common.workspace')}
              options={creatableWorkspaces.map((ws) => ({ value: ws.id, label: ws.name }))}
            />
          </div>
          <div class="chip chip-control">
            <NativeSelect
              bind:value={requirementType}
              onchange={handleTypeChange}
              dataTestid="mobile-requirement-create-type"
              ariaLabel={t('requirements.fieldType')}
              options={typeOptions.map((opt) => ({ value: opt.value, label: opt.label }))}
            />
          </div>
          <div class="chip chip-control">
            <NativeSelect
              bind:value={status}
              dataTestid="mobile-requirement-create-status"
              ariaLabel={t('requirements.fieldStatus')}
              options={statusOptions.map((opt) => ({ value: opt.value, label: opt.label }))}
            />
          </div>
          <button
            class="chip chip-button"
            type="button"
            onclick={() => (ownerSheetOpen = true)}
            data-testid="mobile-requirement-create-owner"
          >
            {t('requirements.fieldOwner')}: {ownerLabel}
          </button>
        </div>
      </div>
    {/if}
  {/snippet}
</MobileEditorPage>

<MobileConfirmSheet
  bind:isOpen={confirmDiscardOpen}
  title={t('common.discardChanges')}
  message={t('dialogs.confirmations.discardChanges')}
  confirmLabel={t('common.discard')}
  cancelLabel={t('pages.discardCancel')}
  destructive
  pushHistory={false}
  onconfirm={discardAndLeave}
  dataTestid="mobile-requirement-create-discard"
/>

<MobileConfirmSheet
  bind:isOpen={confirmTemplateOpen}
  title={t('requirements.fieldType')}
  message={t('requirements.templates.replaceConfirm')}
  confirmLabel={t('common.confirm')}
  cancelLabel={t('common.cancel')}
  onconfirm={confirmTemplateReplace}
  onclose={cancelTemplateReplace}
  dataTestid="mobile-requirement-create-template-replace"
/>

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
  dataTestid="mobile-requirement-create-owner-sheet"
/>

<style>
  .center,
  .msg {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 12rem;
    padding: 1rem;
    color: var(--ds-text-subtle);
  }

  .create-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 0.75rem 1rem 1rem;
  }

  .hero-title,
  .hero-content {
    width: 100%;
    border: none;
    outline: none;
    background: transparent;
    color: var(--ds-text);
    resize: none;
    font-family: inherit;
  }

  .hero-title {
    font-size: 1.375rem;
    font-weight: var(--font-semibold, 600);
    line-height: 1.3;
  }

  .hero-content {
    font-size: 1rem;
    line-height: 1.5;
    min-height: 12rem;
  }

  .chips-bar {
    border-top: 1px solid var(--ds-border);
    background: var(--ds-surface-raised);
    padding: 0.5rem 0.75rem calc(env(safe-area-inset-bottom, 0px) + 0.5rem);
  }

  .chips-scroll {
    display: flex;
    gap: 0.5rem;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .chip {
    flex-shrink: 0;
  }

  .chip-control {
    min-width: 8rem;
    max-width: 12rem;
  }

  .chip-button {
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 0.8125rem;
    padding: 0.5rem 0.75rem;
    white-space: nowrap;
  }
</style>
