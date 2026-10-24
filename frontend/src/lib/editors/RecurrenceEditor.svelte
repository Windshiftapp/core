<script>
  import { t } from '../stores/i18n.svelte.js';
  import { recurrence } from '../api/recurrence.js';
  import { parseRRule, buildRRule, rruleToText, DAY_NAMES, DAY_LABELS, FREQ_LABELS } from './rruleUtils.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import FormField from '../components/FormField.svelte';
  import Input from '../components/Input.svelte';
  import Radio from '../components/Radio.svelte';
  import Select from '../components/Select.svelte';
  import Toggle from '../components/Toggle.svelte';
  import Save from '@lucide/svelte/icons/save';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import X from '@lucide/svelte/icons/x';
  import Eye from '@lucide/svelte/icons/eye';

  let {
    itemId,
    existingRule = null,
    statusOptions = [],
    compact = false,
    onsave = null,
    oncancel = null,
    ondelete = null,
  } = $props();

  // Snapshot existingRule into editable form state; subsequent edits are user-driven.
  // svelte-ignore state_referenced_locally
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
  // svelte-ignore state_referenced_locally
  let copyAssignee = $state(existingRule?.copy_assignee ?? true);
  // svelte-ignore state_referenced_locally
  let copyPriority = $state(existingRule?.copy_priority ?? true);
  // svelte-ignore state_referenced_locally
  let copyCustomFields = $state(existingRule?.copy_custom_fields ?? true);
  // svelte-ignore state_referenced_locally
  let copyDescription = $state(existingRule?.copy_description ?? true);

  // Other settings
  // svelte-ignore state_referenced_locally
  let leadTimeDays = $state(existingRule?.lead_time_days ?? 14);
  // svelte-ignore state_referenced_locally
  let statusOnCreate = $state(existingRule?.status_on_create ?? null);
  // svelte-ignore state_referenced_locally
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
  // svelte-ignore state_referenced_locally
  let dtStart = $state(
    existingRule?.dtstart
      ? existingRule.dtstart.substring(0, 10)
      : localTodayISO()
  );

  // End date for the rule (dtend)
  // svelte-ignore state_referenced_locally
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
  const frequencyOptions = Object.entries(FREQ_LABELS).map(([value, label]) => ({ value, label }));
  const statusSelectOptions = $derived([
    { value: null, label: t('common.none') },
    ...statusOptions.map(status => ({ value: status.id, label: status.label })),
  ]);
  const controlSize = $derived(compact ? 'small' : 'medium');

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
      const result = await recurrence.preview({
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
        result = await recurrence.update(itemId, data);
      } else {
        result = await recurrence.create(itemId, data);
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

<div class="space-y-5" data-testid="recurrence-editor">
  <Card variant="flat" padding="compact" dataTestid="recurrence-editor-summary">
    <div class="flex items-center justify-between gap-4">
      <div class="min-w-0">
        <div class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">
          {t('recurrence.rule')}
        </div>
        <div class="text-sm font-medium truncate" style="color: var(--ds-text);">{summary}</div>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <span class="text-sm" style="color: var(--ds-text-subtle);">{t('recurrence.active')}</span>
        <Toggle bind:checked={isActive} size="small" />
      </div>
    </div>
  </Card>

  <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
    <FormField label={t('recurrence.frequency')} id="recurrence-frequency" class="mb-0">
      <Select
        id="recurrence-frequency"
        bind:value={frequency}
        options={frequencyOptions}
        size={controlSize}
      />
    </FormField>

    <FormField label={t('recurrence.interval')} id="recurrence-interval" class="mb-0">
      <div class="flex items-center gap-2">
        <Input
          id="recurrence-interval"
          type="number"
          min="1"
          max="365"
          bind:value={interval}
          size={controlSize}
          class="max-w-24"
        />
        <span class="text-sm" style="color: var(--ds-text-subtle);">{intervalUnit}</span>
      </div>
    </FormField>
  </div>

  {#if frequency === 'WEEKLY'}
    <FormField label={t('recurrence.daysOfWeek')} class="mb-0">
      <div class="flex flex-wrap gap-2">
        {#each DAY_NAMES as day}
          <Button
            variant={byDay.includes(day) ? 'selected' : 'default'}
            size="small"
            onclick={() => toggleDay(day)}
          >
            {DAY_LABELS[day]}
          </Button>
        {/each}
      </div>
    </FormField>
  {/if}

  {#if frequency === 'MONTHLY'}
    <FormField label={t('recurrence.dayOfMonth')} id="recurrence-day-of-month" class="mb-0">
      <Input
        id="recurrence-day-of-month"
        type="number"
        min="1"
        max="31"
        bind:value={byMonthDay}
        size={controlSize}
        class="max-w-24"
      />
    </FormField>
  {/if}

  <FormField label={t('recurrence.startDate')} id="recurrence-start-date" class="mb-0">
    <Input
      id="recurrence-start-date"
      type="date"
      bind:value={dtStart}
      size={controlSize}
    />
  </FormField>

  <FormField label={t('recurrence.endCondition')} class="mb-0">
    <Card variant="outlined" padding="compact">
      <div class="space-y-3">
        <label class="flex items-center gap-2 cursor-pointer">
          <Radio id="recurrence-end-never" bind:groupValue={endType} value="never" />
          <span class="text-sm" style="color: var(--ds-text);">{t('recurrence.never')}</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <Radio id="recurrence-end-date-option" bind:groupValue={endType} value="date" />
          <span class="text-sm" style="color: var(--ds-text);">{t('recurrence.onDate')}</span>
          {#if endType === 'date'}
            <Input
              id="recurrence-end-date"
              type="date"
              bind:value={endDate}
              size="small"
              class="ml-auto max-w-48"
            />
          {/if}
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <Radio id="recurrence-end-count-option" bind:groupValue={endType} value="count" />
          <span class="text-sm" style="color: var(--ds-text);">{t('recurrence.afterOccurrences')}</span>
          {#if endType === 'count'}
            <Input
              id="recurrence-count"
              type="number"
              min="1"
              max="999"
              bind:value={count}
              size="small"
              class="ml-auto max-w-24"
            />
            <span class="text-sm" style="color: var(--ds-text-subtle);">{t('recurrence.occurrences')}</span>
          {/if}
        </label>
      </div>
    </Card>
  </FormField>

  {#if !compact}
    <Card variant="outlined" padding="default">
      <div class="flex items-center justify-between gap-3 mb-3">
        <div class="text-sm font-medium" style="color: var(--ds-text);">{t('recurrence.preview')}</div>
        <Button variant="default" size="small" icon={Eye} onclick={loadPreview} disabled={previewLoading}>
          {previewLoading ? t('recurrence.previewLoading') : t('recurrence.preview')}
        </Button>
      </div>
      {#if previewDates.length > 0}
        <div class="space-y-1">
          {#each previewDates as date, i}
            <div class="text-sm flex items-center gap-2" style="color: var(--ds-text);">
              <span class="w-5 text-right" style="color: var(--ds-text-subtle);">{i + 1}.</span>
              <span>{formatPreviewDate(date)}</span>
            </div>
          {/each}
        </div>
      {/if}
      {#if previewError}
        <div class="text-sm mt-2" style="color: var(--ds-text-danger);">{previewError}</div>
      {/if}
    </Card>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <Card variant="outlined" padding="default">
        <div class="text-sm font-medium mb-3" style="color: var(--ds-text);">
          {t('recurrence.copySettings')}
        </div>
        <div class="space-y-3">
          <Toggle bind:checked={copyAssignee} size="small" label={t('recurrence.copyAssignee')} />
          <Toggle bind:checked={copyPriority} size="small" label={t('recurrence.copyPriority')} />
          <Toggle bind:checked={copyCustomFields} size="small" label={t('recurrence.copyCustomFields')} />
          <Toggle bind:checked={copyDescription} size="small" label={t('recurrence.copyDescription')} />
        </div>
      </Card>

      <Card variant="outlined" padding="default">
        <FormField label={t('recurrence.leadTime')} id="recurrence-lead-time">
          <Input
            id="recurrence-lead-time"
            type="number"
            min="1"
            max="365"
            bind:value={leadTimeDays}
            size="small"
            class="max-w-28"
          />
        </FormField>
        {#if statusOptions.length > 0}
          <FormField label={t('recurrence.statusOnCreate')} id="recurrence-status-on-create" class="mb-0">
            <Select
              id="recurrence-status-on-create"
              bind:value={statusOnCreate}
              options={statusSelectOptions}
              size="small"
            />
          </FormField>
        {/if}
      </Card>
    </div>

    <div class="flex items-center justify-between pt-4 border-t" style="border-color: var(--ds-border);">
      <div>
        {#if existingRule}
          <Button variant="danger" size="small" icon={Trash2} onclick={handleDelete} dataTestid="recurrence-editor-delete">
            {t('recurrence.deleteRule')}
          </Button>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <Button variant="default" size="small" icon={X} onclick={() => oncancel?.()} dataTestid="recurrence-editor-cancel">
          {t('common.cancel')}
        </Button>
        <Button variant="primary" size="small" icon={Save} onclick={handleSave} disabled={saving} dataTestid="recurrence-editor-save">
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    </div>
  {/if}
</div>
