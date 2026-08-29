<script>
  import { jiraImport } from './JiraImportStore.svelte.js';
  import Modal from '../dialogs/Modal.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Input from '../components/Input.svelte';
  import FormField from '../components/FormField.svelte';
  import {
    Cloud, Server, ChevronRight, ChevronLeft, ArrowRight,
    Briefcase, FileText, Activity, Hash, Box, AlertCircle,
    ExternalLink, Eye, EyeOff, Plus, Users, Paperclip, Flag, Check, X
  } from '@lucide/svelte';
  import Stepper from '../components/Stepper.svelte';
  import { addToast } from '../stores/toasts.svelte.js';
  import { attachmentStatus } from '../stores/attachmentStatus.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import Checkbox from '../components/Checkbox.svelte';
  import Select from '../components/Select.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';

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
  let tokenLabel = $derived(deploymentType === 'datacenter' ? t('jiraImport.form.personalAccessToken') : t('jiraImport.form.apiToken'));
  let tokenHelpText = $derived(deploymentType === 'datacenter'
    ? t('jiraImport.form.tokenHelpDatacenter')
    : t('jiraImport.form.tokenHelpCloud'));
  let tokenHelpLink = $derived(deploymentType === 'datacenter'
    ? null  // No standard link for DC as it varies by instance
    : 'https://id.atlassian.com/manage-profile/security/api-tokens');
  let urlPlaceholder = $derived(deploymentType === 'datacenter'
    ? 'https://jira.your-company.com'
    : 'https://your-domain.atlassian.net');
  // Derived state from store
  let savedConnections = $derived(jiraImport.savedConnections);
  let connection = $derived(jiraImport.connection);
  let modalTitle = $derived(deploymentType === 'datacenter' ? t('jiraImport.title.datacenter') : t('jiraImport.title.cloud'));
  let modalSubtitle = $derived(connection.instanceInfo?.display_name || (deploymentType === 'datacenter' ? t('jiraImport.subtitle.datacenter') : t('jiraImport.subtitle.cloud')));
  let projects = $derived(jiraImport.projects);
  let analysis = $derived(jiraImport.analysis);
  let xray = $derived(jiraImport.xray);
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

  // After loadProjects(), check whether Jira rejected us upstream and surface
  // it instead of advancing into an empty Projects step. JIRA_AUTH_FAILED is
  // unrecoverable from here (the saved token is bad), so we stay on Connect
  // and toast; other codes (rate-limit, generic upstream) advance so the
  // user can see the persistent banner and retry.
  function reportProjectsLoadOutcome() {
    const err = projects.error;
    if (!err) return { ok: true, advance: true };
    addToast({
      message: err.message,
      variant: 'error',
      title: err.code === 'JIRA_AUTH_FAILED' ? 'Reconnect required' : 'Jira request failed',
    });
    return { ok: false, advance: err.code !== 'JIRA_AUTH_FAILED' };
  }

  // Handle connection test (new connection)
  async function handleConnect() {
    const result = await jiraImport.testConnection(jiraUrl, email, apiToken, deploymentType);
    if (result.success) {
      const instanceType = deploymentType === 'datacenter' ? 'Jira Data Center' : 'Jira Cloud';
      addToast({ message: `Connected to ${instanceType} successfully!`, variant: 'success' });
      // Load projects after connecting
      await jiraImport.loadProjects();
      const outcome = reportProjectsLoadOutcome();
      if (outcome.advance) safeNextStep();
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
    const outcome = reportProjectsLoadOutcome();
    if (outcome.advance) safeNextStep();
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
          const outcome = reportProjectsLoadOutcome();
          if (!outcome.advance) return;
        }
        safeNextStep();
      } else {
        handleConnect();
      }
    } else if (currentStepId === 'projects') {
      handleAnalyze();
    } else if (currentStepId === 'preview') {
      await jiraImport.startImport();
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
  <div class="flex flex-col max-h-[90vh]" data-testid="jira-import-wizard">
    <!-- Header -->
    <div
      class="flex items-center gap-2.5 border-b px-5 py-3"
      style="border-color: var(--ds-border);"
    >
      {#if deploymentType === 'datacenter'}
        <Server class="h-5 w-5 shrink-0" style="color: var(--ds-interactive);" />
      {:else}
        <Cloud class="h-5 w-5 shrink-0" style="color: var(--ds-interactive);" />
      {/if}
      <div class="min-w-0 flex-1">
        <h2 class="truncate text-base font-semibold leading-5" style="color: var(--ds-text);">
          {modalTitle}
        </h2>
        {#if modalSubtitle}
          <p class="mt-0.5 truncate text-xs leading-4" style="color: var(--ds-text-subtle);">
            {modalSubtitle}
          </p>
        {/if}
      </div>
      <button
        type="button"
        data-testid="jira-import-close"
        class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)] focus:outline-none focus:ring-2 focus:ring-[var(--ds-border-focused)]"
        style="color: var(--ds-text-subtle);"
        onclick={handleClose}
        aria-label={t('aria.close')}
      >
        <X class="h-[18px] w-[18px]" />
      </button>
    </div>

    <!-- Step indicator -->
    <div class="px-6 w-full py-3 border-b overflow-x-auto" style="border-color: var(--ds-border);">
      <Stepper
        {steps}
        currentStep={currentStep + 1}
        showLabels={true}
        size="small"
        getLabel={(step) => t(`jiraImport.steps.${step.id}`)}
      />
    </div>

    <!-- Content area -->
    <div class="p-6 overflow-y-auto flex-1 min-h-0" data-testid={`jira-import-step-${currentStepId}`}>
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
                  {#if !isDataCenter && connection.email}
                    <p class="text-sm" style="color: var(--ds-text-subtle);">{connection.email}</p>
                  {/if}
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
                        {#if !isDataCenter && conn.email}
                          <p class="text-sm" style="color: var(--ds-text-subtle);">{conn.email}</p>
                        {/if}
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
                data-testid="jira-import-deployment-cloud"
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
                data-testid="jira-import-deployment-datacenter"
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
                dataTestid="jira-import-url"
                placeholder={urlPlaceholder}
                disabled={connection.isConnecting}
              />
            </FormField>

            {#if deploymentType === 'cloud'}
              <FormField label={t('jiraImport.form.email')} required>
                <Input
                  bind:value={email}
                  dataTestid="jira-import-email"
                  type="email"
                  placeholder="your.email@company.com"
                  disabled={connection.isConnecting}
                />
              </FormField>
            {/if}

            <FormField label={tokenLabel} required>
              <div class="relative">
                <Input
                  bind:value={apiToken}
                  dataTestid="jira-import-api-token"
                  type={showToken ? 'text' : 'password'}
                  placeholder={deploymentType === 'datacenter' ? 'Your Jira personal access token' : 'Your Jira API token'}
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
              <DescriptionText>
                {#if tokenHelpLink}
                  <a href={tokenHelpLink}
                     target="_blank" rel="noopener noreferrer"
                     class="underline hover:no-underline" style="color: var(--ds-link);">
                    {t('jiraImport.form.generateToken')}
                  </a> {t('jiraImport.form.tokenHelpCloud')}
                {:else}
                  {tokenHelpText}
                {/if}
              </DescriptionText>
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
          <div class="p-3 rounded-lg border"
               style="border-color: var(--ds-border); background: var(--ds-surface);">
            <Checkbox
              checked={projects.openIssuesOnly}
              dataTestid="jira-import-open-issues-only"
              onchange={async () => {
                jiraImport.toggleOpenIssuesOnly();
                await jiraImport.reloadProjectsWithFilter();
              }}
              label={t('jiraImport.projects.openIssuesOnly')}
              hint={t('jiraImport.projects.openIssuesOnlyDesc')}
              size="small"
            />
          </div>

          <Input
            bind:value={projectSearch}
            dataTestid="jira-import-project-search"
            placeholder="Search projects..."
          />

          {#if projects.error && projects.available.length === 0}
            <AlertBox variant="error" class="mb-4">
              <div class="flex items-start gap-3 w-full">
                <p class="flex-1 font-medium">{projects.error.message}</p>
                {#if projects.error.code === 'JIRA_AUTH_FAILED'}
                  <Button
                    variant="primary"
                    size="small"
                    onclick={() => jiraImport.goToStepId('connect')}
                  >
                    Back to Connect
                  </Button>
                {:else}
                  <Button
                    variant="secondary"
                    size="small"
                    onclick={() => jiraImport.loadProjects()}
                  >
                    Retry
                  </Button>
                {/if}
              </div>
            </AlertBox>
          {/if}

          {#if projects.isLoading}
            <div class="flex items-center justify-center py-12">
              <Spinner size="lg" />
            </div>
          {:else}
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3 max-h-96 overflow-y-auto">
              {#each filteredProjects as project}
                {@const isSelected = projects.selected.includes(project.key)}
                <button
                  type="button"
                  data-testid={`jira-import-project-${project.key}`}
                  data-project-type={project.project_type}
                  data-team-managed={project.is_team_managed ? 'true' : 'false'}
                  data-configuration-mode={project.is_team_managed ? 'conservative' : 'authoritative'}
                  data-issue-count={project.issue_count ?? ''}
                  class="p-4 rounded-lg border text-left transition-all"
                  style="border-color: {isSelected ? 'var(--ds-border-focused)' : 'var(--ds-border)'}; background: {isSelected ? 'var(--ds-background-selected)' : 'transparent'};"
                  onclick={() => jiraImport.toggleProject(project.key)}
                >
                  <div class="flex items-start gap-3">
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <div onclick={(e) => e.stopPropagation()} class="mt-1">
                      <Checkbox
                        checked={isSelected}
                        onchange={() => jiraImport.toggleProject(project.key)}
                        size="small"
                      />
                    </div>
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        {#if project.avatar_url}
                          <img src={project.avatar_url} alt="" class="w-6 h-6 rounded" />
                        {/if}
                        <span class="font-medium truncate" style="color: var(--ds-text);">
                          {project.name}
                        </span>
                        {#if project.is_team_managed}
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
                          {project.issue_count == null
                            ? '…'
                            : t('jiraImport.projects.issues', { count: project.issue_count.toLocaleString() })}
                        </span>
                      </div>
                    </div>
                  </div>
                </button>
              {/each}
            </div>
          {/if}
        </div>

      {:else if currentStepId === 'xray'}
        <div class="space-y-5" data-testid="jira-import-xray-options">
          <div
            class="p-5 rounded-lg border"
            style="border-color: var(--ds-border); background: var(--ds-surface);"
          >
            <div class="flex items-start gap-3">
              <Check size={22} style="color: var(--ds-text-success);" class="mt-0.5 flex-shrink-0" />
              <div class="space-y-1">
                <h3 class="font-medium" style="color: var(--ds-text);">
                  Xray test cases found
                </h3>
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  Windshift positively identified {xray.totalTests.toLocaleString()} Xray
                  {xray.totalTests === 1 ? ' Test' : ' Tests'} in the selected projects using
                  Xray-owned metadata.
                </p>
              </div>
            </div>
          </div>

          <Checkbox
            checked={xray.importTests}
            dataTestid="jira-import-xray-enabled"
            onchange={(enabled) => xray.importTests = enabled}
            label="Import Xray Tests into Windshift Test Management"
            hint="When enabled, Xray Test issues become test cases instead of ordinary work items. Leave off to use the normal issue mapping."
          />

          {#if xray.importTests && xray.requiresCredential}
            <div
              class="space-y-4 p-4 rounded-lg border"
              style="border-color: var(--ds-border); background: var(--ds-background-neutral-subtle);"
              data-testid="jira-import-xray-cloud-credentials"
            >
              <AlertBox variant="info">
                <p class="text-sm">
                  Xray Cloud stores test steps outside Jira. Supply an Xray API client ID and
                  client secret; these credentials are used only for this import.
                </p>
              </AlertBox>

              <FormField label="Xray Cloud region">
                <Select
                  id="jira-import-xray-region"
                  value={xray.region}
                  options={[
                    { value: 'global', label: 'Global' },
                    { value: 'us', label: 'United States' },
                    { value: 'eu', label: 'European Union' },
                    { value: 'au', label: 'Australia' },
                  ]}
                  onchange={(value) => xray.region = value}
                />
              </FormField>

              <FormField label="Xray client ID">
                <Input
                  bind:value={xray.clientId}
                  dataTestid="jira-import-xray-client-id"
                  autocomplete="off"
                  placeholder="Client ID"
                />
              </FormField>

              <FormField label="Xray client secret">
                <Input
                  type="password"
                  bind:value={xray.clientSecret}
                  dataTestid="jira-import-xray-client-secret"
                  autocomplete="new-password"
                  placeholder="Client secret"
                />
              </FormField>
            </div>
          {:else if xray.importTests}
            <AlertBox variant="info">
              <p class="text-sm">
                Xray Data Center test definitions will be read through Raven using the existing
                Jira credentials.
              </p>
            </AlertBox>
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
                <div
                  class="p-3 rounded-lg border"
                  data-testid={`jira-import-workspace-mapping-${mapping.jiraKey}`}
                  style="border-color: var(--ds-border); background: var(--ds-surface);"
                >
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
                        dataTestid={`jira-import-workspace-name-${mapping.jiraKey}`}
                        placeholder="Workspace name"
                        size="small"
                      />
                    </div>
                  </div>
                  {#if mapping.workspaceKeyCollisionFound}
                    <div
                      class="mt-3 space-y-3"
                      data-testid={`jira-import-workspace-key-collision-${mapping.jiraKey}`}
                    >
                      <AlertBox variant="warning">
                        <p class="text-sm">
                          Workspace key <strong>{mapping.jiraKey}</strong> is already in use.
                          This Jira project will be created as a separate workspace with the
                          alias <strong>{mapping.newWorkspaceKey}</strong>. The existing
                          workspace will not be changed or merged.
                        </p>
                      </AlertBox>
                      <Checkbox
                        checked={mapping.keyAliasAcknowledged}
                        dataTestid={`jira-import-workspace-key-alias-ack-${mapping.jiraKey}`}
                        onchange={(checked) =>
                          jiraImport.setWorkspaceKeyAliasAcknowledged(mapping.jiraKey, checked)}
                        label={`Use ${mapping.newWorkspaceKey} for this imported Jira workspace`}
                        hint="I acknowledge that links and imported work-item keys will use this workspace alias."
                        size="small"
                      />
                    </div>
                  {:else}
                    <p class="mt-2 text-xs" style="color: var(--ds-text-subtle);">
                      Windshift workspace key: <strong>{mapping.newWorkspaceKey}</strong>
                    </p>
                  {/if}
                  {#if mapping.isTeamManaged}
                    <div
                      class="mt-3"
                      data-testid={`jira-import-team-managed-limits-${mapping.jiraKey}`}
                    >
                      <AlertBox variant="warning">
                        <p class="text-sm">
                          Issue data and observed custom fields will import. Jira does not expose
                          company-managed workflow and screen schemes for this team-managed project,
                          so Windshift will create a conservative initial workflow and default screens
                          and report that configuration boundary.
                        </p>
                      </AlertBox>
                    </div>
                  {/if}
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
                <div data-testid="jira-import-issue-type-mapping"
                     class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
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
                <div data-testid="jira-import-status-mapping"
                     class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
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
                  <div data-testid="jira-import-version-mapping"
                       class="px-3 py-1.5 rounded-lg border inline-flex items-center gap-2"
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
                  <div data-testid="jira-import-custom-field-mapping"
                       data-mapping-action={mapping.canMap ? 'create' : 'skip'}
                       class="p-2 rounded-lg border flex items-center gap-3"
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
                      {#if mapping.canMap && mapping.windshiftType === 'asset'}
                        <div class="w-64" data-testid="jira-import-asset-field-mapping" data-jira-field-id={mapping.jiraId}>
                          <Select
                            id={`jira-import-asset-field-schema-${mapping.jiraId}`}
                            value={mapping.assetSchemaId}
                            options={[
                              { value: 'auto', label: 'Detect from issue values' },
                              ...(analysis.result?.asset_schemas || []).map((schema) => ({
                                value: schema.id,
                                label: schema.set_name,
                              })),
                              { value: 'text', label: 'Preserve as text' },
                            ]}
                            onchange={(value) => jiraImport.setAssetFieldSchema(mapping.jiraId, value)}
                            size="small"
                          />
                        </div>
                      {:else if mapping.canMap}
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
                  {#if mapping.canMap && mapping.windshiftType === 'asset'}
                    <p class="px-2 text-xs" style="color: var(--ds-text-subtle);">
                      Jira configures this field against one Assets schema even when it is used by multiple projects.
                      Automatic detection checks every populated imported issue; choose a schema explicitly when the field is empty.
                    </p>
                  {/if}
                {/each}
              </div>
            </div>
          {/if}

          {#if analysis.result?.asset_schemas?.length > 0}
            <div
              class="space-y-3 p-4 rounded-lg border"
              data-testid="jira-import-assets-mapping"
              style="border-color: var(--ds-border); background: var(--ds-surface);"
            >
              <div class="flex items-center gap-2">
                <Box size={18} style="color: var(--ds-text-accent-yellow);" />
                <h3 class="font-medium" style="color: var(--ds-text);">Jira Assets schemas</h3>
                <span
                  class="text-xs px-1.5 py-0.5 rounded ml-auto"
                  style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                >
                  {analysis.result.asset_schemas.length}
                </span>
              </div>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                Each accessible Jira Assets schema becomes a Windshift asset set. Object types, attributes, and objects are recreated inside that set.
              </p>
              <div class="space-y-2">
                {#each analysis.result.asset_schemas as schema}
                  <div
                    class="flex items-center justify-between px-3 py-2 rounded"
                    data-testid="jira-import-asset-schema"
                    data-schema-name={schema.name}
                    data-schema-key={schema.key}
                    data-set-name={schema.set_name}
                    data-object-count={schema.object_count}
                    data-type-count={schema.type_count}
                    style="background: var(--ds-background-neutral);"
                  >
                    <span class="text-sm font-medium" style="color: var(--ds-text);">
                      {schema.name}
                      {#if schema.key}
                        <span class="text-xs font-normal ml-1" style="color: var(--ds-text-subtle);">({schema.key})</span>
                      {/if}
                    </span>
                    <span class="text-xs" style="color: var(--ds-text-subtle);">
                      {schema.object_count} {schema.object_count === 1 ? 'object' : 'objects'}
                      · {schema.type_count} {schema.type_count === 1 ? 'type' : 'types'}
                    </span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          {#if analysis.result?.service_management_projects?.length > 0}
            <div
              class="space-y-3 p-4 rounded-lg border"
              data-testid="jira-import-service-management"
              style="border-color: var(--ds-border); background: var(--ds-surface);"
            >
              <div class="flex items-center gap-2">
                <Users size={18} style="color: var(--ds-text-accent-blue);" />
                <h3 class="font-medium" style="color: var(--ds-text);">Jira Service Management portals</h3>
              </div>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                Each service project becomes a Windshift portal. Its request types are created before requests and portal customers are imported.
              </p>
              <div class="space-y-2">
                {#each analysis.result.service_management_projects as project}
                  <div
                    class="flex items-center justify-between px-3 py-2 rounded"
                    data-testid="jira-import-service-management-project"
                    data-project-key={project.project_key}
                    data-request-type-count={project.request_type_count}
                    style="background: var(--ds-background-neutral);"
                  >
                    <span class="text-sm font-medium" style="color: var(--ds-text);">{project.project_key}</span>
                    <span class="text-xs" style="color: var(--ds-text-subtle);">
                      {project.request_type_count} request {project.request_type_count === 1 ? 'type' : 'types'}
                    </span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          {#if analysis.result?.service_management_projects?.some(project => project.organization_count > 0)}
            {@const serviceProjects = analysis.result.service_management_projects}
            {@const organizationCount = serviceProjects.reduce((total, project) => total + project.organization_count, 0)}
            {@const organizationMemberCount = serviceProjects.reduce((total, project) => total + project.organization_member_count, 0)}
            <div
              class="space-y-3 p-4 rounded-lg border"
              data-testid="jira-import-service-management-organizations"
              data-organization-count={organizationCount}
              data-organization-member-count={organizationMemberCount}
              style="border-color: var(--ds-border); background: var(--ds-surface);"
            >
              <div class="flex items-center gap-2">
                <Users size={18} style="color: var(--ds-text-accent-blue);" />
                <h3 class="font-medium" style="color: var(--ds-text);">Jira Service Management organizations</h3>
                <span
                  class="text-xs px-1.5 py-0.5 rounded ml-auto"
                  style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                >
                  {organizationCount}
                </span>
              </div>
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                We found {organizationCount} customer {organizationCount === 1 ? 'organization' : 'organizations'}
                with {organizationMemberCount} member {organizationMemberCount === 1 ? 'account' : 'accounts'}.
                Windshift portal customers are imported either way.
              </p>
              <Checkbox
                checked={mappings.serviceManagement.importOrganizations}
                dataTestid="jira-import-import-organizations"
                onchange={(checked) => jiraImport.setImportServiceManagementOrganizations(checked)}
                label="Create Windshift customer organizations"
                hint="Also assign imported portal customers to their Jira Service Management organization. Leave off to import customers without organizations."
                size="small"
              />
              <div class="flex flex-wrap gap-2">
                {#each serviceProjects.flatMap(project => project.organizations) as organization}
                  <span
                    class="text-xs px-2 py-1 rounded"
                    data-testid="jira-import-discovered-organization"
                    style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                  >
                    {organization.name} ({organization.customer_count})
                  </span>
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
            <div data-testid="jira-import-preview-workspaces" class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
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

            <div data-testid="jira-import-preview-items" class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-green-subtler);">
                  <FileText class="w-5 h-5" style="color: var(--ds-text-accent-green);" />
                </div>
                <div>
                  <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                    {Math.max(
                      0,
                      (analysis.result?.total_issues || 0) - (xray.importTests ? xray.totalTests : 0)
                    ).toLocaleString()}
                  </p>
                  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('jiraImport.preview.workItems')}</p>
                </div>
              </div>
            </div>

            {#if xray.importTests}
              <div
                data-testid="jira-import-preview-xray-tests"
                class="p-4 rounded-lg border"
                style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
              >
                <div class="flex items-center gap-3">
                  <div
                    class="w-10 h-10 rounded-lg flex items-center justify-center"
                    style="background: var(--ds-background-accent-green-subtler);"
                  >
                    <Check class="w-5 h-5" style="color: var(--ds-text-accent-green);" />
                  </div>
                  <div>
                    <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                      {xray.totalTests.toLocaleString()}
                    </p>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">Xray test cases</p>
                  </div>
                </div>
              </div>
            {/if}

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

            {#if analysis.result?.service_management_projects?.some(project => project.organization_count > 0)}
              <div
                data-testid="jira-import-preview-organizations"
                data-import-enabled={mappings.serviceManagement.importOrganizations ? 'true' : 'false'}
                class="p-4 rounded-lg border"
                style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
              >
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                       style="background: var(--ds-background-accent-blue-subtler);">
                    <Users class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                  </div>
                  <div>
                    <p class="text-2xl font-semibold" style="color: var(--ds-text);">
                      {mappings.serviceManagement.importOrganizations
                        ? analysis.result.service_management_projects.reduce((total, project) => total + project.organization_count, 0)
                        : 0}
                    </p>
                    <p class="text-sm" style="color: var(--ds-text-subtle);">
                      Customer organizations
                    </p>
                  </div>
                </div>
              </div>
            {/if}

            {#if analysis.result?.total_assets > 0}
              <div
                data-testid="jira-import-preview-assets"
                data-asset-count={analysis.result.total_assets}
                class="p-4 rounded-lg border"
                style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
              >
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
              <div data-testid={`jira-import-preview-project-${project.key}`}
                   class="p-3 rounded-lg border flex items-center justify-between"
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
                  {project.issue_count == null
                    ? '…'
                    : t('jiraImport.projects.issues', { count: project.issue_count.toLocaleString() })}
                </span>
              </div>
            {/each}
          </div>
        </div>

      {:else if currentStepId === 'import'}
        <!-- Import Step -->
        <div class="flex flex-col items-center justify-center py-12">
          {#if importData.error}
            {#if importData.errorCode === 'JIRA_IMPORT_CONFLICT'}
              <div class="w-full max-w-2xl space-y-4">
                <AlertBox variant="warning" message={importData.error} />
                <div class="rounded-lg border p-4 text-left space-y-3" style="border-color: var(--ds-border); background: var(--ds-surface);">
                  <div class="flex items-start gap-3">
                    <AlertCircle class="w-5 h-5 mt-0.5 flex-shrink-0" style="color: var(--ds-text-warning);" />
                    <div>
                      <p class="font-medium" style="color: var(--ds-text);">Previous Jira import data exists</p>
                      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                        Re-import updates issues matched by their stable Jira ID, reuses imported comments, worklogs, and attachments, and keeps Jira Rank order. You can still close the wizard and delete the previous import data from Import History for a clean replacement.
                      </p>
                    </div>
                  </div>
                  {#if importData.conflictingImports?.length}
                    <div class="space-y-2">
                      {#each importData.conflictingImports as conflict}
                        <div
                          class="rounded border px-3 py-2"
                          data-testid={`jira-import-conflict-${conflict.job_id}`}
                          data-configuration-drift={conflict.configuration_drift ? 'true' : 'false'}
                          style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
                        >
                          <div class="flex items-center justify-between gap-3">
                            <span class="font-mono text-xs" style="color: var(--ds-text);">{conflict.job_id}</span>
                            <span class="text-xs capitalize" style="color: var(--ds-text-subtle);">{conflict.status?.replaceAll('_', ' ')}</span>
                          </div>
                          {#if conflict.project_keys?.length}
                            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">Projects: {conflict.project_keys.join(', ')}</p>
                          {/if}
                          {#if conflict.configuration_drift}
                            <p class="text-xs mt-1" style="color: var(--ds-text-warning);">
                              The selected scope or mapping configuration has changed since this import.
                            </p>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
                <div class="flex justify-center gap-3">
                  <Button
                    variant="primary"
                    dataTestid="jira-import-force-reimport"
                    onclick={() => jiraImport.startImport(true)}
                  >
                    Re-import and update
                  </Button>
                  <Button variant="secondary" onclick={handleClose}>Close and manage imports</Button>
                  <Button variant="ghost" onclick={() => jiraImport.goToStepId('preview')}>Back to preview</Button>
                </div>
              </div>
            {:else}
              <div data-testid="jira-import-error">
                <AlertBox variant="error" message={importData.error} />
              </div>
              <div class="mt-4">
                <Button variant="secondary" onclick={() => jiraImport.startImport()}>
                  {t('jiraImport.buttons.retryImport')}
                </Button>
              </div>
            {/if}
          {:else if importData.result}
            <div
              class="text-center"
              data-testid="jira-import-complete"
              data-total-issues={importData.progress?.total_issues || 0}
              data-imported-issues={importData.progress?.imported_issues || 0}
              data-total-comments={importData.progress?.total_comments || 0}
              data-imported-comments={importData.progress?.imported_comments || 0}
              data-total-attachments={importData.progress?.total_attachments || 0}
              data-imported-attachments={importData.progress?.imported_attachments || 0}
            >
              {#if importData.result.status === 'completed_with_errors'}
                <AlertCircle class="w-16 h-16 mx-auto" style="color: var(--ds-text-warning);" />
              {:else}
                <Check class="w-16 h-16 mx-auto" style="color: var(--ds-text-success);" />
              {/if}
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                {importData.result.status === 'completed_with_errors'
                  ? t('jiraImport.import.completeWithErrors')
                  : t('jiraImport.import.complete')}
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                {t('jiraImport.import.success', { count: importData.progress?.imported_issues || 0 })}
              </p>
              {#if xray.importTests}
                <p
                  class="text-sm mt-1"
                  style="color: var(--ds-text-subtle);"
                  data-testid="jira-import-xray-imported-count"
                >
                  Imported {importData.progress?.imported_tests || 0} Xray test
                  {(importData.progress?.imported_tests || 0) === 1 ? ' case' : ' cases'}.
                </p>
              {/if}
              {#if importData.progress?.failed_issues > 0}
                <p class="text-sm mt-1 text-amber-600" data-testid="jira-import-failed-count">
                  {t('jiraImport.import.failed', { count: importData.progress.failed_issues })}
                </p>
              {/if}
              {#if importData.progress?.failed_tests > 0}
                <p class="text-sm mt-1 text-amber-600" data-testid="jira-import-xray-failed-count">
                  {importData.progress.failed_tests} Xray test
                  {importData.progress.failed_tests === 1 ? ' case failed' : ' cases failed'} to import.
                </p>
              {/if}
              {#if importData.progress?.failed_projects > 0}
                <p class="text-sm mt-1 text-amber-600" data-testid="jira-import-failed-project-count">
                  {importData.progress.failed_projects} Jira
                  {importData.progress.failed_projects === 1 ? ' project failed' : ' projects failed'} to import.
                </p>
              {/if}
              {#if importData.progress?.failed_links > 0}
                <p class="text-sm mt-1 text-amber-600" data-testid="jira-import-failed-link-count">
                  Jira issue links could not be imported.
                </p>
              {/if}
            </div>
          {:else if importData.isImporting || importData.jobId}
            <div class="text-center" data-testid="jira-import-progress">
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
                {#if xray.importTests}
                  <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                    Xray tests: {importData.progress.imported_tests || 0} /
                    {importData.progress.total_tests || 0}
                  </p>
                {/if}
              {/if}
            </div>
          {:else}
            <div class="text-center">
              <FileText class="w-16 h-16 mx-auto" style="color: var(--ds-text-subtle);" />
              <p class="text-lg font-medium mt-4" style="color: var(--ds-text);">
                {t('jiraImport.import.ready')}
              </p>
              <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">
                {t('jiraImport.import.readyDesc', {
                  count: Math.max(
                    0,
                    (analysis.result?.total_issues || 0) - (xray.importTests ? xray.totalTests : 0)
                  )
                })}
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
      confirmTestid="jira-import-next"
      confirmLabel={getConfirmLabel()}
      showKeyboardHint={true}
      loading={connection.isConnecting || isAnalyzing || isContinueLoading || isLoadingSavedConnection}
      disabled={
        connection.isConnecting ||
        isAnalyzing ||
        isContinueLoading ||
        isLoadingSavedConnection ||
        isNavigating ||
        (currentStepId === 'connect' && !connection.isConnected &&
          (!jiraUrl || !apiToken || (deploymentType === 'cloud' && !email))) ||
        (currentStepId === 'projects' && projects.selected.length === 0) ||
        (currentStepId === 'xray' && !jiraImport.canProceed()) ||
        (currentStepId === 'mapping' && !jiraImport.canProceed())
      }
      onConfirm={shouldShowConfirmButton() ? handleNext : null}
    >
      {#snippet extra()}
        <Button
          variant="ghost"
          dataTestid="jira-import-back"
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
