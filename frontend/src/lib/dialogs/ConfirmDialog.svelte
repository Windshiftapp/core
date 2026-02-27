<script>
  import { createEventDispatcher } from 'svelte';  // Keep for backward compatibility
  import { AlertTriangle, X, Trash2, Check } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcut, matchesShortcut } from '../utils/keyboardShortcuts.js';

  const dispatch = createEventDispatcher();  // Keep for backward compatibility

  let {
    show = $bindable(false),
    title = null,
    message = null,
    confirmText = null,
    cancelText = null,
    variant = 'danger',  // 'danger', 'warning', 'info'
    icon = AlertTriangle,
    onconfirm = null,
    oncancel = null
  } = $props();

  const submitShortcut = getShortcut('modal', 'submit');

  // Use translations for defaults
  const resolvedTitle = $derived(title ?? t('common.areYouSure'));
  const resolvedMessage = $derived(message ?? t('common.confirmAction'));
  const resolvedConfirmText = $derived(confirmText ?? t('common.confirm'));
  const resolvedCancelText = $derived(cancelText ?? t('common.cancel'));

  // Handle keyboard navigation for submit shortcuts
  function handleKeydown(event) {
    if (!show) return;

    // Check for submit shortcut (Cmd/Ctrl+Enter)
    if (matchesShortcut(event, submitShortcut)) {
      event.preventDefault();
      doConfirm();
      return;
    }

    // Enter without modifier confirms (unless on cancel button)
    if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey) {
      const activeElement = document.activeElement;
      const isOnCancelButton = activeElement?.textContent?.trim() === resolvedCancelText;
      if (!isOnCancelButton) {
        event.preventDefault();
        doConfirm();
      }
    }
  }

  function doConfirm() {
    dispatch('confirm');  // Keep for backward compatibility
    onconfirm?.();        // New Svelte 5 style
    show = false;
  }

  function cancel() {
    dispatch('cancel');   // Keep for backward compatibility
    oncancel?.();         // New Svelte 5 style
    show = false;
  }

  // Get styles based on variant
  function getVariantStyles(variant) {
    switch (variant) {
      case 'danger':
        return {
          iconColor: 'var(--ds-icon-danger)',
          buttonVariant: 'danger'
        };
      case 'warning':
        return {
          iconColor: 'var(--ds-icon-warning)',
          buttonVariant: 'primary'
        };
      case 'info':
        return {
          iconColor: 'var(--ds-icon-info)',
          buttonVariant: 'primary'
        };
      default:
        return {
          iconColor: 'var(--ds-icon)',
          buttonVariant: 'primary'
        };
    }
  }

  let styles = $derived(getVariantStyles(variant));
</script>

<svelte:window onkeydown={handleKeydown} />

<ModalBackdrop bind:show onclose={cancel} ariaLabelledBy="dialog-title" zIndex={70}>
    <!-- Modal content -->
    <div
      class="bg-white rounded shadow-xl max-w-md w-full transform transition-all"
      style="background-color: var(--ds-surface-raised);"
      onclick={(e) => e.stopPropagation()}
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          {#if icon}
            <div class="flex-shrink-0">
              <svelte:component
                this={icon}
                class="w-6 h-6"
                style="color: {styles.iconColor};"
              />
            </div>
          {/if}
          <h3
            id="dialog-title"
            class="text-lg font-medium flex-1"
            style="color: var(--ds-text);"
          >
            {resolvedTitle}
          </h3>
          <Button
            variant="ghost"
            icon={X}
            onclick={cancel}
            title={t('common.close')}
          />
        </div>
      </div>
      
      <!-- Body -->
      <div class="px-6 py-4">
        <p
          id="dialog-description"
          class="text-sm leading-relaxed"
          style="color: var(--ds-text-subtle);"
        >
          {resolvedMessage}
        </p>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
        <Button
          variant="default"
          onclick={cancel}
          size="small"
          keyboardHint="Esc"
        >
          {resolvedCancelText}
        </Button>

        <Button
          variant={styles.buttonVariant}
          onclick={doConfirm}
          size="small"
          keyboardHint="↵"
        >
          {resolvedConfirmText}
        </Button>
      </div>
    </div>
</ModalBackdrop>

<style>
  /* Custom button styling for different variants */
  :global(.confirm-button-danger) {
    background-color: var(--ds-background-danger-bold) !important;
    border-color: var(--ds-background-danger-bold) !important;
    color: var(--ds-text-inverse) !important;
  }

  :global(.confirm-button-danger:hover) {
    background-color: var(--ds-background-danger-bold-hovered) !important;
    border-color: var(--ds-background-danger-bold-hovered) !important;
  }

  :global(.confirm-button-warning) {
    background-color: var(--ds-background-warning-bold) !important;
    border-color: var(--ds-background-warning-bold) !important;
    color: var(--ds-text-inverse) !important;
  }

  :global(.confirm-button-warning:hover) {
    background-color: var(--ds-background-warning-bold-hovered) !important;
    border-color: var(--ds-background-warning-bold-hovered) !important;
  }

  :global(.confirm-button-info) {
    background-color: var(--ds-interactive) !important;
    border-color: var(--ds-interactive) !important;
    color: var(--ds-text-inverse) !important;
  }

  :global(.confirm-button-info:hover) {
    background-color: var(--ds-interactive-hovered) !important;
    border-color: var(--ds-interactive-hovered) !important;
  }

  :global(.confirm-button-default) {
    background-color: var(--ds-background-neutral-bold) !important;
    border-color: var(--ds-background-neutral-bold) !important;
    color: var(--ds-text-inverse) !important;
  }

  :global(.confirm-button-default:hover) {
    background-color: var(--ds-background-neutral-bold-hovered) !important;
    border-color: var(--ds-background-neutral-bold-hovered) !important;
  }
</style>