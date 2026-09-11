<script>
  import ExcalidrawEditor from '../../components/ExcalidrawEditor.svelte';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import { api } from '../../api.js';
  import { themeStore } from '../../stores/theme.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { portal } from '../../actions/portal.js';
  import {
    pageDiagramSceneFingerprint,
    preparePageDiagramScene,
  } from './pageDiagramScene.js';

  let {
    open = $bindable(false),
    mode = 'create',                  // 'create' | 'edit'
    initialAttachmentId = null,
    initialName = '',
    workspaceId,
    pageId,
    expectedContentHash = '',
    onSaved = (_payload) => {},
  } = $props();

  let editorComponent = $state(null);
  let diagramName = $state('');
  let initialData = $state(null);
  let loadingSeed = $state(false);
  let saving = $state(false);
  let hasChanges = $state(false);
  let loadError = $state('');
  let lastLoadedId = null;
  let editorBaselineFingerprint = null;

  $effect(() => {
    if (!open) {
      // Reset when modal closes so reopening triggers a fresh load.
      lastLoadedId = null;
      initialData = null;
      hasChanges = false;
      loadError = '';
      diagramName = '';
      editorBaselineFingerprint = null;
      return;
    }
    diagramName = initialName || t('editors.diagramUntitled');
    if (mode === 'edit' && initialAttachmentId && initialAttachmentId !== lastLoadedId) {
      lastLoadedId = initialAttachmentId;
      void loadAttachment(initialAttachmentId);
    } else if (mode === 'create') {
      initialData = { elements: [], appState: {}, files: {}, scrollToContent: true };
      editorBaselineFingerprint = null;
    }
  });

  async function loadAttachment(id) {
    loadingSeed = true;
    loadError = '';
    editorBaselineFingerprint = null;
    try {
      const res = await fetch(`/api/attachments/${id}/download`, { credentials: 'same-origin' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      initialData = await preparePageDiagramScene(await res.json());
    } catch (err) {
      console.error('Failed to load diagram for edit:', err);
      errorToast(t('editors.diagramLoadError'));
      loadError = err?.message || t('editors.diagramLoadError');
      initialData = null;
    } finally {
      loadingSeed = false;
    }
  }

  function handleEditorChange(sceneData) {
    const fingerprint = pageDiagramSceneFingerprint(sceneData);
    // Excalidraw normalizes the loaded scene while mounting. Treat its first
    // emitted state as the baseline, then compare complete scene content.
    if (editorBaselineFingerprint === null) {
      editorBaselineFingerprint = fingerprint;
      hasChanges = false;
      return;
    }
    hasChanges = fingerprint !== editorBaselineFingerprint;
  }

  async function handleSave() {
    if (!editorComponent || !workspaceId || !pageId) return;
    saving = true;
    try {
      const sceneData = editorComponent.getSceneData();
      const name = diagramName.trim() || t('editors.diagramUntitled');
      const request = {
        name,
        excalidraw: sceneData,
        expectedContentHash,
      };
      const resp =
        mode === 'edit' && initialAttachmentId
          ? await api.pages.updateDiagram(
              workspaceId,
              pageId,
              initialAttachmentId,
              request
            )
          : await api.pages.createDiagram(workspaceId, pageId, {
              ...request,
              placement: 'end',
            });
      const attachmentId = resp?.attachment_id;
      if (!Number.isInteger(attachmentId)) {
        throw new Error('Diagram response missing attachment id');
      }
      // The shared service has already mutated the Page. Reload its canonical
      // Markdown so the editor mirrors the exact server placement/fence and
      // does not issue a redundant autosave.
      const updatedPage = await api.pages.getPage(workspaceId, pageId);
      onSaved({
        attachmentId,
        name: resp?.name || name,
        // Keep the hash and Markdown from the same read. Another writer may
        // have updated the Page after the diagram mutation completed.
        contentHash: updatedPage?.content_hash || resp?.content_hash || '',
        pageContent: updatedPage?.content || '',
      });
      open = false;
    } catch (err) {
      console.error('Failed to save diagram:', err);
      errorToast(t('editors.diagramSaveError'));
    } finally {
      saving = false;
    }
  }

  async function handleClose() {
    if (hasChanges) {
      const ok = await confirm({
        title: t('common.discardChanges'),
        message: t('editors.diagramUnsavedConfirm'),
        confirmText: t('common.discard'),
        cancelText: t('common.cancel'),
        variant: 'warning',
      });
      if (!ok) return;
    }
    open = false;
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      event.stopPropagation();
      void handleClose();
    }
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="fixed inset-0 flex items-center justify-center z-[60]"
    style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
    onkeydown={handleKeyDown}
    data-testid="page-diagram-modal"
  >
    <div class="rounded shadow-xl w-full h-full max-w-[95vw] max-h-[95vh] flex flex-col" style="background-color: var(--ds-surface-raised);">
      <div class="flex items-center justify-between p-4 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center space-x-4 flex-1 min-w-0">
          <Input
            type="text"
            bind:value={diagramName}
            placeholder={t('editors.diagramNamePlaceholder')}
            class="max-w-md"
            dataTestid="page-diagram-name"
            size="small"
          />
          {#if hasChanges}
            <span class="text-sm text-ds-text-warning">{t('editors.diagramUnsaved')}</span>
          {/if}
        </div>
        <div class="flex items-center space-x-2 shrink-0">
          <Button variant="default" disabled={saving} onclick={handleClose} dataTestid="page-diagram-cancel">
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            disabled={saving || loadingSeed || !!loadError || !editorComponent}
            loading={saving}
            onclick={handleSave}
            dataTestid="page-diagram-save"
          >
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </div>
      <div class="flex-1 overflow-hidden">
        {#if loadingSeed}
          <div class="w-full h-full flex items-center justify-center">
            <span class="text-sm" style="color: var(--ds-text-muted);">{t('common.loading')}</span>
          </div>
        {:else if loadError}
          <div class="w-full h-full flex items-center justify-center" data-testid="page-diagram-load-error">
            <span class="text-sm" style="color: var(--ds-text-danger);">{loadError}</span>
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
{/if}
