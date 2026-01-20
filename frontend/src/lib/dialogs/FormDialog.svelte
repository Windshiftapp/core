<script>
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = false,
    title,
    subtitle = '',
    icon = null,
    confirmLabel = null,
    cancelLabel = null,
    variant = 'primary', // 'primary' | 'danger'
    loading = false,
    disabled = false,
    maxWidth = 'max-w-lg',
    showCloseButton = true,
    preventClose = false,
    onClose = null,
    onSubmit = null,
    footerExtra = null, // Render function for extra footer content
    children
  } = $props();

  function handleClose() {
    if (onClose) onClose();
  }

  function handleSubmit() {
    if (onSubmit) onSubmit();
  }
</script>

<Modal
  {isOpen}
  {maxWidth}
  {preventClose}
  onclose={handleClose}
>
  <ModalHeader
    {title}
    {subtitle}
    {icon}
    showCloseButton={showCloseButton && !preventClose}
    onClose={handleClose}
  />

  <div class="px-6 py-4">
    {@render children()}
  </div>

  <DialogFooter
    confirmLabel={confirmLabel ?? t('dialogs.save')}
    cancelLabel={cancelLabel ?? t('dialogs.cancel')}
    {variant}
    {loading}
    {disabled}
    onCancel={handleClose}
    onConfirm={handleSubmit}
    extra={footerExtra}
  />
</Modal>
