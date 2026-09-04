<script>
  import { tick } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { getShortcut, matchesShortcut, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import {
    applyQlSuggestion,
    buildQlSuggestions,
    completionValues,
    getQlCompletionContext,
  } from '../../utils/qlCompletion.js';
  import DescriptionText from '../../components/DescriptionText.svelte';

  const qlExecuteShortcut = getShortcut('ql', 'execute');

  let {
    query = '',
    mode = 'builder', // 'builder' | 'raw'
    error = null,
    onenterrawmode = null,
    onreset = null,
    onexecute = null,
    onquerychange = null,
  } = $props();

  let textareaRef = $state(null);
  let focused = $state(false);
  let dismissed = $state(false);
  let cursor = $state(0);
  let catalog = $state(null);
  let catalogLoading = false;
  let activeIndex = $state(0);
  let availableValues = $state([]);
  let valueLoadToken = 0;
  const valueCache = new Map();

  let completionContext = $derived(
    catalog ? getQlCompletionContext(query, cursor, catalog) : null,
  );
  let suggestions = $derived(
    focused && !dismissed
      ? buildQlSuggestions(completionContext, catalog, availableValues)
      : [],
  );

  $effect(() => {
    if (mode === 'raw' && !catalog && !catalogLoading) void loadCatalog();
  });

  $effect(() => {
    void loadValues(completionContext);
  });

  async function loadCatalog() {
    catalogLoading = true;
    try {
      catalog = await api.queryLanguage.getCatalog();
    } catch (err) {
      console.warn('Failed to load QL completion catalog:', err);
    } finally {
      catalogLoading = false;
    }
  }

  async function loadValues(context) {
    const token = ++valueLoadToken;
    if (context?.kind !== 'value') {
      availableValues = [];
      return;
    }

    if (context.field?.values?.length) {
      availableValues = context.field.values;
      return;
    }

    const valueHelp = context.field?.value_help;
    if (!valueHelp) {
      availableValues = [];
      return;
    }

    const cacheKey = JSON.stringify(valueHelp);
    if (valueCache.has(cacheKey)) {
      availableValues = valueCache.get(cacheKey);
      return;
    }

    availableValues = [];
    try {
      const rows = await api.queryLanguage.getValues(valueHelp);
      if (token !== valueLoadToken) return;
      const values = completionValues(rows, valueHelp);
      valueCache.set(cacheKey, values);
      availableValues = values;
      activeIndex = 0;
    } catch (err) {
      if (token !== valueLoadToken) return;
      console.warn('Failed to load QL value help:', err);
      availableValues = [];
    }
  }

  function handleQueryChange(event) {
    cursor = event.target.selectionStart ?? event.target.value.length;
    dismissed = false;
    activeIndex = 0;
    onquerychange?.(event.target.value);
  }

  function handleKeydown(event) {
    if (matchesShortcut(event, qlExecuteShortcut)) {
      event.preventDefault();
      onexecute?.();
      return;
    }

    if (suggestions.length === 0) return;
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      activeIndex = (activeIndex + 1) % suggestions.length;
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      activeIndex = (activeIndex - 1 + suggestions.length) % suggestions.length;
    } else if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault();
      void selectSuggestion(suggestions[activeIndex]);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      dismissed = true;
    }
  }

  function updateCursor(event) {
    cursor = event.target.selectionStart ?? query.length;
    activeIndex = 0;
  }

  function handleKeyup(event) {
    if (['ArrowDown', 'ArrowUp', 'Enter', 'Tab', 'Escape'].includes(event.key)) return;
    updateCursor(event);
  }

  function handleFocus(event) {
    focused = true;
    dismissed = false;
    updateCursor(event);
  }

  async function selectSuggestion(suggestion) {
    if (!suggestion || !completionContext) return;
    const result = applyQlSuggestion(query, completionContext, suggestion);
    onquerychange?.(result.query);
    cursor = result.cursor;
    dismissed = false;
    activeIndex = 0;
    await tick();
    textareaRef?.focus();
    textareaRef?.setSelectionRange(result.cursor, result.cursor);
  }

  function suggestionTestId(suggestion, index) {
    const value = String(suggestion.value).toLowerCase().replace(/[^a-z0-9]+/g, '-');
    return `ql-suggestion-${suggestion.kind}-${value || index}`;
  }
</script>

<div class="mb-4">
  <div class="flex items-center gap-3 text-xs" style="color: var(--ds-text-subtle);">
    <div class="flex items-center gap-2 min-w-0">
      <span class="font-medium shrink-0">{t('collections.query')}:</span>
      <code
        class="font-mono truncate"
        title={query || t('collections.noQuery')}
        data-testid="ql-query-summary"
      >
        {query || t('collections.noFiltersApplied')}
      </code>
      {#if mode === 'builder'}
        <Button dataTestid="ql-enter-raw-mode" variant="ghost" size="sm" onclick={() => onenterrawmode?.()}>
          {t('collections.editCqlManually')}
        </Button>
      {:else}
        <Button dataTestid="ql-reset-to-builder" variant="ghost" size="sm" onclick={() => onreset?.()}>
          {t('collections.resetToBuilder')}
        </Button>
      {/if}
    </div>
    {#if error && mode === 'builder'}
      <span style="color: var(--ds-text-danger);">{t('collections.error')}</span>
    {/if}
  </div>

  {#if mode === 'raw'}
    <div class="mt-3 p-3 rounded-lg border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <label for="ql-editor" class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">
        {t('collections.queryLanguage')}
      </label>
      <Textarea
        id="ql-editor"
        data-testid="ql-editor"
        bind:textareaRef
        value={query}
        oninput={handleQueryChange}
        placeholder={t('collections.queryPlaceholder')}
        class="font-mono text-sm"
        rows={2}
        onkeydown={handleKeydown}
        onkeyup={handleKeyup}
        onselect={updateCursor}
        onclick={updateCursor}
        onfocus={handleFocus}
        onblur={() => (focused = false)}
        aria-autocomplete="list"
        aria-controls="ql-suggestions"
        aria-expanded={suggestions.length > 0}
        aria-activedescendant={suggestions[activeIndex] ? `ql-suggestion-${activeIndex}` : undefined}
      />
      {#if suggestions.length > 0}
        <div
          id="ql-suggestions"
          data-testid="ql-suggestions"
          role="listbox"
          class="mt-1 max-h-56 overflow-y-auto rounded-md border p-1 shadow-sm"
          style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
        >
          {#each suggestions as suggestion, index}
            <button
              id={`ql-suggestion-${index}`}
              data-testid={suggestionTestId(suggestion, index)}
              type="button"
              role="option"
              aria-selected={index === activeIndex}
              class="flex w-full items-center justify-between gap-3 rounded px-2 py-1.5 text-left font-mono text-sm"
              style={index === activeIndex
                ? 'background-color: var(--ds-surface-hover); color: var(--ds-text);'
                : 'color: var(--ds-text);'}
              onmouseenter={() => (activeIndex = index)}
              onmousedown={(event) => event.preventDefault()}
              onclick={() => selectSuggestion(suggestion)}
            >
              <span>{suggestion.label || suggestion.value}</span>
              {#if suggestion.label && suggestion.label !== String(suggestion.value)}
                <span class="truncate text-xs" style="color: var(--ds-text-subtle);">
                  {suggestion.value}
                </span>
              {/if}
            </button>
          {/each}
        </div>
      {/if}
      {#if error}
        <DescriptionText as="div" variant="danger" class="font-mono">
          {error}
        </DescriptionText>
      {/if}
      <div class="mt-2 flex items-center justify-between">
        <span class="text-xs" style="color: var(--ds-text-subtlest);">
          {t('collections.executeShortcut', { shortcut: getShortcutDisplay('ql', 'execute') })}
        </span>
        <div class="flex gap-2">
          <Button variant="primary" size="sm" onclick={() => onexecute?.()}>{t('collections.execute')}</Button>
        </div>
      </div>
    </div>
  {/if}
</div>
