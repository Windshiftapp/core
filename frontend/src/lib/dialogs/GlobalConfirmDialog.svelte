<script>
  import { confirmDialog } from '../composables/useConfirm.js';
  import ConfirmDialog from './ConfirmDialog.svelte';
</script>

<ConfirmDialog
  show={$confirmDialog.show}
  title={$confirmDialog.title}
  message={$confirmDialog.message}
  confirmText={$confirmDialog.confirmText}
  cancelText={$confirmDialog.cancelText}
  variant={$confirmDialog.variant}
  icon={$confirmDialog.icon}
  onconfirm={() => $confirmDialog.onConfirm?.()}
  oncancel={() => $confirmDialog.onCancel?.()}
/>
