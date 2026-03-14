<script>
  import { fade, scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';

  let {
    isOpen = $bindable(false),
    preventClose = false,
    maxWidth = 'max-w-lg',
    maxHeight = '',
    autoFocus = true,
    onSubmit = null,
    submitDisabled = false,
    zIndexClass = 'z-50',
    noBackdrop = false,
    onclose = null,
    children
  } = $props();

  let backdropElement = $state(null);
  let modalContentElement = $state(null);
  let hasTextarea = $state(false);

  // Get shortcut configurations
  const submitShortcut = getShortcut('modal', 'submit');
  const cancelShortcut = getShortcut('modal', 'cancel');

  function close() {
    if (!preventClose) {
      isOpen = false;
      onclose?.();
    }
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }

  function handleSubmit() {
    if (onSubmit && !submitDisabled) {
      onSubmit();
    }
  }

  function handleKeydown(e) {
    // Check for cancel shortcut (Escape)
    if (matchesShortcut(e, cancelShortcut)) {
      close();
      return;
    }

    // Only handle submission if onSubmit is provided
    if (!onSubmit || submitDisabled) return;

    const isTextArea = e.target.tagName === 'TEXTAREA';

    // Check for submit shortcut (Ctrl/Cmd+Enter)
    if (matchesShortcut(e, submitShortcut)) {
      e.preventDefault();
      handleSubmit();
      return;
    }

    // Enter without modifier
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
      // In textarea: do nothing (let it create new line)
      if (isTextArea) {
        return;
      }
      // In input field or outside input: submit
      e.preventDefault();
      handleSubmit();
    }
  }

  // Detect if modal contains textarea elements
  function detectTextarea() {
    if (modalContentElement) {
      hasTextarea = modalContentElement.querySelector('textarea') !== null;
    }
  }

  let submitHint = $derived(hasTextarea ? getDisplayString(submitShortcut) : '↵');

  $effect(() => {
    if (isOpen && modalContentElement && backdropElement) {
      setTimeout(() => {
        detectTextarea();
        backdropElement.focus();
        if (autoFocus) {
          const focusable = modalContentElement.querySelector(
            'input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled])'
          );
          if (focusable) {
            focusable.focus();
          }
        }
      }, 100);
    }
  });
</script>

{#if isOpen}
  <!-- Backdrop -->
  <div
    transition:fade={{ duration: 150 }}
    bind:this={backdropElement}
    class={`fixed inset-0 flex items-start justify-center pt-8 overflow-y-auto ${zIndexClass}`}
    style={noBackdrop ? '' : 'background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(4px);'}
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
  >
    <!-- Modal with scale entrance animation -->
    <div
      bind:this={modalContentElement}
      transition:scale={{ duration: 200, start: 0.95, easing: backOut }}
      class="relative rounded-xl overflow-hidden {maxWidth} w-full mx-4 mb-8 modal-content {maxHeight ? 'flex flex-col' : ''}"
      style="background-color: var(--ds-surface-raised, var(--ds-surface, white)); box-shadow: var(--shadow-float, 0 20px 50px rgba(0, 0, 0, 0.18));{maxHeight ? ` max-height: ${maxHeight};` : ''}"
    >
      {@render children?.(submitHint)}
    </div>
  </div>
{/if}

<style>
  .modal-content {
    animation: scale-in var(--duration-normal, 200ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)) forwards;
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    .modal-content {
      animation: none;
    }
  }
</style>
