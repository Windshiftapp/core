<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { useEventListener } from 'runed';
  import Avatar from '../components/Avatar.svelte';
  import Text from '../components/Text.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Generate unique IDs for ARIA attributes
  const listboxId = `mention-listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  // Props using Svelte 5 $props()
  let {
    query = '',
    position = { x: 0, y: 0 },
    open = false,
    isPersonalWorkspace = false,
    onSelect = null,
    onCancel = null
  } = $props();

  // State
  let users = $state([]);
  let loading = $state(false);
  let highlightedIndex = $state(0);
  let containerElement = $state(null);

  // Load users on mount
  onMount(async () => {
    await loadUsers();
  });

  // Handle keyboard events using runed
  useEventListener(
    () => document,
    'keydown',
    handleKeyDown
  );

  async function loadUsers() {
    if (loading) return;
    try {
      loading = true;
      users = await api.getUsers() || [];
    } catch (err) {
      console.error('Failed to load users:', err);
      users = [];
    } finally {
      loading = false;
    }
  }

  // Filter users based on query
  let filteredUsers = $derived.by(() => {
    if (!query.trim()) {
      return users.slice(0, 10);
    }
    const search = query.toLowerCase();
    return users.filter(user =>
      user.first_name?.toLowerCase().includes(search) ||
      user.last_name?.toLowerCase().includes(search) ||
      user.username?.toLowerCase().includes(search) ||
      user.email?.toLowerCase().includes(search)
    ).slice(0, 10);
  });

  // Reset highlight when query changes
  $effect(() => {
    query; // Track query changes
    highlightedIndex = 0;
  });

  function handleSelect(user) {
    onSelect?.(user);
  }

  function handleKeyDown(e) {
    if (!open || filteredUsers.length === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      highlightedIndex = (highlightedIndex + 1) % filteredUsers.length;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      highlightedIndex = highlightedIndex === 0 ? filteredUsers.length - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      if (filteredUsers[highlightedIndex]) {
        e.preventDefault();
        e.stopPropagation();
        handleSelect(filteredUsers[highlightedIndex]);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onCancel?.();
    }
  }
</script>

{#if open}
  <div
    bind:this={containerElement}
    class="mention-picker"
    style="top: {position.y}px; left: {position.x}px;"
    role="listbox"
    id={listboxId}
    aria-label={t('pickers.mentionUsers')}
  >
    {#if loading}
      <div class="loading">{t('pickers.searching')}</div>
    {:else if filteredUsers.length === 0}
      <div class="no-results">{t('pickers.noUsersFound')}</div>
    {:else}
      {#each filteredUsers as user, index}
        <button
          type="button"
          class="mention-option"
          class:highlighted={index === highlightedIndex}
          onclick={() => handleSelect(user)}
          onmouseenter={() => highlightedIndex = index}
          role="option"
          id={getOptionId(index)}
          aria-selected={index === highlightedIndex}
        >
          <Avatar
            src={user.avatar_url}
            firstName={user.first_name}
            lastName={user.last_name}
            size="sm"
            variant="blue"
          />
          <div class="info">
            <Text size="sm" weight="medium">{user.first_name} {user.last_name}</Text>
            <Text size="xs" variant="subtle">@{user.username}</Text>
          </div>
        </button>
      {/each}
    {/if}
    {#if isPersonalWorkspace}
      <div class="personal-warning">
        {t('pickers.noNotificationPersonalTask')}
      </div>
    {/if}
  </div>
{/if}

<style>
  .mention-picker {
    position: fixed;
    z-index: 1000;
    background: var(--ds-surface-raised, white);
    border: 1px solid var(--ds-border, rgba(0, 0, 0, 0.12));
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    min-width: 240px;
    max-width: 320px;
    max-height: 300px;
    overflow-y: auto;
  }

  .loading, .no-results {
    padding: 12px 16px;
    color: var(--ds-text-subtle, #6b7280);
    font-size: 14px;
  }

  .mention-option {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px 12px;
    border: none;
    background: transparent;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
    font-family: inherit;
  }

  .mention-option:hover,
  .mention-option.highlighted {
    background: var(--ds-background-neutral-hovered, rgba(59, 130, 246, 0.08));
  }

  .info {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .personal-warning {
    padding: 8px 12px;
    font-size: 12px;
    color: #92400e;
    background: #fef3c7;
    border-top: 1px solid #fcd34d;
  }
</style>
