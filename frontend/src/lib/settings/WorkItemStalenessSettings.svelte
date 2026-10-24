<script>
  import { onMount } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import IconClockPause from '@tabler/icons-svelte-runes/icons/clock-pause';
  import { workItemStaleness } from '../api/ai.js';
  import Button from '../components/Button.svelte';
  import FormField from '../components/FormField.svelte';
  import Input from '../components/Input.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { workItemStalenessSettings } from '../stores/workItemStalenessSettings.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';

  let loading = $state(true);
  let saving = $state(false);
  let staleAfterDays = $state(30);
  let savedStaleAfterDays = $state(30);
  let validationError = $state('');

  onMount(load);

  async function load() {
    loading = true;
    try {
      const settings = await workItemStaleness.get();
      staleAfterDays = settings.stale_after_days;
      savedStaleAfterDays = settings.stale_after_days;
      workItemStalenessSettings.hydrate(settings);
    } catch (err) {
      console.error('Failed to load work item staleness settings:', err);
      errorToast(t('settings.workItemStaleness.loadFailed'));
    } finally {
      loading = false;
    }
  }

  async function save() {
    const days = Number(staleAfterDays);
    if (!Number.isInteger(days) || days < 1 || days > 365) {
      validationError = t('settings.workItemStaleness.validation');
      return;
    }

    validationError = '';
    saving = true;
    try {
      const settings = await workItemStaleness.update({ stale_after_days: days });
      staleAfterDays = settings.stale_after_days;
      savedStaleAfterDays = settings.stale_after_days;
      workItemStalenessSettings.hydrate(settings);
      successToast(t('settings.workItemStaleness.saveSuccess'));
    } catch (err) {
      console.error('Failed to save work item staleness settings:', err);
      errorToast(err?.message || t('settings.workItemStaleness.saveFailed'));
    } finally {
      saving = false;
    }
  }
</script>

<PageHeader
  title={t('settings.workItemStaleness.title')}
  subtitle={t('settings.workItemStaleness.subtitle')}
/>

{#if loading}
  <StateDisplay type="loading" dataTestid="work-item-staleness-loading" />
{:else}
  <section class="max-w-3xl" aria-labelledby="work-item-staleness-heading">
    <div class="flex flex-col gap-5 rounded-lg border p-5 sm:flex-row sm:items-end sm:justify-between"
      style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
      <div class="min-w-0 flex-1">
        <div class="mb-4 flex items-start gap-3">
          <div class="mt-0.5 shrink-0 rounded-md p-2"
            style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">
            <IconClockPause size={20} stroke={1.5} />
          </div>
          <div class="min-w-0">
            <h2 id="work-item-staleness-heading" class="text-base font-semibold" style="color: var(--ds-text);">
              {t('settings.workItemStaleness.thresholdTitle')}
            </h2>
            <p class="mt-1 max-w-[70ch] text-sm" style="color: var(--ds-text-subtle);">
              {t('settings.workItemStaleness.thresholdDescription')}
            </p>
          </div>
        </div>

        <FormField
          id="work-item-staleness-days"
          label={t('settings.workItemStaleness.daysLabel')}
          helper={t('settings.workItemStaleness.daysHelper')}
          error={validationError}
          class="mb-0 max-w-48"
        >
          <Input
            id="work-item-staleness-days"
            type="number"
            min="1"
            max="365"
            step="1"
            bind:value={staleAfterDays}
            oninput={() => { validationError = ''; }}
            dataTestid="work-item-staleness-days"
            ariaDescribedby="work-item-staleness-impact"
          />
        </FormField>
        <p id="work-item-staleness-impact" class="mt-3 text-xs" style="color: var(--ds-text-subtle);">
          {t('settings.workItemStaleness.impact')}
        </p>
      </div>

      <Button
        variant="primary"
        onclick={save}
        loading={saving}
        disabled={saving || Number(staleAfterDays) === savedStaleAfterDays}
        dataTestid="work-item-staleness-save"
      >
        {t('settings.workItemStaleness.save')}
      </Button>
    </div>
  </section>
{/if}
