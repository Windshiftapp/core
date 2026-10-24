<script>
  import X from '@lucide/svelte/icons/x';
  import Spinner from './Spinner.svelte';
  import ModalBackdrop from './ModalBackdrop.svelte';

  let {
    show = $bindable(false),
    title = '',
    icon: Icon = null,
    loading = false,
    error = null,
    titleId = 'ai-modal-title',
    onclose = null,
    body,
    footer = null,
  } = $props();

  function close() {
    show = false;
    onclose?.();
  }
</script>

<ModalBackdrop bind:show opacity={0.4} blur={4} align="top" paddingTop="pt-8" scrollable onclose={close} ariaLabelledBy={titleId}>
    <div
      class="relative rounded-lg overflow-hidden max-w-2xl w-full mx-4 mb-8"
      style="background-color: var(--ds-surface-raised); box-shadow: 0 20px 50px rgba(0, 0, 0, 0.18);"
    >
      <!-- Header -->
      <div class="px-6 py-3 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          {#if Icon}
            <Icon class="w-5 h-5" style="color: var(--ds-interactive);" />
          {/if}
          <h3 id={titleId} class="text-lg font-semibold" style="color: var(--ds-text);">{title}</h3>
        </div>
        <button
          onclick={close}
          class="modal-close p-1.5 rounded transition-colors"
          aria-label="Close"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Body -->
      <div class="px-6 py-5 max-h-[60vh] overflow-y-auto">
        {#if loading}
          <div class="flex flex-col items-center justify-center py-12 gap-3">
            <Spinner />
            <p class="text-sm" style="color: var(--ds-text-subtle);">Analyzing...</p>
          </div>
        {:else if error}
          <div class="py-8 text-center">
            <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
          </div>
        {:else}
          {@render body()}
        {/if}
      </div>

      <!-- Footer -->
      {#if footer}
        {@render footer()}
      {:else}
        <div class="px-6 py-3 border-t flex justify-end" style="border-color: var(--ds-border);">
          <button
            onclick={close}
            class="secondary-btn px-4 py-2 text-sm font-medium rounded-md transition-colors"
          >
            Close
          </button>
        </div>
      {/if}
    </div>
</ModalBackdrop>

<style>
  .modal-close {
    color: var(--ds-text-subtle);
  }

  .modal-close:hover {
    color: var(--ds-text);
    background-color: var(--ds-background-neutral-hovered);
  }

  /* Secondary modal action: neutral surface that lifts on hover. */
  .secondary-btn {
    color: var(--ds-text);
    background-color: var(--ds-surface);
    border: 1px solid var(--ds-border);
  }

  .secondary-btn:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
