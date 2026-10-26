<!--
  ActionCredentialsManager
  ------------------------
  Shared CRUD surface for action credentials, in two scopes:

    - global    — the system-admin pool (workspace_id IS NULL). Scope fields
                  choose all-workspaces vs. a set of workspaces.
    - workspace — credentials usable in one workspace. Inherited rows (owned
                  by another workspace or by the global pool) render read-only.

  Write-only secret model:
    - Create: plaintext secret entered once, encrypted server-side, never echoed.
    - Edit  : metadata only (name, enabled, metadata JSON, global scope).
    - Rotate: plaintext entered once; server re-encrypts; only the new prefix
              appears in the success toast.
-->
<script>
  import { onMount } from 'svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import Shield from '@lucide/svelte/icons/shield';
  import StateDisplay from '../components/StateDisplay.svelte';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import WorkspaceCheckboxList from '../components/WorkspaceCheckboxList.svelte';
  import Radio from '../components/Radio.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import EnabledStatus from './EnabledStatus.svelte';
  import EntityFormModal from './EntityFormModal.svelte';
  import EntityRowActions from './EntityRowActions.svelte';

  /**
   * @type {{
   *   mode: 'global' | 'workspace',
   *   workspaceId?: number | null,
   *   canManage?: boolean,
   *   api: {
   *     list: () => Promise<any>,
   *     create: (data: any) => Promise<any>,
   *     update: (id: number, data: any) => Promise<any>,
   *     rotate: (id: number, secret: string) => Promise<any>,
   *     remove: (id: number) => Promise<any>,
   *   },
   *   i18n: {
   *     title?: string | null,
   *     subtitle?: string | null,
   *     add: string,
   *     empty: string,
   *     addFirst: string,
   *     description?: string | null,
   *     noPermissionTitle?: string | null,
   *     noPermissionBody?: string | null,
   *   },
   *   testids?: {
   *     add?: string,
   *     empty?: string,
   *     accessDenied?: string,
   *     nameInput?: string,
   *     secretInput?: string,
   *     rotateInput?: string,
   *     rowPrefix?: string,
   *   },
   * }}
   */
  let { mode, workspaceId = null, canManage = true, api, i18n, testids = {} } = $props();

  const isGlobal = $derived(mode === 'global');

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

  // `secret` only lives in this component while a modal is open. We
  // deliberately do NOT pre-populate it from any server response.
  let form = $state(blankForm());

  function blankForm() {
    return {
      name: '',
      credential_type: 'bearer_token',
      secret: '',
      is_enabled: true,
      secret_metadata: '',
      applies_to_all_workspaces: true,
      workspace_ids: [],
    };
  }

  function resetForm() {
    form = blankForm();
  }

  function toggleWorkspaceScope(id) {
    const numericId = Number(id);
    if (form.workspace_ids.includes(numericId)) {
      form.workspace_ids = form.workspace_ids.filter((w) => w !== numericId);
    } else {
      form.workspace_ids = [...form.workspace_ids, numericId];
    }
  }

  function workspaceScopeInvalid() {
    return !form.applies_to_all_workspaces && form.workspace_ids.length === 0;
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

  function rowActionsAvailable(cred) {
    return isGlobal || isWorkspaceOwned(cred);
  }

  async function loadCredentials() {
    try {
      credentials = (await api.list()) || [];
    } catch (err) {
      console.error('Failed to load action credentials:', err);
      errorToast(err.message || t('settings.adminOperations.actionCredentials.loadFailed'));
    }
  }

  async function loadWorkspaces() {
    try {
      workspaces = (await workspacesStore.load()) || [];
    } catch (err) {
      console.error('Failed to load workspaces:', err);
    }
  }

  onMount(async () => {
    loading = true;
    if (isGlobal) {
      await Promise.all([loadCredentials(), loadWorkspaces()]);
    } else if (canManage) {
      await loadCredentials();
    }
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
    if (isGlobal && workspaceScopeInvalid()) {
      errorToast(t('settings.adminOperations.actionCredentials.workspaceRequired'));
      return;
    }
    saving = true;
    try {
      const payload = {
        name: form.name,
        credential_type: form.credential_type,
        secret: form.secret,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata || '',
      };
      if (isGlobal) {
        payload.applies_to_all_workspaces = form.applies_to_all_workspaces;
        payload.workspace_ids = form.applies_to_all_workspaces ? [] : form.workspace_ids;
      }
      const created = await api.create(payload);
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
    if (isGlobal && workspaceScopeInvalid()) {
      errorToast(t('settings.adminOperations.actionCredentials.workspaceRequired'));
      return;
    }
    saving = true;
    try {
      const payload = {
        name: form.name,
        is_enabled: form.is_enabled,
        secret_metadata: form.secret_metadata,
      };
      if (isGlobal) {
        payload.applies_to_all_workspaces = form.applies_to_all_workspaces;
        payload.workspace_ids = form.applies_to_all_workspaces ? [] : form.workspace_ids;
      }
      await api.update(editing.id, payload);
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
      const updated = await api.rotate(rotating.id, form.secret);
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
      await api.remove(cred.id);
      successToast(t('settings.adminOperations.actionCredentials.deleted'));
      await loadCredentials();
    } catch (err) {
      errorToast(err.message || t('settings.adminOperations.actionCredentials.deleteFailed'));
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name'), slot: 'name' },
    { key: 'type', label: t('common.type'), slot: 'type' },
    { key: 'prefix', label: t('settings.adminOperations.actionCredentials.secret'), slot: 'prefix' },
    { key: 'scope', label: t('settings.adminOperations.actionCredentials.scope'), slot: 'scope' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'actions', label: '', slot: 'actions', align: 'right' },
  ]);

  function workspaceNames(ids) {
    if (!Array.isArray(ids) || ids.length === 0) return '';
    return ids
      .map((id) => workspaces.find((w) => w.id === id)?.name)
      .filter(Boolean)
      .join(', ');
  }

  function rowTestid(suffix, id) {
    return testids.rowPrefix ? `${testids.rowPrefix}-${suffix}-${id}` : undefined;
  }
</script>

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
      <div class="ml-6 mt-1">
        <WorkspaceCheckboxList
          {workspaces}
          selected={form.workspace_ids}
          onToggle={toggleWorkspaceScope}
          emptyMessage={t('settings.adminOperations.actionCredentials.noWorkspaces')}
        />
      </div>
    {/if}
  </div>
{/snippet}

{#if !canManage}
  <!-- Reached only by direct URL: the settings nav hides this module without
       action.credential.manage. -->
  <div
    class="rounded-xl p-6 border shadow-sm flex flex-col items-center text-center"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
    data-testid={testids.accessDenied}
  >
    <Shield class="w-12 h-12 mb-4" style="color: var(--ds-icon-warning, var(--ds-text-subtle));" />
    <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">
      {t(i18n.noPermissionTitle)}
    </h2>
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {t(i18n.noPermissionBody)}
    </p>
  </div>
{:else if loading}
  <StateDisplay type="loading" />
{:else}
  <div class="space-y-4">
    {#if isGlobal}
      <PageHeader title={t(i18n.title)} subtitle={t(i18n.subtitle)}>
        {#snippet actions()}
          <Button
            variant="primary"
            onclick={openCreate}
            icon={Plus}
            keyboardHint="A"
            hotkeyConfig={{ key: toHotkeyString('actionCredentials', 'add') }}
          >
            {t(i18n.add)}
          </Button>
        {/snippet}
      </PageHeader>
    {:else}
      <div class="flex items-center justify-between">
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t(i18n.description)}
        </p>
        <Button
          variant="primary"
          onclick={openCreate}
          icon={Plus}
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('actionCredentials', 'add') }}
          dataTestid={testids.add}
        >
          {t(i18n.add)}
        </Button>
      </div>
    {/if}

    {#if credentials.length === 0}
      <div
        class="flex flex-col items-center py-12 gap-3 rounded-lg border"
        style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
        data-testid={testids.empty}
      >
        <p class="text-sm" style="color: var(--ds-text-subtle);">{t(i18n.empty)}</p>
        <Button
          variant="secondary"
          onclick={openCreate}
          icon={Plus}
          keyboardHint="A"
          hotkeyConfig={{ key: toHotkeyString('actionCredentials', 'add') }}
        >
          {t(i18n.addFirst)}
        </Button>
      </div>
    {:else}
      <DataTable {columns} data={credentials} keyField="id">
        {#snippet name(cred)}
          <span
            class="font-medium"
            style="color: var(--ds-text);"
            data-testid={rowTestid('name', cred.id)}
            data-credential-id={testids.rowPrefix ? cred.id : undefined}
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
            <span class="text-xs italic" style="color: var(--ds-text-danger);">{t('settings.adminOperations.actionCredentials.noSecret')}</span>
          {/if}
        {/snippet}
        {#snippet scope(cred)}
          {#if isGlobal}
            {#if cred.applies_to_all_workspaces}
              <Lozenge appearance="success" size="sm">{t('settings.adminOperations.actionCredentials.allWorkspaces')}</Lozenge>
            {:else}
              <span class="text-xs" style="color: var(--ds-text-subtle);" title={workspaceNames(cred.workspace_ids)}>
                {t('settings.adminOperations.actionCredentials.workspaceCount', { count: (cred.workspace_ids || []).length })}
              </span>
            {/if}
          {:else if isWorkspaceOwned(cred)}
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
          {#if rowActionsAvailable(cred)}
            <EntityRowActions actions={[
              { id: 'rotate', icon: KeyRound, title: t('settings.adminOperations.actionCredentials.rotateSecret'), testId: rowTestid('rotate', cred.id), onclick: () => openRotate(cred) },
              { id: 'edit', icon: Edit, title: t('common.edit'), testId: rowTestid('edit', cred.id), onclick: () => openEdit(cred) },
              { id: 'delete', icon: Trash2, title: t('common.delete'), danger: true, testId: rowTestid('delete', cred.id), onclick: () => deleteCredential(cred) },
            ]} />
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

<!-- Create modal -->
{#if showCreateModal}
  <EntityFormModal
    title={t(i18n.add)}
    onclose={closeAndClearSecret}
    onsubmit={handleCreate}
    disabled={!form.name || !form.secret || (isGlobal && workspaceScopeInvalid())}
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
          dataTestid={testids.nameInput}
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
          dataTestid={testids.secretInput}
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
      {#if isGlobal}
        {@render scopeFields()}
      {:else}
        <p class="text-xs" style="color: var(--ds-text-subtle);">
          {t('workspaceSettings.actionCredentials.pinnedHelp')}
        </p>
      {/if}
    {/snippet}
  </EntityFormModal>
{/if}

<!-- Edit (metadata only) modal -->
{#if showEditModal && editing}
  <EntityFormModal
    title={t('settings.adminOperations.actionCredentials.edit')}
    onclose={closeAndClearSecret}
    onsubmit={handleUpdate}
    disabled={!form.name || (isGlobal && workspaceScopeInvalid())}
    {saving}
  >
    {#snippet fields()}
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('common.name')}</span>
        <Input type="text" class="mt-1" bind:value={form.name} required />
      </label>
      <div>
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.secret')}</span>
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          {t('settings.adminOperations.actionCredentials.storedRotate', { prefix: editing.secret_prefix || '••••••••' })}
        </p>
      </div>
      <label class="block">
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.metadata')}</span>
        <Textarea class="mt-1 font-mono" rows={3} bind:value={form.secret_metadata} />
      </label>
      <Checkbox bind:checked={form.is_enabled} label={t('common.enabled')} />
      {#if isGlobal}
        {@render scopeFields()}
      {/if}
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
        <span class="text-sm font-medium" style="color: var(--ds-text);">{t('settings.adminOperations.actionCredentials.newSecret')}</span>
        <Input
          type="password"
          autocomplete="new-password"
          class="mt-1 font-mono"
          bind:value={form.secret}
          dataTestid={testids.rotateInput}
          required
        />
      </label>
    {/snippet}
  </EntityFormModal>
{/if}
