<script>
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import ChipPicker from '../pickers/ChipPicker.svelte';
  import Spinner from '../components/Spinner.svelte';
  import LayoutTemplate from '@lucide/svelte/icons/layout-template';
  import Package from '@lucide/svelte/icons/package';

  let {
    formData = $bindable({
      name: '',
      key: '',
      description: '',
      template_workspace_id: null,
      template_pack: '',
      restricted_to_creator: false
    }),
    templates = [],
    templatesLoading = false,
    templatesError = null,
    packs = [],
    packsLoading = false,
    packsError = null,
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

  // One picker, two template sources: server-embedded packs (fixed by name)
  // and live workspaces marked as templates (cloned by ID). Values are
  // prefixed so the two ID spaces never collide.
  let templatePickerItems = $derived.by(() => {
    const blank = {
      value: 'blank',
      kind: 'blank',
      name: t('createModal.workspaceTemplateBlank')
    };
    const packItems = (packs || []).map((pack) => ({
      value: `pack:${pack.name}`,
      kind: 'pack',
      name: pack.name,
      version: pack.version,
      hasContent: pack.has_content
    }));
    const workspaceItems = (templates || []).map((tpl) => ({
      value: `workspace:${tpl.id}`,
      kind: 'template',
      id: tpl.id,
      name: tpl.name,
      template_count: tpl.template_count,
      item_count: tpl.item_count
    }));
    return [blank, ...packItems, ...workspaceItems];
  });

  let pickerValue = $derived.by(() => {
    if (formData.template_pack) return `pack:${formData.template_pack}`;
    if (formData.template_workspace_id != null) {
      return `workspace:${formData.template_workspace_id}`;
    }
    return 'blank';
  });

  function onTemplateSelect(item) {
    if (item?.kind === 'pack') {
      formData.template_pack = item.name;
      formData.template_workspace_id = null;
    } else if (item?.kind === 'template') {
      formData.template_workspace_id = item.id ?? null;
      formData.template_pack = '';
    } else {
      formData.template_pack = '';
      formData.template_workspace_id = null;
    }
  }

  export function validate() {
    return formData.name.trim() !== '' && formData.key.trim() !== '';
  }

  export function getFormData() {
    return {
      name: formData.name,
      key: formData.key,
      description: formData.description || '',
      active: true,
      template_workspace_id: formData.template_workspace_id ?? null,
      template_pack: formData.template_pack || null,
      restricted_to_creator: formData.restricted_to_creator === true
    };
  }

  export function reset() {
    formData = {
      name: '',
      key: '',
      description: '',
      template_workspace_id: null,
      template_pack: '',
      restricted_to_creator: false
    };
    keyManuallyEdited = false;
  }

  export function isValid() {
    return formData.name.trim() !== '' && formData.key.trim() !== '';
  }
</script>

<div class="space-y-3">
  <!-- Title Input -->
  <Input
    id="workspace-name"
    bind:inputRef={nameInputRef}
    bind:value={formData.name}
    oninput={onNameInput}
    type="text"
    variant="ghost"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.workspaceName', { type: t('createModal.workspace') })}
  />

  <!-- Workspace Key -->
  <Input
    id="workspace-key"
    bind:value={formData.key}
    oninput={onKeyInput}
    type="text"
    variant="ghost"
    class="w-full text-sm border-0 outline-none bg-transparent"
    style="color: var(--ds-text-subtle);"
    placeholder={t('createModal.workspaceKeyPlaceholder')}
  />

  <!-- Template picker: blank workspace is the default and preserves the
       legacy creation flow; built-in packs and live template workspaces are
       the two provisioning sources. -->
  {#if templatesLoading || packsLoading}
    <div
      data-testid="workspace-template-loading"
      class="flex items-center gap-2 text-sm px-2 py-1.5 rounded"
      style="color: var(--ds-text-subtle);"
    >
      <Spinner size="sm" />
      <span>{t('createModal.workspaceTemplateLoading')}</span>
    </div>
  {:else if templatesError || packsError}
    <div
      data-testid="workspace-template-error"
      class="text-sm px-2 py-1.5 rounded"
      style="color: var(--ds-text-danger, #dc2626);"
    >
      {t('createModal.workspaceTemplateError')}
    </div>
  {:else if templatePickerItems.length > 0}
    <div data-testid="workspace-template-section" class="pt-1">
      <ChipPicker
        value={pickerValue}
        items={templatePickerItems}
        getValue={(item) => item.value}
        getLabel={(item) => item.name}
        icon={LayoutTemplate}
        placeholder={t('createModal.workspaceTemplate')}
        searchable={true}
        searchFields={['name']}
        testId="workspace-template-picker"
        onSelect={onTemplateSelect}
      >
        {#snippet itemSnippet({ item })}
          {#if item.kind === 'pack'}
            <Package size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          {:else}
            <LayoutTemplate size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          {/if}
          <span class="truncate">{item.name}</span>
          {#if item.kind === 'pack'}
            <span
              class="text-xs ml-auto pl-2 flex-shrink-0"
              style="color: var(--ds-text-subtle);"
            >
              {t('createModal.workspacePackTemplate')}
            </span>
          {:else if item.kind === 'template'}
            <span
              class="text-xs ml-auto pl-2 flex-shrink-0"
              style="color: var(--ds-text-subtle);"
            >
              {t('createModal.workspaceTemplateMeta', {
                templates: item.template_count ?? 0,
                items: item.item_count ?? 0
              })}
            </span>
          {/if}
        {/snippet}
      </ChipPicker>
    </div>
  {/if}

  <!-- Restrict visibility: gates the workspace to assigned users from the
       first moment by granting the creator Viewer on creation. -->
  <div class="pt-1">
    <Checkbox
      bind:checked={formData.restricted_to_creator}
      label={t('createModal.workspaceRestrict')}
      hint={t('createModal.workspaceRestrictHint')}
      dataTestid="workspace-restrict-checkbox"
    />
  </div>

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
