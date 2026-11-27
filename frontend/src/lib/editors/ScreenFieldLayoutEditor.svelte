<script>
  // Drag-and-drop layout editor for one screen's fields. Shared by the Screens
  // admin page and the workflow designer's transition inspector, which needs to
  // add fields right after creating a screen. The parent owns the Save/Cancel
  // chrome and calls the exported save()/cancel().
  import { onDestroy } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { screenEditorStore } from '../stores/screenEditorStore.svelte.js';
  import { isAlwaysVisibleSystemField } from '../utils/screenFields.js';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import DragHandleDots from '../components/DragHandleDots.svelte';
  import DraggableFieldRow from '../components/DraggableFieldRow.svelte';
  import FieldRemovalAction from '../components/FieldRemovalAction.svelte';
  import FieldTypeIcon from '../components/FieldTypeIcon.svelte';
  import Button from '../components/Button.svelte';
  import TextareaField from '../components/TextareaField.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import MessageSquareText from '@lucide/svelte/icons/message-square-text';
  import { SCREEN_FIELD_HINT_MAX_LENGTH } from '../utils/customFieldTypes.js';
  import { createFieldLayoutDrag, dragAvailableField } from './fieldLayoutDrag.js';

  let { screen } = $props();

  let screenFields = $derived(screenEditorStore.screenFields);
  let fieldSearchQuery = $derived(screenEditorStore.fieldSearchQuery);
  let searchFilteredFields = $derived(screenEditorStore.searchFilteredFields);
  let draggedField = $derived(screenEditorStore.draggedField);
  let fieldDragState = $derived(screenEditorStore.fieldDragState);

  const screenDrag = createFieldLayoutDrag({
    selectedSelector: '[data-screen-field]',
    dropZoneSelector: '[data-drop-zone]',
    selectedType: 'screen-field',
    onDragState: (fieldId, state) => screenEditorStore.setDragState(fieldId, state),
    clearDragState: () => screenEditorStore.clearDragState(),
    setDraggedField: (value) => {
      if (value) screenEditorStore.setDraggedField(true);
      else screenEditorStore.clearDraggedField();
    },
    addFieldAtPosition: (...args) => screenEditorStore.addFieldAtPosition(...args),
    reorderFieldWithEdge: (...args) => screenEditorStore.reorderFieldWithEdge(...args),
    addFieldToEnd: (field) => screenEditorStore.addFieldToScreen(field),
  });

  $effect(() => {
    const target = screen;
    if (target?.id) {
      screenEditorStore.startEditFields(target);
    }
  });

  // Re-arm drag and drop as the field list changes.
  $effect(() => {
    if (screenFields && typeof document !== 'undefined') {
      screenDrag.schedule();
    }
  });

  onDestroy(() => {
    screenDrag.cleanup();
    screenEditorStore.cancelFieldEditor();
  });

  export async function save() {
    const fields = await screenEditorStore.saveScreenFields();
    return {
      field_count: fields?.length ?? 0,
      required_field_count: (fields ?? []).filter((field) => field.is_required).length,
    };
  }

  export function cancel() {
    screenEditorStore.cancelFieldEditor();
  }

  function removeField(index) {
    screenEditorStore.removeField(index);
  }

  // Per-screen hint editor: edits a draft and writes it to the field on Save;
  // the screen itself is persisted by the parent's Save, like Required.
  let hintEditorIndex = $state(null);
  let hintDraft = $state('');
  const hintEditorField = $derived(hintEditorIndex === null ? null : screenFields[hintEditorIndex]);

  function openHintEditor(index) {
    hintEditorIndex = index;
    hintDraft = screenFields[index]?.hint || '';
  }

  function closeHintEditor() {
    hintEditorIndex = null;
    hintDraft = '';
  }

  function saveHint() {
    if (hintEditorIndex !== null) screenEditorStore.setFieldHint(hintEditorIndex, hintDraft);
    closeHintEditor();
  }

  function setFieldSearchQuery(value) {
    screenEditorStore.fieldSearchQuery = value;
  }
</script>

<div class="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_340px] gap-6 mb-8" data-testid="screen-field-layout-editor">
  <!-- Screen Fields Configuration (min-w-0 so long field names and hint
       previews truncate instead of widening the column) -->
  <div class="min-w-0 rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('screensPage.screenFields')} ({screenFields.length})</h3>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('screensPage.dragToReorder')}</p>

    <div
      data-drop-zone
      class="min-h-96 max-h-[70vh] overflow-y-auto border-2 border-dashed rounded p-4 space-y-2"
      style="border-color: {draggedField ? 'var(--ds-border-focused)' : 'var(--ds-border)'}; background-color: {draggedField ? 'var(--ds-interactive-subtle)' : 'transparent'};"
    >
      {#if screenFields.length === 0}
        <div class="text-center py-12">
          <svg class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-text-disabled);" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
          </svg>
          <p class="text-sm" style="color: var(--ds-text-subtle);">{t('screensPage.dropFieldsHere')}</p>
        </div>
      {:else}
        {#each screenFields as field, index (field.field_identifier)}
          <DraggableFieldRow
            attributes={{ 'data-screen-field': true, 'data-testid': `screen-field-${field.field_identifier}` }}
            fieldIndex={index}
            fieldId={field.field_identifier}
            closestEdge={fieldDragState.get(field.field_identifier)?.closestEdge}
            class="gap-3 px-4 py-3 rounded border hover:shadow-sm transition-all duration-200 h-16"
            style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); user-select: none;"
            handleClass="w-4 h-4 group-hover:text-[var(--ds-icon-accent)]"
            handleColor="var(--ds-text-subtle)"
          >
            <div class="flex-1 flex items-center gap-2 min-w-0">
              <FieldTypeIcon fieldType={screenEditorStore.getFieldTypeKey(field)} />
              <span class="font-medium truncate" style="color: var(--ds-text);">
                {screenEditorStore.getFieldDisplayName(field)}
              </span>
            </div>

            <div class="flex items-center gap-3 flex-shrink-0">
              {#if field.hint || screenEditorStore.canFieldHaveHint(field)}
                <Button
                  variant={field.hint ? 'secondary' : 'ghost'}
                  size="small"
                  icon={MessageSquareText}
                  title={t('screensPage.editHint')}
                  onclick={() => openHintEditor(index)}
                  dataTestid={`screen-field-hint-${field.field_identifier}`}
                />
              {/if}
              {#if field.placement === 'main' || screenEditorStore.canFieldUseMainColumn(field)}
                <span title={t('screensPage.mainColumnHint')} data-testid={`screen-field-main-column-${field.field_identifier}`}>
                  <Checkbox
                    checked={field.placement === 'main'}
                    onchange={() => screenEditorStore.toggleFieldPlacement(index)}
                    label={t('screensPage.mainColumn')}
                    size="small"
                  />
                </span>
              {/if}
              <Checkbox
                checked={field.is_required}
                disabled={!field.is_required && !screenEditorStore.canFieldBeRequired(field)}
                onchange={() => screenEditorStore.toggleFieldRequired(index)}
                label={t('screensPage.required')}
                size="small"
              />

              <FieldRemovalAction
                locked={field.field_type === 'system' && isAlwaysVisibleSystemField(field.field_identifier)}
                lockedClass="w-9 h-9 flex items-center justify-center flex-shrink-0"
                lockedColor="var(--ds-text-disabled)"
                removeClass="transition-colors p-1 rounded hover-danger flex-shrink-0"
                removeStyle="color: var(--ds-icon-danger);"
                removeTitle={t('screensPage.removeField')}
                onremove={() => removeField(index)}
              />
            </div>
          </DraggableFieldRow>
        {/each}
      {/if}
    </div>
  </div>

  <!-- Available Fields -->
  <div class="rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('screensPage.availableFields')}</h3>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('screensPage.dragFieldsHint')}</p>

    <Input
      dataTestid="screen-field-search"
      placeholder={t('screensPage.searchFields')}
      value={fieldSearchQuery}
      oninput={(e) => setFieldSearchQuery(e.target.value)}
      class="mb-4"
    />

    <div class="space-y-1 min-h-96 max-h-[70vh] overflow-y-auto">
      {#each searchFilteredFields as field, index (field.identifier)}
        {#if index === 0 || field.category !== searchFilteredFields[index - 1].category}
          <div class="text-xs font-semibold uppercase tracking-wider mt-4 mb-2 first:mt-0" style="color: var(--ds-text-subtle);">
            {field.category === 'System Fields' ? t('screensPage.systemFields') : field.category === 'Custom Fields' ? t('screensPage.customFields') : field.category}
          </div>
        {/if}

        <div
          use:dragAvailableField={() => field}
          data-testid={`screen-available-field-${field.identifier}`}
          class="available-field group flex items-center gap-3 px-4 py-3 rounded border transition-all duration-200 cursor-grab active:cursor-grabbing"
          style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
        >
          <!-- Drag Handle -->
          <div class="flex-shrink-0">
            <DragHandleDots class="w-4 h-4 group-hover:text-[var(--ds-icon-accent)]" color="var(--ds-text-subtle)" />
          </div>

          <div class="flex-1 flex items-center gap-2 min-w-0">
            <FieldTypeIcon fieldType={screenEditorStore.getFieldTypeKey(field)} />
            <span class="font-medium truncate" style="color: var(--ds-text);">{field.name}</span>
          </div>
        </div>
      {/each}

      {#if searchFilteredFields.length === 0}
        <div class="text-center py-8">
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {#if fieldSearchQuery.trim()}
              {t('screensPage.noFieldsMatch')}
            {:else}
              {t('screensPage.allFieldsAdded')}
            {/if}
          </p>
        </div>
      {/if}
    </div>
  </div>
</div>


<!-- Per-screen hint editor -->
<Modal isOpen={hintEditorIndex !== null} onclose={closeHintEditor} onSubmit={saveHint} maxWidth="max-w-lg">
  {#snippet children()}
  <ModalHeader
    title={t('screensPage.hintTitle', { field: hintEditorField ? screenEditorStore.getFieldDisplayName(hintEditorField) : '' })}
    showCloseButton={true}
    onclose={closeHintEditor}
  />
  <div class="px-6 py-4 space-y-2">
    <TextareaField
      label={t('screensPage.hintLabel')}
      id="screen-field-hint"
      labelColor="default"
      rows={4}
      maxlength={SCREEN_FIELD_HINT_MAX_LENGTH}
      helper={`${t('screensPage.hintHelper')} · ${hintDraft.length}/${SCREEN_FIELD_HINT_MAX_LENGTH}`}
      dataTestid="screen-field-hint-input"
      bind:value={hintDraft}
    />
    {#if hintDraft}
      <Button variant="link" size="small" onclick={() => (hintDraft = '')} dataTestid="screen-field-hint-clear">
        {t('screensPage.clearHint')}
      </Button>
    {/if}
  </div>
  <DialogFooter
    onCancel={closeHintEditor}
    onConfirm={saveHint}
    confirmLabel={t('common.save')}
    showKeyboardHint={true}
  />
  {/snippet}
</Modal>

<style>
  .available-field:hover {
    background-color: var(--ds-background-input-hovered) !important;
    border-color: var(--ds-border-focused) !important;
  }
</style>
