/**
 * Store for managing Homepage state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes dashboard data, activity, and UI state.
 */
import { api } from '../api.js';
import {
  buildDefaultDashboardLayout,
  getDashboardWidgetDefaultWidth,
} from '../services/dashboardWidgetRegistry.js';
import { formatDateSimple, formatDateWithOptions } from '../utils/dateFormatter.js';

const ONBOARDING_STORAGE_KEY = 'windshift-dashboard-onboarding-dismissed';

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

  // === Notifications ===
  notifications = $state([]);

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
  layoutLoaded = $state(false);
  isEditMode = $state(false);
  isCustomizeMode = $state(false);
  _saveTimeout = null;
  _savePending = false;
  _pendingSaveQueued = false;

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
    await Promise.all([this.loadDashboardData(), this.loadLayout()]);
  }

  // === Layout loading / saving ===

  async loadLayout() {
    try {
      const layout = await api.homepage.getLayout();
      if (layout && Array.isArray(layout.sections) && layout.sections.length > 0) {
        this.sections = [...layout.sections].sort((a, b) => a.display_order - b.display_order);
        this.widgets = Array.isArray(layout.widgets) ? [...layout.widgets] : [];
      } else {
        const defaults = buildDefaultDashboardLayout();
        this.sections = defaults.sections;
        this.widgets = defaults.widgets;
      }
    } catch (err) {
      console.error('Failed to load dashboard layout:', err);
      const defaults = buildDefaultDashboardLayout();
      this.sections = defaults.sections;
      this.widgets = defaults.widgets;
    } finally {
      this.layoutLoaded = true;
    }
  }

  async saveLayout() {
    if (this._savePending) {
      this._pendingSaveQueued = true;
      return;
    }
    this._savePending = true;

    try {
      const layout = {
        sections: this.sections.map((s, idx) => ({ ...s, display_order: idx })),
        widgets: this.widgets.map((w, idx) => ({ ...w, position: idx })),
      };
      await api.homepage.updateLayout(layout);
    } catch (err) {
      console.error('Failed to save dashboard layout:', err);
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
    this.widgets = this.widgets.map((w) => (w.id === widgetId ? { ...w, width: newWidth } : w));
    this.debouncedSaveLayout();
  }

  getWidgetsForSection(sectionId) {
    return this.widgets
      .filter((w) => w.section_id === sectionId)
      .sort((a, b) => a.position - b.position);
  }

  // === Data Loading ===

  /**
   * Load all homepage data.
   */
  async loadDashboardData() {
    try {
      this.loading = true;
      const data = await api.homepage.get();

      // Load recent workspaces with icon and color
      this.recentWorkspaces = (data.recent_workspaces || []).slice(0, 5);

      // Load total counts
      this.totalWorkspaceCount = data.total_workspace_count || 0;
      this.totalItemCount = data.total_item_count || 0;

      // Load watched items
      this.watchedItems = data.watched_items || [];

      // Load upcoming milestones
      this.upcomingMilestones = data.upcoming_milestones || [];

      // Load activity data
      this.recentlyViewed = data.recently_viewed || [];
      this.recentlyEdited = data.recently_edited || [];
      this.recentlyCommented = data.recently_commented || [];

      // Load notifications. "What's New" hides read notifications once they're
      // older than a day — the tray keeps them, the dashboard doesn't.
      // Fetch a buffer so the visible slice still fills after filtering.
      const notificationsData = await api.notifications.getAll({ limit: 20 });
      const dayAgo = Date.now() - 24 * 60 * 60 * 1000;
      const visible = (notificationsData || []).filter((n) => {
        if (!n.read) return true;
        return new Date(n.timestamp).getTime() >= dayAgo;
      });
      this.notifications = visible.slice(0, 5);
    } catch (err) {
      console.error('Failed to load homepage data:', err);
    } finally {
      this.loading = false;
    }
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
      this.greeting = 'Good morning';
    } else if (hour >= 12 && hour < 18) {
      this.greeting = 'Good afternoon';
    } else if (hour >= 18 && hour < 22) {
      this.greeting = 'Good evening';
    } else {
      this.greeting = 'Good night';
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
    if (!timestamp) return 'Unknown';

    const now = new Date();
    const then = new Date(timestamp);
    const diffMs = now.getTime() - then.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
    if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
    if (diffDays < 7) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;

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
    this.notifications = [];
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
    this.layoutLoaded = false;
    this.isEditMode = false;
    this.isCustomizeMode = false;
    clearTimeout(this._saveTimeout);
  }
}

export const homepageStore = new HomepageStore();
