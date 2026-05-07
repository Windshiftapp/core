<script>
  import { onDestroy, onMount } from 'svelte';
  import { IconClock, IconActivity, IconAlertTriangle, IconRulerMeasure } from '@tabler/icons-svelte-runes';
  import StatCard from '../../components/StatCard.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import {
    DRIFT_THRESHOLD_MS,
    getClockOffset,
    getSampleCount,
    getSamples,
  } from '../../utils/serverClock.js';

  let offsetMs = $state(getClockOffset());
  let sampleCount = $state(getSampleCount());
  let samples = $state(getSamples());
  let now = $state(Date.now());

  let interval;
  onMount(() => {
    interval = setInterval(() => {
      offsetMs = getClockOffset();
      sampleCount = getSampleCount();
      samples = getSamples();
      now = Date.now();
    }, 2000);
  });
  onDestroy(() => {
    if (interval) clearInterval(interval);
  });

  function formatOffset(ms) {
    if (sampleCount === 0) return '—';
    const sec = Math.round(ms / 1000);
    if (sec === 0) return 'in sync';
    const absMin = Math.floor(Math.abs(sec) / 60);
    const absSec = Math.abs(sec) % 60;
    const direction = sec > 0 ? ' ahead' : ' behind';
    if (absMin > 0) return `${absMin}m ${absSec}s${direction}`;
    return `${absSec}s${direction}`;
  }

  function formatThreshold(ms) {
    const sec = Math.round(ms / 1000);
    return sec >= 60 ? `${Math.round(sec / 60)}m` : `${sec}s`;
  }

  function formatSampleOffset(ms) {
    const sec = Math.round(ms / 1000);
    if (sec === 0) return '0s';
    return `${sec > 0 ? '+' : ''}${sec}s`;
  }

  function formatRelative(at) {
    const diff = Math.max(0, now - at);
    if (diff < 1000) return 'just now';
    const sec = Math.round(diff / 1000);
    if (sec < 60) return `${sec}s ago`;
    const min = Math.floor(sec / 60);
    return `${min}m ${sec % 60}s ago`;
  }

  function formatTime(ms) {
    return new Date(ms).toISOString().replace('T', ' ').replace('Z', ' UTC');
  }

  const isOverThreshold = $derived(sampleCount > 0 && Math.abs(offsetMs) > DRIFT_THRESHOLD_MS);
  const statusLabel = $derived(
    sampleCount === 0 ? 'No samples yet' : isOverThreshold ? 'Over threshold' : 'Within threshold'
  );
  const statusColor = $derived(isOverThreshold ? 'orange' : sampleCount === 0 ? 'blue' : 'green');
  const orderedSamples = $derived(samples.slice().reverse());

  const sampleColumns = [
    { key: 'when', label: 'When', render: (s) => formatRelative(s.at) },
    { key: 'clientTime', label: 'Client time (UTC)', render: (s) => formatTime(s.clientTime), textColor: 'var(--ds-text-subtle)' },
    { key: 'serverTime', label: 'Server time (UTC)', render: (s) => formatTime(s.serverTime), textColor: 'var(--ds-text-subtle)' },
    { key: 'offsetMs', label: 'Offset', align: 'text-right', render: (s) => formatSampleOffset(s.offsetMs) },
  ];
</script>

<section class="space-y-4" data-testid="diagnostics-server-clock">
  <div>
    <h3 class="text-base font-semibold" style="color: var(--ds-text);">Server clock</h3>
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      Compares the HTTP <code>Date</code> header on every API response against the browser clock. The rolling median across the last 5 samples is used to correct timestamp display. The warning toast fires when |offset| exceeds the threshold.
    </p>
  </div>

  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
    <div data-testid="clock-stat-offset">
      <StatCard
        icon={IconClock}
        label="Current offset"
        value={formatOffset(offsetMs)}
        color={statusColor}
      />
    </div>
    <div data-testid="clock-stat-status">
      <StatCard
        icon={isOverThreshold ? IconAlertTriangle : IconActivity}
        label="Drift status"
        value={statusLabel}
        color={statusColor}
      />
    </div>
    <div data-testid="clock-stat-sample-count">
      <StatCard
        icon={IconActivity}
        label="Samples collected"
        value={`${sampleCount} / 5`}
        color="blue"
      />
    </div>
    <div data-testid="clock-stat-threshold">
      <StatCard
        icon={IconRulerMeasure}
        label="Drift threshold"
        value={formatThreshold(DRIFT_THRESHOLD_MS)}
        color="purple"
      />
    </div>
  </div>

  <div>
    <div class="flex items-baseline justify-between mb-2">
      <h4 class="text-sm font-semibold" style="color: var(--ds-text);">Recent samples</h4>
      <span class="text-xs" style="color: var(--ds-text-subtle);">Newest first · auto-refreshes every 2s</span>
    </div>
    <DataTable
      columns={sampleColumns}
      data={orderedSamples}
      keyField="id"
      emptyMessage="No samples collected yet. Samples are recorded automatically as API requests complete."
    />
  </div>
</section>
