/**
 * Store for managing Homepage state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes dashboard data, activity, and UI state.
 */
import { api } from '../api.js';
import {
  getDashboardResizeBounds,
  resizeDashboardWidgetRow,
} from '../services/dashboardGridLayout.js';
import {
  buildDefaultDashboardLayout,
  DASHBOARD_GRID_COLUMNS,
  getDashboardWidgetDefaultWidth,
  getDashboardWidgetMinWidth,
} from '../services/dashboardWidgetRegistry.js';
import { formatDateSimple, formatDateWithOptions } from '../utils/dateFormatter.js';
import { t } from './i18n.svelte.js';
import { errorToast } from './toasts.svelte.js';

const ONBOARDING_STORAGE_KEY = 'windshift-dashboard-onboarding-dismissed';
const HOMEPAGE_SNAPSHOT_REUSE_MS = 5_000;
class HomepageStore {
  // === Dashboard Data ===
  recentWorkspaces = $state([]);
  totalWorkspaceCount = $state(0);
  totalItemCount = $state(0);
  watchedItems = $state([]);

  // === Activity Data ===
  recentlyViewed = $state([]);
  recentlyEdited = $state([]);
  recentlyCommented = $state([]);

  // === Milestones ===
  upcomingMilestones = $state([]);

  // === Loading States ===
  loading = $state(true);
  activityLoading = $state(false);
  milestonesLoading = $state(false);

  // === Tab State ===
  activeTab = $state('viewed'); // viewed, edited, commented

  // === Onboarding ===
  onboardingDismissed = $state(false);
  canCreateWorkspaces = $state(false);
  accessibleWorkspaces = $state([]);

  // === Greeting ===
  greeting = $state('');
  currentDate = $state('');

  // === Layout / Customization ===
  sections = $state([]);
  widgets = $state([]);
  layoutRevision = $state('');
  layoutLoaded = $state(false);
  isEditMode = $state(false);
  isCustomizeMode = $state(false);
  _saveTimeout = null;
  _savePending = false;
  _pendingSaveQueued = false;
  _snapshot = null;
  _snapshotFetchedAt = 0;
  _loadPromise = null;
  _generation = 0;

  // === Derived Values ===

  /**
   * Check if in onboarding mode.
   */
  get isOnboarding() {
    if (this.onboardingDismissed) return false;
    if (this.canCreateWorkspaces) {
      return this.totalWorkspaceCount === 0 || this.totalItemCount === 0;
    }
    return this.accessibleWorkspaces.length === 0;
  }

  // === Initialization ===

  /**
   * Initialize the store.
   */
  async init(userTimezone = 'UTC') {
    // Check if onboarding was previously dismissed
    if (typeof localStorage !== 'undefined') {
      this.onboardingDismissed = localStorage.getItem(ONBOARDING_STORAGE_KEY) === 'true';
    }

    this.calculateGreeting(userTimezone);
    const data = await this.getSnapshot();
    this.applyLayout(data?.layout, data?.layout_revision);
  }

  // === Layout loading / saving ===

  async loadLayout() {
    const data = await this.getSnapshot();
    this.applyLayout(data?.layout, data?.layout_revision);
  }

  applyLayout(layout, revision = '') {
    if (layout && Array.isArray(layout.sections) && layout.sections.length > 0) {
      this.sections = [...layout.sections].sort((a, b) => a.display_order - b.display_order);
      const widgets = Array.isArray(layout.widgets) ? [...layout.widgets] : [];
      // Migrate only unversioned legacy 3-column layouts. Compact widgets are
      // valid on the 12-column grid, so their widths alone cannot identify an
      // old layout once the explicit grid marker is present.
      if (
        layout.grid_columns !== DASHBOARD_GRID_COLUMNS &&
        widgets.length > 0 &&
        widgets.every((w) => typeof w.width === 'number' && w.width <= 3)
      ) {
        for (const w of widgets) {
          w.width = w.width * 4;
        }
      }
      this.widgets = widgets;
    } else {
      const defaults = buildDefaultDashboardLayout();
      this.sections = defaults.sections;
      this.widgets = defaults.widgets;
    }
    this.layoutRevision = revision || '';
    this.layoutLoaded = true;
  }

  async saveLayout() {
    if (this._savePending) {
      this._pendingSaveQueued = true;
      return;
    }
    this._savePending = true;

    try {
      const layout = {
        grid_columns: DASHBOARD_GRID_COLUMNS,
        sections: this.sections.map((s, idx) => ({ ...s, display_order: idx })),
        widgets: this.widgets.map((w, idx) => ({ ...w, position: idx })),
      };
      const savedLayout = await api.homepage.updateLayout(layout);
      if (this._snapshot) {
        this._snapshot = { ...this._snapshot, layout: savedLayout || layout };
      }
      // The save endpoint intentionally keeps its backward-compatible layout
      // response shape, so the next aggregate refresh will supply its revision.
      this.layoutRevision = '';
    } catch (err) {
      console.error('Failed to save dashboard layout:', err);
      errorToast(t('toast.layoutSaveFailed'));
    } finally {
      this._savePending = false;
      if (this._pendingSaveQueued) {
        this._pendingSaveQueued = false;
        this.debouncedSaveLayout();
      }
    }
  }

  debouncedSaveLayout() {
    clearTimeout(this._saveTimeout);
    this._saveTimeout = setTimeout(() => this.saveLayout(), 1000);
  }

  // === Mode toggles ===

  toggleEditMode() {
    this.isEditMode = !this.isEditMode;
    if (this.isEditMode && this.isCustomizeMode) {
      this.isCustomizeMode = false;
    }
    if (!this.isEditMode) {
      this.debouncedSaveLayout();
    }
  }

  toggleCustomizeMode() {
    this.isCustomizeMode = !this.isCustomizeMode;
    if (this.isCustomizeMode && this.isEditMode) {
      this.isEditMode = false;
    }
  }

  // === Section management ===

  addSection(title = 'New Section', subtitle = '') {
    const newSection = {
      id: crypto.randomUUID(),
      title,
      subtitle,
      display_order: this.sections.length,
      widget_ids: [],
    };
    this.sections = [...this.sections, newSection];
    this.debouncedSaveLayout();
    return newSection;
  }

  updateSection(sectionId, changes) {
    this.sections = this.sections.map((s) => (s.id === sectionId ? { ...s, ...changes } : s));
    this.debouncedSaveLayout();
  }

  moveSection(sectionId, offset) {
    const currentIndex = this.sections.findIndex((section) => section.id === sectionId);
    const nextIndex = currentIndex + offset;
    if (currentIndex < 0 || nextIndex < 0 || nextIndex >= this.sections.length) return false;

    const reordered = [...this.sections];
    const [section] = reordered.splice(currentIndex, 1);
    reordered.splice(nextIndex, 0, section);
    this.sections = reordered.map((candidate, index) => ({
      ...candidate,
      display_order: index,
    }));
    this.debouncedSaveLayout();
    return true;
  }

  deleteSection(sectionId) {
    this.widgets = this.widgets.filter((w) => w.section_id !== sectionId);
    this.sections = this.sections.filter((s) => s.id !== sectionId);
    this.debouncedSaveLayout();
  }

  // === Widget management ===

  addWidgetToSection(sectionId, widgetType) {
    const newWidget = {
      id: crypto.randomUUID(),
      type: widgetType,
      section_id: sectionId,
      position: this.widgets.filter((w) => w.section_id === sectionId).length,
      width: getDashboardWidgetDefaultWidth(widgetType),
      config: {},
    };
    this.widgets = [...this.widgets, newWidget];
    this.sections = this.sections.map((s) =>
      s.id === sectionId ? { ...s, widget_ids: [...s.widget_ids, newWidget.id] } : s
    );
    this.debouncedSaveLayout();
  }

  removeWidget(widgetId) {
    const widget = this.widgets.find((w) => w.id === widgetId);
    if (!widget) return;
    const sectionId = widget.section_id;
    this.widgets = this.widgets.filter((w) => w.id !== widgetId);
    this.sections = this.sections.map((s) =>
      s.id === sectionId ? { ...s, widget_ids: s.widget_ids.filter((id) => id !== widgetId) } : s
    );
    this.debouncedSaveLayout();
  }

  updateWidgetWidth(widgetId, newWidth) {
    const widget = this.widgets.find((candidate) => candidate.id === widgetId);
    if (!widget) return null;
    const sectionWidgets = this.getWidgetsForSection(widget.section_id);
    const resized = resizeDashboardWidgetRow(
      sectionWidgets,
      widgetId,
      newWidth,
      getDashboardWidgetMinWidth
    );
    if (resized.widgets !== sectionWidgets) {
      const updates = new Map(resized.widgets.map((candidate) => [candidate.id, candidate]));
      this.widgets = this.widgets.map((candidate) => updates.get(candidate.id) ?? candidate);
    }
    this.debouncedSaveLayout();
    return resized.width;
  }

  getWidgetResizeBounds(widgetId) {
    const widget = this.widgets.find((candidate) => candidate.id === widgetId);
    if (!widget) return null;
    return getDashboardResizeBounds(
      this.getWidgetsForSection(widget.section_id),
      widgetId,
      getDashboardWidgetMinWidth
    );
  }

  updateWidgetConfig(widgetId, configChanges) {
    this.widgets = this.widgets.map((w) =>
      w.id === widgetId ? { ...w, config: { ...(w.config ?? {}), ...configChanges } } : w
    );
    this.debouncedSaveLayout();
  }

  getWidgetsForSection(sectionId) {
    return this.widgets
      .filter((w) => w.section_id === sectionId)
      .sort((a, b) => a.position - b.position);
  }

  // === Data Loading ===

  /**
   * Return the current route snapshot, sharing an in-flight request between
   * widgets. Live refreshes call loadDashboardData(), which always replaces it.
   */
  async getSnapshot() {
    const snapshotFresh =
      this._snapshot && Date.now() - this._snapshotFetchedAt <= HOMEPAGE_SNAPSHOT_REUSE_MS;
    if (snapshotFresh) return this._snapshot;
    return this._fetchDashboardData({ force: Boolean(this._snapshot) });
  }

  /** Mark live homepage data stale without blanking currently rendered widgets. */
  invalidateSnapshot() {
    this._snapshotFetchedAt = 0;
  }

  /** Load fresh homepage data and replace the shared widget snapshot. */
  async loadDashboardData() {
    return this._fetchDashboardData({ force: true });
  }

  async _fetchDashboardData({ force = false } = {}) {
    if (this._loadPromise) return this._loadPromise;
    if (!force && this._snapshot) return this._snapshot;

    const generation = this._generation;
    this.loading = true;
    const request = api.homepage
      .get()
      .then((data) => {
        if (generation !== this._generation) return this._snapshot;
        this._snapshot = data || {};
        this._snapshotFetchedAt = Date.now();

        // Load recent workspaces with icon and color
        this.recentWorkspaces = (data?.recent_workspaces || []).slice(0, 5);

        // Load total counts
        this.totalWorkspaceCount = data?.total_workspace_count || 0;
        this.totalItemCount = data?.total_item_count || 0;

        // Load watched items
        this.watchedItems = data?.watched_items || [];

        // Load upcoming milestones
        this.upcomingMilestones = data?.upcoming_milestones || [];

        // Load activity data
        this.recentlyViewed = data?.recently_viewed || [];
        this.recentlyEdited = data?.recently_edited || [];
        this.recentlyCommented = data?.recently_commented || [];

        return this._snapshot;
      })
      .catch((err) => {
        if (generation === this._generation) {
          console.error('Failed to load homepage data:', err);
        }
        return this._snapshot;
      })
      .finally(() => {
        if (generation === this._generation) this.loading = false;
        if (this._loadPromise === request) this._loadPromise = null;
      });

    this._loadPromise = request;
    return request;
  }

  /**
   * Refresh homepage data.
   */
  async refresh() {
    await this.loadDashboardData();
  }

  // === Greeting Calculation ===

  /**
   * Calculate greeting based on time of day.
   */
  calculateGreeting(userTimezone = 'UTC') {
    const now = new Date();

    // Get hour in user's timezone
    const hourString = now.toLocaleString('en-US', {
      timeZone: userTimezone,
      hour: 'numeric',
      hour12: false,
    });
    const hour = parseInt(hourString, 10);

    // Determine greeting based on time of day
    if (hour >= 5 && hour < 12) {
      this.greeting = t('dashboard.goodMorning');
    } else if (hour >= 12 && hour < 18) {
      this.greeting = t('dashboard.goodAfternoon');
    } else if (hour >= 18 && hour < 22) {
      this.greeting = t('dashboard.goodEvening');
    } else {
      this.greeting = t('dashboard.goodNight');
    }

    // Format current date
    this.currentDate = formatDateWithOptions(now, {
      timeZone: userTimezone,
      weekday: 'long',
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  }

  // === Tab Management ===

  /**
   * Set active tab.
   */
  setActiveTab(tab) {
    this.activeTab = tab;
  }

  // === Onboarding ===

  /**
   * Set accessible workspaces and admin flag for non-admin onboarding.
   */
  setAccessibleWorkspaces(workspaces, canCreate) {
    this.accessibleWorkspaces = workspaces;
    this.canCreateWorkspaces = canCreate;
  }

  /**
   * Dismiss onboarding.
   */
  dismissOnboarding() {
    this.onboardingDismissed = true;
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(ONBOARDING_STORAGE_KEY, 'true');
    }
  }

  // === Utility Methods ===

  /**
   * Format relative time.
   */
  formatRelativeTime(timestamp) {
    if (!timestamp) return t('dashboard.relative.unknown');

    const now = new Date();
    const then = new Date(timestamp);
    const diffMs = now.getTime() - then.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return t('dashboard.relative.justNow');
    if (diffMins < 60) return t('dashboard.relative.minutesAgo', { count: diffMins });
    if (diffHours < 24) return t('dashboard.relative.hoursAgo', { count: diffHours });
    if (diffDays < 7) return t('dashboard.relative.daysAgo', { count: diffDays });

    return formatDateSimple(then);
  }

  /**
   * Calculate days until a target date.
   */
  calculateDaysUntil(targetDate) {
    if (!targetDate) return null;

    const now = new Date();
    const target = new Date(targetDate);
    const diffTime = target.getTime() - now.getTime();
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

    return diffDays;
  }

  // === Full Reset ===

  reset() {
    this.recentWorkspaces = [];
    this.totalWorkspaceCount = 0;
    this.totalItemCount = 0;
    this.watchedItems = [];
    this.recentlyViewed = [];
    this.recentlyEdited = [];
    this.recentlyCommented = [];
    this.upcomingMilestones = [];
    this.loading = true;
    this.activityLoading = false;
    this.milestonesLoading = false;
    this.activeTab = 'viewed';
    this.onboardingDismissed = false;
    this.canCreateWorkspaces = false;
    this.accessibleWorkspaces = [];
    this.greeting = '';
    this.currentDate = '';
    this.sections = [];
    this.widgets = [];
    this.layoutRevision = '';
    this.layoutLoaded = false;
    this.isEditMode = false;
    this.isCustomizeMode = false;
    clearTimeout(this._saveTimeout);
    this._generation += 1;
    this._snapshot = null;
    this._snapshotFetchedAt = 0;
    this._loadPromise = null;
  }
}

export const homepageStore = new HomepageStore();
