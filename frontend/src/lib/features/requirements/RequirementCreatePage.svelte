<script>
  import { onMount, tick } from 'svelte';
  import { IconArrowLeft as ArrowLeft, IconFileStack as FileStack } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { authStore } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Button from '../../components/Button.svelte';
  import RequirementCreateForm from './RequirementCreateForm.svelte';
  import {
    DEFAULT_REQUIREMENT_STATUS,
    DEFAULT_REQUIREMENT_TYPE,
    canCreateRequirement,
    defaultRequirementOwnerId,
  } from './requirementFormHelpers.js';

  let { workspaceId } = $props();

  let title = $state('');
  let content = $state('');
  let requirementType = $state(DEFAULT_REQUIREMENT_TYPE);
  let status = $state(DEFAULT_REQUIREMENT_STATUS);
  let ownerId = $state(null);
  let parentId = $state(null);
  let contentTouched = $state(false);
  let saving = $state(false);
  let error = $state('');
  let formRef = $state(null);

  const listUrl = $derived(`/workspaces/${workspaceId}/requirements`);
  const isDirty = $derived(title.trim() !== '' || contentTouched);

  onMount(async () => {
    if (!canCreateRequirement(workspacePermissions, workspaceId)) {
      navigate(listUrl, { replace: true });
      return;
    }
    await tick();
    const currentUserId = authStore.currentUser?.id ?? null;
    let defaultOwnerId = currentUserId;
    try {
      const assignableUsers = await api.getAssignableUsers(workspaceId);
      defaultOwnerId = defaultRequirementOwnerId(currentUserId, assignableUsers ?? []);
    } catch {
      defaultOwnerId = currentUserId;
    }
    formRef?.resetForm(defaultOwnerId);
  });

  $effect(() => {
    function onBeforeUnload(event) {
      if (!isDirty) return;
      event.preventDefault();
      event.returnValue = '';
    }

    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  });

  async function confirmDiscard() {
    return confirm({
      title: t('common.discardChanges'),
      message: t('dialogs.confirmations.discardChanges'),
      confirmText: t('common.discard'),
      cancelText: t('pages.discardCancel'),
      variant: 'warning',
    });
  }

  async function goBack() {
    if (isDirty) {
      const ok = await confirmDiscard();
      if (!ok) return;
    }
    navigate(listUrl);
  }

  async function submit() {
    if (!title.trim()) {
      error = t('requirements.createTitleRequired');
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
        parent_id: parentId,
      });
      navigate(`/workspaces/${workspaceId}/requirements/${created.requirement_number}`);
    } catch (err) {
      error = err?.message || t('requirements.createError');
    } finally {
      saving = false;
    }
  }
</script>

<section
  class="requirement-create-page flex h-full min-h-0 flex-col px-6 py-4 sm:px-8"
  data-testid="requirement-create-page"
>
  <div class="mx-auto flex h-full w-full max-w-5xl min-h-0 flex-col">
  <Button
    variant="subtle"
    size="small"
    icon={ArrowLeft}
    onclick={goBack}
    dataTestid="requirement-create-back"
  >
    {t('requirements.backToList')}
  </Button>

  <PageHeader icon={FileStack} title={t('requirements.create')} />

  {#if error}
    <p class="mb-4 text-sm text-[var(--ds-text-danger)]">{error}</p>
  {/if}

  <div class="requirement-create-page__form min-h-0 flex-1 overflow-y-auto px-1">
    <RequirementCreateForm
      bind:this={formRef}
      {workspaceId}
      bind:title
      bind:content
      bind:requirementType
      bind:status
      bind:ownerId
      bind:parentId
      bind:contentTouched
      {saving}
    />
  </div>

  <div class="requirement-create-page__actions mt-6 flex justify-end gap-2 border-t pt-4" style="border-color: var(--ds-border);">
    <Button variant="secondary" onclick={goBack} disabled={saving}>
      {t('common.cancel')}
    </Button>
    <Button onclick={submit} disabled={saving} dataTestid="requirement-create-submit">
      {t('requirements.create')}
    </Button>
  </div>
  </div>
</section>
