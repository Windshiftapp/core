/**
 * Store for managing Time Entry state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes worklogs, filters, and modal state.
 */
import { api } from '../api.js';

class TimeEntryStore {
  // === Data ===
  worklogs = $state([]);
  customers = $state([]);
  projects = $state([]);
  workItems = $state([]);
  workspaces = $state([]);

  // === Loading ===
  loading = $state(false);
  worklogsLoading = $state(false);

  // === Filters ===
  filters = $state({
    customer_id: '',
    project_id: '',
    date_from: '',
    date_to: ''
  });

  // === Modal State ===
  showOnboarding = $state(false);
  showTimeLogModal = $state(false);
  editingWorklog = $state(null);

  // === Derived Values ===

  /**
   * Get active projects.
   */
  get activeProjects() {
    return this.projects.filter(p => p.status === 'Active');
  }

  /**
   * Get projects filtered by customer.
   */
  get filteredProjects() {
    return this.filters.customer_id
      ? this.activeProjects.filter(p => p.customer_id === parseInt(this.filters.customer_id))
      : this.activeProjects;
  }

  /**
   * Get filtered worklogs based on current filters.
   */
  get filteredWorklogs() {
    return this.worklogs;
  }

  /**
   * Get total duration of filtered worklogs.
   */
  get totalDuration() {
    return this.worklogs.reduce((sum, w) => sum + w.duration_minutes, 0);
  }

  // === Initialization ===

  /**
   * Initialize the store and load all data.
   */
  async init() {
    this.loading = true;
    try {
      await Promise.all([
        this.loadWorklogs(),
        this.loadCustomers(),
        this.loadProjects(),
        this.loadWorkItems(),
        this.loadWorkspaces()
      ]);

      // Show onboarding if no customers or projects
      if (this.customers.length === 0 && this.projects.length === 0) {
        this.showOnboarding = true;
      }

      // Set default date range to current month
      const now = new Date();
      this.filters.date_from = new Date(now.getFullYear(), now.getMonth(), 1).toISOString().split('T')[0];
      this.filters.date_to = new Date(now.getFullYear(), now.getMonth() + 1, 0).toISOString().split('T')[0];

      // Reload worklogs with date filter
      await this.loadWorklogs();
    } finally {
      this.loading = false;
    }
  }

  // === Data Loading ===

  async loadWorklogs() {
    try {
      this.worklogsLoading = true;
      this.worklogs = await api.time.worklogs.getAll(this.filters) || [];
    } catch (err) {
      console.error('Failed to load worklogs:', err);
      this.worklogs = [];
    } finally {
      this.worklogsLoading = false;
    }
  }

  async loadCustomers() {
    try {
      this.customers = await api.customerOrganisations.getAll() || [];
    } catch (err) {
      console.error('Failed to load customers:', err);
      this.customers = [];
    }
  }

  async loadProjects() {
    try {
      this.projects = await api.time.projects.getAll() || [];
    } catch (err) {
      console.error('Failed to load projects:', err);
      this.projects = [];
    }
  }

  async loadWorkItems() {
    try {
      const result = await api.items.getAll({ limit: 100 });
      this.workItems = result.items || [];
    } catch (err) {
      console.error('Failed to load work items:', err);
      this.workItems = [];
    }
  }

  async loadWorkspaces() {
    try {
      this.workspaces = await api.workspaces.getAll() || [];
    } catch (err) {
      console.error('Failed to load workspaces:', err);
      this.workspaces = [];
    }
  }

  // === Filter Management ===

  /**
   * Set a filter value.
   */
  setFilter(key, value) {
    this.filters[key] = value;
  }

  /**
   * Apply filters and reload worklogs.
   */
  async applyFilters() {
    await this.loadWorklogs();
  }

  /**
   * Clear all filters.
   */
  clearFilters() {
    this.filters = {
      customer_id: '',
      project_id: '',
      date_from: '',
      date_to: ''
    };
    this.loadWorklogs();
  }

  // === Worklog CRUD ===

  /**
   * Create a new worklog.
   */
  async createWorklog(data) {
    try {
      await api.time.worklogs.create(data);
      await this.loadWorklogs();
    } catch (err) {
      console.error('Failed to create worklog:', err);
      throw err;
    }
  }

  /**
   * Update an existing worklog.
   */
  async updateWorklog(id, data) {
    try {
      await api.time.worklogs.update(id, data);
      await this.loadWorklogs();
    } catch (err) {
      console.error('Failed to update worklog:', err);
      throw err;
    }
  }

  /**
   * Delete a worklog.
   */
  async deleteWorklog(worklog) {
    try {
      await api.time.worklogs.delete(worklog.id);
      await this.loadWorklogs();
    } catch (err) {
      console.error('Failed to delete worklog:', err);
      throw err;
    }
  }

  /**
   * Save worklog (create or update based on editingWorklog).
   */
  async saveWorklog(data) {
    if (this.editingWorklog) {
      await this.updateWorklog(this.editingWorklog.id, data);
    } else {
      await this.createWorklog(data);
    }
    this.closeTimeLogModal();
  }

  // === Modal Controls ===

  /**
   * Open time log modal for creating new entry.
   */
  openTimeLogModal() {
    this.editingWorklog = null;
    this.showTimeLogModal = true;
  }

  /**
   * Open time log modal for editing existing entry.
   */
  editWorklog(worklog) {
    this.editingWorklog = worklog;
    this.showTimeLogModal = true;
  }

  /**
   * Close time log modal.
   */
  closeTimeLogModal() {
    this.showTimeLogModal = false;
    this.editingWorklog = null;
  }

  /**
   * Show onboarding wizard.
   */
  openOnboarding() {
    this.showOnboarding = true;
  }

  /**
   * Close onboarding wizard.
   */
  closeOnboarding() {
    this.showOnboarding = false;
  }

  /**
   * Handle onboarding completion.
   */
  async handleOnboardingCompleted() {
    await Promise.all([this.loadCustomers(), this.loadProjects()]);
    this.showOnboarding = false;
  }

  // === Utility Methods ===

  /**
   * Format time from unix timestamp.
   */
  formatTime(unixTimestamp) {
    const date = new Date(unixTimestamp * 1000);
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false
    });
  }

  /**
   * Format duration in minutes to human-readable string.
   */
  formatDuration(minutes) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

  /**
   * Check if a project is over budget.
   */
  isProjectOverBudget(worklog) {
    if (!worklog.project_max_hours || worklog.project_max_hours <= 0) return false;
    return (worklog.project_total_hours || 0) > worklog.project_max_hours;
  }

  // === Full Reset ===

  reset() {
    this.worklogs = [];
    this.customers = [];
    this.projects = [];
    this.workItems = [];
    this.workspaces = [];
    this.loading = false;
    this.worklogsLoading = false;
    this.filters = {
      customer_id: '',
      project_id: '',
      date_from: '',
      date_to: ''
    };
    this.showOnboarding = false;
    this.showTimeLogModal = false;
    this.editingWorklog = null;
  }
}

export const timeEntryStore = new TimeEntryStore();
