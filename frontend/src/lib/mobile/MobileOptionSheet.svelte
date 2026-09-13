<script>
  import { Search, Check, Loader } from '@lucide/svelte';
  import MobileSheet from './MobileSheet.svelte';

  /**
   * Generic "pick one of N" bottom sheet — the mobile replacement for desktop
   * BasePicker/UserPicker dropdowns on /m. Big tappable rows, a check on the
   * current selection, optional search (auto-shown for long lists), and an
   * optional clear row. Selecting a row closes the sheet.
   *
   * @type {{
   *   isOpen?: boolean,
   *   title?: string,
   *   options?: Array<any>,
   *   getValue?: (option: any) => any,
   *   getLabel?: (option: any) => string,
   *   selectedValue?: any,
   *   allowClear?: boolean,
   *   clearLabel?: string,
   *   searchable?: boolean | null,  // null = auto (show search above 8 rows)
   *   loading?: boolean,
   *   emptyText?: string,
   *   dataTestid?: string,
   *   row?: import('svelte').Snippet<[any]> | null,
   *   onSelect?: (option: any) => void,
   *   onClear?: () => void,
   *   onclose?: (() => void) | null,
   * }}
   */
  let {
    isOpen = $bindable(false),
    title = '',
    options = [],
    getValue = (option) => option?.id,
    getLabel = (option) => option?.name ?? String(option),
    selectedValue = null,
    allowClear = false,
    clearLabel = 'None',
    searchable = null,
    loading = false,
    emptyText = 'No options',
    dataTestid = undefined,
    row = null,
    onSelect = () => {},
    onClear = () => {},
    onclose = null,
  } = $props();

  let query = $state('');
  // Reset the search each time the sheet opens.
  $effect(() => {
    if (isOpen) query = '';
  });

  const showSearch = $derived(
    searchable === true || (searchable === null && options.length > 8),
  );
  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((option) =>
      getLabel(option).toLowerCase().includes(q),
    );
  });

  function choose(option) {
    isOpen = false;
    onSelect(option);
  }

  function clear() {
    isOpen = false;
    onClear();
  }
</script>

<MobileSheet bind:isOpen {title} {onclose} {dataTestid}>
  {#if showSearch}
    <div class="search-wrap">
      <Search size={16} aria-hidden="true" />
      <input
        type="text"
        bind:value={query}
        placeholder="Search…"
        autocomplete="off"
        data-testid="mobile-sheet-search"
      />
    </div>
  {/if}

  {#if loading}
    <div class="state" data-testid="mobile-sheet-loading">
      <Loader size={18} class="spin" />
    </div>
  {:else if filtered.length === 0}
    <p class="state" data-testid="mobile-sheet-empty">{query ? 'No matches' : emptyText}</p>
  {:else}
    <ul class="options" role="listbox" aria-label={title}>
      {#if allowClear}
        <li>
          <button class="option clear" onclick={clear} type="button" data-testid="mobile-sheet-clear">
            <span>{clearLabel}</span>
            {#if selectedValue == null}<span class="check-wrap"><Check size={18} aria-hidden="true" /></span>{/if}
          </button>
        </li>
      {/if}
      {#each filtered as option (getValue(option))}
        <li>
          <button
            class="option"
            class:selected={getValue(option) === selectedValue}
            onclick={() => choose(option)}
            type="button"
            role="option"
            aria-selected={getValue(option) === selectedValue}
            data-testid={`mobile-sheet-option-${getValue(option)}`}
          >
            {#if row}
              {@render row(option)}
            {:else}
              <span class="label">{getLabel(option)}</span>
            {/if}
            {#if getValue(option) === selectedValue}
              <span class="check-wrap"><Check size={18} aria-hidden="true" /></span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</MobileSheet>

<style>
  .search-wrap {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0 1rem 0.25rem;
    padding: 0 0.75rem;
    min-height: 44px;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-background-input, var(--ds-surface));
    color: var(--ds-text-subtle);
  }
  .search-wrap input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--ds-text);
    font-size: max(1rem, 16px);
  }

  .options {
    list-style: none;
    margin: 0;
    padding: 0.25rem 0.5rem 0.75rem;
  }
  .option {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    min-height: 52px;
    padding: 0.5rem 0.75rem;
    border: none;
    border-radius: var(--radius-lg, 8px);
    background: transparent;
    color: var(--ds-text);
    font-size: 1rem;
    text-align: left;
    cursor: pointer;
  }
  .option:active {
    background-color: var(--ds-background-neutral-hovered);
  }
  .option.selected {
    color: var(--ds-interactive);
    font-weight: var(--font-medium, 500);
  }
  .option .label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .option.clear {
    color: var(--ds-text-subtle);
  }
  .check-wrap {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--ds-interactive);
  }

  .state {
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0;
    padding: 1.5rem 1rem;
    color: var(--ds-text-subtle);
    font-size: 0.9375rem;
  }
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
