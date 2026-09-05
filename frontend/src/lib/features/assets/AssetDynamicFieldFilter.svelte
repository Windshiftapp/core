<script>
  import DynamicFieldFilter from '../items/DynamicFieldFilter.svelte';

  let {
    filter = { field: null, operator: '=', value: '', values: [] },
    compact = false,
    statuses = [],
    assetTypes = [],
    categories = [],
    fieldGroups = [],
    customFieldItems = [],
    onChange = () => {},
    onRemove = () => {},
    onExecute = () => {},
  } = $props();

  function flattenCategoryOptions(items, level = 0) {
    let result = [];
    for (const category of items) {
      result.push({
        value: category.name,
        label: '\u00A0'.repeat(level * 2) + category.name,
      });
      if (category.children?.length) {
        result = result.concat(flattenCategoryOptions(category.children, level + 1));
      }
    }
    return result;
  }

  function loadOptions(field) {
    if (!['enum', 'select', 'multiselect'].includes(field.type)) return [];
    if (field.id === 'status') {
      return statuses.map((status) => ({ value: status.name, label: status.name }));
    }
    if (field.id === 'type') {
      return assetTypes.map((type) => ({ value: type.name, label: type.name }));
    }
    if (field.id === 'category') return flattenCategoryOptions(categories);
    if (field.options?.items && Array.isArray(field.options.items)) {
      return field.options.items.map((option) => ({ value: option.id, label: option.label }));
    }
    if (Array.isArray(field.options)) {
      return field.options.map((option) => ({ value: option, label: option }));
    }
    return [];
  }
</script>

<DynamicFieldFilter
  {filter}
  {compact}
  {fieldGroups}
  {customFieldItems}
  optionLoader={loadOptions}
  onchange={onChange}
  onremove={onRemove}
  onexecute={onExecute}
/>
