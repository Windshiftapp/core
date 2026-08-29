<script>
  import { onMount } from 'svelte';
  import { Sparkles } from '@lucide/svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import { ai } from '../../api/ai.js';
  import { navigate } from '../../router.js';
  import { formatAuthenticatedInstant } from '../../utils/authenticatedDateFormatter.js';
  import { t } from '../../stores/i18n.svelte.js';

  let briefing = $state(null);
  let loading = $state(true);
  let unavailable = $state(false);
  let itemKeyMap = $state({});

  onMount(async () => {
    try {
      const data = await ai.dailyBriefing();
      if (data && data.content) {
        if (data.references) itemKeyMap = data.references;
        briefing = data;
      } else {
        unavailable = true;
      }
    } catch {
      unavailable = true;
    } finally {
      loading = false;
    }
  });

  function preprocess(text) {
    if (!text) return '';
    let result = text.replace(/\[([A-Z]{2,10}-\d+)\](?!\()/g, '$1');
    result = result.replace(/\b([A-Z]{2,10}-\d+)\b/g, (_, key) =>
      itemKeyMap[key] ? `[${key}](#)` : key,
    );
    return result;
  }

  function handleClick(e) {
    const link = e.target.closest('a');
    if (!link) return;
    const key = link.textContent.trim();
    if (!/^[A-Z]{2,10}-\d+$/.test(key)) return;
    e.preventDefault();
    const item = itemKeyMap[key];
    if (item) navigate(`/workspaces/${item.workspace_id}/items/${item.item_id}`);
  }
</script>

{#if loading}
  <div class="animate-pulse space-y-2">
    <div class="h-3 w-full rounded" style="background-color: var(--ds-background-neutral);"></div>
    <div class="h-3 w-3/4 rounded" style="background-color: var(--ds-background-neutral);"></div>
    <div class="h-3 w-1/2 rounded" style="background-color: var(--ds-background-neutral);"></div>
  </div>
{:else if unavailable || !briefing}
  <div class="flex items-start gap-3 py-2">
    <Sparkles class="w-4 h-4 mt-0.5" style="color: var(--ds-icon-accent);" />
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {t('dashboard.states.dailyBriefingUnavailable')}
    </p>
  </div>
{:else}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="briefing-content" style="color: var(--ds-text);" onclick={handleClick}>
    <MilkdownEditor
      content={preprocess(briefing.content)}
      readonly={true}
      showToolbar={false}
      compact={true}
    />
  </div>
  {#if briefing.generated_at}
    <p class="text-xs mt-3" style="color: var(--ds-text-subtlest);">
      {t('dashboard.states.updatedAt', {
        time: formatAuthenticatedInstant(briefing.generated_at, {
          month: 'short',
          day: 'numeric',
          hour: 'numeric',
          minute: '2-digit',
        }),
      })}
    </p>
  {/if}
{/if}

<style>
  .briefing-content :global(.milkdown-editor .ProseMirror h1),
  .briefing-content :global(.milkdown-editor .ProseMirror h2) {
    font-size: 0.875rem;
    font-weight: 600;
    margin: 1.125rem 0 0.5rem;
  }
  .briefing-content :global(.milkdown-editor .ProseMirror h3) {
    font-size: 0.8125rem;
    font-weight: 600;
    margin: 1rem 0 0.375rem;
  }
  .briefing-content :global(.milkdown-editor .ProseMirror > h1:first-child),
  .briefing-content :global(.milkdown-editor .ProseMirror > h2:first-child),
  .briefing-content :global(.milkdown-editor .ProseMirror > h3:first-child) {
    margin-top: 0;
  }
  .briefing-content :global(.milkdown-editor .ProseMirror p) {
    font-size: 0.8125rem;
    line-height: 1.55;
    margin: 0.5rem 0;
  }
  .briefing-content :global(.milkdown-editor .ProseMirror li p) {
    margin: 0.25rem 0;
  }
  .briefing-content :global(.milkdown-editor .ProseMirror li) {
    font-size: 0.8125rem;
    line-height: 1.55;
  }
  .briefing-content :global(.milkdown-editor .ProseMirror ul),
  .briefing-content :global(.milkdown-editor .ProseMirror ol) {
    font-size: 0.8125rem;
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }
  .briefing-content :global(strong) {
    font-weight: 600;
  }
  .briefing-content :global(a) {
    color: var(--ds-link);
    text-decoration: underline;
    cursor: pointer;
  }
</style>
