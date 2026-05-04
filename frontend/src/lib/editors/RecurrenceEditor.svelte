<script>
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { parseRRule, buildRRule, rruleToText, DAY_NAMES, DAY_LABELS, FREQ_LABELS } from './rruleUtils.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { Save, Trash2, X, RefreshCw, Eye } from 'lucide-svelte';

  let {
    itemId,
    existingRule = null,
    statusOptions = [],
    compact = false,
    onsave = null,
    oncancel = null,
    ondelete = null,
  } = $props();

  // Parse existing rule into form state
  const initial = existingRule
    ? parseRRule(existingRule.rrule)
    : parseRRule('');

  let frequency = $state(initial.frequency);
  let interval = $state(initial.interval);
  let byDay = $state(initial.byDay.length > 0 ? [...initial.byDay] : []);
  let byMonthDay = $state(initial.byMonthDay || 1);
  let endType = $state(initial.endType);
  let endDate = $state(initial.endDate || '');
  let count = $state(initial.count || 10);

  // Copy settings
  let copyAssignee = $state(existingRule?.copy_assignee ?? true);
  let copyPriority = $state(existingRule?.copy_priority ?? true);
  let copyCustomFields = $state(existingRule?.copy_custom_fields ?? true);
  let copyDescription = $state(existingRule?.copy_description ?? true);

  // Other settings
  let leadTimeDays = $state(existingRule?.lead_time_days ?? 14);
  let statusOnCreate = $state(existingRule?.status_on_create ?? null);
  let isActive = $state(existingRule?.is_active ?? true);

  // Start date (dtstart). new Date().toISOString() returns UTC, so a user in PT
  // editing at 6pm gets today's date but a user in Sydney editing at 4pm gets
  // tomorrow's. Build YYYY-MM-DD from the browser's local components instead so
  // the default matches what the user sees on their wall clock.
  function localTodayISO() {
    const d = new Date();
    const yyyy = d.getFullYear();
    const mm = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    return `${yyyy}-${mm}-${dd}`;
  }
  let dtStart = $state(
    existingRule?.dtstart
      ? existingRule.dtstart.substring(0, 10)
      : localTodayISO()
  );

  // End date for the rule (dtend)
  let dtEnd = $state(
    existingRule?.dtend
      ? existingRule.dtend.substring(0, 10)
      : ''
  );

  // Preview state
  let previewDates = $state([]);
  let previewLoading = $state(false);
  let previewError = $state('');

  // Build current RRULE from form state
  const currentRRule = $derived(buildRRule({
    frequency,
    interval,
    byDay: frequency === 'WEEKLY' ? byDay : [],
    byMonthDay: frequency === 'MONTHLY' ? byMonthDay : null,
    endType,
    endDate,
    count,
  }));

  // Human-readable summary
  const summary = $derived(rruleToText(currentRRule));

  // Saving state
  let saving = $state(false);

  function toggleDay(day) {
    if (byDay.includes(day)) {
      byDay = byDay.filter(d => d !== day);
    } else {
      byDay = [...byDay, day];
    }
  }

  async function loadPreview() {
    if (!currentRRule || !dtStart) return;
    previewLoading = true;
    previewError = '';
    try {
      const result = await api.recurrence.preview({
        rrule: currentRRule,
        dtstart: dtStart,
        count: 5,
      });
      previewDates = result.occurrences || [];
    } catch (err) {
      previewError = err.message || t('recurrence.previewError');
      previewDates = [];
    } finally {
      previewLoading = false;
    }
  }

  export async function handleSave() {
    saving = true;
    try {
      const data = {
        rrule: currentRRule,
        dtstart: dtStart,
        dtend: dtEnd || null,
        lead_time_days: leadTimeDays,
        copy_assignee: copyAssignee,
        copy_priority: copyPriority,
        copy_custom_fields: copyCustomFields,
        copy_description: copyDescription,
        status_on_create: statusOnCreate,
        is_active: isActive,
      };

      let result;
      if (existingRule) {
        result = await api.recurrence.update(itemId, data);
      } else {
        result = await api.recurrence.create(itemId, data);
      }
      onsave?.(result);
    } catch (err) {
      console.error('Failed to save recurrence:', err);
      errorToast(err?.message || t('errors.UNKNOWN'));
    } finally {
      saving = false;
    }
  }

  async function handleDelete() {
    ondelete?.();
  }

  function formatPreviewDate(dateStr) {
    try {
      return new Date(dateStr).toLocaleDateString(undefined, {
        weekday: 'short',
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      });
    } catch {
      return dateStr;
    }
  }

  // Frequency unit labels for interval display
  const intervalUnit = $derived.by(() => {
    const plural = interval > 1;
    switch (frequency) {
      case 'DAILY': return plural ? t('recurrence.everyDays') : t('recurrence.everyDay');
      case 'WEEKLY': return plural ? t('recurrence.everyWeeks') : t('recurrence.everyWeek');
      case 'MONTHLY': return plural ? t('recurrence.everyMonths') : t('recurrence.everyMonth');
      case 'YEARLY': return plural ? t('recurrence.everyYears') : t('recurrence.everyYear');
      default: return '';
    }
  });
</script>

<div class="space-y-6">
  <!-- Summary -->
  {#if summary}
    <div class="p-3 rounded-lg" style="background: var(--ds-background-neutral);">
      <div class="text-sm font-medium" style="color: var(--ds-text);">{summary}</div>
    </div>
  {/if}

  <!-- Active Toggle -->
  <div class="flex items-center justify-between">
    <span class="text-sm font-medium" style="color: var(--ds-text);">{t('recurrence.active')}</span>
    <Toggle bind:checked={isActive} size="small" />
  </div>

  <!-- Frequency -->
  <div>
    <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.frequency')}</label>
    <select
      bind:value={frequency}
      class="w-full px-3 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
      style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
    >
      {#each Object.entries(FREQ_LABELS) as [value, label]}
        <option {value}>{label}</option>
      {/each}
    </select>
  </div>

  <!-- Interval -->
  <div>
    <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.interval')}</label>
    <div class="flex items-center gap-2">
      <input
        type="number"
        min="1"
        max="365"
        bind:value={interval}
        class="w-20 px-3 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
        style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
      />
      <span class="text-sm" style="color: var(--ds-text-subtle);">{intervalUnit}</span>
    </div>
  </div>

  <!-- Weekly: Day of week chips -->
  {#if frequency === 'WEEKLY'}
    <div>
      <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.daysOfWeek')}</label>
      <div class="flex flex-wrap gap-1.5">
        {#each DAY_NAMES as day}
          <button
            type="button"
            class="px-3 py-1.5 text-xs font-medium rounded-full border transition-colors"
            style="{byDay.includes(day) ? 'background: var(--ds-interactive); color: white; border-color: var(--ds-interactive);' : 'background: var(--ds-surface-raised); color: var(--ds-text); border-color: var(--ds-border);'}"
            onclick={() => toggleDay(day)}
          >
            {DAY_LABELS[day]}
          </button>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Monthly: Day of month -->
  {#if frequency === 'MONTHLY'}
    <div>
      <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.dayOfMonth')}</label>
      <input
        type="number"
        min="1"
        max="31"
        bind:value={byMonthDay}
        class="w-20 px-3 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
        style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
      />
    </div>
  {/if}

  <!-- Start Date -->
  <div>
    <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.startDate')}</label>
    <input
      type="date"
      bind:value={dtStart}
      class="w-full px-3 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
      style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
    />
  </div>

  <!-- End Condition -->
  <div>
    <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.endCondition')}</label>
    <div class="space-y-2">
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="radio" bind:group={endType} value="never" class="text-sm" />
        <span class="text-sm" style="color: var(--ds-text);">{t('recurrence.never')}</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="radio" bind:group={endType} value="date" class="text-sm" />
        <span class="text-sm" style="color: var(--ds-text);">{t('recurrence.onDate')}</span>
        {#if endType === 'date'}
          <input
            type="date"
            bind:value={endDate}
            class="ml-2 px-2 py-1 text-sm border rounded focus:outline-none focus:ring-2"
            style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
          />
        {/if}
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="radio" bind:group={endType} value="count" class="text-sm" />
        <span class="text-sm" style="color: var(--ds-text);">{t('recurrence.afterOccurrences')}</span>
        {#if endType === 'count'}
          <input
            type="number"
            min="1"
            max="999"
            bind:value={count}
            class="ml-2 w-20 px-2 py-1 text-sm border rounded focus:outline-none focus:ring-2"
            style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
          />
          <span class="text-sm" style="color: var(--ds-text-subtle);">{t('recurrence.occurrences')}</span>
        {/if}
      </label>
    </div>
  </div>

  {#if !compact}
  <!-- Preview -->
  <div>
    <div class="flex items-center justify-between mb-1.5">
      <label class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('recurrence.preview')}</label>
      <Button variant="ghost" size="small" icon={Eye} onclick={loadPreview} disabled={previewLoading}>
        {previewLoading ? t('recurrence.previewLoading') : t('recurrence.preview')}
      </Button>
    </div>
    {#if previewDates.length > 0}
      <div class="space-y-1 p-3 rounded-lg" style="background: var(--ds-surface-sunken);">
        {#each previewDates as date, i}
          <div class="text-sm flex items-center gap-2" style="color: var(--ds-text);">
            <span class="w-5 text-right" style="color: var(--ds-text-subtle);">{i + 1}.</span>
            <span>{formatPreviewDate(date)}</span>
          </div>
        {/each}
      </div>
    {/if}
    {#if previewError}
      <div class="text-sm mt-1" style="color: var(--ds-text-danger);">{previewError}</div>
    {/if}
  </div>

  <!-- Divider -->
  <div class="border-t" style="border-color: var(--ds-border);"></div>

  <!-- Copy Settings -->
  <div>
    <label class="block text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">{t('recurrence.copySettings')}</label>
    <div class="space-y-3">
      <Toggle bind:checked={copyAssignee} size="small" label={t('recurrence.copyAssignee')} />
      <Toggle bind:checked={copyPriority} size="small" label={t('recurrence.copyPriority')} />
      <Toggle bind:checked={copyCustomFields} size="small" label={t('recurrence.copyCustomFields')} />
      <Toggle bind:checked={copyDescription} size="small" label={t('recurrence.copyDescription')} />
    </div>
  </div>

  <!-- Lead Time -->
  <div>
    <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.leadTime')}</label>
    <input
      type="number"
      min="1"
      max="365"
      bind:value={leadTimeDays}
      class="w-24 px-3 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
      style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
    />
  </div>

  <!-- Status on Create -->
  {#if statusOptions.length > 0}
    <div>
      <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text-subtle);">{t('recurrence.statusOnCreate')}</label>
      <select
        bind:value={statusOnCreate}
        class="w-full px-3 py-2 text-sm border rounded-md focus:outline-none focus:ring-2"
        style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
      >
        <option value={null}>{t('common.none')}</option>
        {#each statusOptions as status}
          <option value={status.id}>{status.label}</option>
        {/each}
      </select>
    </div>
  {/if}

  <!-- Actions -->
  <div class="flex items-center justify-between pt-2">
    <div>
      {#if existingRule}
        <Button variant="danger" size="small" icon={Trash2} onclick={handleDelete}>
          {t('recurrence.deleteRule')}
        </Button>
      {/if}
    </div>
    <div class="flex items-center gap-2">
      <Button variant="default" size="small" icon={X} onclick={() => oncancel?.()}>
        {t('common.cancel')}
      </Button>
      <Button variant="primary" size="small" icon={Save} onclick={handleSave} disabled={saving}>
        {saving ? t('common.saving') : t('common.save')}
      </Button>
    </div>
  </div>
  {/if}
</div>
