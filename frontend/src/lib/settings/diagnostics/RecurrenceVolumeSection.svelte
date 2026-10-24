<script>
  import IconActivity from '@tabler/icons-svelte-runes/icons/activity';
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import IconCalendarRepeat from '@tabler/icons-svelte-runes/icons/calendar-repeat';
  import IconClock from '@tabler/icons-svelte-runes/icons/clock';
  import IconStack2 from '@tabler/icons-svelte-runes/icons/stack-2';
  import { getRecurrenceVolume, updateRecurrenceVolumeSettings } from '../../api/diagnostics.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import Card from '../../components/Card.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import FormField from '../../components/FormField.svelte';
  import Input from '../../components/Input.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import DiagnosticsSection from './DiagnosticsSection.svelte';

  let view = $state({ loading: true, error: null, data: null });
  let saving = $state(false);
  let diagnosticEnabled = $state(true);
  let warningThreshold = $state(80);

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const data = await getRecurrenceVolume();
      diagnosticEnabled = data.diagnostic_enabled;
      warningThreshold = data.warning_threshold;
      view = { loading: false, error: null, data };
    } catch (err) {
      view = { ...view, loading: false, error: err?.message ?? String(err) };
    }
  }

  async function saveSettings() {
    saving = true;
    try {
      await updateRecurrenceVolumeSettings({
        diagnostic_enabled: diagnosticEnabled,
        warning_threshold: Number(warningThreshold),
      });
      successToast('Recurrence diagnostic settings saved');
      await load();
    } catch (err) {
      errorToast(err?.message || 'Failed to save recurrence diagnostic settings');
    } finally {
      saving = false;
    }
  }

  const workspaces = $derived(view.data?.workspaces ?? []);
  const columns = [
    { key: 'key', label: 'Workspace', render: (workspace) => `${workspace.key} — ${workspace.name}` },
    { key: 'rules', label: 'Rules', render: (workspace) => String(workspace.rule_count) },
    { key: 'active', label: 'Active', render: (workspace) => String(workspace.active_count) },
    { key: 'capacity', label: 'Capacity', render: (workspace) => `${workspace.rule_count} / ${view.data?.hard_limit ?? 100}` },
    { key: 'health', label: 'Diagnostic', slot: 'health' },
  ];
</script>

<DiagnosticsSection
  title="Recurrence volume"
  subtitle="Rule cardinality by workspace and the active scheduler queue. The hard limit is fixed at 100 rules per workspace; this warning threshold only controls when administrators are alerted."
  dataTestId="diagnostics-recurrence-volume"
  onLoad={load}
  bind:loading={view.loading}
  bind:error={view.error}
  refreshInterval={30_000}
>
  {#snippet children()}
    {#if view.data && diagnosticEnabled && !view.data.healthy}
      <Card>
        <div class="flex items-start gap-3 p-3" style="color: var(--ds-accent-orange);" data-testid="recurrence-volume-alert">
          <IconAlertTriangle class="w-4 h-4 mt-0.5 shrink-0" />
          <span class="text-sm">
            Recurrence volume needs attention. Review workspaces at the warning threshold and any due-rule backlog that exceeds one scheduler batch.
          </span>
        </div>
      </Card>
    {/if}

    {#if view.data}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard icon={IconStack2} label="Total rules" value={String(view.data.total_rules)} color="blue" />
        <StatCard icon={IconActivity} label="Active rules" value={String(view.data.active_rules)} color="green" />
        <StatCard icon={IconClock} label="Due now" value={String(view.data.due_rules)} color={view.data.batch_backlogged ? 'orange' : 'purple'} />
        <StatCard icon={IconCalendarRepeat} label="Workspace hard limit" value={String(view.data.hard_limit)} color="orange" />
      </div>

      <Card variant="outlined" padding="default">
        <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div class="space-y-4">
            <Toggle
              bind:checked={diagnosticEnabled}
              label="Enable recurrence-volume warnings"
              dataTestid="recurrence-volume-enabled"
            />
            <FormField
              label="Warn at rules per workspace"
              helper="A warning appears when a workspace reaches this count. Allowed range: 1–100."
              id="recurrence-volume-threshold"
              class="mb-0"
            >
              <Input
                id="recurrence-volume-threshold"
                type="number"
                min="1"
                max={view.data.hard_limit}
                bind:value={warningThreshold}
                disabled={!diagnosticEnabled}
                class="max-w-28"
              />
            </FormField>
          </div>
          <Button
            variant="primary"
            onclick={saveSettings}
            loading={saving}
            dataTestid="recurrence-volume-save"
          >
            Save diagnostic settings
          </Button>
        </div>
      </Card>

      <Card variant="outlined" padding="compact">
        <div class="text-sm" style="color: var(--ds-text-subtle);">
          Scheduler batch capacity: <strong style="color: var(--ds-text);">{view.data.scheduler_batch_size} rules</strong>.
          {#if view.data.batch_backlogged}
            <span style="color: var(--ds-text-danger);">
              {view.data.due_rules - view.data.scheduler_batch_size} due rules exceed one batch.
            </span>
          {:else}
            The current due queue fits in one pass.
          {/if}
        </div>
      </Card>

      <DataTable
        data={workspaces}
        {columns}
        keyField="workspace_id"
        emptyMessage="No recurrence rules configured."
      >
        {#snippet health(workspace)}
          {#if workspace.at_capacity}
            <Lozenge appearance="error" size="sm">At capacity</Lozenge>
          {:else if workspace.warning}
            <Lozenge appearance="warning" size="sm">Warning</Lozenge>
          {:else}
            <Lozenge appearance="success" size="sm">Healthy</Lozenge>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</DiagnosticsSection>
