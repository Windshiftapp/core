<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import DynamicFilterPopover from '../shared/DynamicFilterPopover.svelte';
  import AssetDynamicFieldFilter from './AssetDynamicFieldFilter.svelte';

  let {
    statuses = [],
    assetTypes = [],
    categories = [],
    customFields = [],
    onApply = () => {},
  } = $props();

  let filters = $state([]);

  const assetFieldGroups = $derived([
    {
      category: t('pickers.fieldCategories.basic') || 'Basic',
      fields: [
        { id: 'title', name: t('pickers.fields.title.name'), type: 'text', description: '' },
        { id: 'description', name: t('pickers.fields.description.name'), type: 'text', description: '' },
        { id: 'asset_tag', name: 'Asset Tag', type: 'text', description: '' },
      ],
    },
    {
      category: 'Classification',
      fields: [
        { id: 'status', name: t('pickers.fields.status.name'), type: 'enum', description: '' },
        { id: 'type', name: t('pickers.fields.type.name'), type: 'enum', description: '' },
        { id: 'category', name: t('common.category') || 'Category', type: 'enum', description: '' },
      ],
    },
    {
      category: t('pickers.fieldCategories.dates') || 'Dates',
      fields: [
        { id: 'created_at', name: t('pickers.fields.createdAt.name'), type: 'date', description: '' },
        { id: 'updated_at', name: t('pickers.fields.updatedAt.name'), type: 'date', description: '' },
      ],
    },
    {
      category: t('pickers.fieldCategories.people') || 'People',
      fields: [
        { id: 'creator', name: t('common.createdBy') || 'Creator', type: 'user', description: '' },
      ],
    },
  ]);

  const assetCustomFieldItems = $derived(
    customFields.map((field) => ({
      id: `cf_${field.field_name || field.name}`,
      name: field.field_name || field.name,
      type: field.field_type,
      description: '',
      isCustom: true,
      options: field.options
        ? (typeof field.options === 'string' ? JSON.parse(field.options) : field.options)
        : null,
    })),
  );

  function applyFilters(rows) {
    onApply(QLBuilder.buildQuery({ dynamicFields: rows }) || '');
  }
</script>

<DynamicFilterPopover
  bind:filters
  panelMinWidth="440px"
  onapply={applyFilters}
  onclear={() => onApply('')}
>
  {#snippet filterRow({ filter, onchange, onremove, onexecute })}
    <AssetDynamicFieldFilter
      {filter}
      compact
      {statuses}
      {assetTypes}
      {categories}
      fieldGroups={assetFieldGroups}
      customFieldItems={assetCustomFieldItems}
      onChange={onchange}
      onRemove={onremove}
      onExecute={onexecute}
    />
  {/snippet}
</DynamicFilterPopover>
