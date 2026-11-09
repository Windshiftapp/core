<script>
  import TitleField from './TitleField.svelte';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    item,
    workspace = null,
    saving = false,
    canEdit = false,
    onsavefield = undefined,
  } = $props();

  async function commitTitle(draft) {
    const trimmed = String(draft ?? '').trim();
    if (!trimmed) {
      errorToast(t('items.previousValueRemains'), t('items.titleCannotBeEmpty'));
      return;
    }
    await onsavefield?.({ field: 'title', value: trimmed });
  }
</script>

<div class="mb-2 w-full max-w-full">
  <div class="mb-1">
    {#if canEdit}
      <TitleField
        value={item.title}
        editable={!saving}
        oncommit={commitTitle}
        displayTestid="item-title-edit"
        inputTestid="item-title-input"
        displayClass="block text-xl font-semibold text-start cursor-pointer title-geometry break-words"
        inputClass="w-full text-xl font-semibold bg-transparent border-0 focus:outline-none break-words title-geometry"
        placeholder={t('items.enterTitle')}
      />
    {:else}
      <!-- Read-only title for users without item.edit (server enforces the
           PATCH; the UI simply must not offer the affordance). -->
      <h1
        class="text-xl font-semibold text-start title-geometry break-words"
        dir="auto"
        style="color: var(--ds-text); word-wrap: break-word; overflow-wrap: break-word;"
        data-testid="item-title-readonly"
      >
        {item.title}
      </h1>
    {/if}
  </div>
</div>

<style>
  /* Title and description share one hover pill: an 8px outset beyond the text
     column in --ds-background-neutral-hovered, so their hover backgrounds
     align to the same edges. The edit input gets the same geometry so the
     text does not shift when entering edit mode. */
  .title-geometry {
    width: calc(100% + 1rem);
    margin: 0 -0.5rem;
    padding: 0.375rem 0.5rem;
    border-radius: 0.25rem;
  }
  .title-geometry:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
