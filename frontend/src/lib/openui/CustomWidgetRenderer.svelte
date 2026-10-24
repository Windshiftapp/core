<script>
  import { Renderer } from '@openuidev/svelte-lang';
  import { t } from '../stores/i18n.svelte.js';
  import { setWidgetDataContext } from './context.js';
  import { widgetLibrary } from './library.js';
  import { stripDuplicateTitle } from './programTitle.js';

  /**
   * Renders one widget program. The data context is set here so every
   * data-bound component in the program can read its rows without the program
   * carrying any identifiers.
   *
   * @type {{
   *   program?: string,
   *   tileTitle?: string,
   *   sources?: Record<string, import('./context.js').SourceResult>,
   *   assetSetId?: number|null,
   *   workspaceId?: number|null,
   *   requestSource?: ((spec: object, key: string) => void) | null,
   *   openItem?: ((itemId: number) => void) | null,
   *   onParseErrors?: ((errors: Array<object>) => void) | null
   * }}
   */
  let {
    program = '',
    tileTitle = '',
    sources = {},
    assetSetId = null,
    workspaceId = null,
    requestSource = null,
    openItem = null,
    onParseErrors = null,
  } = $props();

  // The tile heading already shows the widget name, so drop a leading Title
  // that repeats it. Programs saved before the prompt said so still carry one.
  const response = $derived(stripDuplicateTitle(program, tileTitle));

  setWidgetDataContext({
    get sources() {
      return sources;
    },
    get assetSetId() {
      return assetSetId;
    },
    get workspaceId() {
      return workspaceId;
    },
    get requestSource() {
      return requestSource;
    },
    get openItem() {
      return openItem;
    },
  });

  function handleParseResult(result) {
    if (!onParseErrors) return;
    const errors = result?.meta?.errors ?? [];
    if (errors.length > 0) onParseErrors(errors);
  }
</script>

<svelte:boundary>
  <Renderer response={response} library={widgetLibrary} onParseResult={handleParseResult} />

  {#snippet failed(error)}
    <div
      class="rounded-md px-3 py-2 text-sm"
      style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
      data-testid="openui-render-error"
    >
      {t('dashboard.customWidgets.renderError')}
      {#if error instanceof Error}
        <span class="block text-xs opacity-70">{error.message}</span>
      {/if}
    </div>
  {/snippet}
</svelte:boundary>
