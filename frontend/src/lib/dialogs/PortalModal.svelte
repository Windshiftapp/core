<script>
  import { createEventDispatcher } from 'svelte';
  import { X } from 'lucide-svelte';

  const dispatch = createEventDispatcher();

  let {
    isOpen = false,
    isDarkMode = false,
    maxWidth = 'max-w-2xl',
    bodyClass = 'px-6 py-4 max-h-[60vh] overflow-y-auto',
    headerPaddingClass = 'px-6 py-4',
    title = '',
    subtitle = '',
    showHeader = true,
    backdropOpacity = 0.4,
    backdropBlur = 'blur(8px)',
    onClose = null
  } = $props();

  function close() {
    isOpen = false;
    dispatch('close');
    onClose?.();
  }

  function handleBackdropClick(event) {
    if (event.target === event.currentTarget) {
      close();
    }
  }

  function handleKeydown(event) {
    if (event.key === 'Escape' && isOpen) {
      close();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen}
  <div
    class="fixed inset-0 flex items-center justify-center z-50 p-4"
    style={`background-color: rgba(0, 0, 0, ${backdropOpacity}); backdrop-filter: ${backdropBlur};`}
    onclick={handleBackdropClick}
  >
    <div
      class={`w-full ${maxWidth} rounded-2xl shadow-2xl overflow-hidden`}
      style={`background-color: ${isDarkMode ? '#1e293b' : '#ffffff'};`}
      onclick={(e) => e.stopPropagation()}
    >
      {#if showHeader}
        <div
          class={`${headerPaddingClass} border-b flex items-center justify-between`}
          style={`background-color: ${isDarkMode ? '#334155' : '#f9fafb'}; border-color: ${isDarkMode ? '#475569' : '#e5e7eb'};`}
        >
          <slot name="header">
            <div>
              {#if title}
                <h2 class="text-lg font-semibold" style={`color: ${isDarkMode ? '#e2e8f0' : '#111827'};`}>
                  {title}
                </h2>
              {/if}
              {#if subtitle}
                <p class="text-sm mt-1" style={`color: ${isDarkMode ? '#94a3b8' : '#6b7280'};`}>
                  {subtitle}
                </p>
              {/if}
            </div>
          </slot>
          <button
            onclick={close}
            class="p-2 rounded transition-all hover:bg-white/10"
            aria-label="Close"
          >
            <X class="w-5 h-5" style={`color: ${isDarkMode ? '#94a3b8' : '#6b7280'};`} />
          </button>
        </div>
      {/if}

      <div class={bodyClass}>
        <slot />
      </div>
    </div>
  </div>
{/if}
