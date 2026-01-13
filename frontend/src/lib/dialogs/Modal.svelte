<script>
  import { createEventDispatcher } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';

  const dispatch = createEventDispatcher();

  export let isOpen = false;
  export let preventClose = false;
  export let maxWidth = 'max-w-lg';
  export let autoFocus = true;
  export let onSubmit = null; // Optional submit handler
  export let submitDisabled = false; // Whether submit is disabled
  export let zIndexClass = 'z-50';
  export let noBackdrop = false;
  export let onclose = null; // Callback for close (Svelte 5 compatible)

  let backdropElement;
  let modalContentElement;
  let hasTextarea = false;

  // Get shortcut configurations
  const submitShortcut = getShortcut('modal', 'submit');
  const cancelShortcut = getShortcut('modal', 'cancel');

  function close() {
    if (!preventClose) {
      isOpen = false;
      dispatch('close');
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

  // Compute the keyboard hint for submit button using centralized display logic
  $: submitHint = hasTextarea ? getDisplayString(submitShortcut) : '↵';

  // Focus management when modal opens
  $: if (isOpen && modalContentElement && backdropElement) {
    setTimeout(() => {
      // Detect textarea presence
      detectTextarea();

      // First, focus backdrop for keyboard events
      backdropElement.focus();

      // Then, autofocus first focusable element if enabled
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
      class="relative rounded-xl overflow-hidden {maxWidth} w-full mx-4 mb-8 modal-content"
      style="background-color: var(--ds-surface-raised, var(--ds-surface, white)); box-shadow: var(--shadow-float, 0 20px 50px rgba(0, 0, 0, 0.18));"
    >
      <slot {submitHint}></slot>
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
