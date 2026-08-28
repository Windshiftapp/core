<!--
  ActionCredentialManager
  -----------------------
  Admin page for global (workspace_id IS NULL) action credentials. Workspace-
  scoped credentials live under workspace settings; this page only lists/
  manages the global pool.

  Write-only secret model:
    - Create: plaintext secret entered once, encrypted server-side, never echoed.
    - Edit  : metadata only (name, enabled, metadata JSON).
    - Rotate: plaintext entered once; server re-encrypts; only the new prefix
              appears in the success toast.
-->
<script>
  import { onMount } from 'svelte';
  import { Plus, Edit, Trash2, KeyRound, Power, PowerOff } from '@lucide/svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Radio from '../components/Radio.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';

  const CREDENTIAL_TYPES = $derived([
    { value: 'bearer_token', label: t('settings.adminOperations.actionCredentials.bearerToken') },
    { value: 'api_key', label: t('settings.adminOperations.actionCredentials.apiKey') },
    { value: 'basic_auth', label: t('settings.adminOperations.actionCredentials.basicAuth') },
    { value: 'custom_header', label: t('settings.adminOperations.actionCredentials.customHeader') },
  ]);

  let credentials = $state([]);
  let workspaces = $state([]);
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let showRotateModal = $state(false);
  let editing = $state(null);
  let rotating = $state(null);
  let saving = $state(false);

  // Form state — `secret` only lives in this component while the modal is
  // open. We deliberately do NOT pre-populate it from any server response.
  let form = $state({
    name: '',
    credential_type: 'bearer_token',
    secret: '',
    is_enabled: true,
    secret_metadata: '',
    applies_to_all_workspaces: true,
    workspace_ids: [],
  });

  function resetForm() {
    form = {
      name: '',
      credential_type: 'bearer_token',
      secret: '',
      is_enabled: true,
      secret_metadata: '',
      applies_to_all_workspaces: true,
      workspace_ids: [],
    };
  }

  function toggleWorkspaceScope(workspaceId) {
    const id = Number(workspaceId);
    if (form.workspace_ids.includes(id)) {
      form.workspace_ids = form.workspace_ids.filter((w) => w !== id);
    } else {
      form.workspace_ids = [...form.workspace_ids, id];
    }
  }

  function workspaceScopeInvalid() {
    return !form.applies_to_all_workspaces && form.workspace_ids.length === 0;
  }

  async function loadCredentials() {
    try {
      credentials = (await api.actionCredentials.getAllGlobal()) || [];
    } catch (err) {
      console.error('Failed to load action credentials:', err);
      errorToast(err.message || t('settings.adminOperations.actionCredentials.loadFailed'));
    }
  }

  async function loadWorkspaces() {
    try {
      workspaces = (await api.workspaces.getAll()) || [];
    } catch (err) {
      console.error('Failed to load workspaces:', err);
    }
  }

  onMount(async () => {
    loading = true;
    await Promise.all([loadCredentials(), loadWorkspaces()]);
    loading = false;
  });

  function openCreate() {
    resetForm();
    showCreateModal = true;
  }

  function openEdit(cred) {
    editing = cred;
    // Metadata-only fields. The plaintext secret is NEVER pre-populated.
    form = {
      name: cred.name,
      credential_type: cred.credential_type,
      secret: '',
      is_enabled: cred.is_enabled,
      secret_metadata: cred.secret_metadata || '',
      applies_to_all_workspaces: cred.applies_to_all_workspaces ?? true,
      workspace_ids: Array.isArray(cred.workspace_ids) ? [...cred.workspace_ids] : [],
    };
    showEditModal = true;
  }

  function openRotate(cred) {
    rotating = cred;
    form.secret = '';
    showRotateModal = true;
  }

  function closeAndClearSecret() {
    showCreateModal = false;
    showEditModal = false;
    showRotateModal = false;
    editing = null;
    rotating = null;
    // Defensive: zero the secret string before resetting the rest of form.
    form.secret = '';
    resetForm();
  }

  async function handleCreate() {
    if (!form.name || !form.secret) {
      errorToast(t('settings.adminOperations.actionCredentials.nameSecretRequired'));
      return;
    }
    if (workspaceScopeInvalid()) {
      errorToast(t('settings.adminOperations.actionCredentials.workspaceRequired'));
      return;
    }
    saving = true;
    try {
      const created = await api.actionCredentials.createGlobal({
        name: form.name,
        credential_type: form.credential_type,
        secret: form.secret,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata || '',
        applies_to_all_workspaces: form.applies_to_all_workspaces,
        workspace_ids: form.applies_to_all_workspaces ? [] : form.workspace_ids,
      });
      successToast(t('settings.adminOperations.actionCredentials.created', { prefix: created.secret_prefix || t('settings.adminOperations.actionCredentials.masked') }));
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
    if (workspaceScopeInvalid()) {
      errorToast(t('settings.adminOperations.actionCredentials.workspaceRequired'));
      return;
    }
    saving = true;
    try {
      await api.actionCredentials.updateGlobal(editing.id, {
        name: form.name,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata,
        applies_to_all_workspaces: form.applies_to_all_workspaces,
        workspace_ids: form.applies_to_all_workspaces ? [] : form.workspace_ids,
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
      const updated = await api.actionCredentials.rotateGlobal(rotating.id, form.secret);
      successToast(t('settings.adminOperations.actionCredentials.rotated', { prefix: updated.secret_prefix || t('settings.adminOperations.actionCredentials.masked') }));
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
      await api.actionCredentials.deleteGlobal(cred.id);
      successToast(t('settings.adminOperations.actionCredentials.deleted'));
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.actionCredentials.deleteFailed'));
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name') },
    { key: 'type', label: t('common.type') },
    { key: 'prefix', label: t('settings.adminOperations.actionCredentials.secret') },
    { key: 'scope', label: t('settings.adminOperations.actionCredentials.scope') },
    { key: 'status', label: t('common.status') },
    { key: 'actions', label: '', align: 'right' },
  ]);

  function workspaceNames(ids) {
    if (!Array.isArray(ids) || ids.length === 0) return '';
    return ids
      .map((id) => workspaces.find((w) => w.id === id)?.name)
      .filter(Boolean)
      .join(', ');
  }
</script>

<div class="space-y-4">
  <PageHeader
    title={t('settings.adminOperations.actionCredentials.title')}
    subtitle={t('settings.adminOperations.actionCredentials.subtitle')}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        onclick={openCreate}
        icon={Plus}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('actionCredentials', 'add') }}
      >
        {t('settings.adminOperations.actionCredentials.add')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex items-center justify-center py-12"><Spinner /></div>
  {:else if credentials.length === 0}
    <div
      class="flex flex-col items-center py-12 gap-3 rounded-lg border"
      style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.actionCredentials.empty')}</p>
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
        <span class="font-medium" style="color: var(--ds-text);">{cred.name}</span>
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
          <span class="text-xs italic" style="color: var(--ds-text-danger);">{t('settings.adminOperations.actionCredentials.noSecret')}</span>
        {/if}
      {/snippet}
      {#snippet scope(cred)}
        {#if cred.applies_to_all_workspaces}
          <Lozenge appearance="success" size="sm">{t('settings.adminOperations.actionCredentials.allWorkspaces')}</Lozenge>
        {:else}
          <span class="text-xs" style="color: var(--ds-text-subtle);" title={workspaceNames(cred.workspace_ids)}>
            {t('settings.adminOperations.actionCredentials.workspaceCount', { count: (cred.workspace_ids || []).length })}
          </span>
        {/if}
      {/snippet}
      {#snippet status(cred)}
        {#if cred.is_enabled}
          <div class="flex items-center gap-1">
            <Power size={14} style="color: var(--ds-icon-success);" />
            <span class="text-xs" style="color: var(--ds-text-success);">{t('common.enabled')}</span>
          </div>
        {:else}
          <div class="flex items-center gap-1">
            <PowerOff size={14} style="color: var(--ds-text-subtle);" />
            <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.disabled')}</span>
          </div>
        {/if}
      {/snippet}
      {#snippet actions(cred)}
        <div class="flex items-center justify-end gap-1">
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title={t('settings.adminOperations.actionCredentials.rotateSecret')}
            onclick={() => openRotate(cred)}
          >
            <KeyRound size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title={t('common.edit')}
            onclick={() => openEdit(cred)}
          >
            <Edit size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-danger);"
            title={t('common.delete')}
            onclick={() => deleteCredential(cred)}
          >
            <Trash2 size={14} />
          </button>
        </div>
      {/snippet}
    </DataTable>
  {/if}
</div>

{#snippet scopeFields()}
  <div class="space-y-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <div class="block text-xs font-medium" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.actionCredentials.workspaceScope')}</div>
    <label class="flex items-start gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <Radio
        name="cred-scope"
        checked={form.applies_to_all_workspaces}
        onchange={() => { form.applies_to_all_workspaces = true; }}
        class="mt-0.5"
      />
      <div>
        <div>{t('settings.adminOperations.actionCredentials.availableAll')}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.actionCredentials.availableAllHelp')}</div>
      </div>
    </label>
    <label class="flex items-start gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
      <Radio
        name="cred-scope"
        checked={!form.applies_to_all_workspaces}
        onchange={() => { form.applies_to_all_workspaces = false; }}
        class="mt-0.5"
      />
      <div>
        <div>{t('settings.adminOperations.actionCredentials.restrict')}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.actionCredentials.restrictHelp')}</div>
      </div>
    </label>

    {#if !form.applies_to_all_workspaces}
      <div class="ml-6 mt-1 max-h-40 overflow-auto rounded-md border p-2" style="border-color: var(--ds-border); background: var(--ds-surface);">
        {#if workspaces.length === 0}
          <p class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.adminOperations.actionCredentials.noWorkspaces')}</p>
        {:else}
          {#each workspaces as ws}
            <Checkbox
              checked={form.workspace_ids.includes(ws.id)}
              onchange={() => toggleWorkspaceScope(ws.id)}
              label={ws.name}
              size="small"
            />
          {/each}
        {/if}
      </div>
    {/if}
  </div>
{/snippet}

<!-- Create modal -->
{#if showCreateModal}
  <Modal isOpen={true} onclose={closeAndClearSecret} onSubmit={handleCreate} submitDisabled={saving || !form.name || !form.secret || workspaceScopeInvalid()}>
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.actionCredentials.add')} onclose={closeAndClearSecret} />
      <div class="p-4 space-y-4">
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.name')}</span>
          <Input
            type="text"
            class="mt-1"
            bind:value={form.name}
            placeholder={t('settings.adminOperations.actionCredentials.namePlaceholder')}
            required
          />
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.type')}</span>
          <Select bind:value={form.credential_type} options={CREDENTIAL_TYPES} class="mt-1" />
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.secret')}</span>
          <Input
            id="action-credential-secret"
            type="password"
            autocomplete="new-password"
            class="mt-1 font-mono"
            bind:value={form.secret}
            placeholder={t('settings.adminOperations.actionCredentials.secretPlaceholder')}
            required
          />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {t('settings.adminOperations.actionCredentials.secretHelp')}
          </p>
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.metadata')}</span>
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
        {@render scopeFields()}
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={closeAndClearSecret} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button
            variant="primary"
            onclick={handleCreate}
            loading={saving}
            disabled={saving || !form.name || !form.secret || workspaceScopeInvalid()}
            keyboardHint={submitHint}
          >
            {t('common.create')}
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}

<!-- Edit (metadata only) modal -->
{#if showEditModal && editing}
  <Modal isOpen={true} onclose={closeAndClearSecret} onSubmit={handleUpdate} submitDisabled={saving || !form.name || workspaceScopeInvalid()}>
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.actionCredentials.edit')} onclose={closeAndClearSecret} />
      <div class="p-4 space-y-4">
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.name')}</span>
          <Input
            type="text"
            class="mt-1"
            bind:value={form.name}
            required
          />
        </label>
        <div>
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.secret')}</span>
          <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
            {t('settings.adminOperations.actionCredentials.storedRotate', { prefix: editing.secret_prefix || '••••••••' })}
          </p>
        </div>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.metadata')}</span>
          <Textarea
            class="mt-1 font-mono"
            rows={3}
            bind:value={form.secret_metadata}
          />
        </label>
        <Checkbox bind:checked={form.is_enabled} label={t('common.enabled')} />
        {@render scopeFields()}
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={closeAndClearSecret} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button variant="primary" onclick={handleUpdate} loading={saving} disabled={saving || workspaceScopeInvalid()} keyboardHint={submitHint}>
            {t('common.save')}
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}

<!-- Rotate modal -->
{#if showRotateModal && rotating}
  <Modal isOpen={true} onclose={closeAndClearSecret} onSubmit={handleRotate} submitDisabled={saving || !form.secret}>
    {#snippet children(submitHint)}
      <ModalHeader title={t('settings.adminOperations.actionCredentials.rotateTitle', { name: rotating.name })} onclose={closeAndClearSecret} />
      <div class="p-4 space-y-4">
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('settings.adminOperations.actionCredentials.rotateHelp')}
        </p>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.newSecret')}</span>
          <Input
            type="password"
            autocomplete="new-password"
            class="mt-1 font-mono"
            bind:value={form.secret}
            required
          />
        </label>
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={closeAndClearSecret} keyboardHint="Esc">{t('common.cancel')}</Button>
          <Button variant="primary" onclick={handleRotate} loading={saving} disabled={saving || !form.secret} keyboardHint={submitHint}>
            {t('settings.adminOperations.actionCredentials.rotate')}
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}
