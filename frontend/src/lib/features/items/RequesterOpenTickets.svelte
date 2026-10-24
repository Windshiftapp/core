<script>
  import { onDestroy, onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import Copy from '@lucide/svelte/icons/copy';
  import GitMerge from '@lucide/svelte/icons/git-merge';
  import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
  import Search from '@lucide/svelte/icons/search';
  import Spinner from '../../components/Spinner.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Text from '../../components/Text.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import ItemKey from '../../components/ItemKey.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { items } from '../../api/items.js';
  import { ai } from '../../api/ai.js';
  import { aiStore } from '../../stores/aiStore.svelte.js';

  let { itemId, canEdit = false, onrequestmerge = null } = $props();

  let loading = $state(true);
  let tickets = $state([]);
  let loadController = null;

  // AI duplicate suggestions are display-only: they live in component state
  // and are never persisted on the ticket, so a reload drops them.
  let aiDuplicates = $state([]);
  let findingDuplicates = $state(false);
  let findError = $state('');

  let canFindDuplicates = $derived(aiStore.available && aiStore.isFeatureEnabled('find_similar'));

  async function load() {
    loadController?.abort();
    loadController = new AbortController();
    loading = true;
    try {
      const response = await items.requesterOpenTickets(itemId, {
        signal: loadController.signal
      });
      tickets = response ?? [];
    } catch {
      tickets = [];
    } finally {
      loading = false;
    }
  }

  async function findDuplicates() {
    if (findingDuplicates) return;
    findingDuplicates = true;
    findError = '';
    try {
      const result = await ai.findSimilar(itemId);
      const known = new Set(tickets.map((ticket) => String(ticket.id)));
      aiDuplicates = (result?.similar_items ?? []).filter(
        (entry) =>
          String(entry.item_id) !== String(itemId) && !known.has(String(entry.item_id))
      );
    } catch (err) {
      findError = err?.message || t('items.findDuplicatesFailed');
    } finally {
      findingDuplicates = false;
    }
  }

  // The merge action lives in an overflow menu so a long label never competes
  // with the status lozenge for the card's limited width.
  function mergeMenuItems(id, prefix, onSelect) {
    return [
      {
        id: `merge-${prefix}-${id}`,
        icon: GitMerge,
        title: t('items.mergeIntoThisTicket'),
        testid: `requester-${prefix}-merge-${id}`,
        onClick: onSelect
      }
    ];
  }

  // The merge dialog addresses a candidate by `id`; AI suggestions arrive with
  // the AI response's `item_id`.
  function mergeAIDuplicate(entry) {
    onrequestmerge?.({
      id: entry.item_id,
      title: entry.title,
      item_key: entry.item_key,
      status_name: entry.status_name,
      workspace_id: entry.workspace_id,
      creator_portal_customer_id: entry.creator_portal_customer_id
    });
  }

  onMount(() => {
    aiStore.load();
    load();
  });

  onDestroy(() => loadController?.abort());

  // Merging a candidate removes it from this list; refresh without waiting
  // for a full detail reload.
  useEventListener(() => window, 'reload-item-detail', (/** @type {CustomEvent<{itemId?: number|string}>} */ event) => {
    const id = event?.detail?.itemId;
    if (id == null || String(id) !== String(itemId)) return;
    load();
  });
</script>

<div class="pt-4 mt-4 border-t" style="border-color: var(--ds-border);">
  <div class="flex items-center justify-between gap-2">
    <div class="flex items-center gap-2 min-w-0">
      <Copy class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider truncate">
        {t('items.requesterOpenTickets')}
      </Text>
    </div>
    {#if canFindDuplicates}
      <button
        type="button"
        class="flex-shrink-0 p-1 rounded transition-colors"
        style="color: var(--ds-text-subtle);"
        title={t('items.findDuplicates')}
        aria-label={t('items.findDuplicates')}
        data-testid="requester-find-duplicates"
        disabled={findingDuplicates || loading}
        onclick={findDuplicates}
      >
        {#if findingDuplicates}
          <Spinner size="small" />
        {:else}
          <Search class="w-3.5 h-3.5" />
        {/if}
      </button>
    {/if}
  </div>

  <div class="mt-2">
    <DescriptionText>{t('items.requesterOpenTicketsHelp')}</DescriptionText>
  </div>

  {#if loading}
    <div class="flex justify-center pt-3">
      <Spinner size="small" />
    </div>
  {:else}
    {#if tickets.length === 0 && aiDuplicates.length === 0 && !findError}
      <div class="mt-2">
        <DescriptionText>{t('items.requesterOpenTicketsEmpty')}</DescriptionText>
      </div>
    {/if}

    <div class="mt-3 space-y-2" data-testid="requester-open-tickets">
      {#each tickets as ticket (ticket.id)}
        <div
          class="p-2.5 rounded border transition-colors"
          style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
        >
          <div class="flex items-start justify-between gap-2">
            <a
              href={`/workspaces/${ticket.workspace_id}/items/${ticket.id}`}
              data-testid={`requester-open-ticket-${ticket.id}`}
              class="flex-1 min-w-0 block hover:opacity-90"
            >
              <div class="flex items-center justify-between gap-2">
                <ItemKey item={ticket} clickable={false} />
                {#if ticket.status_name}
                  <Lozenge color="gray">{ticket.status_name}</Lozenge>
                {/if}
              </div>
              <div class="mt-1 text-sm truncate" style="color: var(--ds-text);">
                {ticket.title}
              </div>
            </a>
            {#if canEdit && onrequestmerge}
              <DropdownMenu
                items={mergeMenuItems(ticket.id, 'open-ticket', () => onrequestmerge(ticket))}
                placement="bottom-end"
                maxWidth="max-w-xs"
                triggerIcon={MoreHorizontal}
                triggerClass="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ds-border-focused)]"
                triggerStyle="color: var(--ds-text-subtle);"
                iconOnly
                showChevron={false}
                triggerLabel={t('common.actions')}
                triggerTestid={`requester-open-ticket-menu-${ticket.id}`}
              />
            {/if}
          </div>
        </div>
      {/each}

      {#each aiDuplicates as entry (entry.item_id)}
        <div
          class="p-2.5 rounded border transition-colors"
          style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
          data-testid={`requester-ai-duplicate-${entry.item_id}`}
        >
          <div class="flex items-start justify-between gap-2">
            <a
              href={`/workspaces/${entry.workspace_id}/items/${entry.item_id}`}
              class="flex-1 min-w-0 block hover:opacity-90"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="flex items-center gap-1.5 min-w-0">
                  <ItemKey itemKey={entry.item_key} clickable={false} />
                  <Lozenge color="purple">{t('items.aiSuggested')}</Lozenge>
                </div>
                {#if entry.status_name}
                  <Lozenge color="gray">{entry.status_name}</Lozenge>
                {/if}
              </div>
              <div class="mt-1 text-sm truncate" style="color: var(--ds-text);">
                {entry.title}
              </div>
              {#if entry.reason}
                <div class="mt-1">
                  <DescriptionText>{entry.reason}</DescriptionText>
                </div>
              {/if}
            </a>
            {#if canEdit && onrequestmerge}
              <DropdownMenu
                items={mergeMenuItems(entry.item_id, 'ai-duplicate', () => mergeAIDuplicate(entry))}
                placement="bottom-end"
                maxWidth="max-w-xs"
                triggerIcon={MoreHorizontal}
                triggerClass="p-1 rounded hover:bg-[var(--ds-background-neutral-hovered)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ds-border-focused)]"
                triggerStyle="color: var(--ds-text-subtle);"
                iconOnly
                showChevron={false}
                triggerLabel={t('common.actions')}
                triggerTestid={`requester-ai-duplicate-menu-${entry.item_id}`}
              />
            {/if}
          </div>
        </div>
      {/each}
    </div>

    {#if aiDuplicates.length > 0}
      <div class="mt-2">
        <DescriptionText>{t('items.aiSuggestionsTemporary')}</DescriptionText>
      </div>
    {/if}

    {#if findError}
      <div
        class="mt-2 text-sm rounded p-2"
        style="background: var(--ds-surface-danger, rgba(239, 68, 68, 0.1)); color: var(--ds-text-danger);"
        data-testid="requester-find-duplicates-error"
      >
        {findError}
      </div>
    {/if}
  {/if}
</div>
