<script>
  import { onMount, onDestroy } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import {
    IconGripVertical, IconTrash, IconAsterisk, IconSearch, IconPlus, IconDeviceFloppy,
    IconSettings, IconArrowLeft, IconTextSize, IconForms, IconCheckbox, IconSelect, IconAlignBoxLeftTop,
    IconPencil
  } from '@tabler/icons-svelte-runes';
  import { t } from '../../stores/i18n.svelte.js';
  import { formBuilderStore } from '../../stores/formBuilderStore.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import FormFieldPalette from './FormFieldPalette.svelte';

  let { channelId, onBack = () => {}, onCreateForm = null, embedded = true } = $props();

  let saving = $state(false);
  let showSettings = $state(false);
  let setupCleanups = [];
  let expandedFields = $state(new Set());

  function toggleFieldExpanded(fieldKey) {
    const next = new Set(expandedFields);
    if (next.has(fieldKey)) next.delete(fieldKey);
    else next.add(fieldKey);
    expandedFields = next;
  }

  onMount(async () => {
    await formBuilderStore.loadForms(channelId);
  });

  onDestroy(() => {
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
  });

  function setupDragAndDrop() {
    // Clean up previous
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];

    // Setup available fields as draggable
    document.querySelectorAll('[data-available-field]').forEach(element => {
      const fieldDataStr = element.dataset.availableField;
      if (!fieldDataStr) return;
      const fieldData = JSON.parse(fieldDataStr);
      const cleanup = draggable({
        element,
        getInitialData: () => ({ field: fieldData, type: 'available-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => { element.style.opacity = ''; }
      });
      setupCleanups.push(cleanup);
    });

    // Setup form fields as draggable + drop targets
    document.querySelectorAll('[data-form-field]').forEach(element => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.formField;

      // Make draggable
      const dragCleanup = draggable({
        element,
        getInitialData: () => ({ fieldIndex, type: 'form-field' }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          formBuilderStore.setDraggedField(fieldId);
        },
        onDrop: () => {
          element.style.opacity = '';
          formBuilderStore.clearDraggedField();
        }
      });
      setupCleanups.push(dragCleanup);

      // Make drop target
      const dropCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'form-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'available-field' || data.type === 'form-field';
        },
        getData: ({ input, element: el }) => {
          return attachClosestEdge({}, { input, element: el, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          formBuilderStore.setDragState(fieldId, { closestEdge });
        },
        onDragLeave: () => {
          formBuilderStore.setDragState(fieldId, { closestEdge: null });
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          const data = source.data;

          if (data.type === 'available-field') {
            formBuilderStore.addFieldAtPosition(data.field, fieldIndex, closestEdge);
          } else if (data.type === 'form-field') {
            formBuilderStore.reorderField(data.fieldIndex, fieldIndex, closestEdge);
          }

          formBuilderStore.clearDragState();
        }
      });
      setupCleanups.push(dropCleanup);
    });

    // Setup drop zone for empty canvas
    const emptyDropZone = document.querySelector('[data-form-drop-zone]');
    if (emptyDropZone) {
      const dropCleanup = dropTargetForElements({
        element: emptyDropZone,
        canDrop: ({ source }) => source.data.type === 'available-field',
        onDrop: ({ source }) => {
          formBuilderStore.addField(source.data.field);
          formBuilderStore.clearDragState();
        }
      });
      setupCleanups.push(dropCleanup);
    }
  }

  // Re-setup drag and drop when fields change
  $effect(() => {
    if (formBuilderStore.showFieldEditor && formBuilderStore.formFields) {
      // Use microtask to ensure DOM is updated
      queueMicrotask(() => setupDragAndDrop());
    }
  });

  async function handleSave() {
    try {
      saving = true;
      await formBuilderStore.saveFormFields();
      await formBuilderStore.saveFormConfig();
      successToast(t('common.saved'));
    } catch (err) {
      errorToast(err.message || t('common.error'));
    } finally {
      saving = false;
    }
  }

  function handleBack() {
    if (formBuilderStore.showFieldEditor) {
      formBuilderStore.cancelFieldEditor();
    } else {
      onBack();
    }
  }

  async function handleDeleteForm(form, event) {
    event.stopPropagation();
    const ok = await confirm({
      title: t('forms.deleteForm'),
      message: t('forms.confirmDeleteForm', { name: form.name }),
      confirmText: t('forms.deleteForm'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await formBuilderStore.deleteForm(form.id);
      successToast(t('forms.formDeleted'));
    } catch (err) {
      errorToast(err.message || t('common.error'));
    }
  }

  function getFieldTypeIcon(field) {
    if (field.field_type === 'virtual') {
      const type = field.virtual_field_type;
      if (type === 'textarea') return IconAlignBoxLeftTop;
      if (type === 'select') return IconSelect;
      if (type === 'checkbox') return IconCheckbox;
      return IconTextSize;
    }
    if (field.field_type === 'default') return IconForms;
    return IconTextSize;
  }

  function getFieldTypeBadge(field) {
    if (field.field_type === 'default') return 'Default';
    if (field.field_type === 'custom') return 'Custom';
    if (field.field_type === 'virtual') return field.virtual_field_type || 'Virtual';
    return field.field_type;
  }

  function parseOptionsJson(json) {
    if (!json) return [];
    try {
      const parsed = JSON.parse(json);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }

  function writeOptions(fieldIdx, options) {
    formBuilderStore.updateFieldProperty(fieldIdx, 'virtual_field_options', JSON.stringify(options));
  }

  function addOption(fieldIdx) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    opts.push({ value: '', label: '' });
    writeOptions(fieldIdx, opts);
  }

  function removeOption(fieldIdx, optIdx) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    opts.splice(optIdx, 1);
    writeOptions(fieldIdx, opts);
  }

  function updateOptionLabel(fieldIdx, optIdx, value) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    const prev = opts[optIdx] || { value: '', label: '' };
    // If value was auto-synced from label, keep them in sync
    const autoSync = !prev.value || prev.value === prev.label;
    opts[optIdx] = { value: autoSync ? value : prev.value, label: value };
    writeOptions(fieldIdx, opts);
  }

  function updateOptionValue(fieldIdx, optIdx, value) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    const prev = opts[optIdx] || { value: '', label: '' };
    opts[optIdx] = { ...prev, value };
    writeOptions(fieldIdx, opts);
  }
</script>

<div class="h-full flex flex-col">
  <!-- Header (shown when embedded or when in field editor) -->
  {#if embedded || formBuilderStore.showFieldEditor}
  <div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3">
      <Button onclick={handleBack} variant="ghost" size="small" icon={IconArrowLeft} />
      <div>
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          {#if formBuilderStore.showFieldEditor}
            {formBuilderStore.editingForm?.name} - {t('forms.builder.title')}
          {:else}
            {t('forms.title')}
          {/if}
        </h3>
      </div>
    </div>
    {#if formBuilderStore.showFieldEditor}
      <div class="flex items-center gap-2">
        <Button
          onclick={() => showSettings = !showSettings}
          variant="default"
          size="small"
          icon={IconSettings}
        >
          {t('forms.settings.title')}
        </Button>
        <Button
          onclick={handleSave}
          variant="primary"
          size="small"
          icon={IconDeviceFloppy}
          disabled={saving}
        >
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    {/if}
  </div>
  {/if}

  {#if formBuilderStore.loading}
    <div class="flex-1 flex items-center justify-center">
      <Spinner />
    </div>
  {:else if formBuilderStore.showFieldEditor}
    <!-- Field Editor: Two-panel layout -->
    <div class="flex-1 flex overflow-hidden">
      <!-- Left: Form Canvas -->
      <div class="flex-1 overflow-y-auto p-6">
        {#if showSettings}
          <!-- Per-form Settings -->
          <div class="max-w-xl mx-auto space-y-4">
            <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('forms.settings.title')}</h4>

            <div class="flex items-center gap-3">
              <input type="checkbox" bind:checked={formBuilderStore.formConfig.require_auth} class="rounded" />
              <Label color="default">{t('forms.settings.requireAuth')}</Label>
            </div>

            <div>
              <Label color="default" class="mb-2">{t('forms.settings.submitButton')}</Label>
              <Input bind:value={formBuilderStore.formConfig.submit_button_text} placeholder="Submit" />
            </div>

            <div>
              <Label color="default" class="mb-2">{t('forms.settings.successMessage')}</Label>
              <Input bind:value={formBuilderStore.formConfig.success_message} placeholder={t('forms.settings.successMessagePlaceholder')} />
            </div>

            <div>
              <Label color="default" class="mb-2">{t('forms.settings.redirectUrl')}</Label>
              <Input bind:value={formBuilderStore.formConfig.redirect_url} placeholder="https://example.com/thank-you" />
            </div>

            <Button onclick={() => showSettings = false} variant="default" size="small">
              {t('forms.builder.backToFields')}
            </Button>
          </div>
        {:else}
          <!-- Field Canvas -->
          <div class="max-w-xl mx-auto">
            {#if formBuilderStore.formFields.length === 0}
              <div
                data-form-drop-zone
                class="border-2 border-dashed rounded-lg p-12 text-center"
                style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
              >
                <IconForms class="w-12 h-12 mx-auto mb-3" />
                <p class="text-sm font-medium">{t('forms.builder.dropFieldsHere')}</p>
                <p class="text-xs mt-1">{t('forms.builder.dragFromPalette')}</p>
              </div>
            {:else}
              <div class="space-y-2" data-form-drop-zone>
                {#each formBuilderStore.formFields as field, index}
                  {@const dragState = formBuilderStore.fieldDragState.get(field.field_identifier + '_' + index)}
                  {@const isVirtualSelect = field.field_type === 'virtual' && field.virtual_field_type === 'select'}
                  {@const fieldKey = field.field_identifier + '_' + index}
                  {@const isExpanded = expandedFields.has(fieldKey)}
                  <div>
                  <div
                    data-form-field={field.field_identifier + '_' + index}
                    data-field-index={index}
                    class="group relative flex items-center gap-3 p-3 rounded-lg border transition-colors cursor-grab"
                    style="background-color: var(--ds-surface); border-color: var(--ds-border);"
                  >
                    <!-- Drop edge indicators -->
                    {#if dragState?.closestEdge === 'top'}
                      <div class="absolute top-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]" style="transform: translateY(-50%);"></div>
                    {/if}
                    {#if dragState?.closestEdge === 'bottom'}
                      <div class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]" style="transform: translateY(50%);"></div>
                    {/if}

                    <!-- Drag handle -->
                    <IconGripVertical class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-disabled);" />

                    <!-- Field info -->
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="text-sm font-medium truncate" style="color: var(--ds-text);">
                          {field.display_name || field.field_label || field.field_identifier}
                        </span>
                        <span class="px-1.5 py-0.5 text-[10px] rounded font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {getFieldTypeBadge(field)}
                        </span>
                      </div>
                      {#if field.description}
                        <p class="text-xs mt-0.5 truncate" style="color: var(--ds-text-subtle);">{field.description}</p>
                      {/if}
                    </div>

                    <!-- Edit toggle -->
                    <button
                      onclick={() => toggleFieldExpanded(fieldKey)}
                      class="p-1 rounded transition-colors"
                      style="color: {isExpanded ? 'var(--ds-interactive)' : 'var(--ds-text-disabled)'};"
                      title="Edit label and help text"
                    >
                      <IconPencil class="w-4 h-4" />
                    </button>

                    <!-- Required toggle -->
                    <button
                      onclick={() => formBuilderStore.toggleFieldRequired(index)}
                      class="p-1 rounded transition-colors"
                      style="color: {field.is_required ? 'var(--ds-text-danger)' : 'var(--ds-text-disabled)'};"
                      title={field.is_required ? t('forms.builder.required') : t('forms.builder.optional')}
                    >
                      <IconAsterisk class="w-4 h-4" />
                    </button>

                    <!-- Remove button -->
                    <button
                      onclick={() => formBuilderStore.removeField(index)}
                      class="p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity hover:bg-[var(--ds-background-danger-hovered)]"
                      style="color: var(--ds-text-danger);"
                      title={t('common.remove')}
                    >
                      <IconTrash class="w-4 h-4" />
                    </button>
                  </div>

                  {#if isExpanded || isVirtualSelect}
                    {@const options = parseOptionsJson(field.virtual_field_options)}
                    <div class="mt-1 ml-7 pl-4 py-2 border-l-2 space-y-3" style="border-color: var(--ds-border);">
                      {#if isExpanded}
                        <div>
                          <Label color="default" class="mb-1">Label</Label>
                          <input
                            type="text"
                            value={field.display_name ?? ''}
                            oninput={(e) => formBuilderStore.updateFieldProperty(index, 'display_name', e.currentTarget.value)}
                            placeholder={field.field_name || field.field_identifier}
                            class="w-full px-2 py-1 text-sm rounded border"
                            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                          />
                        </div>
                        <div>
                          <Label color="default" class="mb-1">Help text</Label>
                          <textarea
                            value={field.description ?? ''}
                            oninput={(e) => formBuilderStore.updateFieldProperty(index, 'description', e.currentTarget.value)}
                            rows="2"
                            placeholder="Optional instructions shown below the field"
                            class="w-full px-2 py-1 text-sm rounded border"
                            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                          ></textarea>
                        </div>
                      {/if}

                      {#if isVirtualSelect}
                      <div class="text-xs font-semibold" style="color: var(--ds-text-subtle);">Options</div>
                      {#each options as opt, optIdx (optIdx)}
                        <div class="flex items-center gap-2">
                          <input
                            type="text"
                            value={opt.label ?? ''}
                            oninput={(e) => updateOptionLabel(index, optIdx, e.currentTarget.value)}
                            placeholder="Label"
                            class="flex-1 px-2 py-1 text-sm rounded border"
                            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                          />
                          <input
                            type="text"
                            value={opt.value ?? ''}
                            oninput={(e) => updateOptionValue(index, optIdx, e.currentTarget.value)}
                            placeholder="value"
                            class="w-32 px-2 py-1 text-sm rounded border"
                            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                          />
                          <button
                            onclick={() => removeOption(index, optIdx)}
                            class="p-1 rounded hover:bg-[var(--ds-background-danger-hovered)]"
                            title={t('common.remove')}
                          >
                            <IconTrash class="w-3.5 h-3.5" style="color: var(--ds-text-danger);" />
                          </button>
                        </div>
                      {/each}
                      <button
                        onclick={() => addOption(index)}
                        class="text-xs font-medium flex items-center gap-1"
                        style="color: var(--ds-interactive);"
                      >
                        <IconPlus class="w-3.5 h-3.5" /> Add option
                      </button>
                      {/if}
                    </div>
                  {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Right: Field Palette -->
      {#if !showSettings}
        <FormFieldPalette />
      {/if}
    </div>
  {:else}
    <!-- Form List -->
    <div class="flex-1 overflow-y-auto p-6">
      {#if formBuilderStore.forms.length === 0}
        <EmptyState
          icon={IconForms}
          title={t('forms.builder.noForms')}
          description={t('forms.builder.addFormHint')}
        >
          {#snippet action()}
            {#if onCreateForm}
              <Button onclick={onCreateForm} variant="primary" size="small" icon={IconPlus}>
                {t('forms.createForm')}
              </Button>
            {/if}
          {/snippet}
        </EmptyState>
      {:else}
        <div class="space-y-2 max-w-2xl mx-auto">
          {#if onCreateForm}
            <div class="flex justify-end mb-2">
              <Button onclick={onCreateForm} variant="primary" size="small" icon={IconPlus}>
                {t('forms.createForm')}
              </Button>
            </div>
          {/if}
          {#each formBuilderStore.forms as form}
            <div
              onclick={() => formBuilderStore.startEditFields(form)}
              class="group w-full flex items-center gap-4 p-4 rounded-lg border transition-colors hover:bg-[var(--ds-background-neutral-hovered)] cursor-pointer"
              style="border-color: var(--ds-border);"
            >
              <div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background-color: {form.color}20;">
                <IconForms class="w-5 h-5" style="color: {form.color};" />
              </div>
              <div class="flex-1 text-left">
                <div class="text-sm font-medium" style="color: var(--ds-text);">{form.name}</div>
                {#if form.description}
                  <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{form.description}</div>
                {/if}
              </div>
              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('forms.builder.editFields')}</span>
              <Button
                onclick={(e) => handleDeleteForm(form, e)}
                variant="ghost"
                size="small"
                icon={IconTrash}
                title={t('forms.deleteForm')}
                class="opacity-0 group-hover:opacity-100 !text-[var(--ds-text-danger)]"
              />
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>
