<script>
  import Check from '@lucide/svelte/icons/check';
  import X from '@lucide/svelte/icons/x';
  import Button from "../../components/Button.svelte";
  import Input from '../../components/Input.svelte';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    item,
    workspace,
    editingTitle = $bindable(false),
    editTitle = $bindable(''),
    saving = false,
    canEdit = false,
    onsavefield = undefined,
    oncanceledit = undefined,
  } = $props();


  function startEditingTitle() {
    editTitle = item.title;
    editingTitle = true;
  }

  function validateAndSaveTitle() {
    const trimmedTitle = editTitle.trim();

    if (!trimmedTitle) {
      // Show error toast
      errorToast(t('items.previousValueRemains'), t('items.titleCannotBeEmpty'));

      // Revert to original title
      editTitle = item.title;
      editingTitle = false;
      return;
    }

    onsavefield?.({ field: "title", value: trimmedTitle });
  }

  function handleKeydown(event) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      validateAndSaveTitle();
    } else if (event.key === "Escape") {
      event.preventDefault();
      oncanceledit?.({ field: "title" });
    }
  }

</script>

<div class="mb-2 w-full max-w-full">
  <div class="mb-1">
    {#if editingTitle}
      <div class="flex items-center gap-3 w-full pr-4 ">
        <!-- Item key (in edit mode) -->

        <div class="min-w-[80%]">
          <Input
            type="text"
            variant="ghost"
            dataTestid="item-title-input"
            bind:value={editTitle}
            onkeydown={handleKeydown}
            class="w-full text-xl font-semibold bg-transparent border-0 focus:outline-none break-words title-geometry"
            style="color: var(--ds-text); word-wrap: break-word; overflow-wrap: break-word;"
            placeholder={t('items.enterTitle')}
            autofocus
          />
        </div>
        <div class="flex gap-2 mt-2 hidden">
          <Button
            variant="primary"
            size="small"
            icon={Check}
            onclick={validateAndSaveTitle}
            disabled={saving}
          />
          <Button
            variant="default"
            size="small"
            icon={X}
            onclick={() => oncanceledit?.({ field: "title" })}
          />
        </div>
      </div>
    {:else}
      <!-- Item key -->

      {#if canEdit}
        <button
          onclick={startEditingTitle}
          data-testid="item-title-edit"
          class="text-xl font-semibold transition-colors text-left cursor-pointer title-geometry break-words"
          style="color: var(--ds-text); word-wrap: break-word; overflow-wrap: break-word;"
          title={t('items.clickToEditTitle')}
        >
          {item.title}
        </button>
      {:else}
        <!-- Read-only title for users without item.edit (server enforces the
             PATCH; the UI simply must not offer the affordance). -->
        <h1
          class="text-xl font-semibold text-left title-geometry break-words"
          style="color: var(--ds-text); word-wrap: break-word; overflow-wrap: break-word;"
          data-testid="item-title-readonly"
        >
          {item.title}
        </h1>
      {/if}
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
  button.title-geometry:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
