<script>
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/MilkdownEditor.svelte';

  let {
    formData = $bindable({
      name: '',
      key: '',
      description: ''
    }),
    nameInputRef = $bindable(null)
  } = $props();

  export function validate() {
    return formData.name.trim() !== '' && formData.key.trim() !== '';
  }

  export function getFormData() {
    return {
      name: formData.name,
      key: formData.key,
      description: formData.description || '',
      active: true
    };
  }

  export function reset() {
    formData = {
      name: '',
      key: '',
      description: ''
    };
  }

  export function isValid() {
    return formData.name.trim() !== '' && formData.key.trim() !== '';
  }
</script>

<div class="space-y-3">
  <!-- Title Input -->
  <input
    bind:this={nameInputRef}
    bind:value={formData.name}
    type="text"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.workspaceName', { type: t('createModal.workspace') })}
  />

  <!-- Workspace Key -->
  <input
    bind:value={formData.key}
    type="text"
    class="w-full text-sm border-0 outline-none bg-transparent"
    style="color: var(--ds-text-subtle);"
    placeholder={t('createModal.workspaceKeyPlaceholder')}
  />

  <!-- Description -->
  <div class="min-h-[60px]">
    <MilkdownEditor
      bind:content={formData.description}
      placeholder={t('createModal.addDescription')}
      compact={true}
      showToolbar={false}
      readonly={false}
      itemId={null}
    />
  </div>
</div>
