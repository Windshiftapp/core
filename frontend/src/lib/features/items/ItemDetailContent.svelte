<script>
  import { AlertCircle } from '@lucide/svelte';
  import { api } from '../../api.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import ItemDetailBreadcrumbs from '../items/ItemDetailBreadcrumbs.svelte';
  import ItemDetailHeader from '../items/ItemDetailHeader.svelte';
  import ItemDetailDescription from '../items/ItemDetailDescription.svelte';
  import ItemDetailLinks from './ItemDetailLinks.svelte';
  import ItemDetailTabs from '../items/ItemDetailTabs.svelte';
  import ItemDetailSidebar from '../items/ItemDetailSidebar.svelte';
  import SidebarResizeHandle from '../../layout/SidebarResizeHandle.svelte';

  // All the props that the content needs
  let {
    loading = false,
    error = null,
    item = null,
    workspace = null,
    parentHierarchy = [],
    currentItemType = null,
    currentHierarchyLevel = null,
    workspaceId = null,
    editingTitle = $bindable(false),
    editTitle = $bindable(''),
    saving = false,
    dropdownItems = [],
    statusOptions = [],
    pendingApproval = null,
    editingDescription = false,
    editDescription = '',
    itemLinks = [],
    linkTypes = [],
    loadingLinks = false,
    availableSubIssueTypes = [],
    childItems = [],
    loadingChildItems = false,
    itemTypes = [],
    tab = 'comments',
    moduleSettings = {},
    isModal = false,
    timeWorklogs = [],
    showTimeEntry = false,
    timeFormData = {},
    savingTimeEntry = false,
    timeProjects = [],
    activeTimer = null,
    editingStatus = false,
    editingDueDate = false,
    editingStartDate = false,
    editingEndDate = false,
    editingCustomFields = {},
    editCustomFieldValues = {},
    editingPriority = false,
    editingProject = false,
    editingAssignee = false,
    editingMilestone = false,
    editingIteration = false,
    workspaceScreenFields = [],
    workspaceScreenSystemFields = [],
    editableScreenFieldIds = null,
    editableScreenSystemFields = null,
    customFieldDefinitions = [],
    requestTypeFields = [],
    milestones = [],
    iterations = [],
    priorities = [],
    attachments = [],
    attachmentPagination = null,
    diagrams = [],
    manualActions = [],
    // Callback props
    onnavigate = null,
    ongoBack = null,
    oncopyKey = null,
    onsaveField = null,
    oncancelEdit = null,
    onswitchTab = null,
    oncreateSubIssue = null,
    onremoveLink = null,
    onviewTestCase = null,
    onshowLinkModal = null,
    onstartEditingAssignee = null,
    onstartEditingMilestone = null,
    onstartEditingIteration = null,
    onstartEditingPriority = null,
    onstartEditingDueDate = null,
    onstartEditingStartDate = null,
    onstartEditingEndDate = null,
    onstartEditingStatus = null,
    onstartEditingProject = null,
    onstartEditingDescription = null,
    onstartEditingCustomField = null,
    onstartTimer = null,
    onlogTime = null,
    oneditWorklog = null,
    ondeleteWorklog = null,
    onparentChanged = null,
    onattachmentUpload = null,
    onattachmentUploadFiles = null,
    onattachmentDelete = null,
    onattachmentPageChange = null,
    onattachmentPageSizeChange = null,
    ondiagramSaved = null,
    onexecuteAction = null,
    onaiAction = null,
    onreorderChildren = null,
    canCreate = false,
    onclose = null,
    recurrenceRule = null,
    onsetupRecurrence = null,
    oneditRecurrence = null,
    onapprovalsChanged = null,
    onitemtypechange = null,
  } = $props();

  // Keep the editor bundle off the network until the user opens it.
  let DiagramModal = $state(null);
  let diagramPromise = $state(null);

  // Component references
  let descriptionComponent = $state(null);

  // Diagram modal state
  let showDiagramModal = $state(false);
  let editingDiagram = $state(null);

  // Panel resizing state
  let panelWidth = $state(320);
  function updatePanelWidth(width) {
    panelWidth = width;
    document.documentElement.style.setProperty('--panel-width', `${width}px`);
  }

  function handleNavigate(path) {
    onnavigate?.({ path });
  }

  function handleGoBack() {
    ongoBack?.();
  }

  function handleCopyKey() {
    oncopyKey?.();
  }

  function handleSaveField(data) {
    onsaveField?.(data);
  }

  function handleCancelEdit(data) {
    oncancelEdit?.(data);
  }

  function handleSwitchTab(data) {
    onswitchTab?.(data);
  }

  function handleCreateSubIssue() {
    oncreateSubIssue?.();
  }

  function handleRemoveLink(data) {
    onremoveLink?.(data);
  }

  function handleViewTestCase(data) {
    onviewTestCase?.(data);
  }

  function handleShowLinkModal(data) {
    onshowLinkModal?.(data);
  }

  function handleStartEditingAssignee() {
    onstartEditingAssignee?.();
  }

  function handleStartEditingMilestone() {
    onstartEditingMilestone?.();
  }

  function handleStartEditingIteration() {
    onstartEditingIteration?.();
  }

  function handleStartEditingDueDate() {
    onstartEditingDueDate?.();
  }

  function handleStartEditingStartDate() {
    onstartEditingStartDate?.();
  }

  function handleStartEditingEndDate() {
    onstartEditingEndDate?.();
  }

  function handleStartEditingPriority() {
    onstartEditingPriority?.();
  }

  function handleStartEditingStatus() {
    onstartEditingStatus?.();
  }

  function handleStartEditingProject() {
    onstartEditingProject?.();
  }

  function handleStartTimer() {
    onstartTimer?.();
  }

  function handleLogTime() {
    onlogTime?.();
  }

  function handleEditWorklog(data) {
    oneditWorklog?.(data);
  }

  function handleDeleteWorklog(data) {
    ondeleteWorklog?.(data);
  }

  function handleParentChanged() {
    onparentChanged?.();
  }

  // Handle image uploaded via editor drag/paste
  function handleImageUploaded(data) {
    // Refresh attachments list
    onattachmentUpload?.(data);
  }

  // Handle insert image from attachment list
  function handleInsertImage(event) {
    if (descriptionComponent) {
      descriptionComponent.insertImage(event.detail);
    }
  }

  // Diagram handlers
  async function ensureDiagramModal() {
    diagramPromise ??= import('../../components/DiagramModal.svelte');
    DiagramModal ??= (await diagramPromise).default;
  }

  async function handleNewDiagram() {
    await ensureDiagramModal();
    editingDiagram = null;
    showDiagramModal = true;
  }

  async function handleEditDiagram(diagram) {
    await ensureDiagramModal();
    editingDiagram = diagram;
    showDiagramModal = true;
  }

  function handleCloseDiagramModal() {
    showDiagramModal = false;
    editingDiagram = null;
  }

  function handleSaveDiagram() {
    ondiagramSaved?.();
  }

  async function handleDeleteDiagram(diagram) {
    if (!diagram?.id) return;
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('components.diagram.confirmDelete'),
      confirmText: t('common.delete'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await api.deleteDiagram(diagram.id);
      await ondiagramSaved?.();
      successToast(t('editors.diagramDeleted'));
    } catch (err) {
      console.error('Failed to delete diagram:', err);
      errorToast(t('components.diagram.deleteError'));
    }
  }

  function handleExecuteAction(data) {
    onexecuteAction?.(data);
  }

  function handleReorderChildren() {
    onreorderChildren?.();
  }

  function handleAIAction(data) {
    onaiAction?.(data);
  }
</script>

{#if loading}
  <!-- Loading State -->
  <div class="p-8" style="background-color: var(--ds-surface);">
    <div class="animate-pulse space-y-4">
      <div class="flex items-center justify-between">
        <div class="h-6 rounded w-1/4" style="background-color: var(--ds-background-neutral);"></div>
        <div class="h-8 w-8 rounded" style="background-color: var(--ds-background-neutral);"></div>
      </div>
      <div class="h-8 rounded w-1/2" style="background-color: var(--ds-background-neutral);"></div>
      <div class="h-4 rounded w-3/4" style="background-color: var(--ds-background-neutral);"></div>
      <div class="space-y-2">
        <div class="h-4 rounded" style="background-color: var(--ds-background-neutral);"></div>
        <div class="h-4 rounded w-5/6" style="background-color: var(--ds-background-neutral);"></div>
      </div>
    </div>
  </div>
{:else if error}
  <!-- Error State -->
  <div class="p-8 text-center" style="background-color: var(--ds-surface);">
    <AlertCircle class="w-12 h-12 text-red-500 mx-auto mb-4" />
    <h1 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">{t('items.errorLoadingWorkItem')}</h1>
    <p class="mb-6" style="color: var(--ds-text-subtle);">{error}</p>
    <button
      onclick={() => onclose?.()}
      class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
    >
      {t('common.close')}
    </button>
  </div>
{:else if item && workspace}
  <!-- Main Content -->
  <div data-testid="item-detail-ready" class="flex-1 min-h-0 {isModal ? '' : 'min-h-screen'}" style="background-color: var(--ds-surface-raised);">
    <div class="flex flex-col {isModal ? 'h-full' : 'min-h-screen'}">
      <!-- Content -->
      <div class="flex flex-1 relative {isModal ? 'h-full' : 'min-h-screen'} w-full overflow-hidden">
        <!-- Main Content Area - Flexible width -->
        <div class="flex-1 w-0 min-w-0 pt-6 pb-6 overflow-y-auto overflow-x-hidden">
          <div class="{isModal ? '' : 'max-w-5xl mx-auto'} px-10">
          <ItemDetailBreadcrumbs
            {workspace}
            {parentHierarchy}
            {currentItemType}
            {currentHierarchyLevel}
            {item}
            {workspaceId}
            onnavigate={handleNavigate}
            onparentChanged={handleParentChanged}
            oncopyKey={handleCopyKey}
            {itemTypes}
            onitemtypechange={(type) => onitemtypechange?.(type)}
          />
          
          <ItemDetailHeader
            {item}
            {workspace}
            bind:editingTitle
            bind:editTitle
            {saving}
            onsavefield={handleSaveField}
            oncanceledit={handleCancelEdit}
          />
          <ItemDetailDescription
            bind:this={descriptionComponent}
            {item}
            {editingDescription}
            {editDescription}
            {saving}
            {availableSubIssueTypes}
            {attachments}
            {diagrams}
            {manualActions}
            {canCreate}
            onsavefield={handleSaveField}
            oncanceledit={handleCancelEdit}
            onstartEditingDescription={() => onstartEditingDescription?.()}
            onshowAddLink={handleShowLinkModal}
            oncreateSubIssue={handleCreateSubIssue}
            onimageuploaded={handleImageUploaded}
            onattachmentUpload={(data) => onattachmentUpload?.(data)}
            onattachmentUploadFiles={(data) => onattachmentUploadFiles?.(data)}
            onattachmentDelete={(data) => onattachmentDelete?.(data)}
            onnewDiagram={handleNewDiagram}
            oneditDiagram={handleEditDiagram}
            ondeleteDiagram={handleDeleteDiagram}
            onexecuteAction={handleExecuteAction}
            onaiaction={handleAIAction}
          />

          <ItemDetailLinks
            {item}
            {workspace}
            {workspaceId}
            itemId={item.id}
            {isModal}
            {itemLinks}
            {linkTypes}
            {loadingLinks}
            {availableSubIssueTypes}
            {childItems}
            {loadingChildItems}
            {itemTypes}
            isLowestLevel={availableSubIssueTypes.length === 0}
            onnavigate={handleNavigate}
            oncreatesubissue={handleCreateSubIssue}
            onremovelink={handleRemoveLink}
            onviewtestcase={handleViewTestCase}
            onshowlinkmodal={handleShowLinkModal}
            onreorderchildren={handleReorderChildren}
          />

          <br />
          <ItemDetailTabs
            {item}
            {workspace}
            {tab}
            {moduleSettings}
            {timeWorklogs}
            {activeTimer}
            {statusOptions}
            onswitchtab={handleSwitchTab}
            onstarttimer={handleStartTimer}
            onlogtime={handleLogTime}
            oneditworklog={handleEditWorklog}
            ondeleteworklog={handleDeleteWorklog}
          />
          </div>
        </div>

        <!-- Resizable Right Panel -->
        <div class="flex-shrink-0 relative {isModal ? 'h-full' : ''}" style="width: var(--panel-width, 320px); min-width: 280px; max-width: 600px;">
          <SidebarResizeHandle
            width={panelWidth}
            minWidth={280}
            maxWidth={600}
            defaultWidth={320}
            edge="left"
            label="Resize item details"
            title="Drag to resize, double-click to reset"
            onresize={updatePanelWidth}
          />
          
          <!-- Panel Content -->
          <ItemDetailSidebar
            {item}
            {workspace}
            {statusOptions}
            {pendingApproval}
            {onapprovalsChanged}
            {editingStatus}
            {editingDueDate}
            {editingStartDate}
            {editingEndDate}
            {editingCustomFields}
            {editCustomFieldValues}
            {editingPriority}
            {editingProject}
            {editingAssignee}
            {editingMilestone}
            {editingIteration}
            {workspaceScreenFields}
            {workspaceScreenSystemFields}
            {editableScreenFieldIds}
            {editableScreenSystemFields}
            {customFieldDefinitions}
            {requestTypeFields}
            {milestones}
            {iterations}
            {priorities}
            {timeProjects}
            {moduleSettings}
            {dropdownItems}
            onsaveField={onsaveField}
            oncancelEdit={oncancelEdit}
            onstartEditingAssignee={onstartEditingAssignee}
            onstartEditingMilestone={onstartEditingMilestone}
            onstartEditingIteration={onstartEditingIteration}
            onstartEditingDueDate={onstartEditingDueDate}
            onstartEditingStartDate={onstartEditingStartDate}
            onstartEditingEndDate={onstartEditingEndDate}
            onstartEditingPriority={onstartEditingPriority}
            onstartEditingStatus={onstartEditingStatus}
            onstartEditingProject={onstartEditingProject}
            onstartEditingCustomField={(detail) => onstartEditingCustomField?.(detail)}
            {recurrenceRule}
            onsetupRecurrence={onsetupRecurrence}
            oneditRecurrence={oneditRecurrence}
          />
        </div>
      </div>
    </div>
  </div>
{:else}
  <!-- Not Found State -->
  <div class="p-8 text-center" style="background-color: var(--ds-surface);">
    <h1 class="text-xl font-semibold mb-4" style="color: var(--ds-text);">{t('items.workItemNotFound')}</h1>
    <button
      onclick={() => onclose?.()}
      class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
    >
      {t('common.close')}
    </button>
  </div>
{/if}

<!-- Diagram Modal (lazy-loaded) -->
{#if showDiagramModal && item && DiagramModal}
  <DiagramModal
    itemId={item.id}
    diagram={editingDiagram}
    onClose={handleCloseDiagramModal}
    onSave={handleSaveDiagram}
  />
{/if}
