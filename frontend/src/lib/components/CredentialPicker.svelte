<!--
  CredentialPicker
  ----------------
  A read-only chooser for action credentials. Loads the workspace-aware list
  (rows scoped to the workspace plus globals) and renders them as Select
  options.

  Props:
    workspaceId  required — credentials are listed for this workspace's scope
    value        bound — selected credential ID, or 0 / null when none
    placeholder  optional Select placeholder
    types        optional array of credential_type values to restrict the
                 list (e.g. ['bearer_token','api_key']); defaults to all

  The picker never sees or asks for plaintext — secret values are managed in
  the credential manager only. has_secret is shown as a visual hint after
  the name so the admin can spot rows that haven't had a secret stored yet.
-->
<script>
  import Select from './Select.svelte';
  import { actionCredentials } from '../api/ai.js';

  let {
    // workspaceId: when 0 or omitted, the picker loads global credentials
    // only — appropriate for global capability configs.
    workspaceId = 0,
    value = $bindable(0),
    placeholder = 'Select a credential…',
    types = null,
    disabled = false,
    id = undefined,
    class: className = '',
    onchange = undefined,
  } = $props();

  let credentials = $state([]);
  let loading = $state(true);
  let loadError = $state('');
  let loadGeneration = 0;

  async function loadCredentials(requestedWorkspaceId) {
    const generation = ++loadGeneration;
    loading = true;
    loadError = '';
    try {
      const data = requestedWorkspaceId
        ? await actionCredentials.getForWorkspace(requestedWorkspaceId)
        : await actionCredentials.getAllGlobal();
      if (generation !== loadGeneration) return;
      credentials = Array.isArray(data) ? data : [];
    } catch (err) {
      if (generation !== loadGeneration) return;
      loadError = err?.message || 'Failed to load credentials';
      credentials = [];
    } finally {
      if (generation === loadGeneration) loading = false;
    }
  }

  $effect(() => {
    loadCredentials(Number(workspaceId) || 0);
  });

  const filtered = $derived(
    types && types.length
      ? credentials.filter((c) => types.includes(c.credential_type))
      : credentials
  );

  const options = $derived(
    filtered.map((c) => ({
      value: c.id,
      label: formatLabel(c),
    }))
  );

  function formatLabel(c) {
    const scope = c.applies_to_all_workspaces ? 'global' : 'workspace';
    const fp = c.has_secret && c.secret_prefix ? ` · ${c.secret_prefix}` : '';
    const status = c.is_enabled ? '' : ' · disabled';
    return `${c.name} (${c.credential_type}, ${scope}${fp}${status})`;
  }
</script>

{#if loading}
  <div class="text-sm text-ds-text-subtle">Loading credentials…</div>
{:else if loadError}
  <div class="text-sm text-ds-text-danger">{loadError}</div>
{:else if !options.length}
  <div class="text-sm text-ds-text-subtle">
    No credentials available. Create one in <span class="font-medium">Action credentials</span>.
  </div>
{:else}
  <Select
    {id}
    class={className}
    bind:value
    options={options}
    placeholder={placeholder}
    disabled={disabled}
    {onchange}
  />
{/if}
