<script>
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Textarea from '../components/Textarea.svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  // Props
  export let isOpen = false;
  export let formData = {
    name: '',
    color: '#3b82f6',
    description: '',
    is_default: false,
    is_completed: false
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
        {isEditing ? t('statusCategory.editStatusCategory') : t('statusCategory.createStatusCategory')}
      </h3>

      <div class="mb-6">
        <Label required class="mb-2">{t('common.name')}</Label>
        <input
          type="text"
          bind:value={formData.name}
          placeholder={t('statusCategory.namePlaceholder')}
          class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
          style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          required
        />
      </div>

      <div class="mb-6">
        <Label required class="mb-2">{t('statusCategory.color')}</Label>
        <ColorPicker bind:value={formData.color} />
      </div>

      <div class="mb-6">
        <Label class="mb-2">{t('common.description')}</Label>
        <Textarea
          bind:value={formData.description}
          rows={2}
        />
      </div>

      <div class="mt-6 flex flex-col gap-4">
        <label class="inline-flex items-center gap-3 text-sm font-medium" style="color: var(--ds-text);">
          <input
            type="checkbox"
            bind:checked={formData.is_default}
            id="is-default"
            class="w-4 h-4 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span>{t('statusCategory.setAsDefault')}</span>
        </label>

        <div class="flex items-start gap-3">
          <input
            type="checkbox"
            bind:checked={formData.is_completed}
            id="is-completed"
            class="mt-1 w-4 h-4 text-emerald-600 rounded focus:ring-2 focus:ring-emerald-500"
          />
          <div>
            <label for="is-completed" class="text-sm font-medium" style="color: var(--ds-text);">
              {t('statusCategory.marksWorkCompleted')}
            </label>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              {t('statusCategory.marksWorkCompletedHelp')}
            </p>
          </div>
        </div>
      </div>

      <div class="mt-8 flex gap-3">
        <Button
          variant="primary"
          onclick={handleSubmit}
          disabled={!formData.name.trim()}
          size="medium"
          keyboardHint={submitHint}
        >
          {isEditing ? t('statusCategory.updateCategory') : t('statusCategory.createCategory')}
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

