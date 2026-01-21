<script>
  import { createEventDispatcher } from 'svelte';
  import { AlertCircle, Workflow } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import ItemDetailBreadcrumbs from '../items/ItemDetailBreadcrumbs.svelte';
  import ItemDetailHeader from '../items/ItemDetailHeader.svelte';
  import ItemDetailDescription from '../items/ItemDetailDescription.svelte';
  import ItemDetailLinks from './ItemDetailLinks.svelte';
  import ItemDetailTabs from '../items/ItemDetailTabs.svelte';
  import ItemDetailSidebar from '../items/ItemDetailSidebar.svelte';
  import DiagramList from '../../components/DiagramList.svelte';
  import DiagramModal from '../../components/DiagramModal.svelte';
  import Button from '../../components/Button.svelte';

  const dispatch = createEventDispatcher();

  // All the props that the content needs
  export let loading = false;
  export let error = null;
  export let item = null;
  export let workspace = null;
  export let parentHierarchy = [];
  export let currentItemType = null;
  export let currentHierarchyLevel = null;
  export let iconMap = {};
  export let workspaceId = null;
  export let editingTitle = false;
  export let editTitle = '';
  export let saving = false;
  export let dropdownItems = [];
  export let statusOptions = [];
  export let editingDescription = false;
  export let editDescription = '';
  export let itemLinks = [];
  export let loadingLinks = false;
  export let availableSubIssueTypes = [];
  export let childItems = [];
  export let loadingChildItems = false;
  export let showAddLinkForm = false;
  export let addLinkData = {};
  export let linkTypes = [];
  export let searchResults = [];
  export let searchQuery = '';
  export let searching = false;
  export let itemTypes = [];
  export let tab = 'comments';
  export let moduleSettings = {};
  export let isModal = false;
  export let timeWorklogs = [];
  export let showTimeEntry = false;
  export let timeFormData = {};
  export let savingTimeEntry = false;
  export let timeProjects = [];
  export let activeTimer = null;
  export let editingStatus = false;
  export let editingPriority = false;
  export let editingDueDate = false;
  export let editingProject = false;
  export let editingAssignee = false;
  export let editingMilestone = false;
  export let editingIteration = false;
  export let editingCustomFields = {};
  export let editCustomFieldValues = {};
  export let workspaceScreenFields = [];
  export let workspaceScreenSystemFields = [];
  export let customFieldDefinitions = [];
  export let milestones = [];
  export let iterations = [];
  export let priorities = [];
  export let attachments = [];
  export let attachmentPagination = null;
  export let attachmentSettings = null;
  export let diagrams = [];
  export let loadingDiagrams = false;
  export let manualActions = [];

  // Component references
  let diagramListComponent;
  let descriptionComponent;

  // Diagram modal state
  let showDiagramModal = false;
  let editingDiagram = null;

  // Panel resizing state
  let panelWidth = 320; // Default width
  let isResizing = false;
  
  function startResize(event) {
    isResizing = true;
    const startX = event.clientX;
    const startWidth = panelWidth;
    
    function handleMouseMove(event) {
      if (!isResizing) return;
      
      const deltaX = startX - event.clientX;
      const newWidth = Math.max(280, Math.min(600, startWidth + deltaX));
      panelWidth = newWidth;
      
      // Update CSS custom property for panel width
      document.documentElement.style.setProperty('--panel-width', `${newWidth}px`);
    }
    
    function handleMouseUp() {
      isResizing = false;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    }
    
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    event.preventDefault();
  }
  
  // Forward all events
  function handleNavigate(event) {
    dispatch('navigate', event.detail);
  }
  
  function handleGoBack() {
    dispatch('go-back');
  }
  
  function handleCopyKey() {
    dispatch('copy-key');
  }
  
  function handleSaveField(event) {
    dispatch('save-field', event.detail);
  }
  
  function handleCancelEdit(event) {
    dispatch('cancel-edit', event.detail);
  }
  
  function handleSwitchTab(event) {
    dispatch('switch-tab', event.detail);
  }
  
  function handleCreateSubIssue() {
    dispatch('create-sub-issue');
  }


  function handleCancelAddLink() {
    dispatch('cancel-add-link');
  }

  function handleAddLink() {
    dispatch('add-link');
  }

  function handleSelectItem(event) {
    dispatch('select-item', event.detail);
  }

  function handleRemoveLink(event) {
    dispatch('remove-link', event.detail);
  }

  function handleViewTestCase(event) {
    dispatch('view-test-case', event.detail);
  }

  function handleStartEditingAssignee() {
    dispatch('start-editing-assignee');
  }

  function handleStartEditingMilestone() {
    dispatch('start-editing-milestone');
  }

  function handleStartEditingIteration() {
    dispatch('start-editing-iteration');
  }

  function handleStartEditingDueDate() {
    dispatch('start-editing-due-date');
  }

  function handleStartEditingPriority() {
    dispatch('start-editing-priority');
  }

  function handleStartEditingStatus() {
    dispatch('start-editing-status');
  }

  function handleStartEditingProject() {
    dispatch('start-editing-project');
  }

  function handleStartTimer() {
    dispatch('start-timer');
  }

  function handleLogTime() {
    dispatch('log-time');
  }

  function handleEditWorklog(event) {
    dispatch('edit-worklog', event.detail);
  }

  function handleDeleteWorklog(event) {
    dispatch('delete-worklog', event.detail);
  }

  function handleParentChanged() {
    dispatch('parent-changed');
  }

  // Handle image uploaded via editor drag/paste
  function handleImageUploaded(event) {
    // Refresh attachments list
    dispatch('attachment-upload', event.detail);
  }

  // Handle insert image from attachment list
  function handleInsertImage(event) {
    if (descriptionComponent) {
      descriptionComponent.insertImage(event.detail);
    }
  }

  // Diagram handlers
  function handleNewDiagram() {
    editingDiagram = null;
    showDiagramModal = true;
  }

  function handleEditDiagram(diagram) {
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
    dispatch('diagram-saved');
  }

  function handleDeleteDiagram() {
    // Refresh the diagram list
    if (diagramListComponent) {
      diagramListComponent.refresh();
    }
  }

  function handleExecuteAction(event) {
    dispatch('execute-action', event.detail);
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
      onclick={() => dispatch('close')}
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
            on:navigate={handleNavigate}
            on:parent-changed={handleParentChanged}
            on:copy-key={handleCopyKey}
          />
          
          <ItemDetailHeader 
            {item}
            {workspace}
            bind:editingTitle
            bind:editTitle
            {saving}
            on:save-field={handleSaveField}
            on:cancel-edit={handleCancelEdit}
            on:copy-key={handleCopyKey}
            on:go-back={handleGoBack}
          />
          <ItemDetailDescription
            bind:this={descriptionComponent}
            {item}
            bind:editingDescription
            bind:editDescription
            {saving}
            {availableSubIssueTypes}
            {attachments}
            {diagrams}
            {attachmentSettings}
            {manualActions}
            on:save-field={handleSaveField}
            on:cancel-edit={handleCancelEdit}
            on:show-add-link={() => showAddLinkForm = true}
            on:create-sub-issue={handleCreateSubIssue}
            on:image-uploaded={handleImageUploaded}
            on:attachment-upload={(e) => dispatch('attachment-upload', e.detail)}
            on:attachment-upload-files={(e) => dispatch('attachment-upload-files', e.detail)}
            on:attachment-delete={(e) => dispatch('attachment-delete', e.detail)}
            on:new-diagram={handleNewDiagram}
            on:edit-diagram={(e) => handleEditDiagram(e.detail)}
            on:delete-diagram={(e) => handleDeleteDiagram(e.detail)}
            on:execute-action={handleExecuteAction}
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
            bind:showAddLinkForm
            bind:addLinkData
            {linkTypes}
            bind:searchResults
            bind:searchQuery
            {searching}
            {itemTypes}
            on:navigate={handleNavigate}
            on:create-sub-issue={handleCreateSubIssue}
            on:cancel-add-link={handleCancelAddLink}
            on:add-link={handleAddLink}
            on:select-item={handleSelectItem}
            on:remove-link={handleRemoveLink}
            on:view-test-case={handleViewTestCase}
          />

          <ItemDetailTabs
            {item}
            {workspace}
            {tab}
            {moduleSettings}
            {timeWorklogs}
            {showTimeEntry}
            {timeFormData}
            {savingTimeEntry}
            {timeProjects}
            {activeTimer}
            {statusOptions}
            on:switch-tab={handleSwitchTab}
            on:start-timer={handleStartTimer}
            on:log-time={handleLogTime}
            on:edit-worklog={handleEditWorklog}
            on:delete-worklog={handleDeleteWorklog}
          />
        </div>

        <!-- Resizable Right Panel -->
        <div class="flex-shrink-0 relative" style="width: var(--panel-width, 320px); min-width: 280px; max-width: 600px;">
          <!-- Resize Handle -->
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
            {editingPriority}
            {editingDueDate}
            {editingProject}
            {editingAssignee}
            {editingMilestone}
            {editingIteration}
            {editingCustomFields}
            {editCustomFieldValues}
            {workspaceScreenFields}
            {workspaceScreenSystemFields}
            {customFieldDefinitions}
            {milestones}
            {iterations}
            {priorities}
            {timeProjects}
            {moduleSettings}
            {saving}
            {dropdownItems}
            on:save-field={handleSaveField}
            on:cancel-edit={handleCancelEdit}
            on:start-editing-assignee={handleStartEditingAssignee}
            on:start-editing-milestone={handleStartEditingMilestone}
            on:start-editing-iteration={handleStartEditingIteration}
            on:start-editing-due-date={handleStartEditingDueDate}
            on:start-editing-priority={handleStartEditingPriority}
            on:start-editing-status={handleStartEditingStatus}
            on:start-editing-project={handleStartEditingProject}
            on:start-editing-custom-field={(e) => dispatch('start-editing-custom-field', e.detail)}
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
      onclick={() => dispatch('close')}
      class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
    >
      {t('common.close')}
    </button>
  </div>
{/if}

<!-- Diagram Modal -->
{#if showDiagramModal && item}
  <DiagramModal
    itemId={item.id}
    diagram={editingDiagram}
    onClose={handleCloseDiagramModal}
    onSave={handleSaveDiagram}
  />
{/if}
