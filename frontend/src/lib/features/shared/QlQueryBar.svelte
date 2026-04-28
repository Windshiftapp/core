<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { getShortcut, matchesShortcut, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
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

  function handleQueryChange(event) {
    onquerychange?.(event.target.value);
  }

  function handleKeydown(event) {
    if (matchesShortcut(event, qlExecuteShortcut)) {
      event.preventDefault();
      onexecute?.();
    }
  }
</script>

<div class="mb-4">
  <div class="flex items-center gap-3 text-xs" style="color: var(--ds-text-subtle);">
    <div class="flex items-center gap-2 min-w-0">
      <span class="font-medium shrink-0">{t('collections.query')}:</span>
      <code class="font-mono truncate" title={query || t('collections.noQuery')}>
        {query || t('collections.noFiltersApplied')}
      </code>
      {#if mode === 'builder'}
        <Button variant="ghost" size="sm" onclick={() => onenterrawmode?.()}>
          {t('collections.editCqlManually')}
        </Button>
      {:else}
        <Button variant="ghost" size="sm" onclick={() => onreset?.()}>
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
        value={query}
        oninput={handleQueryChange}
        placeholder={t('collections.queryPlaceholder')}
        class="font-mono text-sm"
        rows={2}
        onkeydown={handleKeydown}
      />
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
