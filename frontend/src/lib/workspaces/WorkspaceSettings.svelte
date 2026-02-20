<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { workspacePermissions, attachmentStatus } from '../stores';
  import { Trash2, AlertTriangle, Settings, Clock, Shield } from 'lucide-svelte';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import WorkspaceConfigurationAssigner from './WorkspaceConfigurationAssigner.svelte';
  import WorkspaceConfigurationPreview from './WorkspaceConfigurationPreview.svelte';
  import WorkspaceSCMSettings from './WorkspaceSCMSettings.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import CategoryMultiSelect from '../pickers/CategoryMultiSelect.svelte';
  import WorkspaceMembers from './WorkspaceMembers.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Label from '../components/Label.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Tabs from '../components/Tabs.svelte';
  import Card from '../components/Card.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null, activeTab = $bindable('general') } = $props();

  let workspace = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let showDeleteConfirm = $state(false);
  let deleteConfirmText = $state('');
  let timeProjects = $state([]);
  let configurationRefreshKey = $state(0);

  // Time project categories state
  let timeProjectCategories = $state([]);
  let selectedTimeProjectCategories = $state([]);

  let formData = $state({
    name: '',
    key: '',
    description: '',
    active: true,
    time_project_id: null,
    default_view: 'board'
  });

  // Settings tabs configuration
  const settingsTabs = $derived([
    { id: 'general', label: t('workspaceSettings.tabs.general') },
    { id: 'categories', label: t('workspaceSettings.tabs.categories') },
    { id: 'members', label: t('workspaceSettings.tabs.members') },
    { id: 'configuration', label: t('workspaceSettings.tabs.configurationSets') },
    { id: 'source-control', label: t('workspaceSettings.tabs.sourceControl') },
    { id: 'danger', label: t('workspaceSettings.tabs.removeWorkspace'), className: 'tab-danger' }
  ]);

  // Permission check for workspace admin
  const canAdmin = $derived(workspacePermissions.canAdminWorkspace(workspaceId));

  onMount(async () => {
    await moduleSettings.load();

    // Redirect from base settings route to general tab
    if (window.location.pathname === `/workspaces/${workspaceId}/settings`) {
      navigate(`/workspaces/${workspaceId}/settings/general`);
      // Don't return — still load data so the component isn't stuck in loading state
    }

    // Load all required data
    const loadPromises = [loadWorkspace(), loadTimeProjectCategories()];
    if ($moduleSettings.time_tracking_enabled) {
      loadPromises.push(loadTimeProjects());
    }

    await Promise.all(loadPromises);
    loading = false;
  });

  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(workspaceId);
      if (workspace) {
        formData = {
          name: workspace.name,
          key: workspace.key || '',
          description: workspace.description || '',
          active: workspace.active,
          time_project_id: workspace.time_project_id || null,
          default_view: workspace.default_view || 'board'
        };
      }
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function loadTimeProjects() {
    try {
      timeProjects = await api.time.projects.getAll() || [];
    } catch (error) {
      console.error('Failed to load time projects:', error);
      timeProjects = [];
    }
  }

  async function loadTimeProjectCategories() {
    try {
      timeProjectCategories = await api.time.projectCategories.getAll() || [];
      if (workspace?.time_project_categories) {
        selectedTimeProjectCategories = workspace.time_project_categories;
      }
    } catch (error) {
      console.error('Failed to load time project categories:', error);
      timeProjectCategories = [];
    }
  }

  async function saveWorkspace() {
    if (!formData.name.trim()) {
      errorToast(t('workspaceSettings.workspaceNameRequired'));
      return;
    }

    if (!formData.key.trim()) {
      errorToast(t('workspaceSettings.workspaceKeyRequired'));
      return;
    }

    try {
      saving = true;
      await api.workspaces.update(workspaceId, {
        ...formData,
        time_project_id: formData.time_project_id ? parseInt(formData.time_project_id, 10) : null,
        time_project_categories: selectedTimeProjectCategories
      });

      // Update local workspace object
      workspace = { ...workspace, ...formData };

      successToast(t('workspaceSettings.savedSuccessfully'));
    } catch (error) {
      console.error('Failed to save workspace:', error);
      errorToast(t('workspaceSettings.failedToSave', { error: error.message || error }));
    } finally {
      saving = false;
    }
  }

  function cancelDeleteWorkspace() {
    showDeleteConfirm = false;
    deleteConfirmText = '';
  }

  async function deleteWorkspace() {
    if (deleteConfirmText !== workspace.name) {
      errorToast(t('workspaceSettings.pleaseConfirmDeletion'));
      return;
    }

    try {
      await api.workspaces.delete(workspaceId);
      successToast(t('workspaceSettings.deletedSuccessfully', { name: workspace.name }));
      setTimeout(() => {
        navigate('/workspaces');
      }, 1000);
    } catch (error) {
      console.error('Failed to delete workspace:', error);
      errorToast(t('workspaceSettings.failedToDelete', { error: error.message || error }));
    }
  }

  function goBackToWorkspace() {
    navigate(`/workspaces/${workspaceId}`);
  }

  function goBackToWorkspaceList() {
    navigate('/workspaces');
  }

  function switchTab(tab) {
    if (tab === 'general') {
      navigate(`/workspaces/${workspaceId}/settings/general`);
    } else if (tab === 'categories') {
      navigate(`/workspaces/${workspaceId}/settings/categories`);
    } else if (tab === 'members') {
      navigate(`/workspaces/${workspaceId}/settings/members`);
    } else if (tab === 'configuration') {
      navigate(`/workspaces/${workspaceId}/settings/configuration`);
    } else if (tab === 'source-control') {
      navigate(`/workspaces/${workspaceId}/settings/source-control`);
    } else if (tab === 'danger') {
      navigate(`/workspaces/${workspaceId}/settings/danger`);
    } else {
      navigate(`/workspaces/${workspaceId}/settings`);
    }
  }

  function handleTabChange(detail) {
    switchTab(detail.tab);
  }

  function handleConfigurationChanged() {
    configurationRefreshKey++;
  }
</script>

{#if loading}
  <Card rounded="xl" shadow padding="spacious">
    <div class="animate-pulse">
      <div class="h-4 rounded w-1/4 mb-4" style="background-color: var(--ds-surface);"></div>
      <div class="h-4 rounded w-3/4" style="background-color: var(--ds-surface);"></div>
    </div>
  </Card>
{:else if !canAdmin}
  <Card rounded="xl" shadow padding="loose">
    <div class="text-center py-8">
      <Shield class="w-12 h-12 mx-auto mb-4 text-amber-500" />
      <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">{t('workspaceSettings.accessDenied')}</h2>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('workspaceSettings.accessDeniedDescription')}</p>
      <Button onclick={() => navigate(`/workspaces/${workspaceId}`)} variant="primary">
        {t('workspaceSettings.backToWorkspace')}
      </Button>
    </div>
  </Card>
{:else if workspace}
  <div class="space-y-6">
    <!-- Header -->
    <div class="mb-6">
      <!-- Breadcrumb Navigation -->
      <div class="flex items-center gap-2 text-sm mb-4" style="color: var(--ds-text-subtle);">
        <button
          onclick={goBackToWorkspaceList}
          class="breadcrumb-link transition-colors"
        >
          {t('workspaceSettings.breadcrumbs.workspaces')}
        </button>
        <span>/</span>
        <button
          onclick={goBackToWorkspace}
          class="breadcrumb-link transition-colors"
        >
          {workspace.name}
        </button>
        <span>/</span>
        <span class="flex items-center gap-1" style="color: var(--ds-text);">
          <Settings class="w-4 h-4" style="color: #3b82f6;" />
          {t('workspaceSettings.breadcrumbs.settings')}
        </span>
      </div>

      <PageHeader
        icon={Settings}
        title={t('workspaceSettings.title')}
        subtitle={t('workspaceSettings.subtitle', { name: workspace?.name || 'workspace' })}
      />
    </div>

    <Tabs tabs={settingsTabs} bind:activeTab onTabChange={handleTabChange}>
      {#if activeTab === 'general'}
        <!-- Basic Information -->
        <h3 class="text-lg font-medium mb-6" style="color: var(--ds-text);">{t('workspaceSettings.basicInformation')}</h3>

        <div class="space-y-6">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <Label for="workspace-name" required class="mb-2">{t('workspaceSettings.workspaceName')}</Label>
            <Input
              id="workspace-name"
              bind:value={formData.name}
              placeholder={t('workspaceSettings.workspaceNamePlaceholder')}
              required
            />
          </div>

          <div>
            <Label for="workspace-key" required class="mb-2">{t('workspaceSettings.workspaceKey')}</Label>
            <Input
              id="workspace-key"
              bind:value={formData.key}
              placeholder={t('workspaceSettings.workspaceKeyPlaceholder')}
              required
            />
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {t('workspaceSettings.workspaceKeyHelp')}
            </p>
          </div>
        </div>

        <div>
          <Label for="workspace-description" class="mb-2">{t('workspaceSettings.description')}</Label>
          <Textarea
            id="workspace-description"
            bind:value={formData.description}
            rows={3}
            placeholder={t('workspaceSettings.descriptionPlaceholder')}
          />
        </div>

        {#if $moduleSettings.time_tracking_enabled}
          <div>
            <Label for="workspace-project" class="mb-2">{t('workspaceSettings.defaultTimeProject')}</Label>
            <Select
              id="workspace-project"
              bind:value={formData.time_project_id}
            >
              <option value={null}>{t('workspaceSettings.noDefaultProject')}</option>
              {#each timeProjects as project}
                <option value={project.id}>{project.name} ({project.customer_name})</option>
              {/each}
            </Select>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              {t('workspaceSettings.defaultTimeProjectHelp')}
            </p>
          </div>
        {/if}

        <div>
          <Label for="workspace-view" class="mb-2">{t('workspaceSettings.defaultView')}</Label>
          <Select
            id="workspace-view"
            bind:value={formData.default_view}
          >
            <option value="board">{t('workspaceSettings.views.board')}</option>
            <option value="backlog">{t('workspaceSettings.views.backlog')}</option>
            <option value="list">{t('workspaceSettings.views.list')}</option>
            <option value="tree">{t('workspaceSettings.views.tree')}</option>
            <option value="map">{t('workspaceSettings.views.map')}</option>
            <option value="overview">{t('workspaceSettings.views.overview')}</option>
          </Select>
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {t('workspaceSettings.defaultViewHelp')}
          </p>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium mb-1" style="color: var(--ds-text);">
              {t('workspaceSettings.activeWorkspace')}
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {t('workspaceSettings.activeWorkspaceHelp')}
            </p>
          </div>
          <Toggle bind:checked={formData.active} />
        </div>
        </div>

        <div class="flex items-center gap-3 mt-6">
        <Button
          variant="primary"
          size="medium"
          onclick={saveWorkspace}
          disabled={saving || !formData.name.trim() || !formData.key.trim()}
        >
          {#if saving}{t('workspaceSettings.saving')}{:else}{t('workspaceSettings.saveChanges')}{/if}
        </Button>
        <Button
          variant="secondary"
          size="medium"
          onclick={loadWorkspace}
        >
          {t('workspaceSettings.reset')}
        </Button>
      </div>
    {:else if activeTab === 'categories'}
        <!-- Project Category Restrictions -->
        <div class="flex items-center gap-3 mb-6">
          <Clock class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <h3 class="text-lg font-medium" style="color: var(--ds-text);">{t('workspaceSettings.projectCategoryRestrictions')}</h3>
        </div>

        <CategoryMultiSelect
          categories={timeProjectCategories}
          bind:selectedIds={selectedTimeProjectCategories}
          placeholder={t('workspaceSettings.selectProjectCategories')}
          helperText={t('workspaceSettings.categoryRestrictionsHelp')}
        />

        <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
          {t('workspaceSettings.leaveEmptyNote')}
        </p>

        <div class="flex items-center gap-3 mt-6">
          <Button
            variant="primary"
            size="medium"
            onclick={saveWorkspace}
            disabled={saving || !formData.name.trim() || !formData.key.trim()}
          >
            {#if saving}{t('workspaceSettings.saving')}{:else}{t('workspaceSettings.saveChanges')}{/if}
          </Button>
          <Button
            variant="secondary"
            size="medium"
            onclick={loadWorkspace}
          >
            {t('workspaceSettings.reset')}
          </Button>
        </div>
    {:else if activeTab === 'members'}
        <!-- Workspace Members -->
        <WorkspaceMembers {workspaceId} />
    {:else if activeTab === 'configuration'}
        <!-- Configuration Sets -->
        <WorkspaceConfigurationAssigner workspaceId={workspaceId} onconfigurationChanged={handleConfigurationChanged} />

        <!-- Active Configuration Preview -->
        <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
          <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('workspaceSettings.activeConfiguration')}</h3>
          {#key configurationRefreshKey}
            <WorkspaceConfigurationPreview {workspaceId} />
          {/key}
        </div>

    {:else if activeTab === 'source-control'}
        <!-- Source Control Settings -->
        <WorkspaceSCMSettings {workspaceId} />

    {:else if activeTab === 'danger'}
        <!-- Remove Workspace -->
        <div class="flex items-center gap-3 mb-6">
          <AlertTriangle class="w-5 h-5 text-red-600" />
          <h3 class="text-lg font-medium text-red-900">{t('workspaceSettings.permanentRemoval')}</h3>
        </div>

        <div class="text-sm text-red-700 mb-6">
          <p class="mb-4">{t('workspaceSettings.removeWarningIntro')}</p>
          <ul class="list-disc list-inside space-y-2 ml-4">
            <li>{t('workspaceSettings.removeWarningItems')}</li>
            <li>{t('workspaceSettings.removeWarningFields')}</li>
            <li>{t('workspaceSettings.removeWarningScreens')}</li>
            <li>{t('workspaceSettings.removeWarningFiles')}</li>
          </ul>
          <p class="mt-4 font-medium">{t('workspaceSettings.removeWarningFinal')}</p>
        </div>

        {#if !showDeleteConfirm}
          <button
            onclick={() => showDeleteConfirm = true}
            class="flex items-center gap-2 px-4 py-2 bg-red-600 text-white text-sm font-medium rounded hover:bg-red-700 transition-colors"
          >
            <Trash2 class="w-4 h-4" />
            {t('workspaceSettings.removeWorkspaceButton')}
          </button>
        {:else}
          <div class="space-y-4">
            <div>
              <label for="delete-confirm" class="block text-sm font-medium text-red-900 mb-2">
                {t('workspaceSettings.typeToConfirm', { name: workspace.name })}
              </label>
              <input
                id="delete-confirm"
                type="text"
                bind:value={deleteConfirmText}
                class="w-full px-4 py-2 rounded border border-red-300 text-red-900 bg-white focus:outline-none focus:ring-2 focus:ring-red-500"
                placeholder={t('workspaceSettings.typeNameHere', { name: workspace.name })}
              />
            </div>

            <div class="flex items-center gap-3">
              <button
                onclick={deleteWorkspace}
                disabled={deleteConfirmText !== workspace.name}
                class="px-4 py-2 bg-red-600 text-white text-sm font-medium rounded hover:bg-red-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {t('workspaceSettings.yesRemoveWorkspace')}
              </button>
              <button
                onclick={cancelDeleteWorkspace}
                class="px-4 py-2 text-sm font-medium rounded border border-red-300 text-red-700 hover:bg-red-50 transition-colors"
              >
                {t('workspaceSettings.cancel')}
              </button>
            </div>
          </div>
        {/if}
    {/if}
    </Tabs>

  </div>
{:else}
  <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <p class="text-center" style="color: var(--ds-text-subtle);">{t('workspaceSettings.workspaceNotFound')}</p>
  </div>
{/if}

<style>
  .breadcrumb-link:hover {
    color: var(--ds-text) !important;
  }

  :global(.tab-danger) {
    color: var(--ds-text-danger) !important;
  }
  :global(.tab-danger:hover) {
    color: var(--ds-text-danger) !important;
  }
</style>
