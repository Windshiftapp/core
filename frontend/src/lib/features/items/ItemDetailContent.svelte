<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { AlertCircle } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import ItemDetailBreadcrumbs from '../items/ItemDetailBreadcrumbs.svelte';
  import ItemDetailHeader from '../items/ItemDetailHeader.svelte';
  import ItemDetailDescription from '../items/ItemDetailDescription.svelte';
  import ItemDetailLinks from './ItemDetailLinks.svelte';
  import ItemDetailTabs from '../items/ItemDetailTabs.svelte';
  import ItemDetailSidebar from '../items/ItemDetailSidebar.svelte';

  // All the props that the content needs
  let {
    loading = false,
    error = null,
    item = null,
    workspace = null,
    parentHierarchy = [],
    currentItemType = null,
    currentHierarchyLevel = null,
    iconMap = {},
    workspaceId = null,
    editingTitle = $bindable(false),
    editTitle = $bindable(''),
    saving = false,
    dropdownItems = [],
    statusOptions = [],
    editingDescription = false,
    editDescription = '',
    itemLinks = [],
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
    editingCustomFields = {},
    editCustomFieldValues = {},
    editingPriority = false,
    editingProject = false,
    editingAssignee = false,
    editingMilestone = false,
    editingIteration = false,
    workspaceScreenFields = [],
    workspaceScreenSystemFields = [],
    customFieldDefinitions = [],
    milestones = [],
    iterations = [],
    priorities = [],
    attachments = [],
    attachmentPagination = null,
    diagrams = [],
    loadingDiagrams = false,
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
  } = $props();

  // Lazy-load DiagramModal with background preload (Excalidraw is ~1.2MB)
  let DiagramModal = $state(null);
  let diagramPromise = $state(null);

  onMount(() => {
    // Preload in background after component mounts
    const preload = () => {
      diagramPromise = import('../../components/DiagramModal.svelte');
      diagramPromise.then(module => {
        DiagramModal = module.default;
      });
    };

    if ('requestIdleCallback' in window) {
      requestIdleCallback(preload);
    } else {
      setTimeout(preload, 1000); // Fallback: preload after 1s
    }
  });

  // Component references
  let diagramListComponent = $state(null);
  let descriptionComponent = $state(null);

  // Diagram modal state
  let showDiagramModal = $state(false);
  let editingDiagram = $state(null);

  // Panel resizing state
  let panelWidth = $state(320);
  let isResizing = $state(false);
  let resizeStartX = $state(0);
  let resizeStartWidth = $state(0);

  function startResize(event) {
    isResizing = true;
    resizeStartX = event.clientX;
    resizeStartWidth = panelWidth;
    event.preventDefault();
  }

  function handleResizeMove(event) {
    const deltaX = resizeStartX - event.clientX;
    const newWidth = Math.max(280, Math.min(600, resizeStartWidth + deltaX));
    panelWidth = newWidth;
    document.documentElement.style.setProperty('--panel-width', `${newWidth}px`);
  }

  function handleResizeUp() {
    isResizing = false;
  }

  useEventListener(() => isResizing ? document : undefined, 'mousemove', handleResizeMove);
  useEventListener(() => isResizing ? document : undefined, 'mouseup', handleResizeUp);

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

  function handleShowLinkModal() {
    onshowLinkModal?.();
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
  async function handleNewDiagram() {
    // Ensure DiagramModal is loaded
    if (!DiagramModal && diagramPromise) {
      DiagramModal = (await diagramPromise).default;
    }
    editingDiagram = null;
    showDiagramModal = true;
  }

  async function handleEditDiagram(diagram) {
    // Ensure DiagramModal is loaded
    if (!DiagramModal && diagramPromise) {
      DiagramModal = (await diagramPromise).default;
    }
    editingDiagram = diagram;
    showDiagramModal = true;
  }

  function handleCloseDiagramModal() {
    showDiagramModal = false;
    editingDiagram = null;
  }

  function handleSaveDiagram() {
    // Refresh the diagram list
    if (diagramListComponent) {
      diagramListComponent.refresh();
    }
    ondiagramSaved?.();
  }

  function handleDeleteDiagram() {
    // Refresh the diagram list
    if (diagramListComponent) {
      diagramListComponent.refresh();
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
  <div class="flex-1 min-h-screen" style="background-color: var(--ds-surface-raised);">
    <div class="flex flex-col min-h-screen">
      <!-- Content -->
      <div class="flex flex-1 relative min-h-screen w-full overflow-hidden">
        <!-- Main Content Area - Flexible width -->
        <div class="flex-1 w-0 min-w-0 px-10 pt-6 pb-6 overflow-y-auto overflow-x-hidden">
          <ItemDetailBreadcrumbs
            {workspace}
            {parentHierarchy}
            {currentItemType}
            {currentHierarchyLevel}
            {item}
            {iconMap}
            {workspaceId}
            onnavigate={handleNavigate}
            onparentChanged={handleParentChanged}
            oncopyKey={handleCopyKey}
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

        <!-- Resizable Right Panel -->
        <div class="flex-shrink-0 relative" style="width: var(--panel-width, 320px); min-width: 280px; max-width: 600px;">
          <!-- Resize Handle -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="absolute left-0 top-0 bottom-0 w-1 cursor-ew-resize transition-colors opacity-0 hover:opacity-100"
            style="background-color: var(--ds-border);"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = '#3b82f6'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-border)'}
            onmousedown={startResize}
          ></div>
          
          <!-- Panel Content -->
          <ItemDetailSidebar
            {item}
            {workspace}
            {statusOptions}
            {editingStatus}
            {editingDueDate}
            {editingCustomFields}
            {editCustomFieldValues}
            {editingPriority}
            {editingProject}
            {editingAssignee}
            {editingMilestone}
            {editingIteration}
            {workspaceScreenFields}
            {workspaceScreenSystemFields}
            {customFieldDefinitions}
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
            onstartEditingPriority={onstartEditingPriority}
            onstartEditingStatus={onstartEditingStatus}
            onstartEditingProject={onstartEditingProject}
            onstartEditingCustomField={(detail) => onstartEditingCustomField?.(detail)}
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
