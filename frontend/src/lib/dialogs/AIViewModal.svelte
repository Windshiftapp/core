<script>
  import { X } from 'lucide-svelte';
  import { scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import Spinner from '../components/Spinner.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';

  let {
    show = $bindable(false),
    title = '',
    icon = null,
    loading = false,
    error = null,
    onclose = null,
    children,
  } = $props();

  function close() {
    show = false;
    onclose?.();
  }
</script>

<ModalBackdrop bind:show opacity={0.4} blur={4} align="top" paddingTop="pt-8" scrollable onclose={close} ariaLabelledBy="ai-modal-title">
    <div
      transition:scale={{ duration: 200, start: 0.95, easing: backOut }}
      class="relative rounded-xl overflow-hidden max-w-2xl w-full mx-4 mb-8"
      style="background-color: var(--ds-surface-raised); box-shadow: 0 20px 50px rgba(0, 0, 0, 0.18);"
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          {#if icon}
            <svelte:component this={icon} class="w-5 h-5" style="color: var(--ds-interactive);" />
          {/if}
          <h3 id="ai-modal-title" class="text-lg font-semibold" style="color: var(--ds-text);">{title}</h3>
        </div>
        <button
          onclick={close}
          class="p-1.5 rounded transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => { e.currentTarget.style.color = 'var(--ds-text)'; e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
          onmouseleave={(e) => { e.currentTarget.style.color = 'var(--ds-text-subtle)'; e.currentTarget.style.backgroundColor = 'transparent'; }}
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
          {@render children()}
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-3 border-t flex justify-end" style="border-color: var(--ds-border);">
        <button
          onclick={close}
          class="px-4 py-2 text-sm font-medium rounded-md transition-colors"
          style="color: var(--ds-text); background-color: var(--ds-surface); border: 1px solid var(--ds-border);"
          onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
        >
          Close
        </button>
      </div>
    </div>
</ModalBackdrop>
