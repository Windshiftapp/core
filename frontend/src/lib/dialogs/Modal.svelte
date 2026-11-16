<script>
  import { onDestroy } from 'svelte';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { overlayOwnsFocus, overlayHasFocusedControl, isFocusEligible } from '../utils/overlayScope.js';

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
    ariaLabelledBy = undefined,
    ariaLabel = undefined,
    children
  } = $props();

  let backdropElement = $state(null);
  let modalContentElement = $state(null);
  let hasTextarea = $state(false);

  const submitShortcut = getShortcut('modal', 'submit');
  const cancelShortcut = getShortcut('modal', 'cancel');

  function close() {
    if (!preventClose) {
      isOpen = false;
      onclose?.();
    }
  }

  function handleBackdropClick(e) {
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
    if (e.isComposing || e.keyCode === 229) return;

    // Rich-text editors may prevent Cmd/Ctrl+Enter while still requesting submit.
    if (onSubmit && matchesShortcut(e, submitShortcut)) {
      e.preventDefault();
      handleSubmit();
      return;
    }

    if (e.defaultPrevented) return;

    if (matchesShortcut(e, cancelShortcut)) {
      close();
      return;
    }

    if (!onSubmit) return;

    // Native buttons, pickers and multiline editors own their Enter activation.
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey &&
        e.target instanceof HTMLInputElement &&
        ['text', 'search', 'email', 'url', 'tel', 'password', 'number'].includes(e.target.type) &&
        !e.target.closest('[role="combobox"], [role="listbox"], [contenteditable="true"]')) {
      e.preventDefault();
      handleSubmit();
    }
  }

  // Lazy multiline editors switch the footer hint from Enter to Cmd/Ctrl+Enter.
  function detectTextarea() {
    if (modalContentElement) {
      hasTextarea =
        modalContentElement.querySelector('textarea, [contenteditable="true"]') !== null;
    }
  }

  let submitHint = $derived(hasTextarea ? getDisplayString(submitShortcut) : '↵');

  // Schedule once per opening, not for every content update.
  let autoFocusTimer = null;

  onDestroy(() => {
    if (autoFocusTimer !== null) clearTimeout(autoFocusTimer);
  });

  $effect(() => {
    if (!isOpen) {
      if (autoFocusTimer !== null) {
        clearTimeout(autoFocusTimer);
        autoFocusTimer = null;
      }
      return;
    }
    if (!modalContentElement || !backdropElement) return;

    if (autoFocusTimer === null) {
      autoFocusTimer = setTimeout(() => {
        autoFocusTimer = null;
        if (!isOpen || !modalContentElement || !backdropElement || !overlayOwnsFocus(backdropElement)) return;
        detectTextarea();
        if (overlayHasFocusedControl(backdropElement)) return;
        // Managed focus must not scroll a dialog the user already positioned.
        backdropElement.focus({ preventScroll: true });
        if (autoFocus) {
          const focusable = [...modalContentElement.querySelectorAll(
            'input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled])'
          )].find(isFocusEligible);
          if (focusable) {
            focusable.focus({ preventScroll: true });
          }
        }
      }, 100);
    }

    // Keep the hint current when an editor mounts after opening.
    const observer = new MutationObserver(detectTextarea);
    observer.observe(modalContentElement, { childList: true, subtree: true });
    return () => observer.disconnect();
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
  <ModalBackdrop
    bind:show={isOpen}
    portal={true}
    zIndex={null}
    class={zIndexClass}
    {dataTestid}
    {ariaLabelledBy}
    {ariaLabel}
    align="top" paddingTop="pt-8" scrollable
    opacity={noBackdrop ? 0 : 0.4}
    blur={noBackdrop ? 0 : 4}
    closeOnClick={false} closeOnEscape={false}
    onKeydown={handleKeydown}
    onBackdropClick={handleBackdropClick}
    bind:backdropElement
    focusOnOpen={false}
    onclose={close}
  >
    <div
      bind:this={modalContentElement}
      onfocusin={detectTextarea}
      class="relative rounded-lg overflow-hidden {maxWidth} w-full mx-4 mb-8 {maxHeight ? 'flex flex-col' : ''}"
      style="background-color: var(--ds-surface-raised, var(--ds-surface, white)); box-shadow: var(--shadow-float, 0 20px 50px rgba(0, 0, 0, 0.18));{maxHeight ? ` max-height: ${maxHeight};` : ''}"
    >
      {@render children?.(submitHint)}
    </div>
  </ModalBackdrop>
{/if}
