<script>
  import MobileSheet from './MobileSheet.svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * Confirmation bottom sheet for the phone surface — replaces the desktop
   * ConfirmDialog card on /m for destructive actions and unsaved-changes
   * guards.
   *
   * @type {{
   *   isOpen?: boolean,
   *   title?: string,
   *   message?: string,
   *   confirmLabel?: string,
   *   cancelLabel?: string,
   *   destructive?: boolean,
   *   busy?: boolean,
   *   pushHistory?: boolean,
   *   onconfirm?: () => void,
   *   onclose?: (() => void) | null,
   *   dataTestid?: string,
   * }}
   */
  let {
    isOpen = $bindable(false),
    title = undefined,
    message = '',
    confirmLabel = undefined,
    cancelLabel = undefined,
    destructive = false,
    busy = false,
    pushHistory = true,
    onconfirm = () => {},
    onclose = null,
    dataTestid = undefined,
  } = $props();
</script>

<MobileSheet bind:isOpen title={title ?? t('common.areYouSure')} {onclose} {dataTestid} {pushHistory}>
  <div class="confirm">
    {#if message}
      <p class="message" data-testid="mobile-confirm-message">{message}</p>
    {/if}
    <div class="buttons">
      <button
        class="btn cancel"
        onclick={() => (isOpen = false)}
        disabled={busy}
        type="button"
        data-testid="mobile-confirm-cancel"
      >
        {cancelLabel ?? t('common.cancel')}
      </button>
      <button
        class="btn confirm-btn"
        class:destructive
        onclick={onconfirm}
        disabled={busy}
        type="button"
        data-testid="mobile-confirm-accept"
      >
        {confirmLabel ?? t('common.confirm')}
      </button>
    </div>
  </div>
</MobileSheet>

<style>
  .confirm {
    padding: 0 1rem 1rem;
  }
  .message {
    margin: 0 0 1rem;
    font-size: 0.9375rem;
    line-height: 1.5;
    color: var(--ds-text-subtle);
    overflow-wrap: anywhere;
  }
  .buttons {
    display: flex;
    gap: 0.625rem;
  }
  .btn {
    flex: 1;
    min-height: 48px;
    border-radius: var(--radius-lg, 8px);
    font-size: 1rem;
    font-weight: var(--font-medium, 500);
    cursor: pointer;
  }
  .btn:disabled {
    opacity: 0.6;
  }
  .cancel {
    border: 1px solid var(--ds-border);
    background: transparent;
    color: var(--ds-text);
  }
  .confirm-btn {
    border: none;
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
  }
  .confirm-btn.destructive {
    background: var(--ds-danger, #ef4444);
  }
</style>
