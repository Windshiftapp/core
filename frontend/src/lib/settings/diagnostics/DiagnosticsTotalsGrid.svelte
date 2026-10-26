<script>
  import IconActivity from '@tabler/icons-svelte-runes/icons/activity';
  import IconCheck from '@tabler/icons-svelte-runes/icons/check';
  import IconX from '@tabler/icons-svelte-runes/icons/x';
  import IconClock from '@tabler/icons-svelte-runes/icons/clock';
  import StatCard from '../../components/StatCard.svelte';

  /**
   * Headline totals row shared by the diagnostics sections (total, success
   * rate, failures, average).
   *
   * @type {{
   *   totals: { total: number, successRate: number | null, failed: number, avg: number | null },
   *   testids: { total: string, successRate: string, failures: string, avg: string },
   *   totalLabel: string,
   *   avgLabel: string,
   *   formatAvg: (value: number) => string,
   * }}
   */
  let { totals, testids, totalLabel, avgLabel, formatAvg } = $props();
</script>

<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
  <div data-testid={testids.total}>
    <StatCard icon={IconActivity} label={totalLabel} value={totals.total.toString()} color="blue" />
  </div>
  <div data-testid={testids.successRate}>
    <StatCard
      icon={IconCheck}
      label="Success rate"
      value={totals.successRate == null ? '—' : `${totals.successRate}%`}
      color={totals.successRate != null && totals.successRate < 95 ? 'orange' : 'green'}
    />
  </div>
  <div data-testid={testids.failures}>
    <StatCard
      icon={IconX}
      label="Failures"
      value={totals.failed.toString()}
      color={totals.failed > 0 ? 'orange' : 'green'}
    />
  </div>
  <div data-testid={testids.avg}>
    <StatCard
      icon={IconClock}
      label={avgLabel}
      value={totals.avg == null ? '—' : formatAvg(totals.avg)}
      color="purple"
    />
  </div>
</div>
