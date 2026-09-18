<script>
  import { CheckSquare, Square, ChevronDown, X } from '@lucide/svelte';
  import { BasePicker } from '.';
  import { getVisibleColor } from '../utils/colorUtils.js';
  import { formatDateShort } from '../utils/dateFormatter.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    items = [],
    config = {},
    placeholder = '',
    ariaLabel = undefined,
    showUnassigned = false,
    unassignedLabel = '',
    disabled = false,
    allowClear = true,
    loading = false,
    autoOpen = false,
    showSelectedInTrigger = true,
    multiSelect = false,
    values = $bindable([]),
    maxSelections = null,
    class: className = '',
    children: customTrigger = null,  // Optional custom trigger snippet from callers
    footer = null,
    keepOpenOnFooterTab = false,
    onSearchChange = null,
    searchDebounce = 300,
    allowCreate = false,
    onCreate = null,
    noResultsSnippet = null,
    searchTestid = undefined,
    optionTestid = null,
    // (value) => string | null. Labels the selected value when it is not in
    // items yet (options may load lazily on open).
    resolveMissingLabel = null,
    onOpen = null,
    onSelect = null,
    onCancel = null
  } = $props();

  const finalConfig = $derived.by(() => {
    const defaults = {
      icon: null,
      primary: { text: (item) => item.name || item.label || '' },
      secondary: null,
      badges: [],
      metadata: [],
      searchFields: ['name', 'label'],
      getValue: (item) => item.id,
      getLabel: (item) => item.name || item.label || ''
    };
    return { ...defaults, ...config };
  });
</script>

<!-- Always forward a children snippet to BasePicker -->
<BasePicker
  bind:value
  bind:values
  {items}
  {loading}
  placeholder={placeholder || t('pickers.select')}
  {ariaLabel}
  {showUnassigned}
  unassignedLabel={unassignedLabel || t('common.none')}
  {disabled}
  {allowClear}
  {autoOpen}
  {showSelectedInTrigger}
  multiple={multiSelect}
  {maxSelections}
  class={className}
  searchFields={finalConfig.searchFields}
  getValue={finalConfig.getValue}
  getLabel={finalConfig.getLabel}
  serverSearch={!!onSearchChange}
  {searchDebounce}
  {allowCreate}
  {onCreate}
  {noResultsSnippet}
  {searchTestid}
  {optionTestid}
  onSearchChange={(query) => onSearchChange?.(query)}
  onOpen={() => onOpen?.()}
  onSelect={(item) => onSelect?.(item)}
  onChange={(values) => onSelect?.(values)}
  onCancel={() => onCancel?.()}
  {footer}
  {keepOpenOnFooterTab}
>
  {#snippet children()}
    {#if customTrigger}
      {@render customTrigger()}
    {:else}
      <div
        aria-disabled={disabled}
        class="relative w-full flex items-center justify-between gap-2 px-3 py-2 rounded text-sm transition-colors {className}"
        style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
        style:opacity={disabled ? 0.5 : 1}
        style:cursor={disabled ? 'not-allowed' : 'pointer'}
        data-testid="item-picker-trigger"
      >
        <div class="flex items-center gap-2 flex-1 min-w-0 {multiSelect ? 'flex-wrap' : ''}">
          {#if multiSelect}
            {#if values.length > 0}
              {#each values as val (val)}
                {@const selItem = items.find(i => finalConfig.getValue(i) === val)}
                {@const missingLabel = selItem ? '' : (resolveMissingLabel?.(val) || '')}
                {#if selItem}
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-selected); color: var(--ds-text);">
                    <span class="truncate max-w-[120px]">{finalConfig.getLabel(selItem)}</span>
                  </span>
                {:else if missingLabel}
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs" style="background-color: var(--ds-background-selected); color: var(--ds-text);">
                    <span class="truncate max-w-[120px]">{missingLabel}</span>
                  </span>
                {/if}
              {/each}
            {:else}
              <span style="color: var(--ds-text-subtle);">{placeholder || t('pickers.select')}</span>
            {/if}
          {:else}
            {@const selItem = items.find(i => finalConfig.getValue(i) === value)}
            {@const missingLabel = selItem || !showSelectedInTrigger ? '' : (resolveMissingLabel?.(value) || '')}
            {#if selItem && showSelectedInTrigger}
              {#if finalConfig.icon?.type === 'color-dot'}
                {@const color = finalConfig.icon.source(selItem)}
                <div class="{finalConfig.icon.size || 'w-2 h-2'} rounded-full flex-shrink-0" style="background-color: {getVisibleColor(color)};"></div>
              {:else if finalConfig.icon?.type === 'component'}
                {@const IconComp = finalConfig.icon.source(selItem)}
                {#if IconComp}
                  <IconComp size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                {/if}
              {/if}
              <span class="truncate">{finalConfig.getLabel(selItem)}</span>
            {:else if missingLabel}
              <span class="truncate">{missingLabel}</span>
            {:else}
              <span style="color: var(--ds-text-subtle);">{placeholder || t('pickers.select')}</span>
            {/if}
          {/if}
        </div>
        <div class="flex items-center gap-1 flex-shrink-0">
          {#if multiSelect && allowClear && values.length > 0 && !disabled}
            <button type="button" onclick={() => { values = []; onSelect?.([]); }} class="p-0.5 rounded hover:bg-opacity-10" style="color: var(--ds-text-subtle);" aria-label={t('pickers.clearSelection')}>
              <X size={14} />
            </button>
          {:else if !multiSelect && allowClear && value != null && !disabled && showSelectedInTrigger}
            <button type="button" onclick={(e) => { e.stopPropagation(); onSelect?.(null); }} class="p-0.5 rounded hover:bg-opacity-10" style="color: var(--ds-text-subtle);" aria-label={t('pickers.clearSelection')}>
              <X size={14} />
            </button>
          {/if}
          <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
        </div>
      </div>
    {/if}
  {/snippet}

  {#snippet itemSnippet({ item: listItem, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      {#if multiSelect}
        <div class="flex-shrink-0">
          {#if isSelected}
            <CheckSquare size={16} style="color: var(--ds-interactive);" />
          {:else}
            <Square size={16} style="color: var(--ds-text-subtle);" />
          {/if}
        </div>
      {/if}

      {#if finalConfig.icon?.type === 'color-dot'}
        {@const color = finalConfig.icon.source(listItem)}
        <div class="{finalConfig.icon.size || 'w-2 h-2'} rounded-full flex-shrink-0" style="background-color: {getVisibleColor(color)};"></div>
      {:else if finalConfig.icon?.type === 'component'}
        {@const IconComp = finalConfig.icon.source(listItem)}
        {#if IconComp}
          <IconComp size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        {/if}
      {/if}

      <span class="font-medium" style="color: var(--ds-text);">{finalConfig.primary.text(listItem)}</span>

      {#each finalConfig.badges as badge}
        {@const badgeText = badge.text(listItem)}
        {#if badgeText}
          <span class="px-1.5 py-0.5 rounded text-xs" style="background-color: {badge.bgColor ? badge.bgColor(listItem) : 'var(--ds-background-neutral)'}; color: {badge.textColor ? badge.textColor(listItem) : 'var(--ds-text-subtle)'};">{badgeText}</span>
        {/if}
      {/each}
    </div>

    {#if finalConfig.secondary}
      {@const secondaryText = finalConfig.secondary.text(listItem)}
      {#if secondaryText}
        <div class="text-xs" style="color: var(--ds-text-subtle);">{secondaryText}</div>
      {/if}
    {/if}

    {#each finalConfig.metadata as meta}
      {#if meta.type === 'date-range'}
        {@const startDate = meta.startDate(listItem)}
        {@const endDate = meta.endDate(listItem)}
        {#if startDate || endDate}
          <div class="flex items-center gap-2 text-xs" style="color: var(--ds-text-subtle);">
            {#if meta.icon}{@const MetaIcon = meta.icon}<MetaIcon size={12} />{/if}
            <span>{formatDateShort(startDate)} → {formatDateShort(endDate)}</span>
          </div>
        {/if}
      {:else if meta.type === 'badge'}
        {@const badgeText = meta.text(listItem)}
        {#if badgeText}
          <div class="flex items-center gap-2">
            <span class="inline-block px-2 py-0.5 rounded text-xs font-medium" style="background-color: {meta.bgColor ? meta.bgColor(listItem) : 'var(--ds-background-neutral)'}; color: {meta.textColor ? meta.textColor(listItem) : 'var(--ds-text)'};">{badgeText}</span>
          </div>
        {/if}
      {:else if meta.type === 'text'}
        {@const text = meta.text(listItem)}
        {#if text}
          <div class="flex items-center gap-2 text-xs" style="color: var(--ds-text-subtle);">{#if meta.icon}{@const MetaIcon = meta.icon}<MetaIcon size={12} />{/if}<span>{text}</span></div>
        {/if}
      {/if}
    {/each}
  {/snippet}
</BasePicker>
