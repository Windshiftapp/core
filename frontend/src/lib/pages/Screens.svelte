<script>
  import { onMount } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import { Plus, Edit, Trash2, Settings, MoreHorizontal, Circle, Layout } from 'lucide-svelte';
  import { SYSTEM_FIELDS, getSystemFieldName } from '../stores/fieldConfig.js';
  import { screenEditorStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DropIndicator from '../layout/DropIndicator.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Checkbox from '../components/Checkbox.svelte';

  // Bind to store values
  let screens = $derived(screenEditorStore.screens);
  let showCreateForm = $derived(screenEditorStore.showCreateForm);
  let editingScreen = $derived(screenEditorStore.editingScreen);
  let showFieldEditor = $derived(screenEditorStore.showFieldEditor);
  let editingScreenFields = $derived(screenEditorStore.editingScreenFields);
  let formData = $derived(screenEditorStore.formData);
  let screenFields = $derived(screenEditorStore.screenFields);
  let fieldSearchQuery = $derived(screenEditorStore.fieldSearchQuery);
  let searchFilteredFields = $derived(screenEditorStore.searchFilteredFields);
  let draggedField = $derived(screenEditorStore.draggedField);
  let fieldDragState = $derived(screenEditorStore.fieldDragState);
  let fieldWidths = $derived(screenEditorStore.fieldWidths);

  let setupCleanups = [];
  let setupTimeout;

  onMount(async () => {
    await screenEditorStore.loadScreens();
  });

  $effect(() => {
    return () => {
      cleanupDragAndDrop();
    };
  });

  function cleanupDragAndDrop() {
    if (setupTimeout) clearTimeout(setupTimeout);
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
    screenEditorStore.clearDragState();
  }

  function setupDragAndDrop() {
    cleanupDragAndDrop();

    // Setup available fields as draggable
    document.querySelectorAll('[data-available-field]').forEach(element => {
      const fieldData = JSON.parse(element.dataset.availableField);

      const cleanup = draggable({
        element,
        getInitialData: () => ({ field: fieldData, type: 'available-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => { element.style.opacity = ''; }
      });

      setupCleanups.push(cleanup);
    });

    // Setup screen fields as both draggable and drop targets with edge detection
    document.querySelectorAll('[data-screen-field]').forEach(element => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.fieldId;

      screenEditorStore.setDragState(fieldId, { closestEdge: null });

      // Make draggable
      const dragHandle = element.querySelector('.cursor-grab');
      const draggableCleanup = draggable({
        element,
        dragHandle: dragHandle || element,
        getInitialData: () => ({ fieldIndex, fieldId, type: 'screen-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => {
          element.style.opacity = '';
          screenEditorStore.clearDragState();
        }
      });

      // Make drop target with edge detection
      const dropTargetCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'screen-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'available-field' || data.type === 'screen-field';
        },
        getData: ({ input, element }) => {
          return attachClosestEdge({}, { input, element, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          screenEditorStore.setDragState(fieldId, { closestEdge });
        },
        onDragLeave: () => {
          screenEditorStore.setDragState(fieldId, { closestEdge: null });
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          const data = source.data;

          if (data.type === 'available-field') {
            screenEditorStore.addFieldAtPosition(data.field, fieldIndex, closestEdge);
          } else if (data.type === 'screen-field') {
            screenEditorStore.reorderFieldWithEdge(data.fieldIndex, fieldIndex, closestEdge);
          }

          screenEditorStore.setDragState(fieldId, { closestEdge: null });
        }
      });

      setupCleanups.push(() => {
        draggableCleanup();
        dropTargetCleanup();
      });
    });

    // Setup drop zone for empty area / append to end
    const dropZone = document.querySelector('[data-drop-zone]');
    if (dropZone) {
      const cleanup = dropTargetForElements({
        element: dropZone,
        canDrop: ({ source }) => source.data.type === 'available-field',
        onDragEnter: () => { screenEditorStore.setDraggedField(true); },
        onDragLeave: () => { screenEditorStore.clearDraggedField(); },
        onDrop: ({ source }) => {
          if (source.data.type === 'available-field') {
            screenEditorStore.addFieldToScreen(source.data.field);
          }
          screenEditorStore.clearDraggedField();
        }
      });
      setupCleanups.push(cleanup);
    }
  }

  // Re-setup drag and drop when field editor is shown or fields change
  $effect(() => {
    if (showFieldEditor && screenFields && typeof document !== 'undefined') {
      if (setupTimeout) clearTimeout(setupTimeout);
      setupTimeout = setTimeout(() => setupDragAndDrop(), 50);
    }
  });

  function startCreate() {
    screenEditorStore.startCreate();
  }

  function startEdit(screen) {
    screenEditorStore.startEdit(screen);
  }

  async function startEditFields(screen) {
    await screenEditorStore.startEditFields(screen);
  }

  function cancelForm() {
    screenEditorStore.cancelForm();
  }

  function cancelFieldEditor() {
    screenEditorStore.cancelFieldEditor();
    cleanupDragAndDrop();
  }

  async function saveScreen() {
    try {
      await screenEditorStore.saveScreen();
    } catch (error) {
      alert(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  async function saveScreenFields() {
    try {
      await screenEditorStore.saveScreenFields();
    } catch (error) {
      alert(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  async function deleteScreen(screen) {
    if (screen.id === 1) {
      alert(t('dialogs.alerts.cannotDeleteDefaultScreen'));
      return;
    }

    if (confirm(t('dialogs.confirmations.deleteScreen', { name: screen.name }))) {
      try {
        await screenEditorStore.deleteScreen(screen);
      } catch (error) {
        alert(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function removeField(index) {
    screenEditorStore.removeField(index);
  }

  function getFieldWidthLabel(width) {
    return screenEditorStore.getFieldWidthLabel(width);
  }

  function getFieldDisplayName(field) {
    return screenEditorStore.getFieldDisplayName(field);
  }

  function setFormData(key, value) {
    screenEditorStore.formData[key] = value;
  }

  function setFieldSearchQuery(value) {
    screenEditorStore.fieldSearchQuery = value;
  }

  function buildScreenDropdownItems(screen) {
    const items = [
      {
        id: 'fields',
        type: 'regular',
        icon: Settings,
        title: t('screensPage.fields'),
        hoverClass: 'hover-bg',
        onClick: () => startEditFields(screen)
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(screen)
      }
    ];

    // Don't show delete option for default screen (ID 1)
    if (screen.id !== 1) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: '#dc2626',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteScreen(screen)
      });
    }

    return items;
  }

  // Table column definitions
  const screenColumns = [
    {
      key: 'name',
      label: t('screensPage.screen'),
      slot: 'name'
    },
    {
      key: 'created_at',
      label: t('common.created'),
      render: (screen) => new Date(screen.created_at).toLocaleDateString(),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ];
</script>

{#if !showFieldEditor}
  <PageHeader
    icon={Layout}
    title={t('screens.title')}
    subtitle={t('screens.subtitle')}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('screens', 'add'), guard: () => !showCreateForm }}
      >
        {t('screensPage.addScreen')}
      </Button>
    {/snippet}
  </PageHeader>


    <Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-lg">
      <!-- Modal header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          {editingScreen ? t('screensPage.editScreen') : t('screensPage.createScreen')}
        </h3>
      </div>

      <!-- Modal content -->
      <div class="px-6 py-4">
        <form onsubmit={(e) => { e.preventDefault(); saveScreen(); }}>
          <div class="grid grid-cols-1 gap-6">
            <div>
              <Label for="screen-name" required class="mb-2">{t('screensPage.screenName')}</Label>
              <Input
                id="screen-name"
                value={formData.name}
                oninput={(e) => setFormData('name', e.target.value)}
                placeholder={t('screensPage.screenNamePlaceholder')}
                required
              />
            </div>

            <div>
              <Label for="screen-description" class="mb-2">{t('screensPage.description')}</Label>
              <Textarea
                id="screen-description"
                value={formData.description}
                oninput={(e) => setFormData('description', e.target.value)}
                rows={3}
                placeholder={t('screensPage.optionalDescription')}
              />
            </div>
          </div>
        </form>
      </div>

      <DialogFooter
        onCancel={cancelForm}
        onConfirm={saveScreen}
        confirmLabel={editingScreen ? t('screensPage.updateScreen') : t('screensPage.createScreen')}
        disabled={!formData.name.trim()}
      />
    </Modal>

    <DataTable
      columns={screenColumns}
      data={screens}
      keyField="id"
      emptyMessage={t('screensPage.noScreens')}
      emptyIcon={Circle}
      actionItems={buildScreenDropdownItems}
    >
      <div slot="name" let:item={screen}>
        <div class="font-semibold" style="color: var(--ds-text);">{screen.name}</div>
        {#if screen.description}
          <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{screen.description}</div>
        {/if}
      </div>
    </DataTable>
  {:else}
    <PageHeader
      icon={Settings}
      title={t('screensPage.configureFields')}
      subtitle={t('screensPage.screenSubtitle', { name: editingScreenFields?.name })}
    >
      {#snippet actions()}
        <div class="flex gap-3">
          <Button
            variant="primary"
            onclick={saveScreenFields}
          >
            {t('screensPage.saveFields')}
          </Button>
          <Button
            variant="default"
            onclick={cancelFieldEditor}
          >
            {t('common.cancel')}
          </Button>
        </div>
      {/snippet}
    </PageHeader>

    <!-- Drag and Drop Field Editor -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
      <!-- Available Fields -->
      <div class="rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('screensPage.availableFields')}</h3>
        <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('screensPage.dragFieldsHint')}</p>

        <Input
          placeholder={t('screensPage.searchFields')}
          value={fieldSearchQuery}
          oninput={(e) => setFieldSearchQuery(e.target.value)}
          class="mb-4"
        />

        <div class="space-y-1 min-h-96 max-h-[70vh] overflow-y-auto">
          {#each searchFilteredFields as field, index}
            {#if index === 0 || field.category !== searchFilteredFields[index - 1].category}
              <div class="text-xs font-semibold uppercase tracking-wider mt-4 mb-2 first:mt-0" style="color: var(--ds-text-subtle);">
                {field.category === 'System Fields' ? t('screensPage.systemFields') : field.category === 'Custom Fields' ? t('screensPage.customFields') : field.category}
              </div>
            {/if}

            <div
              data-available-field={JSON.stringify(field)}
              class="group flex items-center gap-3 px-4 py-3 rounded border transition-all duration-200 cursor-grab hover:bg-blue-50 hover:border-blue-300 active:cursor-grabbing"
              style="border-color: var(--ds-border); background-color: var(--ds-background-input); user-select: none; -webkit-user-select: none;"
            >
              <!-- Drag Handle -->
              <div class="flex-shrink-0">
                <svg class="w-4 h-4 group-hover:text-blue-500" style="color: var(--ds-text-subtle);" fill="currentColor" viewBox="0 0 24 24">
                  <circle cx="9" cy="6" r="1.5"/>
                  <circle cx="15" cy="6" r="1.5"/>
                  <circle cx="9" cy="12" r="1.5"/>
                  <circle cx="15" cy="12" r="1.5"/>
                  <circle cx="9" cy="18" r="1.5"/>
                  <circle cx="15" cy="18" r="1.5"/>
                </svg>
              </div>

              <div class="flex-1">
                <div class="font-medium" style="color: var(--ds-text);">{field.name}</div>
                <div class="text-sm" style="color: var(--ds-text-subtle);">
                  {#if field.type === 'system'}
                    {SYSTEM_FIELDS.find(sf => sf.identifier === field.identifier)?.type || field.identifier}
                    • {t('screensPage.system')}
                  {:else}
                    {field.fieldType}
                    • {t('screensPage.custom')}
                  {/if}
                </div>
              </div>
            </div>
          {/each}

          {#if searchFilteredFields.length === 0}
            <div class="text-center py-8">
              <p class="text-sm" style="color: var(--ds-text-subtle);">
                {#if fieldSearchQuery.trim()}
                  {t('screensPage.noFieldsMatch')}
                {:else}
                  {t('screensPage.allFieldsAdded')}
                {/if}
              </p>
            </div>
          {/if}
        </div>
      </div>

      <!-- Screen Fields Configuration -->
      <div class="rounded-xl p-6 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('screensPage.screenFields')} ({screenFields.length})</h3>
        <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('screensPage.dragToReorder')}</p>

        <div
          data-drop-zone
          class="min-h-96 max-h-[70vh] overflow-y-auto border-2 border-dashed rounded p-4 space-y-2" style="border-color: var(--ds-border);"
          class:border-blue-400={draggedField}
          class:bg-blue-50={draggedField}
        >
          {#if screenFields.length === 0}
            <div class="text-center py-12">
              <svg class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-text-disabled);" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
              </svg>
              <p class="text-sm" style="color: var(--ds-text-subtle);">{t('screensPage.dropFieldsHere')}</p>
            </div>
          {:else}
            {#each screenFields as field, index (field.field_identifier)}
              <div
                data-screen-field
                data-field-index={index}
                data-field-id={field.field_identifier}
                class="relative group flex items-center gap-3 px-4 py-3 rounded border hover:shadow-sm transition-all duration-200 h-16"
                style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); user-select: none;"
              >
                <!-- Drop indicator -->
                {#if fieldDragState.get(field.field_identifier)?.closestEdge}
                  <DropIndicator edge={fieldDragState.get(field.field_identifier)?.closestEdge} gap={8} />
                {/if}
                <!-- Drag Handle -->
                <div
                  class="cursor-grab active:cursor-grabbing flex-shrink-0 p-1 rounded hover-bg transition-colors"
                  style="touch-action: none;"
                >
                  <svg class="w-4 h-4 group-hover:text-blue-500" style="color: var(--ds-text-subtle);" fill="currentColor" viewBox="0 0 24 24">
                    <circle cx="9" cy="6" r="1.5"/>
                    <circle cx="15" cy="6" r="1.5"/>
                    <circle cx="9" cy="12" r="1.5"/>
                    <circle cx="15" cy="12" r="1.5"/>
                    <circle cx="9" cy="18" r="1.5"/>
                    <circle cx="15" cy="18" r="1.5"/>
                  </svg>
                </div>

                <div class="flex-1">
                  <div class="font-medium flex items-center gap-2" style="color: var(--ds-text);">
                    {getFieldDisplayName(field)}
                    <span class="text-xs px-1.5 py-0.5 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-background-neutral);">
                      {field.field_type === 'system' ? t('screensPage.system') : t('screensPage.custom')}
                    </span>
                  </div>
                </div>

                <div class="flex items-center gap-3 flex-shrink-0">
                  <Checkbox
                    checked={field.is_required}
                    onchange={() => screenEditorStore.toggleFieldRequired(index)}
                    label={t('screensPage.required')}
                    size="small"
                  />

                  {#if field.field_type === 'system' && (field.field_identifier === 'title' || field.field_identifier === 'status')}
                    <div class="w-9 h-9 flex items-center justify-center flex-shrink-0">
                      <svg class="w-4 h-4" style="color: var(--ds-text-disabled);" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/>
                      </svg>
                    </div>
                  {:else}
                    <button
                      onclick={() => removeField(index)}
                      class="text-red-500 hover:text-red-700 transition-colors p-1 rounded hover:bg-red-50 flex-shrink-0"
                      title={t('screensPage.removeField')}
                    >
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                      </svg>
                    </button>
                  {/if}
                </div>
              </div>
            {/each}
          {/if}
        </div>
      </div>
    </div>

  {/if}
