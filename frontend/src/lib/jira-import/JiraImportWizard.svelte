<script>
  import { jiraImport } from './JiraImportStore.svelte.js';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Input from '../components/Input.svelte';
  import FormField from '../components/FormField.svelte';
  import {
    Cloud, Check, ChevronRight, ChevronLeft, ArrowRight,
    Briefcase, FileText, Activity, Hash, Box, AlertCircle,
    ExternalLink, Eye, EyeOff, Plus, Users
  } from 'lucide-svelte';
  import { addToast } from '../stores/toasts.svelte.js';

  let {
    isOpen = $bindable(false),
    onComplete = () => {},
    onClose = () => {}
  } = $props();

  // Local state for connect form
  let jiraUrl = $state('');
  let email = $state('');
  let apiToken = $state('');
  let showToken = $state(false);
  let showNewConnectionForm = $state(false);

  // Derived state from store
  let savedConnections = $derived(jiraImport.savedConnections);
  let connection = $derived(jiraImport.connection);
  let projects = $derived(jiraImport.projects);
  let analysis = $derived(jiraImport.analysis);
  let mappings = $derived(jiraImport.mappings);
  let wizard = $derived(jiraImport.wizard);
  let importData = $derived(jiraImport.import);

  let currentStep = $derived(wizard.currentStep);
  let steps = $derived(wizard.steps);
  let currentStepId = $derived(steps[currentStep]?.id || 'connect');

  // Load saved connections when modal opens
  $effect(() => {
    if (isOpen) {
      jiraImport.loadSavedConnections();
    }
  });

  // Search filter for projects
  let projectSearch = $state('');
  let filteredProjects = $derived(
    projects.available.filter(p =>
      p.name.toLowerCase().includes(projectSearch.toLowerCase()) ||
      p.key.toLowerCase().includes(projectSearch.toLowerCase())
    )
  );

  // Handle connection test (new connection)
  async function handleConnect() {
    const result = await jiraImport.testConnection(jiraUrl, email, apiToken);
    if (result.success) {
      addToast({ message: 'Connected to Jira successfully!', variant: 'success' });
      // Load projects after connecting
      await jiraImport.loadProjects();
      jiraImport.nextStep();
    } else {
      addToast({ message: result.error, variant: 'error', title: 'Connection Failed' });
    }
  }

  // Select and use a saved connection
  async function selectSavedConnection(conn) {
    isLoadingSavedConnection = true;
    jiraImport.useSavedConnection(conn);
    addToast({ message: `Connected to ${conn.instance_name || 'Jira Cloud'}`, variant: 'success' });
    await jiraImport.loadProjects();
    isLoadingSavedConnection = false;
    jiraImport.nextStep();
  }

  // State for loading saved connection
  let isLoadingSavedConnection = $state(false);

  // State for analyzing
  let isAnalyzing = $state(false);

  // State for loading projects when clicking Continue
  let isContinueLoading = $state(false);

  // Handle project selection confirmation
  async function handleAnalyze() {
    isAnalyzing = true;
    const result = await jiraImport.analyzeProjects();
    isAnalyzing = false;
    if (result.success) {
      jiraImport.nextStep();
    }
  }

  // Close handler
  function handleClose() {
    jiraImport.reset();
    isOpen = false;
    onClose();
  }

  // Next step handler
  async function handleNext() {
    if (currentStepId === 'connect') {
      if (connection.isConnected) {
        // Already connected via saved connection - load projects if needed and proceed
        if (projects.available.length === 0) {
          isContinueLoading = true;
          await jiraImport.loadProjects();
          isContinueLoading = false;
        }
        jiraImport.nextStep();
      } else {
        handleConnect();
      }
    } else if (currentStepId === 'projects') {
      handleAnalyze();
    } else if (currentStepId === 'preview') {
      jiraImport.startImport();
    } else if (currentStepId === 'import') {
      handleClose();
    } else {
      jiraImport.nextStep();
    }
  }

  // Get step status
  function getStepStatus(index) {
    if (index < currentStep) return 'completed';
    if (index === currentStep) return 'current';
    return 'pending';
  }

  // Get confirm button label based on current step
  function getConfirmLabel() {
    if (currentStepId === 'connect') {
      return connection.isConnected ? 'Continue' : 'Connect';
    } else if (currentStepId === 'projects') {
      return 'Analyze & Configure';
    } else if (currentStepId === 'mapping') {
      return 'Continue';
    } else if (currentStepId === 'preview') {
      return 'Start Import';
    } else if (currentStepId === 'import') {
      return 'Done';
    }
    return 'Next';
  }

  // Check if confirm button should be shown
  function shouldShowConfirmButton() {
    // Hide confirm button when showing saved connections list
    if (currentStepId === 'connect' && savedConnections.items.length > 0 && !showNewConnectionForm && !connection.isConnected) {
      return false;
    }
    return true;
  }
</script>

<Modal bind:isOpen maxWidth="max-w-4xl" onclose={handleClose}>
  <div class="flex flex-col max-h-[90vh]">
    <!-- Header -->
    <ModalHeader
      title="Jira Cloud Import"
      subtitle={connection.instanceInfo?.display_name || 'Import work items from Jira Cloud'}
      icon={Cloud}
      onClose={handleClose}
    />

    <!-- Step indicator -->
    <div class="px-6 py-3 border-b flex items-center overflow-x-auto" style="border-color: var(--ds-border);">
      {#each steps as step, index}
        {@const status = getStepStatus(index)}
        <div class="flex items-center flex-shrink-0">
          <div class="flex items-center gap-2">
            <div class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-medium transition-colors"
                 style="background: {status !== 'pending' ? 'var(--ds-interactive)' : 'var(--ds-background-neutral)'}; color: {status !== 'pending' ? 'white' : 'var(--ds-text-subtle)'};">
              {#if status === 'completed'}
                <Check size={14} />
              {:else}
                {index + 1}
              {/if}
            </div>
            <span class="text-xs font-medium whitespace-nowrap"
                  style="color: {status === 'current' ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
              {step.label}
            </span>
          </div>
          {#if index < steps.length - 1}
            <div class="w-8 h-px mx-2"
                 style="background: {status === 'completed' ? 'var(--ds-interactive)' : 'var(--ds-border)'};">
            </div>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Content area -->
    <div class="p-6 overflow-y-auto flex-1 min-h-0">
      {#if currentStepId === 'connect'}
        <!-- Connect Step -->
        <div class="space-y-6">
          {#if connection.isConnected}
            <!-- Already connected -->
            <AlertBox variant="success" message="Connected to {connection.instanceInfo?.display_name || 'Jira Cloud'}" />
            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-blue-subtler);">
                  <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                </div>
                <div>
                  <p class="font-medium" style="color: var(--ds-text);">{connection.instanceInfo?.display_name || 'Jira Cloud'}</p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{connection.email}</p>
                </div>
              </div>
            </div>
          {:else if savedConnections.items.length > 0 && !showNewConnectionForm}
            <!-- Show saved connections -->
            {#if isLoadingSavedConnection}
              <div class="flex flex-col items-center justify-center py-12">
                <Spinner size="lg" />
                <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">Loading projects...</p>
              </div>
            {:else}
              <AlertBox variant="info" message="Select an existing connection or create a new one" />

              <div class="space-y-3">
                {#each savedConnections.items as conn}
                  <button
                    type="button"
                    class="w-full p-4 rounded-lg border text-left transition-all hover:border-blue-400"
                    style="border-color: var(--ds-border); background: var(--ds-surface);"
                    onclick={() => selectSavedConnection(conn)}
                  >
                    <div class="flex items-center gap-3">
                      <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                           style="background: var(--ds-background-accent-blue-subtler);">
                        <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                      </div>
                      <div class="flex-1">
                        <p class="font-medium" style="color: var(--ds-text);">
                          {conn.instance_name || 'Jira Cloud'}
                        </p>
                        <p class="text-sm" style="color: var(--ds-text-subtle);">{conn.email}</p>
                      </div>
                      <ChevronRight size={16} style="color: var(--ds-text-subtle);" />
                    </div>
                  </button>
                {/each}
              </div>

              <div class="pt-2">
                <Button variant="ghost" onclick={() => showNewConnectionForm = true}>
                  <Plus size={16} class="mr-2" />
                  Add New Connection
                </Button>
              </div>
            {/if}
          {:else}
            <!-- New connection form -->
            {#if savedConnections.items.length > 0}
              <div class="flex items-center justify-between mb-4">
                <span class="text-sm font-medium" style="color: var(--ds-text);">New Connection</span>
                <Button variant="ghost" size="small" onclick={() => showNewConnectionForm = false}>
                  Use Existing
                </Button>
              </div>
            {/if}

            <AlertBox variant="info" message="Enter your Jira Cloud credentials to connect. You can generate an API token from your Atlassian account settings." />

            <FormField label="Jira Cloud URL" required>
              <Input
                bind:value={jiraUrl}
                placeholder="https://your-domain.atlassian.net"
                disabled={connection.isConnecting}
              />
            </FormField>

            <FormField label="Email Address" required>
              <Input
                bind:value={email}
                type="email"
                placeholder="your.email@company.com"
                disabled={connection.isConnecting}
              />
            </FormField>

            <FormField label="API Token" required>
              <div class="relative">
                <Input
                  bind:value={apiToken}
                  type={showToken ? 'text' : 'password'}
                  placeholder="Your Jira API token"
                  disabled={connection.isConnecting}
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded hover-bg"
                  onclick={() => showToken = !showToken}
                >
                  {#if showToken}
                    <EyeOff size={16} style="color: var(--ds-text-subtle);" />
                  {:else}
                    <Eye size={16} style="color: var(--ds-text-subtle);" />
                  {/if}
                </button>
              </div>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                <a href="https://id.atlassian.com/manage-profile/security/api-tokens"
                   target="_blank" rel="noopener noreferrer"
                   class="underline hover:no-underline" style="color: var(--ds-link);">
                  Generate a token
                </a> from your Atlassian account settings
              </p>
            </FormField>

            {#if connection.error}
              <AlertBox variant="error" message={connection.error} />
            {/if}
          {/if}
        </div>

      {:else if currentStepId === 'projects'}
        <!-- Project Selection Step -->
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Select projects to import ({projects.selected.length} of {projects.available.length} selected)
            </p>
            <div class="flex gap-2">
              <Button variant="ghost" size="small" onclick={() => jiraImport.selectAllProjects()}>
                Select All
              </Button>
              <Button variant="ghost" size="small" onclick={() => jiraImport.deselectAllProjects()}>
                Deselect All
              </Button>
            </div>
          </div>

          <!-- Open Issues Only Toggle -->
          <div class="flex items-center gap-3 p-3 rounded-lg border"
               style="border-color: var(--ds-border); background: var(--ds-surface);">
            <input
              type="checkbox"
              id="openIssuesOnly"
              checked={projects.openIssuesOnly}
              onchange={async () => {
                jiraImport.toggleOpenIssuesOnly();
                await jiraImport.reloadProjectsWithFilter();
              }}
              class="w-4 h-4 rounded"
            />
            <div class="flex-1">
              <label for="openIssuesOnly" class="text-sm font-medium cursor-pointer" style="color: var(--ds-text);">
                Import open issues only
              </label>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                Excludes issues with status category "Done"
              </p>
            </div>
          </div>

          <Input
            bind:value={projectSearch}
            placeholder="Search projects..."
          />

          {#if projects.isLoading}
            <div class="flex items-center justify-center py-12">
              <Spinner size="lg" />
            </div>
          {:else}
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3 max-h-96 overflow-y-auto">
              {#each filteredProjects as project}
                {@const isSelected = projects.selected.includes(project.key)}
                {@const isDisabled = project.is_team_managed}
                <button
                  type="button"
                  class="p-4 rounded-lg border text-left transition-all"
                  style="border-color: {isSelected ? 'var(--ds-border-focused)' : 'var(--ds-border)'}; background: {isSelected ? 'var(--ds-background-selected)' : 'transparent'}; opacity: {isDisabled ? '0.5' : '1'};"
                  onclick={() => !isDisabled && jiraImport.toggleProject(project.key)}
                  disabled={isDisabled}
                >
                  <div class="flex items-start gap-3">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      disabled={isDisabled}
                      class="mt-1 w-4 h-4 rounded"
                      onclick={(e) => e.stopPropagation()}
                      onchange={() => !isDisabled && jiraImport.toggleProject(project.key)}
                    />
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        {#if project.avatar_url}
                          <img src={project.avatar_url} alt="" class="w-6 h-6 rounded" />
                        {/if}
                        <span class="font-medium truncate" style="color: var(--ds-text);">
                          {project.name}
                        </span>
                        {#if isDisabled}
                          <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-background-warning-subtle); color: var(--ds-text-warning);">
                            Team-managed
                          </span>
                        {/if}
                      </div>
                      <div class="flex items-center gap-2 mt-1">
                        <span class="text-xs px-1.5 py-0.5 rounded"
                              style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {project.key}
                        </span>
                        <span class="text-xs" style="color: var(--ds-text-subtle);">
                          {project.issue_count.toLocaleString()} issues
                        </span>
                      </div>
                    </div>
                  </div>
                </button>
              {/each}
            </div>
          {/if}
        </div>

      {:else if currentStepId === 'mapping'}
        <!-- Consolidated Mapping Step -->
        <div class="space-y-6">
          <!-- Workspaces Section -->
          <div class="space-y-3">
            <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
              <Briefcase size={18} style="color: var(--ds-text-accent-blue);" />
              <h3 class="font-medium" style="color: var(--ds-text);">Workspaces</h3>
              <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {mappings.workspaces.length}
              </span>
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              Each Jira project will become a Windshift workspace
            </p>
            <div class="space-y-2">
              {#each mappings.workspaces as mapping}
                <div class="p-3 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface);">
                  <div class="flex items-center gap-3">
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="font-medium truncate" style="color: var(--ds-text);">{mapping.jiraName}</span>
                        <span class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
                              style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {mapping.jiraKey}
                        </span>
                        <span class="text-xs flex-shrink-0" style="color: var(--ds-text-subtle);">
                          {mapping.issueCount.toLocaleString()} issues
                        </span>
                      </div>
                    </div>
                    <ArrowRight size={14} style="color: var(--ds-text-subtle);" />
                    <div class="w-48 flex-shrink-0">
                      <Input
                        bind:value={mapping.newWorkspaceName}
                        placeholder="Workspace name"
                        size="small"
                      />
                    </div>
                  </div>
                </div>
              {/each}
            </div>
          </div>

          <!-- Issue Types Section -->
          <div class="space-y-3">
            <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
              <FileText size={18} style="color: var(--ds-text-accent-purple);" />
              <h3 class="font-medium" style="color: var(--ds-text);">Issue Types</h3>
              <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {mappings.issueTypes.length}
              </span>
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              Issue types will be created as item types in Windshift
            </p>
            <div class="flex flex-wrap gap-2">
              {#each mappings.issueTypes as mapping}
                <div class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
                     style="border-color: var(--ds-border); background: var(--ds-surface);">
                  <span class="text-sm" style="color: var(--ds-text);">{mapping.jiraName}</span>
                  {#if mapping.isSubtask}
                    <span class="text-xs px-1 py-0.5 rounded"
                          style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                      Sub-task
                    </span>
                  {/if}
                </div>
              {/each}
            </div>
          </div>

          <!-- Statuses Section -->
          <div class="space-y-3">
            <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
              <Activity size={18} style="color: var(--ds-text-accent-green);" />
              <h3 class="font-medium" style="color: var(--ds-text);">Statuses</h3>
              <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {mappings.statuses.length}
              </span>
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              Statuses will be created and grouped by category
            </p>
            <div class="flex flex-wrap gap-2">
              {#each mappings.statuses as mapping}
                <div class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
                     style="border-color: var(--ds-border); background: var(--ds-surface);">
                  {#if mapping.color}
                    <div class="w-2.5 h-2.5 rounded-full flex-shrink-0" style="background: {mapping.color};"></div>
                  {/if}
                  <span class="text-sm" style="color: var(--ds-text);">{mapping.jiraName}</span>
                  <span class="text-xs px-1 py-0.5 rounded"
                        style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    {mapping.categoryName}
                  </span>
                </div>
              {/each}
            </div>
          </div>

          <!-- Custom Fields Section -->
          {#if mappings.customFields.length > 0}
            <div class="space-y-3">
              <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
                <Hash size={18} style="color: var(--ds-text-accent-orange);" />
                <h3 class="font-medium" style="color: var(--ds-text);">Custom Fields</h3>
                <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {mappings.customFields.filter(f => f.canMap).length} / {mappings.customFields.length}
                </span>
              </div>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                Custom fields that can be mapped will be created in Windshift
              </p>
              <div class="space-y-2 max-h-48 overflow-y-auto">
                {#each mappings.customFields as mapping}
                  <div class="p-2 rounded-lg border flex items-center gap-3"
                       style="border-color: var(--ds-border); background: var(--ds-surface);">
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="text-sm truncate" style="color: var(--ds-text);">{mapping.jiraName}</span>
                        <span class="text-xs px-1 py-0.5 rounded flex-shrink-0"
                              style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {mapping.windshiftType}
                        </span>
                      </div>
                      {#if mapping.notes}
                        <p class="text-xs mt-0.5 truncate" style="color: var(--ds-text-subtle);">
                          {mapping.notes}
                        </p>
                      {/if}
                    </div>
                    <div class="flex-shrink-0">
                      {#if mapping.canMap}
                        <span class="text-xs px-2 py-1 rounded"
                              style="background: var(--ds-background-success-bold); color: white;">
                          Create
                        </span>
                      {:else}
                        <span class="text-xs px-2 py-1 rounded"
                              style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          Skip
                        </span>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        </div>

      {:else if currentStepId === 'preview'}
        <!-- Preview Step -->
        <div class="space-y-6">
          <AlertBox variant="warning" message="Review the import summary before proceeding. This operation may take several minutes for large projects." />

          <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-blue-subtler);">
                  <Briefcase class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                </div>
                <div>
                  <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                    {projects.selected.length}
                  </p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">Workspaces</p>
                </div>
              </div>
            </div>

            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-green-subtler);">
                  <FileText class="w-5 h-5" style="color: var(--ds-text-accent-green);" />
                </div>
                <div>
                  <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                    {analysis.result?.total_issues?.toLocaleString() || 0}
                  </p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">Work Items</p>
                </div>
              </div>
            </div>

            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-purple-subtler);">
                  <Activity class="w-5 h-5" style="color: var(--ds-text-accent-purple);" />
                </div>
                <div>
                  <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                    {mappings.statuses.length}
                  </p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">Statuses</p>
                </div>
              </div>
            </div>

            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-orange-subtler);">
                  <Hash class="w-5 h-5" style="color: var(--ds-text-accent-orange);" />
                </div>
                <div>
                  <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                    {mappings.issueTypes.length}
                  </p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">Item Types</p>
                </div>
              </div>
            </div>

            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-teal-subtler);">
                  <Box class="w-5 h-5" style="color: var(--ds-text-accent-teal);" />
                </div>
                <div>
                  <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                    {mappings.customFields.filter(f => f.canMap).length}
                  </p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">Custom Fields</p>
                </div>
              </div>
            </div>

            {#if analysis.result?.users?.length > 0}
              <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                       style="background: var(--ds-background-accent-blue-subtler);">
                    <Users class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                  </div>
                  <div>
                    <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                      {analysis.result.users.length}
                    </p>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">
                      Users
                      {#if analysis.result.users.filter(u => !u.matched_user_id).length > 0}
                        <span class="text-xs ml-1" style="color: var(--ds-text-accent-orange);">
                          ({analysis.result.users.filter(u => !u.matched_user_id).length} new)
                        </span>
                      {/if}
                    </p>
                  </div>
                </div>
              </div>
            {/if}

            {#if analysis.result?.total_assets > 0}
              <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                       style="background: var(--ds-background-accent-yellow-subtler);">
                    <Box class="w-5 h-5" style="color: var(--ds-text-accent-yellow);" />
                  </div>
                  <div>
                    <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                      {analysis.result?.total_assets?.toLocaleString() || 0}
                    </p>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">Assets</p>
                  </div>
                </div>
              </div>
            {/if}
          </div>

          <!-- Project breakdown -->
          <div class="space-y-2">
            <h3 class="font-medium" style="color: var(--ds-text);">Projects to Import</h3>
            {#each analysis.result?.projects || [] as project}
              <div class="p-3 rounded-lg border flex items-center justify-between"
                   style="border-color: var(--ds-border); background: var(--ds-surface);">
                <div class="flex items-center gap-2">
                  <Briefcase size={16} style="color: var(--ds-text-subtle);" />
                  <span class="font-medium" style="color: var(--ds-text);">{project.name}</span>
                  <span class="text-xs px-1.5 py-0.5 rounded"
                        style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    {project.key}
                  </span>
                </div>
                <span class="text-sm" style="color: var(--ds-text-subtle);">
                  {project.issue_count.toLocaleString()} issues
                </span>
              </div>
            {/each}
          </div>
        </div>

      {:else if currentStepId === 'import'}
        <!-- Import Step -->
        <div class="flex flex-col items-center justify-center py-12">
          {#if importData.error}
            <AlertBox variant="error" message={importData.error} />
            <div class="mt-4">
              <Button variant="secondary" onclick={() => jiraImport.startImport()}>
                Retry Import
              </Button>
            </div>
          {:else if importData.result}
            <div class="text-center">
              <Check class="w-16 h-16 mx-auto" style="color: var(--ds-text-success);" />
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                Import Complete!
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                Successfully imported {importData.progress?.imported_issues || 0} items.
              </p>
              {#if importData.progress?.failed_issues > 0}
                <p class="text-sm mt-1 text-amber-600">
                  {importData.progress.failed_issues} items failed to import.
                </p>
              {/if}
            </div>
          {:else if importData.isImporting || importData.jobId}
            <div class="text-center">
              <Spinner size="lg" class="mx-auto" />
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                Importing...
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                {importData.phase || 'Starting import...'}
              </p>
              {#if importData.progress}
                <div class="mt-4 w-64 mx-auto">
                  <div class="flex justify-between text-xs mb-1" style="color: var(--ds-text-subtle);">
                    <span>Progress</span>
                    <span>{importData.progress.imported_issues || 0} / {importData.progress.total_issues || 0}</span>
                  </div>
                  <div class="w-full h-2 rounded-full" style="background: var(--ds-background-neutral);">
                    <div
                      class="h-full rounded-full transition-all"
                      style="background: var(--ds-interactive-primary); width: {((importData.progress.imported_issues || 0) / (importData.progress.total_issues || 1)) * 100}%;"
                    ></div>
                  </div>
                </div>
              {/if}
            </div>
          {:else}
            <div class="text-center">
              <FileText class="w-16 h-16 mx-auto" style="color: var(--ds-text-subtle);" />
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                Ready to Import
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                Click "Start Import" to begin importing {analysis.result?.total_issues || 0} items.
              </p>
              <div class="mt-6">
                <Button variant="primary" onclick={() => jiraImport.startImport()}>
                  Start Import
                </Button>
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer with navigation -->
    <DialogFooter
      showCancel={false}
      confirmLabel={getConfirmLabel()}
      loading={connection.isConnecting || isAnalyzing || isContinueLoading}
      disabled={
        connection.isConnecting ||
        isAnalyzing ||
        isContinueLoading ||
        (currentStepId === 'connect' && !connection.isConnected && (!jiraUrl || !email || !apiToken)) ||
        (currentStepId === 'projects' && projects.selected.length === 0)
      }
      onConfirm={shouldShowConfirmButton() ? handleNext : null}
    >
      {#snippet extra()}
        <Button
          variant="ghost"
          onclick={() => currentStep > 0 ? jiraImport.prevStep() : handleClose()}
          disabled={connection.isConnecting || isAnalyzing}
        >
          {#if currentStep === 0}
            Cancel
          {:else}
            <ChevronLeft size={16} class="mr-1" />
            Back
          {/if}
        </Button>
      {/snippet}
    </DialogFooter>
  </div>
</Modal>
