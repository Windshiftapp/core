<script>
  import BasePicker from '../../pickers/BasePicker.svelte';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { t } from '../../stores/i18n.svelte.js';

  /**
   * BasePicker variant for the hierarchy level. One option per item-type
   * hierarchy level; the option renders every item type in that level (a level
   * can hold several, e.g. Task + Bug) plus the level's filtered count.
   *
   * @type {{
   *   levels: Array<{ level: number, types: Array<object>, total: number }>,
   *   value?: number|null,
   *   loading?: boolean,
   *   disabled?: boolean,
   *   onchange?: (level: number) => void,
   * }}
   */
  let { levels = [], value = $bindable(null), loading = false, disabled = false, onchange } = $props();

  const selected = $derived(levels.find((entry) => entry.level === value) ?? null);

  function labelFor(entry) {
    if (entry?.label) return entry.label;
    return (entry?.types ?? []).map((type) => type.name).join(', ');
  }
</script>

{#snippet levelRow({ item })}
  <div class="flex items-center gap-2 min-w-0 flex-1">
    <div class="flex items-center gap-1 flex-shrink-0">
      {#each item.types as type (type.id)}
        <ItemTypeIcon itemType={type} title={type.name} />
      {/each}
    </div>
    <div class="flex flex-col min-w-0">
      <span class="font-medium truncate">{labelFor(item)}</span>
      <span class="text-xs" style="color: var(--ds-text-subtle);">
        {t('collectionHierarchy.levelN', { level: item.level })}
      </span>
    </div>
  </div>
  <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--ds-text-subtle);">
    {t('collectionHierarchy.itemsCount', { count: item.total })}
  </span>
{/snippet}

<BasePicker
  bind:value
  items={levels}
  {loading}
  {disabled}
  searchFields={['name']}
  getValue={(entry) => entry.level}
  getLabel={(entry) => labelFor(entry)}
  itemSnippet={levelRow}
  optionTestid={(opt) => `hierarchy-level-option-${opt.item?.level}`}
  menuTestid="hierarchy-level-menu"
  searchTestid="hierarchy-level-search"
  scrollTestid="hierarchy-level-scroll"
  placeholder={t('collectionHierarchy.selectLevel')}
  ariaLabel={t('collectionHierarchy.selectLevel')}
  onSelect={(entry) => onchange?.(entry?.level)}
>
  <button
    type="button"
    data-testid="hierarchy-level-trigger"
    class="flex items-center gap-2 px-3 py-2 text-sm border rounded-md transition-colors"
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
    {disabled}
  >
    <span class="flex items-center gap-1 flex-shrink-0">
      {#each selected?.types ?? [] as type (type.id)}
        <ItemTypeIcon itemType={type} title={type.name} />
      {/each}
    </span>
    <span class="truncate max-w-[16rem]">
      {selected ? labelFor(selected) : t('collectionHierarchy.selectLevel')}
    </span>
    {#if selected}
      <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--ds-text-subtle);">
        {selected.total}
      </span>
    {/if}
    <ChevronDown class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
  </button>
</BasePicker>
