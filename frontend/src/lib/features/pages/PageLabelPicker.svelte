<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { IconPlus, IconCheck, IconX } from '@tabler/icons-svelte-runes';
  import { tick } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Input from '../../components/Input.svelte';

  /** Page-label picker for page editing and sidebar filtering. The parent owns
   * selection; callbacks receive full labels. Sidebar callers disable creation. */

  let {
    workspaceId,
    selectedIds = new Set(),
    allowCreate = true,
    triggerLabel,
    triggerIconOnly = false,
    triggerTestid = 'page-label-picker-trigger',
    onToggle = () => {},
    onCreate = () => {},
    /** Optional pre-fetched labels list. When omitted, the picker fetches lazily on open. */
    labels: providedLabels = null,
  } = $props();

  let labels = $state(/** @type {any[]} */ ([]));
  let loading = $state(false);
  let loadedOnce = $state(false);
  let search = $state('');
  let creating = $state(false);
  let createColor = $state('#3B82F6');
  let searchInputEl = $state(null);

  const {
    elements: { trigger, content },
    states: { open },
  } = createPopover({
    forceVisible: true,
    positioning: { strategy: 'fixed', placement: 'bottom-start', fitViewport: true },
    portal: 'body',
  });

  // Lazy-load labels on first open so the trigger button doesn't fire a
  // workspace-wide list call on every page load.
  $effect(() => {
    if ($open && !loadedOnce) {
      void load();
    }
    if ($open) {
      tick().then(() => searchInputEl?.focus({ preventScroll: true }));
    } else {
      search = '';
      creating = false;
      createColor = '#3B82F6';
    }
  });

  // If the parent passes in a fresh providedLabels list (e.g., after sidebar
  // filter reloads the tree), sync it. We deliberately do NOT replace the
  // local labels when the popover is open — that would yank the create form.
  $effect(() => {
    if (providedLabels !== null && !$open) {
      labels = providedLabels;
      loadedOnce = true;
    }
  });

  async function load() {
    loading = true;
    try {
      const resp = await api.pageLabels.list(workspaceId);
      labels = resp || [];
      loadedOnce = true;
    } catch (err) {
      errorToast(err?.message || t('pages.labelsErrorLoad'));
    } finally {
      loading = false;
    }
  }

  const filtered = $derived.by(() => {
    const q = search.trim().toLowerCase();
    if (!q) return labels;
    return labels.filter((l) => l.name?.toLowerCase().includes(q));
  });

  const canCreate = $derived(
    allowCreate &&
      search.trim().length > 0 &&
      !labels.some((l) => l.name.toLowerCase() === search.trim().toLowerCase())
  );

  async function handleToggle(label) {
    onToggle(label);
  }

  async function handleCreate() {
    const name = search.trim();
    if (!name) return;
    creating = true;
    try {
      const created = await api.pageLabels.create(workspaceId, { name, color: createColor });
      labels = [...labels, created].sort((a, b) => a.name.localeCompare(b.name));
      search = '';
      createColor = '#3B82F6';
      onCreate(created);
      // Auto-select the just-created label so the user doesn't have to
      // create-then-click. Both callers (page header picker, sidebar
      // filter) want the new label active; the sidebar filter never
      // reaches this branch because it passes allowCreate=false.
      onToggle(created);
    } catch (err) {
      errorToast(err?.message || t('pages.labelsErrorSave'));
    } finally {
      creating = false;
    }
  }

  function onSearchKeydown(e) {
    if (e.key === 'Enter' && canCreate) {
      e.preventDefault();
      void handleCreate();
    }
  }

  // Six-step color palette — same shade scale used elsewhere in the app.
  const palette = [
    '#3B82F6', // blue
    '#10B981', // green
    '#8B5CF6', // purple
    '#F59E0B', // amber
    '#EF4444', // red
    '#6B7280', // gray
  ];

  // True when createColor isn't one of the preset palette swatches — the
  // 7th "custom" swatch then renders the actual color (rather than its
  // rainbow gradient) so the user can see what they picked.
  let customColorActive = $derived(!palette.includes(createColor));
</script>

<button
  use:melt={$trigger}
  type="button"
  class="trigger"
  class:trigger--icon-only={triggerIconOnly}
  data-testid={triggerTestid}
  aria-label={triggerLabel || t('pages.labelsAdd')}
>
  <IconPlus size={14} />
  {#if !triggerIconOnly}<span>{triggerLabel || t('pages.labelsAdd')}</span>{/if}
</button>

{#if $open}
  <div use:melt={$content} class="popover" data-testid="page-label-picker">
    <div class="search-row">
      <Input
        bind:inputRef={searchInputEl}
        bind:value={search}
        onkeydown={onSearchKeydown}
        type="text"
        class="search-input"
        placeholder={allowCreate
          ? t('pages.labelsSearchPlaceholder')
          : t('pages.labelsFilterPlaceholder')}
        dataTestid="page-label-picker-search"
        size="small"
      />
    </div>

    {#if loading}
      <p class="status">{t('common.loading')}</p>
    {:else if filtered.length === 0 && !canCreate}
      <p class="status">{t('pages.labelsEmpty')}</p>
    {:else}
      <ul class="list" data-testid="page-label-picker-list" role="listbox" aria-multiselectable="true">
        {#each filtered as label (label.id)}
          {@const checked = selectedIds.has(label.id)}
          <li>
            <button
              type="button"
              class="row"
              class:row--checked={checked}
              role="option"
              aria-selected={checked}
              onclick={() => handleToggle(label)}
              data-testid="page-label-picker-row"
              data-label-id={label.id}
            >
              <span
                class="swatch"
                style="background-color: {label.color || '#3B82F6'};"
                aria-hidden="true"
              ></span>
              <span class="name">{label.name}</span>
              {#if checked}
                <IconCheck size={14} class="check" />
              {/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    {#if canCreate}
      <div class="create-row">
        <div class="palette" role="group" aria-label="Color">
          {#each palette as hex}
            <button
              type="button"
              class="swatch-btn"
              class:swatch-btn--active={createColor === hex}
              style="background-color: {hex};"
              onclick={() => (createColor = hex)}
              aria-label={hex}
            ></button>
          {/each}
          <!-- 7th swatch = custom. Renders the current createColor when it
               isn't in the preset palette, otherwise a rainbow gradient so
               the user knows what the button does. Clicking it opens the
               native OS color picker via the hidden <input type="color">. -->
          <label
            class="swatch-btn swatch-btn--custom"
            class:swatch-btn--active={customColorActive}
            style={customColorActive ? `background: ${createColor};` : ''}
            aria-label="Custom color"
            data-testid="page-label-picker-custom-swatch"
          >
            <input
              type="color"
              class="color-input"
              bind:value={createColor}
              aria-label="Pick a custom color"
            />
          </label>
        </div>
        <button
          type="button"
          class="create-btn"
          onclick={handleCreate}
          disabled={creating}
          data-testid="page-label-picker-create"
        >
          {t('pages.labelsCreateNamed', { name: search.trim() })}
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .trigger {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0.5rem;
    border: 1px dashed var(--ds-border);
    background: transparent;
    color: var(--ds-text-subtle);
    border-radius: 999px;
    font-size: 0.75rem;
    cursor: pointer;
    transition: background 120ms, color 120ms;
  }

  .trigger:hover {
    background: var(--ds-surface-hovered);
    color: var(--ds-text);
  }

  .trigger--icon-only {
    padding: 0.25rem;
  }

  .popover {
    z-index: 1000;
    min-width: 260px;
    max-width: 320px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    border-radius: 0.5rem;
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .search-row {
    flex-shrink: 0;
    padding: 0.5rem;
    border-bottom: 1px solid var(--ds-border);
  }

  .search-input {
    width: 100%;
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.25rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 0.875rem;
  }

  .search-input:focus {
    outline: none;
    border-color: var(--ds-accent-blue);
  }

  .status {
    padding: 0.75rem;
    margin: 0;
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    text-align: center;
  }

  .list {
    min-height: 0;
    overscroll-behavior: contain;
    list-style: none;
    padding: 0.25rem;
    margin: 0;
    max-height: 240px;
    overflow-y: auto;
  }

  .row {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.375rem 0.5rem;
    background: transparent;
    border: none;
    border-radius: 0.25rem;
    color: var(--ds-text);
    font-size: 0.875rem;
    text-align: left;
    cursor: pointer;
  }

  .row:hover {
    background: var(--ds-surface-hovered);
  }

  .row--checked {
    background: var(--ds-surface-selected);
  }

  .swatch {
    width: 12px;
    height: 12px;
    border-radius: 999px;
    flex-shrink: 0;
  }

  .name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  :global(.check) {
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }

  .create-row {
    border-top: 1px solid var(--ds-border);
    padding: 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .palette {
    display: flex;
    gap: 0.25rem;
  }

  .swatch-btn {
    width: 20px;
    height: 20px;
    border-radius: 999px;
    border: 2px solid transparent;
    cursor: pointer;
    padding: 0;
  }

  /* Custom-color swatch: conic-gradient rainbow when no custom color has
     been picked yet, otherwise the parent's inline `background` wins.
     The native <input type="color"> sits inside but is invisible — the
     <label> wrapping it forwards the click to the OS picker. */
  .swatch-btn--custom {
    background: conic-gradient(
      from 0deg,
      #ef4444,
      #f59e0b,
      #10b981,
      #3b82f6,
      #8b5cf6,
      #ef4444
    );
    display: inline-flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    position: relative;
  }

  .color-input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    cursor: pointer;
    padding: 0;
    border: none;
    background: transparent;
  }

  .swatch-btn--active {
    border-color: var(--ds-text);
  }

  .create-btn {
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.25rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 0.75rem;
    cursor: pointer;
  }

  .create-btn:hover:not(:disabled) {
    background: var(--ds-surface-hovered);
  }

  .create-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>
