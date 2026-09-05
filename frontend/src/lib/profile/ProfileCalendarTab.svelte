<script>
  import { onDestroy } from 'svelte';
  import { CalendarDays, Copy, Eye, EyeOff, Link2, RefreshCw, Trash2 } from '@lucide/svelte';
  import { createCalendarFeedToken, getCalendarFeedToken, revokeCalendarFeedToken } from '../api.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Spinner from '../components/Spinner.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { t } from '../stores/i18n.svelte.js';
  import { copyToClipboard } from '../utils/clipboard.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';

  let feedInfo = $state(null);
  let loading = $state(false);
  let error = $state('');
  let generating = $state(false);
  let revoking = $state(false);
  let showFullURL = $state(false);
  let copied = $state(false);
  let copiedTimer = null;

  onDestroy(() => {
    if (copiedTimer) clearTimeout(copiedTimer);
  });

  async function load() {
    loading = true;
    error = '';
    try {
      feedInfo = await getCalendarFeedToken();
    } catch (err) {
      error = err.message?.includes('disabled')
        ? t('users.calendarFeedsDisabled')
        : err.message || t('dialogs.alerts.failedToLoad', { error: 'calendar feed info' });
    } finally {
      loading = false;
    }
  }

  async function generate() {
    generating = true;
    error = '';
    try {
      await createCalendarFeedToken();
      await load();
      showFullURL = true;
    } catch (err) {
      error = err.message?.includes('disabled')
        ? t('users.calendarFeedsDisabled')
        : err.message || t('dialogs.alerts.failedToCreate', { error: 'calendar feed' });
    } finally {
      generating = false;
    }
  }

  async function revoke() {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.revokeCalendarFeed'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    revoking = true;
    error = '';
    try {
      await revokeCalendarFeedToken();
      feedInfo = { has_token: false };
      showFullURL = false;
    } catch (err) {
      error = err.message || t('dialogs.alerts.failedToDelete', { error: 'calendar feed' });
    } finally {
      revoking = false;
    }
  }

  async function copyURL() {
    const url = feedInfo?.feed?.feed_url;
    if (!url) return;
    await copyToClipboard(url);
    copied = true;
    if (copiedTimer) clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => (copied = false), 2000);
  }

  function maskedURL(url) {
    if (!url || url.length <= 70) return url || '';
    return `${url.substring(0, 40)}...${url.substring(url.length - 20)}`;
  }

  const subscriptionSteps = $derived([
    { description: t('users.copyFeedUrlStep') },
    { title: t('users.googleCalendar'), description: t('users.googleCalendarInstructions') },
    { title: t('users.outlook'), description: t('users.outlookInstructions') },
    { title: t('users.appleCalendar'), description: t('users.appleCalendarInstructions') },
  ]);
</script>

<div class="mb-6">
  <h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
    <CalendarDays class="h-5 w-5" style="color: var(--ds-text-subtle);" />
    {t('users.calendarIntegration')}
  </h2>
  <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.calendarIntegrationDesc')}</p>
</div>

{#if error}
  <AlertBox message={error} />
{/if}

{#if loading}
  <div class="flex items-center justify-center py-8"><Spinner size="md" /></div>
{:else if !feedInfo}
  <div class="py-4">
    <Button variant="default" onclick={load}>{t('users.loadCalendarFeedSettings')}</Button>
  </div>
{:else if !feedInfo.has_token}
  <div
    class="border rounded-lg p-6"
    style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
  >
    <div class="flex items-start gap-4">
      <div class="p-3 rounded-lg" style="background-color: var(--ds-background-neutral);">
        <Link2 class="w-6 h-6" style="color: var(--ds-icon);" />
      </div>
      <div class="flex-1">
        <h3 class="text-base font-medium" style="color: var(--ds-text);">
          {t('users.enableCalendarSubscription')}
        </h3>
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          {t('users.calendarSubscriptionDesc')}
        </p>
        <div class="mt-4">
          <Button variant="primary" onclick={generate} disabled={generating} icon={CalendarDays}>
            {generating ? t('common.generating') : t('users.generateCalendarFeedUrl')}
          </Button>
        </div>
      </div>
    </div>
  </div>
{:else}
  <div class="space-y-6">
    <div
      class="border rounded-lg p-6"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
    >
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-base font-medium" style="color: var(--ds-text);">{t('users.yourCalendarFeedUrl')}</h3>
        <button
          class="text-sm px-2 py-1 rounded hover-bg"
          style="color: var(--ds-link);"
          onclick={() => (showFullURL = !showFullURL)}
        >
          {#if showFullURL}
            <EyeOff class="w-4 h-4 inline mr-1" /> {t('common.hide')}
          {:else}
            <Eye class="w-4 h-4 inline mr-1" /> {t('users.showFullUrl')}
          {/if}
        </button>
      </div>
      <div class="flex items-center gap-2">
        <Input
          type="text"
          readonly
          value={showFullURL ? feedInfo.feed?.feed_url : maskedURL(feedInfo.feed?.feed_url)}
          class="flex-1 font-mono"
        />
        <Button variant="default" onclick={copyURL} icon={Copy} size="small">
          {copied ? t('toast.copied') : t('common.copy')}
        </Button>
      </div>
      <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">{t('users.calendarFeedWarning')}</p>
      {#if feedInfo.feed?.last_accessed_at}
        <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
          {t('users.lastSynced')}: {formatDateSimple(feedInfo.feed.last_accessed_at)}
        </p>
      {/if}
    </div>

    <div
      class="border rounded-lg p-6"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
    >
      <h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">{t('users.howToSubscribe')}</h3>
      <div class="space-y-4 text-sm" style="color: var(--ds-text-subtle);">
        {#each subscriptionSteps as step, index}
          <div class="flex items-start gap-3">
            <span
              class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium"
              style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
            >{index + 1}</span>
            <div>
              {#if step.title}
                <p class="font-medium" style="color: var(--ds-text);">{step.title}</p>
              {/if}
              <p>{step.description}</p>
            </div>
          </div>
        {/each}
      </div>
    </div>

    <div class="flex items-center gap-4">
      <Button variant="default" onclick={generate} disabled={generating} icon={RefreshCw}>
        {generating ? t('common.regenerating') : t('users.regenerateUrl')}
      </Button>
      <Button variant="danger" onclick={revoke} disabled={revoking} icon={Trash2}>
        {revoking ? t('common.revoking') : t('users.revokeFeed')}
      </Button>
    </div>
    <p class="text-xs" style="color: var(--ds-text-subtle);">
      <strong>{t('common.note')}:</strong> {t('users.regenerateUrlNote')}
    </p>
  </div>
{/if}
