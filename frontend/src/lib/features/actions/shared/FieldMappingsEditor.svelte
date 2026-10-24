<script>
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import HelpCircle from '@lucide/svelte/icons/help-circle';
  import { t } from '../../../stores/i18n.svelte.js';
  import Select from '../../../components/Select.svelte';
  import FieldSelector from '../../../pickers/FieldSelector.svelte';
  import Input from '../../../components/Input.svelte';
  import { getFieldSelectorValue, backendFieldName } from './fieldNameMapping.js';

  let {
    mappings = [],
    targetFields = [],
    showPlaceholderModal = $bindable(false),
    onchange = () => {},
  } = $props();

  const sourceTypes = [
    { value: 'variable', label: t('actions.config.sourceTypeVariable') },
    { value: 'item_field', label: t('actions.config.sourceTypeItemField') },
    { value: 'literal', label: t('actions.config.sourceTypeLiteral') },
  ];

  function handleMappingChange(index, field, value) {
    const updated = [...mappings];
    updated[index] = { ...updated[index], [field]: value };
    onchange(updated);
  }

  function addMapping() {
    onchange([
      ...mappings,
      { source_type: 'variable', source_value: '', target_field_id: '' },
    ]);
  }

  function removeMapping(index) {
    const updated = [...mappings];
    updated.splice(index, 1);
    onchange(updated);
  }
</script>

<div class="pt-2 border-t" style="border-color: var(--ds-border);">
  <div class="flex items-center justify-between mb-2">
    <span class="block text-xs font-medium">{t('actions.config.fieldMappingsLabel')}</span>
    <button
      onclick={() => showPlaceholderModal = true}
      class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
      title={t('actions.placeholders.showReference')}
    >
      <HelpCircle class="w-3.5 h-3.5" />
    </button>
  </div>

  <div class="space-y-3">
    {#each mappings as mapping, index}
      <div class="mapping-row p-2 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-sunken);">
        <div class="flex items-start gap-2">
          <div class="flex-1 space-y-2">
            <Select
              options={sourceTypes}
              value={mapping.source_type}
              onchange={(v) => handleMappingChange(index, 'source_type', v)}
              size="small"
            />
            {#if mapping.source_type === 'item_field'}
              <div data-testid={`mapping-source-field-${index}`}>
                <FieldSelector
                  placeholder={t('actions.config.fromField')}
                  selectedField={getFieldSelectorValue({ field_name: mapping.source_value })}
                  onSelect={(field) => handleMappingChange(index, 'source_value', backendFieldName(field))}
                  onClear={() => handleMappingChange(index, 'source_value', '')}
                />
              </div>
            {:else}
              <Input
                type="text"
                class="text-xs"
                value={mapping.source_value}
                oninput={(e) => handleMappingChange(index, 'source_value', e.currentTarget.value)}
                placeholder={mapping.source_type === 'variable' ? '{{item.assignee_id}}' : t('actions.config.fromField')}
              />
            {/if}
            <Select
              options={[{ value: '', label: t('actions.config.selectTargetField') }, ...targetFields.map(f => ({ value: f.field_name, label: f.name ?? f.display_name ?? f.label ?? f.field_name }))]}
              value={mapping.target_field_id}
              onchange={(v) => handleMappingChange(index, 'target_field_id', v)}
              size="small"
            />
          </div>
          <button
            onclick={() => removeMapping(index)}
            class="p-1 hover-danger rounded transition-colors flex-shrink-0" style="color: var(--ds-icon-danger);"
            title="Remove mapping"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>
    {/each}

    <button
      onclick={addMapping}
      class="w-full px-3 py-2 text-sm border border-dashed rounded-md flex items-center justify-center gap-2 add-mapping-btn"
    >
      <Plus size={14} />
      {t('actions.config.addMapping')}
    </button>
  </div>
</div>

<style>
  .add-mapping-btn {
    color: var(--ds-text-subtle);
    border-color: var(--ds-border);
    background-color: transparent;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .add-mapping-btn:hover {
    background-color: var(--ds-background-neutral-hovered);
    border-color: var(--ds-interactive);
    color: var(--ds-interactive);
  }
</style>
