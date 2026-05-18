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
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';

  const CREDENTIAL_TYPES = [
    { value: 'bearer_token', label: 'Bearer token' },
    { value: 'api_key', label: 'API key' },
    { value: 'basic_auth', label: 'Basic auth (user:password)' },
    { value: 'custom_header', label: 'Custom header value' },
  ];

  let credentials = $state([]);
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
  });

  function resetForm() {
    form = {
      name: '',
      credential_type: 'bearer_token',
      secret: '',
      is_enabled: true,
      secret_metadata: '',
    };
  }

  async function loadCredentials() {
    loading = true;
    try {
      credentials = (await api.actionCredentials.getAllGlobal()) || [];
    } catch (err) {
      console.error('Failed to load action credentials:', err);
      errorToast(err.message || 'Failed to load action credentials');
    } finally {
      loading = false;
    }
  }

  onMount(loadCredentials);

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
      errorToast('Name and secret are required');
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
      });
      successToast(`Credential created (${created.secret_prefix || 'masked'})`);
      closeAndClearSecret();
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || 'Failed to create credential');
    } finally {
      saving = false;
    }
  }

  async function handleUpdate() {
    if (!editing) return;
    saving = true;
    try {
      await api.actionCredentials.updateGlobal(editing.id, {
        name: form.name,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata,
      });
      successToast('Credential updated');
      closeAndClearSecret();
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || 'Failed to update credential');
    } finally {
      saving = false;
    }
  }

  async function handleRotate() {
    if (!rotating || !form.secret) return;
    saving = true;
    try {
      const updated = await api.actionCredentials.rotateGlobal(rotating.id, form.secret);
      successToast(`Secret rotated (${updated.secret_prefix || 'masked'})`);
      closeAndClearSecret();
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || 'Failed to rotate secret');
    } finally {
      saving = false;
    }
  }

  async function deleteCredential(cred) {
    const ok = await confirm({
      title: 'Delete credential',
      message: `Delete ${cred.name}? Any capability still referencing it will fail at runtime.`,
      confirmText: 'Delete',
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.actionCredentials.deleteGlobal(cred.id);
      successToast('Credential deleted');
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || 'Failed to delete credential');
    }
  }

  const columns = [
    { key: 'name', label: 'Name' },
    { key: 'type', label: 'Type' },
    { key: 'prefix', label: 'Secret' },
    { key: 'status', label: 'Status' },
    { key: 'actions', label: '', align: 'right' },
  ];
</script>

<div class="space-y-4">
  <PageHeader
    title="Action credentials"
    subtitle="Encrypted API tokens that HTTP capabilities reference instead of storing tokens inline. Secret values are write-only — once entered, they cannot be read back."
  >
    {#snippet actions()}
      <Button variant="primary" onclick={openCreate} icon={Plus}>Add credential</Button>
    {/snippet}
  </PageHeader>

  {#if loading}
    <div class="flex items-center justify-center py-12"><Spinner /></div>
  {:else if credentials.length === 0}
    <div
      class="flex flex-col items-center py-12 gap-3 rounded-lg border"
      style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">No credentials yet.</p>
      <Button variant="secondary" onclick={openCreate} icon={Plus}>Add the first credential</Button>
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
            title="Stored — never displayed in full"
          >
            {cred.secret_prefix || '••••••••'}
          </code>
        {:else}
          <span class="text-xs italic" style="color: var(--ds-text-danger);">no secret</span>
        {/if}
      {/snippet}
      {#snippet status(cred)}
        {#if cred.is_enabled}
          <div class="flex items-center gap-1">
            <Power size={14} style="color: var(--ds-icon-success);" />
            <span class="text-xs" style="color: var(--ds-text-success);">Enabled</span>
          </div>
        {:else}
          <div class="flex items-center gap-1">
            <PowerOff size={14} style="color: var(--ds-text-subtle);" />
            <span class="text-xs" style="color: var(--ds-text-subtle);">Disabled</span>
          </div>
        {/if}
      {/snippet}
      {#snippet actions(cred)}
        <div class="flex items-center justify-end gap-1">
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title="Rotate secret"
            onclick={() => openRotate(cred)}
          >
            <KeyRound size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-subtle);"
            title="Edit"
            onclick={() => openEdit(cred)}
          >
            <Edit size={14} />
          </button>
          <button
            class="p-1.5 rounded hover:opacity-80"
            style="color: var(--ds-text-danger);"
            title="Delete"
            onclick={() => deleteCredential(cred)}
          >
            <Trash2 size={14} />
          </button>
        </div>
      {/snippet}
    </DataTable>
  {/if}
</div>

<!-- Create modal -->
{#if showCreateModal}
  <Modal isOpen={true} onclose={closeAndClearSecret} onSubmit={handleCreate} submitDisabled={saving || !form.name || !form.secret}>
    {#snippet children(submitHint)}
      <ModalHeader title="Add credential" onclose={closeAndClearSecret} />
      <div class="p-4 space-y-4">
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">Name</span>
          <input
            type="text"
            class="mt-1 w-full px-3 py-2 text-sm rounded border bg-transparent"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            bind:value={form.name}
            placeholder="GitHub PAT"
            required
          />
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">Type</span>
          <Select bind:value={form.credential_type} options={CREDENTIAL_TYPES} class="mt-1" />
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">Secret</span>
          <input
            id="action-credential-secret"
            type="password"
            autocomplete="new-password"
            class="mt-1 w-full px-3 py-2 text-sm rounded border bg-transparent font-mono"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            bind:value={form.secret}
            placeholder="Enter token, API key, or user:password"
            required
          />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            Stored encrypted. You will never see this value again — write it down or rotate
            later if you need it.
          </p>
        </label>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">Metadata (JSON, optional)</span>
          <textarea
            class="mt-1 w-full px-3 py-2 text-sm rounded border bg-transparent font-mono"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            rows="3"
            bind:value={form.secret_metadata}
            placeholder={'{"provider":"github","scope":"repo"}'}
          ></textarea>
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            Non-sensitive metadata only. Keys like <code>token</code>, <code>secret</code>, <code>password</code> are rejected.
          </p>
        </label>
        <label class="flex items-center gap-2">
          <input type="checkbox" bind:checked={form.is_enabled} />
          <span class="text-sm" style="color: var(--ds-text);">Enabled</span>
        </label>
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={closeAndClearSecret} keyboardHint="Esc">Cancel</Button>
          <Button
            variant="primary"
            onclick={handleCreate}
            loading={saving}
            disabled={saving || !form.name || !form.secret}
            keyboardHint={submitHint}
          >
            Create
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}

<!-- Edit (metadata only) modal -->
{#if showEditModal && editing}
  <Modal isOpen={true} onclose={closeAndClearSecret} onSubmit={handleUpdate} submitDisabled={saving || !form.name}>
    {#snippet children(submitHint)}
      <ModalHeader title="Edit credential" onclose={closeAndClearSecret} />
      <div class="p-4 space-y-4">
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">Name</span>
          <input
            type="text"
            class="mt-1 w-full px-3 py-2 text-sm rounded border bg-transparent"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            bind:value={form.name}
            required
          />
        </label>
        <div>
          <span class="text-sm font-medium" style="color: var(--ds-text);">Secret</span>
          <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
            Stored (<code>{editing.secret_prefix || '••••••••'}</code>) — use "Rotate" to replace.
          </p>
        </div>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">Metadata (JSON, optional)</span>
          <textarea
            class="mt-1 w-full px-3 py-2 text-sm rounded border bg-transparent font-mono"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            rows="3"
            bind:value={form.secret_metadata}
          ></textarea>
        </label>
        <label class="flex items-center gap-2">
          <input type="checkbox" bind:checked={form.is_enabled} />
          <span class="text-sm" style="color: var(--ds-text);">Enabled</span>
        </label>
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={closeAndClearSecret} keyboardHint="Esc">Cancel</Button>
          <Button variant="primary" onclick={handleUpdate} loading={saving} disabled={saving} keyboardHint={submitHint}>
            Save
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
      <ModalHeader title={`Rotate secret — ${rotating.name}`} onclose={closeAndClearSecret} />
      <div class="p-4 space-y-4">
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          Enter the new secret value. The old value will be replaced immediately and cannot be recovered.
        </p>
        <label class="block">
          <span class="text-sm font-medium" style="color: var(--ds-text);">New secret</span>
          <input
            type="password"
            autocomplete="new-password"
            class="mt-1 w-full px-3 py-2 text-sm rounded border bg-transparent font-mono"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            bind:value={form.secret}
            required
          />
        </label>
        <div class="flex justify-end gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
          <Button variant="secondary" onclick={closeAndClearSecret} keyboardHint="Esc">Cancel</Button>
          <Button variant="primary" onclick={handleRotate} loading={saving} disabled={saving || !form.secret} keyboardHint={submitHint}>
            Rotate
          </Button>
        </div>
      </div>
    {/snippet}
  </Modal>
{/if}
