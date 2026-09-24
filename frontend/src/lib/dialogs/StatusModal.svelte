<script>
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import Input from '../components/Input.svelte';
  import TextareaField from '../components/TextareaField.svelte';
  import Toggle from '../components/Toggle.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import LocalizedObjectFields from '../settings/LocalizedObjectFields.svelte';

  let {
    isOpen = false,
    formData = $bindable({
      name: '',
      description: '',
      category_id: null,
      is_default: false
    }),
    categories = [],
    isEditing = false,
    objectId = null,
    displayName = '',
    displayDescription = '',
    translationEditor = $bindable(null),
    saving = false,
    onsave = undefined,
    oncancel = undefined
  } = $props();

  function handleSubmit() {
    if (formData.name.trim()) {
      onsave?.();
    }
  }

  function handleCancel() {
    oncancel?.();
  }

  function categoryDisplayName(category) {
    return category?.display_name || category?.name || '';
  }
</script>

{#if isOpen}
  <Modal {isOpen} onSubmit={handleSubmit} submitDisabled={!formData.name.trim()} maxWidth="max-w-lg" onclose={handleCancel}>
    {#snippet children(submitHint)}
      <ModalHeader title={isEditing ? t('statuses.editStatus') : t('statuses.createStatus')} showCloseButton={false} />

      <div class="px-6 py-4">
        <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
          {#if isEditing}
            {#key objectId}
              <LocalizedObjectFields
                bind:this={translationEditor}
                objectType="status"
                {objectId}
                bind:canonicalName={formData.name}
                bind:canonicalDescription={formData.description}
                {displayName}
                {displayDescription}
              />
            {/key}
          {:else}
            <div class="mb-4">
              <Label color="default" required class="mb-2">{t('common.name')}</Label>
              <Input
                type="text"
                dataTestid="status-modal-name"
                placeholder={t('statuses.namePlaceholder')}
                bind:value={formData.name}
                required
                size="small"
              />
            </div>
          {/if}

          <div class="mb-4">
            <Label color="default" required class="mb-2">{t('common.category')}</Label>
            <BasePicker
              bind:value={formData.category_id}
              items={categories}
              placeholder={t('categories.selectCategory')}
              getValue={(item) => item.id}
              getLabel={categoryDisplayName}
            />
          </div>

          {#if !isEditing}
            <div class="mb-4">
              <TextareaField
                label={t('common.description')}
                placeholder={t('placeholders.optionalDescription')}
                rows={2}
                bind:value={formData.description}
              />
            </div>
          {/if}

          <div class="mb-6">
            <Toggle
              bind:checked={formData.is_default}
              label={t('common.default')}
              size="small"
            />
          </div>

          <DialogFooter
            onCancel={handleCancel}
            onConfirm={handleSubmit}
            confirmLabel={isEditing ? t('common.update') : t('common.create')}
            loading={saving}
            showKeyboardHint={true}
            confirmKeyboardHint={submitHint}
            class="mx-[-1.5rem] mb-[-1rem] mt-0"
            confirmTestid="status-modal-submit"
            cancelTestid="status-modal-cancel"
          />
        </form>
      </div>
    {/snippet}
  </Modal>
{/if}
