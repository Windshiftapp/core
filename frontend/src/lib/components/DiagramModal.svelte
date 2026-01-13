<script>
  import { onMount } from 'svelte';
  import ExcalidrawEditor from './ExcalidrawEditor.svelte';
  import Button from './Button.svelte';
  import { api } from '../api.js';
  import { themeStore } from '../stores/theme.svelte.js';

  export let itemId;
  export let diagram = null; // null for new diagram, object for editing
  export let onClose = () => {};
  export let onSave = () => {};

  let editorComponent;
  let diagramName = diagram ? diagram.name : 'Untitled Diagram';
  let initialData = null;
  let saving = false;
  let hasChanges = false;

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
      alert('Please enter a diagram name');
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
      alert('Failed to save diagram');
    } finally {
      saving = false;
    }
  }

  function handleClose() {
    if (hasChanges && !confirm('You have unsaved changes. Are you sure you want to close?')) {
      return;
    }
    onClose();
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      handleClose();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  });
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
          placeholder="Diagram name"
          class="px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 max-w-md"
          style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
        />
        {#if hasChanges}
          <span class="text-sm text-orange-600">Unsaved changes</span>
        {/if}
      </div>
      <div class="flex items-center space-x-2">
        <Button variant="default" disabled={saving} onclick={handleClose}>
          Cancel
        </Button>
        <Button variant="primary" disabled={saving} loading={saving} onclick={handleSave}>
          {saving ? 'Saving...' : 'Save'}
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

