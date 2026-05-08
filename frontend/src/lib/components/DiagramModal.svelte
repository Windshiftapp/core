<script>
  import ExcalidrawEditor from './ExcalidrawEditor.svelte';
  import Button from './Button.svelte';
  import { api } from '../api.js';
  import { themeStore } from '../stores/theme.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { portal } from '../actions/portal.js';

  let { itemId, diagram = null, onClose = () => {}, onSave = () => {} } = $props();

  let editorComponent = $state(null);
  let diagramName = $state('');
  let initialData = $state(null);
  let loadingSeed = $state(false);
  let saving = $state(false);
  let hasChanges = $state(false);
  let initialized = false;

  $effect.pre(() => {
    if (!initialized) {
      initialized = true;
      diagramName = diagram ? diagram.name : t('components.diagram.untitled');
      if (diagram && diagram.diagram_data) {
        try {
          const parsed = JSON.parse(diagram.diagram_data);
          // Mermaid seed wrapper produced by agent-facing tools (MCP / ws CLI):
          // expand to a real Excalidraw scene the first time the diagram opens.
          // Once the user saves, the scene is persisted and the source is gone.
          if (parsed && parsed.type === 'mermaid' && typeof parsed.source === 'string') {
            loadingSeed = true;
            seedFromMermaid(parsed.source);
          } else {
            initialData = parsed;
          }
        } catch (err) {
          console.error('Failed to parse diagram data:', err);
          errorToast(t('components.diagram.saveError'));
        }
      }
    }
  });

  async function seedFromMermaid(source) {
    try {
      const [{ parseMermaidToExcalidraw }, { convertToExcalidrawElements }] = await Promise.all([
        import('@excalidraw/mermaid-to-excalidraw'),
        import('@excalidraw/excalidraw'),
      ]);
      const { elements: skeletons, files } = await parseMermaidToExcalidraw(source);
      initialData = {
        elements: convertToExcalidrawElements(skeletons),
        appState: {},
        files: files || {},
        scrollToContent: true,
      };
    } catch (err) {
      console.error('Failed to convert mermaid source:', err);
      errorToast(t('components.diagram.saveError'));
    } finally {
      loadingSeed = false;
    }
  }

  function handleEditorChange(sceneData) {
    hasChanges = true;
  }

  async function handleSave() {
    if (!diagramName.trim()) {
      errorToast(t('components.diagram.nameRequired'));
      return;
    }
    if (!editorComponent) return;

    try {
      saving = true;

      // Get scene data from editor
      const sceneData = editorComponent.getSceneData();
      const diagramData = JSON.stringify(sceneData);

      if (diagram) {
        // Update existing diagram
        await api.updateDiagram(diagram.id, diagramName, diagramData);
      } else {
        // Create new diagram
        await api.createDiagram(itemId, diagramName, diagramData);
      }

      onSave();
      onClose();
    } catch (err) {
      console.error('Failed to save diagram:', err);
      errorToast(t('components.diagram.saveError'));
    } finally {
      saving = false;
    }
  }

  async function handleClose() {
    if (hasChanges) {
      const confirmed = await confirm({
        title: t('common.discardChanges'),
        message: t('components.diagram.unsavedChangesConfirm'),
        confirmText: t('common.discard'),
        cancelText: t('common.cancel'),
        variant: 'warning'
      });
      if (!confirmed) return;
    }
    onClose();
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      event.stopPropagation();
      handleClose();
    }
  }
</script>

<!-- Modal overlay -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  use:portal
  class="fixed inset-0 flex items-center justify-center z-[60]"
  style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
  onkeydown={handleKeyDown}
>
  <!-- Modal container -->
  <div class="rounded shadow-xl w-full h-full max-w-[95vw] max-h-[95vh] flex flex-col" style="background-color: var(--ds-surface-raised);">
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center space-x-4 flex-1 min-w-0">
        <input
          type="text"
          bind:value={diagramName}
          placeholder={t('components.diagram.namePlaceholder')}
          class="px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 max-w-md"
          style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
        />
        {#if hasChanges}
          <span class="text-sm text-orange-600">{t('components.diagram.unsavedChanges')}</span>
        {/if}
      </div>
      <div class="flex items-center space-x-2 shrink-0">
        <Button variant="default" disabled={saving} onclick={handleClose}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" disabled={saving || loadingSeed} loading={saving} onclick={handleSave}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    </div>

    <!-- Editor -->
    <div class="flex-1 overflow-hidden">
      {#if loadingSeed}
        <div class="w-full h-full flex items-center justify-center">
          <span class="text-sm" style="color: var(--ds-text-muted);">{t('common.loading')}</span>
        </div>
      {:else}
        <ExcalidrawEditor
          bind:this={editorComponent}
          initialData={initialData}
          onChange={handleEditorChange}
          theme={themeStore.resolvedTheme}
        />
      {/if}
    </div>
  </div>
</div>

