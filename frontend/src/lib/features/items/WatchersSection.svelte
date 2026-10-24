<script>
  import Eye from '@lucide/svelte/icons/eye';
  import X from '@lucide/svelte/icons/x';
  import Plus from '@lucide/svelte/icons/plus';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Spinner from '../../components/Spinner.svelte';
  import Text from '../../components/Text.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import { getAssignableUsers } from '../../api/users.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import { itemDetailStore } from '../../stores/itemDetailStore.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';

  // Anyone who can view the item sees its watchers and can add or remove
  // themselves; canEdit additionally allows managing other users.
  let { itemId, workspaceId, canEdit = false } = $props();

  const STORAGE_KEY_EXPANDED = 'itemWatchersExpanded';

  let expanded = $state(loadExpandedState());
  let loadFailed = $state(false);
  let showAddForm = $state(false);
  let candidates = $state(null);
  let busy = $state(false);

  const currentUserId = $derived(authStore.currentUser?.id ?? null);
  const watchers = $derived(itemDetailStore.watchers);
  const watcherIds = $derived(new Set((watchers ?? []).map((watcher) => watcher.user_id)));
  // Offer only users who are not already watching.
  const availableUsers = $derived((candidates ?? []).filter((user) => !watcherIds.has(user.id)));

  // The collapsed state is a per-browser preference shared by all items.
  function loadExpandedState() {
    try {
      return localStorage.getItem(STORAGE_KEY_EXPANDED) !== 'false';
    } catch {
      return true;
    }
  }

  function toggleExpanded() {
    expanded = !expanded;
    if (!expanded) showAddForm = false;
    try {
      localStorage.setItem(STORAGE_KEY_EXPANDED, String(expanded));
    } catch {
      // Storage unavailable; the toggle still works for this session.
    }
  }

  // Collapsed sections show the summary count only, so the list is fetched
  // the first time the section is open for an item.
  $effect(() => {
    if (!expanded || !itemId || itemDetailStore.item?.id !== itemId || itemDetailStore.watchers !== null) return;
    loadFailed = false;
    itemDetailStore.loadWatchers().catch(() => {
      loadFailed = true;
    });
  });

  function displayName(watcher) {
    return `${watcher.first_name || ''} ${watcher.last_name || ''}`.trim() || watcher.username || watcher.email;
  }

  async function openAddForm() {
    showAddForm = true;
    if (candidates !== null || !workspaceId) return;
    try {
      candidates = (await getAssignableUsers(workspaceId)) ?? [];
    } catch (error) {
      candidates = [];
      errorToast(error?.message || t('items.watchersAddFailed'));
    }
  }

  async function add(user) {
    if (!user?.id || busy) return;
    busy = true;
    try {
      await itemDetailStore.addWatcher(user.id);
      showAddForm = false;
    } catch (error) {
      errorToast(error?.message || t('items.watchersAddFailed'));
    } finally {
      busy = false;
    }
  }

  async function remove(watcher) {
    if (busy) return;
    busy = true;
    try {
      await itemDetailStore.removeWatcher(watcher.user_id);
    } catch (error) {
      errorToast(error?.message || t('items.watchersRemoveFailed'));
    } finally {
      busy = false;
    }
  }
</script>

<div class="pt-4 mt-4 border-t" style="border-color: var(--ds-border);" data-testid="item-watchers-section">
  <div class="flex items-center justify-between gap-2 group">
    <button
      type="button"
      class="flex items-center gap-2 min-w-0 flex-1 text-left"
      aria-expanded={expanded}
      data-testid="item-watchers-toggle"
      onclick={toggleExpanded}
    >
      <Eye class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">
        {t('items.watchers')}
      </Text>
      <span
        class="text-xs px-1.5 rounded-full"
        style="color: var(--ds-text-subtle); background: var(--ds-background-neutral);"
        data-testid="item-watcher-count"
      >
        {itemDetailStore.watcherCount}
      </span>
    </button>
    <div class="flex items-center gap-1">
      {#if expanded && canEdit && watchers !== null}
        <button
          type="button"
          class="p-1 rounded transition-colors opacity-40 group-hover:opacity-100 focus-visible:opacity-100 disabled:opacity-20 disabled:cursor-not-allowed"
          style="color: var(--ds-text-subtle);"
          title={t('items.watcherAdd')}
          aria-label={t('items.watcherAdd')}
          data-testid="item-watcher-add-toggle"
          disabled={showAddForm}
          onclick={openAddForm}
        >
          <Plus class="w-4 h-4" />
        </button>
      {/if}
      <button
        type="button"
        class="p-1 rounded"
        style="color: var(--ds-text-subtle);"
        aria-label={t('items.watchers')}
        aria-expanded={expanded}
        onclick={toggleExpanded}
        tabindex="-1"
      >
        {#if expanded}
          <ChevronDown class="w-4 h-4" />
        {:else}
          <ChevronRight class="w-4 h-4" />
        {/if}
      </button>
    </div>
  </div>

  {#if !expanded}
    <!-- Collapsed: the header count is all that shows. -->
  {:else if watchers === null && !loadFailed}
    <div class="pt-3 flex justify-center">
      <Spinner size="small" />
    </div>
  {:else if watchers !== null}
    <div class="mt-3 space-y-1" data-testid="item-watchers">
      {#if watchers.length === 0}
        <Text variant="subtle" size="sm">{t('items.watchersEmpty')}</Text>
      {/if}
      {#each watchers as watcher (watcher.user_id)}
        {@const isSelf = watcher.user_id === currentUserId}
        <div class="flex items-center justify-between gap-2 px-1 py-1 rounded" data-testid={`item-watcher-${watcher.user_id}`}>
          <div class="flex items-center gap-2 min-w-0">
            <Avatar src={watcher.avatar_url} name={displayName(watcher)} size="xs" variant="teal" />
            <span class="text-sm truncate" style="color: var(--ds-text);" title={watcher.email}>
              {displayName(watcher)}
              {#if isSelf}
                <span style="color: var(--ds-text-subtle);">{t('items.watcherYou')}</span>
              {/if}
            </span>
          </div>
          {#if canEdit || isSelf}
            <button
              type="button"
              class="p-1 rounded hover-bg flex-shrink-0"
              title={t('items.watcherRemove')}
              aria-label={t('items.watcherRemove')}
              data-testid={`item-watcher-remove-${watcher.user_id}`}
              disabled={busy}
              onclick={() => remove(watcher)}
            >
              <X class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
            </button>
          {/if}
        </div>
      {/each}
    </div>

    {#if canEdit && showAddForm}
      <div class="mt-3 flex items-center gap-2">
        <UserPicker
          value={null}
          users={availableUsers}
          loading={candidates === null}
          placeholder={t('items.watcherPickUser')}
          allowClear={false}
          autoOpen={true}
          disabled={busy}
          class="w-full"
          onSelect={add}
          onCancel={() => (showAddForm = false)}
        />
        <button
          type="button"
          class="p-1 rounded hover-bg flex-shrink-0"
          title={t('common.cancel')}
          aria-label={t('common.cancel')}
          onclick={() => (showAddForm = false)}
        >
          <X class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
        </button>
      </div>
    {/if}
  {/if}
</div>
