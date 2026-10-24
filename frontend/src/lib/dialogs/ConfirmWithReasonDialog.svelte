<script>
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import ConfirmDialog from './ConfirmDialog.svelte';

  // ConfirmDialog with a required reason input. The Confirm button stays
  // disabled until the reason is non-empty; onconfirm receives the trimmed
  // reason string so callers can audit-log it.
  let {
    show = $bindable(false),
    title = null,
    message = null,
    reasonLabel = null,
    reasonPlaceholder = null,
    confirmText = null,
    cancelText = null,
    variant = 'warning', // 'danger', 'warning', 'info'
    icon = AlertTriangle,
    onconfirm = null,
    oncancel = null,
  } = $props();
</script>

<ConfirmDialog
  bind:show
  {title}
  {message}
  {confirmText}
  {cancelText}
  {variant}
  {icon}
  {onconfirm}
  {oncancel}
  {reasonLabel}
  {reasonPlaceholder}
  requireReason
  testIdPrefix="reason-dialog"
/>
