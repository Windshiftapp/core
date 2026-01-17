<script>
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  // Props
  export let isOpen = false;
  export let formData = {
    name: '',
    description: ''
  };
  export let isEditing = false;

  function handleSubmit() {
    if (formData.name.trim()) {
      dispatch('save');
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

{#if isOpen}
  <Modal
    {isOpen}
    onSubmit={handleSubmit}
    submitDisabled={!formData.name.trim()}
    maxWidth="max-w-lg"
    onclose={handleCancel}
    let:submitHint
  >
    <div class="p-6">
      <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
        {isEditing ? t('timeProjectCategory.editCategory') : t('timeProjectCategory.newCategory')}
      </h3>

      <div class="space-y-4">
        <div>
          <Label required class="mb-2">{t('timeProjectCategory.categoryName')}</Label>
          <Input
            bind:value={formData.name}
            placeholder={t('timeProjectCategory.categoryNamePlaceholder')}
            required
          />
        </div>

        <div>
          <Label class="mb-2">{t('common.description')}</Label>
          <Textarea
            bind:value={formData.description}
            rows={3}
            placeholder={t('timeProjectCategory.optionalDescription')}
          />
        </div>
      </div>

      <div class="mt-6 flex gap-3">
        <Button
          variant="primary"
          onclick={handleSubmit}
          disabled={!formData.name.trim()}
          size="medium"
          keyboardHint={submitHint}
        >
          {isEditing ? t('timeProjectCategory.updateCategory') : t('timeProjectCategory.createCategory')}
        </Button>
        <Button
          variant="default"
          onclick={handleCancel}
          size="medium"
          keyboardHint="Esc"
        >
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  </Modal>
{/if}
