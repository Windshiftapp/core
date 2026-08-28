<script>
  import { IconClock, IconActivity, IconAlertTriangle, IconRulerMeasure } from '@tabler/icons-svelte-runes';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatUtcTime } from './format-utils.js';
  import {
    DRIFT_THRESHOLD_MS,
    getClockOffset,
    getSampleCount,
    getSamples,
  } from '../../utils/serverClock.js';
  import { t } from '../../stores/i18n.svelte.js';

  let offsetMs = $state(getClockOffset());
  let sampleCount = $state(getSampleCount());
  let samples = $state(getSamples());
  let now = $state(Date.now());

  function refresh() {
    offsetMs = getClockOffset();
    sampleCount = getSampleCount();
    samples = getSamples();
    now = Date.now();
  }

  function formatOffset(ms) {
    if (sampleCount === 0) return '—';
    const sec = Math.round(ms / 1000);
    if (sec === 0) return t('diagnostics.clock.inSync');
    const absMin = Math.floor(Math.abs(sec) / 60);
    const absSec = Math.abs(sec) % 60;
    const value = absMin > 0
      ? t('diagnostics.clock.minutesSeconds', { minutes: absMin, seconds: absSec })
      : t('diagnostics.clock.seconds', { seconds: absSec });
    return sec > 0
      ? t('diagnostics.clock.ahead', { value })
      : t('diagnostics.clock.behind', { value });
  }

  function formatThreshold(ms) {
    const sec = Math.round(ms / 1000);
    return sec >= 60
      ? t('diagnostics.clock.minutesShort', { count: Math.round(sec / 60) })
      : t('diagnostics.clock.secondsShort', { count: sec });
  }

  function formatSampleOffset(ms) {
    const sec = Math.round(ms / 1000);
    if (sec === 0) return t('diagnostics.clock.zeroSeconds');
    return t('diagnostics.clock.signedSeconds', { value: `${sec > 0 ? '+' : ''}${sec}` });
  }

  function formatRelative(at) {
    const diff = Math.max(0, now - at);
    if (diff < 1000) return t('diagnostics.clock.justNow');
    const sec = Math.round(diff / 1000);
    if (sec < 60) return t('diagnostics.clock.secondsAgo', { count: sec });
    const min = Math.floor(sec / 60);
    return t('diagnostics.clock.minutesSecondsAgo', { minutes: min, seconds: sec % 60 });
  }

  const isOverThreshold = $derived(sampleCount > 0 && Math.abs(offsetMs) > DRIFT_THRESHOLD_MS);
  const statusLabel = $derived(
    sampleCount === 0
      ? t('diagnostics.clock.noSamplesShort')
      : isOverThreshold
        ? t('diagnostics.clock.overThreshold')
        : t('diagnostics.clock.withinThreshold')
  );
  const statusColor = $derived(isOverThreshold ? 'orange' : sampleCount === 0 ? 'blue' : 'green');
  const orderedSamples = $derived(samples.slice().reverse());

  let sampleColumns = $derived([
    { key: 'when', label: t('diagnostics.clock.when'), render: (s) => formatRelative(s.at) },
    { key: 'clientTime', label: t('diagnostics.clock.clientTime'), render: (s) => formatUtcTime(s.clientTime), textColor: 'var(--ds-text-subtle)' },
    { key: 'serverTime', label: t('diagnostics.clock.serverTime'), render: (s) => formatUtcTime(s.serverTime), textColor: 'var(--ds-text-subtle)' },
    { key: 'offsetMs', label: t('diagnostics.clock.offset'), align: 'text-right', render: (s) => formatSampleOffset(s.offsetMs) },
  ]);
</script>

<DiagnosticsSection
  title={t('diagnostics.clock.title')}
  subtitle={t('diagnostics.clock.subtitle')}
  dataTestId="diagnostics-server-clock"
  onLoad={refresh}
  refreshInterval={2_000}
  showRefresh={false}
>
  {#snippet children()}
  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
    <div data-testid="clock-stat-offset">
      <StatCard
        icon={IconClock}
        label={t('diagnostics.clock.currentOffset')}
        value={formatOffset(offsetMs)}
        color={statusColor}
      />
    </div>
    <div data-testid="clock-stat-status">
      <StatCard
        icon={isOverThreshold ? IconAlertTriangle : IconActivity}
        label={t('diagnostics.clock.driftStatus')}
        value={statusLabel}
        color={statusColor}
      />
    </div>
    <div data-testid="clock-stat-sample-count">
      <StatCard
        icon={IconActivity}
        label={t('diagnostics.clock.samplesCollected')}
        value={`${sampleCount} / 5`}
        color="blue"
      />
    </div>
    <div data-testid="clock-stat-threshold">
      <StatCard
        icon={IconRulerMeasure}
        label={t('diagnostics.clock.driftThreshold')}
        value={formatThreshold(DRIFT_THRESHOLD_MS)}
        color="purple"
      />
    </div>
  </div>

  <div>
    <div class="flex items-baseline justify-between mb-2">
      <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('diagnostics.clock.recentSamples')}</h4>
      <span class="text-xs" style="color: var(--ds-text-subtle);">{t('diagnostics.clock.autoRefresh')}</span>
    </div>
    <DataTable
      columns={sampleColumns}
      data={orderedSamples}
      keyField="id"
      emptyMessage={t('diagnostics.clock.noSamples')}
    />
  </div>
  {/snippet}
</DiagnosticsSection>
