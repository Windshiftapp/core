<script>
  import { onDestroy } from 'svelte';
  import { ensureExtensionsLoaded, extensions, getEditorProviders } from '../stores/extensions.svelte.js';
  import { createPluginBridge } from '../services/PluginBridge.js';
  import { confirm } from '../composables/useConfirm.js';
  import { t } from '../stores/i18n.svelte.js';
  import { themeStore } from '../stores/theme.svelte.js';
  import Spinner from '../components/Spinner.svelte';
  import LazyMilkdownEditor from './LazyMilkdownEditor.svelte';

  // Keep in step with validation.MarkdownMaxBytes in core.
  const MARKDOWN_MAX_BYTES = 256 * 1024;

  let {
    content = $bindable(''),
    readonly = false,
    entityType = null,
    entityId = null,
    workspaceId = null,
    onContentChange = null,
    testId = null,
    // Opt-in: only surfaces that pass this mount the plugin toggle.
    editorProviders = false,
    ...rest
  } = $props();

  let activeProvider = $state(null);
  let sessionId = $state(null);
  let iframe = $state(null);
  let bridge = $state(null);
  let nativeEditor = $state(null);
  let pluginReady = $state(false);
  let pluginError = $state(null);
  let dirty = $state(false);

  // Imperative surface mirrors MilkdownEditor. Commands the plugin protocol
  // cannot express yet (insertImage/insertMarkdown) no-op with a warning in
  // plugin mode instead of silently doing nothing.
  export function focus() {
    nativeEditor?.focus?.();
  }

  export function focusEnd() {
    nativeEditor?.focusEnd?.();
  }

  export function clear() {
    nativeEditor?.clear?.();
  }

  export function insertImage(...args) {
    if (activeProvider) {
      console.warn('[PluginEditorHost] insertImage is not supported by plugin editors');
      return;
    }
    nativeEditor?.insertImage?.(...args);
  }

  export function insertMarkdown(...args) {
    if (activeProvider) {
      console.warn('[PluginEditorHost] insertMarkdown is not supported by plugin editors');
      return;
    }
    nativeEditor?.insertMarkdown?.(...args);
  }

  $effect(() => {
    if (editorProviders) void ensureExtensionsLoaded();
  });

  const providers = $derived(
    editorProviders ? getEditorProviders(extensions.value, entityType) : []
  );

  const activeSrc = $derived(
    activeProvider
      ? `/api/plugins/${encodeURIComponent(activeProvider.pluginName)}/assets/${activeProvider.component}`
      : null
  );
  const activeStyles = $derived(
    activeProvider?.styles
      ? `/api/plugins/${encodeURIComponent(activeProvider.pluginName)}/assets/${activeProvider.styles}`
      : null
  );

  function disposeBridge() {
    if (bridge) {
      bridge.sendEditorDispose?.();
      bridge.destroy();
      bridge = null;
    }
  }

  function closeProvider() {
    disposeBridge();
    activeProvider = null;
    sessionId = null;
    pluginReady = false;
    pluginError = null;
    dirty = false;
  }

  async function confirmDiscard() {
    if (!dirty) return true;
    return confirm({
      title: t('editors.discardProviderChanges'),
      message: t('editors.discardProviderChangesMessage', { label: activeProvider?.label || '' }),
      confirmText: t('editors.discardChanges'),
      variant: 'danger',
    });
  }

  async function openProvider(provider) {
    if (!(await confirmDiscard())) return;
    sessionId = crypto.randomUUID();
    activeProvider = provider;
    pluginReady = false;
    pluginError = null;
    dirty = false;
  }

  async function returnToNative() {
    if (!(await confirmDiscard())) return;
    closeProvider();
  }

  function handleIframeLoad() {
    if (!activeProvider || !iframe) return;
    disposeBridge();
    bridge = createPluginBridge(iframe, {
      pluginName: activeProvider.pluginName,
      sessionId,
      onEditorReady: () => {
        pluginReady = true;
        // The host owns the content: it sends the current Markdown and persists
        // whatever comes back through the normal item/page APIs.
        bridge?.sendEditorInit({
          entityType,
          entityId,
          workspaceId,
          markdown: content,
          readonly,
          locale: document.documentElement.lang || null,
          theme: themeStore.resolvedTheme,
          capabilities: ['markdown.raw-editor'],
        });
      },
      onEditorChange: (message) => {
        dirty = message?.dirty ?? true;
      },
      onEditorSave: (message) => {
        if (typeof message?.markdown !== 'string') return;
        if (new TextEncoder().encode(message.markdown).length > MARKDOWN_MAX_BYTES) {
          pluginError = t('editors.providerError', { message: 'Markdown exceeds 256 KB' });
          return;
        }
        content = message.markdown;
        onContentChange?.(message.markdown);
        closeProvider();
      },
      onEditorCancel: () => closeProvider(),
      onEditorError: (message) => {
        pluginError = t('editors.providerError', { message: message?.message || '' });
      },
    });
  }

  onDestroy(() => disposeBridge());
</script>

<div class="plugin-editor-host">
  {#if providers.length > 0 && !readonly}
    <div
      class="plugin-editor-toggle"
      data-testid={testId ? `${testId}-provider-toggle` : 'plugin-editor-toggle'}
    >
      {#if activeProvider}
        <span class="plugin-editor-active">{activeProvider.label || activeProvider.id}</span>
        <button
          type="button"
          class="plugin-editor-button"
          onclick={returnToNative}
          data-testid={testId ? `${testId}-provider-native` : 'plugin-editor-native'}
        >
          {t('editors.useRichText')}
        </button>
      {:else}
        {#each providers as provider (provider.id)}
          <button
            type="button"
            class="plugin-editor-button"
            onclick={() => openProvider(provider)}
            data-testid={testId
              ? `${testId}-provider-${provider.id}`
              : `plugin-editor-provider-${provider.id}`}
          >
            {t('editors.useProvider', { label: provider.label || provider.id })}
          </button>
        {/each}
      {/if}
    </div>
  {/if}

  {#if activeProvider}
    {#if activeStyles}
      <link rel="stylesheet" href={activeStyles} />
    {/if}
    {#if pluginError}
      <div
        class="plugin-editor-error"
        role="alert"
        data-testid={testId ? `${testId}-provider-error` : 'plugin-editor-error'}
      >
        {pluginError}
      </div>
    {/if}
    {#if !pluginReady}
      <div class="plugin-editor-loading">
        <Spinner />
      </div>
    {/if}
    <iframe
      bind:this={iframe}
      src={activeSrc}
      title={activeProvider.label || activeProvider.id}
      class="plugin-editor-frame"
      data-testid={testId ? `${testId}-provider-frame` : 'plugin-editor-frame'}
      onload={handleIframeLoad}
    ></iframe>
  {:else}
    <LazyMilkdownEditor
      bind:this={nativeEditor}
      bind:content
      {readonly}
      {entityType}
      {entityId}
      {workspaceId}
      {onContentChange}
      {testId}
      {...rest}
    />
  {/if}
</div>

<style>
  .plugin-editor-host {
    width: 100%;
  }

  .plugin-editor-toggle {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .plugin-editor-button {
    font-size: 0.75rem;
    padding: 0.125rem 0.5rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.25rem;
    background: var(--ds-surface);
    color: var(--ds-text-subtle);
    cursor: pointer;
  }

  .plugin-editor-button:hover {
    background: var(--ds-surface-raised);
  }

  .plugin-editor-active {
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }

  .plugin-editor-frame {
    width: 100%;
    min-height: 240px;
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
    display: block;
    background: var(--ds-surface);
  }

  .plugin-editor-error {
    color: var(--ds-text-danger);
    font-size: 0.8125rem;
    margin-bottom: 0.25rem;
  }

  .plugin-editor-loading {
    display: flex;
    justify-content: center;
    padding: 1rem 0;
  }
</style>
