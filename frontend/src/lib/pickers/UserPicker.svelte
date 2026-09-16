<script>
  import { ChevronDown, X } from '@lucide/svelte';
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onDestroy, onMount } from 'svelte';
  import Avatar from '../components/Avatar.svelte';
  import Text from '../components/Text.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    placeholder = '',
    ariaLabel = undefined,
    showUnassigned = false,
    unassignedLabel = '',
    disabled = false,
    class: className = '',
    showSelectedInTrigger = true,
    children: customTrigger = null,  // Optional custom trigger from caller
    workspaceId = null,
    users = null,
    loading = false,
    label = '',
    autoOpen = false,
    allowClear = true,
    positioning = null,
    onOpen = null,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectUser'));
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('pickers.unassigned'));

  // Load users — use assignable-users endpoint when workspaceId is provided
  const loader = createAsyncLoader(() =>
    workspaceId ? api.getAssignableUsers(workspaceId) : api.getUsers()
  );
  onMount(() => { if (!users) loader.load(); });
  onDestroy(() => loader.dispose());

  let usersList = $derived(users ?? loader.data ?? []);

  // Selected user lookup
  let selectedUser = $derived(
    usersList.find(u => u.id === value) || null
  );

  // Agent presence (WI-272): the assignable-users endpoint decorates agent
  // users with whether anything would actually pick up an assigned item.
  function presenceMeta(presence) {
    switch (presence) {
      case 'online': return { color: '#22c55e', label: t('pickers.agentOnline') };
      case 'local': return { color: '#22c55e', label: t('pickers.agentLocal') };
      case 'offline': return { color: '#ef4444', label: t('pickers.agentOffline') };
      default: return { color: '#9ca3af', label: t('pickers.agentUnbound') };
    }
  }

  // Helper: build user display label
  function getUserLabel(user) {
    if (!user) return '';
    return `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.email || user.username || '';
  }

  function handleSelect(user) {
    onSelect(user);
  }

  function handleCancel() {
    onCancel();
  }
</script>

<BasePicker
  bind:value
  items={usersList}
  loading={users === null ? loader.loading : loading}
  placeholder={resolvedPlaceholder}
  {ariaLabel}
  {showUnassigned}
  unassignedLabel={resolvedUnassignedLabel}
  {disabled}
  allowClear={true}
  {autoOpen}
  {positioning}
  {showSelectedInTrigger}
  {label}
  class={className}
  searchFields={['first_name', 'last_name', 'email', 'username']}
  getValue={(user) => user?.id}
  getLabel={getUserLabel}
  searchTestid="user-picker-search"
  optionTestid={(opt) => `user-picker-option-${opt.value}`}
  onOpen={() => onOpen?.()}
  onSelect={handleSelect}
  onCancel={handleCancel}
>
  {#snippet children()}
    {#if customTrigger}
      {@render customTrigger()}
    {:else}
      <div
        aria-disabled={disabled}
        class="relative w-full flex items-center justify-between gap-2 px-3 py-2 rounded text-sm transition-colors"
        style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        style:opacity={disabled ? 0.5 : 1}
        style:cursor={disabled ? 'not-allowed' : 'pointer'}
        data-testid="user-picker-trigger"
      >
        <div class="flex items-center gap-2 flex-1 min-w-0">
          {#if selectedUser && showSelectedInTrigger}
            <Avatar src={selectedUser.avatar_url} firstName={selectedUser.first_name} lastName={selectedUser.last_name} size="xs" />
            <span class="truncate">{selectedUser.first_name} {selectedUser.last_name}</span>
          {:else}
            <span style="color: var(--ds-text-subtle);">{resolvedPlaceholder}</span>
          {/if}
        </div>
        <div class="flex items-center gap-1 flex-shrink-0">
          {#if allowClear && value != null && !disabled && showSelectedInTrigger}
            <button type="button" onclick={(e) => { e.stopPropagation(); onSelect(null); }} class="p-0.5 rounded hover:bg-opacity-10" style="color: var(--ds-text-subtle);" aria-label={t('pickers.clearSelection')}>
              <X size={14} />
            </button>
          {/if}
          <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
        </div>
      </div>
    {/if}
  {/snippet}

  {#snippet itemSnippet({ item: user, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <Avatar src={user.avatar_url} firstName={user.first_name} lastName={user.last_name} size="sm" />
      <div class="flex flex-col min-w-0 flex-1">
        <span class="font-medium truncate flex items-center gap-1.5" style="color: var(--ds-text);">
          {user.first_name} {user.last_name}
          {#if user.agent_presence}
            {@const presence = presenceMeta(user.agent_presence)}
            <span class="inline-block w-2 h-2 rounded-full shrink-0" style="background-color: {presence.color};" title={presence.label} data-testid={`agent-presence-${user.id}`}></span>
          {/if}
        </span>
        <Text size="xs" variant="subtle" truncate>{user.email}</Text>
      </div>
    </div>
  {/snippet}
</BasePicker>
