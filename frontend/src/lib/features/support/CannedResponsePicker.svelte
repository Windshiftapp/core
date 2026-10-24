<script>
  // Canned response picker (WI-1138): lets an agent search the workspace's
  // canned responses and insert one into the reply composer. Private
  // snippets are marked so the composer can switch to an internal note. The
  // picker renders nothing until the workspace's responses load and hides
  // itself entirely when there are none.
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import MessageSquareText from '@lucide/svelte/icons/message-square-text';
  import { onMount } from 'svelte';
  import { cannedResponses } from '../../api/configuration.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';

  let { workspaceId, onSelect, isOpen = $bindable(false) } = $props();

  let responses = $state([]);
  let query = $state('');
  let loadedOnce = $state(false);

  async function load() {
    if (!workspaceId) return;
    try {
      responses = (await cannedResponses.getAll(workspaceId)) ?? [];
      loadedOnce = true;
      query = '';
    } catch (err) {
      // Non-admin agents may still read; anything else is reported once.
      console.error('Failed to load canned responses:', err);
      errorToast(err?.message || t('cannedResponses.loadFailed'));
    }
  }
  onMount(load);

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return responses;
    return responses.filter(
      (r) => r.name.toLowerCase().includes(q) || (r.body || '').toLowerCase().includes(q)
    );
  });

  let menuItems = $derived.by(() => {
    /** @type {any[]} */
    const items = [
      {
        type: 'search',
        testid: 'canned-response-search',
        placeholder: t('cannedResponses.searchPlaceholder'),
        value: query,
        onInput: (value) => (query = value),
      },
    ];
    if (filtered.length === 0) {
      items.push({ type: 'text', text: loadedOnce ? t('cannedResponses.noneMatching') : t('common.loading') });
      return items;
    }
    for (const response of filtered) {
      items.push({
        id: `canned-response-${response.id}`,
        title: response.name,
        icon: response.is_private ? MessageSquareText : undefined,
        subtitle: (response.body || '').slice(0, 90),
        testid: `canned-response-option-${response.id}`,
        onClick: () => onSelect?.(response),
      });
    }
    return items;
  });
</script>

{#if loadedOnce && responses.length > 0}
  <DropdownMenu
    triggerIcon={MessageSquareText}
    triggerTestid="canned-response-picker"
    triggerLabel={t('cannedResponses.insertResponse')}
    triggerIconClass="w-4 h-4"
    items={menuItems}
    bind:isOpen
    placement="top-start"
    matchTriggerWidth={false}
    maxWidth="max-w-md"
    onOpen={load}
  />
{/if}
