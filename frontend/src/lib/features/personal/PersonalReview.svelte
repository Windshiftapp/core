<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { authStore, uiStore } from '../../stores';
  import { ChevronLeft, ChevronRight, Calendar, BookOpen, Clock, Lightbulb, Maximize, Minimize, BookOpenCheck, FileEdit } from 'lucide-svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Card from '../../components/Card.svelte';
  import WorkItemRow from '../items/WorkItemRow.svelte';
  import { scale } from 'svelte/transition';
  import PageHeader from '../../layout/PageHeader.svelte';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import Button from '../../components/Button.svelte';
  import { getShortcut, matchesShortcut, toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateSimple, formatDateWithOptions } from '../../utils/dateFormatter.js';

  // Props (Svelte 5)
  let { currentUser = null } = $props();

  // State (Svelte 5)
  let currentDate = $state(new Date().toISOString().split('T')[0]);
  let reviewType = $state(localStorage.getItem('reviewType') || 'daily');
  let reviewContent = $state('');
  let completedItems = $state([]);
  let itemTypes = $state([]);
  let existingReview = $state(null);
  let loading = $state(false);
  let saving = $state(false);
  let autoSaveTimeout = $state(null);
  let reviewHistory = $state([]);
  let showHistory = $state(false);
  let previousContent = $state('');
  let initialLoadDone = $state(false);

  // Keyboard shortcut for save
  const saveShortcut = getShortcut('description', 'save');

  // Calendar week calculation
  function getWeekDates(date) {
    const d = new Date(date);
    const day = d.getDay();
    const diff = d.getDate() - day + (day === 0 ? -6 : 1);
    const monday = new Date(d.setDate(diff));
    const sunday = new Date(monday);
    sunday.setDate(monday.getDate() + 6);

    return {
      start: monday.toISOString().split('T')[0],
      end: sunday.toISOString().split('T')[0],
      week: `Week of ${formatDateWithOptions(monday, { month: 'short', day: 'numeric' })} - ${formatDateWithOptions(sunday, { month: 'short', day: 'numeric', year: 'numeric' })}`
    };
  }

  // Template for new reviews
  function getDefaultTemplate() {
    if (reviewType === 'daily') {
      return `## ${t('personal.whatAccomplished')}

${t('personal.placeholderAccomplishments')}

## ${t('personal.whatWentWell')}

${t('personal.placeholderWentWell')}

## ${t('personal.whatImprove')}

${t('personal.placeholderImprovements')}`;
    } else {
      return `## ${t('personal.weeklyAccomplishments')}

...

## ${t('personal.weeklyChallenges')}

...

## ${t('personal.weeklyPriorities')}

...`;
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
  }

  function goToToday() {
    currentDate = new Date().toISOString().split('T')[0];
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

      const [items, types] = await Promise.all([
        api.reviews.getCompletedItems(startDate, endDate),
        api.itemTypes.getAll()
      ]);
      completedItems = items || [];
      itemTypes = types || [];
    } catch (error) {
      console.error('Failed to load completed items:', error);
      completedItems = [];
    }
  }

  // Load existing review for the current date and type
  async function loadReview() {
    loading = true;
    try {
      await loadCompletedItems();

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
          // Support both old format (separate fields) and new format (single content)
          if (data.content) {
            reviewContent = data.content;
          } else {
            // Migrate old format to new
            reviewContent = `## ${t('personal.whatAccomplished')}\n\n${data.accomplishments || ''}\n\n## ${t('personal.whatWentWell')}\n\n${data.went_well || ''}\n\n## ${t('personal.whatImprove')}\n\n${data.improvements || ''}`;
          }
        } catch (e) {
          console.error('Failed to parse review data:', e);
          reviewContent = getDefaultTemplate();
        }
      } else {
        existingReview = null;
        reviewContent = getDefaultTemplate();
      }
      previousContent = reviewContent;
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

  // Auto-save functionality
  function scheduleAutoSave() {
    if (autoSaveTimeout) {
      clearTimeout(autoSaveTimeout);
    }
    autoSaveTimeout = setTimeout(() => {
      saveReview(true);
    }, 2000);
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
          content: reviewContent,
          completed_items: completedItems.map(item => item.id),
          date: currentDate,
          week_start: reviewType === 'weekly' ? getWeekDates(currentDate).start : null
        })
      };

      if (existingReview) {
        const updated = await api.reviews.update(existingReview.id, {
          review_data: payload.review_data
        });
        existingReview = updated;
      } else {
        existingReview = await api.reviews.create(payload);
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
    loadReviewHistory();
  }

  // Format date for display
  function formatDisplayDate(dateStr, type) {
    const date = new Date(dateStr);
    if (type === 'daily') {
      return formatDateWithOptions(date, {
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

  // Keyboard shortcut handler
  function handleKeydown(event) {
    if (matchesShortcut(event, saveShortcut)) {
      event.preventDefault();
      saveReview(false);
    }
  }

  // Derived values (Svelte 5)
  const displayDate = $derived(formatDisplayDate(currentDate, reviewType));

  // Effects (Svelte 5)
  $effect(() => {
    // Track dependencies and reload when they change
    const type = reviewType;
    const date = currentDate;
    loadReview();
  });

  // Track content changes for auto-save
  $effect(() => {
    const content = reviewContent;
    if (initialLoadDone && content !== previousContent && !loading) {
      previousContent = content;
      scheduleAutoSave();
    }
  });

  // Initialize
  onMount(() => {
    authStore.subscribe(auth => {
      currentUser = auth.user;
    });

    loadReviewHistory();
    initialLoadDone = true;
  });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
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
                        {formatDateSimple(historyItem.review_date)}
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
          <div class="w-7 h-7 rounded-md flex items-center justify-center" style="background-color: var(--ds-accent-purple);">
            <BookOpenCheck class="w-4 h-4" style="color: white;" />
          </div>
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
          <div class="space-y-2">
            {#each completedItems as item (item.id)}
              <WorkItemRow {item} {itemTypes} showWorkspace={true} />
            {/each}
          </div>
        {/if}
      </div>

      <!-- Reflection Section with Single Editor -->
      <Card variant="raised" padding="spacious" shadow={true}>
        <div class="flex items-center space-x-3 mb-6">
          <div class="w-7 h-7 rounded-md flex items-center justify-center" style="background-color: var(--ds-accent-teal);">
            <Lightbulb class="w-4 h-4" style="color: white;" />
          </div>
          <h2 class="text-xl font-light" style="color: var(--ds-text);">{t('personal.reflection')}</h2>
          {#if saving}
            <div class="flex items-center space-x-2 text-sm" style="color: var(--ds-text-subtle);">
              <div class="animate-spin w-4 h-4 border-2 border-t-transparent rounded-full" style="border-color: var(--ds-text-subtle); border-top-color: transparent;"></div>
              <span>{t('common.saving')}</span>
            </div>
          {/if}
        </div>

        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div onkeydown={handleKeydown} class="review-editor-container">
          <MilkdownEditor
            bind:content={reviewContent}
            placeholder={t('personal.startWriting')}
            showToolbar={true}
          />
        </div>

        <div class="flex justify-end pt-6">
          <Button
            onclick={() => saveReview(false)}
            disabled={saving}
            variant="primary"
            hotkeyConfig={{ key: toHotkeyString('description', 'save') }}
          >
            {saving ? t('common.saving') : t('personal.saveReview')}
          </Button>
        </div>
      </Card>
    </div>
  </div>
</div>

<style>
  :global(.review-editor-container .milkdown-editor .milkdown) {
    min-height: 300px;
  }
</style>
