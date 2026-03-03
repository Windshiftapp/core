<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { currentRoute, navigate } from '../router.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { currentWorkspace, workspacesStore } from '../stores';
  import { formatDateSimple } from '../utils/dateFormatter.js';
  import { Plus, CheckSquare } from 'lucide-svelte';
  import WorkspaceNavigation from './WorkspaceNavigation.svelte';
  import TodoList from '../features/items/TodoList.svelte';
  import ViewHeader from '../layout/ViewHeader.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Card from '../components/Card.svelte';

  export let workspaceId;

  let workspace = null;
  let projects = [];
  let loading = true;
  let showCreateForm = false;
  let currentCollection = 'Default';
  let currentView = 'board';
  let formData = {
    name: '',
    description: '',
    active: true
  };

  // Reactive statement to get workspace ID from store if on personal route
  $: effectiveWorkspaceId = $currentRoute.path?.startsWith('/personal')
    ? $workspacesStore.personalWorkspace?.id
    : workspaceId;

  onMount(async () => {
    // Wait for personal workspace to load if on personal route
    if ($currentRoute.path?.startsWith('/personal')) {
      await workspacesStore.loadPersonalWorkspace();
    }

    if (effectiveWorkspaceId) {
      await Promise.all([loadWorkspace(), loadProjects()]);
    }
    loading = false;
  });

  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(effectiveWorkspaceId);
      // Update the currentWorkspace store so WorkspaceNavigation can react to it
      await currentWorkspace.load(effectiveWorkspaceId);
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function loadProjects() {
    try {
      const result = await api.workspaces.getProjects(effectiveWorkspaceId);
      projects = result || [];
    } catch (error) {
      console.error('Failed to load projects:', error);
      projects = [];
    }
  }

  function startCreate() {
    showCreateForm = true;
    resetForm();
  }

  function resetForm() {
    formData = {
      name: '',
      description: '',
      active: true
    };
  }

  function cancelForm() {
    showCreateForm = false;
    resetForm();
  }

  async function saveProject() {
    try {
      const data = {
        ...formData,
        workspace_id: parseInt(effectiveWorkspaceId)
      };

      await api.projects.create(data);
      await loadProjects();
      cancelForm();
    } catch (error) {
      console.error('Failed to save project:', error);
      alert('Failed to save project. Please check your input and try again.');
    }
  }

  async function deleteProject(project) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: `Are you sure you want to delete "${project.name}"?`,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await api.projects.delete(project.id);
        await loadProjects();
      } catch (error) {
        console.error('Failed to delete project:', error);
      }
    }
  }
  
  function handleViewChange(event) {
    currentView = event.detail.view;
    currentCollection = event.detail.collection;
  }
  
  function handleCollectionChange(event) {
    currentCollection = event.detail.collection;
    currentView = event.detail.view;
  }
</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">Loading...</div>
  </div>
{:else if workspace}
  {#if workspace.is_personal}
    <!-- Personal Todo Workspace - Simplified Interface -->
    <div class="p-6" style="background-color: var(--ds-surface); min-height: 100vh;">
      <PageHeader
        icon={CheckSquare}
        title={workspace.name}
        subtitle="Personal task management"
      >
        {#snippet actions()}
          <span class="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-orange-100 text-orange-800">
            Personal
          </span>
        {/snippet}
      </PageHeader>

      <!-- Todo List Interface -->
      <TodoList workspaceId={effectiveWorkspaceId} />
    </div>
  {:else}
    <!-- Regular Workspace - Full Interface -->
    <div class="p-6" style="background-color: var(--ds-surface); min-height: 100vh;">
      <!-- Workspace Navigation -->
      <WorkspaceNavigation
        workspaceId={effectiveWorkspaceId}
        bind:currentCollection
        bind:currentView
        onviewchange={handleViewChange}
        oncollectionchange={handleCollectionChange}
      />

      <!-- Content Container -->
      <div class="max-w-[80vw] mx-auto p-6 mt-8">
        <div class="mb-8">
          <ViewHeader 
            workspaceName={workspace.name}
            viewName="Overview"
            itemCount={projects.length}
          >
            <div slot="actions">
              <button
                onclick={startCreate}
                class="bg-blue-600 text-white px-6 py-3 rounded hover:bg-blue-700 transition-all duration-200 font-semibold shadow-sm hover:shadow-md"
              >
                <div class="flex items-center gap-2">
                  <Plus class="w-4 h-4" />
                  Add Project
                </div>
              </button>
            </div>
          </ViewHeader>
        </div>

    {#if showCreateForm}
      <Card rounded="xl" shadow padding="spacious" class="mb-8">
        <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
          New Project
        </h3>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div>
            <Label required class="mb-2">Project Name</Label>
            <input
              type="text"
              bind:value={formData.name}
              class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="Enter project name"
              required
            />
          </div>

          <div>
            <Label class="mb-2">Description</Label>
            <input
              type="text"
              bind:value={formData.description}
              class="w-full px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="Optional project description"
            />
          </div>
        </div>

        <div class="mt-6">
          <Checkbox
            bind:checked={formData.active}
            label="Active Project"
            size="small"
          />
        </div>

        <div class="mt-8 flex gap-3">
          <button
            onclick={saveProject}
            class="bg-blue-600 text-white px-6 py-3 rounded hover:bg-blue-700 transition-all duration-200 font-semibold shadow-sm hover:shadow-md"
            disabled={!formData.name.trim()}
          >
            Create Project
          </button>
          <button
            onclick={cancelForm}
            class="px-6 py-3 rounded transition-all duration-200 font-medium hover:shadow-sm"
            style="background-color: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);"
          >
            Cancel
          </button>
        </div>
      </Card>
    {/if}

    <!-- Projects List -->
    <Card rounded="xl" shadow padding="none" class="overflow-hidden">
      {#if projects.length === 0}
        <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
          No projects found in this workspace. Create your first project to get started.
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full">
            <thead style="background-color: var(--ds-background-neutral);">
              <tr>
                <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Project</th>
                <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Status</th>
                <th class="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Created</th>
                <th class="px-6 py-4 text-right text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y" style="divide-color: var(--ds-border);">
              {#each projects as project (project.id)}
                <tr class="transition-colors duration-150 hover:bg-opacity-50" style="hover:background-color: var(--ds-background-neutral-hovered);">
                  <td class="px-6 py-4">
                    <div>
                      <div class="font-semibold" style="color: var(--ds-text);">{project.name}</div>
                      {#if project.description}
                        <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{project.description}</div>
                      {/if}
                    </div>
                  </td>
                  <td class="px-6 py-4">
                    <Lozenge color={project.active ? 'green' : 'gray'} text={project.active ? 'Active' : 'Inactive'} />
                  </td>
                  <td class="px-6 py-4 text-sm" style="color: var(--ds-text-subtle);">
                    {formatDateSimple(project.created_at)}
                  </td>
                  <td class="px-6 py-4 text-right text-sm font-medium">
                    <button
                      onclick={() => deleteProject(project)}
                      class="text-red-600 hover:text-red-800 transition-colors duration-150 px-2 py-1 rounded hover:bg-red-50"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </Card>
      </div>
    </div>
  {/if}
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      Workspace not found.
    </div>
  </div>
{/if}