<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { Sparkles, CalendarPlus, RotateCcw, CheckCircle, AlertCircle } from 'lucide-svelte';
  import { authStore } from '../../stores';
  import { navigate } from '../../router.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Card from '../../components/Card.svelte';
  import Button from '../../components/Button.svelte';

  let loading = $state(false);
  let scheduling = $state(false);
  let plan = $state(null);
  let error = $state('');
  let scheduled = $state(false);
  let scheduleErrors = $state([]);
  let connections = $state([]);
  let selectedConnectionId = $state(null);

  onMount(async () => {
    try {
      connections = await api.llmProviders.getForFeature('plan_my_day');
      const def = connections.find(c => c.is_default);
      if (def) selectedConnectionId = def.id;
    } catch (e) { /* silent — fallback still works */ }
  });

  async function generate() {
    loading = true;
    error = '';
    plan = null;
    scheduled = false;
    scheduleErrors = [];

    try {
      plan = await api.ai.planMyDay(selectedConnectionId);
    } catch (err) {
      console.error('Plan My Day failed:', err);
      error = err.message || 'Failed to generate plan. Is the LLM service running?';
    } finally {
      loading = false;
    }
  }

  function formatDuration(minutes) {
    if (minutes >= 60) {
      const h = Math.floor(minutes / 60);
      const m = minutes % 60;
      return m > 0 ? `${h}h ${m}m` : `${h}h`;
    }
    return `${minutes}m`;
  }

  async function addToCalendar() {
    scheduling = true;
    scheduleErrors = [];

    const user = authStore.currentUser;
    if (!user) {
      error = 'Not authenticated.';
      scheduling = false;
      return;
    }

    const today = new Date();
    const scheduledDate = today.getFullYear() + '-' +
      String(today.getMonth() + 1).padStart(2, '0') + '-' +
      String(today.getDate()).padStart(2, '0');

    const promises = plan.activities.map(async (activity) => {
      const res = await fetch(`/api/items/${activity.item_id}/schedule`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          user_id: user.id,
          workspace_id: activity.workspace_id,
          scheduled_date: scheduledDate,
          scheduled_time: activity.time,
          duration_minutes: activity.duration_minutes,
        }),
      });
      if (!res.ok) {
        const text = await res.text().catch(() => 'Unknown error');
        throw new Error(`Failed to schedule ${activity.item_key}: ${text}`);
      }
      return activity;
    });

    const results = await Promise.allSettled(promises);
    const failed = results
      .filter((r) => r.status === 'rejected')
      .map((r) => r.reason.message);

    if (failed.length === 0) {
      scheduled = true;
    } else if (failed.length < results.length) {
      scheduleErrors = failed;
      scheduled = true;
    } else {
      error = 'Failed to schedule all items.';
      scheduleErrors = failed;
    }

    scheduling = false;
  }
</script>

<div class="p-6" style="background-color: var(--ds-surface); min-height: 100vh;">
  <PageHeader
    icon={Sparkles}
    title="Plan My Day"
    subtitle="AI-powered daily planning based on your assigned items"
  />

  <div class="max-w-3xl space-y-6">
    {#if !plan && !loading}
      <!-- Initial state: Generate button -->
      <Card variant="raised" padding="default">
        <div class="flex flex-col items-center py-8 gap-4">
          <div class="rounded-full p-4" style="background-color: var(--ds-surface-sunken);">
            <Sparkles size={32} style="color: var(--ds-text-subtle);" />
          </div>
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            Generate a prioritized daily schedule from your open work items.
          </p>
          {#if connections.length > 0}
            <div class="flex items-center gap-2">
              <label class="text-xs" style="color: var(--ds-text-subtle);">AI Model:</label>
              <select
                bind:value={selectedConnectionId}
                class="text-sm rounded px-2 py-1 border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              >
                {#each connections as conn}
                  <option value={conn.id}>{conn.name}</option>
                {/each}
              </select>
            </div>
          {/if}
          <Button variant="primary" onclick={generate}>
            Generate Plan
          </Button>
        </div>
      </Card>
    {/if}

    {#if loading}
      <!-- Loading state -->
      <Card variant="raised" padding="default">
        <div class="flex flex-col items-center py-8 gap-3">
          <div class="animate-spin rounded-full h-8 w-8 border-2 border-t-transparent" style="border-color: var(--ds-border); border-top-color: transparent;"></div>
          <p class="text-sm" style="color: var(--ds-text-subtle);">Generating your daily plan...</p>
        </div>
      </Card>
    {/if}

    {#if error}
      <Card variant="outlined" padding="default">
        <div class="flex items-start gap-2">
          <AlertCircle size={16} style="color: var(--ds-text-danger); flex-shrink: 0; margin-top: 2px;" />
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        </div>
      </Card>
    {/if}

    {#if scheduleErrors.length > 0}
      <Card variant="outlined" padding="default">
        <p class="text-sm font-medium mb-2" style="color: var(--ds-text-danger);">Some items failed to schedule:</p>
        <ul class="text-sm space-y-1" style="color: var(--ds-text-danger);">
          {#each scheduleErrors as err}
            <li>• {err}</li>
          {/each}
        </ul>
      </Card>
    {/if}

    {#if plan && !loading && !scheduled}
      <!-- Plan review view -->
      {#if plan.summary}
        <Card variant="raised" padding="default">
          <p class="text-sm" style="color: var(--ds-text);">{plan.summary}</p>
        </Card>
      {/if}

      {#if plan.activities && plan.activities.length > 0}
        <div class="space-y-2">
          {#each plan.activities as activity, i}
            <Card variant="raised" padding="default">
              <div class="flex items-start gap-4">
                <!-- Time badge -->
                <div class="flex flex-col items-center flex-shrink-0 pt-0.5" style="min-width: 56px;">
                  <span class="text-sm font-mono font-semibold" style="color: var(--ds-text);">{activity.time}</span>
                  <span class="text-xs" style="color: var(--ds-text-subtle);">{formatDuration(activity.duration_minutes)}</span>
                </div>

                <!-- Divider line -->
                <div class="flex flex-col items-center flex-shrink-0 pt-1">
                  <div class="w-2.5 h-2.5 rounded-full" style="background-color: var(--ds-icon-accent);"></div>
                  {#if i < plan.activities.length - 1}
                    <div class="w-px flex-1 mt-1" style="background-color: var(--ds-border); min-height: 24px;"></div>
                  {/if}
                </div>

                <!-- Content -->
                <div class="flex-1 min-w-0 pb-1">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-xs font-mono px-1.5 py-0.5 rounded" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">{activity.item_key}</span>
                    <span class="text-sm font-medium truncate" style="color: var(--ds-text);">{activity.title}</span>
                  </div>
                  {#if activity.reason}
                    <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">{activity.reason}</p>
                  {/if}
                </div>
              </div>
            </Card>
          {/each}
        </div>

        <!-- Action buttons -->
        <div class="flex items-center gap-3">
          <Button variant="primary" onclick={addToCalendar} loading={scheduling} icon={CalendarPlus}>
            {#if scheduling}
              Adding to Calendar...
            {:else}
              Add to Calendar
            {/if}
          </Button>
          <Button variant="secondary" onclick={generate} icon={RotateCcw}>
            Regenerate
          </Button>
        </div>
      {:else}
        <Card variant="raised" padding="default">
          <p class="text-sm" style="color: var(--ds-text-subtle);">No activities were generated. Try again or check that you have open items assigned to you.</p>
        </Card>
        <Button variant="secondary" onclick={generate} icon={RotateCcw}>
          Regenerate
        </Button>
      {/if}
    {/if}

    {#if scheduled}
      <!-- Success state -->
      <Card variant="raised" padding="default">
        <div class="flex flex-col items-center py-6 gap-3">
          <div class="rounded-full p-3" style="background-color: color-mix(in srgb, var(--ds-icon-success) 15%, transparent);">
            <CheckCircle size={28} style="color: var(--ds-icon-success);" />
          </div>
          <p class="text-sm font-medium" style="color: var(--ds-text);">
            {scheduleErrors.length > 0
              ? `Scheduled ${plan.activities.length - scheduleErrors.length} of ${plan.activities.length} items`
              : 'All items have been added to your calendar'}
          </p>
          <div class="flex items-center gap-3">
            <Button variant="primary" onclick={() => navigate('/personal/calendar')} icon={CalendarPlus}>
              View Calendar
            </Button>
            <Button variant="secondary" onclick={generate} icon={RotateCcw}>
              Plan Again
            </Button>
          </div>
        </div>
      </Card>
    {/if}
  </div>
</div>
