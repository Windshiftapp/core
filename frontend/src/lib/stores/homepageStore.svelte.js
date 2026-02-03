/**
 * Store for managing Homepage state.
 * Uses Svelte 5 class-based reactive state pattern.
 * Centralizes dashboard data, activity, and UI state.
 */
import { api } from '../api.js';

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

  // === Greeting ===
  greeting = $state('');
  currentDate = $state('');

  // === Derived Values ===

  /**
   * Check if in onboarding mode.
   */
  get isOnboarding() {
    return (
      (this.totalWorkspaceCount === 0 || this.totalItemCount === 0) && !this.onboardingDismissed
    );
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
    await this.loadDashboardData();
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

      // Load notifications
      const notificationsData = await api.notifications.getAll({ limit: 5 });
      this.notifications = notificationsData || [];
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
    this.currentDate = now.toLocaleDateString('en-US', {
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
    const diffMs = now - then;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
    if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
    if (diffDays < 7) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;

    return then.toLocaleDateString();
  }

  /**
   * Calculate days until a target date.
   */
  calculateDaysUntil(targetDate) {
    if (!targetDate) return null;

    const now = new Date();
    const target = new Date(targetDate);
    const diffTime = target - now;
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
    this.greeting = '';
    this.currentDate = '';
  }
}

export const homepageStore = new HomepageStore();
