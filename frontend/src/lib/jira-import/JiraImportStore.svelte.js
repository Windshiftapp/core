// Jira Import Store - State management for the import wizard
// Uses Svelte 5 runes for reactivity

import { api } from '../api.js';
import { createWizardNavigation } from '../utils/wizardNavigation.js';

function defaultJiraWizardSteps(includeXray = false) {
  const steps = [
    { id: 'connect', label: 'Connect', completed: false },
    { id: 'projects', label: 'Projects', completed: false },
  ];
  if (includeXray) {
    steps.push({ id: 'xray', label: 'Xray', completed: false });
  }
  steps.push(
    { id: 'mapping', label: 'Mapping', completed: false },
    { id: 'preview', label: 'Preview', completed: false },
    { id: 'import', label: 'Import', completed: false }
  );
  return steps;
}

function wizardStepIndex(stepId) {
  return wizardState.steps.findIndex((step) => step.id === stepId);
}

function markWizardStepComplete(stepId) {
  const index = wizardStepIndex(stepId);
  if (index >= 0) wizardState.steps[index].completed = true;
}

function goToWizardStep(stepId) {
  const index = wizardStepIndex(stepId);
  if (index >= 0) wizardState.currentStep = index;
}

// toProjectError normalizes whatever shape fetchAPI threw into a stable
// { message, code, status } the wizard renders. The `code` is the upstream
// classification (JIRA_AUTH_FAILED / JIRA_FORBIDDEN / JIRA_RATE_LIMITED /
// JIRA_UPSTREAM_ERROR) when the backend identified a Jira-side failure;
// it's null for everything else.
function toProjectError(err) {
  /** @type {any} */
  const e = err || {};
  return {
    message: e.message || 'Failed to load Jira projects',
    code: e.code || e.errorCode || null,
    status: e.status || null,
  };
}

// Saved connections list (for management page)
let savedConnectionsState = $state({
  items: [],
  isLoading: false,
  error: null,
});

// Import jobs list (for management page)
let importJobsState = $state({
  items: [],
  isLoading: false,
  error: null,
});

// Connection state
let connectionState = $state({
  jiraUrl: '',
  email: '',
  apiToken: '',
  deploymentType: 'cloud', // 'cloud' or 'datacenter'
  connectionId: null,
  instanceInfo: null,
  isConnecting: false,
  isConnected: false,
  error: null,
});

// Projects state
let projectsState = $state({
  available: [],
  selected: [],
  openIssuesOnly: false,
  isLoading: false,
  isLoadingCounts: false,
  error: null,
});

// Analysis state
let analysisState = $state({
  isAnalyzing: false,
  result: null,
  error: null,
});

// Xray choices are intentionally ephemeral. Jira connections remain reusable,
// while the Xray Cloud client secret is held only for this wizard/import.
let xrayState = $state({
  available: false,
  detectionStatus: 'not_detected',
  totalTests: 0,
  projects: [],
  testIssueTypeIds: [],
  requiresCredential: false,
  importTests: false,
  region: 'global',
  clientId: '',
  clientSecret: '',
});

// Mappings state
let mappingsState = $state({
  workspaces: [], // { jiraKey, jiraName, createNew, newWorkspaceName, newWorkspaceKey, workspaceKeyCollisionFound, keyAliasAcknowledged }
  issueTypes: [], // { jiraIds[], jiraName, isSubtask, hierarchyLevel, windshiftId, createNew } - deduplicated by name
  statuses: [], // { jiraIds[], jiraName, categoryKey, categoryName, color, windshiftId, createNew } - deduplicated by name
  customFields: [], // { jiraId, jiraName, windshiftType, action, windshiftId }
  versions: [], // { jiraId, jiraName, projectKey, released, releaseDate, createNew }
  serviceManagement: {
    importOrganizations: false,
  },
});

// Import state
let importState = $state({
  isImporting: false,
  jobId: null,
  phase: 'idle',
  progress: null,
  error: null,
  errorCode: null,
  conflictingImports: [],
  result: null,
});

// Wizard state
let wizardState = $state({
  currentStep: 0,
  steps: defaultJiraWizardSteps(),
});

// Export reactive getters
export const jiraImport = {
  // Getters for reactive access
  get savedConnections() {
    return savedConnectionsState;
  },
  get importJobs() {
    return importJobsState;
  },
  get connection() {
    return connectionState;
  },
  get projects() {
    return projectsState;
  },
  get analysis() {
    return analysisState;
  },
  get xray() {
    return xrayState;
  },
  get mappings() {
    return mappingsState;
  },
  get import() {
    return importState;
  },
  get wizard() {
    return wizardState;
  },

  // Load saved connections
  async loadSavedConnections() {
    savedConnectionsState.isLoading = true;
    savedConnectionsState.error = null;

    try {
      const connections = await api.jiraImport.getConnections();
      savedConnectionsState.items = connections;
    } catch (err) {
      savedConnectionsState.error = err.message || 'Failed to load connections';
    } finally {
      savedConnectionsState.isLoading = false;
    }
  },

  // Load import jobs
  async loadImportJobs() {
    importJobsState.isLoading = true;
    importJobsState.error = null;

    try {
      const jobs = await api.jiraImport.getImportJobs();
      importJobsState.items = jobs;
    } catch (err) {
      importJobsState.error = err.message || 'Failed to load import jobs';
    } finally {
      importJobsState.isLoading = false;
    }
  },

  // Delete a saved connection
  async deleteSavedConnection(connectionId) {
    try {
      await api.jiraImport.deleteConnection(connectionId);
      savedConnectionsState.items = savedConnectionsState.items.filter(
        (c) => c.id !== connectionId
      );
      return { success: true };
    } catch (err) {
      return { success: false, error: err.message || 'Failed to delete connection' };
    }
  },

  async deleteImportedData(jobId, confirmation) {
    try {
      await api.jiraImport.deleteImportedData(jobId, confirmation);
      await this.loadImportJobs();
      return { success: true };
    } catch (err) {
      return { success: false, error: err.message || 'Failed to delete imported data' };
    }
  },

  // Use a saved connection (for wizard)
  useSavedConnection(connection) {
    connectionState.connectionId = connection.id;
    connectionState.jiraUrl = connection.instance_url;
    connectionState.email = connection.email;
    connectionState.deploymentType = connection.deployment_type || 'cloud';
    connectionState.instanceInfo = { display_name: connection.instance_name };
    connectionState.isConnected = true;
    markWizardStepComplete('connect');
  },

  // Connection methods
  async testConnection(url, email, token, deploymentType = 'cloud') {
    connectionState.isConnecting = true;
    connectionState.error = null;

    const connectionEmail = deploymentType === 'datacenter' ? '' : email;

    try {
      const response = await api.jiraImport.testConnection({
        instance_url: url,
        email: connectionEmail,
        api_token: token,
        deployment_type: deploymentType,
      });

      connectionState.connectionId = response.connection_id;
      connectionState.instanceInfo = response.instance_info;
      connectionState.jiraUrl = url;
      connectionState.email = connectionEmail;
      connectionState.apiToken = token;
      connectionState.deploymentType = deploymentType;
      connectionState.isConnected = true;
      markWizardStepComplete('connect');

      return { success: true };
    } catch (err) {
      connectionState.error = err.message || 'Failed to connect to Jira';
      return { success: false, error: connectionState.error };
    } finally {
      connectionState.isConnecting = false;
    }
  },

  // Set deployment type
  setDeploymentType(type) {
    connectionState.deploymentType = type;
  },

  // Load project metadata once and refresh counts independently. Structured
  // errors let the wizard branch on codes before advancing.
  async loadProjects() {
    if (!connectionState.connectionId) return;

    projectsState.isLoading = true;
    projectsState.error = null;
    projectsState.available = [];

    try {
      const projects = await api.jiraImport.getProjects(connectionState.connectionId);
      projectsState.available = projects;
    } catch (err) {
      projectsState.error = toProjectError(err);
    } finally {
      projectsState.isLoading = false;
    }

    if (projectsState.error) return; // Don't chase counts when the list itself failed.
    // Fire-and-forget: counts populate cards after the wizard has already moved on.
    this.loadProjectCounts();
  },

  async loadProjectCounts() {
    if (!connectionState.connectionId) return;
    const keys = projectsState.available.map((p) => p.key);
    if (keys.length === 0) return;

    // Capture the request token so a stale openIssuesOnly toggle can't
    // overwrite newer state when its slower response finally lands.
    const requestedOpenOnly = projectsState.openIssuesOnly;
    projectsState.isLoadingCounts = true;
    try {
      const counts = await api.jiraImport.getProjectCounts(
        connectionState.connectionId,
        keys,
        requestedOpenOnly
      );
      if (requestedOpenOnly !== projectsState.openIssuesOnly) return; // stale
      projectsState.available = projectsState.available.map((p) => ({
        ...p,
        issue_count: counts[p.key] ?? null,
      }));
    } catch (err) {
      // Surface upstream Jira errors (e.g. token revoked between project list
      // and counts) the same way as loadProjects so the UI banner can render.
      // Other errors are non-fatal — cards just keep showing "…".
      const e = toProjectError(err);
      if (e.code?.startsWith('JIRA_')) {
        projectsState.error = e;
      } else {
        console.warn('Failed to load Jira project counts:', err);
      }
    } finally {
      projectsState.isLoadingCounts = false;
    }
  },

  // Re-fetch counts only — the project list itself doesn't change when the
  // open-issues filter toggles.
  async reloadProjectsWithFilter() {
    await this.loadProjectCounts();
  },

  // Toggle open issues only filter
  toggleOpenIssuesOnly() {
    projectsState.openIssuesOnly = !projectsState.openIssuesOnly;
  },

  toggleProject(projectKey) {
    const idx = projectsState.selected.indexOf(projectKey);
    if (idx >= 0) {
      projectsState.selected = projectsState.selected.filter((k) => k !== projectKey);
    } else {
      projectsState.selected = [...projectsState.selected, projectKey];
    }
  },

  selectAllProjects() {
    projectsState.selected = projectsState.available.map((p) => p.key);
  },

  deselectAllProjects() {
    projectsState.selected = [];
  },

  // Analysis methods
  async analyzeProjects() {
    if (!connectionState.connectionId || projectsState.selected.length === 0) {
      return { success: false, error: 'No projects selected' };
    }

    analysisState.isAnalyzing = true;
    analysisState.error = null;
    analysisState.result = null;

    try {
      const result = await api.jiraImport.analyzeProjects(
        connectionState.connectionId,
        projectsState.selected,
        projectsState.openIssuesOnly
      );
      analysisState.result = result;
      markWizardStepComplete('projects');
      this.configureXray(result.xray);

      // Initialize mappings from analysis
      this.initializeMappings(result);

      return { success: true };
    } catch (err) {
      analysisState.error = err.message || 'Failed to analyze projects';
      return { success: false, error: analysisState.error };
    } finally {
      analysisState.isAnalyzing = false;
    }
  },

  configureXray(xrayAnalysis) {
    const detected = xrayAnalysis?.detection_status === 'detected' && xrayAnalysis.total_tests > 0;
    xrayState.available = detected;
    xrayState.detectionStatus = xrayAnalysis?.detection_status || 'not_detected';
    xrayState.totalTests = xrayAnalysis?.total_tests || 0;
    xrayState.projects = xrayAnalysis?.projects || [];
    xrayState.testIssueTypeIds = xrayAnalysis?.test_issue_type_ids || [];
    xrayState.requiresCredential = xrayAnalysis?.requires_credential === true;
    xrayState.importTests = false;
    xrayState.region = 'global';
    xrayState.clientId = '';
    xrayState.clientSecret = '';

    const completed = new Map(wizardState.steps.map((step) => [step.id, step.completed]));
    wizardState.steps = defaultJiraWizardSteps(detected).map((step) => ({
      ...step,
      completed: completed.get(step.id) || false,
    }));
    goToWizardStep('projects');
  },

  initializeMappings(analysis) {
    // Initialize workspace mappings
    mappingsState.workspaces = analysis.projects.map((p) => ({
      jiraKey: p.key,
      jiraName: p.name,
      issueCount: p.issue_count,
      windshiftId: null,
      createNew: true,
      newWorkspaceName: p.name,
      newWorkspaceKey: p.suggested_workspace_key || p.key,
      workspaceKeyCollisionFound: p.workspace_key_collision === true,
      keyAliasAcknowledged: false,
      isTeamManaged: p.is_team_managed === true,
    }));

    // Deduplicate issue types by name (keep all Jira IDs for mapping during import)
    const issueTypesByName = new Map();
    for (const it of analysis.issue_types) {
      const existing = issueTypesByName.get(it.name);
      if (existing) {
        existing.jiraIds.push(it.id); // Add additional Jira ID
      } else {
        issueTypesByName.set(it.name, {
          jiraIds: [it.id], // Array of all Jira IDs with this name
          jiraName: it.name,
          isSubtask: it.subtask,
          hierarchyLevel: it.hierarchy_level,
          windshiftId: null,
          createNew: true,
        });
      }
    }
    mappingsState.issueTypes = Array.from(issueTypesByName.values());

    // Deduplicate statuses by name (keep all Jira IDs for mapping during import)
    const statusesByName = new Map();
    for (const s of analysis.statuses) {
      const existing = statusesByName.get(s.name);
      if (existing) {
        existing.jiraIds.push(s.id); // Add additional Jira ID
      } else {
        statusesByName.set(s.name, {
          jiraIds: [s.id], // Array of all Jira IDs with this name
          jiraName: s.name,
          categoryKey: s.category_key,
          categoryName: s.category_name,
          color: s.color,
          windshiftId: null,
          createNew: true,
        });
      }
    }
    mappingsState.statuses = Array.from(statusesByName.values());

    // Initialize version mappings
    mappingsState.versions = (analysis.versions || []).map((v) => ({
      jiraId: v.id,
      jiraName: v.name,
      projectKey: v.project_key,
      released: v.released,
      releaseDate: v.release_date,
      createNew: true,
    }));

    // Initialize field mappings
    mappingsState.customFields = analysis.custom_fields.map((f) => ({
      jiraId: f.jira_field_id,
      jiraName: f.jira_field_name,
      jiraType: f.jira_field_type,
      windshiftType: f.windshift_field_type,
      canMap: f.can_map,
      notes: f.notes,
      preserveRaw: f.preserve_raw === true,
      action: f.can_map ? 'create' : 'skip', // 'create', 'map', 'skip'
      windshiftId: null,
      assetSchemaId: f.windshift_field_type === 'asset' ? 'auto' : null,
    }));
    mappingsState.serviceManagement = {
      // Customer organizations are global Windshift entities, so importing
      // them always requires an explicit operator choice.
      importOrganizations: false,
    };
  },

  // Mapping setters
  setWorkspaceMapping(jiraKey, config) {
    const mapping = mappingsState.workspaces.find((m) => m.jiraKey === jiraKey);
    if (mapping) {
      Object.assign(mapping, config);
    }
  },

  setWorkspaceKeyAliasAcknowledged(jiraKey, acknowledged) {
    const mapping = mappingsState.workspaces.find((m) => m.jiraKey === jiraKey);
    if (mapping?.workspaceKeyCollisionFound) {
      mapping.keyAliasAcknowledged = acknowledged;
    }
  },

  setIssueTypeMapping(jiraName, windshiftId, createNew = false) {
    const mapping = mappingsState.issueTypes.find((m) => m.jiraName === jiraName);
    if (mapping) {
      mapping.windshiftId = windshiftId;
      mapping.createNew = createNew;
    }
  },

  setStatusMapping(jiraName, windshiftId, createNew = false) {
    const mapping = mappingsState.statuses.find((m) => m.jiraName === jiraName);
    if (mapping) {
      mapping.windshiftId = windshiftId;
      mapping.createNew = createNew;
    }
  },

  setFieldAction(jiraId, action, windshiftId = null) {
    const mapping = mappingsState.customFields.find((m) => m.jiraId === jiraId);
    if (mapping) {
      mapping.action = action;
      mapping.windshiftId = windshiftId;
    }
  },

  setAssetFieldSchema(jiraId, assetSchemaId) {
    const mapping = mappingsState.customFields.find((m) => m.jiraId === jiraId);
    if (mapping?.windshiftType === 'asset') {
      mapping.assetSchemaId = assetSchemaId;
    }
  },

  setImportServiceManagementOrganizations(enabled) {
    mappingsState.serviceManagement.importOrganizations = enabled;
  },

  // Navigation
  ...createWizardNavigation(() => wizardState),
  goToStepId(stepId) {
    goToWizardStep(stepId);
  },

  // Validation
  canProceed() {
    const step = wizardState.steps[wizardState.currentStep];
    switch (step.id) {
      case 'connect':
        return connectionState.isConnected;
      case 'projects':
        return projectsState.selected.length > 0;
      case 'xray':
        return (
          !xrayState.importTests ||
          !xrayState.requiresCredential ||
          (xrayState.clientId.trim() !== '' && xrayState.clientSecret.trim() !== '')
        );
      case 'mapping':
        return (
          analysisState.result !== null &&
          mappingsState.workspaces.every(
            (mapping) =>
              !mapping.workspaceKeyCollisionFound || mapping.keyAliasAcknowledged === true
          )
        );
      case 'preview':
        return true;
      case 'import':
        return importState.result !== null;
      default:
        return false;
    }
  },

  // Get import summary
  getImportSummary() {
    if (!analysisState.result) return null;

    const users = analysisState.result.users || [];
    const matchedUsers = users.filter((u) => u.matched_user_id != null);
    const unmatchedUsers = users.filter((u) => u.matched_user_id == null);

    return {
      projectCount: projectsState.selected.length,
      issueCount: Math.max(
        0,
        analysisState.result.total_issues - (xrayState.importTests ? xrayState.totalTests : 0)
      ),
      testCaseCount: xrayState.importTests ? xrayState.totalTests : 0,
      issueTypeCount: mappingsState.issueTypes.length,
      statusCount: mappingsState.statuses.length,
      fieldCount: mappingsState.customFields.filter((f) => f.action !== 'skip').length,
      assetCount: analysisState.result.total_assets,
      userCount: users.length,
      matchedUserCount: matchedUsers.length,
      unmatchedUserCount: unmatchedUsers.length,
    };
  },

  // Get users from analysis result
  getUsers() {
    if (!analysisState.result?.users) return [];
    return analysisState.result.users;
  },

  // Get matched vs unmatched users
  getUserStats() {
    const users = this.getUsers();
    const matched = users.filter((u) => u.matched_user_id != null);
    const unmatched = users.filter((u) => u.matched_user_id == null);
    return {
      total: users.length,
      matched: matched.length,
      unmatched: unmatched.length,
      matchedUsers: matched,
      unmatchedUsers: unmatched,
    };
  },

  // Start the import process
  async startImport(forceReimport = false) {
    if (!connectionState.connectionId || projectsState.selected.length === 0) return;

    importState.isImporting = true;
    importState.error = null;
    importState.errorCode = null;
    importState.conflictingImports = [];
    // Starting an import can spend significant time preparing Jira/Xray data.
    // Move to the progress step immediately so the user sees causal feedback
    // and cannot submit the same import again while that request is in flight.
    markWizardStepComplete('preview');
    goToWizardStep('import');

    try {
      const response = await api.jiraImport.startImport({
        connection_id: connectionState.connectionId,
        project_keys: projectsState.selected,
        open_issues_only: projectsState.openIssuesOnly,
        force_reimport: forceReimport,
        mappings: mappingsState,
        xray: {
          import_tests: xrayState.available && xrayState.importTests,
          region: xrayState.region,
          client_id: xrayState.importTests ? xrayState.clientId : '',
          client_secret: xrayState.importTests ? xrayState.clientSecret : '',
          test_issue_type_ids: xrayState.testIssueTypeIds,
        },
      });

      importState.jobId = response.job_id;

      // Start polling for job status
      this.pollJobStatus();

      return { success: true, jobId: response.job_id };
    } catch (err) {
      /** @type {any} */
      const e = err || {};
      importState.error = e.message || 'Failed to start import';
      importState.errorCode = e.code || e.errorCode || null;
      importState.conflictingImports = e.details?.conflicting_imports || [];
      goToWizardStep('import'); // Surface start failures in the visible import step
      return { success: false, error: importState.error, code: importState.errorCode };
    } finally {
      importState.isImporting = false;
    }
  },

  // Poll for job status updates
  async pollJobStatus() {
    if (!importState.jobId) return;

    const poll = async () => {
      try {
        const status = await api.jiraImport.getJobStatus(importState.jobId);
        importState.phase = status.phase || 'running';
        importState.progress = status.progress;

        if (status.status === 'completed' || status.status === 'completed_with_errors') {
          importState.result = status;
          markWizardStepComplete('import');
          return; // Stop polling
        } else if (status.status === 'failed') {
          importState.error = status.error_message || 'Import failed';
          return; // Stop polling
        }

        // Continue polling every 2 seconds
        setTimeout(poll, 2000);
      } catch (err) {
        console.error('Failed to poll job status:', err);
        // Continue polling even on error
        setTimeout(poll, 5000);
      }
    };

    poll();
  },

  // Reset everything
  reset() {
    connectionState = {
      jiraUrl: '',
      email: '',
      apiToken: '',
      deploymentType: 'cloud',
      connectionId: null,
      instanceInfo: null,
      isConnecting: false,
      isConnected: false,
      error: null,
    };

    projectsState = {
      available: [],
      selected: [],
      openIssuesOnly: false,
      isLoading: false,
      isLoadingCounts: false,
      error: null,
    };

    analysisState = {
      isAnalyzing: false,
      result: null,
      error: null,
    };

    xrayState = {
      available: false,
      detectionStatus: 'not_detected',
      totalTests: 0,
      projects: [],
      testIssueTypeIds: [],
      requiresCredential: false,
      importTests: false,
      region: 'global',
      clientId: '',
      clientSecret: '',
    };

    mappingsState = {
      workspaces: [],
      issueTypes: [],
      statuses: [],
      customFields: [],
      versions: [],
      serviceManagement: {
        importOrganizations: false,
      },
    };

    importState = {
      isImporting: false,
      jobId: null,
      phase: 'idle',
      progress: null,
      error: null,
      errorCode: null,
      conflictingImports: [],
      result: null,
    };

    wizardState = {
      currentStep: 0,
      steps: defaultJiraWizardSteps(),
    };
  },
};

export default jiraImport;
