<script>
  import { onDestroy } from 'svelte';
  import { Loader } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import MobileSheet from './MobileSheet.svelte';

  /**
   * @type {{
   *   isOpen?: boolean,
   *   title?: string,
   *   searchPlaceholder?: string,
   *   searchFn?: (query: string) => Promise<Array<any>>,
   *   getKey?: (option: any) => string | number,
   *   getLabel?: (option: any) => string,
   *   onPick?: (option: any) => Promise<void> | void,
   *   dataTestid?: string,
   * }}
   */
  let {
    isOpen = $bindable(false),
    title = '',
    searchPlaceholder = '',
    searchFn = async () => [],
    getKey = (option) => option?.id,
    getLabel = (option) => option?.title ?? String(option?.id ?? ''),
    onPick = async () => {},
    dataTestid = undefined,
  } = $props();

  let query = $state('');
  let results = $state([]);
  let searching = $state(false);
  let picking = $state(false);
  let searchTimer;
  let searchVersion = 0;

  $effect(() => {
    if (isOpen) {
      query = '';
      results = [];
      searching = false;
      picking = false;
    } else {
      clearTimeout(searchTimer);
      searchVersion += 1;
    }
  });

  onDestroy(() => {
    clearTimeout(searchTimer);
    searchVersion += 1;
  });

  function handleInput(event) {
    const q = event.currentTarget.value;
    query = q;
    clearTimeout(searchTimer);
    const version = ++searchVersion;
    if (q.trim().length < 2) {
      results = [];
      searching = false;
      return;
    }
    searching = true;
    searchTimer = setTimeout(() => runSearch(q.trim(), version), 250);
  }

  async function runSearch(q, version) {
    try {
      const rows = await searchFn(q);
      if (version !== searchVersion) return;
      results = Array.isArray(rows) ? rows : [];
    } catch {
      if (version !== searchVersion) return;
      results = [];
    } finally {
      if (version === searchVersion) searching = false;
    }
  }

  async function choose(option) {
    if (picking) return;
    picking = true;
    try {
      await onPick(option);
      isOpen = false;
    } catch {
      // Parent surfaces errors; keep the sheet open for retry.
    } finally {
      picking = false;
    }
  }
</script>

<MobileSheet bind:isOpen {title} {dataTestid}>
  <div class="add-sheet">
    <input
      class="search"
      type="search"
      value={query}
      oninput={handleInput}
      placeholder={searchPlaceholder}
      autocomplete="off"
      data-testid={dataTestid ? `${dataTestid}-search` : undefined}
    />

    {#if searching || picking}
      <div class="center"><Loader class="spin" size={20} /></div>
    {:else if query.trim().length >= 2 && results.length === 0}
      <p class="empty">{t('pickers.noResultsFor', { query })}</p>
    {:else if results.length > 0}
      <ul class="results">
        {#each results as option (getKey(option))}
          <li>
            <button
              class="result-row"
              type="button"
              onclick={() => choose(option)}
              disabled={picking}
              data-testid={dataTestid ? `${dataTestid}-option-${getKey(option)}` : undefined}
            >
              {getLabel(option)}
            </button>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="hint">{searchPlaceholder}</p>
    {/if}
  </div>
</MobileSheet>

<style>
  .add-sheet {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 0 0.25rem 0.5rem;
  }

  .search {
    width: 100%;
    box-sizing: border-box;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    padding: 0.625rem 0.75rem;
    font-size: 1rem;
    background: var(--ds-surface);
    color: var(--ds-text);
  }

  .center {
    display: flex;
    justify-content: center;
    padding: 1rem 0;
    color: var(--ds-text-subtle);
  }

  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .empty,
  .hint {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ds-text-subtle);
    text-align: center;
    padding: 0.5rem 0;
  }

  .results {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    max-height: 50vh;
    overflow-y: auto;
  }

  .result-row {
    width: 100%;
    text-align: left;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background: var(--ds-surface-raised);
    color: var(--ds-text);
    padding: 0.75rem;
    font-size: 0.9375rem;
    cursor: pointer;
  }

  .result-row:active {
    background: var(--ds-background-neutral-hovered);
  }

  .result-row:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
