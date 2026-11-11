<script>
  // Drag-and-drop layout editor for one screen's fields. Shared by the Screens
  // admin page and the workflow designer's transition inspector, which needs to
  // add fields right after creating a screen. The parent owns the Save/Cancel
  // chrome and calls the exported save()/cancel().
  import { onDestroy } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { screenEditorStore } from '../stores/screenEditorStore.svelte.js';
  import { SYSTEM_FIELDS } from '../stores/fieldConfig.js';
  import { isAlwaysVisibleSystemField } from '../utils/screenFields.js';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import DragHandleDots from '../components/DragHandleDots.svelte';
  import DraggableFieldRow from '../components/DraggableFieldRow.svelte';
  import FieldRemovalAction from '../components/FieldRemovalAction.svelte';
  import { createFieldLayoutDrag } from './fieldLayoutDrag.js';

  let { screen } = $props();

  let screenFields = $derived(screenEditorStore.screenFields);
  let fieldSearchQuery = $derived(screenEditorStore.fieldSearchQuery);
  let searchFilteredFields = $derived(screenEditorStore.searchFilteredFields);
  let draggedField = $derived(screenEditorStore.draggedField);
  let fieldDragState = $derived(screenEditorStore.fieldDragState);

  const screenDrag = createFieldLayoutDrag({
    availableSelector: '[data-available-field]',
    availableDataKey: 'availableField',
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

  function setFieldSearchQuery(value) {
    screenEditorStore.fieldSearchQuery = value;
  }
</script>

<div class="grid grid-cols-1 lg:grid-cols-[1fr_340px] gap-6 mb-8" data-testid="screen-field-layout-editor">
  <!-- Screen Fields Configuration -->
  <div class="rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
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
            <div class="flex-1">
              <div class="font-medium flex items-center gap-2" style="color: var(--ds-text);">
                {screenEditorStore.getFieldDisplayName(field)}
                <span class="text-xs px-1.5 py-0.5 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-background-neutral);">
                  {field.field_type === 'system' ? t('screensPage.system') : t('screensPage.custom')}
                </span>
              </div>
            </div>

            <div class="flex items-center gap-3 flex-shrink-0">
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
      placeholder={t('screensPage.searchFields')}
      value={fieldSearchQuery}
      oninput={(e) => setFieldSearchQuery(e.target.value)}
      class="mb-4"
    />

    <div class="space-y-1 min-h-96 max-h-[70vh] overflow-y-auto">
      {#each searchFilteredFields as field, index}
        {#if index === 0 || field.category !== searchFilteredFields[index - 1].category}
          <div class="text-xs font-semibold uppercase tracking-wider mt-4 mb-2 first:mt-0" style="color: var(--ds-text-subtle);">
            {field.category === 'System Fields' ? t('screensPage.systemFields') : field.category === 'Custom Fields' ? t('screensPage.customFields') : field.category}
          </div>
        {/if}

        <div
          data-available-field={JSON.stringify(field)}
          class="available-field group flex items-center gap-3 px-4 py-3 rounded border transition-all duration-200 cursor-grab active:cursor-grabbing"
          style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
        >
          <!-- Drag Handle -->
          <div class="flex-shrink-0">
            <DragHandleDots class="w-4 h-4 group-hover:text-[var(--ds-icon-accent)]" color="var(--ds-text-subtle)" />
          </div>

          <div class="flex-1">
            <div class="font-medium" style="color: var(--ds-text);">{field.name}</div>
            <div class="text-sm" style="color: var(--ds-text-subtle);">
              {#if field.type === 'system'}
                {SYSTEM_FIELDS.find(sf => sf.identifier === field.identifier)?.type || field.identifier}
                • {t('screensPage.system')}
              {:else}
                {field.fieldType}
                • {t('screensPage.custom')}
              {/if}
            </div>
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

<style>
  .available-field:hover {
    background-color: var(--ds-background-input-hovered) !important;
    border-color: var(--ds-border-focused) !important;
  }
</style>
