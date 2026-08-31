<script>
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import LocalizedObjectFields from '../settings/LocalizedObjectFields.svelte';

  let {
    isOpen = false,
    formData = $bindable({
      name: '',
      color: '#3b82f6',
      description: '',
      is_default: false,
      is_completed: false
    }),
    isEditing = false,
    objectId = null,
    displayName = '',
    displayDescription = '',
    translationEditor = $bindable(null),
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
</script>

{#if isOpen}
  <Modal
    {isOpen}
    onSubmit={handleSubmit}
    submitDisabled={!formData.name.trim()}
    maxWidth="max-w-lg"
    onclose={handleCancel}
  >
    {#snippet children(submitHint)}
    <div class="p-6">
      <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
        {isEditing ? t('statusCategory.editStatusCategory') : t('statusCategory.createStatusCategory')}
      </h3>

      {#if isEditing}
        {#key objectId}
          <LocalizedObjectFields
            bind:this={translationEditor}
            objectType="status_category"
            {objectId}
            bind:canonicalName={formData.name}
            bind:canonicalDescription={formData.description}
            {displayName}
            {displayDescription}
          />
        {/key}
      {:else}
        <div class="mb-6">
          <Label required class="mb-2">{t('common.name')}</Label>
          <Input
            type="text"
            bind:value={formData.name}
            placeholder={t('statusCategory.namePlaceholder')}
            required
            size="medium"
          />
        </div>
      {/if}

      <div class="mb-6">
        <Label required class="mb-2">{t('statusCategory.color')}</Label>
        <IconSelector bind:selectedColor={formData.color} colorOnly compact />
      </div>

      {#if !isEditing}
        <div class="mb-6">
          <Label class="mb-2">{t('common.description')}</Label>
          <Textarea
            bind:value={formData.description}
            rows={2}
          />
        </div>
      {/if}

      <div class="mt-6 flex flex-col gap-4">
        <Checkbox
          bind:checked={formData.is_default}
          label={t('statusCategory.setAsDefault')}
          size="small"
        />

        <Checkbox
          bind:checked={formData.is_completed}
          label={t('statusCategory.marksWorkCompleted')}
          hint={t('statusCategory.marksWorkCompletedHelp')}
          size="small"
        />
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
    {/snippet}
  </Modal>
{/if}
