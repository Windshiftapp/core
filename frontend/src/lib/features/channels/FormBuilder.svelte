<script>
  import { onMount, onDestroy } from 'svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { attachClosestEdge, extractClosestEdge } from '@atlaskit/pragmatic-drag-and-drop-hitbox/closest-edge';
  import {
    IconGripVertical, IconTrash, IconAsterisk, IconSearch, IconPlus, IconDeviceFloppy,
    IconSettings, IconArrowLeft, IconTextSize, IconForms, IconCheckbox, IconSelect, IconAlignBoxLeftTop,
    IconPencil, IconEye, IconExternalLink
  } from '@tabler/icons-svelte-runes';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { formBuilderStore } from '../../stores/formBuilderStore.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Input from '../../components/Input.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Label from '../../components/Label.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import IconSelector from '../../pickers/IconSelector.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import CopyButton from '../../components/CopyButton.svelte';
  import { publicBaseURL } from '../../runtime/contextPath.js';
  import FormFieldPalette from './FormFieldPalette.svelte';
  import FormPreviewModal from './FormPreviewModal.svelte';

  let {
    channelId,
    channelSlug = '',
    channelWorkspaceIds = [],
    channelBrandColor = '#14b8a6',
    onBack = () => {},
    onCreateForm = null,
    onOpenSettings = () => {},
    embedded = true,
  } = $props();

  let saving = $state(false);
  let savingRouting = $state(false);
  let showSettings = $state(false);
  let showPreview = $state(false);
  let setupCleanups = [];
  let expandedFields = $state(new Set());
  let previewCustomFieldDefinitions = $state([]);
  let directFormUrl = $derived(
    formBuilderStore.editingForm && channelSlug
      ? `${publicBaseURL()}/forms/${channelSlug}/${formBuilderStore.editingForm.id}`
      : ''
  );

  // Routing-metadata editing (workspace / item type / identity).
  let availableWorkspaces = $state([]);
  let routingItemTypes = $state([]);
  let configSets = [];

  function toggleFieldExpanded(fieldKey) {
    const next = new Set(expandedFields);
    if (next.has(fieldKey)) next.delete(fieldKey);
    else next.add(fieldKey);
    expandedFields = next;
  }

  onMount(async () => {
    await formBuilderStore.loadForms(channelId);
    try {
      const [allWorkspaces, allConfigSets, customFields] = await Promise.all([
        api.workspaces.getAll(),
        api.configurationSets.getAll(),
        api.customFields.getAll(),
      ]);
      configSets = allConfigSets?.configuration_sets || [];
      previewCustomFieldDefinitions = customFields || [];
      availableWorkspaces = (channelWorkspaceIds && channelWorkspaceIds.length > 0)
        ? allWorkspaces.filter(ws => channelWorkspaceIds.includes(ws.id))
        : allWorkspaces;
    } catch (err) {
      console.error('Failed to load workspaces for routing:', err);
    }
  });

  onMount(() => {
    function warnBeforeUnload(event) {
      if (!formBuilderStore.hasUnsavedChanges) return;
      event.preventDefault();
      event.returnValue = '';
    }
    window.addEventListener('beforeunload', warnBeforeUnload);
    return () => window.removeEventListener('beforeunload', warnBeforeUnload);
  });

  // Reload selectable item types when the routing workspace changes, scoped to
  // that workspace's configuration set (mirrors CreateFormModal). Only clears
  // item_type_id when the current selection isn't valid in the new workspace,
  // so editing an existing form doesn't wipe its item type on initial load.
  $effect(() => {
    const wsId = formBuilderStore.routingMeta.workspace_id;
    if (!wsId) {
      routingItemTypes = [];
      return;
    }
    const ws = availableWorkspaces.find(w => w.id === wsId);
    let configSetId = ws?.configuration_set_id;
    if (!configSetId) {
      const defaultCs = configSets.find(cs => cs.is_default);
      if (defaultCs) configSetId = defaultCs.id;
    }
    const filters = configSetId ? { configuration_set_id: configSetId } : {};
    const requestedWsId = wsId;
    api.itemTypes.getAll(filters).then(types => {
      if (formBuilderStore.routingMeta.workspace_id !== requestedWsId) return;
      routingItemTypes = types;
      const current = formBuilderStore.routingMeta.item_type_id;
      if (current && !types.some(ty => ty.id === current)) {
        formBuilderStore.routingMeta.item_type_id = null;
      }
    }).catch(err => {
      if (formBuilderStore.routingMeta.workspace_id !== requestedWsId) return;
      console.error('Failed to load item types for routing:', err);
      routingItemTypes = [];
    });
  });

  async function handleSaveRouting() {
    const meta = formBuilderStore.routingMeta;
    if (!meta.name.trim()) {
      errorToast(t('forms.formNameRequired', 'Name is required'));
      return;
    }
    if (!meta.workspace_id || !meta.item_type_id) {
      errorToast(t('forms.selectItemType', 'Please select an item type'));
      return;
    }
    try {
      savingRouting = true;
      await formBuilderStore.saveRoutingMetadata();
      successToast(t('common.saved'));
    } catch (err) {
      errorToast(err.message || t('common.error'));
    } finally {
      savingRouting = false;
    }
  }

  onDestroy(() => {
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];
  });

  function setupDragAndDrop() {
    // Clean up previous
    setupCleanups.forEach(fn => fn());
    setupCleanups = [];

    // Setup available fields as draggable
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-available-field]')).forEach(element => {
      const fieldDataStr = element.dataset.availableField;
      if (!fieldDataStr) return;
      const fieldData = JSON.parse(fieldDataStr);
      const cleanup = draggable({
        element,
        getInitialData: () => ({ field: fieldData, type: 'available-field' }),
        onDragStart: () => { element.style.opacity = '0.5'; },
        onDrop: () => { element.style.opacity = ''; }
      });
      setupCleanups.push(cleanup);
    });

    // Setup form fields as draggable + drop targets
    /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-form-field]')).forEach(element => {
      const fieldIndex = parseInt(element.dataset.fieldIndex);
      const fieldId = element.dataset.formField;

      // Make draggable
      const dragCleanup = draggable({
        element,
        getInitialData: () => ({ fieldIndex, type: 'form-field' }),
        onDragStart: () => {
          element.style.opacity = '0.5';
          formBuilderStore.setDraggedField(fieldId);
        },
        onDrop: () => {
          element.style.opacity = '';
          formBuilderStore.clearDraggedField();
        }
      });
      setupCleanups.push(dragCleanup);

      // Make drop target
      const dropCleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => {
          const data = source.data;
          if (data.type === 'form-field' && data.fieldIndex === fieldIndex) return false;
          return data.type === 'available-field' || data.type === 'form-field';
        },
        getData: ({ input, element: el }) => {
          return attachClosestEdge({}, { input, element: el, allowedEdges: ['top', 'bottom'] });
        },
        onDragEnter: ({ self }) => {
          const closestEdge = extractClosestEdge(self.data);
          formBuilderStore.setDragState(fieldId, { closestEdge });
        },
        onDragLeave: () => {
          formBuilderStore.setDragState(fieldId, { closestEdge: null });
        },
        onDrop: ({ self, source }) => {
          const closestEdge = extractClosestEdge(self.data);
          const data = source.data;

          if (data.type === 'available-field') {
            formBuilderStore.addFieldAtPosition(data.field, fieldIndex, closestEdge);
          } else if (data.type === 'form-field') {
            formBuilderStore.reorderField(data.fieldIndex, fieldIndex, closestEdge);
          }

          formBuilderStore.clearDragState();
        }
      });
      setupCleanups.push(dropCleanup);
    });

    // Setup drop zone for empty canvas
    const emptyDropZone = /** @type {HTMLElement | null} */ (document.querySelector('[data-form-drop-zone]'));
    if (emptyDropZone) {
      const dropCleanup = dropTargetForElements({
        element: emptyDropZone,
        canDrop: ({ source }) => source.data.type === 'available-field',
        onDrop: ({ source }) => {
          formBuilderStore.addField(source.data.field);
          formBuilderStore.clearDragState();
        }
      });
      setupCleanups.push(dropCleanup);
    }
  }

  // Re-setup drag and drop when fields change
  $effect(() => {
    if (formBuilderStore.showFieldEditor && formBuilderStore.formFields) {
      // Use microtask to ensure DOM is updated
      queueMicrotask(() => setupDragAndDrop());
    }
  });

  async function handleSave() {
    try {
      saving = true;
      await formBuilderStore.saveFormFields();
      await formBuilderStore.saveFormConfig();
      if (formBuilderStore.hasUnsavedRoutingChanges) {
        await formBuilderStore.saveRoutingMetadata();
      }
      successToast(t('common.saved'));
    } catch (err) {
      errorToast(err.message || t('common.error'));
    } finally {
      saving = false;
    }
  }

  async function handleBack() {
    if (formBuilderStore.showFieldEditor) {
      if (formBuilderStore.hasUnsavedChanges) {
        const discard = await confirm({
          title: 'Discard unsaved form changes?',
          message: 'Your field and per-form setting changes have not been saved.',
          confirmText: 'Discard changes',
          variant: 'danger',
        });
        if (!discard) return;
      }
      formBuilderStore.cancelFieldEditor();
    } else {
      onBack();
    }
  }

  async function handleDeleteForm(form, event) {
    event.stopPropagation();
    const ok = await confirm({
      title: t('forms.deleteForm'),
      message: t('forms.confirmDeleteForm', { name: form.name }),
      confirmText: t('forms.deleteForm'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await formBuilderStore.deleteForm(form.id);
      successToast(t('forms.formDeleted'));
    } catch (err) {
      errorToast(err.message || t('common.error'));
    }
  }

  function getFieldTypeIcon(field) {
    if (field.field_type === 'virtual') {
      const type = field.virtual_field_type;
      if (type === 'textarea') return IconAlignBoxLeftTop;
      if (type === 'select') return IconSelect;
      if (type === 'checkbox') return IconCheckbox;
      return IconTextSize;
    }
    if (field.field_type === 'default') return IconForms;
    return IconTextSize;
  }

  function getFieldTypeBadge(field) {
    if (field.field_type === 'default') return 'Default';
    if (field.field_type === 'custom') return 'Custom';
    if (field.field_type === 'virtual') return field.virtual_field_type || 'Virtual';
    return field.field_type;
  }

  function parseOptionsJson(json) {
    if (!json) return [];
    try {
      const parsed = JSON.parse(json);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }

  function writeOptions(fieldIdx, options) {
    formBuilderStore.updateFieldProperty(fieldIdx, 'virtual_field_options', JSON.stringify(options));
  }

  function addOption(fieldIdx) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    opts.push({ value: '', label: '' });
    writeOptions(fieldIdx, opts);
  }

  function removeOption(fieldIdx, optIdx) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    opts.splice(optIdx, 1);
    writeOptions(fieldIdx, opts);
  }

  function updateOptionLabel(fieldIdx, optIdx, value) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    const prev = opts[optIdx] || { value: '', label: '' };
    // If value was auto-synced from label, keep them in sync
    const autoSync = !prev.value || prev.value === prev.label;
    opts[optIdx] = { value: autoSync ? value : prev.value, label: value };
    writeOptions(fieldIdx, opts);
  }

  function updateOptionValue(fieldIdx, optIdx, value) {
    const opts = parseOptionsJson(formBuilderStore.formFields[fieldIdx].virtual_field_options);
    const prev = opts[optIdx] || { value: '', label: '' };
    opts[optIdx] = { ...prev, value };
    writeOptions(fieldIdx, opts);
  }
</script>

<div class="h-full flex flex-col">
  <!-- Header (shown when embedded or when in field editor) -->
  {#if embedded || formBuilderStore.showFieldEditor}
  <div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-3">
      <Button onclick={handleBack} variant="ghost" size="small" icon={IconArrowLeft} />
      <div>
        <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
          {#if formBuilderStore.showFieldEditor}
            {formBuilderStore.editingForm?.name} - {t('forms.builder.title')}
          {:else}
            {t('forms.title')}
          {/if}
        </h3>
        {#if formBuilderStore.showFieldEditor}
          <p class="text-xs" style="color: {formBuilderStore.hasUnsavedChanges ? 'var(--ds-text-warning)' : 'var(--ds-text-subtle)'};">
            {formBuilderStore.hasUnsavedChanges
              ? 'Unsaved changes'
              : 'All changes saved'}
          </p>
        {/if}
      </div>
    </div>
    {#if formBuilderStore.showFieldEditor}
      <div class="flex items-center gap-2">
        <Button
          onclick={() => showPreview = true}
          variant="default"
          size="small"
          icon={IconEye}
          dataTestid="form-builder-preview-btn"
        >
          {t('common.preview')}
        </Button>
        {#if directFormUrl}
          <CopyButton text={directFormUrl} size="sm" label="Copy link" />
          <Button
            onclick={() => window.open(directFormUrl, '_blank', 'noopener')}
            variant="default"
            size="small"
            icon={IconExternalLink}
            dataTestid="form-builder-open-btn"
          >
            {t('channel.openForm')}
          </Button>
        {:else}
          <Button
            onclick={onOpenSettings}
            variant="default"
            size="small"
            icon={IconExternalLink}
            dataTestid="form-builder-configure-slug-btn"
          >
            Set public URL
          </Button>
        {/if}
        <Button
          onclick={() => showSettings = !showSettings}
          variant="default"
          size="small"
          icon={IconSettings}
          dataTestid="form-builder-settings-btn"
        >
          {t('forms.settings.title')}
        </Button>
        <Button
          onclick={handleSave}
          variant="primary"
          size="small"
          icon={IconDeviceFloppy}
          disabled={saving}
          dataTestid="form-builder-save-btn"
        >
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    {/if}
  </div>
  {/if}

  {#if formBuilderStore.loading}
    <div class="flex-1 flex items-center justify-center">
      <Spinner />
    </div>
  {:else if formBuilderStore.showFieldEditor}
    <!-- Field Editor: Two-panel layout -->
    <div class="flex-1 flex overflow-hidden">
      <!-- Left: Form Canvas -->
      <div class="flex-1 overflow-y-auto p-6">
        {#if showSettings}
          <!-- Per-form Settings -->
          <div class="max-w-xl mx-auto space-y-4">
            <!-- Routing metadata: identity + where submissions are created -->
            <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('forms.routing.title')}</h4>
            <DescriptionText>
              Each response creates a work item in the selected target workspace using the selected item type.
            </DescriptionText>

            <div>
              <Label color="default" required class="mb-2">{t('forms.formName')}</Label>
              <Input id="form-routing-name" bind:value={formBuilderStore.routingMeta.name} placeholder={t('forms.formNamePlaceholder')} />
            </div>

            <div>
              <Label color="default" class="mb-2">{t('forms.formDescription')}</Label>
              <Textarea bind:value={formBuilderStore.routingMeta.description} rows={2} placeholder={t('forms.formDescriptionPlaceholder')} />
            </div>

            <div>
              <IconSelector
                bind:selectedIcon={formBuilderStore.routingMeta.icon}
                bind:selectedColor={formBuilderStore.routingMeta.color}
                label={t('portal.iconAndColor')}
                compact
              />
            </div>

            <div>
              <Label color="default" required class="mb-2">{t('channel.targetWorkspace')}</Label>
              <BasePicker
                id="form-routing-workspace"
                bind:value={formBuilderStore.routingMeta.workspace_id}
                items={availableWorkspaces}
                placeholder={t('channel.selectWorkspace')}
                getValue={(item) => item.id}
                getLabel={(item) => item.name}
                optionTestid={(opt) => `form-routing-ws-${opt.value}`}
              />
            </div>

            <div>
              <Label color="default" required class="mb-2">{t('forms.createsItemType')}</Label>
              <BasePicker
                id="form-routing-itemtype"
                bind:value={formBuilderStore.routingMeta.item_type_id}
                items={routingItemTypes}
                placeholder={t('forms.selectItemType')}
                getValue={(item) => item.id}
                getLabel={(item) => item.name}
                disabled={!formBuilderStore.routingMeta.workspace_id}
                optionTestid={(opt) => `form-routing-it-${opt.value}`}
              />
              {#if !formBuilderStore.routingMeta.workspace_id}
                <DescriptionText>{t('channel.selectWorkspaceFirst')}</DescriptionText>
              {/if}
            </div>

            <Button
              onclick={handleSaveRouting}
              variant="primary"
              size="small"
              icon={IconDeviceFloppy}
              dataTestid="form-routing-save-btn"
              disabled={savingRouting || !formBuilderStore.routingMeta.name.trim() || !formBuilderStore.routingMeta.workspace_id || !formBuilderStore.routingMeta.item_type_id}
            >
              {savingRouting ? t('common.saving') : t('forms.routing.save', 'Save routing')}
            </Button>

            <hr style="border-color: var(--ds-border);" />

            <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('forms.settings.title')}</h4>
            <DescriptionText>
              These settings apply only to this form. Branding and the public URL are channel settings.
            </DescriptionText>

            <Checkbox
              id="form-require-auth"
              bind:checked={formBuilderStore.formConfig.require_auth}
              label={t('forms.settings.requireAuth')}
              hint="Only signed-in Windshift users can submit this form."
            />

            <Checkbox
              id="form-allow-attachments"
              dataTestid="form-allow-attachments"
              bind:checked={formBuilderStore.formConfig.allow_attachments}
              label={t('forms.settings.allowAttachments')}
            />

            <div>
              <Label color="default" class="mb-2">{t('forms.settings.submitButton')}</Label>
              <Input bind:value={formBuilderStore.formConfig.submit_button_text} placeholder="Submit" />
            </div>

            <div>
              <Label color="default" class="mb-2">{t('forms.settings.successMessage')}</Label>
              <Input bind:value={formBuilderStore.formConfig.success_message} placeholder={t('forms.settings.successMessagePlaceholder')} />
              <DescriptionText>Shown after a response is accepted.</DescriptionText>
            </div>

            <div>
              <Label color="default" class="mb-2">{t('forms.settings.redirectUrl')}</Label>
              <Input bind:value={formBuilderStore.formConfig.redirect_url} placeholder="https://example.com/thank-you" />
              <DescriptionText>Optional HTTPS destination opened after a successful response.</DescriptionText>
            </div>

            <Button onclick={() => showSettings = false} variant="default" size="small">
              {t('forms.builder.backToFields')}
            </Button>
          </div>
        {:else}
          <!-- Field Canvas -->
          <div class="max-w-xl mx-auto">
            {#if formBuilderStore.formFields.length === 0}
              <div
                data-form-drop-zone
                class="border-2 border-dashed rounded-lg p-12 text-center"
                style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
              >
                <IconForms class="w-12 h-12 mx-auto mb-3" />
                <p class="text-sm font-medium">{t('forms.builder.dropFieldsHere')}</p>
                <p class="text-xs mt-1">{t('forms.builder.dragFromPalette')}</p>
              </div>
            {:else}
              <div class="space-y-2" data-form-drop-zone>
                {#each formBuilderStore.formFields as field, index}
                  {@const dragState = formBuilderStore.fieldDragState.get(field.field_identifier + '_' + index)}
                  {@const isVirtualSelect = field.field_type === 'virtual' && field.virtual_field_type === 'select'}
                  {@const fieldKey = field.field_identifier + '_' + index}
                  {@const isExpanded = expandedFields.has(fieldKey)}
                  <div>
                  <div
                    data-form-field={field.field_identifier + '_' + index}
                    data-field-index={index}
                    class="group relative flex items-center gap-3 p-3 rounded-lg border transition-colors cursor-grab"
                    style="background-color: var(--ds-surface); border-color: var(--ds-border);"
                  >
                    <!-- Drop edge indicators -->
                    {#if dragState?.closestEdge === 'top'}
                      <div class="absolute top-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]" style="transform: translateY(-50%);"></div>
                    {/if}
                    {#if dragState?.closestEdge === 'bottom'}
                      <div class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--ds-interactive)]" style="transform: translateY(50%);"></div>
                    {/if}

                    <!-- Drag handle -->
                    <IconGripVertical class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-disabled);" />

                    <!-- Field info -->
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="text-sm font-medium truncate" style="color: var(--ds-text);">
                          {field.display_name || field.field_label || field.field_identifier}
                        </span>
                        <span class="px-1.5 py-0.5 text-[10px] rounded font-medium" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {getFieldTypeBadge(field)}
                        </span>
                      </div>
                      {#if field.description}
                        <p class="text-xs mt-0.5 truncate" style="color: var(--ds-text-subtle);">{field.description}</p>
                      {/if}
                    </div>

                    <!-- Edit toggle -->
                    <button
                      onclick={() => toggleFieldExpanded(fieldKey)}
                      data-testid={`form-field-edit-${index}`}
                      class="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors"
                      style="color: {isExpanded ? 'var(--ds-interactive)' : 'var(--ds-text-disabled)'};"
                      title="Edit label and help text"
                    >
                      <IconPencil class="w-4 h-4" />
                      Edit
                    </button>

                    <!-- Required toggle -->
                    <button
                      onclick={() => formBuilderStore.toggleFieldRequired(index)}
                      data-testid={`form-field-required-${index}`}
                      class="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors"
                      style="color: {field.is_required ? 'var(--ds-text-danger)' : 'var(--ds-text-disabled)'};"
                      title={field.is_required ? t('forms.builder.required') : t('forms.builder.optional')}
                    >
                      <IconAsterisk class="w-4 h-4" />
                      {field.is_required ? t('forms.builder.required') : t('forms.builder.optional')}
                    </button>

                    <!-- Remove button -->
                    <button
                      onclick={() => formBuilderStore.removeField(index)}
                      data-testid={`form-field-remove-${index}`}
                      class="flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-[var(--ds-background-danger-hovered)]"
                      style="color: var(--ds-text-danger);"
                      title={t('common.remove')}
                    >
                      <IconTrash class="w-4 h-4" />
                      {t('common.remove')}
                    </button>
                  </div>

                  {#if isExpanded || isVirtualSelect}
                    {@const options = parseOptionsJson(field.virtual_field_options)}
                    <div class="mt-1 ml-7 pl-4 py-2 border-l-2 space-y-3" style="border-color: var(--ds-border);">
                      {#if isExpanded}
                        <div>
                          <Label color="default" class="mb-1">Label</Label>
                          <Input
                            type="text"
                            value={field.display_name ?? ''}
                            oninput={(e) => formBuilderStore.updateFieldProperty(index, 'display_name', e.currentTarget.value)}
                            placeholder={field.field_name || field.field_identifier}
                            size="small"
                          />
                        </div>
                        <div>
                          <Label color="default" class="mb-1">Help text</Label>
                          <Textarea
                            value={field.description ?? ''}
                            oninput={(e) => formBuilderStore.updateFieldProperty(index, 'description', e.currentTarget.value)}
                            rows={2}
                            placeholder="Optional instructions shown below the field"
                            size="small"
                          />
                        </div>
                      {/if}

                      {#if isVirtualSelect}
                      <div class="text-xs font-semibold" style="color: var(--ds-text-subtle);">Options</div>
                      {#each options as opt, optIdx (optIdx)}
                        <div class="flex items-center gap-2">
                          <Input
                            type="text"
                            value={opt.label ?? ''}
                            oninput={(e) => updateOptionLabel(index, optIdx, e.currentTarget.value)}
                            placeholder="Label"
                            class="flex-1"
                            size="small"
                          />
                          <Input
                            type="text"
                            value={opt.value ?? ''}
                            oninput={(e) => updateOptionValue(index, optIdx, e.currentTarget.value)}
                            placeholder="value"
                            class="w-32"
                            size="small"
                          />
                          <button
                            onclick={() => removeOption(index, optIdx)}
                            class="p-1 rounded hover:bg-[var(--ds-background-danger-hovered)]"
                            title={t('common.remove')}
                          >
                            <IconTrash class="w-3.5 h-3.5" style="color: var(--ds-text-danger);" />
                          </button>
                        </div>
                      {/each}
                      <button
                        onclick={() => addOption(index)}
                        class="text-xs font-medium flex items-center gap-1"
                        style="color: var(--ds-interactive);"
                      >
                        <IconPlus class="w-3.5 h-3.5" /> Add option
                      </button>
                      {/if}
                    </div>
                  {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Right: Field Palette -->
      {#if !showSettings}
        <FormFieldPalette />
      {/if}
    </div>
  {:else}
    <!-- Form List -->
    <div class="flex-1 overflow-y-auto p-6">
      {#if formBuilderStore.forms.length === 0}
        <EmptyState
          icon={IconForms}
          title={t('forms.builder.noForms')}
          description={t('forms.builder.addFormHint')}
        >
          {#snippet action()}
            {#if onCreateForm}
              <!-- shortcut-guard-exempt: This is the page's single primary action. -->
              <Button onclick={onCreateForm} variant="primary" size="small" icon={IconPlus} dataTestid="form-create-open">
                {t('forms.createForm')}
              </Button>
            {/if}
          {/snippet}
        </EmptyState>
      {:else}
        <div class="space-y-2 max-w-2xl mx-auto">
          {#if onCreateForm}
            <div class="flex justify-end mb-2">
              <!-- shortcut-guard-exempt: This is the page's single primary action. -->
              <Button onclick={onCreateForm} variant="primary" size="small" icon={IconPlus} dataTestid="form-create-open">
                {t('forms.createForm')}
              </Button>
            </div>
          {/if}
          {#each formBuilderStore.forms as form}
            <div
              role="button"
              tabindex="0"
              data-testid={`form-row-${form.id}`}
              onclick={() => formBuilderStore.startEditFields(form)}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); formBuilderStore.startEditFields(form); } }}
              class="group w-full flex items-center gap-4 p-4 rounded-lg border transition-colors hover:bg-[var(--ds-background-neutral-hovered)] cursor-pointer"
              style="border-color: var(--ds-border);"
            >
              <div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background-color: {form.color}20;">
                <IconForms class="w-5 h-5" style="color: {form.color};" />
              </div>
              <div class="flex-1 text-left">
                <div class="text-sm font-medium" style="color: var(--ds-text);">{form.name}</div>
                {#if form.description}
                  <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{form.description}</div>
                {/if}
              </div>
              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('forms.builder.editFields')}</span>
              <Button
                onclick={(e) => handleDeleteForm(form, e)}
                variant="ghost"
                size="small"
                icon={IconTrash}
                title={t('forms.deleteForm')}
                class="opacity-0 group-hover:opacity-100 !text-[var(--ds-text-danger)]"
              />
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<FormPreviewModal
  bind:isOpen={showPreview}
  form={formBuilderStore.editingForm}
  fields={formBuilderStore.formFields}
  customFieldDefinitions={previewCustomFieldDefinitions}
  formConfig={formBuilderStore.formConfig}
  brandColor={channelBrandColor}
  onClose={() => showPreview = false}
/>
