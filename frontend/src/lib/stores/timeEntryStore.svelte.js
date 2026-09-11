import { api } from '../api.js';
import { getUserTimezone } from '../utils/dateFormatter.js';
import { formatClockInZone, monthBoundsInZone } from '../utils/worklogTimezone.js';
import { authStore } from './auth.svelte.js';

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
    date_to: '',
  });

  // === Modal State ===
  showOnboarding = $state(false);
  showTimeLogModal = $state(false);
  editingWorklog = $state(null);

  // === Derived Values ===

  get activeProjects() {
    return this.projects.filter((p) => p.status === 'Active');
  }

  get filteredProjects() {
    return this.filters.customer_id
      ? this.activeProjects.filter((p) => p.customer_id === parseInt(this.filters.customer_id, 10))
      : this.activeProjects;
  }

  get filteredWorklogs() {
    return this.worklogs;
  }

  get totalDuration() {
    return this.worklogs.reduce((sum, w) => sum + w.duration_minutes, 0);
  }

  // === Initialization ===

  async init() {
    this.loading = true;
    try {
      // Default date range to the current month in the reporting timezone so
      // client-side labels and server-side inclusion agree.
      const { from, to } = monthBoundsInZone(getUserTimezone(authStore?.currentUser));
      this.filters.date_from = from;
      this.filters.date_to = to;

      await Promise.all([
        this.loadWorklogs(),
        this.loadCustomers(),
        this.loadProjects(),
        this.loadWorkItems(),
        this.loadWorkspaces(),
      ]);

      // Show onboarding if no customers or projects
      if (this.customers.length === 0 && this.projects.length === 0) {
        this.showOnboarding = true;
      }
    } finally {
      this.loading = false;
    }
  }

  // === Data Loading ===

  async loadWorklogs() {
    try {
      this.worklogsLoading = true;
      this.worklogs = (await api.time.worklogs.getAll(this.dateFilters())) || [];
    } catch (err) {
      console.error('Failed to load worklogs:', err);
      this.worklogs = [];
    } finally {
      this.worklogsLoading = false;
    }
  }

  // Date filters travel with the reporting timezone the views group in, so
  // the server interprets the civil range the same way the client displays it.
  dateFilters() {
    const { date_from, date_to, ...rest } = this.filters;
    return {
      ...rest,
      ...(date_from || date_to ? { timezone: getUserTimezone(authStore?.currentUser) } : {}),
    };
  }

  async loadCustomers() {
    try {
      this.customers = (await api.customerOrganisations.getAll()) || [];
    } catch (err) {
      console.error('Failed to load customers:', err);
      this.customers = [];
    }
  }

  async loadProjects() {
    try {
      this.projects = (await api.time.projects.getAll()) || [];
    } catch (err) {
      console.error('Failed to load projects:', err);
      this.projects = [];
    }
  }

  async loadWorkItems() {
    try {
      const result = await api.items.getAll({ limit: 100 });
      this.workItems = result.data ?? [];
    } catch (err) {
      console.error('Failed to load work items:', err);
      this.workItems = [];
    }
  }

  async loadWorkspaces() {
    try {
      this.workspaces = (await api.workspaces.getAll()) || [];
    } catch (err) {
      console.error('Failed to load workspaces:', err);
      this.workspaces = [];
    }
  }

  // === Filter Management ===

  setFilter(key, value) {
    this.filters[key] = value;
  }

  async applyFilters() {
    await this.loadWorklogs();
  }

  clearFilters() {
    this.filters = {
      customer_id: '',
      project_id: '',
      date_from: '',
      date_to: '',
    };
    this.loadWorklogs();
  }

  // === Worklog CRUD ===

  async createWorklog(data) {
    try {
      await api.time.worklogs.create(data);
      await this.loadWorklogs();
    } catch (err) {
      console.error('Failed to create worklog:', err);
      throw err;
    }
  }

  async updateWorklog(id, data) {
    try {
      await api.time.worklogs.update(id, data);
      await this.loadWorklogs();
    } catch (err) {
      console.error('Failed to update worklog:', err);
      throw err;
    }
  }

  async deleteWorklog(worklog) {
    try {
      await api.time.worklogs.delete(worklog.id);
      await this.loadWorklogs();
    } catch (err) {
      console.error('Failed to delete worklog:', err);
      throw err;
    }
  }

  async saveWorklog(data) {
    if (this.editingWorklog) {
      await this.updateWorklog(this.editingWorklog.id, data);
    } else {
      await this.createWorklog(data);
    }
    this.closeTimeLogModal();
  }

  // === Modal Controls ===

  openTimeLogModal() {
    this.editingWorklog = null;
    this.showTimeLogModal = true;
  }

  editWorklog(worklog) {
    this.editingWorklog = worklog;
    this.showTimeLogModal = true;
  }

  closeTimeLogModal() {
    this.showTimeLogModal = false;
    this.editingWorklog = null;
  }

  openOnboarding() {
    this.showOnboarding = true;
  }

  closeOnboarding() {
    this.showOnboarding = false;
  }

  async handleOnboardingCompleted() {
    await Promise.all([this.loadCustomers(), this.loadProjects()]);
    this.showOnboarding = false;
  }

  // === Utility Methods ===

  formatTime(unixTimestamp) {
    return formatClockInZone(unixTimestamp, getUserTimezone(authStore?.currentUser));
  }

  formatDuration(minutes) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

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
      date_to: '',
    };
    this.showOnboarding = false;
    this.showTimeLogModal = false;
    this.editingWorklog = null;
  }
}

export const timeEntryStore = new TimeEntryStore();
