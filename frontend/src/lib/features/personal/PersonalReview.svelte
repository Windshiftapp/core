<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { authStore, uiStore } from '../../stores';
  import { ChevronLeft, ChevronRight, Calendar, BookOpen, CheckSquare, Clock, Lightbulb, Maximize, Minimize, BookOpenCheck, FileEdit } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { scale } from 'svelte/transition';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  // Props
  export let currentUser = null;

  // State
  let currentDate = new Date().toISOString().split('T')[0]; // YYYY-MM-DD format
  let reviewType = localStorage.getItem('reviewType') || 'daily';
  let reviewData = {
    accomplishments: '',
    went_well: '',
    improvements: '',
    mood: '',
    tags: []
  };
  let completedItems = [];
  let existingReview = null;
  let loading = false;
  let saving = false;
  let autoSaveTimeout = null;
  let reviewHistory = [];
  let showHistory = false;

  // Calendar week calculation
  function getWeekDates(date) {
    const d = new Date(date);
    const day = d.getDay();
    const diff = d.getDate() - day + (day === 0 ? -6 : 1); // adjust when day is sunday
    const monday = new Date(d.setDate(diff));
    const sunday = new Date(monday);
    sunday.setDate(monday.getDate() + 6);
    
    return {
      start: monday.toISOString().split('T')[0],
      end: sunday.toISOString().split('T')[0],
      week: `Week of ${monday.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} - ${sunday.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}`
    };
  }

  // Get prompts based on review type
  function getPrompts() {
    if (reviewType === 'daily') {
      return {
        accomplishments: t('personal.whatAccomplished'),
        went_well: t('personal.whatWentWell'),
        improvements: t('personal.whatImprove')
      };
    } else {
      return {
        accomplishments: t('personal.weeklyAccomplishments'),
        went_well: t('personal.weeklyChallenges'),
        improvements: t('personal.weeklyPriorities')
      };
    }
  }

  // Navigation functions
  function navigateDate(direction) {
    const date = new Date(currentDate);
    if (reviewType === 'daily') {
      date.setDate(date.getDate() + direction);
    } else {
      date.setDate(date.getDate() + (direction * 7));
    }
    currentDate = date.toISOString().split('T')[0];
    loadReview();
  }

  function goToToday() {
    currentDate = new Date().toISOString().split('T')[0];
    loadReview();
  }

  // Load completed items for the date range
  async function loadCompletedItems() {
    try {
      let startDate, endDate;
      
      if (reviewType === 'daily') {
        startDate = endDate = currentDate;
      } else {
        const week = getWeekDates(currentDate);
        startDate = week.start;
        endDate = week.end;
      }

      const items = await api.reviews.getCompletedItems(startDate, endDate);
      completedItems = items || [];
    } catch (error) {
      console.error('Failed to load completed items:', error);
      completedItems = [];
    }
  }

  // Load existing review for the current date and type
  async function loadReview() {
    loading = true;
    try {
      // Load completed items first
      await loadCompletedItems();

      // Check for existing review
      const reviews = await api.reviews.getAll({
        type: reviewType,
        start_date: currentDate,
        end_date: currentDate,
        limit: 1
      });

      if (reviews && reviews.length > 0) {
        existingReview = reviews[0];
        try {
          const data = JSON.parse(existingReview.review_data);
          reviewData = {
            accomplishments: data.accomplishments || '',
            went_well: data.went_well || '',
            improvements: data.improvements || '',
            mood: data.mood || '',
            tags: data.tags || []
          };
        } catch (e) {
          console.error('Failed to parse review data:', e);
          resetReviewData();
        }
      } else {
        existingReview = null;
        resetReviewData();
      }
    } catch (error) {
      console.error('Failed to load review:', error);
    } finally {
      loading = false;
    }
  }

  // Load recent review history
  async function loadReviewHistory() {
    try {
      const history = await api.reviews.getAll({
        type: reviewType,
        limit: 10
      });
      reviewHistory = history || [];
    } catch (error) {
      console.error('Failed to load review history:', error);
      reviewHistory = [];
    }
  }

  function resetReviewData() {
    reviewData = {
      accomplishments: '',
      went_well: '',
      improvements: '',
      mood: '',
      tags: []
    };
  }

  // Auto-save functionality
  function scheduleAutoSave() {
    if (autoSaveTimeout) {
      clearTimeout(autoSaveTimeout);
    }
    autoSaveTimeout = setTimeout(() => {
      saveReview(true);
    }, 2000); // Auto-save after 2 seconds of inactivity
  }

  // Save review
  async function saveReview(isAutoSave = false) {
    if (saving) return;
    
    saving = true;
    try {
      const payload = {
        review_date: currentDate,
        review_type: reviewType,
        review_data: JSON.stringify({
          ...reviewData,
          completed_items: completedItems.map(item => item.id),
          date: currentDate,
          week_start: reviewType === 'weekly' ? getWeekDates(currentDate).start : null
        })
      };

      if (existingReview) {
        // Update existing review
        const updated = await api.reviews.update(existingReview.id, {
          review_data: payload.review_data
        });
        existingReview = updated;
      } else {
        // Create new review
        existingReview = await api.reviews.create(payload);
      }

      if (!isAutoSave) {
        // Show success feedback for manual saves
      }
    } catch (error) {
      console.error('Failed to save review:', error);
    } finally {
      saving = false;
    }
  }

  // Handle review type change
  function handleTypeChange() {
    localStorage.setItem('reviewType', reviewType);
    loadReview();
    loadReviewHistory();
  }

  // Format date for display
  function formatDisplayDate(dateStr = currentDate, type = reviewType) {
    const date = new Date(dateStr);
    if (type === 'daily') {
      return date.toLocaleDateString('en-US', { 
        weekday: 'long', 
        year: 'numeric', 
        month: 'long', 
        day: 'numeric' 
      });
    } else {
      return getWeekDates(dateStr).week;
    }
  }

  // Handle history item click
  function goToHistoryDate(historyItem) {
    currentDate = historyItem.review_date;
    reviewType = historyItem.review_type;
    loadReview();
    showHistory = false;
  }

  // Toggle fullscreen mode
  function toggleFullscreen() {
    uiStore.toggleReviewFullscreen();
  }

  // Close history popover when clicking outside
  function handleClickOutside(event) {
    if (showHistory && !event.target.closest('.history-popover-container')) {
      showHistory = false;
    }
  }

  // Initialize
  onMount(() => {
    authStore.subscribe(auth => {
      currentUser = auth.user;
    });
    
    loadReview();
    loadReviewHistory();
  });

  // Reactive statements
  $: if (reviewType || currentDate) {
    loadReview();
  }
  
  // Reactive date display update
  $: displayDate = formatDisplayDate(currentDate, reviewType);
</script>

<div class="p-6" style="background-color: var(--ds-surface); min-height: 100vh;" onclick={handleClickOutside}>
  <PageHeader
    icon={FileEdit}
    title={t('personal.personalReview')}
    subtitle="{displayDate}"
  >
    {#snippet actions()}
      <div class="flex items-center space-x-4">
        <!-- Date Navigation -->
        <div class="flex items-center space-x-2">
          <button
            class="p-2 rounded transition-colors"
            style="color: var(--ds-text-subtle); hover:background-color: var(--ds-background-neutral-hovered);"
            onclick={() => navigateDate(-1)}
          >
            <ChevronLeft class="w-4 h-4" />
          </button>
          <input
            type="date"
            bind:value={currentDate}
            onchange={loadReview}
            class="text-sm border-none bg-transparent cursor-pointer rounded px-2 py-1 transition-colors"
            style="color: var(--ds-text-subtle); hover:background-color: var(--ds-background-neutral-hovered);"
          />
          <button
            class="p-2 rounded transition-colors"
            style="color: var(--ds-text-subtle); hover:background-color: var(--ds-background-neutral-hovered);"
            onclick={() => navigateDate(1)}
          >
            <ChevronRight class="w-4 h-4" />
          </button>
          <button
            class="text-xs px-2 py-1 rounded transition-colors"
            style="color: var(--ds-text-subtle); hover:background-color: var(--ds-background-neutral-hovered); hover:color: var(--ds-text);"
            onclick={goToToday}
          >
            {t('personal.today')}
          </button>
        </div>

        <!-- Review Type Toggle -->
        <div class="flex rounded p-1" style="background-color: var(--ds-surface-raised);">
          <button
            class="px-3 py-1 text-sm font-medium rounded-md transition-colors"
            class:active={reviewType === 'daily'}
            style="background-color: {reviewType === 'daily' ? 'var(--ds-surface)' : 'transparent'}; color: var(--ds-text);"
            onclick={() => { reviewType = 'daily'; handleTypeChange(); }}
          >
            {t('personal.daily')}
          </button>
          <button
            class="px-3 py-1 text-sm font-medium rounded-md transition-colors"
            class:active={reviewType === 'weekly'}
            style="background-color: {reviewType === 'weekly' ? 'var(--ds-surface)' : 'transparent'}; color: var(--ds-text);"
            onclick={() => { reviewType = 'weekly'; handleTypeChange(); }}
          >
            {t('personal.weekly')}
          </button>
        </div>

        <div class="relative history-popover-container">
          <button
            class="p-2 rounded transition-colors"
            style="color: var(--ds-text-subtle); hover:background-color: var(--ds-background-neutral-hovered);"
            onclick={() => showHistory = !showHistory}
          >
            <Calendar class="w-4 h-4" />
          </button>

          <!-- History Popover -->
          {#if showHistory}
            <div
              class="absolute right-0 top-12 w-80 rounded shadow-lg border z-50 p-4"
              style="background-color: var(--ds-surface); border-color: var(--ds-border);"
              transition:scale={{ duration: 150, start: 0.95 }}
            >
              <div class="flex items-center space-x-3 mb-4">
                <Clock class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                <h3 class="text-sm font-medium" style="color: var(--ds-text);">{t('personal.recentReviews')}</h3>
              </div>

              {#if reviewHistory.length === 0}
                <p class="text-sm" style="color: var(--ds-text-subtle);">{t('personal.noPreviousReviews')}</p>
              {:else}
                <div class="space-y-1 max-h-64 overflow-y-auto">
                  {#each reviewHistory as historyItem (historyItem.id)}
                    <button
                      class="w-full text-left p-2 rounded-md transition-colors text-sm"
                      style="background-color: {historyItem.review_date === currentDate && historyItem.review_type === reviewType ? 'var(--ds-background-brand-subtle)' : 'transparent'}; hover:background-color: var(--ds-background-neutral-hovered);"
                      onclick={() => {
                        goToHistoryDate(historyItem);
                        showHistory = false;
                      }}
                    >
                      <div class="font-medium" style="color: var(--ds-text);">
                        {new Date(historyItem.review_date).toLocaleDateString()}
                      </div>
                      <div class="text-xs capitalize mt-1" style="color: var(--ds-text-subtle);">
                        {historyItem.review_type === 'daily' ? t('personal.dailyReview') : t('personal.weeklyReview')}
                      </div>
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <button
          class="p-2 rounded transition-colors"
          style="color: var(--ds-text-subtle); hover:background-color: var(--ds-background-neutral-hovered);"
          onclick={toggleFullscreen}
          title={$uiStore.reviewFullscreen ? t('personal.exitFocusMode') : t('personal.enterFocusMode')}
        >
          {#if $uiStore.reviewFullscreen}
            <Minimize class="w-4 h-4" />
          {:else}
            <Maximize class="w-4 h-4" />
          {/if}
        </button>
      </div>
    {/snippet}
  </PageHeader>

  <div class="flex justify-center">
    <!-- Main Review Content -->
    <div class="{$uiStore.reviewFullscreen ? 'w-full max-w-4xl space-y-12' : 'w-full max-w-3xl space-y-12'}">
      <!-- Completed Items -->
      <div>
        <div class="flex items-center space-x-3 mb-8">
          <BookOpenCheck class="w-5 h-5" style="color: var(--ds-text-success);" />
          <h2 class="text-xl font-light" style="color: var(--ds-text);">
            {reviewType === 'daily' ? t('personal.completedToday') : t('personal.completedThisWeek')}
          </h2>
          <span class="text-sm" style="color: var(--ds-text-subtle);">
            {completedItems.length}
          </span>
        </div>
          
        {#if loading}
          <div class="text-center py-8">
            <div class="animate-spin w-6 h-6 border-2 border-t-transparent rounded-full mx-auto" style="border-color: var(--ds-background-brand); border-top-color: transparent;"></div>
            <p class="mt-2" style="color: var(--ds-text-subtle);">{t('personal.loadingCompletedItems')}</p>
          </div>
        {:else if completedItems.length === 0}
          <EmptyState
            icon={BookOpenCheck}
            title={reviewType === 'daily' ? t('personal.noCompletedItemsDay') : t('personal.noCompletedItemsWeek')}
          />
        {:else}
          <div class="space-y-4">
            {#each completedItems as item (item.id)}
              <div class="flex items-start space-x-4 py-3" transition:scale>
                <CheckSquare class="w-4 h-4 mt-1 flex-shrink-0" style="color: var(--ds-text-success);" />
                <div class="flex-1 min-w-0">
                  <p class="font-medium" style="color: var(--ds-text);">{item.title}</p>
                  <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                    {item.workspace_name} • {new Date(item.completed_at || item.updated_at).toLocaleDateString()}
                  </p>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>

        <!-- Reflection Questions -->
        <div>
          <div class="flex items-center space-x-3 mb-8">
            <Lightbulb class="w-5 h-5" style="color: var(--ds-text-warning);" />
            <h2 class="text-xl font-light" style="color: var(--ds-text);">{t('personal.reflection')}</h2>
            {#if saving}
              <div class="flex items-center space-x-2 text-sm" style="color: var(--ds-text-subtle);">
                <div class="animate-spin w-4 h-4 border-2 border-t-transparent rounded-full" style="border-color: var(--ds-text-subtle); border-top-color: transparent;"></div>
                <span>{t('common.saving')}</span>
              </div>
            {/if}
          </div>

          <div class="space-y-12">
            <!-- Accomplishments -->
            <div>
              <label class="block text-sm font-light mb-4" style="color: var(--ds-text-subtle);">
                {getPrompts().accomplishments}
              </label>
              <Textarea
                bind:value={reviewData.accomplishments}
                oninput={scheduleAutoSave}
                class="h-32 border-0"
                placeholder={t('personal.placeholderAccomplishments')}
              />
            </div>

            <!-- What Went Well -->
            <div>
              <label class="block text-sm font-light mb-4" style="color: var(--ds-text-subtle);">
                {getPrompts().went_well}
              </label>
              <Textarea
                bind:value={reviewData.went_well}
                oninput={scheduleAutoSave}
                class="h-32 border-0"
                placeholder={t('personal.placeholderWentWell')}
              />
            </div>

            <!-- Improvements -->
            <div>
              <label class="block text-sm font-light mb-4" style="color: var(--ds-text-subtle);">
                {getPrompts().improvements}
              </label>
              <Textarea
                bind:value={reviewData.improvements}
                oninput={scheduleAutoSave}
                class="h-32 border-0"
                placeholder={t('personal.placeholderImprovements')}
              />
            </div>

            <!-- Save Button -->
            <div class="flex justify-end pt-8">
              <button
                onclick={() => saveReview(false)}
                disabled={saving}
                class="px-8 py-3 rounded font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200"
                style="background-color: var(--ds-background-brand); color: var(--ds-text-on-brand);"
              >
                {saving ? t('common.saving') : t('personal.saveReview')}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
