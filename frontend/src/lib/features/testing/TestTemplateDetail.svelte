<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { ArrowLeft, Play, Edit2, Trash2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';

  let template = null;
  let executions = [];
  let testSet = null;
  let loading = true;
  let editMode = false;
  let editName = '';
  let editDescription = '';

  // Dialog state
  let showConfirmDialog = false;
  let confirmMessage = '';
  let confirmTitle = '';
  let confirmAction = null;

  $: workspaceId = $currentRoute.params.id;
  $: templateId = $currentRoute.params.templateId;

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  onMount(async () => {
    if (templateId) {
      await loadTemplate(templateId);
    }
  });

  async function loadTemplate(templateId) {
    try {
      loading = true;
      template = await api.tests.testRunTemplates.get(workspaceId, templateId);
      executions = await api.tests.testRunTemplates.getExecutions(workspaceId, templateId);

      if (template.set_id) {
        testSet = await api.tests.testSets.get(workspaceId, template.set_id);
      }
    } catch (error) {
      console.error('Failed to load template:', error);
    } finally {
      loading = false;
    }
  }

  function goBack() {
    navigate(testPath('/templates'));
  }

  function toggleEditMode() {
    if (!editMode) {
      editName = template.name;
      editDescription = template.description || '';
      editMode = true;
    } else {
      editMode = false;
    }
  }

  async function saveEdit() {
    if (!editName.trim()) {
      confirmTitle = 'Validation Error';
      confirmMessage = 'Template name cannot be empty';
      confirmAction = null;
      showConfirmDialog = true;
      return;
    }

    try {
      await api.tests.testRunTemplates.update(workspaceId, templateId, {
        set_id: template.set_id,
        name: editName,
        description: editDescription
      });

      template.name = editName;
      template.description = editDescription;
      editMode = false;
    } catch (error) {
      console.error('Failed to update template:', error);
      confirmTitle = 'Error';
      confirmMessage = 'Failed to update template. Please try again.';
      confirmAction = null;
      showConfirmDialog = true;
    }
  }

  async function deleteTemplate() {
    confirmTitle = 'Delete Template';
    confirmMessage = `Are you sure you want to delete "${template.name}"? This will not delete existing test runs created from this template.`;
    confirmAction = async () => {
      try {
        await api.tests.testRunTemplates.delete(workspaceId, templateId);
        navigate(testPath('/templates'));
      } catch (error) {
        console.error('Failed to delete template:', error);
        confirmTitle = 'Error';
        confirmMessage = 'Failed to delete template. Please try again.';
        confirmAction = null;
        showConfirmDialog = true;
      }
    };
    showConfirmDialog = true;
  }

  async function executeTemplate() {
    try {
      const newRun = await api.tests.testRunTemplates.execute(workspaceId, templateId);
      // Navigate to the execution page
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to execute template:', error);
      confirmTitle = 'Error';
      confirmMessage = 'Failed to start execution. Please try again.';
      confirmAction = null;
      showConfirmDialog = true;
    }
  }

  function viewRunDetails(run) {
    navigate(testPath(`/runs/${run.id}`));
  }

  function continueExecution(execution) {
    navigate(testPath(`/runs/${execution.id}/execute`));
  }

  function getRunStatus(run) {
    if (run.ended_at) {
      return { text: 'Completed', color: 'bg-green-100 text-green-800' };
    }
    return { text: 'In Progress', color: 'bg-blue-100 text-blue-800' };
  }

  // Keyboard shortcuts
  function handleEditKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      saveEdit();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      toggleEditMode();
    }
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if template}
      <!-- Header -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-3">
          <button
            onclick={goBack}
            class="p-2 hover:bg-gray-100 rounded cursor-pointer"
          >
            <ArrowLeft class="w-5 h-5" />
          </button>
          <div class="flex-1">
            {#if editMode}
              <input
                type="text"
                bind:value={editName}
                onkeydown={handleEditKeydown}
                class="text-2xl font-semibold px-2 py-1 border rounded w-full"
                style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                autofocus
              />
            {:else}
              <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
                {template.name}
              </h1>
            {/if}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              Created: {new Date(template.created_at).toLocaleString()}
              {#if template.updated_at && template.updated_at !== template.created_at}
                • Updated: {new Date(template.updated_at).toLocaleString()}
              {/if}
            </div>
          </div>
        </div>

        <div class="flex items-center gap-3">
          {#if editMode}
            <button
              onclick={saveEdit}
              class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition cursor-pointer"
            >
              Save
            </button>
            <button
              onclick={toggleEditMode}
              class="px-4 py-2 border rounded hover:bg-gray-50 transition cursor-pointer"
              style="border-color: var(--ds-border); color: var(--ds-text);"
            >
              Cancel
            </button>
          {:else}
            <button
              onclick={toggleEditMode}
              class="flex items-center gap-2 px-4 py-2 border rounded hover:bg-gray-50 transition cursor-pointer"
              style="border-color: var(--ds-border); color: var(--ds-text);"
            >
              <Edit2 class="w-4 h-4" />
              Edit
            </button>
            <button
              onclick={deleteTemplate}
              class="flex items-center gap-2 px-4 py-2 border border-red-300 text-red-600 rounded hover:bg-red-50 transition cursor-pointer"
            >
              <Trash2 class="w-4 h-4" />
              Delete
            </button>
            <Button
              variant="primary"
              onclick={executeTemplate}
              icon={Play}
              size="medium"
            >
              Execute Template
            </Button>
          {/if}
        </div>
      </div>

      <!-- Template Details -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Main Content -->
        <div class="lg:col-span-2 space-y-6">
          <!-- Template Information -->
          <div class="p-6" style="background-color: var(--ds-surface-raised);">
            <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Template Information</h2>

            <div class="space-y-4">
              <div>
                <div class="text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">Test Plan</div>
                {#if testSet}
                  <a href={`/workspaces/${workspaceId}/tests/sets/${testSet.id}`} class="text-blue-600 hover:text-blue-800 hover:underline">
                    {testSet.name}
                  </a>
                {:else}
                  <div style="color: var(--ds-text);">Loading...</div>
                {/if}
              </div>

              <div>
                <div class="text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">Description</div>
                {#if editMode}
                  <Textarea
                    bind:value={editDescription}
                    onkeydown={handleEditKeydown}
                    rows={4}
                    placeholder="Add a description for this template..."
                  />
                {:else}
                  <div class="text-sm" style="color: var(--ds-text);">
                    {template.description || 'No description provided'}
                  </div>
                {/if}
              </div>
            </div>
          </div>

          <!-- Executions List -->
          <div class="p-6" style="background-color: var(--ds-surface-raised);">
            <SectionHeader title="Executions ({executions.length})">
              {#snippet actions()}
                <Button
                  variant="primary"
                  onclick={executeTemplate}
                  icon={Play}
                  size="small"
                >
                  New Execution
                </Button>
              {/snippet}
            </SectionHeader>

            {#if executions.length > 0}
              <div class="space-y-3">
                {#each executions as execution}
                  {@const status = getRunStatus(execution)}
                  <div class="border rounded p-4 hover:bg-gray-50 transition" style="border-color: var(--ds-border);">
                    <div class="flex items-center justify-between">
                      <div class="flex-1">
                        <div class="font-medium mb-1" style="color: var(--ds-text);">
                          {execution.name}
                        </div>
                        <div class="text-sm" style="color: var(--ds-text-subtle);">
                          Started: {new Date(execution.started_at).toLocaleString()}
                          {#if execution.ended_at}
                            • Ended: {new Date(execution.ended_at).toLocaleString()}
                          {/if}
                        </div>
                      </div>
                      <div class="flex items-center gap-3">
                        <span class="px-2 py-1 text-xs font-semibold rounded-full {status.color}">
                          {status.text}
                        </span>
                        <div class="flex gap-2">
                          {#if !execution.ended_at}
                            <button
                              onclick={() => continueExecution(execution)}
                              class="text-green-600 hover:text-green-900 cursor-pointer text-sm font-medium"
                            >
                              Continue
                            </button>
                          {/if}
                          <button
                            onclick={() => viewRunDetails(execution)}
                            class="text-blue-600 hover:text-blue-900 cursor-pointer text-sm"
                          >
                            {execution.ended_at ? 'Results' : 'Progress'}
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {:else}
              <div class="text-center py-8">
                <div class="text-6xl mb-4">🚀</div>
                <div class="text-lg font-medium mb-2" style="color: var(--ds-text);">No executions yet</div>
                <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">
                  Click "Execute Template" to create your first test run from this template
                </div>
                <Button
                  variant="primary"
                  onclick={executeTemplate}
                  icon={Play}
                  size="medium"
                >
                  Execute Template
                </Button>
              </div>
            {/if}
          </div>
        </div>

        <!-- Sidebar -->
        <div class="space-y-6">
          <!-- Quick Stats -->
          <div class="p-6" style="background-color: var(--ds-surface-raised);">
            <h3 class="font-semibold mb-4" style="color: var(--ds-text);">Quick Stats</h3>

            <div class="space-y-3">
              <div class="flex justify-between">
                <span class="text-sm" style="color: var(--ds-text-subtle);">Total Executions</span>
                <span class="text-sm font-medium" style="color: var(--ds-text);">{executions.length}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-sm" style="color: var(--ds-text-subtle);">Completed</span>
                <span class="text-sm font-medium text-green-600">
                  {executions.filter(e => e.ended_at).length}
                </span>
              </div>
              <div class="flex justify-between">
                <span class="text-sm" style="color: var(--ds-text-subtle);">In Progress</span>
                <span class="text-sm font-medium text-blue-600">
                  {executions.filter(e => !e.ended_at).length}
                </span>
              </div>
            </div>
          </div>

          <!-- Test Set Info -->
          {#if testSet}
            <div class="p-6" style="background-color: var(--ds-surface-raised);">
              <h3 class="font-semibold mb-4" style="color: var(--ds-text);">Test Plan Details</h3>

              <div class="space-y-3">
                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">Name</div>
                  <a href={`/workspaces/${workspaceId}/tests/sets/${testSet.id}`} class="text-sm text-blue-600 hover:text-blue-800 hover:underline">
                    {testSet.name}
                  </a>
                </div>
                {#if testSet.description}
                  <div>
                    <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">Description</div>
                    <div class="text-sm" style="color: var(--ds-text);">
                      {testSet.description}
                    </div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      </div>
    {:else}
      <div class="text-center py-12">
        <div class="text-gray-500">Template not found</div>
        <button
          onclick={goBack}
          class="mt-4 bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700 transition cursor-pointer"
        >
          Back to Templates
        </button>
      </div>
    {/if}
  </div>
</div>

<!-- Confirm Dialog -->
<ConfirmDialog
  bind:show={showConfirmDialog}
  title={confirmTitle}
  message={confirmMessage}
  confirmText={confirmAction ? "Confirm" : "OK"}
  cancelText={confirmAction ? "Cancel" : ""}
  variant={confirmAction ? "danger" : "info"}
  onconfirm={() => {
    if (confirmAction) {
      confirmAction();
    }
    showConfirmDialog = false;
  }}
  oncancel={() => showConfirmDialog = false}
/>
