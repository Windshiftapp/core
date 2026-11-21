<script>
  // Long-form custom fields (multi-line and rich text) whose screen placement is
  // "main" render here, between the description and its action row, instead of
  // in the sidebar. The label is the edit affordance so links in rendered
  // content stay clickable; an empty field mirrors the description's empty
  // state. Editing mirrors the description (explicit Save/Cancel) rather than
  // the sidebar's commit-on-outside-click, which is too eager for a wide,
  // multi-paragraph editor surrounded by other sections.
  import Text from '../../components/Text.svelte';
  import CustomFieldRenderer from './CustomFieldRenderer.svelte';
  import ItemDetailMainFieldEditor from './ItemDetailMainFieldEditor.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { isMainColumnScreenField } from '../../utils/screenFields.js';

  let {
    item,
    canEdit = false,
    workspaceScreenFields = [],
    customFieldDefinitions = [],
    editableScreenFieldIds = null,
    editingCustomFields = {},
    saving = false,
    onsaveField = null,
    oncancelEdit = null,
    onstartEditingCustomField = null,
  } = $props();

  let mainFields = $derived(
    (workspaceScreenFields || [])
      .filter(isMainColumnScreenField)
      .map((screenField) => ({
        screenField,
        fieldDef: customFieldDefinitions.find(
          (field) => field.id === parseInt(screenField.field_identifier, 10)
        ),
      }))
      .filter(({ fieldDef }) => !!fieldDef)
  );

  function isEditable(fieldId) {
    return !editableScreenFieldIds || editableScreenFieldIds.has(fieldId);
  }

  function isEmpty(value) {
    return value === undefined || value === null || String(value).trim() === '';
  }

  function startEditing(screenField, fieldDef) {
    if (!canEdit || !isEditable(fieldDef.id)) return;
    onstartEditingCustomField?.({ fieldId: screenField.field_identifier });
  }
</script>

{#each mainFields as { screenField, fieldDef } (screenField.field_identifier)}
  {@const identifier = screenField.field_identifier}
  {@const isEditing = editingCustomFields[identifier]}
  {@const fieldEditable = canEdit && isEditable(fieldDef.id)}
  {@const storedValue = item.custom_field_values?.[identifier]}
  {@const emptyText = t('items.setField', { field: fieldDef.name.toLowerCase() })}
  <section class="mt-5" data-testid={`item-main-custom-field-${fieldDef.id}`}>
    <button
      type="button"
      onclick={() => startEditing(screenField, fieldDef)}
      data-testid={`item-main-custom-field-edit-${fieldDef.id}`}
      class="mb-1.5 rounded-sm text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ds-border-focused disabled:cursor-default"
      disabled={!fieldEditable}
    >
      <Text weight="semibold" size="sm">{fieldDef.name}</Text>
    </button>
    {#if isEditing}
      <ItemDetailMainFieldEditor
        field={fieldDef}
        initialValue={storedValue}
        workspaceId={item?.workspace_id}
        {saving}
        onsave={(value) => onsaveField?.({ field: `custom_field_${identifier}`, value })}
        oncancel={() => oncancelEdit?.({ field: `custom_field_${identifier}` })}
      />
    {:else if isEmpty(storedValue)}
      {#if fieldEditable}
        <button
          type="button"
          onclick={() => startEditing(screenField, fieldDef)}
          class="field-empty text-start text-[0.875rem] transition-colors cursor-pointer"
          data-testid={`item-main-custom-field-empty-${fieldDef.id}`}
        >
          {emptyText}
        </button>
      {:else}
        <!-- Read-only empty state: no edit affordance, so avoid the
             imperative "Set X" the editable button uses. -->
        <p
          class="text-start w-full py-2 text-[0.875rem] text-ds-text-subtle"
          data-testid={`item-main-custom-field-empty-${fieldDef.id}`}
        >
          {t('items.notSet')}
        </p>
      {/if}
    {:else}
      <!-- Display only; editing always goes through the editor above. -->
      <CustomFieldRenderer
        field={fieldDef}
        value={storedValue}
        readonly={true}
        disabled={!fieldEditable}
        renderRichText
        noPadding
        itemId={item?.id}
        workspaceId={item?.workspace_id}
        onStartEdit={() => startEditing(screenField, fieldDef)}
      />
    {/if}
  </section>
{/each}

<style>
  /* Matches the description's empty state: same 8px-outset hover pill. */
  .field-empty {
    width: calc(100% + 1rem);
    margin: 0 -0.5rem;
    padding: 0.5rem;
    border-radius: 0.25rem;
    color: var(--ds-text-subtle);
  }

  .field-empty:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
