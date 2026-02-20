<script>
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';

  let {
    formData = $bindable({
      name: '',
      key: '',
      description: ''
    }),
    nameInputRef = $bindable(null)
  } = $props();

  let keyManuallyEdited = false;

  function generateKey(name) {
    const words = name.trim().split(/\s+/).filter(Boolean);
    if (words.length === 0) return '';
    if (words.length === 1) {
      return words[0].substring(0, 2).toUpperCase();
    }
    return words.map(w => w[0]).join('').substring(0, 5).toUpperCase();
  }

  function onNameInput() {
    if (!keyManuallyEdited) {
      formData.key = generateKey(formData.name);
    }
  }

  function onKeyInput(e) {
    keyManuallyEdited = true;
    formData.key = e.target.value.toUpperCase();
  }

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
    keyManuallyEdited = false;
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
    oninput={onNameInput}
    type="text"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.workspaceName', { type: t('createModal.workspace') })}
  />

  <!-- Workspace Key -->
  <input
    bind:value={formData.key}
    oninput={onKeyInput}
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
