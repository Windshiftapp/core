<script>
  import { useEventListener } from 'runed';
  import ExcalidrawEditor from './ExcalidrawEditor.svelte';
  import Button from './Button.svelte';
  import { api } from '../api.js';
  import { themeStore } from '../stores/theme.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let { itemId, diagram = null, onClose = () => {}, onSave = () => {} } = $props();

  let editorComponent = $state(null);
  let diagramName = $state(diagram ? diagram.name : t('components.diagram.untitled'));
  let initialData = null;
  let saving = $state(false);
  let hasChanges = $state(false);

  if (diagram && diagram.diagram_data) {
    try {
      initialData = JSON.parse(diagram.diagram_data);
    } catch (err) {
      console.error('Failed to parse diagram data:', err);
    }
  }

  function handleEditorChange(sceneData) {
    hasChanges = true;
  }

  async function handleSave() {
    if (!diagramName.trim()) {
      alert(t('components.diagram.nameRequired'));
      return;
    }

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
      alert(t('components.diagram.saveError'));
    } finally {
      saving = false;
    }
  }

  function handleClose() {
    if (hasChanges && !confirm(t('components.diagram.unsavedChangesConfirm'))) {
      return;
    }
    onClose();
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      handleClose();
    }
  }

  useEventListener(() => window, 'keydown', handleKeyDown);
</script>

<!-- Modal overlay -->
<div
  class="fixed inset-0 flex items-center justify-center z-50"
  style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
>
  <!-- Modal container -->
  <div class="rounded shadow-xl w-full h-full max-w-[95vw] max-h-[95vh] flex flex-col" style="background-color: var(--ds-surface-raised);">
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center space-x-4 flex-1">
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
      <div class="flex items-center space-x-2">
        <Button variant="default" disabled={saving} onclick={handleClose}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" disabled={saving} loading={saving} onclick={handleSave}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    </div>

    <!-- Editor -->
    <div class="flex-1 overflow-hidden">
      <ExcalidrawEditor
        bind:this={editorComponent}
        initialData={initialData}
        onChange={handleEditorChange}
        theme={themeStore.resolvedTheme}
      />
    </div>
  </div>
</div>

