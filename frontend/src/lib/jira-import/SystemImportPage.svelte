<script>
  import { onMount } from 'svelte';
  import { jiraImport } from './JiraImportStore.svelte.js';
  import { toHotkeyString, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import JiraImportWizard from './JiraImportWizard.svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import {
    Cloud, Plus, Trash2, ExternalLink, Link, Clock,
    CheckCircle, XCircle, Loader, PlayCircle
  } from 'lucide-svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { addToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateTimeLocale } from '../utils/dateFormatter.js';
  import { confirm } from '../composables/useConfirm.js';

  // State
  let showWizard = $state(false);
  let selectedConnectionId = $state(null);

  // Derived state from store
  let savedConnections = $derived(jiraImport.savedConnections);
  let importJobs = $derived(jiraImport.importJobs);

  onMount(() => {
    jiraImport.loadSavedConnections();
    jiraImport.loadImportJobs();
  });

  function openWizard(connectionId = null) {
    selectedConnectionId = connectionId;
    jiraImport.reset();
    if (connectionId) {
      const conn = savedConnections.items.find(c => c.id === connectionId);
      if (conn) {
        jiraImport.useSavedConnection(conn);
      }
    }
    showWizard = true;
  }

  function closeWizard() {
    showWizard = false;
    selectedConnectionId = null;
    // Refresh the lists after wizard closes
    jiraImport.loadSavedConnections();
    jiraImport.loadImportJobs();
  }

  async function deleteConnection(connectionId) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: 'Are you sure you want to delete this connection? This action cannot be undone.',
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;
    const result = await jiraImport.deleteSavedConnection(connectionId);
    if (result.success) {
      addToast({ message: 'Connection deleted', variant: 'success' });
    } else {
      addToast({ message: result.error, variant: 'error' });
    }
  }

  function formatDate(dateString) {
    if (!dateString) return '-';
    return formatDateTimeLocale(dateString);
  }

  function getStatusColor(status) {
    switch (status) {
      case 'completed': return 'var(--ds-text-success)';
      case 'in_progress': return 'var(--ds-text-accent-blue)';
      case 'failed': return 'var(--ds-text-danger)';
      case 'queued': return 'var(--ds-text-subtle)';
      default: return 'var(--ds-text-subtle)';
    }
  }

  function getStatusIcon(status) {
    switch (status) {
      case 'completed': return CheckCircle;
      case 'in_progress': return Loader;
      case 'failed': return XCircle;
      case 'queued': return Clock;
      default: return Clock;
    }
  }
</script>

<div class="space-y-8">
  <!-- Page Header -->
  <PageHeader title="System Import" subtitle="Import data from Jira Cloud and other external systems" icon={Cloud}>
    {#snippet actions()}
      <Button variant="primary" onclick={() => openWizard()} keyboardHint={getShortcutDisplay('systemImport', 'add')} hotkeyConfig={{ key: toHotkeyString('systemImport', 'add'), guard: () => !showWizard }}>
        <Plus size={16} class="mr-2" />
        New Import
      </Button>
    {/snippet}
  </PageHeader>

  <!-- Saved Connections Section -->
  <div class="rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-2">
        <Link size={18} style="color: var(--ds-text-subtle);" />
        <h2 class="text-lg font-medium" style="color: var(--ds-text);">Saved Connections</h2>
      </div>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        Manage your Jira Cloud connections for importing data
      </p>
    </div>

    <div class="p-6">
      {#if savedConnections.isLoading}
        <div class="flex items-center justify-center py-8">
          <Spinner size="md" />
        </div>
      {:else if savedConnections.error}
        <AlertBox variant="error" message={savedConnections.error} />
      {:else if savedConnections.items.length === 0}
        <div class="text-center py-8">
          <Cloud class="w-12 h-12 mx-auto opacity-50" style="color: var(--ds-text-subtle);" />
          <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
            No saved connections yet. Start a new import to connect to Jira Cloud.
          </p>
        </div>
      {:else}
        <div class="space-y-3">
          {#each savedConnections.items as connection}
            <div class="p-4 rounded-lg border flex items-center justify-between"
                 style="border-color: var(--ds-border); background: var(--ds-surface);">
              <div class="flex items-center gap-4">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-blue-subtler);">
                  <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <span class="font-medium" style="color: var(--ds-text);">
                      {connection.instance_name || 'Jira Cloud'}
                    </span>
                    <a href={connection.instance_url}
                       target="_blank"
                       rel="noopener noreferrer"
                       class="hover:opacity-70">
                      <ExternalLink size={14} style="color: var(--ds-text-subtle);" />
                    </a>
                  </div>
                  <div class="flex items-center gap-3 mt-1">
                    <span class="text-xs" style="color: var(--ds-text-subtle);">
                      {connection.email}
                    </span>
                    {#if connection.last_used_at}
                      <span class="text-xs" style="color: var(--ds-text-subtle);">
                        Last used: {formatDate(connection.last_used_at)}
                      </span>
                    {/if}
                  </div>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <Button variant="secondary" size="small" onclick={() => openWizard(connection.id)}>
                  <PlayCircle size={14} class="mr-1" />
                  Start Import
                </Button>
                <Button variant="ghost" size="small" onclick={() => deleteConnection(connection.id)}>
                  <Trash2 size={14} style="color: var(--ds-text-danger);" />
                </Button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <!-- Import History Section -->
  <div class="rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-2">
        <Clock size={18} style="color: var(--ds-text-subtle);" />
        <h2 class="text-lg font-medium" style="color: var(--ds-text);">Import History</h2>
      </div>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        View the status and results of previous imports
      </p>
    </div>

    <div class="p-6">
      {#if importJobs.isLoading}
        <div class="flex items-center justify-center py-8">
          <Spinner size="md" />
        </div>
      {:else if importJobs.error}
        <AlertBox variant="error" message={importJobs.error} />
      {:else if importJobs.items.length === 0}
        <div class="text-center py-8">
          <Clock class="w-12 h-12 mx-auto opacity-50" style="color: var(--ds-text-subtle);" />
          <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
            No imports yet. Start a new import to see the history here.
          </p>
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr style="border-bottom: 1px solid var(--ds-border);">
                <th class="text-left py-3 px-4 text-xs font-medium uppercase tracking-wider"
                    style="color: var(--ds-text-subtle);">Status</th>
                <th class="text-left py-3 px-4 text-xs font-medium uppercase tracking-wider"
                    style="color: var(--ds-text-subtle);">Instance</th>
                <th class="text-left py-3 px-4 text-xs font-medium uppercase tracking-wider"
                    style="color: var(--ds-text-subtle);">Scope</th>
                <th class="text-left py-3 px-4 text-xs font-medium uppercase tracking-wider"
                    style="color: var(--ds-text-subtle);">Started</th>
                <th class="text-left py-3 px-4 text-xs font-medium uppercase tracking-wider"
                    style="color: var(--ds-text-subtle);">Completed</th>
              </tr>
            </thead>
            <tbody>
              {#each importJobs.items as job}
                {@const StatusIcon = getStatusIcon(job.status)}
                <tr style="border-bottom: 1px solid var(--ds-border);">
                  <td class="py-3 px-4">
                    <div class="flex items-center gap-2">
                      <StatusIcon size={16} style="color: {getStatusColor(job.status)};"
                                  class={job.status === 'in_progress' ? 'animate-spin' : ''} />
                      <span class="text-sm capitalize" style="color: {getStatusColor(job.status)};">
                        {job.status.replace('_', ' ')}
                      </span>
                    </div>
                    {#if job.phase && job.status === 'in_progress'}
                      <span class="text-xs mt-1 block" style="color: var(--ds-text-subtle);">
                        {job.phase}
                      </span>
                    {/if}
                    {#if job.error_message}
                      <span class="text-xs mt-1 block" style="color: var(--ds-text-danger);">
                        {job.error_message}
                      </span>
                    {/if}
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-sm" style="color: var(--ds-text);">
                      {job.instance_name || job.instance_url || '-'}
                    </span>
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-xs px-2 py-1 rounded capitalize"
                          style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                      {job.scope.replace('_', ' ')}
                    </span>
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-sm" style="color: var(--ds-text-subtle);">
                      {formatDate(job.started_at)}
                    </span>
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-sm" style="color: var(--ds-text-subtle);">
                      {formatDate(job.completed_at)}
                    </span>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Import Wizard Modal -->
<JiraImportWizard
  bind:isOpen={showWizard}
  onClose={closeWizard}
  onComplete={closeWizard}
/>
