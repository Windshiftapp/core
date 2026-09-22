<script>
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Command as CommandIcon } from '@lucide/svelte';
  import TodoList from '../features/items/TodoList.svelte';
  import MobileHeader from './MobileHeader.svelte';
  import { mobilePalette } from './mobilePalette.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  // The Personal tab embeds the desktop TodoList (sections, quick-add,
  // completed-range filter, status toggles) instead of keeping a parallel
  // mobile checklist. Rows delegate to the mobile item detail route; only
  // the workspace resolution and error state live here.
  let personalWorkspaceId = $state(null);
  let loadFailed = $state(false);
  // Bumping the nonce remounts the list: a task created in this tab (mobile
  // create flow) doesn't broadcast to the posting tab, so refresh directly.
  let listNonce = $state(0);

  async function loadWorkspace() {
    loadFailed = false;
    try {
      const ws = await api.workspaces.getOrCreatePersonal();
      personalWorkspaceId = ws?.id ?? null;
      loadFailed = !personalWorkspaceId;
    } catch (err) {
      console.error('Failed to load personal workspace:', err);
      personalWorkspaceId = null;
      loadFailed = true;
    }
  }

  $effect(() => {
    loadWorkspace();
  });

  $effect(() => {
    const onCreated = () => (listNonce += 1);
    window.addEventListener('personal-task-created', onCreated);
    return () => window.removeEventListener('personal-task-created', onCreated);
  });
</script>

<MobileHeader title={t('workspaces.personal')}>
  {#snippet right()}
    <button class="hdr-palette" onclick={() => mobilePalette.open()} data-testid="mobile-palette-open" aria-label={t('mobile.palette.title')} type="button">
      <CommandIcon size={20} />
    </button>
  {/snippet}
</MobileHeader>

{#if loadFailed}
  <div class="state" data-testid="personal-error">
    <p>{t('mobile.personal.loadFailed')}</p>
    <button class="retry" onclick={loadWorkspace} type="button">{t('common.retry')}</button>
  </div>
{:else if personalWorkspaceId}
  {#key listNonce}
    <TodoList
      workspaceId={personalWorkspaceId}
      onopen={(itemId) => navigate(`/m/items/${itemId}`)}
    />
  {/key}
{/if}

<style>
  .state {
    padding: 3rem 1.25rem;
    text-align: center;
    color: var(--ds-text-subtle);
  }
  .state p { margin: 0; }
  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }
  .retry:disabled { opacity: 0.6; }

  .hdr-palette {
    display: inline-flex; align-items: center; justify-content: center;
    width: 36px; height: 36px; border: none; background: transparent;
    color: var(--ds-text); cursor: pointer;
  }
</style>
