<script>
  import { onMount } from 'svelte';
  import Spinner from '../components/Spinner.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';

  import { aiFeatures } from '../api/ai.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { logbookStore } from '../stores/logbook.svelte.js';
  import Select from '../components/Select.svelte';
  import IconMessageChatbot from '@tabler/icons-svelte-runes/icons/message-chatbot';
  import IconSun from '@tabler/icons-svelte-runes/icons/sun';
  import IconCalendarCheck from '@tabler/icons-svelte-runes/icons/calendar-check';
  import IconPlayerTrackNext from '@tabler/icons-svelte-runes/icons/player-track-next';
  import IconSearch from '@tabler/icons-svelte-runes/icons/search';
  import IconPuzzle from '@tabler/icons-svelte-runes/icons/puzzle';
  import IconFileDescription from '@tabler/icons-svelte-runes/icons/file-description';
  import IconArrowsShuffle from '@tabler/icons-svelte-runes/icons/arrows-shuffle';
  import IconNotebook from '@tabler/icons-svelte-runes/icons/notebook';

  const FEATURE_ICONS = {
    ai_chat: IconMessageChatbot,
    daily_briefing: IconSun,
    plan_my_day: IconCalendarCheck,
    catch_me_up: IconPlayerTrackNext,
    find_similar: IconSearch,
    decompose: IconPuzzle,
    release_notes: IconFileDescription,
    dependency_analysis: IconArrowsShuffle,
    logbook_articles: IconNotebook,
  };

  const BASE_FEATURE_KEYS = [
    'ai_chat',
    'daily_briefing',
    'plan_my_day',
    'catch_me_up',
    'find_similar',
    'decompose',
    'release_notes',
    'dependency_analysis',
  ];

  let featureKeys = $derived(
    logbookStore.available
      ? [...BASE_FEATURE_KEYS, 'logbook_articles']
      : BASE_FEATURE_KEYS
  );

  let loading = $state(true);
  let saving = $state(false);
  let config = $state({});
  let connections = $state([]);

  onMount(async () => {
    await loadConfig();
  });

  async function loadConfig() {
    loading = true;
    try {
      const data = await aiFeatures.getConfig();
      config = data.config ?? {};
      connections = data.connections ?? [];
    } catch (err) {
      errorToast(t('settings.aiFeatures.loadFailed'));
      console.error('Failed to load AI features config:', err);
    } finally {
      loading = false;
    }
  }

  function getMode(key) {
    return config[key]?.mode || 'default';
  }

  function getConnectionId(key) {
    return config[key]?.connection_id || 0;
  }

  function getSchedule(key) {
    return config[key]?.schedule || 'daily';
  }

  async function setMode(key, mode) {
    config = {
      ...config,
      [key]: {
        mode,
        connection_id: mode === 'specific' ? (config[key]?.connection_id || (connections[0]?.id ?? 0)) : 0,
        ...(key === 'daily_briefing' ? { schedule: config[key]?.schedule || 'daily' } : {}),
      },
    };
    await save();
  }

  async function setConnectionId(key, connectionId) {
    const current = config[key];
    config = {
      ...config,
      [key]: {
        ...current,
        mode: current?.mode || 'default',
        connection_id: parseInt(connectionId, 10),
      },
    };
    await save();
  }

  async function setSchedule(key, schedule) {
    const current = config[key];
    config = {
      ...config,
      [key]: {
        ...current,
        mode: current?.mode || 'default',
        schedule,
      },
    };
    await save();
  }

  async function save() {
    saving = true;
    try {
      const result = await aiFeatures.updateConfig(config);
      config = result.config ?? config;
      successToast(t('settings.aiFeatures.saveSuccess'));
    } catch (err) {
      errorToast(t('settings.aiFeatures.saveFailed'));
      console.error('Failed to save AI features config:', err);
    } finally {
      saving = false;
    }
  }
</script>

{#if loading}
  <StateDisplay type="loading" />
{:else}
  {#if connections.length === 0}
    <div class="mb-6 text-sm rounded p-4 border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text-subtle);">
      {t('settings.aiFeatures.noConnections')}
    </div>
  {/if}

  <div class="space-y-4">
    {#each featureKeys as key}
      {@const mode = getMode(key)}
      {@const FeatureIcon = FEATURE_ICONS[key]}
      <div class="border rounded p-5" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-start justify-between gap-4">
          <div class="flex items-start gap-3">
            {#if FeatureIcon}
              <div class="mt-0.5 shrink-0 rounded-md p-1.5" style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">
                <FeatureIcon size={18} stroke={1.5} />
              </div>
            {/if}
            <div class="min-w-0">
              <h3 class="text-sm font-medium" style="color: var(--ds-text);">
                {t(`settings.aiFeatures.features.${key}.name`)}
              </h3>
              <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">
                {t(`settings.aiFeatures.features.${key}.description`)}
              </p>
            </div>
          </div>

          <div class="flex items-center gap-2 shrink-0">
            {#if saving}
              <Spinner size="sm" />
            {/if}
            <Select
              value={mode}
              onchange={(v) => setMode(key, v)}
              size="small"
              options={[
                { value: 'default', label: t('settings.aiFeatures.modeDefault') },
                { value: 'specific', label: t('settings.aiFeatures.modeSpecific') },
                { value: 'disabled', label: t('settings.aiFeatures.modeDisabled') }
              ]}
            />
          </div>
        </div>

        {#if mode === 'specific'}
          <div class="mt-3 pt-3 border-t" style="border-color: var(--ds-border);">
            <Select
              value={getConnectionId(key)}
              onchange={(v) => setConnectionId(key, v)}
              size="small"
              placeholder={t('settings.aiFeatures.selectConnection')}
              options={connections.map(c => ({ value: c.id, label: c.name }))}
              class="max-w-xs"
            />
          </div>
        {/if}

        {#if key === 'daily_briefing' && mode !== 'disabled'}
          <div class="mt-3 pt-3 border-t" style="border-color: var(--ds-border);">
            <div class="block text-xs mb-1" style="color: var(--ds-text-subtle);">
              {t('settings.aiFeatures.scheduleLabel')}
            </div>
            <Select
              value={getSchedule(key)}
              onchange={(v) => setSchedule(key, v)}
              size="small"
              options={[
                { value: 'daily', label: t('settings.aiFeatures.scheduleDaily') },
                { value: 'every_6h', label: t('settings.aiFeatures.scheduleEvery6h') }
              ]}
              class="max-w-xs"
            />
          </div>
        {/if}
      </div>
    {/each}
  </div>
{/if}
