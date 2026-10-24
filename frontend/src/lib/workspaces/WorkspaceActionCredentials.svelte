<!--
  WorkspaceActionCredentials
  --------------------------
  Workspace settings module for action credentials usable in this workspace
  (WI-1433). Mounted at /workspaces/:id/settings/action-credentials, gated on
  action.credential.manage.

  The list also includes rows the workspace inherits (all-workspace or
  multi-workspace credentials managed by a system admin); those render
  read-only — the workspace API only serves create/rotate/delete for rows
  pinned to exactly this workspace, so the UI must not offer them.
-->
<script>
  import { onMount } from 'svelte';
  import { actionCredentials } from '../api/ai.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import Shield from '@lucide/svelte/icons/shield';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import EnabledStatus from '../settings/EnabledStatus.svelte';
  import EntityFormModal from '../settings/EntityFormModal.svelte';
  import EntityRowActions from '../settings/EntityRowActions.svelte';

  let { workspaceId = null } = $props();

  const CREDENTIAL_TYPES = $derived([
    { value: 'bearer_token', label: t('settings.adminOperations.actionCredentials.bearerToken') },
    { value: 'api_key', label: t('settings.adminOperations.actionCredentials.apiKey') },
    { value: 'basic_auth', label: t('settings.adminOperations.actionCredentials.basicAuth') },
    { value: 'custom_header', label: t('settings.adminOperations.actionCredentials.customHeader') },
  ]);

  const canManage = $derived(
    workspacePermissions.hasPermission(workspaceId, 'action.credential.manage')
  );

  let credentials = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let showRotateModal = $state(false);
  let editing = $state(null);
  let rotating = $state(null);
  let saving = $state(false);

  // Secret only lives in this component while a modal is open; it is never
  // pre-populated from a server response.
  let form = $state(blankForm());

  function blankForm() {
    return {
      name: '',
      credential_type: 'bearer_token',
      secret: '',
      is_enabled: true,
      secret_metadata: '',
    };
  }

  function closeAndClearSecret() {
    showCreateModal = false;
    showEditModal = false;
    showRotateModal = false;
    editing = null;
    rotating = null;
    form.secret = '';
    form = blankForm();
  }

  // The workspace API only serves mutations for rows pinned to exactly this
  // workspace; inherited rows (all-workspace or shared) are system-admin
  // territory and render read-only.
  function isWorkspaceOwned(cred) {
    return (
      !cred.applies_to_all_workspaces &&
      Array.isArray(cred.workspace_ids) &&
      cred.workspace_ids.length === 1 &&
      Number(cred.workspace_ids[0]) === Number(workspaceId)
    );
  }

  async function loadCredentials() {
    loading = true;
    try {
      credentials = (await actionCredentials.getForWorkspace(workspaceId)) || [];
    } catch (err) {
      console.error('Failed to load workspace action credentials:', err);
      errorToast(err.message || t('settings.adminOperations.actionCredentials.loadFailed'));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (canManage) {
      void loadCredentials();
    } else {
      loading = false;
    }
  });

  function openCreate() {
    form = blankForm();
    showCreateModal = true;
  }

  function openEdit(cred) {
    editing = cred;
    // Metadata only — the plaintext secret is never pre-populated.
    form = {
      name: cred.name,
      credential_type: cred.credential_type,
      secret: '',
      is_enabled: cred.is_enabled,
      secret_metadata: cred.secret_metadata || '',
    };
    showEditModal = true;
  }

  function openRotate(cred) {
    rotating = cred;
    form.secret = '';
    showRotateModal = true;
  }

  async function handleCreate() {
    if (!form.name || !form.secret) {
      errorToast(t('settings.adminOperations.actionCredentials.nameSecretRequired'));
      return;
    }
    saving = true;
    try {
      // The server pins the credential to this workspace; scope fields are
      // ignored on this endpoint.
      const created = await actionCredentials.createForWorkspace(workspaceId, {
        name: form.name,
        credential_type: form.credential_type,
        secret: form.secret,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata || '',
      });
      successToast(
        t('settings.adminOperations.actionCredentials.created', {
          prefix: created.secret_prefix || t('settings.adminOperations.actionCredentials.masked'),
        })
      );
      closeAndClearSecret();
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.actionCredentials.createFailed'));
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!editing) return;
    saving = true;
    try {
      await actionCredentials.updateForWorkspace(workspaceId, editing.id, {
        name: form.name,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata,
      });
      successToast(t('settings.adminOperations.actionCredentials.updated'));
      closeAndClearSecret();
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.actionCredentials.updateFailed'));
    } finally {
      saving = false;
    }
  }

  async function handleRotate() {
    if (!rotating || !form.secret) return;
    saving = true;
    try {
      const updated = await actionCredentials.rotateForWorkspace(
        workspaceId,
        rotating.id,
        form.secret
      );
      successToast(
        t('settings.adminOperations.actionCredentials.rotated', {
          prefix: updated.secret_prefix || t('settings.adminOperations.actionCredentials.masked'),
        })
      );
      closeAndClearSecret();
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.actionCredentials.rotateFailed'));
    } finally {
      saving = false;
    }
  }

  async function deleteCredential(cred) {
    const ok = await confirm({
      title: t('settings.adminOperations.actionCredentials.deleteTitle'),
      message: t('settings.adminOperations.actionCredentials.deleteMessage', { name: cred.name }),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await actionCredentials.deleteForWorkspace(workspaceId, cred.id);
      successToast(t('settings.adminOperations.actionCredentials.deleted'));
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.actionCredentials.deleteFailed'));
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name'), slot: 'name' },
    { key: 'type', label: t('common.type'), slot: 'type' },
    {
      key: 'prefix',
      label: t('settings.adminOperations.actionCredentials.secret'),
      slot: 'prefix',
    },
    {
      key: 'scope',
      label: t('settings.adminOperations.actionCredentials.scope'),
      slot: 'scope',
    },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'actions', label: '', slot: 'actions', align: 'right' },
  ]);
</script>

{#if !canManage}
  <!-- Reached only by direct URL: the settings nav hides this module without
       action.credential.manage. -->
  <div
    class="rounded-xl p-6 border shadow-sm flex flex-col items-center text-center"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
    data-testid="workspace-credentials-access-denied"
  >
    <Shield class="w-12 h-12 mb-4" style="color: var(--ds-icon-warning, var(--ds-text-subtle));" />
    <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">
      {t('workspaceSettings.actionCredentials.noPermissionTitle')}
    </h2>
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {t('workspaceSettings.actionCredentials.noPermissionBody')}
    </p>
  </div>
{:else if loading}
  <StateDisplay type="loading" />
{:else}
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('workspaceSettings.actionCredentials.description')}
      </p>
      <Button
        variant="primary"
        onclick={openCreate}
        icon={Plus}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('actionCredentials', 'add') }}
        dataTestid="workspace-credential-add"
      >
        {t('settings.adminOperations.actionCredentials.add')}
      </Button>
    </div>

    {#if credentials.length === 0}
      <div
        class="flex flex-col items-center py-12 gap-3 rounded-lg border"
        style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
        data-testid="workspace-credentials-empty"
      >
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('workspaceSettings.actionCredentials.empty')}
        </p>
        <Button
          variant="secondary"
          onclick={openCreate}
          icon={Plus}
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('actionCredentials', 'add') }}
        >
          {t('settings.adminOperations.actionCredentials.addFirst')}
        </Button>
      </div>
    {:else}
      <DataTable {columns} data={credentials} keyField="id">
        {#snippet name(cred)}
          <span
            class="font-medium"
            style="color: var(--ds-text);"
            data-testid={`workspace-credential-name-${cred.id}`}
            data-credential-id={cred.id}
          >
            {cred.name}
          </span>
        {/snippet}
        {#snippet type(cred)}
          <Lozenge appearance="default" size="sm">{cred.credential_type}</Lozenge>
        {/snippet}
        {#snippet prefix(cred)}
          {#if cred.has_secret}
            <code
              class="text-xs font-mono"
              style="color: var(--ds-text-subtle);"
              title={t('settings.adminOperations.actionCredentials.storedSecret')}
            >
              {cred.secret_prefix || '••••••••'}
            </code>
          {:else}
            <span class="text-xs italic" style="color: var(--ds-text-danger);">
              {t('settings.adminOperations.actionCredentials.noSecret')}
            </span>
          {/if}
        {/snippet}
        {#snippet scope(cred)}
          {#if isWorkspaceOwned(cred)}
            <Lozenge appearance="success" size="sm">
              {t('workspaceSettings.actionCredentials.ownedHere')}
            </Lozenge>
          {:else}
            <span
              class="text-xs"
              style="color: var(--ds-text-subtle);"
              title={t('workspaceSettings.actionCredentials.inheritedHelp')}
            >
              {t('workspaceSettings.actionCredentials.inherited')}
            </span>
          {/if}
        {/snippet}
        {#snippet status(cred)}
          <EnabledStatus enabled={cred.is_enabled} />
        {/snippet}
        {#snippet actions(cred)}
          {#if isWorkspaceOwned(cred)}
            <EntityRowActions
              actions={[
                {
                  id: 'rotate',
                  icon: KeyRound,
                  title: t('settings.adminOperations.actionCredentials.rotateSecret'),
                  testId: `workspace-credential-rotate-${cred.id}`,
                  onclick: () => openRotate(cred),
                },
                {
                  id: 'edit',
                  icon: Edit,
                  title: t('common.edit'),
                  testId: `workspace-credential-edit-${cred.id}`,
                  onclick: () => openEdit(cred),
                },
                {
                  id: 'delete',
                  icon: Trash2,
                  title: t('common.delete'),
                  danger: true,
                  testId: `workspace-credential-delete-${cred.id}`,
                  onclick: () => deleteCredential(cred),
                },
              ]}
            />
          {:else}
            <span class="text-xs" style="color: var(--ds-text-subtlest);" title={t('workspaceSettings.actionCredentials.inheritedHelp')}>
              {t('workspaceSettings.actionCredentials.managedByAdmin')}
            </span>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </div>
{/if}

<!-- Create modal: no scope fields — the server pins the credential to this workspace. -->
{#if showCreateModal}
  <EntityFormModal
    title={t('settings.adminOperations.actionCredentials.add')}
    onclose={closeAndClearSecret}
    onsubmit={handleCreate}
    disabled={!form.name || !form.secret}
    {saving}
    confirmLabel={t('common.create')}
  >
    {#snippet fields()}
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.name')}</span>
        <Input
          type="text"
          class="mt-1"
          bind:value={form.name}
          placeholder={t('settings.adminOperations.actionCredentials.namePlaceholder')}
          dataTestid="workspace-credential-name-input"
          required
        />
      </label>
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.type')}</span>
        <Select bind:value={form.credential_type} options={CREDENTIAL_TYPES} class="mt-1" />
      </label>
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          {t('settings.adminOperations.actionCredentials.secret')}
        </span>
        <Input
          type="password"
          autocomplete="new-password"
          class="mt-1 font-mono"
          bind:value={form.secret}
          placeholder={t('settings.adminOperations.actionCredentials.secretPlaceholder')}
          dataTestid="workspace-credential-secret-input"
          required
        />
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {t('settings.adminOperations.actionCredentials.secretHelp')}
        </p>
      </label>
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          {t('settings.adminOperations.actionCredentials.metadata')}
        </span>
        <Textarea
          class="mt-1 font-mono"
          rows={3}
          bind:value={form.secret_metadata}
          placeholder={'{"provider":"github","scope":"repo"}'}
        />
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {t('settings.adminOperations.actionCredentials.metadataHelp')}
        </p>
      </label>
      <Checkbox bind:checked={form.is_enabled} label={t('common.enabled')} />
      <p class="text-xs" style="color: var(--ds-text-subtle);">
        {t('workspaceSettings.actionCredentials.pinnedHelp')}
      </p>
    {/snippet}
  </EntityFormModal>
{/if}

<!-- Edit (metadata only) modal -->
{#if showEditModal && editing}
  <EntityFormModal
    title={t('settings.adminOperations.actionCredentials.edit')}
    onclose={closeAndClearSecret}
    onsubmit={handleUpdate}
    disabled={!form.name}
    {saving}
  >
    {#snippet fields()}
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.name')}</span>
        <Input type="text" class="mt-1" bind:value={form.name} required />
      </label>
      <div>
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          {t('settings.adminOperations.actionCredentials.secret')}
        </span>
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          {t('settings.adminOperations.actionCredentials.storedRotate', {
            prefix: editing.secret_prefix || '••••••••',
          })}
        </p>
      </div>
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          {t('settings.adminOperations.actionCredentials.metadata')}
        </span>
        <Textarea class="mt-1 font-mono" rows={3} bind:value={form.secret_metadata} />
      </label>
      <Checkbox bind:checked={form.is_enabled} label={t('common.enabled')} />
    {/snippet}
  </EntityFormModal>
{/if}

<!-- Rotate modal -->
{#if showRotateModal && rotating}
  <EntityFormModal
    title={t('settings.adminOperations.actionCredentials.rotateTitle', { name: rotating.name })}
    onclose={closeAndClearSecret}
    onsubmit={handleRotate}
    disabled={!form.secret}
    {saving}
    confirmLabel={t('settings.adminOperations.actionCredentials.rotate')}
  >
    {#snippet fields()}
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('settings.adminOperations.actionCredentials.rotateHelp')}
      </p>
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          {t('settings.adminOperations.actionCredentials.newSecret')}
        </span>
        <Input
          type="password"
          autocomplete="new-password"
          class="mt-1 font-mono"
          bind:value={form.secret}
          dataTestid="workspace-credential-rotate-input"
          required
        />
      </label>
    {/snippet}
  </EntityFormModal>
{/if}
