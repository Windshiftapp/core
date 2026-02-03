// Jira Import Store - State management for the import wizard
// Uses Svelte 5 runes for reactivity

import { api } from '../api.js';

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
  error: null,
});

// Analysis state
let analysisState = $state({
  isAnalyzing: false,
  result: null,
  error: null,
});

// Mappings state
let mappingsState = $state({
  workspaces: [], // { jiraKey, jiraName, windshiftId, createNew, newWorkspaceName, newWorkspaceKey }
  issueTypes: [], // { jiraIds[], jiraName, isSubtask, hierarchyLevel, windshiftId, createNew } - deduplicated by name
  statuses: [], // { jiraIds[], jiraName, categoryKey, categoryName, color, windshiftId, createNew } - deduplicated by name
  customFields: [], // { jiraId, jiraName, windshiftType, action, windshiftId }
  versions: [], // { jiraId, jiraName, projectKey, released, releaseDate, createNew }
});

// Import state
let importState = $state({
  isImporting: false,
  jobId: null,
  phase: 'idle',
  progress: null,
  error: null,
  result: null,
});

// Wizard state
let wizardState = $state({
  currentStep: 0,
  steps: [
    { id: 'connect', label: 'Connect', completed: false },
    { id: 'projects', label: 'Projects', completed: false },
    { id: 'mapping', label: 'Mapping', completed: false },
    { id: 'preview', label: 'Preview', completed: false },
    { id: 'import', label: 'Import', completed: false },
  ],
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

  // Use a saved connection (for wizard)
  useSavedConnection(connection) {
    connectionState.connectionId = connection.id;
    connectionState.jiraUrl = connection.instance_url;
    connectionState.email = connection.email;
    connectionState.deploymentType = connection.deployment_type || 'cloud';
    connectionState.instanceInfo = { display_name: connection.instance_name };
    connectionState.isConnected = true;
    wizardState.steps[0].completed = true;
  },

  // Connection methods
  async testConnection(url, email, token, deploymentType = 'cloud') {
    connectionState.isConnecting = true;
    connectionState.error = null;

    try {
      const response = await api.jiraImport.testConnection({
        instance_url: url,
        email: email,
        api_token: token,
        deployment_type: deploymentType,
      });

      connectionState.connectionId = response.connection_id;
      connectionState.instanceInfo = response.instance_info;
      connectionState.jiraUrl = url;
      connectionState.email = email;
      connectionState.apiToken = token;
      connectionState.deploymentType = deploymentType;
      connectionState.isConnected = true;
      wizardState.steps[0].completed = true;

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

  // Project methods
  async loadProjects() {
    if (!connectionState.connectionId) return;

    projectsState.isLoading = true;
    projectsState.error = null;

    try {
      const projects = await api.jiraImport.getProjects(
        connectionState.connectionId,
        projectsState.openIssuesOnly
      );
      projectsState.available = projects;
    } catch (err) {
      projectsState.error = err.message || 'Failed to load projects';
    } finally {
      projectsState.isLoading = false;
    }
  },

  // Reload projects with the current filter (called when toggle changes)
  async reloadProjectsWithFilter() {
    await this.loadProjects();
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
    // Only select company-managed projects (exclude team-managed)
    projectsState.selected = projectsState.available
      .filter((p) => !p.is_team_managed)
      .map((p) => p.key);
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
      wizardState.steps[1].completed = true;
      wizardState.steps[2].completed = true;

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

  initializeMappings(analysis) {
    // Initialize workspace mappings
    mappingsState.workspaces = analysis.projects.map((p) => ({
      jiraKey: p.key,
      jiraName: p.name,
      issueCount: p.issue_count,
      windshiftId: null,
      createNew: true,
      newWorkspaceName: p.name,
      newWorkspaceKey: p.key,
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
      action: f.can_map ? 'create' : 'skip', // 'create', 'map', 'skip'
      windshiftId: null,
    }));
  },

  // Mapping setters
  setWorkspaceMapping(jiraKey, config) {
    const mapping = mappingsState.workspaces.find((m) => m.jiraKey === jiraKey);
    if (mapping) {
      Object.assign(mapping, config);
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

  // Navigation
  nextStep() {
    if (wizardState.currentStep < wizardState.steps.length - 1) {
      wizardState.currentStep++;
    }
  },

  prevStep() {
    if (wizardState.currentStep > 0) {
      wizardState.currentStep--;
    }
  },

  goToStep(stepIndex) {
    if (stepIndex >= 0 && stepIndex < wizardState.steps.length) {
      wizardState.currentStep = stepIndex;
    }
  },

  // Validation
  canProceed() {
    const step = wizardState.steps[wizardState.currentStep];
    switch (step.id) {
      case 'connect':
        return connectionState.isConnected;
      case 'projects':
        return projectsState.selected.length > 0;
      case 'mapping':
        return analysisState.result !== null; // Can proceed once analysis is done
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
      issueCount: analysisState.result.total_issues,
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
    if (!analysisState.result || !analysisState.result.users) return [];
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
  async startImport() {
    if (!connectionState.connectionId || projectsState.selected.length === 0) return;

    importState.isImporting = true;
    importState.error = null;

    try {
      const response = await api.jiraImport.startImport({
        connection_id: connectionState.connectionId,
        project_keys: projectsState.selected,
        open_issues_only: projectsState.openIssuesOnly,
        mappings: mappingsState,
      });

      importState.jobId = response.job_id;
      wizardState.steps[3].completed = true;
      wizardState.currentStep = 4; // Move to import step

      // Start polling for job status
      this.pollJobStatus();

      return { success: true, jobId: response.job_id };
    } catch (err) {
      importState.error = err.message || 'Failed to start import';
      return { success: false, error: importState.error };
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

        if (status.status === 'completed') {
          importState.result = status;
          wizardState.steps[4].completed = true;
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
      error: null,
    };

    analysisState = {
      isAnalyzing: false,
      result: null,
      error: null,
    };

    mappingsState = {
      workspaces: [],
      issueTypes: [],
      statuses: [],
      customFields: [],
      versions: [],
    };

    importState = {
      isImporting: false,
      jobId: null,
      phase: 'idle',
      progress: null,
      error: null,
      result: null,
    };

    wizardState = {
      currentStep: 0,
      steps: wizardState.steps.map((s) => ({ ...s, completed: false })),
    };
  },
};

export default jiraImport;
