<script>
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconCircleCheck from '@tabler/icons-svelte-runes/icons/circle-check';
  import IconClock from '@tabler/icons-svelte-runes/icons/clock';
  import IconPlugOff from '@tabler/icons-svelte-runes/icons/plug-off';
  import Badge from '../../components/Badge.svelte';
  import Card from '../../components/Card.svelte';
  import { getSCMConnectionHealth } from '../../api/diagnostics.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatUtcTime } from './format-utils.js';

  let view = $state({ loading: true, error: null, connections: [] });
  let lastRefreshed = $state(null);

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const connections = await getSCMConnectionHealth();
      view = { loading: false, error: null, connections: connections ?? [] };
      lastRefreshed = new Date();
    } catch (error) {
      view = { ...view, loading: false, error: error?.message ?? String(error) };
    }
  }

  const unhealthy = $derived(view.connections.filter((connection) => connection.state === 'unhealthy'));

  const operationLabels = {
    repository_sync: 'Repository sync',
    pull_request_refresh: 'Pull request refresh',
  };

  function operationLabel(operation) {
    return operationLabels[operation] ?? operation;
  }

  function stateLabel(state) {
    return {
      healthy: 'Healthy',
      unhealthy: 'Unhealthy',
      disabled: 'Disabled',
      never_checked: 'Never checked',
    }[state] ?? state;
  }

  function stateVariant(state) {
    return {
      healthy: 'success',
      unhealthy: 'danger',
      disabled: 'neutral',
      never_checked: 'warning',
    }[state] ?? 'neutral';
  }

  function stateIcon(state) {
    return {
      healthy: IconCircleCheck,
      unhealthy: IconAlertTriangle,
      disabled: IconPlugOff,
      never_checked: IconClock,
    }[state] ?? IconClock;
  }

  function providerEndpoint(connection) {
    if (connection.provider_base_url) {
      try {
        return new URL(connection.provider_base_url).host;
      } catch {
        return connection.provider_base_url;
      }
    }
    return {
      github: 'github.com',
      gitlab: 'gitlab.com',
      bitbucket: 'bitbucket.org',
    }[connection.provider_type] ?? 'default endpoint';
  }

  function repositorySummary(connection) {
    const repositories = connection.repositories ?? [];
    const visible = repositories.slice(0, 5).map((repository) => repository.name);
    const remaining = repositories.length - visible.length;
    return `${visible.join(', ')}${remaining > 0 ? `, +${remaining} more` : ''}`;
  }
</script>

<DiagnosticsSection
  title="SCM connection health"
  subtitle="Latest repository sync and pull-request refresh results for each workspace connection. Repeated unchanged failures are retained here without repeating warning logs on every poll."
  dataTestId="diagnostics-scm-health"
  onLoad={load}
  lastRefreshed={lastRefreshed}
  bind:loading={view.loading}
  bind:error={view.error}
>
  {#snippet children()}
    {#if !view.loading && !view.error && unhealthy.length > 0}
      <Card>
        <div class="flex items-start gap-3 p-4" data-testid="scm-health-alert">
          <IconAlertTriangle class="h-6 w-6 flex-shrink-0 mt-0.5" style="color: var(--ds-text-danger);" />
          <div>
            <div class="font-semibold" style="color: var(--ds-text);">SCM attention required</div>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              {unhealthy.length} connection{unhealthy.length === 1 ? '' : 's'} unhealthy:
              {unhealthy.map((connection) => `${connection.workspace_name} / ${connection.provider_name}`).join(', ')}.
            </p>
          </div>
        </div>
      </Card>
    {/if}

    {#if !view.loading && !view.error && view.connections.length === 0}
      <Card variant="outlined">
        <p class="text-sm" style="color: var(--ds-text-subtle);">No SCM connections configured.</p>
      </Card>
    {:else if !view.error}
      <div class="space-y-4">
        {#each view.connections as connection (connection.id)}
          <Card variant="outlined">
            <div class="space-y-4" data-testid={`scm-health-connection-${connection.id}`}>
              <div class="flex items-start justify-between gap-3">
                <div>
                  <h4 class="font-semibold" style="color: var(--ds-text);">
                    {connection.workspace_name} ({connection.workspace_key})
                    <span class="font-normal" style="color: var(--ds-text-subtle);">— {connection.provider_name}</span>
                  </h4>
                  <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                    Connection #{connection.id} · {connection.provider_slug} · {providerEndpoint(connection)} · {connection.auth_method} ·
                    {connection.active_repository_count} active of {connection.repository_count} repositories
                  </p>
                  {#if connection.repositories?.length > 0}
                    <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                      Repositories: {repositorySummary(connection)}
                    </p>
                  {/if}
                </div>
                <Badge variant={stateVariant(connection.state)} icon={stateIcon(connection.state)}>
                  {stateLabel(connection.state)}
                </Badge>
              </div>

              <div class="grid gap-3 lg:grid-cols-2">
                {#each connection.operations as operation (`${connection.id}:${operation.operation}`)}
                  <div class="rounded-md border p-3" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
                    <div class="flex items-center justify-between gap-2">
                      <span class="text-sm font-medium" style="color: var(--ds-text);">{operationLabel(operation.operation)}</span>
                      <Badge variant={stateVariant(operation.state)} icon={stateIcon(operation.state)} size="xs">
                        {stateLabel(operation.state)}
                      </Badge>
                    </div>
                    <p class="text-xs mt-2 font-medium" style="color: var(--ds-text);">
                      Connection #{connection.id} · {connection.workspace_key} · {connection.provider_name} ({providerEndpoint(connection)})
                    </p>
                    <div class="text-xs mt-2 space-y-1" style="color: var(--ds-text-subtle);">
                      {#if operation.last_attempt_at}
                        <p>Last attempt: {formatUtcTime(operation.last_attempt_at)}</p>
                        <p>{operation.failed_resources} failed of {operation.checked_resources} checked</p>
                      {:else}
                        <p>This operation has not run for this connection.</p>
                      {/if}
                      {#if operation.consecutive_failures > 0}
                        <p><span>{operation.consecutive_failures} consecutive</span> failure{operation.consecutive_failures === 1 ? '' : 's'}</p>
                      {/if}
                    </div>
                    {#if operation.last_error}
                      <pre class="text-xs mt-3 p-2 rounded whitespace-pre-wrap break-words" style="color: var(--ds-text-danger); background: var(--ds-background-danger);">{operation.last_error}</pre>
                    {/if}
                  </div>
                {/each}
              </div>
            </div>
          </Card>
        {/each}
      </div>
    {/if}
  {/snippet}
</DiagnosticsSection>
