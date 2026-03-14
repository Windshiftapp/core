<script>
  import Modal from './Modal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * FormModal - Standard form modal with header, content, and footer
   *
   * Combines the repeated pattern from TimeProjectModal, TimeCustomerModal,
   * IterationModal, CategoryModal, etc.
   *
   * @example
   * <FormModal
   *   isOpen={showModal}
   *   title="Create Item"
   *   editTitle="Edit Item"
   *   isEditing={editingId !== null}
   *   onSave={handleSave}
   *   onCancel={handleCancel}
   *   saveDisabled={!formData.name}
   * >
   *   <Input bind:value={formData.name} />
   * </FormModal>
   */
  let {
    isOpen = false,
    title = '',
    editTitle = null,          // Title when editing (defaults to title if not provided)
    subtitle = null,           // Optional subtitle
    isEditing = false,
    maxWidth = 'max-w-lg',
    onSave = () => {},
    onCancel = () => {},
    saveLabel = null,          // Defaults to 'Create' or 'Update' based on isEditing
    saving = false,
    saveDisabled = false,
    showFooter = true,
    footerVariant = 'primary', // 'primary' | 'danger'
    children,
    headerExtra = null,        // Extra content for header (right side)
    footerExtra = null         // Extra content for footer (left side)
  } = $props();

  function handleSubmit() {
    if (!saveDisabled && !saving) {
      onSave();
    }
  }

  function handleCancel() {
    onCancel();
  }

  // Compute display title based on editing state
  const displayTitle = $derived(isEditing && editTitle ? editTitle : title);

  // Compute save button label
  const computedSaveLabel = $derived(
    saveLabel || (isEditing ? t('common.update') : t('common.create'))
  );
</script>

{#if isOpen}
  <Modal
    {isOpen}
    onSubmit={handleSubmit}
    submitDisabled={saveDisabled || saving}
    {maxWidth}
    onclose={handleCancel}
  >
    {#snippet children(submitHint)}
    <!-- Header -->
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
            {displayTitle}
          </h3>
          {#if subtitle}
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{subtitle}</p>
          {/if}
        </div>
        {#if headerExtra}
          <div class="ml-4">
            {@render headerExtra()}
          </div>
        {/if}
      </div>
    </div>

    <!-- Content -->
    <div class="px-6 py-4">
      {@render children?.()}
    </div>

    <!-- Footer -->
    {#if showFooter}
      <DialogFooter
        onCancel={handleCancel}
        onConfirm={handleSubmit}
        confirmLabel={computedSaveLabel}
        variant={footerVariant}
        loading={saving}
        disabled={saveDisabled}
        showKeyboardHint={true}
        confirmKeyboardHint={submitHint}
        extra={footerExtra}
        class="mx-0"
      />
    {/if}
    {/snippet}
  </Modal>
{/if}
