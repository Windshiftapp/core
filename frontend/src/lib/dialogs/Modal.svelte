<script>
  import { fade, scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';
  import { portal } from '../actions/portal.js';

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
    inline = false,
    closeOnBackdropClick = true,
    onclose = null,
    onKeydown = null,
    dataTestid = undefined,
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
    // Clicking outside the modal content can silently dismiss it and lose
    // anything the user has typed. Creation / editing dialogs gate this off
    // (closeOnBackdropClick=false); the modal is closed through its explicit
    // buttons or Escape instead.
    if (closeOnBackdropClick && e.target === e.currentTarget) {
      close();
    }
  }

  function handleSubmit() {
    if (onSubmit && !submitDisabled) {
      onSubmit();
    }
  }

  function handleKeydown(e) {
    onKeydown?.(e);
    e.stopPropagation();

    // The submit gesture (Ctrl/Cmd+Enter) must fire even when inner content
    // already handled the key: a rich-text editor (ProseMirror) calls
    // preventDefault() on Cmd+Enter, which would otherwise bail at the
    // defaultPrevented guard below and swallow the submit. Checking it first
    // mirrors CreateModal's window-level handler.
    if (onSubmit && !submitDisabled && matchesShortcut(e, submitShortcut)) {
      e.preventDefault();
      handleSubmit();
      return;
    }

    if (e.defaultPrevented) return;

    // Check for cancel shortcut (Escape)
    if (matchesShortcut(e, cancelShortcut)) {
      close();
      return;
    }

    // Only handle submission if onSubmit is provided
    if (!onSubmit || submitDisabled) return;

    // Enter without modifier
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
      // A <textarea> or a rich-text contenteditable (the Markdown editor's
      // ProseMirror surface) owns bare Enter — it inserts a line rather than
      // submitting the modal.
      if (e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) {
        return;
      }
      // In input field or outside input: submit
      e.preventDefault();
      handleSubmit();
    }
  }

  // Detect if the modal contains a multiline editor (a <textarea> or a
  // contenteditable rich-text surface). This drives the footer submit hint:
  // Ctrl/Cmd+Enter when present, plain Enter otherwise. Re-run on focusin too,
  // since a lazily-mounted editor may not exist yet at initial detection.
  function detectTextarea() {
    if (modalContentElement) {
      hasTextarea =
        modalContentElement.querySelector('textarea, [contenteditable="true"]') !== null;
    }
  }

  let submitHint = $derived(hasTextarea ? getDisplayString(submitShortcut) : '↵');

  $effect(() => {
    if (isOpen && modalContentElement && backdropElement) {
      const timer = setTimeout(() => {
        detectTextarea();
        if (modalContentElement.contains(document.activeElement)) return;
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
      // A lazily-mounted editor may appear after the initial detect — keep the
      // submit hint in sync as the modal's subtree changes.
      const observer = new MutationObserver(detectTextarea);
      observer.observe(modalContentElement, { childList: true, subtree: true });
      return () => {
        clearTimeout(timer);
        observer.disconnect();
      };
    }
  });
</script>

{#if isOpen && inline}
  <!-- Inline mode is used for long creation flows that need page-sized space
       without dialog semantics, a backdrop, or scroll trapping. -->
  <div
    class="relative rounded-lg overflow-hidden w-full border"
    style="background-color: var(--ds-surface-raised, var(--ds-surface, white)); border-color: var(--ds-border);"
  >
    {@render children?.(getDisplayString(submitShortcut))}
  </div>
{:else if isOpen}
  <!-- Backdrop -->
  <div
    use:portal
    transition:fade={{ duration: 150 }}
    bind:this={backdropElement}
    class={`fixed inset-0 flex items-start justify-center pt-8 overflow-y-auto ${zIndexClass}`}
    style={noBackdrop ? '' : 'background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(4px);'}
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    onfocusin={detectTextarea}
    role="dialog"
    aria-modal="true"
    data-testid={dataTestid}
  >
    <!-- Modal with scale entrance animation -->
    <div
      bind:this={modalContentElement}
      transition:scale={{ duration: 200, start: 0.95, easing: backOut }}
      class="relative rounded-lg overflow-hidden {maxWidth} w-full mx-4 mb-8 {maxHeight ? 'flex flex-col' : ''}"
      style="background-color: var(--ds-surface-raised, var(--ds-surface, white)); box-shadow: var(--shadow-float, 0 20px 50px rgba(0, 0, 0, 0.18));{maxHeight ? ` max-height: ${maxHeight};` : ''}"
    >
      {@render children?.(submitHint)}
    </div>
  </div>
{/if}
