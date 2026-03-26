<script>
  import { logbookActions } from '../../api/logbookActions.js';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { IconPlayerPlay as Play } from '@tabler/icons-svelte-runes';

  let {
    bucketId,
    documentId,
    variant = 'button', // 'button' for toolbar, 'icon' for card hover
  } = $props();

  let actions = $state([]);
  let loading = $state(true);
  let executingId = $state(null);

  $effect(() => {
    if (bucketId) {
      loading = true;
      logbookActions.getAll(bucketId).then(result => {
        const all = result?.data ?? result ?? [];
        actions = all.filter(a => a.trigger_type === 'manual' && a.is_enabled);
      }).catch(() => {
        actions = [];
      }).finally(() => {
        loading = false;
      });
    }
  });

  async function executeAction(action) {
    if (executingId) return;
    executingId = action.id;
    try {
      await logbookActions.execute(bucketId, action.id, documentId);
      successToast(`Action "${action.name}" executed`);
    } catch (error) {
      errorToast(error.message || String(error));
    } finally {
      executingId = null;
    }
  }

  let menuItems = $derived(actions.map(action => ({
    id: action.id,
    title: action.name,
    icon: Play,
    onClick: () => executeAction(action),
  })));

  let hasActions = $derived(!loading && actions.length > 0);
</script>

{#if hasActions}
  {#if variant === 'icon'}
    <DropdownMenu
      items={menuItems}
      showChevron={false}
      iconOnly={true}
      triggerClass="p-1.5 rounded-lg shadow-sm border transition-colors hover:bg-opacity-90"
      triggerStyle="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
      placement="bottom-end"
    >
      {#snippet children()}
        <Play class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
      {/snippet}
    </DropdownMenu>
  {:else}
    <DropdownMenu
      items={menuItems}
      triggerIcon={Play}
      triggerText={t('logbook.runAction')}
      showChevron={false}
      placement="bottom-end"
    />
  {/if}
{/if}
