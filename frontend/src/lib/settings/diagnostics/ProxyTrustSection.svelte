<script>
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconNetwork from '@tabler/icons-svelte-runes/icons/network';
  import IconShieldLock from '@tabler/icons-svelte-runes/icons/shield-lock';
  import IconRoute from '@tabler/icons-svelte-runes/icons/route';
  import AlertBox from '../../components/AlertBox.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import { getProxyTrust } from '../../api/diagnostics.js';
  import { formatRelativeTime, formatUtcTime } from './format-utils.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';

  let view = $state({ loading: true, error: null, data: null });

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      view = { loading: false, error: null, data: await getProxyTrust() };
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  const unlistedColumns = [
    { key: 'ip', label: 'Address', render: (peer) => peer.ip },
    { key: 'trusted', label: 'Effective', render: (peer) => (peer.trusted ? 'Trusted' : 'Ignored') },
    { key: 'count', label: 'Observations', align: 'text-right', render: (peer) => peer.count.toLocaleString() },
    { key: 'first_seen', label: 'First seen', render: (peer) => formatUtcTime(peer.first_seen), textColor: 'var(--ds-text-subtle)' },
    { key: 'last_seen', label: 'Last seen', render: (peer) => formatRelativeTime(peer.last_seen) },
  ];
  const clientColumns = [
    { key: 'ip', label: 'Address', render: (peer) => peer.ip },
    { key: 'count', label: 'Observations', align: 'text-right', render: (peer) => peer.count.toLocaleString() },
    { key: 'first_seen', label: 'First seen', render: (peer) => formatUtcTime(peer.first_seen), textColor: 'var(--ds-text-subtle)' },
    { key: 'last_seen', label: 'Last seen', render: (peer) => formatRelativeTime(peer.last_seen) },
  ];

  const data = $derived(view.data);
  const trustLabel = $derived(data?.trust_mode === 'strict' ? 'Strict' : 'Legacy');
  const strict = $derived(data?.trust_mode === 'strict');
  const forwardingHeading = $derived(strict ? 'Unlisted proxies' : 'Peers sending forwarding headers');
  const forwardingEmpty = $derived(
    strict
      ? 'No unlisted proxy has sent forwarding headers.'
      : 'No peer has sent forwarding headers.'
  );
  const proxyLabel = $derived(
    data?.use_proxy ? (data.proxy_auto_detected ? 'Auto-detected' : 'Enabled') : 'Disabled'
  );
  // In strict mode every unlisted peer is ignored; with proxy mode off nothing is
  // trusted. Under legacy the peers are mixed, so each row's Effective column
  // carries the per-peer verdict and the badge stays neutral.
  const forwardingBadge = $derived(!data?.use_proxy || strict ? 'Headers ignored' : 'Not configured');
  const listedProxies = $derived(data?.additional_proxies ?? []);
  const invalidProxies = $derived(data?.invalid_proxies ?? []);
  const invalidMessage = $derived.by(() => {
    if (!invalidProxies.length) return '';
    const base = `These ADDITIONAL_PROXIES entries are not literal IPv4/IPv6 addresses and are ignored: ${invalidProxies.join(', ')}.`;
    if (!data?.use_proxy) {
      return `${base} Proxy mode is disabled, so no forwarded headers are trusted.`;
    }
    if (strict) {
      return `${base} Strict trust trusts only loopback and the parsed addresses above.`;
    }
    return `${base} Legacy trust still trusts every private, loopback, and link-local peer in addition to the parsed addresses above.`;
  });
</script>

<DiagnosticsSection
  title="Proxy trust"
  subtitle="How forwarded headers are trusted and whether this installation is affected by the private-range client-IP finding. Counts are process-local and cover the peers this replica has seen since start."
  dataTestId="diagnostics-proxy-trust"
  onLoad={load}
  bind:loading={view.loading}
  bind:error={view.error}
  refreshInterval={10_000}
>
  {#snippet children()}
    {#if data}
      {#if data.status === 'action_required'}
        <AlertBox
          variant="warning"
          message="Strict trust is enabled, but an unlisted peer is sending forwarding headers. Those headers are ignored, so its clients are attributed to the proxy address and may lose Secure-cookie and HSTS detection."
        />
      {:else if data.status === 'protected'}
        <AlertBox
          variant="success"
          message="Strict proxy trust is active. Only loopback and the listed proxies may supply forwarding headers; this reflects the peers observed since start."
        />
      {:else}
        <AlertBox variant="info" message="Proxy mode is disabled. Forwarding headers are never trusted, so a private-address client cannot spoof its IP." />
      {/if}

      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          icon={IconShieldLock}
          label="Trust mode"
          value={trustLabel}
          color={data.status === 'vulnerable' ? 'orange' : 'green'}
        />
        <StatCard icon={IconNetwork} label="Proxy mode" value={proxyLabel} color="blue" />
        <StatCard
          icon={IconAlertTriangle}
          label="Affected"
          value={data.affected ? 'Yes' : 'No'}
          color={data.affected ? 'orange' : 'green'}
        />
        <StatCard
          icon={IconRoute}
          label="Listed proxies"
          value={String(listedProxies.length)}
          color={invalidProxies.length ? 'orange' : 'purple'}
        />
      </div>

      <div class="text-sm text-ds-text-subtle">
        Listed proxies: {listedProxies.length ? listedProxies.join(', ') : 'None'}
      </div>

      {#if invalidProxies.length}
        <AlertBox variant="warning" message={invalidMessage} />
      {/if}

      <div class="text-sm text-ds-text-subtle">{data.summary}</div>
      {#if data.hint}
        <div class="text-sm text-ds-text-subtle">{data.hint}</div>
      {/if}

      <div>
        <div class="flex items-center gap-2 mb-2">
          <h4 class="text-sm font-semibold text-ds-text">{forwardingHeading}</h4>
          {#if data.unlisted_proxies?.length}
            <Lozenge appearance="warning">{forwardingBadge}</Lozenge>
          {:else}
            <Lozenge appearance="success">None seen</Lozenge>
          {/if}
        </div>
        <DataTable
          columns={unlistedColumns}
          data={data.unlisted_proxies ?? []}
          keyField="ip"
          emptyMessage={forwardingEmpty}
        />
      </div>

      <div>
        <div class="flex items-center gap-2 mb-2">
          <h4 class="text-sm font-semibold text-ds-text">Private clients</h4>
          <Lozenge appearance="info">Identified directly</Lozenge>
        </div>
        <DataTable
          columns={clientColumns}
          data={data.private_clients ?? []}
          keyField="ip"
          emptyMessage="No private-address client has connected directly."
        />
      </div>
    {/if}
  {/snippet}
</DiagnosticsSection>
