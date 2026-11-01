<script>
  import { createParser } from '@openuidev/svelte-lang';
  import IconMessageChatbot from '@tabler/icons-svelte-runes/icons/message-chatbot';
  import Button from '../components/Button.svelte';
  import SelectField from '../components/SelectField.svelte';
  import TextField from '../components/TextField.svelte';
  import TextareaField from '../components/TextareaField.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import { assetSets } from '../api/assets.js';
  import { customWidgets } from '../api/customWidgets.js';
  import { aiStore } from '../stores/aiStore.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import CustomWidgetRenderer from './CustomWidgetRenderer.svelte';
  import { widgetLibrary } from './library.js';
  import { assetSpec, itemSpec, sourceKey } from './querySpec.js';

  /**
   * @type {{
   *   isOpen?: boolean,
   *   widgetId?: number|null,
   *   workspaceId?: number|null,
   *   onclose?: () => void,
   *   onsaved?: (widget: object) => void
   * }}
   */
  let { isOpen = $bindable(false), widgetId = null, workspaceId = null, onclose = null, onsaved = null } = $props();

  const parser = createParser(widgetLibrary.toJSONSchema());
  const examplePrompts = [
    'My open work, with counts and a table',
    'Items completed per assignee over the last 30 days',
    'Overdue items grouped by priority',
  ];

  let loading = $state(false);
  let saving = $state(false);
  let generating = $state(false);
  let name = $state('');
  // The user's words. Never overwritten by a generated draft: the model's
  // description is widget metadata, not a replacement for the request.
  let prompt = $state('');
  let widgetDescription = $state('');
  let program = $state('');
  /** @type {{
   *   item?: { ql?: string, sort?: string, limit?: number },
   *   asset?: { ql?: string, sort?: string, limit?: number, asset_set_id?: number }
   * }}
   */
  let queries = $state({});
  let error = $state('');
  let parseErrors = $state([]);
  let sources = $state({});
  let assetSetOptions = $state([]);
  let previewVersion = $state(0);

  const isEditing = $derived(Boolean(widgetId));
  const aiReady = $derived(aiStore.available && aiStore.isFeatureEnabled('custom_widget'));
  const itemQuery = $derived(queries?.item?.ql ?? '');
  const assetQuery = $derived(queries?.asset?.ql ?? '');
  const assetSetId = $derived(queries?.asset?.asset_set_id ?? '');

  // Plain flag, not $state: reading reactive state inside the effect that
  // writes it would re-trigger the effect forever.
  let assetSetsRequested = false;

  $effect(() => {
    if (!isOpen) return;
    aiStore.load();
    if (!assetSetsRequested) {
      assetSetsRequested = true;
      loadAssetSets();
    }
    if (widgetId) loadWidget(widgetId);
    else reset();
  });

  // Re-parse the program whenever it changes so raw edits report errors inline.
  $effect(() => {
    if (!program) {
      parseErrors = [];
      return;
    }
    const result = parser.parse(program);
    parseErrors = result?.meta?.errors ?? [];
  });

  // Preview through the same data path the dashboard uses.
  $effect(() => {
    if (!isOpen) return;
    void previewVersion;
    void program;
    refreshPreview();
  });

  function reset() {
    name = '';
    prompt = '';
    widgetDescription = '';
    program = '';
    queries = {};
    error = '';
    sources = {};
  }

  async function loadWidget(id) {
    loading = true;
    error = '';
    try {
      const widget = await customWidgets.get(id);
      name = widget.name ?? '';
      prompt = widget.description ?? '';
      widgetDescription = widget.description ?? '';
      program = widget.program ?? '';
      queries = widget.queries ?? {};
    } catch {
      error = t('dashboard.widgetBuilder.loadFailed');
    } finally {
      loading = false;
    }
  }

  async function loadAssetSets() {
    try {
      const sets = await assetSets.getAll();
      assetSetOptions = (sets ?? []).map((set) => ({ value: String(set.id), label: set.name }));
    } catch {
      assetSetOptions = [];
    }
  }

  function slotsFromQueries() {
    const slots = [];
    if (queries?.item) {
      const spec = itemSpec({ query: queries.item.ql, sort: queries.item.sort, limit: queries.item.limit });
      slots.push({
        key: sourceKey(spec),
        entity: 'item',
        ql: queries.item.ql ?? '',
        sort: queries.item.sort ?? '',
        limit: queries.item.limit ?? 0,
      });
    }
    if (queries?.asset) {
      const spec = assetSpec({
        query: queries.asset.ql,
        sort: queries.asset.sort,
        limit: queries.asset.limit,
        assetSetId: queries.asset.asset_set_id,
      });
      slots.push({
        key: sourceKey(spec),
        entity: 'asset',
        ql: queries.asset.ql ?? '',
        sort: queries.asset.sort ?? '',
        limit: queries.asset.limit ?? 0,
        asset_set_id: queries.asset.asset_set_id,
      });
    }
    return slots;
  }

  /** Slots already in flight, keyed by source key, so parallel components share one request. */
  const pendingSources = new Map();

  function toSlot(spec, key) {
    if (spec.entity === 'links') return { key, entity: 'links', links: spec.links };
    const slot = { key, entity: spec.entity, ql: spec.ql ?? '', sort: spec.sort ?? '', limit: spec.limit ?? 0 };
    if (spec.entity === 'asset') slot.asset_set_id = spec.asset_set_id;
    return slot;
  }

  /**
   * Resolve one spec on demand. The prefetch only covers the stored queries, so
   * a component whose props produce a different spec (a different limit, say)
   * would otherwise sit on a skeleton forever.
   */
  function requestSource(spec, key) {
    if (!key || sources[key] || pendingSources.has(key)) return;
    sources = { ...sources, [key]: { status: 'loading' } };
    const request = customWidgets
      .data({ workspace_id: workspaceId ?? undefined, slots: [toSlot(spec, key)] })
      .then((response) => {
        sources = {
          ...sources,
          [key]: response?.results?.[key] ?? {
            status: 'error',
            error: { code: 'missing', message: t('dashboard.customWidgets.sourceError') },
          },
        };
      })
      .catch(() => {
        sources = {
          ...sources,
          [key]: {
            status: 'error',
            error: { code: 'unavailable', message: t('dashboard.customWidgets.sourceError') },
          },
        };
      })
      .finally(() => pendingSources.delete(key));
    pendingSources.set(key, request);
  }

  async function refreshPreview() {
    const slots = slotsFromQueries();
    if (slots.length === 0) {
      sources = {};
      return;
    }
    try {
      const response = await customWidgets.data({ workspace_id: workspaceId ?? undefined, slots });
      sources = response?.results ?? {};
    } catch {
      sources = {};
    }
  }

  async function generate() {
    if (!aiReady) return;
    generating = true;
    error = '';
    try {
      const draft = await customWidgets.generate({
        prompt: prompt || name || 'a useful dashboard widget',
        current_program: program,
        current_queries: queries,
        workspace_id: workspaceId ?? undefined,
      });
      name = draft.name ?? name;
      widgetDescription = draft.description ?? widgetDescription;
      program = draft.program ?? '';
      queries = draft.queries ?? {};
      previewVersion += 1;
    } catch (requestError) {
      error = requestError?.message || t('dashboard.widgetBuilder.generateFailed');
    } finally {
      generating = false;
    }
  }

  function updateItemQuery(value) {
    queries = { ...queries, item: { ...(queries.item ?? {}), ql: value } };
    previewVersion += 1;
  }

  function updateAssetQuery(value) {
    queries = { ...queries, asset: { ...(queries.asset ?? {}), ql: value } };
    previewVersion += 1;
  }

  function updateAssetSet(value) {
    const next = { ...queries };
    if (!value) {
      if (next.asset) next.asset = { ...next.asset, asset_set_id: undefined };
    } else {
      next.asset = { ...(next.asset ?? { ql: '' }), asset_set_id: Number(value) };
    }
    queries = next;
    previewVersion += 1;
  }

  async function save() {
    saving = true;
    error = '';
    const payload = {
      name,
      description: widgetDescription || prompt,
      program,
      queries,
      workspace_id: workspaceId ?? null,
    };
    try {
      const widget = isEditing
        ? await customWidgets.update(widgetId, payload)
        : await customWidgets.create(payload);
      onsaved?.(widget);
      isOpen = false;
      onclose?.();
    } catch (requestError) {
      error = requestError?.message || t('dashboard.widgetBuilder.saveFailed');
    } finally {
      saving = false;
    }
  }

  const canSave = $derived(
    !saving && name.trim() !== '' && program.trim() !== '' && parseErrors.length === 0
  );
</script>

<Modal bind:isOpen maxWidth="max-w-4xl" maxHeight="max-h-[90vh]" onclose={onclose} dataTestid="widget-builder-modal">
  <ModalHeader
    title={isEditing ? t('dashboard.widgetBuilder.editTitle') : t('dashboard.widgetBuilder.createTitle')}
    subtitle={t('dashboard.widgetBuilder.subtitle')}
    icon={IconMessageChatbot}
    onclose={() => {
      isOpen = false;
      onclose?.();
    }}
  />

  <div class="grid max-h-[70vh] gap-6 overflow-y-auto px-6 py-5 md:grid-cols-2">
    <div class="space-y-4">
      <TextField
        label={t('dashboard.widgetBuilder.nameLabel')}
        bind:value={name}
        placeholder={t('dashboard.widgetBuilder.namePlaceholder')}
        dataTestid="widget-builder-name"
      />
      <TextareaField
        label={t('dashboard.widgetBuilder.promptLabel')}
        bind:value={prompt}
        rows={2}
        placeholder={t('dashboard.widgetBuilder.promptPlaceholder')}
        dataTestid="widget-builder-prompt"
      />

      {#if aiReady}
        <div class="flex flex-wrap items-center gap-2">
          <Button
            variant="primary"
            size="small"
            loading={generating}
            disabled={generating}
            onclick={generate}
            dataTestid="widget-builder-generate"
          >
            {generating ? t('dashboard.widgetBuilder.generating') : t('dashboard.widgetBuilder.generate')}
          </Button>
          {#each examplePrompts as example (example)}
            <button
              type="button"
              class="rounded-full px-2.5 py-1 text-xs"
              style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle);"
              onclick={() => {
                prompt = example;
              }}
            >
              {example}
            </button>
          {/each}
        </div>
      {:else}
        <p class="text-xs" style="color: var(--ds-text-subtle);" data-testid="widget-builder-ai-unavailable">
          {isEditing
            ? t('dashboard.widgetBuilder.aiUnavailableEditing')
            : t('dashboard.widgetBuilder.aiUnavailable')}
        </p>
      {/if}

      {#if parseErrors.length > 0}
        <ul class="space-y-1 text-xs" style="color: var(--ds-danger, #dc2626);" data-testid="widget-builder-parse-errors">
          {#each parseErrors.slice(0, 5) as parseError (parseError.code + (parseError.path ?? ''))}
            <li>{parseError.component ? `${parseError.component}: ` : ''}{parseError.message ?? parseError.code}</li>
          {/each}
        </ul>
      {/if}

      <TextField
        label={t('dashboard.widgetBuilder.itemQueryLabel')}
        value={itemQuery}
        placeholder="status_completed = false"
        oninput={(event) => updateItemQuery(event?.target?.value ?? '')}
        dataTestid="widget-builder-item-query"
      />
      <TextField
        label={t('dashboard.widgetBuilder.assetQueryLabel')}
        value={assetQuery}
        oninput={(event) => updateAssetQuery(event?.target?.value ?? '')}
        dataTestid="widget-builder-asset-query"
      />
      <SelectField
        label={t('dashboard.widgetBuilder.assetSetLabel')}
        value={assetSetId ? String(assetSetId) : ''}
        options={assetSetOptions}
        placeholder={t('dashboard.widgetBuilder.assetSetNone')}
        onchange={(event) => updateAssetSet(event?.target?.value ?? '')}
        id="widget-builder-asset-set"
      />

      {#if error}
        <p class="text-sm" style="color: var(--ds-danger, #dc2626);" data-testid="widget-builder-error">{error}</p>
      {/if}
    </div>

    <div class="space-y-2">
      <p class="text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">
        {t('dashboard.widgetBuilder.previewLabel')}
      </p>
      <div
        class="min-h-40 rounded-lg border p-3"
        style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
        data-testid="widget-builder-preview"
      >
        {#if loading}
          <p class="text-sm" style="color: var(--ds-text-subtle);">{t('dashboard.widgetBuilder.loading')}</p>
        {:else if program}
          <CustomWidgetRenderer {program} tileTitle={name} {sources} {workspaceId} {requestSource} />
        {:else}
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {t('dashboard.widgetBuilder.previewEmpty')}
          </p>
        {/if}
      </div>
    </div>
  </div>

  <div class="flex items-center justify-end gap-2 border-t px-6 py-4" style="border-color: var(--ds-border);">
    <Button
      variant="ghost"
      onclick={() => {
        isOpen = false;
        onclose?.();
      }}
    >
      {t('common.cancel')}
    </Button>
    <Button variant="primary" loading={saving} disabled={!canSave} onclick={save} dataTestid="widget-builder-save">
      {saving ? t('dashboard.widgetBuilder.saving') : t('common.save')}
    </Button>
  </div>
</Modal>
