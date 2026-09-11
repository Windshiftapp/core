<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Plus, Edit2, Trash2, Loader2, PlugZap, RefreshCw } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import NativeSelect from '../components/NativeSelect.svelte';
  import FormField from '../components/FormField.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import SectionHeader from '../layout/SectionHeader.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { successToast, errorToast, warningToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';

  let connections = $state([]);
  let workspaces = $state([]);
  let statuses = $state([]);
  let metadataByConnection = $state({});
  let loading = $state(true);
  let saving = $state(false);
  let testingId = $state(null);
  let authorizingId = $state(null);
  let refreshingAll = $state(false);
  let error = $state('');
  let showModal = $state(false);
  let editing = $state(null);
  let form = $state(emptyForm());

  function emptyForm() {
    return {
      slug: '',
      name: '',
      enabled: true,
      base_url: '',
      auth_method: 'api_token',
      api_token: '',
      oauth_client_id: '',
      oauth_client_secret: '',
      has_oauth_client_secret: false,
      default_group_id: '',
      default_group_name: '',
      allowed_groups: [],
      default_customer: '',
      correlation_field: 'windshift_item_key',
      closed_state_ids: [],
      completion_status_id: '',
      applies_to_all_workspaces: false,
      workspace_ids: [],
    };
  }

  onMount(() => {
    handleOAuthCallback();
    load();
  });

  function handleOAuthCallback() {
    const result = new URLSearchParams(window.location.search).get('oauth');
    if (!result) return;

    if (result === 'success') {
      successToast(t('zammad.oauthConnected'));
    } else if (result === 'error') {
      errorToast(t('zammad.oauthConnectionFailed'));
    }
    const url = new URL(window.location.href);
    url.searchParams.delete('oauth');
    url.searchParams.set('tab', 'zammad');
    window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);
  }

  async function load() {
    loading = true;
    error = '';
    try {
      [connections, workspaces, statuses] = await Promise.all([
        api.zammadConnections.getAll(),
        api.workspaces.getAll(),
        api.statuses.getAll(),
      ]);
    } catch (err) {
      console.error('Failed to load Zammad settings:', err);
      error = t('zammad.loadConnectionsFailed');
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    editing = null;
    form = emptyForm();
    showModal = true;
  }

  function openEdit(connection) {
    editing = connection;
    form = {
      slug: connection.slug,
      name: connection.name,
      enabled: connection.enabled,
      base_url: connection.base_url,
      auth_method: connection.auth_method || 'api_token',
      api_token: '',
      oauth_client_id: connection.oauth_client_id || '',
      oauth_client_secret: '',
      has_oauth_client_secret: connection.has_oauth_client_secret === true,
      default_group_id: connection.default_group_id || '',
      default_group_name: connection.default_group_name || '',
      allowed_groups: connection.allowed_groups || [],
      default_customer: connection.default_customer || '',
      correlation_field: connection.correlation_field || 'windshift_item_key',
      closed_state_ids: connection.closed_state_ids || [],
      completion_status_id: connection.completion_status_id || '',
      applies_to_all_workspaces: connection.applies_to_all_workspaces,
      workspace_ids: connection.workspace_ids || [],
    };
    showModal = true;
  }

  async function save() {
    saving = true;
    try {
      const data = {
        ...form,
        default_group_id: Number(form.default_group_id) || 0,
        completion_status_id: Number(form.completion_status_id) || undefined,
        clear_completion_status: false,
      };
      if (data.allowed_groups.length === 0 && data.default_group_id && data.default_group_name.trim()) {
        data.allowed_groups = [{ id: data.default_group_id, name: data.default_group_name.trim() }];
      }
      delete data.has_oauth_client_secret;
      if (data.auth_method === 'oauth') {
        delete data.api_token;
        if (!data.oauth_client_secret) delete data.oauth_client_secret;
      } else {
        delete data.oauth_client_id;
        delete data.oauth_client_secret;
      }
      if (!data.api_token) delete data.api_token;
      if (!data.completion_status_id) {
        delete data.completion_status_id;
        if (editing) data.clear_completion_status = true;
      }
      if (editing) {
        await api.zammadConnections.update(editing.id, data);
        successToast(t('zammad.connectionUpdated'));
      } else {
        await api.zammadConnections.create(data);
        successToast(t('zammad.connectionCreated'));
      }
      showModal = false;
      await load();
    } catch (err) {
      console.error('Failed to save Zammad connection:', err);
      errorToast(err.message || t('zammad.saveConnectionFailed'));
    } finally {
      saving = false;
    }
  }

  async function testConnection(connection) {
    testingId = connection.id;
    try {
      const result = await api.zammadConnections.test(connection.id);
      metadataByConnection = { ...metadataByConnection, [connection.id]: result.metadata };
      successToast(t('zammad.connectionTestSucceeded'));
      if (result.metadata?.group_catalog_verified === false) {
        warningToast(t('zammad.groupCatalogNotVerified'));
      }
      if (result.metadata?.correlation_field_verified === false) {
        warningToast(t('zammad.correlationFieldNotVerified'));
      }
    } catch (err) {
      console.error('Zammad connection test failed:', err);
      errorToast(t('zammad.connectionTestFailed'));
    } finally {
      testingId = null;
    }
  }

  async function refreshAllTickets() {
    refreshingAll = true;
    try {
      const result = await api.zammadConnections.refreshAllTickets();
      if (result.started) successToast(t('zammad.refreshAllTicketsStarted'));
      else warningToast(t('zammad.refreshAllTicketsAlreadyRunning'));
    } catch (err) {
      console.error('Failed to refresh all Zammad tickets:', err);
      errorToast(t('zammad.refreshAllTicketsFailed'));
    } finally {
      refreshingAll = false;
    }
  }

  async function remove(connection) {
    const accepted = await confirm({
      title: t('zammad.deleteConnection'),
      message: t('zammad.deleteConnectionConfirm', { name: connection.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!accepted) return;
    try {
      await api.zammadConnections.delete(connection.id);
      successToast(t('zammad.connectionDeleted'));
      await load();
    } catch (err) {
      console.error('Failed to delete Zammad connection:', err);
      errorToast(t('zammad.deleteConnectionFailed'));
    }
  }

  async function startOAuth(connection) {
    authorizingId = connection.id;
    try {
      const result = await api.zammadConnections.startOAuth(connection.id);
      window.location.href = result.auth_url;
    } catch (err) {
      console.error('Failed to start Zammad OAuth authorization:', err);
      errorToast(t('zammad.oauthAuthorizationFailed'));
      authorizingId = null;
    }
  }

  function authStatus(connection) {
    if (connection.auth_method !== 'oauth') {
      return {
        appearance: connection.has_api_token ? 'success' : 'warning',
        label: connection.has_api_token ? t('zammad.apiTokenAvailable') : t('zammad.apiTokenMissing'),
      };
    }
    if (connection.reauthorization_required) {
      return { appearance: 'warning', label: t('zammad.oauthReauthorizationRequired') };
    }
    if (connection.oauth_connected) {
      return { appearance: 'success', label: t('zammad.oauthConnectedStatus') };
    }
    return { appearance: 'default', label: t('zammad.oauthPending') };
  }

  function oauthAuthorizationRequired(connection) {
    return connection.auth_method === 'oauth' &&
      (!connection.oauth_connected || connection.reauthorization_required);
  }

  function toggleWorkspace(id, checked) {
    form.workspace_ids = checked
      ? [...new Set([...form.workspace_ids, id])]
      : form.workspace_ids.filter((workspaceId) => workspaceId !== id);
  }

  function toggleClosedState(id, checked) {
    form.closed_state_ids = checked
      ? [...new Set([...form.closed_state_ids, id])]
      : form.closed_state_ids.filter((stateId) => stateId !== id);
  }

  function toggleAllowedGroup(id, checked) {
    const group = metadataByConnection[editing?.id]?.groups?.find((entry) => entry.id === id);
    form.allowed_groups = checked
      ? [...form.allowed_groups.filter((entry) => entry.id !== id), { id, name: group?.name || '' }]
      : form.allowed_groups.filter((entry) => entry.id !== id);
  }

  function setAllowedGroupName(id, name) {
    form.allowed_groups = form.allowed_groups.map((entry) =>
      entry.id === id ? { ...entry, name } : entry,
    );
    if (Number(form.default_group_id) === id) form.default_group_name = name;
  }

  function selectDefaultGroup(value) {
    form.default_group_id = value;
    const group = metadataByConnection[editing?.id]?.groups?.find(
      (entry) => entry.id === Number(form.default_group_id),
    );
    form.default_group_name = group?.name || '';
  }

  function hasKnownGroupName(group) {
    return Boolean(group?.name?.trim());
  }

  function groupLabel(group) {
    return hasKnownGroupName(group)
      ? group.name.trim()
      : t('zammad.unverifiedGroup', { id: group.id });
  }

  function serializeAllowedGroups(groups) {
    return groups.map((group) => `${group.id}:${group.name}`).join(', ');
  }

  function parseAllowedGroups(value) {
    return value.split(',').map((entry) => {
      const [rawId, ...nameParts] = entry.split(':');
      return { id: Number(rawId.trim()), name: nameParts.join(':').trim() };
    }).filter((group) => group.id > 0 && group.name);
  }
</script>

<div>
  <SectionHeader title={t('zammad.connections')} subtitle={t('zammad.connectionsDescription')} class="mb-6">
    {#snippet actions()}
      <div class="flex items-center gap-2">
        <Button variant="ghost" size="small" onclick={refreshAllTickets} disabled={refreshingAll || connections.length === 0}>
          {#if refreshingAll}<Loader2 class="w-4 h-4 animate-spin" />{:else}<RefreshCw class="w-4 h-4" />{/if}
          {t('zammad.refreshAllTickets')}
        </Button>
        <!-- shortcut-guard-exempt: this secondary integration tab does not own a global add shortcut -->
        <Button variant="primary" size="small" icon={Plus} onclick={openCreate}>
          {t('zammad.addConnection')}
        </Button>
      </div>
    {/snippet}
  </SectionHeader>

  {#if error}<div class="mb-4"><AlertBox message={error} /></div>{/if}

  {#if loading}
    <div class="flex justify-center py-12"><Loader2 class="w-6 h-6 animate-spin" /></div>
  {:else if connections.length === 0}
    <EmptyState title={t('zammad.noConnections')} />
  {:else}
    <div class="space-y-3">
      {#each connections as connection}
        {@const connectionAuthStatus = authStatus(connection)}
        <div class="border rounded-lg p-4" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
          <div class="flex items-center gap-4">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-medium">{connection.name}</h3>
                <Lozenge appearance={connection.enabled ? 'success' : 'default'}>
                  {connection.enabled ? t('common.enabled') : t('common.disabled')}
                </Lozenge>
                <Lozenge appearance={connectionAuthStatus.appearance}>{connectionAuthStatus.label}</Lozenge>
              </div>
              <p class="text-xs mt-1 truncate" style="color: var(--ds-text-subtle);">{connection.base_url}</p>
              {#if connection.last_test_error}
                <p class="text-xs mt-1" style="color: var(--ds-text-danger);">{connection.last_test_error}</p>
              {/if}
            </div>
            <div class="flex items-center gap-1">
              <Button variant="ghost" size="small" onclick={() => testConnection(connection)} disabled={testingId === connection.id || oauthAuthorizationRequired(connection)}>
                {#if testingId === connection.id}<Loader2 class="w-4 h-4 animate-spin" />{:else}<PlugZap class="w-4 h-4" />{/if}
                {t('zammad.testConnection')}
              </Button>
              {#if connection.auth_method === 'oauth'}
                <Button variant="ghost" size="small" onclick={() => startOAuth(connection)} disabled={authorizingId === connection.id}>
                  {#if authorizingId === connection.id}<Loader2 class="w-4 h-4 animate-spin" />{/if}
                  {connection.oauth_connected || connection.reauthorization_required ? t('zammad.reauthorizeOAuth') : t('zammad.connectOAuth')}
                </Button>
              {/if}
              <Button variant="ghost" size="small" onclick={() => openEdit(connection)}><Edit2 class="w-4 h-4" /></Button>
              <Button variant="danger-ghost" size="small" onclick={() => remove(connection)}><Trash2 class="w-4 h-4" /></Button>
            </div>
          </div>
          {#if metadataByConnection[connection.id]}
            <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
              {t('zammad.metadataSummary', {
                groups: metadataByConnection[connection.id].groups.length,
                states: metadataByConnection[connection.id].states.length,
              })}
            </p>
            {#if metadataByConnection[connection.id].correlation_field_verified === false}
              <p class="text-xs mt-1" style="color: var(--ds-text-warning);">{t('zammad.correlationFieldNotVerified')}</p>
            {/if}
            {#if metadataByConnection[connection.id].group_catalog_verified === false}
              <p class="text-xs mt-1" style="color: var(--ds-text-warning);">{t('zammad.groupCatalogNotVerified')}</p>
            {/if}
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<Modal bind:isOpen={showModal}>
  <ModalHeader title={editing ? t('zammad.editConnection') : t('zammad.addConnection')} onclose={() => (showModal = false)} />
  <form onsubmit={(event) => { event.preventDefault(); save(); }} class="p-4 space-y-4 max-h-[75vh] overflow-y-auto">
    <FormField label={t('common.name')} required><Input bind:value={form.name} /></FormField>
    <FormField label={t('zammad.slug')} required><Input bind:value={form.slug} placeholder="zammad-main" /></FormField>
    <FormField label={t('zammad.baseUrl')} required><Input bind:value={form.base_url} placeholder="https://support.example.com" /></FormField>
    {#if editing}
      <FormField label={t('zammad.authMethod')}>
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {form.auth_method === 'oauth' ? t('zammad.oauth') : t('zammad.apiToken')}
        </p>
      </FormField>
    {:else}
      <FormField label={t('zammad.authMethod')} required>
        <div class="grid grid-cols-2 gap-3">
          <button
            type="button"
            onclick={() => form.auth_method = 'api_token'}
            class="p-3 rounded border-2 text-left transition-all"
            style={form.auth_method === 'api_token'
              ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
              : 'border-color: var(--ds-border);'}
          >
            <div class="font-medium" style="color: var(--ds-text);">{t('zammad.apiToken')}</div>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('zammad.apiTokenDescription')}</p>
          </button>
          <button
            type="button"
            onclick={() => form.auth_method = 'oauth'}
            class="p-3 rounded border-2 text-left transition-all"
            style={form.auth_method === 'oauth'
              ? 'border-color: var(--ds-border-focused); background: var(--ds-surface-selected);'
              : 'border-color: var(--ds-border);'}
          >
            <div class="font-medium" style="color: var(--ds-text);">{t('zammad.oauth')}</div>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('zammad.oauthDescription')}</p>
          </button>
        </div>
      </FormField>
    {/if}
    {#if form.auth_method === 'oauth'}
      <FormField label={t('zammad.oauthClientId')} required>
        <Input bind:value={form.oauth_client_id} />
      </FormField>
      <FormField label={t('zammad.oauthClientSecret')} required={!editing || !form.has_oauth_client_secret}>
        <Input
          bind:value={form.oauth_client_secret}
          type="password"
          placeholder={form.has_oauth_client_secret ? t('zammad.secretStored') : t('zammad.oauthClientSecret')}
        />
      </FormField>
    {:else}
      <FormField label={t('zammad.apiToken')} required={!editing}>
        <Input bind:value={form.api_token} type="password" placeholder={editing ? t('zammad.secretStored') : t('zammad.apiToken')} />
      </FormField>
    {/if}
    <FormField label={t('zammad.defaultCustomer')} required>
      <Input bind:value={form.default_customer} placeholder="windshift@example.com" />
    </FormField>
    {#if editing && metadataByConnection[editing.id]?.groups?.length}
      <FormField label={t('zammad.defaultGroup')} required>
        <NativeSelect
          value={String(form.default_group_id)}
          onchange={selectDefaultGroup}
          options={metadataByConnection[editing.id].groups
            .filter((group) => hasKnownGroupName(group) || group.id === Number(form.default_group_id))
            .map((group) => ({ value: String(group.id), label: groupLabel(group) }))}
        />
      </FormField>
      <FormField label={t('zammad.allowedGroups')} required>
        <div class="space-y-2 max-h-40 overflow-y-auto border rounded p-3" style="border-color: var(--ds-border);">
          {#each metadataByConnection[editing.id].groups as group}
            <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(10rem,1fr)] sm:items-center">
              <Checkbox
                id={`zammad-group-${group.id}`}
                checked={form.allowed_groups.some((entry) => entry.id === group.id)}
                onchange={(checked) => toggleAllowedGroup(group.id, checked)}
                label={groupLabel(group)}
                size="small"
              />
              {#if !hasKnownGroupName(group) && form.allowed_groups.some((entry) => entry.id === group.id)}
                <Input
                  value={form.allowed_groups.find((entry) => entry.id === group.id)?.name || ''}
                  oninput={(event) => setAllowedGroupName(group.id, event.currentTarget.value)}
                  placeholder={t('settings.groups.groupName')}
                  ariaLabel={t('settings.groups.groupName')}
                  size="small"
                />
              {/if}
            </div>
          {/each}
        </div>
      </FormField>
    {:else}
      <FormField label={t('zammad.defaultGroup')} required>
        <div class="grid grid-cols-[8rem_1fr] gap-2">
          <Input bind:value={form.default_group_id} type="number" placeholder="7" />
          <Input bind:value={form.default_group_name} placeholder="Support" />
        </div>
      </FormField>
      <FormField label={t('zammad.allowedGroupIds')}>
        <Input
          value={serializeAllowedGroups(form.allowed_groups)}
          oninput={(event) => (form.allowed_groups = parseAllowedGroups(event.currentTarget.value))}
          placeholder="7:Support, 8:Escalations"
        />
      </FormField>
    {/if}
    <FormField label={t('zammad.correlationField')} required>
      <Input bind:value={form.correlation_field} />
      <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('zammad.correlationFieldHint')}</p>
    </FormField>
    <FormField label={t('zammad.completionStatus')}>
      <NativeSelect bind:value={form.completion_status_id} options={[
        { value: '', label: t('zammad.noAutomaticCompletion') },
        ...statuses.map((status) => ({ value: status.id, label: status.name })),
      ]} size="small" />
    </FormField>

    {#if editing && metadataByConnection[editing.id]?.states?.length}
      <FormField label={t('zammad.closedStates')}>
        <div class="space-y-2">
          {#each metadataByConnection[editing.id].states as state}
            <Checkbox
              id={`zammad-state-${state.id}`}
              checked={form.closed_state_ids.includes(state.id)}
              onchange={(checked) => toggleClosedState(state.id, checked)}
              label={state.name}
              size="small"
            />
          {/each}
        </div>
      </FormField>
    {:else}
      <FormField label={t('zammad.closedStateIds')}>
        <Input
          value={form.closed_state_ids.join(',')}
          oninput={(event) => (form.closed_state_ids = event.currentTarget.value.split(',').map(Number).filter(Boolean))}
          placeholder="4"
        />
      </FormField>
    {/if}

    <Checkbox id="zammad-global" bind:checked={form.applies_to_all_workspaces} label={t('zammad.allWorkspaces')} size="small" />
    {#if !form.applies_to_all_workspaces}
      <FormField label={t('zammad.allowedWorkspaces')} required>
        <div class="space-y-2 max-h-40 overflow-y-auto border rounded p-3" style="border-color: var(--ds-border);">
          {#each workspaces as workspace}
            <Checkbox
              id={`zammad-workspace-${workspace.id}`}
              checked={form.workspace_ids.includes(workspace.id)}
              onchange={(checked) => toggleWorkspace(workspace.id, checked)}
              label={`${workspace.key} - ${workspace.name}`}
              size="small"
            />
          {/each}
        </div>
      </FormField>
    {/if}
    <Checkbox id="zammad-enabled" bind:checked={form.enabled} label={t('common.enabled')} size="small" />

    <div class="flex justify-end gap-2 pt-2">
      <Button variant="ghost" onclick={() => (showModal = false)}>{t('common.cancel')}</Button>
      <Button variant="primary" type="submit" disabled={saving || !form.name || !form.slug || !form.base_url || !form.default_customer || (form.auth_method === 'oauth' ? (!form.oauth_client_id || (!editing && !form.oauth_client_secret) || (editing && !form.has_oauth_client_secret && !form.oauth_client_secret)) : (!editing && !form.api_token)) || (!form.applies_to_all_workspaces && form.workspace_ids.length === 0)}>
        {#if saving}<Loader2 class="w-4 h-4 animate-spin" />{/if}
        {editing ? t('common.update') : t('common.create')}
      </Button>
    </div>
  </form>
</Modal>
