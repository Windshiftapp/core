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
    Cloud, Server, Check, ChevronRight, ChevronLeft, ArrowRight,
    Briefcase, FileText, Activity, Hash, Box, AlertCircle,
    ExternalLink, Eye, EyeOff, Plus, Users, Paperclip, Flag
  } from 'lucide-svelte';
  import { addToast } from '../stores/toasts.svelte.js';
  import { attachmentStatus } from '../stores/attachmentStatus.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    onComplete = () => {},
    onClose = () => {}
  } = $props();

  // Local state for connect form
  let jiraUrl = $state('');
  let email = $state('');
  let apiToken = $state('');
  let deploymentType = $state('cloud'); // 'cloud' or 'datacenter'
  let showToken = $state(false);
  let showNewConnectionForm = $state(false);

  // Computed labels based on deployment type
  let emailLabel = $derived(deploymentType === 'datacenter' ? t('jiraImport.form.username') : t('jiraImport.form.email'));
  let emailPlaceholder = $derived(deploymentType === 'datacenter' ? 'your.username' : 'your.email@company.com');
  let tokenLabel = $derived(deploymentType === 'datacenter' ? t('jiraImport.form.password') : t('jiraImport.form.apiToken'));
  let tokenHelpText = $derived(deploymentType === 'datacenter'
    ? t('jiraImport.form.tokenHelpDatacenter')
    : t('jiraImport.form.tokenHelpCloud'));
  let tokenHelpLink = $derived(deploymentType === 'datacenter'
    ? null  // No standard link for DC as it varies by instance
    : 'https://id.atlassian.com/manage-profile/security/api-tokens');
  let urlPlaceholder = $derived(deploymentType === 'datacenter'
    ? 'https://jira.your-company.com'
    : 'https://your-domain.atlassian.net');
  let modalTitle = $derived(deploymentType === 'datacenter' ? t('jiraImport.title.datacenter') : t('jiraImport.title.cloud'));
  let modalSubtitle = $derived(connection.instanceInfo?.display_name || (deploymentType === 'datacenter' ? t('jiraImport.subtitle.datacenter') : t('jiraImport.subtitle.cloud')));

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

  // Load saved connections and attachment status when modal opens
  $effect(() => {
    if (isOpen) {
      jiraImport.loadSavedConnections();
      attachmentStatus.load();
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
    const result = await jiraImport.testConnection(jiraUrl, email, apiToken, deploymentType);
    if (result.success) {
      const instanceType = deploymentType === 'datacenter' ? 'Jira Data Center' : 'Jira Cloud';
      addToast({ message: `Connected to ${instanceType} successfully!`, variant: 'success' });
      // Load projects after connecting
      await jiraImport.loadProjects();
      safeNextStep();
    } else {
      addToast({ message: result.error, variant: 'error', title: 'Connection Failed' });
    }
  }

  // Select and use a saved connection
  async function selectSavedConnection(conn) {
    isLoadingSavedConnection = true;
    jiraImport.useSavedConnection(conn);
    const instanceType = conn.deployment_type === 'datacenter' ? 'Jira Data Center' : 'Jira Cloud';
    addToast({ message: `Connected to ${conn.instance_name || instanceType}`, variant: 'success' });
    await jiraImport.loadProjects();
    isLoadingSavedConnection = false;
    safeNextStep();
  }

  // State for loading saved connection
  let isLoadingSavedConnection = $state(false);

  // State for analyzing
  let isAnalyzing = $state(false);

  // State for loading projects when clicking Continue
  let isContinueLoading = $state(false);

  // Navigation lock to prevent double navigation
  let isNavigating = $state(false);

  // Safe navigation that prevents multiple nextStep() calls in the same tick
  async function safeNextStep() {
    if (isNavigating) return;
    isNavigating = true;
    jiraImport.nextStep();
    // Reset after microtask to allow subsequent navigation
    await Promise.resolve();
    isNavigating = false;
  }

  // Handle project selection confirmation
  async function handleAnalyze() {
    isAnalyzing = true;
    const result = await jiraImport.analyzeProjects();
    isAnalyzing = false;
    if (result?.success) {
      safeNextStep();
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
        safeNextStep();
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
      safeNextStep();
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
      return connection.isConnected ? t('jiraImport.buttons.continue') : t('jiraImport.buttons.connect');
    } else if (currentStepId === 'projects') {
      return t('jiraImport.buttons.analyzeAndConfigure');
    } else if (currentStepId === 'mapping') {
      return t('jiraImport.buttons.continue');
    } else if (currentStepId === 'preview') {
      return t('jiraImport.buttons.startImport');
    } else if (currentStepId === 'import') {
      return t('jiraImport.buttons.done');
    }
    return t('jiraImport.buttons.continue');
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
      title={modalTitle}
      subtitle={modalSubtitle}
      icon={deploymentType === 'datacenter' ? Server : Cloud}
      onClose={handleClose}
    />

    <!-- Step indicator -->
    <div class="px-6 w-full py-3 border-b flex items-center overflow-x-auto" style="border-color: var(--ds-border);">
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
              {t(`jiraImport.steps.${step.id}`)}
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
          {#if attachmentStatus.loaded && !attachmentStatus.enabled}
            <div class="flex items-start gap-3 p-4 rounded-lg border" style="border-color: var(--ds-border-warning); background: var(--ds-background-warning-subtle);">
              <Paperclip size={20} class="flex-shrink-0 mt-0.5" style="color: var(--ds-text-warning);" />
              <div>
                <p class="font-medium" style="color: var(--ds-text-warning);">{t('jiraImport.messages.noAttachments')}</p>
                <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                  {t('jiraImport.messages.noAttachmentsDesc')}
                </p>
              </div>
            </div>
          {/if}

          {#if connection.isConnected}
            <!-- Already connected -->
            {@const isDataCenter = connection.deploymentType === 'datacenter'}
            <AlertBox variant="success" message={t('jiraImport.messages.connected', { name: connection.instanceInfo?.display_name || (isDataCenter ? t('jiraImport.deploymentType.datacenter') : t('jiraImport.deploymentType.cloud')) })} />
            <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: {isDataCenter ? 'var(--ds-background-accent-purple-subtler)' : 'var(--ds-background-accent-blue-subtler)'};">
                  {#if isDataCenter}
                    <Server class="w-5 h-5" style="color: var(--ds-text-accent-purple);" />
                  {:else}
                    <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                  {/if}
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <p class="font-medium" style="color: var(--ds-text);">{connection.instanceInfo?.display_name || (isDataCenter ? t('jiraImport.deploymentType.datacenter') : t('jiraImport.deploymentType.cloud'))}</p>
                    <span class="text-xs px-1.5 py-0.5 rounded"
                          style="background: {isDataCenter ? 'var(--ds-background-accent-purple-subtler)' : 'var(--ds-background-accent-blue-subtler)'}; color: {isDataCenter ? 'var(--ds-text-accent-purple)' : 'var(--ds-text-accent-blue)'};">
                      {isDataCenter ? t('jiraImport.deploymentType.datacenter') : t('jiraImport.deploymentType.cloud')}
                    </span>
                  </div>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{connection.email}</p>
                </div>
              </div>
            </div>
          {:else if savedConnections.items.length > 0 && !showNewConnectionForm}
            <!-- Show saved connections -->
            {#if isLoadingSavedConnection}
              <div class="flex flex-col items-center justify-center py-12">
                <Spinner size="lg" />
                <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">{t('projects.loadingProjects')}</p>
              </div>
            {:else}
              <AlertBox variant="info" message={t('jiraImport.messages.selectConnection')} />

              <div class="space-y-3">
                {#each savedConnections.items as conn}
                  {@const isDataCenter = conn.deployment_type === 'datacenter'}
                  <button
                    type="button"
                    class="w-full p-4 rounded-lg border text-left transition-all hover:border-blue-400"
                    style="border-color: var(--ds-border); background: var(--ds-surface);"
                    onclick={() => selectSavedConnection(conn)}
                  >
                    <div class="flex items-center gap-3">
                      <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                           style="background: {isDataCenter ? 'var(--ds-background-accent-purple-subtler)' : 'var(--ds-background-accent-blue-subtler)'};">
                        {#if isDataCenter}
                          <Server class="w-5 h-5" style="color: var(--ds-text-accent-purple);" />
                        {:else}
                          <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                        {/if}
                      </div>
                      <div class="flex-1">
                        <div class="flex items-center gap-2">
                          <p class="font-medium" style="color: var(--ds-text);">
                            {conn.instance_name || (isDataCenter ? t('jiraImport.deploymentType.datacenter') : t('jiraImport.deploymentType.cloud'))}
                          </p>
                          <span class="text-xs px-1.5 py-0.5 rounded"
                                style="background: {isDataCenter ? 'var(--ds-background-accent-purple-subtler)' : 'var(--ds-background-accent-blue-subtler)'}; color: {isDataCenter ? 'var(--ds-text-accent-purple)' : 'var(--ds-text-accent-blue)'};">
                            {isDataCenter ? t('jiraImport.deploymentType.datacenter') : t('jiraImport.deploymentType.cloud')}
                          </span>
                        </div>
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
                  {t('jiraImport.buttons.addNewConnection')}
                </Button>
              </div>
            {/if}
          {:else}
            <!-- New connection form -->
            {#if savedConnections.items.length > 0}
              <div class="flex items-center justify-between mb-4">
                <span class="text-sm font-medium" style="color: var(--ds-text);">{t('connections.createConnection')}</span>
                <Button variant="ghost" size="small" onclick={() => showNewConnectionForm = false}>
                  {t('jiraImport.buttons.useExisting')}
                </Button>
              </div>
            {/if}

            <!-- Deployment Type Selector -->
            <div class="flex gap-2 mb-4">
              <button
                type="button"
                class="flex-1 p-3 rounded-lg border text-left transition-all flex items-center gap-3"
                style="border-color: {deploymentType === 'cloud' ? 'var(--ds-border-focused)' : 'var(--ds-border)'}; background: {deploymentType === 'cloud' ? 'var(--ds-background-selected)' : 'transparent'};"
                onclick={() => deploymentType = 'cloud'}
              >
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-blue-subtler);">
                  <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                </div>
                <div>
                  <p class="font-medium" style="color: var(--ds-text);">{t('jiraImport.deploymentType.cloud')}</p>
                  <p class="text-xs" style="color: var(--ds-text-subtle);">{t('jiraImport.deploymentType.cloudDesc')}</p>
                </div>
                {#if deploymentType === 'cloud'}
                  <Check size={16} class="ml-auto" style="color: var(--ds-text-accent-blue);" />
                {/if}
              </button>
              <button
                type="button"
                class="flex-1 p-3 rounded-lg border text-left transition-all flex items-center gap-3"
                style="border-color: {deploymentType === 'datacenter' ? 'var(--ds-border-focused)' : 'var(--ds-border)'}; background: {deploymentType === 'datacenter' ? 'var(--ds-background-selected)' : 'transparent'};"
                onclick={() => deploymentType = 'datacenter'}
              >
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-purple-subtler);">
                  <Server class="w-5 h-5" style="color: var(--ds-text-accent-purple);" />
                </div>
                <div>
                  <p class="font-medium" style="color: var(--ds-text);">{t('jiraImport.deploymentType.datacenter')}</p>
                  <p class="text-xs" style="color: var(--ds-text-subtle);">{t('jiraImport.deploymentType.datacenterDesc')}</p>
                </div>
                {#if deploymentType === 'datacenter'}
                  <Check size={16} class="ml-auto" style="color: var(--ds-text-accent-purple);" />
                {/if}
              </button>
            </div>

            <AlertBox variant="info" message={deploymentType === 'datacenter'
              ? t('jiraImport.messages.credentialsHelpDatacenter')
              : t('jiraImport.messages.credentialsHelpCloud')} />

            <FormField label={deploymentType === 'datacenter' ? t('jiraImport.form.urlDatacenter') : t('jiraImport.form.urlCloud')} required>
              <Input
                bind:value={jiraUrl}
                placeholder={urlPlaceholder}
                disabled={connection.isConnecting}
              />
            </FormField>

            <FormField label={emailLabel} required>
              <Input
                bind:value={email}
                type={deploymentType === 'datacenter' ? 'text' : 'email'}
                placeholder={emailPlaceholder}
                disabled={connection.isConnecting}
              />
            </FormField>

            <FormField label={tokenLabel} required>
              <div class="relative">
                <Input
                  bind:value={apiToken}
                  type={showToken ? 'text' : 'password'}
                  placeholder={deploymentType === 'datacenter' ? 'Your password or token' : 'Your Jira API token'}
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
                {#if tokenHelpLink}
                  <a href={tokenHelpLink}
                     target="_blank" rel="noopener noreferrer"
                     class="underline hover:no-underline" style="color: var(--ds-link);">
                    {t('jiraImport.form.generateToken')}
                  </a> {t('jiraImport.form.tokenHelpCloud')}
                {:else}
                  {tokenHelpText}
                {/if}
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
              {t('jiraImport.projects.selected', { selected: projects.selected.length, total: projects.available.length })}
            </p>
            <div class="flex gap-2">
              <Button variant="ghost" size="small" onclick={() => jiraImport.selectAllProjects()}>
                {t('jiraImport.buttons.selectAll')}
              </Button>
              <Button variant="ghost" size="small" onclick={() => jiraImport.deselectAllProjects()}>
                {t('jiraImport.buttons.deselectAll')}
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
                {t('jiraImport.projects.openIssuesOnly')}
              </label>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {t('jiraImport.projects.openIssuesOnlyDesc')}
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
                            {t('jiraImport.projects.teamManaged')}
                          </span>
                        {/if}
                      </div>
                      <div class="flex items-center gap-2 mt-1">
                        <span class="text-xs px-1.5 py-0.5 rounded"
                              style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {project.key}
                        </span>
                        <span class="text-xs" style="color: var(--ds-text-subtle);">
                          {t('jiraImport.projects.issues', { count: project.issue_count.toLocaleString() })}
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
              <h3 class="font-medium" style="color: var(--ds-text);">{t('jiraImport.mapping.workspaces')}</h3>
              <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {mappings.workspaces.length}
              </span>
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {t('jiraImport.mapping.workspacesDesc')}
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
              <h3 class="font-medium" style="color: var(--ds-text);">{t('jiraImport.mapping.issueTypes')}</h3>
              <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {mappings.issueTypes.length}
              </span>
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {t('jiraImport.mapping.issueTypesDesc')}
            </p>
            <div class="flex flex-wrap gap-2">
              {#each mappings.issueTypes as mapping}
                <div class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
                     style="border-color: var(--ds-border); background: var(--ds-surface);">
                  <span class="text-sm" style="color: var(--ds-text);">{mapping.jiraName}</span>
                  {#if mapping.isSubtask}
                    <span class="text-xs px-1 py-0.5 rounded"
                          style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                      {t('jiraImport.mapping.subtask')}
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
              <h3 class="font-medium" style="color: var(--ds-text);">{t('jiraImport.mapping.statuses')}</h3>
              <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {mappings.statuses.length}
              </span>
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              {t('jiraImport.mapping.statusesDesc')}
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

          <!-- Versions / Milestones Section -->
          {#if mappings.versions.length > 0}
            <div class="space-y-3">
              <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
                <Flag size={18} style="color: var(--ds-text-accent-teal);" />
                <h3 class="font-medium" style="color: var(--ds-text);">{t('jiraImport.mapping.versions')}</h3>
                <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {mappings.versions.length}
                </span>
              </div>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {t('jiraImport.mapping.versionsDesc')}
              </p>
              <div class="flex flex-wrap gap-2">
                {#each mappings.versions as version}
                  <div class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
                       style="border-color: var(--ds-border); background: var(--ds-surface);">
                    <span class="text-sm" style="color: var(--ds-text);">{version.jiraName}</span>
                    {#if version.released}
                      <span class="text-xs px-1 py-0.5 rounded"
                            style="background: var(--ds-background-success-bold); color: white;">
                        Released
                      </span>
                    {/if}
                    <span class="text-xs px-1 py-0.5 rounded"
                          style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                      {version.projectKey}
                    </span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Custom Fields Section -->
          {#if mappings.customFields.length > 0}
            <div class="space-y-3">
              <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
                <Hash size={18} style="color: var(--ds-text-accent-orange);" />
                <h3 class="font-medium" style="color: var(--ds-text);">{t('jiraImport.mapping.customFields')}</h3>
                <span class="text-xs px-1.5 py-0.5 rounded ml-auto" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {mappings.customFields.filter(f => f.canMap).length} / {mappings.customFields.length}
                </span>
              </div>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {t('jiraImport.mapping.customFieldsDesc')}
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
                          {t('jiraImport.mapping.create')}
                        </span>
                      {:else}
                        <span class="text-xs px-2 py-1 rounded"
                              style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                          {t('jiraImport.mapping.skip')}
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
          <AlertBox variant="warning" message={t('jiraImport.messages.reviewSummary')} />

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
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.workspaces')}</p>
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
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.workItems')}</p>
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
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.statuses')}</p>
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
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.itemTypes')}</p>
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
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.customFields')}</p>
                </div>
              </div>
            </div>

            {#if mappings.versions.length > 0}
              <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                       style="background: var(--ds-background-accent-teal-subtler);">
                    <Flag class="w-5 h-5" style="color: var(--ds-text-accent-teal);" />
                  </div>
                  <div>
                    <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                      {mappings.versions.length}
                    </p>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.milestones')}</p>
                  </div>
                </div>
              </div>
            {/if}

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
                      {t('jiraImport.preview.users')}
                      {#if analysis.result.users.filter(u => !u.matched_user_id).length > 0}
                        <span class="text-xs ml-1" style="color: var(--ds-text-accent-orange);">
                          {t('jiraImport.preview.usersNew', { count: analysis.result.users.filter(u => !u.matched_user_id).length })}
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
                    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.assets')}</p>
                  </div>
                </div>
              </div>
            {/if}
          </div>

          <!-- Project breakdown -->
          <div class="space-y-2">
            <h3 class="font-medium" style="color: var(--ds-text);">{t('jiraImport.preview.projectsToImport')}</h3>
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
                  {t('jiraImport.projects.issues', { count: project.issue_count.toLocaleString() })}
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
                {t('jiraImport.buttons.retryImport')}
              </Button>
            </div>
          {:else if importData.result}
            <div class="text-center">
              <Check class="w-16 h-16 mx-auto" style="color: var(--ds-text-success);" />
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                {t('jiraImport.import.complete')}
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                {t('jiraImport.import.success', { count: importData.progress?.imported_issues || 0 })}
              </p>
              {#if importData.progress?.failed_issues > 0}
                <p class="text-sm mt-1 text-amber-600">
                  {t('jiraImport.import.failed', { count: importData.progress.failed_issues })}
                </p>
              {/if}
            </div>
          {:else if importData.isImporting || importData.jobId}
            <div class="text-center">
              <Spinner size="lg" class="mx-auto" />
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                {t('jiraImport.import.importing')}
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                {importData.phase || t('jiraImport.import.starting')}
              </p>
              {#if importData.progress}
                <div class="mt-4 w-64 mx-auto">
                  <div class="flex justify-between text-xs mb-1" style="color: var(--ds-text-subtle);">
                    <span>{t('jiraImport.import.progress')}</span>
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
                {t('jiraImport.import.ready')}
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                {t('jiraImport.import.readyDesc', { count: analysis.result?.total_issues || 0 })}
              </p>
              <div class="mt-6">
                <Button variant="primary" onclick={() => jiraImport.startImport()}>
                  {t('jiraImport.buttons.startImport')}
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
      loading={connection.isConnecting || isAnalyzing || isContinueLoading || isLoadingSavedConnection}
      disabled={
        connection.isConnecting ||
        isAnalyzing ||
        isContinueLoading ||
        isLoadingSavedConnection ||
        isNavigating ||
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
            {t('jiraImport.buttons.cancel')}
          {:else}
            <ChevronLeft size={16} class="mr-1" />
            {t('jiraImport.buttons.back')}
          {/if}
        </Button>
      {/snippet}
    </DialogFooter>
  </div>
</Modal>
