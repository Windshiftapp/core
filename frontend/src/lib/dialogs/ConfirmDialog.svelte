<script>
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import X from '@lucide/svelte/icons/x';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcut, matchesShortcut } from '../utils/keyboardShortcuts.js';

  // With requireReason, the Confirm button stays disabled until the reason is
  // non-empty and onconfirm receives the trimmed reason string so callers can
  // audit-log it. Without it, onconfirm is called with no arguments.
  let {
    show = $bindable(false),
    title = null,
    message = null,
    confirmText = null,
    cancelText = null,
    variant = 'danger',  // 'danger', 'warning', 'info'
    icon: Icon = AlertTriangle,
    requireReason = false,
    reasonLabel = null,
    reasonPlaceholder = null,
    testIdPrefix = 'dialog',
    onconfirm = null,
    oncancel = null
  } = $props();

  const submitShortcut = getShortcut('modal', 'submit');

  // Use translations for defaults
  const resolvedTitle = $derived(title ?? t('common.areYouSure'));
  const resolvedMessage = $derived(message ?? t('common.confirmAction'));
  const resolvedReasonLabel = $derived(reasonLabel ?? 'Reason (audit-logged)');
  const resolvedReasonPlaceholder = $derived(reasonPlaceholder ?? 'Why are you making this change?');
  const resolvedConfirmText = $derived(confirmText ?? t('common.confirm'));
  const resolvedCancelText = $derived(cancelText ?? t('common.cancel'));

  let reason = $state('');
  let reasonInputEl = $state(null);

  // Reset the reason whenever the dialog opens; focus the input after the
  // backdrop has rendered.
  $effect(() => {
    if (show && requireReason) {
      reason = '';
      queueMicrotask(() => reasonInputEl?.focus());
    }
  });

  let canConfirm = $derived(!requireReason || reason.trim().length > 0);

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
    if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey && !event.shiftKey) {
      const activeElement = document.activeElement;
      const isOnCancelButton = activeElement?.textContent?.trim() === resolvedCancelText;
      if (!isOnCancelButton) {
        event.preventDefault();
        doConfirm();
      }
    }
  }

  function doConfirm() {
    if (!canConfirm) return;
    if (requireReason) {
      onconfirm?.(reason.trim());
    } else {
      onconfirm?.();
    }
    show = false;
  }

  function cancel() {
    oncancel?.();
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

<ModalBackdrop bind:show onclose={cancel} ariaLabelledBy="{testIdPrefix}-title" zIndex={70}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div
      role="presentation"
      class="bg-white rounded shadow-xl max-w-md w-full transform transition-all"
      style="background-color: var(--ds-surface-raised);"
      onclick={(e) => e.stopPropagation()}
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          {#if Icon}
            <div class="flex-shrink-0">
              <Icon
                class="w-6 h-6"
                style="color: {styles.iconColor};"
              />
            </div>
          {/if}
          <h3
            id="{testIdPrefix}-title"
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
      <div class="px-6 py-4 space-y-4">
        <p
          id="{testIdPrefix}-description"
          class="text-sm leading-relaxed"
          style="color: var(--ds-text-subtle);"
        >
          {resolvedMessage}
        </p>
        {#if requireReason}
          <div>
            <label for="confirm-reason-input" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              {resolvedReasonLabel}
            </label>
            <Input
              id="confirm-reason-input"
              bind:value={reason}
              bind:inputRef={reasonInputEl}
              placeholder={resolvedReasonPlaceholder}
              required
            />
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
        <Button
          variant="default"
          onclick={cancel}
          size="small"
          keyboardHint="Esc"
          dataTestid="{testIdPrefix}-cancel"
        >
          {resolvedCancelText}
        </Button>

        <Button
          variant={styles.buttonVariant}
          onclick={doConfirm}
          size="small"
          keyboardHint="↵"
          disabled={!canConfirm}
          dataTestid="{testIdPrefix}-confirm"
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