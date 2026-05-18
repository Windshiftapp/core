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
  import { onMount } from 'svelte';
  import Select from './Select.svelte';
  import { actionCredentials } from '$lib/api/ai.js';

  let {
    workspaceId,
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

  async function loadCredentials() {
    loading = true;
    loadError = '';
    try {
      const data = await actionCredentials.getForWorkspace(workspaceId);
      credentials = Array.isArray(data) ? data : [];
    } catch (err) {
      loadError = err?.message || 'Failed to load credentials';
      credentials = [];
    } finally {
      loading = false;
    }
  }

  onMount(loadCredentials);

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
    const scope = c.workspace_id ? 'workspace' : 'global';
    const fp = c.has_secret && c.secret_prefix ? ` · ${c.secret_prefix}` : '';
    const status = c.is_enabled ? '' : ' · disabled';
    return `${c.name} (${c.credential_type}, ${scope}${fp}${status})`;
  }
</script>

{#if loading}
  <div class="text-sm text-zinc-500 dark:text-zinc-400">Loading credentials…</div>
{:else if loadError}
  <div class="text-sm text-rose-600 dark:text-rose-400">{loadError}</div>
{:else if !options.length}
  <div class="text-sm text-zinc-500 dark:text-zinc-400">
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
