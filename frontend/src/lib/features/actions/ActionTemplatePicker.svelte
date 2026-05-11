<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import { Sparkles } from '@lucide/svelte';
  import { actionTemplates } from '../../api/actions.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';

  /** @type {{ workspaceId: number, onclose?: () => void, onapplied?: (result: any) => void }} */
  let { workspaceId, onclose, onapplied } = $props();

  /** @type {{ key: string, name: string, description?: string, category?: string, trigger_type: string, node_count: number }[]} */
  let templates = $state([]);
  let loading = $state(true);
  let applyingKey = $state(/** @type {string | null} */ (null));
  let error = $state(/** @type {string | null} */ (null));

  $effect(() => {
    void loadTemplates();
  });

  async function loadTemplates() {
    loading = true;
    error = null;
    try {
      templates = await actionTemplates.list();
    } catch (e) {
      error = e?.message || 'Failed to load templates';
    } finally {
      loading = false;
    }
  }

  async function applyTemplate(key) {
    applyingKey = key;
    error = null;
    try {
      const result = await actionTemplates.apply(workspaceId, key);
      onapplied?.(result);
      onclose?.();
    } catch (e) {
      error = e?.message || 'Failed to apply template';
      applyingKey = null;
    }
  }
</script>

<Modal isOpen={true} onclose={onclose} maxWidth="max-w-2xl">
  <ModalHeader
    title={t('actions.templates.pickTitle', 'Choose an action template')}
    icon={Sparkles}
    onClose={onclose}
  />
  <div class="p-6">
    {#if loading}
      <div class="state">{t('common.loading', 'Loading...')}</div>
    {:else if error}
      <div class="state error">{error}</div>
    {:else if templates.length === 0}
      <div class="state">{t('actions.templates.empty', 'No templates available.')}</div>
    {:else}
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
        {t('actions.templates.help', 'Apply a shipped automation blueprint to this workspace. Each apply creates a new action you can edit afterwards.')}
      </p>
      <div class="template-list">
        {#each templates as tmpl (tmpl.key)}
          <div class="template-card">
            <div class="template-meta">
              <h4 class="template-name">{tmpl.name}</h4>
              {#if tmpl.category}
                <span class="category">{tmpl.category}</span>
              {/if}
            </div>
            {#if tmpl.description}
              <p class="description">{tmpl.description}</p>
            {/if}
            <div class="footer">
              <span class="trigger">Trigger: <code>{tmpl.trigger_type}</code></span>
              <Button
                variant="primary"
                size="sm"
                disabled={applyingKey !== null}
                onclick={() => applyTemplate(tmpl.key)}
              >
                {applyingKey === tmpl.key ? t('common.applying', 'Applying...') : t('actions.templates.apply', 'Apply')}
              </Button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</Modal>

<style>
  .template-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .template-card {
    border: 1px solid var(--ds-border);
    border-radius: 8px;
    padding: 12px 14px;
    background: var(--ds-surface);
    transition: background 120ms ease;
  }
  .template-card:hover {
    background: var(--ds-background-neutral-hovered);
  }
  .template-meta {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
  }
  .template-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--ds-text);
    margin: 0;
  }
  .category {
    font-size: 11px;
    color: var(--ds-text-subtle);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .description {
    color: var(--ds-text-subtle);
    font-size: 12px;
    margin: 6px 0 10px;
    white-space: pre-line;
  }
  .footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }
  .trigger {
    font-size: 12px;
    color: var(--ds-text-subtle);
  }
  code {
    background: var(--ds-background-neutral);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
  }
  .state {
    padding: 24px;
    text-align: center;
    color: var(--ds-text-subtle);
  }
  .state.error {
    color: var(--ds-text-danger);
  }
</style>
