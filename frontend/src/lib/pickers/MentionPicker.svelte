<script>
  import { untrack } from 'svelte';
  import { getAssignableUsers, getUsers } from '../api/users.js';
  import { useEventListener } from 'runed';
  import Avatar from '../components/Avatar.svelte';
  import Text from '../components/Text.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { mentionTextFor } from '../editors/mentionResolver.js';

  // Generate unique IDs for ARIA attributes
  const listboxId = `mention-listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  // Props using Svelte 5 $props()
  let {
    query = '',
    position = { x: 0, y: 0 },
    open = $bindable(false),
    workspaceId = null,
    isPersonalWorkspace = false,
    onSelect = null,
    onCancel = null
  } = $props();

  // State
  let users = $state([]);
  let loading = $state(false);
  let loadedWorkspaceId = $state(undefined);
  let loadRequest = 0;
  let highlightedIndex = $state(0);
  let containerElement = $state(null);
  // Measured menu size, used to keep the picker fully on-screen.
  let menuWidth = $state(0);
  let menuHeight = $state(0);

  // Editors keep the picker mounted while it is closed. Defer the user
  // catalog until somebody actually types an @ mention; pages with several
  // editors otherwise issue one identical roster request per editor.
  $effect(() => {
    const shouldLoad = open;
    const requestedWorkspaceId = workspaceId;
    if (shouldLoad) untrack(() => void loadUsers(requestedWorkspaceId));
  });

  // Re-measure after result changes so the transient menu clamp stays accurate.
  $effect(() => {
    if (!open || !containerElement) return;
    // Touch the reactive deps that change the menu's size.
    filteredUsers;
    loading;
    isPersonalWorkspace;
    menuWidth = containerElement.offsetWidth;
    menuHeight = containerElement.offsetHeight;
  });

  // Clamp against visualViewport so narrow screens and mobile keyboards cannot
  // place the fixed menu outside the visible viewport. Recompute while typing;
  // no resize listener is needed.
  const EDGE_MARGIN = 8;
  const FALLBACK_W = 320; // .mention-picker max-width, before it is measured
  const FALLBACK_H = 300; // .mention-picker max-height, before it is measured
  let clampedPosition = $derived.by(() => {
    const win = typeof window !== 'undefined' ? window : null;
    const vv = win?.visualViewport ?? null;
    const vw = vv?.width ?? win?.innerWidth ?? 0;
    const vh = vv?.height ?? win?.innerHeight ?? 0;
    const offX = vv?.offsetLeft ?? 0;
    const offY = vv?.offsetTop ?? 0;
    const w = menuWidth || FALLBACK_W;
    const h = menuHeight || FALLBACK_H;
    const minX = offX + EDGE_MARGIN;
    const minY = offY + EDGE_MARGIN;
    const maxX = Math.max(minX, offX + vw - w - EDGE_MARGIN);
    const maxY = Math.max(minY, offY + vh - h - EDGE_MARGIN);
    return {
      x: Math.min(Math.max(minX, position.x), maxX),
      y: Math.min(Math.max(minY, position.y), maxY),
    };
  });

  // Capture before ProseMirror handles arrows or Enter and invalidates the
  // active @-pattern.
  useEventListener(
    () => document,
    'keydown',
    handleKeyDown,
    { capture: true }
  );

  async function loadUsers(requestedWorkspaceId) {
    const scopedWorkspaceId = requestedWorkspaceId || null;
    if (users.length > 0 && loadedWorkspaceId === scopedWorkspaceId) return;

    const request = ++loadRequest;
    try {
      loading = true;
      const roster = scopedWorkspaceId
        ? await getAssignableUsers(scopedWorkspaceId)
        : await getUsers();
      if (request !== loadRequest) return;

      // The backend roster is shared with assignment and already enforces
      // workspace access plus ready agent bindings.
      users = roster || [];
      loadedWorkspaceId = scopedWorkspaceId;
    } catch (err) {
      if (request !== loadRequest) return;
      if (err?.name === 'AbortError') return;
      console.error('Failed to load users:', err);
      users = [];
    } finally {
      if (request === loadRequest) loading = false;
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
    // The roster decides whether a display-name mention would be ambiguous, so
    // the editor inserts a username instead when another user shares the name.
    onSelect?.({ ...user, mentionText: mentionTextFor(user, users) });
  }

  // Keys the picker owns while it is open. While a mention is in progress,
  // these must always be consumed (preventDefault + stopPropagation) so the
  // underlying ProseMirror editor never sees them — otherwise Enter inserts a
  // newline right at the cursor, splitting the in-progress @mention and
  // leaving a broken chip behind (WI-200).
  const PICKER_KEYS = new Set(['ArrowDown', 'ArrowUp', 'Enter', 'Tab', 'Escape']);

  function handleKeyDown(e) {
    if (!open || !PICKER_KEYS.has(e.key)) return;

    // Consume the key so ProseMirror doesn't get to act on it regardless of
    // whether there is a result to select. (Arrow keys only make sense with a
    // non-empty list, so leave them alone then.)
    const hasResults = filteredUsers.length > 0;

    if (e.key === 'ArrowDown') {
      if (!hasResults) return;
      e.preventDefault();
      e.stopPropagation();
      highlightedIndex = (highlightedIndex + 1) % filteredUsers.length;
    } else if (e.key === 'ArrowUp') {
      if (!hasResults) return;
      e.preventDefault();
      e.stopPropagation();
      highlightedIndex = highlightedIndex === 0 ? filteredUsers.length - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      // Always consume Enter/Tab while the picker is open — even with no
      // results — so the editor can't insert a newline mid-mention.
      e.preventDefault();
      e.stopPropagation();
      if (filteredUsers[highlightedIndex]) {
        handleSelect(filteredUsers[highlightedIndex]);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      onCancel?.();
    }
  }
</script>

{#if open}
  <div
    bind:this={containerElement}
    class="mention-picker"
    data-testid="mention-picker"
    style="top: {clampedPosition.y}px; left: {clampedPosition.x}px;"
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
          data-testid="mention-option"
          class:highlighted={index === highlightedIndex}
          onmousedown={(event) => event.preventDefault()}
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
    /* Keep the menu inside the viewport on small/touch screens. The clamp in
       the script positions it, but also cap width so a 320px menu never
       overflows a 320px phone. */
    box-sizing: border-box;
  }

  /* On very narrow viewports, allow the menu to shrink to fit instead of
     forcing min-width: 240px and overflowing the screen edge. */
  @media (max-width: 360px) {
    .mention-picker {
      min-width: 0;
      width: calc(100vw - 16px);
    }
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

  /* Larger, touch-friendly tap targets on small screens so a user can be
     reliably selected by tap (WI-431). */
  @media (max-width: 768px) {
    .mention-option {
      padding: 12px 12px;
    }
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
