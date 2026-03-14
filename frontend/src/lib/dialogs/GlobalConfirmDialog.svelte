<script>
  import { confirmDialog } from '../composables/useConfirm.js';
  import ConfirmDialog from './ConfirmDialog.svelte';

  let dialog = $derived($confirmDialog);
  
  function handleConfirm() {
    if (dialog.onConfirm) {
      dialog.onConfirm();
    }
  }
  
  function handleCancel() {
    if (dialog.onCancel) {
      dialog.onCancel();
    }
  }
</script>

<ConfirmDialog
  bind:show={dialog.show}
  title={dialog.title}
  message={dialog.message}
  confirmText={dialog.confirmText}
  cancelText={dialog.cancelText}
  variant={dialog.variant}
  icon={dialog.icon}
  onconfirm={handleConfirm}
  oncancel={handleCancel}
/>